# LogMon — Hiểu Từ Gốc

> File này trả lời: LogMon giải quyết gì? Input/Output là gì? Chạy độc lập hay tích hợp? Vận hành thế nào?

---

## 1. LogMon Giải Quyết Vấn Đề Gì?

### Vấn đề thực tế

Bạn có 10 microservices chạy trên production. Một ngày đẹp trời:

```
14:30  Khách hàng báo "đặt hàng lỗi"
14:31  Bạn SSH vào server, đọc log → không thấy gì (log quá nhiều, cuốn mất)
14:35  Bạn check CPU → bình thường
14:40  Bạn restart service → vẫn lỗi
14:50  Cuối cùng tìm ra: user-service trả 503 → order-service retry 100 lần → DB connection pool exhausted
15:10  Fix xong. 40 phút downtime. Không ai biết lúc nào bắt đầu lỗi.
```

**Không có LogMon:** Bạn đoán, SSH, grep log thủ công, không biết root cause, không đo được impact.

**Có LogMon:**

```
14:30  Prometheus alert: "user-service error rate > 5%" → Slack notification tự động
14:30  Incident auto-created (SEV1) → PagerDuty page on-call engineer
14:31  Engineer mở Grafana → thấy user-service latency spike từ 14:28
14:32  Click trace_id trong log → thấy order-service → user-service → timeout chain
14:33  Root cause: user-service DB connection pool full
14:35  Fix deployed. 5 phút MTTR.
14:36  SLO dashboard: error budget consumed 2.1%, còn 97.9%
Sau đó: Postmortem → action items → thêm circuit breaker + connection pool alert
```

### Một câu tóm tắt

> **LogMon là "hệ thần kinh" của microservices** — nó không xử lý business logic (đặt hàng, thanh toán), mà nó **quan sát, cảnh báo, và giúp bạn phản ứng** khi hệ thống có vấn đề.

---

## 2. Input / Output — LogMon Nhận Gì, Trả Gì?

```
╔══════════════════════════════════════════════════════════════════════╗
║                        INPUT (Thu thập)                             ║
║                                                                      ║
║  Microservices của bạn tự động emit 3 loại dữ liệu:                ║
║                                                                      ║
║  ┌─────────┐    ┌─────────┐    ┌─────────┐                         ║
║  │ METRICS │    │  LOGS   │    │ TRACES  │                         ║
║  │ (số)    │    │ (text)  │    │ (hành   │                         ║
║  │         │    │         │    │  trình) │                         ║
║  └────┬────┘    └────┬────┘    └────┬────┘                         ║
║       │              │              │                               ║
╠═══════╪══════════════╪══════════════╪═══════════════════════════════╣
║       │         LogMon Platform     │                               ║
║       │     (thu thập, lưu trữ,    │                               ║
║       │      phân tích, cảnh báo)   │                               ║
║       │              │              │                               ║
╠═══════╪══════════════╪══════════════╪═══════════════════════════════╣
║       ▼              ▼              ▼                               ║
║                    OUTPUT (Giá trị)                                  ║
║                                                                      ║
║  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ║
║  │Dashboards│ │  Alerts  │ │Incidents │ │ Reports  │ │   SLO    │ ║
║  │(nhìn)    │ │(biết)    │ │(xử lý)  │ │(báo cáo) │ │(cam kết) │ ║
║  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘ ║
╚══════════════════════════════════════════════════════════════════════╝
```

### INPUT chi tiết — 3 loại dữ liệu

