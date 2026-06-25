# 16 — Infrastructure-as-Code & Runbooks

> Làm cụ thể phần **hạ tầng khai báo (IaC)** và **runbook vận hành** mà [10](10-deployment-operations.md) mới mô tả ở mức nguyên tắc. Phần A: cấu trúc hạ tầng thực tế trong repo + đường lên K8s đã chốt. Phần B: runbook bám **đúng bộ alert thật** trong [infra/prometheus/rules/base-alerts.yml](../infra/prometheus/rules/base-alerts.yml) và quy trình vận hành đã chốt. KHÔNG lặp capacity/backup table của [10](10-deployment-operations.md) — chỉ trỏ tới.
>
> **Quy ước trạng thái** (như [15](15-devsecops-cicd.md)): **✅ Đã có** trong repo · **📐 Đã chốt** trong doc_v2 (chưa triển khai) · **⬜ Chưa quyết** (cần ADR, không tự ý thêm).

---

# Phần A — Infrastructure as Code

## 1. Nguyên Tắc

- **Mọi hạ tầng khai báo trong git** — compose, config (Prometheus/Alertmanager/OTel/ES/Grafana), rule, dashboard, migration. Thao tác tay không track là anti-pattern.
- **Idempotent** — chạy lại an toàn (migrate track `schema_migrations`; `es-init` PUT ghi đè).
- **Tách môi trường** qua override + profile, không sửa file dev cho prod ([10 §1](10-deployment-operations.md)).

## 2. Cấu Trúc `infra/` Hiện Tại (✅ — sự kiện repo)

| Đường dẫn | Vai trò |
|-----------|---------|
| [infra/docker/docker-compose.yml](../infra/docker/docker-compose.yml) | Stack Mode A + profiles `observability`/`demo` (Kafka/Thanos = `scale`, chưa có service) |
| `infra/docker/secrets/` | Secrets file-based: `slack_webhook_url.txt`, `healthchecks_url.txt` + `.gitignore` + `*.example` |
| [infra/prometheus/prometheus.yml](../infra/prometheus/prometheus.yml) | Scrape config |
| [infra/prometheus/rules/base-alerts.yml](../infra/prometheus/rules/base-alerts.yml) | 8 alert nền (Phần B) |
| [infra/alertmanager/alertmanager.yml](../infra/alertmanager/alertmanager.yml) | Routing theo severity + inhibition + deadman |
| `infra/otel/agent.yaml`, `gateway.yaml` | OTel Collector agent→gateway (ADR-018) |
| `infra/elasticsearch/init.sh` | Bootstrap ILM + index template (one-shot, idempotent) |
| `infra/elasticsearch/ilm-policy.json` | ILM: hot (rollover 50gb/7d) → delete 30d |
| `infra/elasticsearch/index-template.json` | Index template `logmon-logs` |
| `infra/grafana/provisioning/`, `dashboards/` | Datasource + 4 dashboard JSON (infrastructure, logs-explorer, service-overview, pipeline-health) |

## 3. Compose: Hiện Trạng vs Mục Tiêu

**✅ Hiện trạng:** một file `docker-compose.yml` (`name: logmon`); service nền (`postgres`, `migrate`, `userservice`) chạy mặc định; observability sau profile `observability`; demo sau profile `demo`. Secrets `slack_webhook_url`, `healthchecks_url` file-based. Volumes named cho mọi data.

**📐 Mục tiêu [10 §1](10-deployment-operations.md) — chưa có:**
- Tách `compose.yaml` (base) + **`compose.prod.yaml`** (override: limits, logging driver, TLS). ⬜ `compose.prod.yaml` chưa tồn tại.
- **Network segmentation:** `backend` (`internal: true`, DB/ES/Kafka không ra internet) + `frontend` (chỉ reverse proxy). ⬜ `docker-compose.yml` hiện **chưa khai báo networks**.
- Profile `scale` (Kafka/Thanos/ES nodes) — service chưa có (GĐ4).

> **Lưu ý nhất quán (cần đồng bộ tên):** [10 §1-3](10-deployment-operations.md) tham chiếu `infra/docker/compose.yaml`; repo dùng `infra/docker/docker-compose.yml`. Khi tách base/prod, thống nhất tên ở cả doc 10 và file thật.

## 4. Config-as-Code & Validate (✅)

