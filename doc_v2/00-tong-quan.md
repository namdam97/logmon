# 00 — Tổng Quan LogMon

> **LogMon** là nền tảng observability self-hosted cho Go microservices: thu thập **metrics + logs + traces**, cảnh báo thông minh, quản lý SLO và incident — vận hành được bởi team nhỏ trên hạ tầng tự quản.

---

## 1. Bài Toán

Khi hệ thống có nhiều microservices chạy production, không có observability tập trung nghĩa là:

- Sự cố được phát hiện bởi **khách hàng**, không phải hệ thống.
- Debug bằng SSH + grep log thủ công trên từng server.
- Không trả lời được: lỗi bắt đầu từ lúc nào? service nào là gốc? ảnh hưởng bao nhiêu user?
- Không đo được MTTR, không có postmortem, lặp lại cùng sự cố.

LogMon giải quyết bằng vòng khép kín: **Thu thập → Lưu trữ → Phân tích → Cảnh báo → Xử lý sự cố → Học hỏi (postmortem) → Cam kết chất lượng (SLO)**.

### Một câu tóm tắt

> LogMon là "hệ thần kinh" của microservices — không xử lý business logic, mà **quan sát, cảnh báo và hỗ trợ phản ứng** khi hệ thống có vấn đề. LogMon down → business vẫn chạy (chỉ mất khả năng quan sát).

---

## 2. Ba Trụ Cột Observability + Vòng Vận Hành

| Trụ cột | Trả lời câu hỏi | Công nghệ | Cách thu thập |
|---------|-----------------|-----------|---------------|
| **Metrics** | "CÓ vấn đề không?" (số liệu, xu hướng) | Prometheus 3.x + Thanos | PULL — scrape `/metrics` |
| **Logs** | "Vấn đề LÀ GÌ?" (chi tiết sự kiện) | OTel Collector → Elasticsearch 9.x | PUSH — stdout JSON → collector |
| **Traces** | "Vấn đề Ở ĐÂU?" (đường đi request) | OTel SDK → Jaeger v2 | PUSH — OTLP gRPC |

**Correlation** là giá trị cốt lõi: `trace_id` xuyên suốt cả 3 trụ cột → từ metric spike click sang logs, từ log click sang trace waterfall.

Trên 3 trụ cột là tầng vận hành (LogMon backend tự xây):

```
Alerting (rules, silence, inhibition)
   → SLO (error budget, burn rate)
   → Incident (lifecycle, on-call, MTTR, postmortem)
   → Notification (Slack, Email, PagerDuty, Teams, webhook)
```

---

## 3. Personas

| Persona | Nhu cầu chính | Bề mặt sử dụng |
|---------|---------------|----------------|
| **Developer** | Debug lỗi, trace request, search logs | Grafana (service overview, logs, traces) |
| **DevOps** | Sức khỏe hạ tầng, quản lý pipeline | Grafana (infrastructure) + LogMon UI (pipeline status, DLQ) |
| **SRE** | SLO/error budget, alert rules, incident, on-call | LogMon UI (alerts, SLO, incidents) + Grafana |
| **Engineering Manager** | Báo cáo tuần, MTTR trends, chi phí | Email reports + cost dashboard |

---

## 4. Phạm Vi (Scope)

### 4.1 Platform Core (sản phẩm chính)

| Năng lực | Giai đoạn |
|----------|-----------|
| Metrics collection + dashboards (Prometheus, Grafana, exporters) | GĐ 1 |
| Log aggregation + ILM (OTel Collector → ES data streams) | GĐ 1 |
| Alert rules cơ bản + Alertmanager routing | GĐ 1 |
| Alerting BC (rule CRUD, sync, ack/silence, webhook receiver) | GĐ 2 |
| Log Search API (search, tail SSE, by trace_id) | GĐ 2 |
| Distributed tracing + correlation (OTel SDK, Jaeger v2) | GĐ 2 |
| SLO BC (error budget, multiwindow burn-rate alerts) | GĐ 3 |
| Incident BC (lifecycle, on-call, postmortem, MTTR) | GĐ 3 |
| Notification Hub (đa kênh, templates, retry queue) | GĐ 3 |
| Multi-tenancy (workspace, RBAC) | GĐ 3 |
| Mode B scale (Kafka buffer, Thanos long-term, ES cluster) | GĐ 4 |
| Reports/export, service topology, cost dashboard | GĐ 4 |