| Input | Ví dụ cụ thể | Ai emit? | LogMon thu thập bằng gì? |
|-------|-------------|----------|--------------------------|
| **Metrics** (số, time-series) | `http_requests_total = 15000`, `cpu_usage = 72%`, `p99_latency = 120ms` | Go service expose `/metrics` endpoint | Prometheus **kéo** (PULL) mỗi 15 giây |
| **Logs** (text, sự kiện) | `{"level":"error","message":"connection refused","trace_id":"abc-123"}` | Go service ghi ra stdout (JSON) | Filebeat **đẩy** (PUSH) → Kafka → Logstash → Elasticsearch |
| **Traces** (hành trình request) | `order-service → user-service → PostgreSQL` (request mất 120ms, user-service chiếm 95ms) | OTel SDK trong Go service tự instrument | OTel Collector **nhận** (PUSH) → Jaeger |

### OUTPUT chi tiết — 5 giá trị

| Output | Mô tả | Ai dùng? |
|--------|-------|----------|
| **Dashboards** | Biểu đồ real-time: request rate, error rate, latency, CPU, RAM, disk | Developer, DevOps, SRE |
| **Alerts** | Cảnh báo tự động khi threshold bị phá: "error rate > 5%" → Slack/PagerDuty | On-call engineer |
| **Incidents** | Workflow xử lý sự cố: create → assign → fix → resolve → postmortem | SRE team |
| **Reports** | Báo cáo hàng tuần: SLO compliance, MTTR trends, incident summary | Engineering Manager |
| **SLO Tracking** | "Tháng này availability = 99.95%, error budget còn 67%" | SRE, Leadership |

---

## 3. Ba Trụ Cột Observability

> Đây là framework chuẩn ngành (Google, Netflix, Uber đều dùng). LogMon implement cả 3.

```
                    ┌─────────────────────────────────────────┐
                    │         OBSERVABILITY                    │
                    │    "Hiểu hệ thống đang làm gì          │
                    │     mà không cần thay đổi code"         │
                    └──────────────┬──────────────────────────┘
                                   │
                 ┌─────────────────┼─────────────────┐
                 │                 │                 │
           ┌─────▼─────┐    ┌─────▼─────┐    ┌─────▼─────┐
           │  METRICS   │    │   LOGS    │    │  TRACES   │
           │            │    │           │    │           │
           │ "Bao nhiêu"│    │ "Cái gì  │    │ "Đường đi │
           │ "Mấy %"   │    │  xảy ra" │    │  của 1    │
           │ "Xu hướng" │    │ "Chi tiết"│    │  request" │
           │            │    │           │    │           │
           │ Ví dụ:     │    │ Ví dụ:    │    │ Ví dụ:    │
           │ - CPU 72%  │    │ - "conn   │    │ - order → │
           │ - 150 req/s│    │   refused"│    │   user →  │
           │ - p99=120ms│    │ - stack   │    │   DB      │
           │ - error 2% │    │   trace   │    │ - 120ms   │
           │            │    │           │    │   total   │
           │ Trả lời:   │    │ Trả lời:  │    │ Trả lời: │
           │ "CÓ vấn đề │    │ "VẤN ĐỀ   │    │ "VẤN ĐỀ  │
           │  không?"   │    │  LÀ GÌ?"  │    │  Ở ĐÂU?" │
           └────────────┘    └───────────┘    └───────────┘
```

**Correlation — sức mạnh khi có cả 3:**

```
Bước 1: Metrics báo "error rate tăng" (CÓ vấn đề)
         ↓
Bước 2: Logs cho thấy "connection refused to user-service:5432" (vấn đề LÀ GÌ)
         ↓
Bước 3: Traces cho thấy "order-service → user-service (timeout 5s)" (vấn đề Ở ĐÂU)
         ↓
Kết luận: user-service DB connection pool full → order-service cascade failure

→ Liên kết bằng trace_id: 1 ID xuyên suốt metrics, logs, traces
```

---

## 4. Vòng Đời Dữ Liệu — Từ Sinh Ra Đến Xóa Đi

