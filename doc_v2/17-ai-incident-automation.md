# 17 · Tự động hoá xử lý sự cố bằng AI — AI-Assisted Incident Automation (GĐ5)

> **Trạng thái:** Design doc (source of truth) · **Cập nhật:** 2026-06-23
> **Giai đoạn:** GĐ5 (mới) — *sau* GĐ3 (incident + notification BC); xếp **cuối** roadmap, sau GĐ4 (Scale & Enterprise).
> **Phụ thuộc:** `internal/incident` (state machine, MTTA/MTTR), `internal/alerting`, `internal/slo` (error budget), `internal/logpipeline`, và toàn bộ pipeline observability (Prometheus/Thanos · zerolog→OTel→Elasticsearch · OTel→Jaeger v2 · Grafana 13.1).
> **Mục tiêu một dòng:** Dùng AI hỗ trợ **chẩn đoán sự cố (RCA)** + **RAG runbook/postmortem** ở mức **human-in-the-loop** để **giảm MTTR**, an toàn và đo lường được.

---

## 0. Vị trí trong roadmap & lý do đặt ở GĐ5

| | |
|---|---|
| Roadmap | GĐ1 nền tảng · GĐ2 alerting · **GĐ3 incident + notification** · GĐ4 Scale & Enterprise · **(GĐ5 — tài liệu này)** |
| Vì sao sau GĐ3 | AI xử lý sự cố cần **nền** đã có: incident aggregate (state machine SEV1–4, MTTA/MTTR, escalation, postmortem), notification hub đa kênh, alerting + error budget của SLO BC, và telemetry đầy đủ. Không có các BC này thì AI không có nơi ghi kết quả, không có runbook/postmortem để học, và không có "đồng hồ" để đo cải thiện. |
| Vì sao là GĐ5 (không phải GĐ4) | GĐ4 đã dành cho **Scale & Enterprise** trong `12-roadmap.md`. AI incident automation phụ thuộc GĐ3 và hưởng lợi từ độ chín vận hành của GĐ4 → đặt cuối roadmap, **không đánh số lại** các GĐ hiện có. |
| Đã đồng bộ | `12-roadmap.md` (thêm GĐ5) · `README.md` (index 00→17) · `13-adr.md` (ADR-031…035). |

**Nguyên tắc đặt phase (đã chốt với chủ dự án):** giai đoạn AI mới, đặt **cuối** roadmap (sau GĐ4 Scale & Enterprise) vì phụ thuộc incident BC (GĐ3); không tái cấu trúc/đánh số lại các GĐ hiện có.

---

## 1. Vấn đề & mục tiêu

### 1.1 MTTR — định nghĩa chính xác (tránh đo sai)

| Metric | Nghĩa | Đồng hồ |
|---|---|---|
| **MTTD** | Mean Time **to Detect** — từ lúc lỗi bắt đầu đến khi hệ thống *nhận biết* | phụ thuộc độ phủ monitoring, không phải tốc độ người |
| **MTTA** | Mean Time **to Acknowledge** — từ alert đến khi người bắt đầu xử lý | sức khoẻ on-call/paging |
| **MTTM** | Mean Time **to Mitigate** — đến khi *giảm/chặn* tác động (failover/rollback) | thường là cái người dùng quan tâm nhất |
| **MTTR** | Mean Time **to Recover/Repair/Resolve** — **MƠ HỒ, phải nói rõ chữ "R"** + mốc bắt đầu/kết thúc | metric bị lạm dụng nhất |
| **MTBF** | Mean Time **Between Failures** — độ tin cậy/tần suất | Availability ≈ MTBF / (MTBF + MTTR) |

> **Quy ước LogMon:** khi nói "MTTR" trong dự án, mặc định là **Mean Time to *Resolve*** (detect → service restored), tách riêng **MTTM** (mitigate). Mọi so sánh phải cùng định nghĩa "R" và cùng mốc.

### 1.2 Vòng đời sự cố — nơi thời gian dồn lại

