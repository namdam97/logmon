# eBPF — Auto-instrumentation không cần sửa code
> Module ADV-4 · kernel-level telemetry, zero-code, khi nào dùng vs SDK · Độ khó: 🥇 (nâng cao) · Prereqs: OBS-3

> ⚠️ **Trạng thái trong LogMon: PLANNED / định hướng.** Không một dòng nào trong `doc_v2/` nhắc tới eBPF, Beyla/OBI, Pixie hay Cilium (đã `grep` toàn bộ). Bài này dạy kỹ năng và vạch **đích đến**: eBPF *bổ sung* (không thay thế) lớp OTel SDK đã có ở OBS-3, lấp các khoảng mù mà SDK không với tới. Mọi đề xuất tích hợp dưới đây là thiết kế tương lai, neo vào kiến trúc hiện hữu của LogMon.

## 1. Vì sao kỹ năng này quan trọng trong LogMon

OBS-3 đã cho LogMon distributed tracing **chất lượng cao** nhờ OTel SDK: `tracing.go` dựng TracerProvider, `otelgin` tạo server span, `otelpgx` tạo DB span. Nhưng cách tiếp cận đó có một tiền đề: **bạn phải sửa được code và build lại service**. Tiền đề này gãy ở ba chỗ thực tế trong một nền tảng observability:

1. **Service không phải của bạn.** LogMon giám sát "Go microservices" của *khách hàng/team khác*. Họ có thể chưa nhúng OTel SDK, hoặc viết bằng ngôn ngữ khác, hoặc là container bên thứ ba (Postgres, Redis, Nginx) — bạn không có source để thêm `otel.Tracer(...)`.
2. **Hạ tầng mạng (L3–L7) nằm ngoài tầm SDK.** SDK đo từ *bên trong* app: thời gian nó nghĩ request bắt đầu. Nó **không thấy** thời gian request nằm chờ trong accept queue, TCP retransmit, hay DNS chậm. Đó đúng là phần khách hàng *thực sự* cảm nhận.
3. **Chi phí onboarding.** Bắt mọi service nhúng SDK + redeploy để "có observability" là rào cản. eBPF cho bạn một baseline RED metrics + service map **trong vài phút, không sửa code** — rồi mới đầu tư SDK cho service quan trọng.

eBPF biến chính **kernel Linux** thành điểm quan sát: nó hook vào syscall/uprobe để thấy mọi byte vào/ra socket của *mọi* process trên host, không cần app hợp tác. Với LogMon, đây là con đường mở rộng vùng phủ observability ra ngoài các service ta tự viết — và là tiền đề cho GĐ5 (AI chẩn đoán cần dữ liệu càng rộng càng tốt).

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Quên Beyla, quên Pixie. Câu hỏi gốc: *làm sao quan sát một chương trình mà không sửa nó, cũng không làm nó chậm đáng kể?*

Mọi việc "có ý nghĩa" của một service đều đi qua **kernel**: đọc/ghi socket (`read`/`write`/`sendto`), mở file, cấp phát — đều là **system call**. Nếu ta đặt được một "máy quay" ngay tại các cửa ngõ kernel đó, ta thấy toàn bộ traffic mà *không* chạm vào code ứng dụng.

**eBPF (extended Berkeley Packet Filter)** chính là cơ chế đó. Hình dung nó như một **VM sandbox tí hon chạy bên trong kernel**:

- Bạn nạp một chương trình eBPF nhỏ, gắn vào một **hook** (kprobe = hàm kernel, uprobe = hàm trong binary userspace, tracepoint, XDP = sớm nhất trên đường nhận gói).
- Khi luồng thực thi chạm hook (ví dụ process gọi `write()` lên socket), chương trình eBPF chạy, đọc đối số, ghi dữ liệu vào một **eBPF map** (vùng nhớ chia sẻ kernel↔userspace).
- Một agent userspace đọc map đó, ráp thành span/metric, rồi xuất đi (với LogMon: **xuất OTLP** về đúng Collector đã có ở OBS-3).