```
                              THỜI GIAN
    ──────────────────────────────────────────────────────────▶

    SINH RA         THU THẬP        LƯU TRỮ         XÓA
    ────────        ────────        ────────         ────

    Go service      Prometheus      ┌─ HOT (0-7 ngày)
    emit metric ──→ scrape ──────→ │  SSD, full speed
                                    │  query < 1s
                                    │
                                    ├─ WARM (7-30 ngày)
                                    │  HDD, slower
                                    │  query < 5s
                                    │
                                    ├─ COLD (30-90+ ngày)
                                    │  MinIO/S3, cheapest
                                    │  restore khi cần
                                    │
                                    └─ DELETE (90-180 ngày)
                                       Tự động xóa

    ┌──────────────────────────────────────────────────────┐
    │              Vòng đời cụ thể mỗi loại               │
    ├──────────────┬───────────┬─────────┬─────────────────┤
    │              │ Hot       │ Warm    │ Cold/Delete     │
    ├──────────────┼───────────┼─────────┼─────────────────┤
    │ Metrics      │ Prometheus│ Thanos  │ Thanos MinIO    │
    │              │ 0-15 ngày │ (auto)  │ 15 ngày - 1 năm│
    ├──────────────┼───────────┼─────────┼─────────────────┤
    │ Logs         │ ES SSD    │ ES HDD  │ ES snapshot→S3  │
    │              │ 0-7 ngày  │ 7-30d   │ 30-180d, delete │
    ├──────────────┼───────────┼─────────┼─────────────────┤
    │ Traces       │ Jaeger/ES │ —       │ Delete sau 7d   │
    │              │ 0-7 ngày  │         │ (volume quá lớn)│
    ├──────────────┼───────────┼─────────┼─────────────────┤
    │ Audit Logs   │ PostgreSQL│ —       │ MinIO 2 năm     │
    │              │ (always)  │         │ (compliance)    │
    └──────────────┴───────────┴─────────┴─────────────────┘
```

---

## 5. Data Flow — Toàn Bộ Luồng Dữ Liệu

```
┌──────────────────────────────────────────────────────────────────────────┐
│                     YOUR MICROSERVICES                                    │
│                                                                          │
│   ┌──────────────┐    ┌──────────────┐    ┌──────────────┐              │
│   │ order-service │    │ user-service  │    │ payment-svc  │   ...       │
│   │   :8080       │    │   :8081       │    │   :8082      │              │
│   └──┬───┬───┬────┘    └──┬───┬───┬────┘    └──┬───┬───┬────┘           │
│      │   │   │            │   │   │            │   │   │                │
│      │   │   │            │   │   │            │   │   │                │
└──────┼───┼───┼────────────┼───┼───┼────────────┼───┼───┼────────────────┘
       │   │   │            │   │   │            │   │   │
       │   │   └── traces ──┼───┼───┴── traces ──┼───┼───┘
       │   │      (OTel SDK)│   │      (OTel SDK)│   │
       │   │                │   │                │   │
       │   └── logs ────────┼───┴── logs ────────┼───┘
       │      (stdout JSON) │      (stdout JSON) │
       │                    │                    │
       └── metrics ─────────┴── metrics ─────────┘
          (GET /metrics)       (GET /metrics)

       ▼ METRICS (PULL)         ▼ LOGS (PUSH)              ▼ TRACES (PUSH)
       │                        │                           │
┌──────▼──────┐          ┌──────▼──────┐             ┌──────▼──────┐
│ Prometheus   │          │  Filebeat    │             │ OTel        │
│ (scrape 15s) │          │ (collect     │             │ Collector   │
│              │          │  container   │             │ (sampling,  │
│              │          │  stdout)     │             │  batching)  │
└──────┬──────┘          └──────┬──────┘             └──────┬──────┘
       │                        │                           │
       │                  ┌─────▼──────┐                    │
       │                  │   Kafka     │ (Mode B only)     │
       │                  │  (buffer    │                    │
       │                  │   24h)      │                    │
       │                  └─────┬──────┘                    │
       │                        │                           │
       │                  ┌─────▼──────┐                    │
       │                  │  Logstash   │                    │
       │                  │ (parse JSON,│                    │
       │                  │  enrich,    │                    │
       │                  │  grok)      │                    │
       │                  └─────┬──────┘                    │
       │                        │                           │
       │                  ┌─────▼──────┐             ┌──────▼──────┐
       │                  │Elasticsearch│             │   Jaeger     │
       │                  │(full-text   │             │  (trace      │
       │                  │ index)      │◄────────────│   storage,   │
       │                  │             │ shared ES   │   uses ES)   │
       │                  └──────┬─────┘             └──────┬──────┘
       │                         │                          │
  ┌────▼────┐                    │                          │
  │ Thanos   │                   │                          │
  │ Sidecar  │                   │                          │
  └────┬────┘                    │                          │
       │ upload blocks           │                          │
  ┌────▼─────┐                   │                          │
  │  MinIO   │                   │                          │
  │ (S3-like)│                   │                          │
  │ 1 year   │                   │                          │
  └────┬─────┘                   │                          │
       │                         │                          │
       └──────────┬──────────────┴──────────────────────────┘
                  │
           ┌──────▼──────┐          ┌──────────────┐
           │   Grafana    │          │  Next.js      │
           │  (PromQL +   │─ embed ─▶│  Dashboard    │
           │   ES query + │  iframe  │  (admin UI)   │
           │   Jaeger)    │          │               │
           └──────┬──────┘          └───────────────┘
                  │
           ┌──────▼──────┐
           │ Alertmanager │
           │              │
           └──┬───┬───┬──┘
              │   │   │
              ▼   ▼   ▼
           Slack Email PagerDuty
```

