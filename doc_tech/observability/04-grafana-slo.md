# Grafana & SLO/Error Budget
> Module OBS-4 · dashboard, datasource, SLI/SLO, burn-rate alert · Độ khó: 🥉→🥇 · Prereqs: OBS-1

## 1. Vì sao kỹ năng này quan trọng trong LogMon

LogMon là nền tảng observability bán chính sự tin cậy. Nếu chính LogMon không đo được "dịch vụ của tôi đang khỏe đến mức nào", sản phẩm vô nghĩa. Hai năng lực mấu chốt:

- **Grafana** — lớp trực quan hóa cho cả 3 trụ cột (metrics/logs/traces). Trong repo, Grafana đọc datasource Prometheus/Elasticsearch/Jaeger và render dashboard-as-code (`infra/grafana/dashboards/`).
- **SLO/Error budget/Burn rate** — ngôn ngữ định lượng độ tin cậy. Thay vì "trang có vẻ chậm", ta nói "SLO availability 99.9%, còn 40% error budget, burn rate 1h = 6x → page". Đây là nền cho `slo-dashboard` và burn-rate alert (GĐ3, **đang làm dở** — `slo/domain/` đã có aggregate SLO, nhưng rule generator còn ở trạng thái test-trước-impl; xem §4).

Kỹ năng này nối thẳng vào alerting BC đã có thật trong repo: rule do LogMon quản lý vòng đời, Prometheus đánh giá, Alertmanager route (ADR-024, `doc_v2/05-alerting-slo.md:3`).

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Bắt đầu từ một câu hỏi: *làm sao biết dịch vụ "đủ tốt"?*

1. **Đo bằng số (metric).** Mỗi request, ta tăng một bộ đếm. Trong LogMon, mỗi HTTP request làm tăng `logmon_http_requests_total` và ghi độ trễ vào `logmon_http_request_duration_seconds` — xem `backend/internal/shared/metrics/metrics.go:24-38`.
2. **Prometheus là cơ sở dữ liệu chuỗi-thời-gian (PULL).** Nó tự gọi `GET /metrics` của service mỗi 15s (`infra/prometheus/prometheus.yml`, job `logmon-services`) và lưu lại theo thời gian. Không phải service đẩy đi.
3. **PromQL** là ngôn ngữ truy vấn các chuỗi đó. `rate(...[5m])` = tốc độ tăng trung bình/giây trong 5 phút.
4. **Grafana** chỉ là cái màn hình: gửi PromQL tới Prometheus, vẽ ra đồ thị.
5. **SLI (indicator)** = một con số đo trải nghiệm, ví dụ tỉ lệ request *không lỗi*. **SLO (objective)** = mục tiêu cho SLI đó, ví dụ 99.9% trong 28 ngày. **Error budget** = phần được phép hỏng = `1 − SLO` = 0.1%. **Burn rate** = tốc độ tiêu budget so với mức "vừa khít hết đúng cuối kỳ" (1x). Burn rate 14.4x nghĩa là nếu cứ thế, budget 30 ngày cháy trong ~2 ngày.

Mấu chốt tư duy: **alert trên triệu chứng người dùng cảm nhận (error rate, latency), không alert trên nguyên nhân (CPU cao)** — và dùng error budget để quyết định *khi nào* đáng đánh thức người.

## 3. Khái niệm cốt lõi (tăng dần độ khó)

### 3.1 Metric types
| Loại | Ý nghĩa | Ví dụ trong LogMon |
|------|---------|--------------------|
| Counter | Chỉ tăng, suffix `_total` | `logmon_http_requests_total` |
| Histogram | Phân phối giá trị theo bucket `_bucket{le=...}` | `logmon_http_request_duration_seconds` |
| Gauge | Lên/xuống tự do | `logmon_outbox_lag_seconds` (`backend/internal/shared/outbox/metrics.go:33`) |

### 3.2 RED method (cho service)
Rate, Errors, Duration — 3 con số mô tả sức khỏe một service. Dashboard `service-overview.json` đã hiện thực đúng RED:
- Rate: `sum by (job) (rate(logmon_http_requests_total[5m]))`
- Errors (panel "Error Ratio" — là *tỉ lệ* 5xx/tổng, không phải rate thô): `sum by (job) (rate(logmon_http_requests_total{status=~"5.."}[5m])) / sum by (job) (rate(logmon_http_requests_total[5m]))`
- Duration (P95): `histogram_quantile(0.95, sum by (job, le) (rate(logmon_http_request_duration_seconds_bucket[5m])))`

> USE method (Utilization/Saturation/Errors) là cặp song sinh dành cho *tài nguyên hạ tầng* (CPU, disk) — đó là dashboard `infrastructure.json`.

