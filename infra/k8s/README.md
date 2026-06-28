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

## Quy trình (thứ tự bring-up đầy đủ)

```bash
make k8s-up          # C0: tạo cluster k3d + namespaces (logmon, observability)
make k8s-app         # C1: PG/Redis/migrate/userservice/frontend/Ingress
make k8s-monitoring  # C2: kube-prometheus-stack + ServiceMonitor
                     # C3: RuleSyncer→PrometheusRule CR đã bật sẵn (RULE_SYNC_MODE=k8s)
make k8s-eck         # C4: ECK operator + Elasticsearch CR (đồng bộ pass → userservice)
make k8s-kafka       # C5: Strimzi + Kafka CR (KRaft) + topics
make k8s-otel        # C6: OTel logs pipeline (agent→gateway→Kafka→consumer→ES)
make k8s-thanos      # C7: SeaweedFS (S3) + Thanos sidecar/store/query/compactor
make k8s-ps          # trạng thái pods cả 2 namespace
make k8s-down        # xoá cluster (mất PVC; `k3d cluster stop logmon` để giữ data)
```

> **Thứ tự quan trọng:** C2 trước C7 (Thanos sidecar bật qua helm upgrade kube-prom);
> C4+C5 trước C6 (OTel consumer cần ES + Kafka). C3 không cần lệnh riêng — bật bằng
> env trong C1 (userservice apply PrometheusRule khi rule CRUD).

## Verify nhanh

```bash
# App qua Ingress (thêm 127.0.0.1 logmon.local vào /etc/hosts hoặc -H Host:)
curl -H 'Host: logmon.local' http://127.0.0.1:8088/login              # 200
# Grafana
curl -H 'Host: logmon.local' http://127.0.0.1:8088/grafana/login      # 200
# Thanos query UI
kubectl -n observability port-forward svc/thanos-query 10902:10902    # → :10902
# Kafka consumer group (logs pipeline)
kubectl exec -n observability logmon-combined-0 -- \
  bin/kafka-consumer-groups.sh --bootstrap-server localhost:9092 --describe --group otel-gateway
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
