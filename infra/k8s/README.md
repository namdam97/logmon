# LogMon trên Kubernetes (local k3d) — Phase III

Production-like single-node cluster để học/verify bộ công cụ K8s thật ở quy mô tí
hon, **$0**. Map theo `doc_v2/10 §7`:

| Compose (Mode A/B)            | K8s tương đương                                  |
|-------------------------------|--------------------------------------------------|
| Prometheus/Alertmanager/Grafana | **kube-prometheus-stack** (Helm)               |
| Elasticsearch                 | **ECK operator** + `Elasticsearch` CR            |
| Kafka                         | **Strimzi** + `Kafka` CR (KRaft)                 |
| Thanos                        | sidecar (kube-prom) + store/query/compactor      |
| OTel agent / gateway          | DaemonSet / Deployment                            |
| LogMon API + Frontend         | Deployment + Service + **Ingress** (Traefik)     |
| RuleSyncer (file reload)      | sinh **`PrometheusRule` CR** (ADR-024)           |

## Vì sao k3d?

`k3s` cần root (systemd). Máy dev này **sudo bị khoá** → dùng **k3d** (k3s chạy
trong Docker, không cần sudo). Cùng k3s v1.31.x, cùng Traefik + local-path SC →
manifests tái dùng nguyên vẹn khi lên VPS k3s/k8s thật (Phase IV).

## Quy trình

```bash
make k8s-up      # tạo cluster + namespaces (logmon, observability)
make k8s-app     # C1: PG/Redis/migrate/userservice/frontend/Ingress
# … (C2..C7 thêm dần)
make k8s-ps      # trạng thái pods
make k8s-down    # xoá cluster (mất PVC)
```

Ingress publish loopback: http://127.0.0.1:8088 · https://127.0.0.1:8443
(Host header `logmon.local` — thêm vào /etc/hosts hoặc `curl -H 'Host: logmon.local'`).

## Bố cục

```
infra/k8s/
  bootstrap.sh        # tạo cluster k3d (idempotent)
  teardown.sh         # xoá cluster
  base/namespace.yaml # namespaces
  app/                # C1: core app + stateful deps
  observability/      # C2+: kube-prom, ECK, Strimzi, Thanos, OTel
```