Hai tính chất làm eBPF an toàn để chạy production — khác hẳn việc tự viết kernel module:
- **Verifier**: trước khi nạp, kernel chứng minh tĩnh rằng chương trình *kết thúc* (không vòng lặp vô hạn), không truy cập bộ nhớ ngoài phạm vi. Không pass verifier → không nạp được. Đây là vì sao eBPF không crash kernel như module thường.
- **JIT + chi phí thấp**: chương trình được biên dịch sang mã máy, chạy ở tốc độ gần native. Quan sát ở kernel nên **không cần sidecar proxy** — đây là lý do eBPF cắt tới ~90% overhead so với telemetry kiểu service-mesh sidecar.

Điểm mấu chốt cần nội hoá: **eBPF quan sát *cú pháp giao thức trên dây* (wire), không quan sát *ngữ nghĩa nghiệp vụ trong process*.** Nó biết "có một HTTP POST /login mất 380ms", nhưng *không* biết "380ms đó là do Argon2id hashing" — vì hashing là logic trong process, không lộ ra socket. Toàn bộ trade-off ở mục 6 nảy ra từ ranh giới này.

## 3. Khái niệm cốt lõi (tăng dần độ khó)

### 3.1 Hook: kprobe / uprobe / tracepoint
- **kprobe/tracepoint**: gắn vào hàm/sự kiện *kernel* (vd `tcp_sendmsg`) — thấy mọi process, ngôn ngữ-agnostic.
- **uprobe**: gắn vào *symbol trong binary userspace* (vd hàm `crypto/tls.(*Conn).Write` của Go). Mạnh hơn: đọc được dữ liệu *trước khi mã hoá* — đây là cách OBI/Beyla lấy được HTTP path từ traffic HTTPS của một Go binary (xem 3.4).

### 3.2 RED metrics & service map "miễn phí"
Vì eBPF thấy mọi cặp request/response trên socket, nó tự suy ra **Rate / Errors / Duration** per service-endpoint và vẽ **service graph** (ai gọi ai) — *không* cần app emit metric. Khác biệt quan trọng với spanmetrics (OBS-3 §2.3): spanmetrics phái sinh RED từ *span đã sample*; eBPF đo trên **100% request** ở tầng mạng nên baseline không bị thiên lệch do sampling.

### 3.3 Network observability (L3/L4)
eBPF thấy thứ SDK mù: TCP RTT, retransmit, connection refused, thời gian chờ trong queue. Ví dụ kinh điển: thread pool cạn → SDK báo latency phẳng (vì nó chỉ tính từ lúc handler chạy), eBPF báo spike (vì tính cả thời gian request *xếp hàng* trước handler). eBPF đo cái khách hàng thật sự chịu.

### 3.4 Hai bức tường khó nhất: TLS và context propagation
- **TLS/mã hoá**: trên dây traffic là ciphertext → eBPF kprobe ở tầng socket chỉ thấy byte mã hoá, vô dụng cho L7. **Lối thoát**: uprobe vào *hàm TLS của runtime* (vd Go `crypto/tls`, OpenSSL) để đọc plaintext *trước khi mã hoá*. Hiệu quả với binary biên dịch tĩnh như Go; khó/không làm được với TLS phần cứng, library lạ, hoặc kernel bật **lockdown/Secure Boot** (chặn `bpf_probe_write_user`).
- **Distributed context propagation**: để nối span across service, cần truyền `traceparent` (W3C, xem OBS-3 §3.2). SDK làm tự nhiên qua header. eBPF phải **chèn header vào request đang bay** — khó vì với HTTPS dữ liệu đã/đang mã hoá, và với async (goroutine pool) phải lần được "goroutine nào đẻ ra goroutine nào". Beyla có *goroutine lineage tracking* cho việc này, nhưng đây là **giới hạn lớn nhất phải hiểu trước**: tracing eBPF "thuần" thường mới chỉ nối tốt trong phạm vi hẹp; trace đa-service chất lượng cao vẫn cần SDK hoặc lai.