---

## 6. Chạy Độc Lập Hay Tích Hợp?

### Câu trả lời ngắn

> **LogMon TÍCH HỢP với microservices của bạn**, nhưng bản thân nó **CHẠY SONG SONG, KHÔNG XEN VÀO business logic**.

### Phân tách rõ ràng

```
┌──────────────────────────────────────────────────────────────┐
│                    HỆ THỐNG CỦA BẠN                          │
│                                                              │
│   order-service ──→ user-service ──→ payment-service         │
│        │                  │                │                 │
│        ▼                  ▼                ▼                 │
│   PostgreSQL           PostgreSQL       Stripe API           │
│                                                              │
│   (Business logic: đặt hàng, thanh toán, user management)   │
└──────────────────────────────────────────────────────────────┘
        │ metrics              │ logs              │ traces
        │ (GET /metrics)       │ (stdout JSON)     │ (OTel SDK)
        ▼                      ▼                   ▼
┌──────────────────────────────────────────────────────────────┐
│                      LOGMON PLATFORM                          │
│                                                              │
│   Prometheus ── Thanos ── Grafana ── Alertmanager            │
│   Filebeat ── Kafka ── Logstash ── Elasticsearch             │
│   OTel Collector ── Jaeger                                   │
│   LogMon Backend (alerting, SLO, incident, notification)     │
│   Next.js Dashboard                                          │
│                                                              │
│   (Observability: quan sát, cảnh báo, incident management)   │
└──────────────────────────────────────────────────────────────┘
```

### Điểm tích hợp — Microservices cần thêm gì?

| Thay đổi trong microservice | Effort | Mô tả |
|------------------------------|--------|-------|
| Expose `/metrics` endpoint | **5 dòng code** | Thêm `prometheus/client_golang` middleware vào Gin |
| Ghi log ra stdout JSON | **Đã sẵn** (nếu dùng zerolog) | LogMon collect từ Docker container stdout |
| Thêm OTel SDK | **~20 dòng code** | Auto-instrument Gin, pgx, go-redis. Zero manual spans |
| Inject trace_id vào log | **~5 dòng code** | OTel context → zerolog field |