### 3.3 Recording rule vs Alerting rule
- **Recording rule**: tính trước một biểu thức tốn kém, lưu thành chuỗi mới (vd `slo:errors:ratio_rate5m`). Dùng cho `histogram_quantile` nặng và cho SLO.
- **Alerting rule**: biểu thức + ngưỡng + `for:` → khi đúng đủ lâu thì firing. Xem `infra/prometheus/rules/base-alerts.yml`.

### 3.4 Multiwindow multi-burn-rate (đỉnh cao SLO alerting)
Công thức chuẩn Google SRE Workbook Ch.5 (ADR-025). Mỗi alert là `long-window AND short-window` để vừa nhạy vừa tự tắt:

| Severity | Budget cháy để trigger | Long | Short | Burn factor |
|----------|------------------------|------|-------|-------------|
| page | 2% | 1h | 5m | 14.4 |
| page | 5% | 6h | 30m | 6 |
| ticket | 10% | 3d | 6h | 1 |

`for: 1m` (critical) và `for: 5m` (warning) là **bất biến** được enforce ở domain — `Severity.MinForDuration()` tại `backend/internal/alerting/domain/severity.go:32-41`.

## 4. LogMon dùng nó thế nào (bám code thật — implemented/planned)

**Đã implemented:**
- Metric layer: `backend/internal/shared/metrics/metrics.go:21-59` định nghĩa 2 collector và `ObserveRequest`. Middleware gọi nó với **route template** (`c.FullPath()`, không phải URL thật) để tránh high-cardinality: `backend/internal/shared/middleware/middleware.go:66-75`.
- Expose `/metrics`: `backend/cmd/userservice/main.go:369` dùng `promhttp.HandlerFor(mx.Registry(), ...)`.
- Prometheus scrape + rule load: `infra/prometheus/prometheus.yml` (rule_files trỏ cả `rules/*.yml` static lẫn `generated/*.yml` do BC render).
- Static alerts: `infra/prometheus/rules/base-alerts.yml` — ServiceDown, HighErrorRate, HighLatencyP95, OutboxLag, PGConnHigh, ESDiskHigh, CollectorQueueFull, Watchdog (deadman, **cố ý không có `for:`**).
- Grafana provisioning: datasource `infra/grafana/provisioning/datasources/datasources.yml` (có derived field `trace_id` ES→Jaeger), dashboard provider `.../dashboards/dashboards.yml`.
- 4 dashboard JSON: `service-overview`, `infrastructure`, `logs-explorer`, `pipeline-health`.
- Alerting BC vòng đời rule: validate PromQL bằng parser chính chủ (`backend/internal/alerting/adapters/promql/validator.go:25-33`); render→validate→atomic-write→reload Prometheus (`backend/internal/alerting/adapters/promfile/syncer.go:60-96`); enforce label/annotation bắt buộc `summary` + `runbook_url` ở domain (`backend/internal/alerting/domain/rule.go:116-121`); silence/ack qua Alertmanager API.

**Đang làm dở (TDD RED — domain xong, generator chưa):**
- SLO BC: thư mục `backend/internal/slo/` **không còn trống**. Tầng `domain/` đã có aggregate SLO thật (`backend/internal/slo/domain/slo.go` — value object `SLIType`/`SLOID`, validate invariant, `ErrorBudget() = 1 − target`, domain events `SLODefined`/`BudgetExhausted` ở `events.go`). NHƯNG recording-rule generator còn ở trạng thái **test-trước-impl**: `rules_test.go` đã viết kỳ vọng cho `GenerateRuleGroup()`, `RecordingRule`, `AlertingRule` và recording rule `slo:errors:ratio_rate*`, nhưng **chưa có `rules.go`** → package test hiện **chưa compile** (`go vet ./internal/slo/...` báo `undefined: domain.RecordingRule`). Đây chính là task 🥇 trong §7 — viết `rules.go` cho test xanh.

**Planned (chưa có code):**
- SLO app/ports/adapters + budget snapshot job: chưa có (chỉ mới `domain/`); recording rules `slo:errors:ratio_rate*`, burn-rate alert, snapshot goroutine đều là GĐ3 (`doc_v2/05-alerting-slo.md:134-178`).
- `slo-dashboard.json`: doc liệt kê nhưng file **chưa tồn tại** trong `infra/grafana/dashboards/`.
- Exemplars (metrics→traces): doc_v2/04 §3 mô tả nhưng dashboard JSON hiện **chưa bật** exemplar.
- `logmon_http_requests_in_flight` (gauge): panel "In-Flight Requests" trong `service-overview.json` truy vấn metric này, **nhưng Go code chưa định nghĩa nó** → panel rỗng cho tới khi implement. Đây là một bug-thật đáng sửa.
- Thanos, go-redis instrumentation, k8s `PrometheusRule` CR: đều planned (Mode B / GĐ4).