### 3.5 Bản đồ công cụ (2025–2026)
| Công cụ | Vai trò chính | Output | Liên hệ LogMon |
|---|---|---|---|
| **OBI** (OpenTelemetry eBPF Instrumentation) | Auto-instrument app → traces + RED | **OTLP native** | Khớp nhất: xuất thẳng về Collector OBS-3 |
| **Grafana Beyla** | Distribution của Grafana cho OBI | OTLP / Prometheus | Beyla 2.x = OBI + extras |
| **Cilium + Hubble** | CNI + network L3–L7 flow visibility | Hubble flows / metrics | Khi LogMon lên K8s (K8S-1/2): network map |
| **Tetragon** | Security/runtime observability + enforcement | events | Ngoài phạm vi traces, hướng security |
| **Pixie** | Auto-APM + service map, lưu in-cluster | scriptable (PxL) | Mạnh debug ad-hoc; kho lưu riêng (khác mô hình OTLP-về-Collector của LogMon) |

> Mốc quan trọng: tháng 5/2025 Grafana **donate Beyla cho OpenTelemetry** thành **OBI** (CNCF). Hệ quả cho LogMon: chọn công cụ **xuất OTLP native** (OBI/Beyla) để cắm thẳng vào pipeline OTel hiện có, tránh khoá vào kho lưu độc quyền.

## 4. LogMon dùng/sẽ dùng nó thế nào (bám doc_v2 + code; ghi rõ implemented/planned)

**Nền tảng đã có (IMPLEMENTED) — eBPF sẽ cắm vào đây, không phá vỡ:**
- OTLP gRPC `:4317` ở `infra/otel/agent.yaml`, gateway chạy `tail_sampling` rồi xuất Jaeger (`infra/otel/gateway.yaml`) — đối chiếu OBS-3 §4. Bất kỳ nguồn span nào **nói được OTLP** đều dùng lại nguyên si đường ống này.
- TracerProvider + W3C propagator trong `backend/internal/shared/tracing/tracing.go` (`New()`, dòng 46–79); correlation `trace_id`↔log trong `logger.go`. Đây là lớp SDK chất lượng cao mà eBPF **bổ trợ**, không thay.
- Quy ước metrics naming + CẤM high-cardinality label (`doc_v2/04 §1.3`) — ràng buộc này áp luôn cho RED metrics do eBPF sinh.

**Đích đến (PLANNED) — chưa có code, là thiết kế đề xuất:**
- **OBI/Beyla như nguồn OTLP thứ hai.** Triển khai OBI cạnh các service (đặc biệt các service *chưa* nhúng SDK, hoặc demo `examples/demo-order/`), cấu hình `OTEL_EXPORTER_OTLP_ENDPOINT` của OBI trỏ vào **đúng `otel-agent:4317`** đã có. Span/RED của OBI chảy chung pipeline `tail_sampling` → Jaeger, hiện ngay trên `traces-explorer`/`service-overview` (`doc_v2/04 §4`).
- **Mô hình lai (hybrid) chính thức.** Đây là quyết định kiến trúc cốt lõi: **SDK cho service lõi LogMon** (identity/alerting/slo… — nơi cần custom business span như `verify-password`, đúng layer direction trong CLAUDE.md), **eBPF cho vùng phủ rộng** (service bên thứ ba, baseline RED, network L4). Khi chạy cả hai cho *cùng* một service: tắt span-metrics phía SDK (`span.metrics.skip=true`) để eBPF làm baseline RED không thiên lệch, SDK lo trace độ phân giải cao — tránh đếm trùng.
- **Network layer khi lên K8s.** OBS không có file nào nói K8s networking sâu; K8S-1/2 (`doc_tech/kubernetes/`) là chỗ Cilium + Hubble bổ sung service-to-service flow/RTT mà SDK và otelgin không thấy. Là *option*, chưa cam kết — `doc_v2` chưa chọn CNI.
- **Ranh giới layer (CLAUDE.md).** eBPF/OBI là **tiến trình hạ tầng ngoài binary Go** (daemon/sidecar trên host) — nó *không* phải Go package, *không* import vào BC nào, *không* vi phạm `adapters → ports ← app → domain`. Đúng tinh thần GĐ5: thành phần ngoài Go core, tích hợp qua OTLP/event.

