# OTel Collector & Tail Sampling chuyên sâu
> Module ADV-2 · agent vs gateway, pipeline config, head vs tail sampling · Độ khó: 🥇 (nâng cao) · Prereqs: OBS-3

## 1. Vì sao kỹ năng này quan trọng trong LogMon

OBS-3 dạy *làm sao một service phát ra trace*. Module này lùi một bước: **toàn bộ telemetry — logs, metrics, traces — đi đâu sau khi rời service, ai quyết định giữ/bỏ, tốn bao nhiêu RAM/disk.** Trong LogMon, "ai" đó là **OpenTelemetry Collector** — điểm thắt cổ chai có chủ đích của cả nền tảng. Nắm nó là nắm 3 đòn bẩy:

1. **Tách app khỏi backend.** Service chỉ biết "bắn OTLP sang `otel-agent:4317`". Đổi Jaeger→Tempo hay thêm Kafka (Mode B) → **không một dòng Go nào đổi**, chỉ sửa YAML. Đây là lý do `tracing.go` chỉ ~130 dòng và hoàn toàn vendor-neutral.
2. **Kiểm soát chi phí.** Lưu 100% trace của hệ thống vài nghìn req/s sẽ đốt cháy ES. Tail sampling giữ **100% trace lỗi + chậm** nhưng chỉ **10% trace nhàm chán** — giữ giá trị chẩn đoán, cắt 80-90% dung lượng.
3. **Một binary cho 3 tín hiệu.** ADR-018: LogMon bỏ Filebeat + Logstash (tốn 1.5-3.5 GB RAM chỉ để parse JSON) vì collector vốn đã cần cho traces.

Tail sampling còn là tiền đề GĐ5 (AI chẩn đoán incident): corpus trace để AI học phải *đậm đặc lỗi*, không loãng bởi 90% trace 200-OK. Sai sampling → AI học rác.

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Quên YAML đi. Một OTel Collector chỉ là **một ống nước (pipeline) với ba loại khớp nối**:

```
[receivers] ──▶ [processors] ──▶ [exporters]
  (nhận)          (biến đổi)        (gửi đi)
```

- **Receiver** = miệng vào. Biết *một giao thức cụ thể*: `otlp` nói gRPC/HTTP OTLP, `filelog` đọc file trên đĩa, `kafka` đọc topic. Nhiệm vụ duy nhất: biến dữ liệu ngoài thành mô hình dữ liệu nội bộ của OTel (`pdata`).
- **Processor** = trạm xử lý *theo thứ tự*. `memory_limiter` chặn OOM, `batch` gom lô, `transform` đổi field, `tail_sampling` quyết định giữ/bỏ. Thứ tự trong list **là** thứ tự thực thi — đây là điểm gây lỗi nhiều nhất.
- **Exporter** = miệng ra. `elasticsearch` ghi vào ES, `otlp/jaeger` bắn sang Jaeger, `prometheus` expose `/metrics`.

Ba khớp này lắp thành **pipeline**, và **một collector có nhiều pipeline song song, mỗi pipeline một loại tín hiệu**: `logs`, `traces`, `metrics`. Chúng dùng chung receiver/exporter nhưng chạy độc lập. Trong `gateway.yaml` của LogMon có đúng 2 pipeline: `logs:` (→ Elasticsearch) và `traces:` (→ Jaeger).

Khái niệm thứ tư: **connector** — vừa là exporter của pipeline này vừa là receiver của pipeline kia, nó *nối hai pipeline*. `spanmetrics` connector nhận span cuối pipeline `traces` rồi *đẻ ra* metrics RED ở đầu pipeline `metrics` — cách LogMon sinh metrics latency/throughput per-endpoint **không cần code đo thủ công**.

First-principle cuối, quan trọng nhất: **sản xuất span (SDK) tách rời quyết định giữ span (Collector)**. SDK LogMon cố tình `AlwaysSample` — xuất *hết* (`tracing.go:99-104`); việc cắt tỉa đẩy hết về gateway. Vì sao? Chỉ khi *trace đã hoàn tất* mới biết nó lỗi/chậm hay không — đó chính là head vs tail sampling (§3.4).