**Tổng effort:** ~30 dòng code per service. Không thay đổi business logic. Không thay đổi API. Không thay đổi database.

### LogMon KHÔNG làm gì?

```
❌ Không xử lý đặt hàng, thanh toán, user management
❌ Không proxy request (không phải API Gateway)
❌ Không thay thế database của bạn
❌ Không can thiệp vào business flow
❌ Nếu LogMon down → microservices vẫn chạy bình thường
   (chỉ mất observability, không mất business)
```

### LogMon CÓ LÀM gì?

```
✅ Thu thập metrics từ mọi service (CPU, latency, error rate)
✅ Tập trung logs (thay vì SSH từng server để đọc)
✅ Trace request xuyên services (tìm bottleneck)
✅ Cảnh báo tự động (Slack, PagerDuty khi có sự cố)
✅ Track SLO (error budget, compliance)
✅ Quản lý incident (lifecycle, on-call, postmortem)
✅ Dashboard cho mọi persona (dev, devops, SRE, manager)
```

---

## 7. Vận Hành Production — Ngày Bình Thường vs Ngày Có Sự Cố

### Ngày bình thường (99% thời gian)

```
08:00  SRE check Grafana dashboard → tất cả services healthy (green)
       SLO dashboard: availability 99.97%, budget remaining 89%
       Không có active alerts
       → Chuyển sang làm việc khác

09:00  Developer deploy feature mới
       → CI/CD chạy tests → build image → deploy staging → deploy production
       → Post-deploy: Grafana auto-refresh, error rate không tăng
       → Done. Không cần manual verification.

14:00  Weekly SLO report gửi tự động qua email
       → Manager xem: "Tuần này 0 incidents, MTTR N/A, tất cả SLOs met"

17:00  DevOps check infrastructure dashboard
       → ES disk 62%, Prometheus healthy, Kafka lag 0
       → Nothing to do
```

### Ngày có sự cố (rare nhưng critical)

```
╔═══════════════════════════════════════════════════════════════════════╗
║  TIMELINE CỦA MỘT INCIDENT THỰC TẾ                                  ║
╠═══════════════════════════════════════════════════════════════════════╣
║                                                                       ║
║  14:28  [Prometheus]                                                  ║
║         Alert rule evaluate: error_rate{service="order"} = 0.08      ║
║         Threshold: > 0.05 → PENDING                                  ║
║                                                                       ║
║  14:30  [Prometheus → Alertmanager]                                   ║
║         Alert FIRING (đã pending 2m, vượt "for: 2m")                 ║
║         → Alertmanager route: severity=critical → Slack + PagerDuty  ║
║                                                                       ║
║  14:30  [LogMon Backend]                                              ║
║         AlertFired event → Auto-create Incident SEV1                  ║
║         → On-call engineer nhận PagerDuty notification                ║
║                                                                       ║
║  14:31  [Engineer mở Grafana]                                         ║
║         Service Overview: order-service error rate 8%, spike từ 14:28 ║
║         → Click "View logs" → filter level=error, last 15m            ║
║         → Thấy: "get user: connection refused" (100+ entries)         ║
║                                                                       ║
║  14:32  [Engineer xem Traces]                                         ║
║         → Search traces: service=order-service, status=error          ║
║         → Trace waterfall: order-service (120ms) → user-service (ERR) ║
║         → user-service span: "pgx: connection pool exhausted"         ║
║         → ROOT CAUSE FOUND: user-service DB pool config               ║
║                                                                       ║
║  14:33  [Engineer update Incident]                                    ║
║         POST /incidents/inc-001/timeline                              ║
║         "Root cause: user-service DB pool max=10, cần tăng lên 50"   ║
║         Status: open → mitigating                                     ║
║                                                                       ║
║  14:35  [Deploy hotfix]                                               ║
║         git push → CI/CD → deploy user-service with pool_max=50      ║
║                                                                       ║
║  14:37  [Prometheus]                                                  ║
║         error_rate{service="order"} = 0.001 → Alert RESOLVED          ║
║         → Alertmanager → Slack: "✅ Alert Resolved"                   ║
║         → Incident auto-resolve                                       ║
║                                                                       ║
║  14:37  [LogMon Backend]                                              ║
║         IncidentResolved event:                                       ║
║         - MTTR = 7 phút (14:30 → 14:37)                              ║
║         - SLO impact: budget consumed 0.3%                            ║
║         - Notification: "Incident resolved" → Slack                   ║
║                                                                       ║
║  Ngày hôm sau:                                                        ║
║  [Engineer viết Postmortem]                                           ║
║         Root cause: DB pool default=10, không có monitoring            ║
║         Action items:                                                  ║
║         1. Tăng pool mọi services lên 50 (do: 3 ngày)               ║
║         2. Thêm alert: pg_stat_activity_count > 80% pool (do: 1 ngày)║
║         3. Thêm circuit breaker order→user (do: 1 tuần)              ║
║                                                                       ║
╚═══════════════════════════════════════════════════════════════════════╝
```