Chi tiết và Definition of Done từng giai đoạn: [12-roadmap.md](12-roadmap.md).

### 4.2 Demo Workload (KHÔNG phải platform)

`examples/demo-order/` — service mẫu được instrument đầy đủ (metrics + logs + traces) để: (1) sinh telemetry thật cho dev/test platform, (2) làm tài liệu sống "cách tích hợp một service vào LogMon" (~30 dòng code). Demo workload **không** nằm trong `internal/` của platform — xem ADR-029.

### 4.3 Non-Goals (chủ động KHÔNG làm)

- **Không** là APM thương mại đầy đủ (profiling, RUM, synthetics).
- **Không** tự build engine đánh giá alert rule — Prometheus đánh giá, LogMon quản lý và đồng bộ (ADR-024).
- **Không** SQL-based log query (ClickHouse) và AI/ML anomaly detection — để ngỏ tương lai.
- **Không** hỗ trợ non-containerized workloads ở GĐ 1-3 (chỉ Docker/K8s stdout logs).
- **Không** billing/payment thật — "cost dashboard" chỉ là ước tính usage.

---

## 5. Vị Thế So Với Thị Trường

| Tiêu chí | LogMon | Datadog | Grafana Cloud | ELK tự host |
|----------|--------|---------|---------------|-------------|
| 3 trụ cột + correlation | ✅ | ✅ | ✅ | Một phần |
| SLO engine (burn rate chuẩn Google) | ✅ built-in | ✅ | ✅ | ❌ |
| Incident lifecycle + postmortem | ✅ built-in | ✅ | OnCall OSS đã archived (03/2026) | ❌ |
| Self-hosted, không vendor lock-in | ✅ | ❌ | ❌ | ✅ |
| Chi phí Medium scale (~20 services) | ~$150-250/tháng VPS | ~$300+/tháng | ~$28+/tháng (ít control) | Tương đương LogMon |
| Ops effort | Cao (tự vận hành) | Không | Thấp | Cao |

**Khác biệt của LogMon:** (1) SRE workflow đầy đủ alert → incident → postmortem → SLO trong một platform tự host; (2) Log pipeline quản lý qua UI/API (mode switch, DLQ retry, ILM) không cần SSH; (3) kiến trúc DDD rõ ràng — đồng thời là dự án học tập system design ở mức production.

**Khuyến nghị trung thực:** dưới 5 services và chưa cần data ownership → dùng free tier SaaS (Grafana Cloud 50GB, New Relic 100GB) rẻ hơn tự vận hành. LogMon có giá trị từ Medium scale hoặc khi cần self-hosted.

---

## 6. Nguyên Tắc Thiết Kế

1. **Progressive architecture** — Mode A trước Mode B; outbox in-process trước Kafka; Compose trước K8s. Chỉ thêm complexity khi nhu cầu thực sự phát sinh.
2. **Đứng trên vai người khổng lồ** — Prometheus đánh giá rules, Alertmanager routing, ES index, Grafana visualize. LogMon chỉ xây phần các tool trên không có: quản lý vòng đời (rules, SLO, incidents) + trải nghiệm thống nhất.
3. **Mọi quyết định có ADR** — context, lựa chọn, hệ quả, nguồn kiểm chứng ([13-adr.md](13-adr.md)).
4. **Dogfooding** — LogMon tự monitor chính nó (meta-monitoring, ADR-026), nhưng deadman switch phải nằm NGOÀI stack.
5. **Mọi alert phải actionable** — symptom-based, có runbook_url, severity tương ứng hành động (page vs ticket).