## 3. Khái niệm cốt lõi (tăng dần độ khó)

### 3.1 Pipeline & data model `pdata`
Mọi thứ trong collector là `pdata` (`ptrace.Span`, `plog.LogRecord`, `pmetric.Metric`). Processor thao tác trên `pdata`, không trên JSON thô. Hiểu điều này giúp đọc được tại sao `transform` processor dùng cú pháp OTTL (`set(attributes[...], ...)`) chứ không phải sed.

### 3.2 `memory_limiter` & `batch` — luôn đi cùng nhau
`memory_limiter` (luôn đặt **đầu tiên**) kiểm tra RAM mỗi `check_interval` và bắt đầu *từ chối nhận* (back-pressure) khi vượt `limit_mib`, tránh OOM giết cả collector. `batch` (luôn đặt **gần cuối**) gom record thành lô để giảm số lần gọi network/ES. LogMon: agent `limit_mib: 192`, gateway `384`, cả hai `batch: 5s / 512`.

### 3.3 Mô hình agent → gateway
LogMon dùng đúng **agent-to-gateway pattern** mà OTel khuyến nghị:

| | Agent (`agent.yaml`) | Gateway (`gateway.yaml`) |
|---|---|---|
| Chạy ở đâu | Mỗi host (DaemonSet ở prod) | Trung tâm, scale riêng |
| Nhận | `filelog` + `otlp` | `otlp` (từ agents) |
| Xử lý | parse log, `memory_limiter`, `batch` | `tail_sampling`, `transform`, route ES |
| Gửi tới | `otlp/gateway` (thô) | Elasticsearch + Jaeger |