### Các hoạt động vận hành định kỳ

| Tần suất | Hoạt động | Ai làm | Công cụ |
|-----------|-----------|--------|---------|
| **Hàng ngày** | Check dashboard, review alerts đêm qua | On-call SRE | Grafana |
| **Hàng tuần** | Review SLO compliance report, MTTR trends | SRE Team Lead | Email report (auto) |
| **Hàng tuần** | Review alert noise (false positives) | SRE | Alert Noise Report |
| **Hàng tháng** | Capacity planning: disk, memory trends | DevOps | Infrastructure dashboard |
| **Hàng tháng** | Cost review: ingestion volume, storage growth | Engineering Manager | Cost dashboard |
| **Khi cần** | Tune alert thresholds (giảm noise) | SRE | API: PUT /alerts/rules/:id |
| **Khi cần** | Update ILM policy (tăng/giảm retention) | DevOps | API: PUT /pipeline/ilm |
| **Khi cần** | On-call rotation handoff | SRE | API: PUT /oncall/schedule |
| **Sau incident** | Postmortem + action items | Incident owner | API: POST /incidents/:id/postmortem |

---

## 8. LogMon Trong Bức Tranh Lớn — Ecosystem Integration

```
┌─────────────────────────────────────────────────────────────────────┐
│                        YOUR ORGANIZATION                             │
│                                                                     │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐              │
│  │ Source Code  │   │  CI/CD      │   │ Cloud Infra │              │
│  │ (GitHub)     │   │ (Actions)   │   │ (AWS/GCP)   │              │
│  └──────┬──────┘   └──────┬──────┘   └──────┬──────┘              │
│         │                 │                  │                      │
│         ▼                 ▼                  ▼                      │
│  ┌──────────────────────────────────────────────────────────┐      │
│  │              MICROSERVICES (Production)                    │      │
│  │  order-svc   user-svc   payment-svc   inventory-svc      │      │
│  │  PostgreSQL  Redis      Kafka         Elasticsearch       │      │
│  └───────┬────────────┬───────────┬──────────────────────────┘      │
│          │            │           │                                  │
│     metrics        logs        traces                               │
│          │            │           │                                  │
│  ┌───────▼────────────▼───────────▼──────────────────────────┐      │
│  │                  LOGMON PLATFORM                            │      │
│  │                                                            │      │
│  │  Collect → Store → Analyze → Alert → Incident → Report    │      │
│  └──────┬─────────┬──────────┬────────────┬──────────────────┘      │
│         │         │          │            │                          │
│         ▼         ▼          ▼            ▼                          │
│  ┌──────────┐ ┌────────┐ ┌────────┐ ┌──────────┐                   │
│  │ Slack    │ │PagerDuty│ │ Email  │ │ Grafana  │                   │
│  │(alerts)  │ │(on-call)│ │(reports│ │(dashboards│                  │
│  │          │ │         │ │ weekly)│ │          │                   │
│  └──────────┘ └────────┘ └────────┘ └──────────┘                   │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────┐      │
│  │              OTHER SYSTEMS (tích hợp qua webhook/API)     │      │
│  │                                                            │      │
│  │  Jira (tạo ticket từ postmortem action items)              │      │
│  │  Confluence (publish postmortem reports)                    │      │
│  │  Telegram Bot (alert notifications)                        │      │
│  │  Custom scripts (query LogMon API for automation)          │      │
│  └──────────────────────────────────────────────────────────┘      │
└─────────────────────────────────────────────────────────────────────┘
```