**Yêu cầu vận hành cần ghi nhận từ bây giờ (sẽ vào `doc_v2/10`/`16` khi hiện thực):** kernel Linux ≥ 5.8 + BTF; cần `CAP_BPF`/`CAP_PERFMON` (hoặc root) — xung đột với pod security mặc định, phải xin cấp quyền có kiểm soát; lockdown/Secure Boot có thể chặn uprobe-TLS.

## 5. Best practices (mỗi mục kèm 1 nguồn đã research)

1. **Coi eBPF và SDK là bổ sung, không phải thay thế.** eBPF cho vùng phủ rộng + baseline mạng; SDK cho business span + log correlation. LogMon đã có SDK (OBS-3) → eBPF lấp khoảng mù, không viết lại. — [Why OpenTelemetry instrumentation needs both eBPF and SDKs (Grafana)](https://grafana.com/blog/why-opentelemetry-instrumentation-needs-both-ebpf-and-sdks/)
2. **Chọn công cụ xuất OTLP native (OBI/Beyla), cắm vào Collector sẵn có.** Tránh khoá vào kho lưu độc quyền; tái dùng `tail_sampling` + Jaeger. — [OpenTelemetry eBPF Instrumentation (OBI) docs](https://opentelemetry.io/docs/zero-code/obi/)
3. **Đừng kỳ vọng trace đa-service "tự động hoàn toàn".** Context propagation qua eBPF còn giới hạn (async, TLS); trace chất lượng cao vẫn cần SDK truyền `traceparent`. Hiểu giới hạn này *trước khi* hứa hẹn. — [Announcing Beta of OpenTelemetry Go Auto-Instrumentation using eBPF](https://opentelemetry.io/blog/2025/go-auto-instrumentation-beta/)
4. **Khi chạy lai cùng một service, tắt span-metrics phía SDK** (`span.metrics.skip=true`) để eBPF làm RED baseline, tránh đếm trùng. — [Why OpenTelemetry needs both eBPF and SDKs (Grafana)](https://grafana.com/blog/why-opentelemetry-instrumentation-needs-both-ebpf-and-sdks/)
5. **Lập kế hoạch đặc quyền & kernel sớm.** Cần Linux ≥5.8 + BTF, `CAP_BPF`/root; va chạm K8s Pod Security & lockdown. Audit trước khi POC. — [OpenTelemetry eBPF Instrumentation requirements (OBI)](https://opentelemetry.io/docs/zero-code/obi/)
6. **Tách trách nhiệm theo lớp khi lên K8s**: Cilium/Hubble cho network L3–L7, OBI/Beyla cho app traces+RED — không gom làm một. — [eBPF Observability 2026 deep dive (Pixie/Hubble/Beyla)](https://www.youngju.dev/blog/culture/2026-05-15-ebpf-observability-2026-pixie-parca-cilium-hubble-tetragon-beyla-deep-dive.en)

## 6. Lỗi thường gặp & anti-patterns

- **"eBPF thay được SDK, gỡ hết instrumentation thủ công."** Sai. Mất custom business span (`verify-password`), mất runtime metrics (GC/heap), mất correlation `trace_id` trong log app — đúng những thứ OBS-3 đang dựa vào. eBPF *thêm*, không *thay*.
- **Kỳ vọng eBPF đọc được mọi HTTPS.** Không decrypt được TLS ở tầng socket; chỉ uprobe runtime mới lấy plaintext, và thất bại khi lockdown/Secure Boot/library lạ. Đừng hứa "100% traffic mã hoá" trước khi POC trên đúng kernel target.
- **Trace eBPF đứt khúc rồi đổ lỗi công cụ.** Phần lớn là context propagation async/cross-service chưa nối. Giải pháp đúng là **lai**: SDK truyền `traceparent`, eBPF bù span tầng dưới — chứ không phải bỏ eBPF.
- **Đếm trùng RED metrics.** Bật cả spanmetrics (SDK) lẫn RED (eBPF) cho cùng service mà không tắt một bên → số liệu gấp đôi, dashboard sai.
- **Nhồi high-cardinality vào attribute eBPF** (`user_id`, raw path). Vi phạm `doc_v2/04 §1.3`; eBPF dễ phát sinh nhiều endpoint → phải normalize path template, đúng như SDK.
- **Cấp `--privileged` bừa cho agent eBPF.** Mở rộng bề mặt tấn công cả node. Dùng capability tối thiểu (`CAP_BPF`/`CAP_PERFMON`), không root toàn phần — vi phạm `doc_v2/09` nếu làm ẩu.
- **Bỏ qua chi phí kernel.** Quá nhiều uprobe trên hot path vẫn tốn CPU; phải đo overhead trước/sau (xem mục 8, `ecc:performance-optimizer`).

## 7. Lộ trình luyện tập (🥉→🥈→🥇 — vì chủ đề PLANNED, task là thiết kế/POC trong repo LogMon)

### 🥉 Cơ bản — hiểu & quan sát
1. Chạy `make up-demo` (có `demo-order` + loadgen). Vẽ tay sơ đồ: với service này, **SDK** thấy gì, **eBPF** sẽ thấy thêm gì (TCP RTT, queue time, traffic tới Postgres mà app không log).
2. Đọc `tracing.go` (OBS-3) và viết 5 dòng: nếu thêm OBI cạnh `userservice`, dữ liệu OBI sẽ trùng/khác gì so với span `otelgin`+`otelpgx` đang có?
3. Liệt kê pre-flight kernel cho máy dev của bạn: `uname -r` (≥5.8?), kiểm tra BTF (`ls /sys/kernel/btf/vmlinux`), Secure Boot có bật không. Ghi kết quả vào một note POC.

### 🥈 Trung cấp — POC cắm vào pipeline LogMon
1. Viết bản **draft compose service** `obi` (image `otel/ebpf-instrument` hoặc `grafana/beyla`) cho `examples/demo-order/`, set `OTEL_EXPORTER_OTLP_ENDPOINT=otel-agent:4317`, đặt sau profile `observability`. Mục tiêu: span OBI hiện trên Jaeger `:16686` *mà không sửa demo-order*.
2. So sánh trên Jaeger: trace của `userservice` (SDK) vs `demo-order` (eBPF) — chỉ ra chỗ eBPF *thiếu* DB child span chi tiết và custom attribute. Viết bảng đối chiếu.
3. Cấu hình OBI chỉ giám sát đúng cổng/đường dẫn cần (tránh `/healthz`, `/metrics`) — ánh xạ với logic `shouldTrace` của SDK ở OBS-3.

### 🥇 Nâng cao — kiến trúc lai + đề xuất ADR
1. **Soạn ADR đề xuất** (theo mẫu `doc_v2/13-adr.md`): "eBPF auto-instrumentation cho LogMon" — quyết định OBI vs Beyla vs Pixie, mô hình lai (SDK lõi + eBPF vùng phủ), `span.metrics.skip=true` để tránh đếm trùng, ràng buộc capability/kernel, ảnh hưởng `doc_v2/04/09/10/16`.
2. **Thiết kế POC TLS-aware** trên `userservice` (Go, binary tĩnh): chứng minh OBI dùng uprobe đọc được HTTP path từ traffic HTTPS *trước* mã hoá; ghi lại điều kiện thất bại (lockdown).
3. **Phác mô hình lai trace nối liền**: SDK truyền `traceparent` từ `userservice` → một service eBPF-only, kiểm tra trace có nối thành một waterfall hay đứt; ghi nhận giới hạn context propagation.
4. **Mở rộng K8s (gắn K8S-1/2)**: phác chỗ Cilium+Hubble cung cấp network flow mà OBI/SDK không thấy, và ranh giới trách nhiệm 3 lớp (network / app-trace / security).

## 8. Skill/agent ECC nên dùng

- **ecc:architect** — *dùng trước tiên*: quyết định mô hình lai (SDK lõi vs eBPF vùng phủ), nơi đặt OBI (sidecar/daemon ngoài Go core), giữ `adapters → ports ← app → domain` và quy tắc "AI/hạ tầng ngoài core" của GĐ5. Dùng khi soạn ADR ở bài 🥇.
- **ecc:latency-critical-systems** — phân tích đúng giá trị cốt lõi của eBPF: đo *thật* phần latency khách hàng chịu (queue time, TCP RTT) mà SDK bỏ sót; suy luận tail latency, head-of-line blocking, đối chiếu spanmetrics (sample) vs RED eBPF (100%).
- **ecc:performance-optimizer** — đo overhead eBPF (uprobe trên hot path) trước/sau, sizing agent, xác nhận eBPF không làm chậm node; dùng *sau* khi có baseline.
- **ecc:go-review** — review draft compose/config và bất kỳ glue code Go nào đọc telemetry từ OBI (không nuốt error ở adapter boundary, đúng style guide).

## 9. Tài nguyên học thêm (link đã research)

- [Why OpenTelemetry instrumentation needs both eBPF and SDKs — Grafana Labs](https://grafana.com/blog/why-opentelemetry-instrumentation-needs-both-ebpf-and-sdks/) — luận điểm trung tâm: lai, không thay thế; ví dụ thread-pool latency.
- [OpenTelemetry eBPF Instrumentation (OBI) — docs](https://opentelemetry.io/docs/zero-code/obi/) — signals (traces/RED/network), ngôn ngữ/giao thức, yêu cầu kernel ≥5.8/BTF + capability, giới hạn TLS.
- [Why we donated Grafana Beyla to OpenTelemetry — Grafana Labs (5/2025)](https://grafana.com/blog/2025/05/07/opentelemetry-ebpf-instrumentation-beyla-donation/) — mốc Beyla → OBI/CNCF, vì sao chọn OTLP-native.
- [Announcing the Beta of OpenTelemetry Go Auto-Instrumentation using eBPF (2025)](https://opentelemetry.io/blog/2025/go-auto-instrumentation-beta/) — giới hạn context propagation Go, uprobe TLS, hiện trạng beta.
- [grafana/beyla — GitHub](https://github.com/grafana/beyla) — distribution Beyla, cấu hình thực tế, ví dụ deploy.
- [eBPF Observability 2026: Pixie/Hubble/Tetragon/Beyla deep dive](https://www.youngju.dev/blog/culture/2026-05-15-ebpf-observability-2026-pixie-parca-cilium-hubble-tetragon-beyla-deep-dive.en) — so sánh công cụ, mô hình stack lai cho K8s.
- [eBPF-based Network Observability: Cilium Hubble & alternatives](https://www.cloudraft.io/blog/ebpf-based-network-observability-using-cilium-hubble) — lớp network L3–L7 khi LogMon lên K8s.

## 10. Checklist "đã hiểu"

- [ ] Giải thích được eBPF là gì (VM sandbox trong kernel + verifier + map) và vì sao nó an toàn/chi phí thấp để chạy production.
- [ ] Phân biệt kprobe/tracepoint (kernel, ngôn ngữ-agnostic) vs uprobe (symbol userspace, đọc plaintext trước TLS).
- [ ] Nói rõ eBPF quan sát *wire/giao thức*, **không** quan sát *ngữ nghĩa nghiệp vụ* — và mọi trade-off nảy ra từ ranh giới đó.
- [ ] Nêu được ≥3 thứ eBPF thấy mà SDK mù (queue time, TCP RTT, traffic service bên thứ ba) và ≥3 thứ SDK làm mà eBPF không (custom business span, GC/runtime metric, log↔trace correlation).
- [ ] Trình bày được hai bức tường khó: TLS (cần uprobe runtime) và distributed context propagation (async/cross-service).
- [ ] Khẳng định LogMon chọn **mô hình lai**: SDK cho lõi (đã có OBS-3), eBPF cho vùng phủ — và cách tránh đếm trùng RED (`span.metrics.skip=true`).
- [ ] Chỉ ra eBPF cắm vào LogMon *qua OTLP về `otel-agent:4317`* (tái dùng `tail_sampling`/Jaeger), không phải Go package, không vi phạm layer direction.
- [ ] Phân biệt rõ: trong LogMon hiện tại đây là **PLANNED** (không có trong `doc_v2`/code) — SDK tracing mới là IMPLEMENTED.
- [ ] Biết pre-flight bắt buộc: kernel ≥5.8 + BTF, capability tối thiểu (`CAP_BPF`), va chạm Pod Security/Secure Boot.