Mọi config hạ tầng là file trong git và được **validate trong CI** (job `validate-configs`, [15 §2](15-devsecops-cicd.md)):
- `promtool check config` cho `prometheus.yml`; `promtool check rules` cho từng file `rules/*.yml`.
- `docker compose config -q` cho compose.

Grafana dashboard/datasource provisioned từ git (đổi qua PR, không sửa trên UI runtime) ([10 §4](10-deployment-operations.md)).

## 5. Secrets Layout (✅ — theo [09 §5](09-security.md))

```
infra/docker/secrets/
├── .gitignore                      # chặn commit file .txt thật
├── slack_webhook_url.txt(.example)
└── healthchecks_url.txt(.example)
```
- File `*.txt` thật **không commit**; chỉ `*.example` được track. Bootstrap: `cp *.example *.txt` rồi điền giá trị thật ([10 §2](10-deployment-operations.md)).
- Alertmanager đọc qua `*_file` (`/run/secrets/...`), KHÔNG hardcode ([alertmanager.yml](../infra/alertmanager/alertmanager.yml)).
- Quản lý & rotation secret production: **⬜ chưa quyết** ([15 §7, G6](15-devsecops-cicd.md)).

## 6. Migrations Là Hạ Tầng (✅)

- Service `migrate` (`migrate/migrate:v4.18.1`, **golang-migrate**) chạy one-shot khi `up`, mount `backend/migrations` read-only, idempotent (`schema_migrations`). Cũng dùng cho `make migrate` / `make migrate-down`.
- Migrations hiện có: `000001_init`, `000002_outbox`, `000003_alert_rules` (mỗi cái có `.up.sql`/`.down.sql`).

> **✅ Đã thống nhất (2026-06-23):** toàn bộ doc ([08](08-database-schema.md), [11 §1](11-coding-testing-standards.md), [README](README.md), [12](12-roadmap.md), root README) dùng **golang-migrate** (`migrate/migrate:v4.18.1`, định dạng `NNNNNN_name.up/down.sql`) khớp repo. *(Trước đây 08/11 ghi nhầm "goose".)*

## 7. Bootstrap & Verify ([10 §2](10-deployment-operations.md))

- Bootstrap server Ubuntu LTS: Docker + Compose plugin, reverse proxy, clone repo, tạo secrets, `docker compose up -d`. 📐 (script hóa chưa có).
- **Verify** sau deploy: [10 §2](10-deployment-operations.md) tham chiếu script `infra/scripts/verify.sh` (check `/health`, Prometheus targets, ES data streams, Alertmanager status). ⬜ **`infra/scripts/` chưa tồn tại** — cần tạo (cũng là gate deploy ở [15 §3, §8](15-devsecops-cicd.md)).

## 8. Reverse Proxy & TLS (📐 [10 §2](10-deployment-operations.md), [09 §7](09-security.md))

- TLS termination tại reverse proxy: **Caddy** (khuyến nghị mới — TLS tự động) hoặc Nginx + certbot. Let's Encrypt auto-renew. 📐 — chưa có trong compose; ⬜ chưa chọn dứt khoát giữa Caddy/Nginx.
- Cùng origin cho `/` (frontend) và `/api` (backend) để cookie `SameSite=Strict` hoạt động ([14 §16](14-frontend-architecture.md)).

## 9. Đường Lên Kubernetes (📐 [10 §7](10-deployment-operations.md))

Chỉ chuyển khi cần >1 node / rolling deploy tự động / autoscaling. Mapping **đã chốt**:

| Compose hiện tại | K8s tương đương | Ghi chú |
|------------------|------------------|---------|
| Prometheus + Alertmanager + Grafana | kube-prometheus-stack (helm 86.x) | Rule sync đổi sang sinh `PrometheusRule` CR |
| Elasticsearch | ECK operator 3.4 | |
| Kafka (Mode B) | Strimzi 1.0 | |
| OTel Agent / Gateway | DaemonSet / Deployment+HPA | |
| LogMon API / Frontend | Deployment + Ingress | |

Thiết kế đã chuẩn bị sẵn: `ports.RuleSyncer` swap được sang PrometheusRule CR; collector tách agent/gateway; state ở PG/ES/S3. ⬜ Helm values / manifest cụ thể chưa viết (GĐ4).

---

# Phần B — Runbooks

## 10. Quy Ước `runbook_url` (✅)