```
lỗi bắt đầu ─► DETECT ─► ACKNOWLEDGE ─► TRIAGE ─► DIAGNOSE ─► MITIGATE ─► REPAIR/RESOLVE ─► postmortem
              └MTTD─┘   └───MTTA────┘   └────── (nằm trong MTTR) ──────┘   └── MTTM / MTTR ──┘
```

Bằng chứng nhất quán (2025–2026): **>50% thời gian sự cố nằm ở TRIAGE + DIAGNOSE** — và đó chính là nơi AI có đòn bẩy cao nhất. MTTD/MTTA khó cải thiện bằng AI (do độ phủ monitoring + cấu hình paging quyết định).

### 1.3 Mục tiêu GĐ5

- **Cắt thời gian triage + diagnosis** bằng AI: tự gom alert, kéo telemetry, dựng giả thuyết RCA có dẫn chứng, gợi ý runbook.
- **Mục tiêu thực tế: giảm ~25–40% MTTR** — *có điều kiện* về chất lượng telemetry và mức độ sự cố thiên về chẩn đoán. **KHÔNG hứa hẹn 50–80%** (xem §10.3 — đó là số marketing).
- Tự sinh nháp postmortem, giảm nhiễu alert.

### 1.4 Non-goals của GĐ5 (ranh giới rõ ràng)

- ❌ **KHÔNG** auto-remediation vòng kín (AI tự sửa production không người duyệt). Để giai đoạn sau, khi đã có track record.
- ❌ **KHÔNG** thay thế người on-call. AI thu hẹp không gian tìm kiếm + tự động việc tẻ nhạt; *con người vẫn ra quyết định*.
- ❌ **KHÔNG** để AI tự ý đổi cấu hình/schema/restart production.

---

## 2. Năm nguyên tắc nền (bám thực tế, chống ảo tưởng)

