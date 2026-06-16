# 13 — Architecture Decision Records

> Trạng thái: **Accepted** (giữ từ v1, vẫn đúng) · **Updated** (giữ quyết định, cập nhật chi tiết) · **Superseded** (bị thay thế) · **New** (mới trong v2). ADR mới đều có nguồn kiểm chứng (research 06/2026).

## Index

| # | Quyết định | Trạng thái |
|---|------------|-----------|
| 001 | Clean Architecture + DDD + CQRS cho BC phức tạp | Accepted |
| 002 | Kafka làm log buffer (Mode B) | Updated (KRaft — xem 027) |
| 003 | Elasticsearch thay Loki | Updated |
| 004 | Prometheus PULL model | Accepted |
| 005 | Grafana single pane (không Kibana) | Accepted |
| 006 | Dedicated exporters per component | Accepted |
| 007 | 2 deployment modes (A/B, compose profiles) | Accepted |
| 008 | CQRS cho read:write ~100:1 | Accepted |
| 009 | Domain events cho cross-BC | Accepted (cơ chế → 016) |
| 010 | OpenTelemetry cho tracing | Updated |
| 011 | Thanos cho long-term metrics | Accepted (tái xác nhận 2026) |
| 012 | Multi-tenancy qua Workspace | Updated |
| 013 | Log Search API | Accepted |
| 014 | Incident Management BC | Accepted |
| 015 | Notification Hub | Accepted |
| 016 | Transactional Outbox | Updated (SKIP LOCKED) |
| 017 | Object storage tiering | Updated (MinIO → 021) |
| **018** | **OTel Collector thay Filebeat + Logstash** | **New** |
| **019** | **ES data streams + ILM rollover thay daily indices** | **New** |
| **020** | **Jaeger v2 (v1 EOL); Tempo là alternative** | **New** |
| **021** | **Bỏ MinIO → cloud S3/B2 hoặc SeaweedFS** | **New** |
| **022** | **argon2id thay bcrypt** | **New** |
| **023** | **Refresh token rotation + reuse detection + CSRF** | **New** |
| **024** | **Rule sync qua file + reload (không build evaluator, không chờ API)** | **New** |
| **025** | **Multiwindow multi-burn-rate chuẩn Google SRE** | **New** |
| **026** | **Meta-monitoring: Watchdog deadman switch ngoài stack** | **New** |
| **027** | **Kafka 4.x KRaft-only** | **New** |
| **028** | **redis_rate (GCRA) thay sliding window tự viết** | **New** |
| **029** | **Tách demo workload khỏi platform** | **New** |
| **030** | **Modular monolith trước, tách service sau** | **New** |

---

## ADR giữ từ v1 (tóm tắt — chi tiết gốc trong doc/logmon.md §10, §19)

**001 — Clean Arch + DDD + CQRS** cho `alerting/slo/logpipeline/incident`; Clean Arch thuần cho `identity/notification`. Layer rule `adapters → ports ← app → domain`. Lý do: alerting/SLO/pipeline là business logic thật, không phải CRUD; testable không cần infra.

**004 — PULL model**: backpressure tự nhiên, service discovery qua `up`, Pushgateway cho batch jobs.

**005 — Grafana single pane**: 1 tool cho metrics+logs+traces; dashboards-as-code; correlation links.

**007 — 2 modes**: A (nhỏ, không Kafka/Thanos) / B (scale) qua compose profiles. Điểm chuyển: >5-10K logs/s hoặc cần SLO window >15d.

**008 — CQRS**: monitoring read:write ~100:1; read models cache/denormalized riêng.

**011 — Thanos** *(tái xác nhận 06/2026)*: sidecar + store + query + compactor vẫn là best practice cho single-Prometheus cần 90d-1y trên object storage. Mimir 3.0 quá khổ (RAM ~4x VM, kiến trúc Kafka-based nhiều microservices); VictoriaMetrics OSS không object-storage-native. Retention: raw 30d / 5m 180d / 1h 1y. *Nguồn: sanj.dev/post/prometheus-scaling-thanos-mimir-victoriametrics, onidel.com/blog/prometheus-storage-comparison-2025.*

**013 — Log Search API**, **014 — Incident BC**, **015 — Notification Hub**: giữ nguyên lý do v1 (programmability; full SRE lifecycle; plugin channels).

---

## ADR cập nhật

### ADR-003 (Updated) — Elasticsearch thay Loki