Mọi alert có `runbook_url` trỏ tới `https://github.com/namdam97/logmon/wiki/runbooks/<AlertName>` ([base-alerts.yml](../infra/prometheus/rules/base-alerts.yml)). [05](05-alerting-slo.md) quy định `runbook_url` **bắt buộc** cho mọi rule. ⬜ **Nội dung wiki chưa được viết** — §12 dưới đây là nguồn để soạn các trang đó (mỗi alert một trang).

## 11. Khung Runbook Chuẩn

Mỗi runbook gồm: **Trigger** (điều kiện firing) · **Severity & route** · **Ảnh hưởng** · **Chẩn đoán** (lệnh/dashboard cụ thể) · **Khắc phục** · **Xác minh đã hết** · **Leo thang/postmortem** (SEV1/2 → postmortem ≤48h, [10 §6](10-deployment-operations.md)).

## 12. Catalog Runbook — 8 Alert Nền

> Bám **đúng** expr + routing thật. Lệnh chẩn đoán dùng công cụ đã có: `make logs S=<svc>`, `make ps`, dashboard Grafana đang tồn tại (`service-overview`, `pipeline-health`, `infrastructure`), Prometheus `:9090`, Alertmanager `:9093`. Routing theo [alertmanager.yml](../infra/alertmanager/alertmanager.yml): `critical→#alerts-critical`, `warning→#alerts`, `Watchdog→healthchecks.io`.