## 5. Best practices (mỗi mục kèm 1 nguồn đã research)

1. **Alert trên triệu chứng, đặt RED ở trung tâm dashboard.** RED dashboard là proxy cho trải nghiệm người dùng và là nơi nên alert. ([Grafana dashboard best practices](https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/best-practices/))
2. **Dùng multiwindow multi-burn-rate đúng bộ số chuẩn (14.4/6/1).** Đừng tự chế ngưỡng — bộ số này có tính chất thống kê đã được chứng minh. ([Google SRE Workbook — Alerting on SLOs](https://sre.google/workbook/alerting-on-slos/))
3. **Pre-compute quantile/SLI bằng recording rule.** `histogram_quantile` tốn kém; tính trước theo naming `scope:metric:aggregation` (vd `slo:errors:ratio_rate5m`). ([Prometheus histograms & summaries](https://prometheus.io/docs/practices/histograms/))
4. **Mượn pattern Sloth/Pyrra để sinh rule SLO.** Sinh recording + multi-burn-rate alert tự động theo type ratio/latency thay vì viết tay. ([SLO made easy with Sloth and Pyrra](https://0xdc.me/blog/service-level-objectives-made-easy-with-sloth-and-pyrra/))
5. **Bật exemplar để nhảy từ metric sang trace.** Exemplar là cây cầu từ "thấy latency cao" sang "xem đúng trace gây ra p99". ([Grafana — configure and use exemplars](https://grafana.com/docs/grafana-cloud/send-data/traces/configure/exemplars/))
6. **Cân nhắc native histogram cho histogram mới.** Đã GA (PromCon EU 2025); chính xác hơn, ít chỉnh bucket; metric quan trọng nên gửi song song classic + native trong giai đoạn chuyển tiếp. ([Grafana — Prometheus native histograms](https://grafana.com/blog/prometheus-native-histograms-in-grafana-cloud-more-precise-easier-to-use-and-better-compatibility/))

## 6. Lỗi thường gặp & anti-patterns

- **Quantile không cộng được.** Đừng `avg(p95)` qua nhiều instance. Phải `histogram_quantile(0.95, sum by (le)(rate(..._bucket[5m])))` — gộp bucket trước, tính quantile sau (đúng như `base-alerts.yml` HighLatencyP95).
- **High-cardinality labels.** Không nhét `user_id`/`request_id`/`trace_id` vào label, không dùng URL thật làm `path`. LogMon ép `c.FullPath()` (template) chính vì thế (CLAUDE.md, doc_v2/04 §1.3).
- **Alert trên raw counter.** `logmon_http_requests_total > 100` vô nghĩa (counter chỉ tăng). Luôn `rate()`/`increase()`.
- **Panel trỏ metric chưa tồn tại.** Như `logmon_http_requests_in_flight` ở §4 — dashboard xanh rờn nhưng panel rỗng. Luôn kiểm tra metric có thật ở `/metrics`.
- **`for:` cho Watchdog.** Thêm `for:` vào deadman switch sẽ phá chức năng — comment trong `base-alerts.yml` đã cảnh báo.
- **Single-window burn-rate alert.** Chỉ 1 cửa sổ → hoặc quá nhạy (spam) hoặc quá trễ. Phải cặp long+short.
- **Page vào việc không actionable.** Mọi page phải có `runbook_url` (domain đã enforce) và phải đáng đánh thức người.

## 7. Lộ trình luyện tập NGAY trong repo LogMon

### 🥉 Cơ bản
1. `make up` rồi mở Prometheus, chạy `rate(logmon_http_requests_total[5m])`; tạo vài request tới userservice và xem số đổi.
2. Mở Grafana, import `infra/grafana/dashboards/service-overview.json`, xác định panel nào đang rỗng (gợi ý: In-Flight) và giải thích vì sao.
3. Thêm gauge `logmon_http_requests_in_flight` vào `backend/internal/shared/metrics/metrics.go`, Inc/Dec trong middleware `Metrics()` — panel In-Flight phải lên số.
4. Viết PromQL P50 + P99 cạnh P95 hiện có, thêm thành 2 panel mới trong `service-overview.json`.

### 🥈 Trung cấp
1. Thêm 1 alert rule mới vào `infra/prometheus/rules/base-alerts.yml` (vd `DLQRateHigh` từ bảng doc_v2/05 §3), `make up` lại, kiểm tra firing ở Prometheus `/alerts`.
2. Tạo recording rule `job:logmon_http_errors:ratio_rate5m` (errors/total) thành file `rules/recording.yml`, dùng nó trong panel Error Ratio thay biểu thức inline.
3. Dùng API alerting BC tạo một rule động (qua handler `backend/internal/alerting/adapters/http/handler.go`), xác minh file `generated/logmon-generated.yml` được render đúng và Prometheus reload.
4. Viết test cho gauge mới ở `metrics_test.go` (table-driven, `testify/require`), chạy `go test -race ./internal/shared/metrics/...`.

### 🥇 Nâng cao
1. Tạo `infra/grafana/dashboards/slo-dashboard.json` mới: panel error-budget remaining + 3 burn-rate (1h/6h/3d) dùng PromQL multi-burn-rate ở doc_v2/05 §4.2.
2. Viết bộ recording rules `slo:errors:ratio_rate{5m,30m,1h,6h,3d}` và cặp alert page/ticket theo bảng 14.4/6/1; validate bằng `promtool check rules` hoặc qua `promfile.validate` in-process.
3. Bật exemplar: thêm `ObserveWithExemplar(trace_id)` cho histogram trong middleware, bật `--enable-feature=exemplar-storage` ở Prometheus, thêm "Query with exemplars" vào panel P95.
4. Làm xanh test SLO BC: `backend/internal/slo/domain/` đã có aggregate + `rules_test.go` (kỳ vọng `GenerateRuleGroup()` sinh `RecordingRule`/`AlertingRule` theo bảng 14.4/6/1) nhưng thiếu `rules.go` → viết `rules.go` cho `go test ./internal/slo/...` PASS, rồi mở rộng app/ports/adapters tái dùng `ports.RuleSyncer` để đẩy qua đúng pipeline đã có.
5. Thêm meta-monitoring: route Watchdog tới healthchecks.io và mô phỏng pipeline chết để xác nhận deadman switch (doc_v2/05 §5).

## 8. Skill/agent ECC nên dùng khi luyện

- **ecc:architect** — khi scaffold SLO BC (🥇 task 4): quyết định ranh giới domain/app/ports, đảm bảo không vi phạm layer direction và tái dùng `RuleSyncer` thay vì pipeline thứ hai.
- **ecc:dashboard-builder** + **ecc:design-system** — khi dựng `slo-dashboard.json` và sửa panel (🥈/🥇): đúng phần "admin dashboard data-table" mà CLAUDE.md khuyến nghị (không dùng taste-skill cho dashboard).
- **ecc:production-audit** — sau khi thêm alert/recording rule: rà soát alert fatigue, label cardinality, runbook coverage, deadman switch trước khi coi là xong.
- Phụ trợ: **ecc:go-test** (TDD cho gauge mới), **ecc:go-review** cho thay đổi `metrics.go`/middleware.

## 9. Tài nguyên học thêm

- [Google SRE Workbook — Alerting on SLOs](https://sre.google/workbook/alerting-on-slos/) — chương gốc của multiwindow multi-burn-rate, nguồn của ADR-025.
- [Grafana — How to implement multi-window multi-burn-rate alerts](https://grafana.com/blog/how-to-implement-multi-window-multi-burn-rate-alerts-with-grafana-cloud/) — ví dụ thực hành PromQL burn-rate.
- [Grafana dashboard best practices](https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/best-practices/) — RED-at-center, maturity model, dashboard-as-code.
- [Prometheus — Histograms and summaries](https://prometheus.io/docs/practices/histograms/) — vì sao quantile không cộng được, khi nào dùng recording rule.
- [SLO made easy with Sloth and Pyrra](https://0xdc.me/blog/service-level-objectives-made-easy-with-sloth-and-pyrra/) — pattern sinh SLO rule mà SLO BC sẽ học theo.
- [Grafana — Prometheus native histograms (GA 2025)](https://grafana.com/blog/prometheus-native-histograms-in-grafana-cloud-more-precise-easier-to-use-and-better-compatibility/) — định hướng histogram thế hệ mới.

## 10. Checklist "đã hiểu"

- [ ] Phân biệt được Counter/Histogram/Gauge và chỉ ra ví dụ tương ứng trong code LogMon.
- [ ] Giải thích được vì sao `path` phải là route template (`c.FullPath()`) chứ không phải URL thật.
- [ ] Viết được PromQL P95 đúng (gộp bucket trước, `histogram_quantile` sau) mà không `avg` quantile.
- [ ] Định nghĩa được SLI, SLO, error budget, burn rate bằng lời và bằng số (99.9% → budget 0.1%).
- [ ] Đọc được bảng 14.4/6/1 và nói được vì sao cần cặp long+short window.
- [ ] Chỉ ra phần nào của Grafana/SLO trong LogMon là implemented / đang-làm-dở / planned (SLO BC: domain xong nhưng rule generator còn TDD-RED; slo-dashboard, exemplar, in-flight gauge: chưa có).
- [ ] Biết rule động đi qua pipeline render→validate→atomic-write→reload và vì sao validate phải đứng trước ghi file.
- [ ] Giải thích được Watchdog deadman switch và vì sao nó cố ý không có `for:`.