1. **"AI proposes, human approves, AI executes."** Đây là đồng thuận ngành 2025–2026. Tự chủ là thứ *phải kiếm được*, không cấp sẵn ngày một.
2. **Telemetry là input KHÔNG đáng tin.** Nghiên cứu *AIOpsDoom* (USENIX Security '26, arXiv 2508.06394): payload đối kháng nhúng trong logs/metrics/traces lái agent đi sửa sai với **tỉ lệ tấn công ~89%**, *né được* PromptShields/Prompt-Guard. → phải **sanitize telemetry** trước khi đưa vào LLM.
3. **RCA tự chủ vẫn còn yếu.** Benchmark thực: **OpenRCA** (ICLR 2025) model tốt nhất chỉ giải **11.34%**; **ITBench** (ICML 2025) agent giải **13.8%** kịch bản SRE. → AI là *trợ lý chẩn đoán*, không phải người quyết; luôn kèm **confidence + dẫn chứng**.
4. **Không telemetry → không lợi ích.** "AI không giảm MTTR chỉ vì biết tóm tắt sự cố." Điều kiện tiên quyết là **telemetry hợp nhất, chất lượng cao** (xem §7).
5. **Đo bằng "error budget saved", không chỉ MTTR thô.** Một SEV1 được mitigate nhanh đáng giá hơn nhiều một blip lưu lượng thấp — MTTR thô không nắm được điều đó.

---

## 3. Mức tự chủ (Autonomy Ladder) — GĐ5 trần ở **L2**

| Rung | Tên | Mô tả | Phạm vi GĐ5 |
|---|---|---|---|
| **L1** | Assistive (CRAWL) | AI quan sát, tương quan, tóm tắt, dựng **giả thuyết RCA** — **không hành động** | ✅ Khởi đầu (read-only ≥4 tuần) |
| **L2** | Human-in-the-loop suggestions (WALK) | AI điều tra đa bước + **gợi ý remediation cụ thể**; con người quyết | ✅ Trần của GĐ5 |
| **L3** | Approval-gated remediation | AI **thực thi** fix *với phê duyệt từng hành động* (dry-run, rollback, RBAC, audit) | ⏭️ Ngoài GĐ5 — giai đoạn sau |
| **L4** | Closed-loop auto-remediation | AI tự xử lý cho kịch bản hẹp đã chứng minh | ⏭️ Ngoài GĐ5 |

> **"Soạn remediation có cổng" (đã chọn) = L2:** AI *chuẩn bị sẵn* các bước khắc phục (lệnh, runbook, PR nháp) để **người duyệt rồi thực thi** — *chưa* tự chạy. Promotion lên L3 chỉ sau khi có bằng chứng clean-window (xem §9).

---

## 4. Kiến trúc tổng thể

### 4.1 Quyết định build-vs-buy (đã chốt: **dựa trên framework/nền tảng có sẵn**)

Lõi agentic **không tự cuốn từ đầu trong Go**. Thay vào đó ghép các nền tảng đã chứng minh, nối qua **MCP**:

| Vai trò | Lựa chọn lõi | Lý do |
|---|---|---|
| **Engine RCA agentic** | **HolmesGPT** (robusta-dev, Apache-2.0, CNCF Sandbox 10/2025) | Cùng nguồn dữ liệu LogMon (Prometheus, Grafana, ES, Tempo/Jaeger, K8s); vòng ReAct alert→điều tra→ghi-kết-quả; *output budgeting* chống OOM; runbook-driven. Có thể **deploy thẳng làm baseline** trước khi tuỳ biến. |
| **RAG runbook/postmortem** | **WeKnora** (Tencent, MIT) hoặc tự cuốn trên **pgvector/ES** | Hybrid BM25+dense+**GraphRAG** + citations; Wiki Mode tự bảo trì; chạy trên Postgres/ES bạn đã có. Xem [16-iac-runbooks](16-iac-runbooks.md) làm nguồn runbook. |
| **Correlation/dedup alert** | **Keep**-style (hoặc logic trong `internal/incident`) | Gom N alert → 1 incident trước khi agent điều tra (giảm nhiễu, giảm MTTx). |
| **Orchestration loop** | **Claude Agent SDK** *hoặc* LangGraph (nếu cần state machine điều tra) | Vòng tool-calling + subagent + nén context + hooks guardrail. |

> **Ranh giới kiến trúc trung thực:** lớp AI là **service Python độc lập** (HolmesGPT/framework), **tách khỏi Go core** của LogMon. Nó tích hợp với LogMon **qua MCP (đọc telemetry) + webhook (Alertmanager) + API các BC** — **KHÔNG** vi phạm layer direction, **KHÔNG** cross-BC import. Đây là một *bounded service* mới, giao tiếp qua sự kiện/HTTP như mọi BC khác.

### 4.2 Pipeline agentic SRE (9 bước, chuẩn ngành)

```
[1] INGESTION   alert (Alertmanager) · metrics · logs · traces · deploy/CI events
       │
[2] CORRELATION/DEDUP   gom alert liên quan → 1 incident, enrich context
       │
[3] TRIAGE      severity · blast radius · risk · routing (chỉ kích hoạt khi burn-rate đe doạ budget)
       │
[4] DIAGNOSIS/RCA ◄──── VÒNG TOOL TRỰC TIẾP (MCP) ────┐  query→observe→refine→re-query
       │   tương quan metrics+logs+traces+topology+deploy │
[5] HYPOTHESIS RANKING   giả thuyết có citation + confidence
       │   (confidence thấp?) ─────────────────────────►─┘
[6] RUNBOOK RETRIEVAL    RAG trên runbook + sự cố quá khứ (citations)
       │
[7] REMEDIATION          ĐỀ XUẤT (GĐ5 dừng ở đây — người duyệt & thực thi)
       │
[8] VERIFICATION         re-query telemetry: error rate/latency đã hồi phục? (nếu chưa → [4])
       │
[9] POSTMORTEM           tự sinh nháp timeline/RCA/comms → người review → feed lại KB
```

### 4.3 Vị trí của agent trong luồng (Alertmanager receiver song song)

```
Prometheus/Thanos ──fire──► Alertmanager ──route──┬─► [receiver: PagerDuty] ──► người on-call
  (rules + recording rule anomaly                  │
   Prophet/z-score; multi-window burn-rate)        └─► [receiver: AI Incident Agent webhook]  (song song, KHÔNG thay paging)
Netdata ML anomalies ───────────────────────────────────────┘
                                                             │
                          Agent (vòng điều khiển + subagent: metrics / logs / traces)
                          tools (MCP): Grafana(Prom/Loki/Sift/OnCall) · Prometheus/Thanos ·
                                       Elasticsearch(logs+Jaeger) · custom Jaeger/TraceQL ·
                                       custom "đọc API các BC LogMon"
                                                             │
                       thu thập dẫn chứng → RCA → giả thuyết xếp hạng + gợi ý fix
        ┌────────────────────────────────────┼────────────────────────────────────┐
        ▼                                     ▼                                     ▼
  Slack (ChatOps):                    notification BC:                      incident BC:
  RCA + Grafana deeplink/PNG,         đẩy đa kênh, @mention on-call         tạo/annotate incident aggregate,
  hỏi-đáp tiếp trong thread                                                 MTTA/MTTR, stage postmortem
```

---

## 5. Năng lực GĐ5 (5 mục đã chọn)

### 5.1 Chẩn đoán / RCA
- Vòng tool ReAct qua MCP: **query→observe→refine**. Subagent tách theo modality để cô lập context lớn:
  - `metrics-investigator` (Prometheus/Thanos), `logs-investigator` (ES), `traces-investigator` (Jaeger/ES) → `orchestrator` tổng hợp.
- Tín hiệu RCA, theo thứ tự độ tin cậy: **tương quan deploy↔lỗi** (cao nhất, mở đường rollback) → anomaly đã chấm điểm (Netdata 18-model / Prophet) → topology/dependency → log pattern clustering → **retrieval sự cố quá khứ (RAG)**.
- Đầu ra: **giả thuyết xếp hạng + citation cụ thể + confidence + chain-of-thought** (bắt buộc log lại).

### 5.2 RAG runbook / postmortem
- **Docs-as-code**: runbook/postmortem là Markdown cạnh code, cập nhật cùng PR đổi hành vi (nguồn: [16-iac-runbooks](16-iac-runbooks.md)). *Freshness là yếu tố số 1 quyết định chất lượng RAG.*
- Retrieval **hybrid (BM25 + dense + GraphRAG)** + **citations**; grounding nghiêm: "chỉ trả lời từ ngữ cảnh được cấp, không có thì nói không biết".
- **GraphRAG** quan trọng cho sự cố: liên kết thực thể (service, error code, owner, deploy) để truy "sự cố tương tự đã từng xảy ra".

### 5.3 Soạn remediation có cổng (L2)
- AI đề xuất **các bước khắc phục cụ thể** (lệnh, runbook đã có, PR nháp) — ưu tiên **idempotent / rollback-style**, kèm **dry-run**.
- **Người duyệt rồi thực thi.** Không tự chạy. (Cơ chế thực thi tự động = L3, giai đoạn sau.)

### 5.4 Tự sinh postmortem
- Sinh nháp từ **artifact đã ghi**: timeline incident BC, hội thoại Slack, dữ liệu điều tra. **Bắt buộc human-review** (workflow In Progress → In Review → Completed).
- *Không bao giờ* để model bịa mục timeline/nguyên nhân. Postmortem hoàn tất **feed lại KB** → cải thiện MTTD/triage lần sau (vòng học khép kín).

### 5.5 Giảm nhiễu alert
- Dedup/group/correlate (giảm noise >90% theo báo cáo ngành) — nhưng cẩn trọng **false-suppression** (bỏ sót).
- **Chỉ kích hoạt agent cho alert đe doạ error budget**: dùng **multi-window, multi-burn-rate** (SRE Workbook) — ví dụ fast-burn 1h ≥ 14.4× & 6h elevated → page + trigger agent. Tránh đốt token vào alert nhiễu.

---

## 6. Tích hợp với các BC hiện có (qua sự kiện / API, không cross-import)

| BC | Vai trò trong GĐ5 |
|---|---|
| `alerting` | `AlertFired` (burn-rate breach) → webhook kích hoạt agent. |
| `slo` | Error budget **gate** mức độ tự chủ & là thước đo "budget saved"; cung cấp burn-rate. |
| `incident` | Agent tạo/annotate **incident aggregate** (state machine, severity, MTTA/MTTR), stage postmortem. Nguồn artifact để sinh postmortem. |
| `notification` | Đẩy RCA/đề xuất ra Slack/PagerDuty/email; `get_current_oncall` để @mention đúng người/ca trực. |
| `logpipeline` | Nguồn logs (ES data streams) cho `logs-investigator`. |

Mỗi tích hợp hiện thực dưới dạng **tool MCP / read-API**; tuân thủ "BCs giao tiếp qua domain events hoặc shared kernel".

---

## 7. Telemetry sẵn sàng cho agent (điều kiện tiên quyết)

OTel là **chất nền tương quan** giúp RCA đa tín hiệu khả thi cho agent:
- **Semantic conventions chặt** → telemetry portable, so sánh & tương quan được; thiếu nó RCA "thoái hoá thành parse tuỳ biến".
- **trace_id vào log records** (W3C Trace Context) → agent nhảy 1-hop từ error log sang trace/span.
- **Exemplars** nối metric → trace (anomaly trên histogram latency → trace chậm đại diện).
- **service.name / topology attributes** → nuôi reasoning đồ thị phụ thuộc.

> **Hệ quả:** phải hoàn tất chuẩn-hoá OTel semconv + trace-log correlation **trước** khi bật agent (mốc 5.0 trong rollout §11). *Không telemetry chất lượng → AI chỉ tóm tắt, không chẩn đoán được.*

---

## 8. Guardrails & bảo mật (BẮT BUỘC — map [09-security](09-security.md))

| Guardrail | Hiện thực trong LogMon |
|---|---|
| **Read-only khi điều tra** | Hook `PreToolUse` chặn mọi tool gây thay đổi trong vòng auto-investigation. |
| **Audit mọi truy vấn** | Hook `PostToolUse` ghi (ai/cái gì/khi nào) qua **zerolog**; log **immutable, tamper-evident**. |
| **Sanitize telemetry** | Redact/anonymize trường log (pattern `--anonymize` kiểu k8sgpt) **trước** khi vào LLM — chống prompt-injection (AIOpsDoom) + rò rỉ secret/PII. |
| **Identity least-privilege** | Agent có **identity riêng**, không "ambient access"; mỗi tool bọc capability contract (input/output/scope). |
| **Dry-run mặc định** | Mọi API hướng-agent hỗ trợ `dry_run=true` (Google bắt buộc). |
| **Approval gate** | Mọi hành động (silence alert, restart…) sau **phê duyệt người** — không tự chạy trong vòng điều tra. |
| **Circuit breaker / rate limit** | Giới hạn riêng cho agent; dừng remediation chạy loạn. |
| **Change-freeze awareness** | Biết maintenance window / code freeze (bài học Replit 7/2025: agent xoá prod DB *trong code freeze*). |

---

## 9. Đánh giá (Evaluation) — không có eval thì không thăng rung

- **Golden replay suite** trên sự cố quá khứ: ~30 case, chạy <5 phút, **block merge khi hồi quy**; bộ mở rộng ~200 case khi đổi phiên bản LLM.
- **Observability OF the agent**: trace toàn bộ reasoning + tool-call (OTel spans), review tool-trace hàng tuần tìm misuse/hallucination.
- **Metrics chất lượng**: agreement-rate với RCA của người · time-to-RCA · **false-RCA rate** · missed findings · % gợi ý được chấp nhận · rollback/regression rate.
- **Promotion gate giữa các rung**: ví dụ "2 tuần sạch" trước khi cho agent tự kích hoạt theo alert; chỉ lên L3 (ngoài GĐ5) sau "3 tháng sạch ở L2".
- Tham chiếu benchmark công khai để hiệu chỉnh kỳ vọng: **OpenRCA, RCAEval, ITBench**.

---

## 10. KPI & cách đo (để số liệu có ý nghĩa)

1. **Cố định định nghĩa trước** (chữ "R", mốc start/stop) — nếu không, "cải thiện" chỉ là đổi cách bấm giờ.
2. **Phân rã MTTR theo phase** (detect→ack→triage→diagnose→mitigate→repair). AI phải làm giảm *triage + diagnose*; nếu tổng MTTR giảm mà 2 phase này không → nguyên nhân khác.
3. **So sánh có kiểm soát** (A/B hoặc shadow-mode), **phân khúc theo severity** (đừng trộn SEV1 với SEV4).
4. **Dùng phân phối** p50/p90, không chỉ mean (MTTR lệch phải).
5. **Đo chất lượng + an toàn cùng tốc độ**: nhanh-mà-sai là giá trị âm.
6. **Biểu đạt tác động cuối cùng = error budget saved.**

> ### 10.3 Thực tế hoá kỳ vọng (chống marketing)
> - **Số đáng tin để lập kế hoạch: ~25–40% MTTR**, *tuỳ chất lượng telemetry*.
> - Các con số **50–80% là vendor/marketing hoặc kịch bản dựng** (incident.io "80%", "90→15 phút"; Meta "~50%" nội bộ; PagerDuty "up to 50%"). **Không** dùng làm cam kết KPI.
> - Traversal–AmEx (82% RCA accuracy, 32% MTTR) là khách hàng Fortune-100 đặt tên (nặng ký hơn) nhưng vẫn **tự báo cáo, chưa kiểm chứng độc lập**.

---

## 11. Lộ trình triển khai (crawl → walk)

| Mốc | Nội dung | Rung |
|---|---|---|
| **5.0 Telemetry-readiness** | Hoàn tất OTel semconv chặt, trace_id→log, exemplars, topology attrs. *Tiền đề bắt buộc.* | — |
| **5.1 Read-only RCA assistant** | Agent điều tra → post RCA + dẫn chứng vào Slack/incident BC. **Không hành động.** ≥4 tuần. | L1 |
| **5.2 RAG runbook/postmortem** | KB docs-as-code + hybrid/GraphRAG + citations; auto-draft postmortem (human-review). | L1 |
| **5.3 Correlation + burn-rate gating** | Dedup/group alert; chỉ trigger agent khi burn-rate đe doạ budget. | L1 |
| **5.4 Gợi ý remediation có cổng** | AI đề xuất bước khắc phục (dry-run, idempotent); người duyệt & thực thi. | **L2 (trần GĐ5)** |
| *(sau GĐ5)* | L3 gated remediation cho sự cố low-risk, reversible. | L3 |

---

## 12. Rủi ro & giảm thiểu

| Rủi ro | Giảm thiểu |
|---|---|
| Hallucination RCA (benchmark ~11–14%) | Confidence + citation bắt buộc; human-in-loop; eval false-RCA. |
| Over-trust / complacency · deskilling junior | Giữ người ở quyết định; thiết kế lộ trình phát triển junior; review tool-trace. |
| Prompt-injection qua telemetry (AIOpsDoom ~89%) | Sanitize/redact telemetry trước LLM; agent read-only; circuit breaker. |
| False-suppression alert | Giám sát recall của dedup; không suppress mù. |
| Chi phí token (payload telemetry lớn) | Context compaction + **output budgeting per-tool** + subagent isolation + tách discovery/query (list trước, query sau). |
| Lệch stack Python↔Go | AI là bounded service riêng, giao tiếp qua MCP/HTTP/event; không nhúng vào Go core. |

---

## 13. ADR liên quan (đã ghi trong `13-adr.md`)

1. **ADR-031** — Mức tự chủ AI = human-in-the-loop, **trần L2** cho GĐ5 ("AI proposes, human approves, AI executes"); cấm closed-loop trong GĐ5.
2. **ADR-032** — Lõi AI = **framework có sẵn** (HolmesGPT engine RCA + WeKnora/pgvector RAG) nối qua **MCP**; lớp AI là **service Python tách khỏi Go core**.
3. **ADR-033** — **Telemetry là untrusted input** → sanitize/redact bắt buộc trước LLM.
4. **ADR-034** — **Chỉ trigger AI trên burn-rate breach** (multi-window multi-burn-rate, xem ADR-025), không trên mọi alert.
5. **ADR-035** — **Tiền đề OTel semconv + trace-log correlation** là điều kiện vào GĐ5.

---

## 14. Nguồn tham khảo (chắt lọc, đã fact-check 2026-06)

- **Kiến trúc agentic SRE:** [Google SRE — agentic AI for operations](https://cloud.google.com/blog/products/devops-sre/how-google-sre-is-using-agentic-ai-to-improve-operations) · [AWS — multi-agent SRE với Bedrock AgentCore](https://aws.amazon.com/blogs/machine-learning/build-multi-agent-site-reliability-engineering-assistants-with-amazon-bedrock-agentcore/) · [Anthropic — multi-agent research system](https://www.anthropic.com/engineering/multi-agent-research-system) · [incident.io — What is AI SRE](https://incident.io/blog/what-is-ai-sre-complete-guide-2026)
- **Build trên stack:** [HolmesGPT](https://github.com/robusta-dev/holmesgpt) · [k8sgpt](https://github.com/k8sgpt-ai/k8sgpt) · [Keep](https://github.com/keephq/keep) · [Grafana MCP](https://github.com/grafana/mcp-grafana) · [Netdata MCP](https://learn.netdata.cloud/docs/netdata-ai/mcp) + [netdata/skills](https://github.com/netdata/skills) · [Elasticsearch MCP](https://github.com/elastic/mcp-server-elasticsearch) · [Claude Agent SDK](https://code.claude.com/docs/en/agent-sdk/overview)
- **RAG/runbook/postmortem:** [Backstage TechDocs](https://backstage.io/docs/features/techdocs/) · [WeKnora](https://github.com/Tencent/WeKnora) · [PagerDuty Runbook Automation (Rundeck)](https://www.pagerduty.com/platform/automation/runbook/) · [incident.io postmortems](https://docs.incident.io/post-incident/postmortems-overview)
- **MTTR/SLO:** [Atlassian — incident metrics](https://www.atlassian.com/incident-management/kpis/common-metrics) · [Google SRE Workbook — alerting on SLOs](https://sre.google/workbook/alerting-on-slos/)
- **An toàn/benchmark (đối kháng):** [OpenRCA (ICLR 2025)](https://github.com/microsoft/OpenRCA) · [ITBench — arXiv 2502.05352](https://arxiv.org/abs/2502.05352) · [AIOpsDoom — arXiv 2508.06394](https://arxiv.org/html/2508.06394v1) · [Google SRE — reliable operations (autonomy ladder L0–L4)](https://sre.google/resources/practices-and-processes/ai-engineering-reliable-operations/) · [Augment — AI SRE incident management](https://www.augmentcode.com/guides/ai-sre-incident-management)
- **Bối cảnh thị trường:** Resolve AI, Traversal, Cleric, Parity (AI-SRE startups) · PagerDuty SRE Agent · Datadog Bits AI SRE.

---

> **Liên quan:** [05-alerting-slo](05-alerting-slo.md) · [06-incident-notification](06-incident-notification.md) · [10-deployment-operations](10-deployment-operations.md) · [11-coding-testing-standards](11-coding-testing-standards.md) · [16-iac-runbooks](16-iac-runbooks.md) · [13-adr](13-adr.md)