Giữ quyết định: full-text search mọi field + aggregations. Cập nhật 2026: (1) ES 9.x, license có AGPLv3 (open source trở lại từ 2024); (2) security on-by-default — không tắt; (3) **OpenSearch 3.x là alternative nghiêm túc** — DLS/FLS miễn phí (ES cần Platinum), Apache 2.0; chọn ES vì tốc độ phát triển + hệ sinh thái, nhưng nếu multi-tenancy rẽ hướng shared-index + DLS thì đánh giá lại OpenSearch. *Nguồn: elastic.co/blog/elasticsearch-is-open-source-again, bigdataboutique.com/blog/opensearch-vs-elasticsearch-compared.*

### ADR-010 (Updated) — OpenTelemetry

Giữ quyết định. Cập nhật: OTel Go **traces + metrics đã stable, logs còn Beta** → tiếp tục zerolog cho logs (inject trace context qua hook), không dùng OTel Logs SDK. Spanmetrics **connector** thay processor (processor đã bị gỡ). Deployment pattern agent + gateway. *Nguồn: opentelemetry.io/docs/languages/go, spanmetricsconnector README.*

### ADR-012 (Updated) — Workspace multi-tenancy

Giữ mô hình. Cập nhật cơ chế isolation: ES dùng **data-stream-per-workspace** (namespace) thay index prefix thủ công — đúng khuyến nghị cho 5-50 tenants; Prometheus label `workspace` **không phải security boundary** — backend inject matcher vào mọi query (pattern prom-label-proxy); tenancy cứng cho metrics → Mimir/Thanos Receive (tương lai xa). *Nguồn: bigdataboutique.com/blog/multi-tenancy-with-elasticsearch-and-opensearch.*

### ADR-016 (Updated) — Transactional Outbox

Giữ pattern. Cập nhật implementation: polling dùng **`FOR UPDATE SKIP LOCKED`** (nhiều relay instance an toàn), baseline poll 1s/batch 100, partial index trên `status='pending'`, mark published sau khi subscribers xử lý xong, cleanup 7 ngày. Alternative đóng hộp: Watermill v1.5 Forwarder + watermill-sql. *Nguồn: milanjovanovic.tech/blog/scaling-the-outbox-pattern, watermill.io/advanced/forwarder.*

### ADR-017 (Updated) — Storage tiering

Giữ 3-tier (hot ES SSD → warm → cold S3 snapshot). Cập nhật: cold tier dùng **searchable snapshots**; object storage không còn là MinIO (ADR-021).

---

## ADR mới (v2)

### ADR-018 — OTel Collector thay Filebeat + Logstash

**Status:** Accepted · **Supersedes:** pipeline logs của v1

**Context:** v1 dùng Filebeat → Kafka → Logstash → ES. Research 06/2026: (1) benchmark VictoriaMetrics 03/2026 trên 9 collectors — **Filebeat bắt đầu mất log trước 10K logs/s** (yêu cầu Mode B là >10K/s); (2) Logstash JVM 1-4GB chỉ để parse JSON; Elastic chính thức publish hướng dẫn convert Logstash → OTel Collector; (3) Elastic Agent ≥9.2 bản thân chạy EDOT (OTel) Collector bên trong — chính Elastic đã xoay trục; (4) LogMon vốn cần OTel Collector cho traces.

**Decision:** OTel Collector contrib làm shipper + processor duy nhất: agent (filelog receiver, parse JSON, resource attrs) → [Kafka Mode B] → gateway (elasticsearchexporter, ECS mapping, persistent queue). Enrichment phức tạp (nếu cần): ES ingest pipelines. Fluent Bit là alternative nhẹ hơn nếu OTel filelog gặp vấn đề hiệu năng thực tế.

**Consequences:** (+) bỏ 2 component, tiết kiệm ~1.5-3.5GB RAM; một config ngữ nghĩa cho 3 tín hiệu; vendor-neutral. (−) filelog operators ít chín hơn Beats ở edge cases (multiline stack traces — cần test kỹ); team phải học OTel config.

*Nguồn: victoriametrics.com/blog/log-collectors-benchmark-2026, elastic.co/observability-labs/blog/logstash-to-otel, elastic.co/observability-labs/blog/elastic-agent-pivot-opentelemetry.*

### ADR-019 — ES data streams + ILM rollover thay daily indices

**Status:** Accepted · **Supersedes:** index pattern `logs-{ws}-{svc}-{yyyy.MM.dd}` của v1

**Context:** Daily index per service per workspace → bùng nổ shard (ví dụ 20 services × 5 workspaces × 90 ngày = 9.000 indices), vi phạm giới hạn ≤1000 shards/node. Elastic khuyến nghị chính thức: data streams + rollover theo **`max_primary_shard_size: 50gb`** (không theo max_age đơn thuần — tránh index rỗng/lệch size).

