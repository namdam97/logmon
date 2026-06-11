# 10 — Deployment & Operations

> Docker Compose vẫn hợp lệ cho production nhỏ năm 2026 — nếu đóng đủ các lỗ hổng vận hành dưới đây. Đường lên Kubernetes đã trải sẵn (mục 7) nhưng **một VPS đơn thì ở lại Compose**.

---

## 1. Docker Compose Production Practices

```yaml
# infra/docker/compose.yaml (trích — pattern chuẩn cho mọi service)
services:
  elasticsearch:
    image: docker.elastic.co/elasticsearch/elasticsearch:9.4.2   # pin minor, KHÔNG :latest
    restart: unless-stopped
    deploy:
      resources:
        limits: { memory: 2g, cpus: "1.0" }    # hoạt động với plain docker compose
    healthcheck:
      test: ["CMD-SHELL", "curl -fsk -u elastic:$$ELASTIC_PASSWORD https://localhost:9200/_cluster/health | grep -qE '(green|yellow)'"]
      interval: 15s
      timeout: 5s
      retries: 5
      start_period: 60s
    volumes: [esdata:/usr/share/elasticsearch/data]
    networks: [backend]
    secrets: [elastic_password]

  otel-gateway:
    image: otel/opentelemetry-collector-contrib:0.154.0
    depends_on:
      elasticsearch: { condition: service_healthy }   # thứ tự khởi động đúng
    networks: [backend]

  kafka-1:
    profiles: [scale]                                  # Mode B only
    image: apache/kafka:4.3.0                          # KRaft-only

networks:
  frontend: {}
  backend: { internal: true }     # DB/ES/Kafka không có đường ra internet

secrets:
  elastic_password: { file: ./secrets/elastic_password.txt }   # KHÔNG commit; .gitignore

volumes:
  esdata: {}
```

Quy tắc bắt buộc:
1. **Pin image** theo minor version (lý tưởng: digest). Nâng version qua PR.
2. Mọi service: `restart: unless-stopped` + `healthcheck` (có `start_period`) + `deploy.resources.limits`.
3. `depends_on.condition: service_healthy` cho chuỗi khởi động (ES → Gateway → backend).
4. Named volumes cho mọi data (ES, Prometheus, Postgres, Kafka, Grafana).
5. Secrets: Compose `secrets:` file-based cho credentials; `env_file` chỉ cho config không nhạy cảm; `.env` không commit.
6. Network segmentation: `backend` internal-only; chỉ reverse proxy + frontend ở `frontend`.
7. Tách `compose.yaml` (base) + `compose.prod.yaml` (override: limits, logging driver, TLS) — không dùng dev config cho prod.
8. Profiles: `scale` (Kafka, Thanos, ES nodes 2-3), `demo` (demo-order workload).

---

## 2. Bootstrap Server (Ubuntu 22.04/24.04 LTS)

```bash
# 1. Docker + Compose plugin
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER && sudo apt install -y docker-compose-plugin

# 2. Reverse proxy — chọn 1:
#    Nginx + certbot (hiện tại) HOẶC Caddy (khuyến nghị mới — TLS tự động, ít moving parts)
sudo apt install -y nginx certbot python3-certbot-nginx     # phương án Nginx

# 3. Clone + secrets
git clone <repo> /opt/logmon && cd /opt/logmon/infra/docker
mkdir -p secrets && <ghi các file secrets>     # elastic_password.txt, jwt_secret.txt, ...

# 4. Khởi động
docker compose up -d                            # Mode A
docker compose --profile scale up -d            # Mode B

# 5. Verify (script hóa: infra/scripts/verify.sh)
curl -f localhost:8080/health                                        # LogMon API
curl -s localhost:9090/api/v1/targets | jq '[.data.activeTargets[] | select(.health!="up")] | length'  # = 0
curl -sk -u elastic:*** https://localhost:9200/_data_stream/logs-* | jq '.data_streams | length'       # > 0
curl -s localhost:9093/api/v2/status | jq .cluster.status            # ready
```

---

## 3. CI/CD (GitHub Actions — pattern giữ nguyên, bổ sung gates)

```
PR → test-backend (go test -race -cover ./... + golangci-lint v2 + govulncheck)
   → test-frontend (pnpm test + build + playwright e2e)
   → gitleaks (secret scan)
main → build & push images (ghcr, tag = sha + semver)
     → deploy staging (SSH: compose pull + up -d)
     → verify staging: healthcheck-gated — ./verify.sh fail thì dừng pipeline
     → deploy production (environment: production — manual approval)
     → verify production + watch error rate 10 phút (query Prometheus API)
     → docker system prune -af --filter "until=168h" (dọn image cũ)
```

Rollback: `BACKEND_TAG=<sha-trước> docker compose pull && docker compose up -d` — image tag theo sha nên rollback = redeploy tag cũ. DB migration phải backward-compatible 1 version (quy tắc zero-downtime ở 08).

---

## 4. Backup & Disaster Recovery

