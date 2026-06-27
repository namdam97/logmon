# Lộ trình học theo vai trò (Reading Paths)

> Capstone-0 · "Tôi nên đọc bài nào trước?" — 5 lộ trình tinh gọn theo vai trò, đều bám LogMon + doc_v2.

28+ bài là nhiều. Đừng đọc tuần tự từ đầu — chọn **vai trò bạn muốn mạnh lên**, đi theo lộ trình của nó (mỗi bài có mục 🥉🥈🥇 để luyện tay). Tất cả lộ trình đều bắt đầu bằng **SD-1 (3 trụ cột)** vì đó là "vì sao" của cả dự án.

> Ký hiệu: ⭐ = cốt lõi của vai trò · ➕ = nên biết · 🔭 = mở rộng khi rảnh.

## 1. Backend Engineer (Go) — xây dịch vụ & domain

| # | Bài | Vì sao |
|---|-----|--------|
| ⭐ | [BE-1 Go production](../backend-go/01-go-production.md) → [BE-2 Gin](../backend-go/02-gin-http-api.md) → [BE-3 pgx+migrations](../backend-go/03-postgres-pgx-migrations.md) | nền tảng viết service |
| ⭐ | [ARCH-1 Clean Arch](../architecture/01-clean-architecture.md) → [ARCH-2 DDD](../architecture/02-ddd-bounded-contexts.md) → [ARCH-3 CQRS/Outbox](../architecture/03-cqrs-event-driven.md) | cấu trúc code đúng |
| ⭐ | [BE-6 API/OpenAPI](../backend-go/06-api-design-openapi.md) · [TEST-1 Testing](../backend-go/05-testing-strategy.md) | hợp đồng API + test |
| ➕ | [BE-4 Redis](../backend-go/04-redis.md) · [BE-7 Performance/profiling](../backend-go/07-performance-profiling.md) · [SEC-1 Auth](../security/01-appsec-owasp-auth.md) | cache, tối ưu, bảo mật |
| 🔭 | [INC-1](../incident/01-incident-management.md) · [NOT-1](../notification/01-notification-hub.md) | BC nghiệp vụ GĐ3 |

## 2. SRE / Platform / Observability — vận hành & độ tin cậy

| # | Bài | Vì sao |
|---|-----|--------|
| ⭐ | [SD-1 3 trụ cột](../system-design/01-observability-3-pillars.md) | tư duy observability |
| ⭐ | [OBS-1 Metrics](../observability/01-metrics-prometheus.md) → [OBS-4 Grafana/SLO](../observability/04-grafana-slo.md) | đo & SLO/error budget |
| ⭐ | [OBS-2 Logs](../observability/02-logs-otel-elasticsearch.md) · [OBS-3 Traces](../observability/03-traces-opentelemetry-jaeger.md) | 2 trụ còn lại |
| ⭐ | [INC-1 Incident](../incident/01-incident-management.md) · [NOT-1 Notification](../notification/01-notification-hub.md) | MTTA/MTTR, on-call |
| ➕ | [ADV-1 Thanos](../observability/05-thanos-long-term-metrics.md) · [ADV-2 Tail sampling](../observability/06-otel-collector-tail-sampling.md) · [ADV-3 ES/ILM](../data/01-elasticsearch-data-streams-ilm.md) | quy mô lớn |
| 🔭 | [ADV-4 eBPF](../observability/07-ebpf-auto-instrumentation.md) · [AI-1 AI automation](../ai/01-ai-incident-automation.md) | tương lai giảm MTTR |

## 3. Frontend Engineer — admin dashboard

| # | Bài | Vì sao |
|---|-----|--------|
| ⭐ | [FE-1 Next.js + TS](../frontend/01-nextjs-typescript.md) → [FE-2 Tailwind + shadcn](../frontend/02-tailwind-shadcn.md) | toàn bộ FE stack |
| ⭐ | [BE-6 API/OpenAPI](../backend-go/06-api-design-openapi.md) | hợp đồng API để gọi |
| ➕ | [SEC-1 Auth](../security/01-appsec-owasp-auth.md) · [SEC-2 RBAC/multi-tenancy](../security/02-identity-rbac-multitenancy.md) | login, phân quyền UI |
| 🔭 | [SD-1 3 trụ cột](../system-design/01-observability-3-pillars.md) | hiểu dữ liệu mình hiển thị |

## 4. DevOps / DevSecOps / Platform Infra

| # | Bài | Vì sao |
|---|-----|--------|
| ⭐ | [DEPLOY Docker/Compose/Nginx](../devsecops/01-docker-compose-nginx.md) → [K8S-1 K8s Architecture](../kubernetes/01-architecture-components-concepts.md) → [K8S-2 Deploy LogMon](../kubernetes/02-deploying-logmon.md) | đóng gói → cụm → triển khai |
| ⭐ | [DSO CI/CD + supply-chain](../devsecops/02-ci-cd-supply-chain.md) · [DSO-2 Config/12-factor/secrets](../devsecops/04-config-12factor-secrets.md) | pipeline + cấu hình an toàn |
| ➕ | [IAC-1 Terraform/runbooks](../devsecops/03-iac-terraform-runbooks.md) · [SEC-1 Auth](../security/01-appsec-owasp-auth.md) | hạ tầng & secrets |
| 🔭 | [DATA-2 Kafka Mode-B](../data/02-kafka-mode-b-buffer.md) | scale ingest |

## 5. Architect / Tech Lead / Full-stack — bức tranh lớn

Đọc rộng: bắt đầu [SD-1](../system-design/01-observability-3-pillars.md) + cụm **ARCH-1/2/3**, xem [bản đồ Bounded Contexts](../../doc_v2/diagrams/logmon_bc_map.png), rồi lướt mục 1–2 của mọi bài để nắm trade-off. Kết bằng **capstone** [feature end-to-end](01-feature-end-to-end.md) để thấy mọi tầng ráp lại.

---
**Mẹo:** mỗi tuần chọn 1 bài, làm hết task 🥉 + ít nhất 1 task 🥈 *trên repo LogMon thật* (`make up` để có stack chạy). Học bằng tay > đọc suông.