**Decision:** Data stream `logs-{service}-{workspace}`; ILM hot (rollover 50gb hoặc 7d) → warm (shrink+forcemerge) → cold (searchable snapshot S3, Mode B) → delete. Shard 10-50GB, <200M docs.

**Consequences:** (+) shard count kiểm soát được, ILM per-workspace, đúng chuẩn ES 9.x. (−) không xóa "index theo ngày" thủ công được nữa — mọi retention qua ILM (đó là điều đúng).

*Nguồn: elastic.co/docs/.../size-shards (official sizing), elastic data streams docs.*

### ADR-020 — Jaeger v2; Tempo là alternative

**Status:** Accepted · **Supersedes:** Jaeger v1 trong v1

**Context:** Jaeger v1 **EOL 31/12/2025**. Jaeger v2 (11/2024) xây lại trên OTel Collector framework, nhận OTLP native, hiện v2.19. Grafana Tempo 3.0 là alternative object-storage-native (chi phí lưu rẻ hơn ~10x, TraceQL GA).

**Decision:** Jaeger v2.19 với ES backend (dùng chung cluster logs, retention 7d) — ít component mới nhất, đã có ES. Đánh giá lại Tempo nếu: chi phí ES cho traces thành vấn đề, hoặc đã chuẩn hóa hoàn toàn quanh Grafana UI. ClickHouse backend của Jaeger (alpha) — theo dõi.

*Nguồn: cncf.io/blog/2024/11/12/jaeger-v2-released, github.com/jaegertracing/jaeger/issues/6321 (EOL), grafana/tempo releases.*

### ADR-021 — Bỏ MinIO → cloud S3/B2 (ưu tiên) hoặc SeaweedFS

**Status:** Accepted · **Supersedes:** MinIO trong v1

**Context:** 2025 MinIO gỡ gần hết tính năng quản trị khỏi Web UI community edition (mất IAM/policies/monitoring UI), chuyển trọng tâm sang AIStor trả phí (~$96k/năm tối thiểu) — cộng đồng coi là maintenance mode.

**Decision:** Nhu cầu object storage của LogMon (Thanos blocks, ES snapshots, export files) → **cloud S3/Backblaze B2/R2** là thực dụng nhất (chi phí ~$0.005-0.023/GB/tháng, zero ops). Bắt buộc on-prem → **SeaweedFS** (Apache 2.0, 12+ năm). Garage thiếu lifecycle rules/versioning; Ceph quá nặng.

*Nguồn: blocksandfiles.com 2025-06-19 (MinIO features removal), github.com/minio/minio/issues/21584.*

### ADR-022 — argon2id thay bcrypt

**Status:** Accepted · **Supersedes:** bcrypt trong v1 + CLAUDE.md (cần cập nhật CLAUDE.md)

**Context:** OWASP Password Storage Cheat Sheet hiện hành: **argon2id là khuyến nghị số 1** (minimum m=19456 KiB, t=2, p=1); bcrypt xếp "legacy systems" (và giới hạn input 72 bytes).

**Decision:** argon2id qua `golang.org/x/crypto/argon2`, PHC string format. Migration lazy: verify bcrypt khi login → re-hash argon2id.

### ADR-023 — Refresh rotation + reuse detection + CSRF token

**Status:** Accepted · **Supersedes:** JWT cookie 30 phút đơn lẻ của v1

**Context:** Access token dài (30m) không revoke được; OWASP: SameSite không đủ làm phòng tuyến CSRF duy nhất (hành vi khác nhau giữa browsers).

**Decision:** Access 15m + refresh 14d (cookie path-scoped) với rotation mỗi lần dùng; refresh cũ bị dùng lại → revoke cả family (reuse detection); CSRF signed double-submit cho mọi state-changing request. Lib: golang-jwt/jwt/v5.

*Nguồn: OWASP CSRF Prevention + Password Storage cheat sheets.*

### ADR-024 — Rule sync qua file + reload

**Status:** Accepted · làm rõ điều v1 bỏ ngỏ

**Context:** Prometheus **không có API ghi rule và upstream từ chối thêm**. Các lựa chọn: (a) tự build evaluation engine — re-invent Prometheus, sai hướng; (b) Mimir/Cortex Ruler API — kéo theo cả stack Mimir, quá khổ; (c) Grafana-managed alerting — gắn chặt Grafana; (d) rule files + `POST /-/reload`.

**Decision:** (d): PostgreSQL là source of truth → render YAML → `promtool check rules` → atomic write (temp + rename) → reload → verify qua `/api/v1/rules`. Interface `ports.RuleSyncer` để sau này swap sang PrometheusRule CR (K8s) hoặc Mimir Ruler.