Lý do tách: việc *nặng và có trạng thái* (tail sampling cần buffer toàn bộ trace) gom về một chỗ scale được; agent giữ nhẹ để chạy dày đặc trên mọi host. Đây là kiến trúc HA chuẩn ([OTel docs](https://opentelemetry.io/docs/collector/deploy/other/agent-to-gateway/)).

### 3.4 Head sampling vs Tail sampling — tâm điểm module

| | Head sampling | **Tail sampling** |
|---|---|---|
| Quyết định khi | Span *bắt đầu* (SDK) | Trace *hoàn tất* (gateway) |
| Biết trace lỗi/chậm? | **Không** | **Có** |
| Buffer? | Không cần | Cần giữ cả trace trong RAM tới khi quyết |
| RAM | Rẻ | Tốn (xem §3.6) |
| Vị trí trong LogMon | SDK `AlwaysSample` | `tail_sampling` ở gateway |

Head sampling (vd `TraceIDRatioBased(0.1)`) quyết định *trước khi biết kết quả* — nó có thể vứt đúng cái trace bị lỗi mà bạn cần. Tail sampling đợi tới khi cả trace về đủ rồi mới phán. Đánh đổi: phải **giữ tất cả span của một trace trong RAM** suốt `decision_wait` giây.

### 3.5 Anatomy của `tail_sampling` trong LogMon
`gateway.yaml:37-49`:

```yaml
tail_sampling:
  decision_wait: 10s        # đợi 10s sau span đầu rồi mới phán cho cả trace
  num_traces: 50000         # số trace giữ đồng thời trong RAM
  policies:
    - name: errors-always   # giữ MỌI trace có span status=ERROR
      type: status_code
      status_code: { status_codes: [ERROR] }
    - name: slow-requests   # giữ MỌI trace > 1000ms
      type: latency
      latency: { threshold_ms: 1000 }
    - name: probabilistic-default  # còn lại: giữ 10%
      type: probabilistic
      probabilistic: { sampling_percentage: 10 }
```

**Quy tắc vàng về policy: OR + first-match.** Một trace được *giữ* nếu khớp **bất kỳ** policy nào. Vì vậy đặt policy "có chủ đích" (errors, slow) lên trước, `probabilistic` catch-all xuống cuối. Một trace lỗi *và* chậm vẫn chỉ được giữ một lần.

### 3.6 Bài toán RAM của tail sampling (kiến thức 🥇)
Công thức ước lượng ([OTel contrib README](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/tailsamplingprocessor/README.md)):

```
RAM ≈ avg_trace_size × num_traces
    ≈ avg_trace_size × traces_per_sec × decision_wait
```

Hệ quả trực tiếp: `num_traces: 50000` *và* `decision_wait: 10s` của LogMon ngầm giả định tải ≤ 5000 trace/s. Tăng tải → trace bị **drop trước khi kịp quyết** (mất oan, kể cả trace lỗi). Theo dõi metric `otelcol_processor_tail_sampling_sampling_trace_removal_age`: nếu nó tiệm cận `decision_wait`, bạn đang ở mép vực — hoặc tăng RAM/`num_traces`, hoặc giảm `decision_wait`.

### 3.7 Ràng buộc vàng khi scale: "cả trace về một gateway"
Tail sampling chỉ đúng nếu **mọi span của một trace tới cùng một gateway instance**. Một gateway (dev LogMon hiện tại) → đương nhiên thỏa. Nhiều gateway → cần tầng `loadbalancing` exporter ở agent, route theo `routing_key: traceID`, để mọi span cùng `trace_id` rơi vào đúng một gateway ([OTel gateway pattern](https://opentelemetry.io/docs/collector/deploy/gateway/)). Đây là **planned** trong LogMon (doc_v2/04 §2.2 ghi rõ điều kiện kích hoạt).

## 4. LogMon dùng/sẽ dùng nó thế nào (bám doc_v2 + code; ghi rõ implemented/planned)

Phải phân biệt rạch ròi — module này **một nửa đã chạy, một nửa là đích**.

**ĐÃ IMPLEMENTED (có file, `make up-full` chạy được):**
- `infra/otel/agent.yaml` + `gateway.yaml`: hai collector contrib `v0.154.0`, wired trong `infra/docker/docker-compose.yml` (services `otel-agent`, `otel-gateway`, profile `observability`).
- Pipeline **logs** đầy đủ: `filelog` tail Docker json-file → operators parse zerolog JSON + `severity` + `trace_parser` → `transform/datastream` route data stream `logs-{service}.otel-default` (ADR-019) → `elasticsearch` exporter với `sending_queue` file-backed chống mất log khi ES chập chờn.
- Pipeline **traces** cơ bản: `otlp` (agent) → forward thô → gateway `tail_sampling` → `otlp/jaeger`. **Tail sampling với 3 policy errors/slow/probabilistic đã thực sự chạy** ở gateway (`gateway.yaml:37-49`).
- SDK phía service: `backend/internal/shared/tracing/tracing.go` — `AlwaysSample` + batch 5s/512, W3C propagator, OTLP gRPC sang agent. Endpoint rỗng → no-op (test/dev nhẹ chạy không cần collector).
- Jaeger v2 (`jaegertracing/jaeger:2.10.0`) nhận OTLP, UI `:16686`.

**PLANNED / đích (chỉ trong doc_v2, chưa có trong YAML repo):**
- **Policy `drop-health-checks`**: doc_v2/04 §2.2 mô tả nó (drop `/health`, `/ready`, `/metrics` qua `string_attribute` + `invert_match`), **nhưng `gateway.yaml` hiện tại chưa có policy này** — đây là gap cần đóng. *Lưu ý best-practice mới (§6): `invert_match` đã deprecated, nên hiện thực bằng `drop` policy.*
- **`spanmetrics` connector** (doc_v2/04 §2.3): sinh RED metrics + exemplars per-endpoint từ traces. `gateway.yaml:86-87` ghi comment "bổ sung ở bước correlation sau" — **chưa wired**. Đây là mảnh ghép cho correlation Metrics→Traces.
- **Jaeger storage = Elasticsearch** (index `jaeger-*`, ILM 7d, doc_v2/04 §2.4): compose hiện dùng **storage in-memory** mặc định của image cho dev; ES backend là production target.
- **Mode B — Kafka buffer** (doc_v2/03 §5, ADR-027): agent đổi exporter `otlp/gateway` → `kafka` (topic `otlp_logs`), gateway thêm `kafka` receiver. Kích hoạt khi >5-10K logs/s hoặc cần replay.
- **Multi-gateway + `loadbalancing` exporter** (§3.7): khi một gateway hết sức chứa.
- **`pipeline-health` dashboard** (doc_v2/04 §4): meta-monitoring collector qua `:8888/metrics` (throughput, queue size, DLQ rate). Cả hai collector đã expose `:8888` — dashboard là planned.

Nói cách khác: **xương sống logs + tail sampling traces đã sống**; phần *correlation đầy đủ* (spanmetrics/exemplars), *bền vững* (ES storage cho Jaeger), và *scale* (Kafka, multi-gateway) là lộ trình GĐ2→GĐ4.

## 5. Best practices (mỗi mục kèm 1 nguồn đã research)

1. **Đặt `decision_wait` đủ rộng nhưng không thừa.** 10s đủ cho web service sub-second (đúng như LogMon); chỉ tăng lên 30s nếu có span dài/async/batch job. ([oneuptime — tail sampling processor](https://oneuptime.com/blog/post/2026-02-06-tail-sampling-processor-opentelemetry-collector/view))
2. **Tính RAM trước, đừng đoán.** `RAM ≈ avg_trace_size × traces_per_sec × decision_wait`. `num_traces` phải tỷ lệ với traffic; theo dõi `..._trace_removal_age` để biết khi nào sắp drop oan. ([OTel contrib tailsampling README](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/tailsamplingprocessor/README.md))
3. **Error policy luôn đứng đầu, probabilistic luôn cuối.** Policy đánh giá theo OR + thứ tự; đặt sai thứ tự khiến trace lỗi bị catch-all "nuốt" sớm. ([oneuptime — implement tail-based sampling](https://oneuptime.com/blog/post/2026-01-25-tail-based-sampling-opentelemetry/view))
4. **Dùng agent→gateway, không nhồi tail sampling vào agent.** Việc nặng/stateful gom về tầng gateway scale được; agent giữ nhẹ trên mọi host. ([OTel agent-to-gateway pattern](https://opentelemetry.io/docs/collector/deploy/other/agent-to-gateway/))
5. **Khi scale gateway, bắt buộc `loadbalancing` exporter `routing_key: traceID`.** Không có nó, span một trace tản ra nhiều gateway → quyết định sampling sai. Ở K8s, deploy gateway dạng StatefulSet + headless service để DNS resolver ổn định. ([oneuptime — trace gateway load balancing](https://oneuptime.com/blog/post/2026-02-09-otel-collector-trace-gateway-load-balancing/view))
6. **Ưu tiên `drop` policy thay vì `invert_match`.** "Inverted decision" đã bị deprecated trong tail sampling processor; muốn loại health-check thì dùng policy `drop` tường minh. ([OTel contrib tailsampling README](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/tailsamplingprocessor/README.md))

## 6. Lỗi thường gặp & anti-patterns

- **Quên `memory_limiter`, hoặc đặt sai vị trí.** Phải là processor **đầu tiên**. Thiếu nó, một burst log/trace giết collector bằng OOM — kéo sập cả pipeline observability đúng lúc bạn cần nó nhất.
- **Đặt `probabilistic` lên trước `errors-always`.** Vì là first-match, trace lỗi có thể bị nhánh 10% "bốc thăm trượt" trước khi tới policy errors → **mất trace lỗi**. Đây là lỗi ngầm, không báo gì.
- **Bật tail sampling trên *agent* (per-host).** Mỗi agent chỉ thấy span của host nó → không bao giờ ráp đủ một trace cross-service → quyết định rác. Tail sampling **chỉ ở gateway**.
- **Scale gateway lên N replica mà không có `loadbalancing`.** Span một trace rải đều N gateway, mỗi gateway thấy mảnh vụn → sampling sai, trace gãy. Triệu chứng kinh điển khi chuyển dev→prod.
- **`num_traces` cố định khi traffic tăng 10×.** Trace bị evict trước `decision_wait` → drop oan cả trace lỗi. Phải scale `num_traces` theo tải (§3.6).
- **Dựa `invert_match` để drop noise.** Đã deprecated; dùng `drop` policy (§5.6). Áp dụng ngay khi đóng gap `drop-health-checks` của LogMon.
- **Tưởng head sampling ở SDK + tail sampling ở gateway là cộng dồn an toàn.** Nếu SDK head-sample 10% thì gateway *chỉ còn thấy 10%* — và 90% trace lỗi đã bị vứt từ SDK, tail sampling vô dụng. Vì vậy LogMon cố ý để SDK `AlwaysSample` (`tracing.go:100-102`).
- **Để Jaeger in-memory rồi tưởng đó là production.** Restart là mất sạch trace. Production phải chuyển sang ES backend (planned).

## 7. Lộ trình luyện tập (🥉→🥈→🥇)

Chủ đề phần lớn là planned/định hướng, nên task hướng tới **thiết kế/POC ngay trong repo LogMon**, có thể verify.

**🥉 Cơ bản — quan sát cái đang chạy**
1. `make up-full` rồi `make up-demo` + `make seed`. Sinh tải, mở Jaeger `:16686`, tìm một trace của `demo-order`.
2. Truy ngược: trace đó được giữ vì policy nào? Đọc `gateway.yaml:37-49`, đối chiếu trace có ERROR/`>1000ms` không, hay rơi vào nhánh 10%.
3. `curl localhost:8888/metrics` trên cả agent và gateway; tìm `otelcol_processor_tail_sampling_*` và `otelcol_exporter_sent_spans`. Ghi lại baseline.

**🥈 Trung cấp — sửa pipeline có kiểm chứng**
4. **Đóng gap `drop-health-checks`** (§4): thêm policy `type: drop` loại `/health`, `/ready`, `/metrics` vào `gateway.yaml` (KHÔNG dùng `invert_match` — §5.6). Restart gateway, xác nhận bằng `otelcol_validate`/log khởi động không lỗi, rồi sinh tải health-check và xác minh chúng không còn trong Jaeger.
5. Hạ `decision_wait` xuống `1s` *cố ý*, sinh tải có span chậm >1s, quan sát `_trace_removal_age` tăng và trace chậm bị drop oan. Khôi phục `10s`. Viết lại quan sát theo công thức RAM §3.6.

**🥇 Nâng cao — thiết kế cho scale (POC + ADR)**
6. **POC spanmetrics connector** (doc_v2/04 §2.3): thêm `connectors: spanmetrics` + pipeline `metrics/spans` vào `gateway.yaml`, export Prometheus. Xác minh sinh được `calls_total`/`duration` per `http.method`, kèm exemplar trỏ `trace_id`. Đây chính là mảnh correlation Metrics→Traces.
7. **Thiết kế tầng multi-gateway**: viết một mini-ADR (theo style doc_v2/13) cho `loadbalancing` exporter `routing_key: traceID`, DNS resolver, gateway StatefulSet — nêu rõ điều kiện kích hoạt (ngưỡng trace/s) và rủi ro nếu thiếu. Không cần triển khai thật, nhưng config YAML phải pass `otelcol validate`.
8. **Thiết kế Mode B**: phác config agent đổi sang `kafka` exporter (`otlp_logs`) + gateway `kafka` receiver, kèm tính toán partition/consumer group (doc_v2/03 §5).

## 8. Skill/agent ECC nên dùng

- **`ecc:architect`** — khi quyết định kiến trúc tầng collector: 1 vs N gateway, có nên kéo Kafka vào (Mode A→B), routing_key, StatefulSet vs Deployment. Đây là quyết định kiến trúc, đúng tầm agent này (CLAUDE.md → agents.md).
- **`ecc:performance-optimizer`** — khi cân chỉnh `num_traces`/`decision_wait`/`batch`/`memory_limiter` theo tải thật và RAM budget; phân tích `:8888/metrics` để tìm bottleneck queue/drop.
- **`ecc:latency-critical-systems`** — khi đảm bảo pipeline export **không bao giờ chặn request path** của service (batch non-blocking, drop-on-overload) và khi thiết kế ngưỡng `slow-requests` (latency policy) khớp với SLO.
- Phụ trợ: **`ecc:go-review`** cho `tracing.go` khi mở rộng instrumentation; **`/cso`** (security review) khi bật mTLS agent↔gateway cho multi-host (doc_v2/09 §6) — hiện đang `insecure: true` chỉ hợp lệ trong network single-host nội bộ.

## 9. Tài nguyên học thêm (link đã research)

- [Tail Sampling Processor — OTel Collector Contrib README](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/tailsamplingprocessor/README.md) — nguồn gốc về mọi policy type, công thức RAM, và ghi chú deprecate `invert_match`.
- [Agent-to-gateway deployment pattern — OpenTelemetry docs](https://opentelemetry.io/docs/collector/deploy/other/agent-to-gateway/) — kiến trúc LogMon đang theo.
- [Gateway deployment pattern — OpenTelemetry docs](https://opentelemetry.io/docs/collector/deploy/gateway/) — gateway, scale, loadbalancing.
- [Sampling — OpenTelemetry concepts](https://opentelemetry.io/docs/concepts/sampling/) — head vs tail, nền tảng.
- [Tail Sampling with OpenTelemetry — official blog](https://opentelemetry.io/blog/2022/tail-sampling/) — vì sao tail, đánh đổi.
- [How to Deploy the OTel Collector as a Trace Gateway (load balancing) — oneuptime](https://oneuptime.com/blog/post/2026-02-09-otel-collector-trace-gateway-load-balancing/view) — routing_key traceID, StatefulSet, DNS resolver.
- [Tail-Based Sampling: Sizing, Memory Crashes and Cost Model — Michal Drozd](https://www.michal-drozd.com/en/blog/otel-tail-sampling/) — đào sâu RAM/cost.
- Nội bộ: `doc_v2/03-logs-pipeline.md`, `doc_v2/04-metrics-tracing.md`, ADR-018/019/020/027 (`doc_v2/13-adr.md`); code `infra/otel/agent.yaml`, `infra/otel/gateway.yaml`, `backend/internal/shared/tracing/tracing.go`. Module trước: OBS-3 (`03-traces-opentelemetry-jaeger.md`).

## 10. Checklist "đã hiểu"

- [ ] Giải thích được pipeline receiver → processor → exporter, và vì sao **một** collector có nhiều pipeline (logs/traces/metrics) song song.
- [ ] Phân biệt rạch ròi head vs tail sampling, và nói được *vì sao LogMon để SDK `AlwaysSample`* (`tracing.go:100`).
- [ ] Đọc 3 policy trong `gateway.yaml` và giải thích quy tắc **OR + first-match** + vì sao thứ tự errors→slow→probabilistic là bắt buộc.
- [ ] Viết được công thức RAM của tail sampling và suy ra tải tối đa ngầm định của `num_traces: 50000 / decision_wait: 10s`.
- [ ] Phát biểu **ràng buộc vàng** "cả trace về một gateway" và biết khi nào cần `loadbalancing` exporter `routing_key: traceID`.
- [ ] Phân biệt được phần **đã implemented** (logs pipeline đầy đủ + tail sampling 3-policy + Jaeger v2) vs **planned** (spanmetrics, ES storage cho Jaeger, Mode B Kafka, multi-gateway, `drop-health-checks`).
- [ ] Biết vì sao `memory_limiter` phải là processor đầu tiên và `batch` ở gần cuối.
- [ ] Nêu được lý do nên dùng `drop` policy thay cho `invert_match` đã deprecated.
- [ ] Chỉ ra `tls: insecure: true` của agent↔gateway chỉ hợp lệ trong network single-host, multi-host phải bật mTLS (doc_v2/09 §6).