### Điểm tích hợp với hệ thống bên ngoài

| Hệ thống bên ngoài | Cách tích hợp | Hướng | Mục đích |
|---------------------|---------------|-------|----------|
| **Slack** | Webhook (HTTP POST) | LogMon → Slack | Alert + incident notifications |
| **PagerDuty** | Events API v2 | LogMon → PagerDuty | On-call paging, escalation |
| **Email (SMTP)** | SMTP protocol | LogMon → Email server | Reports, digests |
| **Microsoft Teams** | Incoming Webhook | LogMon → Teams | Enterprise notifications |
| **Jira / Linear** | Generic Webhook | LogMon → Jira | Auto-create tickets từ postmortem |
| **Telegram** | Bot API (webhook) | LogMon → Telegram | Custom alert channel |
| **CI/CD (GitHub Actions)** | Webhook callback | CI/CD → LogMon (post-deploy verify) | Verify deploy health |
| **Custom tools** | REST API | Any → LogMon | Query logs, metrics, create alerts programmatically |

### Nếu LogMon down thì sao?

```
Microservices:     VẪN CHẠY bình thường. Business không bị ảnh hưởng.
                   (LogMon là observer, không phải participant trong business flow)

Mất gì:           - Không có metrics (Prometheus down) → không detect anomalies
                   - Không có logs tập trung → phải SSH đọc log thủ công
                   - Không có traces → debug cross-service bằng... cầu nguyện
                   - Không có alerts → sự cố xảy ra mà không ai biết

Kết luận:          LogMon down = bạn MÙ về production, nhưng production vẫn chạy.
                   → Đó là lý do LogMon cần HA (high availability) ở Phase 4.
```

---

## 9. Tóm Tắt Bằng Phép Ẩn Dụ

```
Microservices     = CƠ THỂ (tim, phổi, gan, thận)
                    → Chạy business logic

LogMon            = HỆ THẦN KINH + BÁC SĨ TRỰC
                    → Cảm nhận (metrics, logs, traces)
                    → Cảnh báo (alerts)
                    → Chẩn đoán (dashboards, traces)
                    → Điều trị (incident management)
                    → Ghi chép bệnh án (postmortem)
                    → Đo sức khỏe (SLO)

Grafana           = MÀN HÌNH THEO DÕI bệnh nhân (ECG, SpO2, huyết áp)
Prometheus        = CÁC CẢM BIẾN đo sinh hiệu mỗi 15 giây
Elasticsearch     = HỒ SƠ BỆNH ÁN chi tiết (full-text search)
Jaeger            = CHỤP CT/MRI — nhìn bên trong request flow
Alertmanager      = HỆ THỐNG BÁO ĐỘNG khi sinh hiệu vượt ngưỡng
Incident Mgmt     = QUY TRÌNH CẤP CỨU (triage → assign → treat → discharge)
SLO               = CAM KẾT CHẤT LƯỢNG DỊCH VỤ ("uptime ≥ 99.9%")
```

---

> Chi tiết đầy đủ: [`logmon.md`](logmon.md) (4400+ dòng) | Tóm tắt nhanh: [`logmon_summary.md`](logmon_summary.md)