**Consequences:** (+) không re-invent; rule files là artifact rollback được. (−) LogMon backend cần quyền ghi vào volume rules của Prometheus + gọi reload endpoint (network internal); reload là toàn cục — validate trước khi ghi là bắt buộc.

*Nguồn: groups.google.com/g/prometheus-users (no rule write API), grafana.com/docs/mimir ruler API.*

### ADR-025 — Multiwindow multi-burn-rate (Google SRE Workbook)

**Status:** Accepted · chuẩn hóa SLO alerting của v1

**Decision:** Mọi SLO sinh đúng 3 cặp rule: page 2%budget/1h-5m @14.4x; page 5%/6h-30m @6x; ticket 10%/3d-6h @1x. Long AND short window (short = 1/12 long). Recording rules đa window theo pattern Sloth/Pyrra. Không phát minh ngưỡng riêng.

*Nguồn: sre.google/workbook/alerting-on-slos (Table 5-8), github.com/slok/sloth, github.com/pyrra-dev/pyrra.*

### ADR-026 — Meta-monitoring: deadman switch ngoài stack

**Status:** Accepted · lấp lỗ hổng v1 ("ai canh người canh gác")

**Decision:** Alert `Watchdog` (`vector(1)`) luôn firing → route riêng (`repeat_interval: 50s`) → **healthchecks.io** (period 5m, grace 10m). Ping ngừng = chuỗi Prometheus→Alertmanager→internet đứt → healthchecks.io báo qua kênh độc lập. Kèm external uptime check cho các endpoint chính. Lưu ý: Grafana OnCall OSS đã archived 03/2026 — không dùng.

*Nguồn: runbooks.prometheus-operator.dev/runbooks/general/watchdog.*

### ADR-027 — Kafka 4.x KRaft-only

**Status:** Accepted · **Supersedes:** Kafka + Zookeeper của v1

**Context:** Kafka 4.0 (03/2025) **xóa hoàn toàn ZooKeeper**; hiện 4.3. Combined mode (broker+controller cùng node) không được hỗ trợ production.

**Decision:** Kafka 4.3 KRaft: 3 nodes production (RF=3, min.insync.replicas=2); 1 node combined chỉ cho staging. Redpanda ghi nhận là alternative nhẹ (không phải mặc định).

*Nguồn: kafka.apache.org/blog 4.0.0 announcement, docs.confluent.io KRaft config.*

### ADR-028 — redis_rate (GCRA) cho rate limiting

**Status:** Accepted

**Decision:** `go-redis/redis_rate/v10` — GCRA atomic qua 1 Lua script, chính xác hơn fixed window, rẻ hơn sliding window chính xác, hoạt động đúng khi nhiều replica. Không tự viết sliding window. Fail-open khi Redis down (kèm log + metric).

### ADR-029 — Tách demo workload khỏi platform

**Status:** Accepted · **Supersedes:** `order/` BC trong v1

**Context:** v1 đặt `order/` (demo CRUD) ngang hàng các BC platform trong `internal/` — lẫn lộn "sản phẩm" với "dữ liệu mẫu"; `user/` thực chất là auth của platform.

**Decision:** `internal/user/` tiến hóa thành `internal/identity/` (auth + workspaces + RBAC — BC platform thật). Demo service chuyển sang `examples/demo-order/` — instrument đầy đủ, kèm traffic generator, đóng vai trò tài liệu sống về tích hợp (~30 dòng/service) và nguồn telemetry cho dev/test/load test.

### ADR-030 — Modular monolith trước, tách service sau

**Status:** Accepted · làm rõ điều v1 ngầm định ngược

**Context:** v1 vẽ nhiều service (orderservice, userservice...) ngay từ đầu. Team nhỏ + 6 BC + outbox in-process → nhiều process là ops cost không có lợi ích.

**Decision:** Một binary `logmon-api` chứa mọi BC; ranh giới enforce bằng package layout + lint (depguard), KHÔNG bằng network. Outbox relay chạy in-process. Tách service chỉ khi: một BC cần scale độc lập thực sự, hoặc deploy cadence khác nhau gây xung đột. Khi tách: outbox relay đổi sang publish Kafka — domain không đổi.

**Consequences:** (+) 1 deploy unit, transaction xuyên BC dễ (vẫn cấm — dùng events), local dev nhanh. (−) cần kỷ luật giữ boundary; blast radius 1 process (chấp nhận ở quy mô này — restart nhanh, có HA ở GĐ4).