| Alert | Trigger (expr thật) | Sev / route | Chẩn đoán → Khắc phục |
|-------|---------------------|-------------|------------------------|
| **ServiceDown** | `up{job="logmon-services"}==0` for 1m | critical / page | `make ps` + `make logs S=<svc>`; container restart-loop? OOM (limits [10 §5](10-deployment-operations.md))? DB unreachable? → `docker compose up -d <svc>`; nếu do dependency (postgres) sửa dependency trước. |
| **HighErrorRate** | 5xx / tổng > 0.05 for 2m | critical / page | `service-overview` dashboard → service nào; `logs-explorer` lọc level=error; có deploy gần đây? → rollback ([§13](#13-runbook-vận-hành)) nếu trùng deploy; nếu downstream lỗi, xử lý downstream. |
| **HighLatencyP95** | `histogram_quantile(0.95, …)>1s` for 5m | warning / ticket | `service-overview` p95 panel; DB chậm? (`PGConnHigh` kèm theo?); GC/CPU? → kiểm tra query chậm, kết nối DB, tải. |
| **OutboxLag** | `logmon_outbox_lag_seconds>30` for 5m | warning / ticket | `pipeline-health`; relay có chạy? subscriber lỗi (`logmon_outbox_failed_total`)? → restart service chứa relay; xem event `status='failed'` ([02 §5](02-backend-architecture.md)). |
| **PGConnHigh** | `sum(pg_stat_activity_count)/max_connections>0.8` for 5m | warning / ticket | `infrastructure` dashboard; connection leak? pool size? → tăng `max_connections` hoặc giảm pool/đóng connection rò rỉ. |
| **ESDiskHigh** | `1 - avail/size > 0.8` for 10m | warning / ticket | `pipeline-health` + ES `_cat/allocation`; ILM có chạy? (`ilm-policy.json`: rollover 50gb/7d, delete 30d) → mở rộng disk hoặc giảm retention ([§13](#13-runbook-vận-hành) ILM); ES bật read-only ở 95% — cần clear watermark sau khi dọn. |
| **CollectorQueueFull** | `otelcol_exporter_queue_size/capacity>0.8` for 5m | warning / ticket | ES nhận được không (kèm `ESDiskHigh`?); gateway log → ES reject? → khôi phục ES; queue có persistent volume (`otel-gateway-state`) nên không mất khi gateway restart. |
| **Watchdog** | `vector(1)` (KHÔNG `for:`) | none / deadman | **Cảnh báo ngược:** alert này phải firing LIÊN TỤC; ping ~50s tới healthchecks.io. Nếu **ngừng** → healthchecks.io báo → Prometheus hoặc Alertmanager đã chết (ADR-026). → kiểm tra `make ps prometheus alertmanager`, network, config; KHÔNG thêm `for:` vào rule này. |

## 13. Runbook Vận Hành (quy trình)

Bám quyết định đã chốt; đánh dấu mục phụ thuộc tính năng chưa build.

| Quy trình | Các bước (grounded) | Nguồn / trạng thái |
|-----------|---------------------|--------------------|
| **Deploy** | main → build/push image (sha) → (📐) staging SSH `compose pull && up -d` → `verify.sh` → prod manual approval | [10 §3](10-deployment-operations.md) / 📐 (job + verify.sh chưa có) |
| **Rollback** | `BACKEND_TAG=<sha-trước> docker compose pull && up -d`; DB migration phải backward-compat 1 version | [10 §3](10-deployment-operations.md), [08](08-database-schema.md) / 📐 |
| **Restore drill** | Mỗi quý: dựng môi trường tạm, restore `pg_dump`/ES snapshot, đo RTO; script `restore.sh` | [10 §4, §6](10-deployment-operations.md) / ⬜ `infra/scripts/restore.sh` chưa có |
| **Xem/sửa ILM** | Hiện: `ilm-policy.json` + `es-init` PUT lại; qua UI/API `GET/PUT /pipeline/ilm` | [03](03-logs-pipeline.md), [07 §2.5](07-api-specification.md) / API là GĐ2-3 📐 |
| **DLQ review/retry** | `GET /pipeline/dlq` → review → `POST /pipeline/dlq/retry` | [07 §2.5](07-api-specification.md) / GĐ2-3 📐 |
| **Pipeline mode A↔B** | `POST /pipeline/mode` (orchestrated, confirm); compose profile `scale` | [07 §2.5](07-api-specification.md), [10 §1](10-deployment-operations.md) / GĐ4 📐 |
| **Secret rotation** | Cadence hàng quý; JWT `kid`, encryption key versioned `v1:` | [09 §5](09-security.md), [10 §6](10-deployment-operations.md) / cơ chế 📐, automation ⬜ ([15 §7](15-devsecops-cicd.md)) |

## 14. Vận Hành Định Kỳ ([10 §6](10-deployment-operations.md))

Hàng ngày: review alert đêm + `pipeline-health`. Hàng tuần: SLO compliance + alert noise (auto email GĐ4). Hàng tháng: capacity review + dependency updates (Renovate 📐). Hàng quý: **restore drill** + rotate secrets + review runbook. Sau incident: postmortem ≤48h (SEV1/2) có owner + due date.

---

## 15. Khoảng Trống Cần Bổ Sung (CHƯA có)

| # | Khoảng trống | Ghi chú |
|---|--------------|---------|
| I1 | Nội dung wiki runbook (8 trang) | Nguồn soạn: §12 |
| I2 | `infra/scripts/verify.sh` + `restore.sh` | Được [10](10-deployment-operations.md) tham chiếu, chưa tồn tại; verify.sh là gate deploy [15 §8](15-devsecops-cicd.md) |
| I3 | `compose.prod.yaml` + network segmentation (`backend internal`) | [10 §1](10-deployment-operations.md) 📐 |
| I4 | Reverse proxy + TLS trong compose | ✅ **Nginx** + certbot (ADR-041) |
| I5 | Helm values / K8s manifest | GĐ4 📐 |
| I6 | Thống nhất công cụ migration → **golang-migrate** (08/11/README/12 đã sửa) | ✅ đã thống nhất 2026-06-23 |
| I7 | Thống nhất tên compose → **`docker-compose.yml`** (ADR-040; doc 10 còn vài ref cần sweep) | ✅ chốt 2026-06-23 |
| I8 | (Tùy chọn) Terraform cho VPS/DNS/S3 backup | ⬜ chưa quyết có IaC provisioning hay thủ công |

## 16. ADR Đề Xuất (cần quyết)

Đã chốt & ghi vào [13](13-adr.md) (2026-06-23):
- **ADR-040** (← IAC-1): giữ **`docker-compose.yml`** + network segmentation (I3, I7).
- **ADR-041** (← IAC-2): reverse proxy **Nginx** + TLS certbot (I4).
- **ADR-042** (← IAC-3): provisioning thủ công trước, Terraform GĐ4 (I8).
- **ADR-043** (← IAC-4): migration **golang-migrate** (I6 — đã đồng bộ 08/11).

> **Nhất quán:** thay đổi `infra/*` hay thêm runbook lệch file này → cập nhật doc cùng PR ([11 §4](11-coding-testing-standards.md)). Khi mục ⬜/📐 hoàn tất, chuyển trạng thái và cập nhật register §15.