| Component | Phương pháp | Tần suất | Retention | Đích |
|-----------|-------------|----------|-----------|------|
| PostgreSQL | `pg_dump -Fc` + (GĐ4: WAL archiving) | Daily 02:00 | 30 ngày | S3/B2 |
| Elasticsearch | Snapshot API → S3 repository (`logs-*`, `jaeger-*`) | Daily | 30 ngày | S3/B2 |
| Prometheus | Thanos sidecar tự upload (Mode B); Mode A: chấp nhận mất metrics local (RPO 15d data) | 2h auto | 1 năm | S3/B2 |
| Grafana dashboards | JSON trong git (provisioning) | per change | git history | Git |
| Configs (compose, otel, rules) | Git | per change | git history | Git |
| Secrets | Trong password manager / vault của team — KHÔNG trong git | — | — | — |

**Backup phải được TEST**: mỗi quý chạy restore drill vào môi trường tạm (script `infra/scripts/restore.sh`), đo RTO thực tế.

| Scenario | RTO | RPO |
|----------|-----|-----|
| Service crash | < 1m (auto-restart) | 0 |
| Postgres mất data | < 30m | < 24h |
| ES index hỏng | < 1h | < 24h |
| Mất cả server | < 2h (VPS mới + restore) | < 24h |
| Kafka mất (Mode B) | < 30m (recreate topics; agent collector retry/replay từ file position) | logs trong buffer |

---

## 5. Capacity Planning

| Component | Small (Mode A) | Medium (Mode B) | Large (Mode B) |
|-----------|----------------|------------------|----------------|
| LogMon API (Go) | 256 MB | 512 MB | 1 GB |
| PostgreSQL | 1 GB / 20 GB | 4 GB / 100 GB | 8 GB / 500 GB |
| Redis | 256 MB | 1 GB | 4 GB |
| Prometheus | 2 GB / 50 GB | 4 GB / 200 GB | 8 GB / 500 GB |
| Thanos (cả bộ) | — | 2 GB | 4 GB |
| Elasticsearch | 2 GB / 50 GB (1 node) | 12 GB / 600 GB (3 nodes) | 40 GB / 2.5 TB (5 nodes hot/warm) |
| Kafka | — | 3 GB / 50 GB (3 brokers) | 6 GB / 200 GB |
| Jaeger v2 | 512 MB | 1 GB | 2 GB |
| OTel Agent (per host) | 256 MB | 256 MB | 512 MB |
| OTel Gateway | 512 MB | 1 GB | 2 GB |
| Grafana | 256 MB | 512 MB | 1 GB |
| Frontend (Next.js) | 256 MB | 512 MB | 512 MB |
| **TỔNG (≈)** | **~7 GB RAM / 150 GB** | **~30 GB / 1.2 TB** | **~75 GB / 5 TB** |

(So v1: tiết kiệm 1-3.5 GB nhờ bỏ Logstash + Filebeat; thêm ~0.5-1 GB cho OTel agent/gateway.)

Công thức disk (giữ từ v1, vẫn đúng):
```
ES daily disk   = log_GB_per_day × (1 + replicas) × 1.1
Prometheus 15d ≈ series × 2 bytes × (86400/15) × 15 / 10   (ví dụ 50K series ≈ 8.6 GB)
Thanos 1y      ≈ Prom_15d × 24.3 × 0.3 (downsampling)
Kafka          = GB_per_hour × 24h × (1 + RF)
```

JVM heap ES = 50% RAM node, max 32 GB (compressed oops).

---

## 6. Vận Hành Định Kỳ

| Tần suất | Việc | Công cụ |
|----------|------|---------|
| Hàng ngày | Review alerts đêm qua; check `pipeline-health` dashboard | Grafana |
| Hàng tuần | SLO compliance + alert noise report (auto email GĐ4) | LogMon |
| Hàng tháng | Capacity review (disk/RAM trends); dependency updates (renovate) | Grafana + GitHub |
| Hàng quý | **Restore drill**; rotate secrets; review runbooks | scripts |
| Sau incident | Postmortem ≤ 48h (SEV1/2); action items có owner + due date | LogMon |

---

## 7. Đường Lên Kubernetes (GĐ 4+, khi cần HA thật)

Chỉ chuyển khi: cần > 1 node, rolling deploy tự động, autoscaling, hoặc team đã có kinh nghiệm K8s. Stack mapping:

| Compose hiện tại | K8s tương đương | Trạng thái 2026 |
|------------------|------------------|------------------|
| Prometheus + Alertmanager + Grafana | **kube-prometheus-stack** helm (86.x) | Chuẩn de-facto; rule sync đổi sang sinh `PrometheusRule` CR |
| Elasticsearch | **ECK operator** 3.4 | Production-grade, Elastic hỗ trợ chính thức |
| Kafka | **Strimzi 1.0** (GA 04/2026) | Vừa đạt 1.0 sau 7 năm — đủ chín |
| OTel Agent | DaemonSet | Pattern chuẩn |
| OTel Gateway | Deployment + HPA (+ loadbalancing exporter khi >1 replica tail sampling) | |
| LogMon API/Frontend | Deployment + Ingress | |

Thiết kế hiện tại đã chuẩn bị sẵn: `ports.RuleSyncer` swap được sang PrometheusRule CR; collector configs tách agent/gateway; mọi state nằm ở PG/ES/S3.
