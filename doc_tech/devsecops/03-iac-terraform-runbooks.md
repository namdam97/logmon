# IaC (Terraform) & SRE Runbooks
> Module IAC-1 · Terraform modules, GitOps, runbook, DR, backup/restore · Độ khó: 🥇 (nâng cao) · Prereqs: K8S-2

> **Trạng thái thực tế (đọc trước):** Phần lớn chủ đề này là **đích đến (planned)**, KHÔNG phải code đang chạy. Hiện trạng IaC của LogMon là **Docker Compose + config-as-code trong `infra/`** (✅ có thật). **Terraform, ArgoCD/GitOps, script `verify.sh`/`restore.sh`, wiki runbook** đều **chưa tồn tại** trong repo — đã chốt là việc của GĐ4 hoặc còn treo (xem `doc_v2/16-iac-runbooks.md §15-16`, ADR-042). Bài này dạy *vì sao* và *làm thế nào* để khi tới lúc đó bạn dựng đúng — mọi neo vào doc_v2 đã ghi rõ `✅`/`📐`/`⬜`.

---

## 1. Vì sao kỹ năng này quan trọng trong LogMon

LogMon là nền tảng observability — chỗ tập trung log, metric, trace của nhiều microservice. Nó có hai đặc thù khiến IaC + runbook là kỹ năng sống còn:

1. **Khi LogMon chết, bạn mất luôn "đèn pin" để soi hệ thống khác.** ServiceDown của chính LogMon là sự cố ưu tiên cao nhất. Vì vậy mọi mảnh hạ tầng phải tái dựng được *xác định, lặp lại, có version* — đúng tinh thần `doc_v2/16 §1`: "Mọi hạ tầng khai báo trong git… thao tác tay không track là anti-pattern."
2. **Stack rất nhiều bộ phận có trạng thái:** Postgres (source of truth của alert rule, user, outbox), Elasticsearch (log + trace), Prometheus (metric). Mất một trong số đó mà không có backup *đã test* = mất dữ liệu thật. `doc_v2/10 §4` đặt RPO 24h / RTO 2h cho kịch bản "mất cả server" — con số đó chỉ đúng nếu restore drill thực sự chạy.

Bốn trụ cột của bài — **Terraform** (provision hạ tầng), **GitOps/ArgoCD** (đồng bộ cluster về đúng git), **runbook SRE** (hành động khi alert nổ), **DR/backup** (cứu dữ liệu) — cùng trả lời một câu: *"Server bốc hơi lúc 3h sáng, bạn dựng lại LogMon trong 2 giờ bằng git + backup, không cần trí nhớ ai cả."* Đó là định nghĩa của hạ tầng trưởng thành.

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Bắt đầu từ vấn đề gốc: hạ tầng là **trạng thái** (servers, DNS, bucket, cluster, app đang chạy). Trạng thái thay đổi theo thời gian, do nhiều người, và *trôi* (drift) khỏi ý định ban đầu. Mọi kỹ thuật dưới đây là cách trị drift.

- **Mô hình mệnh lệnh (imperative)** = bạn gõ từng lệnh (`apt install`, click console). Vấn đề: không ai biết trạng thái *hiện tại* gồm những gì, và lệnh chạy lần hai có thể hỏng. Đây là cách bootstrap LogMon hiện tại (`doc_v2/10 §2`) — chấp nhận được ở quy mô 1 VPS, nhưng không scale.
- **Mô hình khai báo (declarative)** = bạn viết ra *trạng thái mong muốn* trong file, một công cụ lo phần "làm sao đạt được". Đây là nền của cả Compose, Terraform lẫn K8s. Compose của LogMon đã khai báo: bạn viết `services:`, Compose tự reconcile.
- **Idempotent** = chạy file đó N lần cho cùng một kết quả. `migrate` của LogMon idempotent nhờ track `schema_migrations`; `es-init` idempotent vì PUT ghi đè (`doc_v2/16 §1, §6`). Đây là tính chất *bắt buộc* của mọi IaC.
- **State (Terraform)** = Terraform phải nhớ "tôi đã tạo những gì" để biết lần sau cần thêm/sửa/xoá gì. File state đó là source of truth về *ánh xạ giữa code và tài nguyên thật*. Mất state = Terraform "quên" và có thể tạo trùng. Nên state phải ở **remote backend có khoá** (S3 + DynamoDB lock), không để máy cá nhân.
- **Reconciliation loop (GitOps)** = nâng "khai báo" lên một bậc: một agent *trong cluster* liên tục so git ↔ cluster và tự kéo về khớp. Khác CI push (CI gõ `kubectl apply`), GitOps là *pull*: git là nguồn duy nhất, mọi sửa tay bị agent revert. Đây chính là control loop của K8s mà bạn học ở K8S-2, mở rộng ra toàn bộ định nghĩa app.
- **Runbook** = phần con người của control loop. Khi tự động hoá hết khả năng, alert nổ và *người* phải hành động. Runbook biến tri thức trong đầu một người thành quy trình lặp lại được, giảm MTTR và rủi ro thao tác sai (Google SRE).
- **Backup & DR** = thừa nhận mọi thứ trên đều có thể *mất*. Backup là bản sao; DR là *kế hoạch + bài tập* để dùng bản sao đó kịp thời. Backup chưa từng restore thành công = chưa có backup.

Một câu neo: **mệnh lệnh → khai báo → reconcile tự động → runbook cho phần người còn lại → backup cho khi mọi thứ vẫn sập.** Mỗi trụ cột bịt một lớp rủi ro.

## 3. Khái niệm cốt lõi (tăng dần độ khó)

### 3.1 Config-as-code (nền — LogMon đã ở đây)
Mọi file config hạ tầng nằm trong git và được **validate trong CI**. LogMon: `promtool check config/rules` cho Prometheus, `docker compose config -q` cho compose (`doc_v2/16 §4`). Đây là bậc thang đầu tiên của IaC — bạn đã đứng trên nó.

### 3.2 Terraform: provider, resource, state, module
- **Provider** = plugin nói chuyện với một API (AWS, Cloudflare, hetzner…). Khai báo version cố định.
- **Resource** = một vật thể hạ tầng (`aws_s3_bucket`, `cloudflare_record`). Bạn mô tả thuộc tính mong muốn.
- **State** = ánh xạ resource ↔ vật thật, để ở remote backend có lock.
- **Module** = nhóm resource tham số hoá, tái dùng. Cấu trúc chuẩn: `main.tf`, `variables.tf`, `outputs.tf`, `README.md` (HashiCorp/Google).

```hcl
# (planned — minh hoạ) modules/backup-bucket/main.tf
variable "name"           { type = string }
variable "retention_days" { type = number, default = 30 }

resource "aws_s3_bucket" "this" { bucket = var.name }

resource "aws_s3_bucket_lifecycle_configuration" "expire" {
  bucket = aws_s3_bucket.this.id
  rule {
    id     = "expire-${var.retention_days}d"
    status = "Enabled"
    expiration { days = var.retention_days }
  }
}
output "bucket_arn" { value = aws_s3_bucket.this.arn }
```

### 3.3 Dependency inversion trong module
HashiCorp khuyến nghị **module nhận thứ nó cần qua biến đầu vào**, KHÔNG tự dò/tạo. Module `backup-bucket` ở trên không tự tạo IAM user — caller truyền ARN vào. Đổi cách lấy ARN sau này không phải sửa module. Giống đúng nguyên tắc `ports/` ↔ `adapters/` trong CLAUDE.md: phụ thuộc hướng vào abstraction.

### 3.4 Tách môi trường: folder, không phải workspace
Production / staging / dev là *thư mục riêng*, mỗi cái state + backend riêng, gọi lại cùng module. Workspace hợp cho thử nghiệm nhanh; folder-based thắng ở dự án thật vì blast radius rõ ràng (env0, Brainboard).

### 3.5 GitOps & ArgoCD: pull, không push
Một **Application** (CR của ArgoCD) trỏ tới một thư mục manifest trong git. Agent trong cluster reconcile cluster ↔ git. **App-of-apps**: một "root app" trỏ tới repo chứa nhiều Application khác → quản hàng chục app tập trung (adesso, Akuity). Bốn nguyên tắc OpenGitOps: *declarative · versioned & immutable · pulled automatically · continuously reconciled*.

### 3.6 PrometheusRule CR — cầu nối GitOps của LogMon
Đây là điểm GitOps chạm code thật. Hiện tại adapter `promfile.Syncer` (`backend/internal/alerting/adapters/promfile/syncer.go`) implement `ports.RuleSyncer`: render alert rule từ Postgres → file YAML → validate `rulefmt` in-process → ghi atomic → reload Prometheus qua `/-/reload`. Lên K8s, bạn **swap adapter** sang sinh `PrometheusRule` CR cho prometheus-operator (`doc_v2/05 §31`, `doc_v2/16 §9`). Interface `ports.RuleSyncer` không đổi — đây là lý do thiết kế port/adapter được chuẩn bị sẵn cho GitOps.

### 3.7 Runbook & DR
- **Runbook** theo khung chuẩn `doc_v2/16 §11`: Trigger · Severity/route · Ảnh hưởng · Chẩn đoán · Khắc phục · Xác minh · Leo thang.
- **RTO** = thời gian downtime tối đa chấp nhận; **RPO** = lượng dữ liệu tối đa chấp nhận mất. LogMon đặt sẵn bảng RTO/RPO (`doc_v2/10 §4`).
- **3-2-1**: 3 bản sao, 2 loại media, 1 offsite. LogMon: data gốc + backup local + S3/B2 offsite.

## 4. LogMon dùng/sẽ dùng nó thế nào (bám doc_v2 + code; ghi rõ implemented/planned)

| Trụ cột | Hiện trạng LogMon | Nguồn |
|---------|-------------------|-------|
| Config-as-code | **✅ Có** — `infra/` đầy đủ: compose, Prometheus rules, Alertmanager, OTel, ES ILM, 4 Grafana dashboard; validate trong CI (`.github/workflows/ci.yml`) | `doc_v2/16 §2, §4` |
| Secrets file-based | **✅ Có** — `infra/docker/secrets/*.txt` (gitignored) + `*.example`; Alertmanager đọc qua `*_file` | `doc_v2/16 §5` |
| Migrations as infra | **✅ Có** — service `migrate` (golang-migrate v4.18.1), idempotent | `doc_v2/16 §6`, ADR-043 |
| `compose.prod.yml` + network segmentation | **📐 Đã chốt, chưa có file** — compose hiện chưa khai báo `networks:` | `doc_v2/16 §3`, ADR-040 |
| `verify.sh` / `restore.sh` | **⬜ Chưa tồn tại** — `infra/scripts/` chưa có; verify.sh là gate deploy | `doc_v2/16 §7, §15 (I2)` |
| Reverse proxy + TLS | **📐 Nginx + certbot** đã chốt, chưa có trong compose | `doc_v2/16 §8`, ADR-041 |
| Wiki runbook (8 trang) | **⬜ Chưa viết** — `runbook_url` đã trỏ tới wiki nhưng trang trống; nguồn soạn là `§12` | `doc_v2/16 §10, §15 (I1)` |
| GitOps / ArgoCD | **📐 GĐ4** — `ports.RuleSyncer` (✅ có, file-based) sẵn sàng swap sang PrometheusRule CR | `doc_v2/16 §9`, `syncer.go` |
| Helm / K8s manifest | **⬜ Chưa viết** — stack mapping đã chốt (kube-prometheus-stack, ECK 3.4, Strimzi) | `doc_v2/10 §7` |
| Terraform (VPS/DNS/S3) | **📐 GĐ4** — GĐ1-3 provision **thủ công + runbook**, Terraform chỉ khi lên multi-env/K8s | ADR-042 |
| Backup PG/ES + restore drill | **📐 Đã chốt quy trình** — `pg_dump -Fc` + ES snapshot→S3, daily, retention 30d, drill hàng quý; script chưa có | `doc_v2/10 §4, §6` |

Tóm tắt: LogMon đã làm **rất tốt bậc config-as-code**, nhưng các bậc cao hơn (Terraform, GitOps, script DR, wiki runbook) là *đích* — và đã được thiết kế để bước lên không cần refactor lớn (port/adapter, state ở PG/ES/S3).

## 5. Best practices (mỗi mục kèm 1 nguồn đã research)

1. **State ở remote backend có khoá.** Đừng để state Terraform trên máy cá nhân — dùng S3 + DynamoDB lock (hoặc HCP Terraform) để tránh race và mất state. ([HashiCorp recommended practices](https://developer.hashicorp.com/terraform/cloud-docs/recommended-practices))
2. **Module nhỏ + dependency inversion.** Truyền dependency qua input variable thay vì để module tự dò/tạo; module nhỏ giảm blast radius và dễ test. ([Terraform Module Composition](https://developer.hashicorp.com/terraform/language/modules/develop/composition))
3. **Pin version cho mọi thứ "remote".** Provider, module nguồn, helm chart, base image — dùng tag/commit SHA, không HEAD/`:latest`. Khớp quy tắc "pin image theo minor" của LogMon (`doc_v2/10 §1`). ([ArgoCD Best Practices](https://argo-cd.readthedocs.io/en/stable/user-guide/best_practices/))
4. **Tách repo config khỏi repo source (GitOps).** Manifest ở repo riêng → git history sạch, audit rõ, không trigger CI loop, tách quyền dev vs deploy. ([ArgoCD Best Practices](https://argo-cd.readthedocs.io/en/stable/user-guide/best_practices/))
5. **Một alert ⇒ một runbook.** Mỗi rule có `runbook_url`; trang runbook ghi đủ chẩn đoán + khắc phục để giảm MTTR — đúng quy ước LogMon (`doc_v2/05`). ([Google SRE — On-call](https://sre.google/workbook/on-call/))
6. **Backup phải test bằng restore, theo 3-2-1.** Drill định kỳ restore vào môi trường tạm và đo RTO thật; thêm bản immutable/offsite. ([Druva — 3-2-1 Backup Rule](https://www.druva.com/learning-center/glossary/3-2-1-backup-rule))
7. **Đừng track trong git thứ do hệ thống mệnh lệnh điều khiển.** Ví dụ bỏ `replicas` khi dùng HPA, để tránh git ↔ runtime đánh nhau. ([ArgoCD Best Practices](https://argo-cd.readthedocs.io/en/stable/user-guide/best_practices/))

## 6. Lỗi thường gặp & anti-patterns

- **Sửa tay rồi mất đồng bộ với git.** Click console / `kubectl edit` / sửa dashboard Grafana trên UI runtime → drift. LogMon chống bằng provisioning dashboard từ git và đổi qua PR (`doc_v2/16 §4`). Với ArgoCD: bật auto-sync nhưng *dựng exclusion list trước khi bật*, không phải sau.
- **State Terraform commit vào git / để local.** State chứa giá trị nhạy cảm và là single point of failure — luôn remote + lock + mã hoá.
- **`:latest` / HEAD ở mọi nơi.** "Manifest đổi nghĩa mà repo của bạn không hề đổi" (ArgoCD). LogMon pin image theo minor, lý tưởng là digest (`doc_v2/10 §1`).
- **Mega-module / mega-compose.** Một file ôm hết → blast radius lớn, khó test, khó nâng cấp. Tách theo mục đích.
- **Backup chưa từng restore.** Anti-pattern kinh điển: backup chạy xanh mỗi đêm nhưng chưa ai restore thử — đến lúc cần thì hỏng. `doc_v2/10 §4` ghi thẳng "Backup phải được TEST".
- **Runbook là văn xuôi mơ hồ.** "Kiểm tra service" vô dụng lúc 3h sáng. Phải là lệnh cụ thể: `make ps`, `make logs S=<svc>`, panel Grafana cụ thể — đúng như catalog `doc_v2/16 §12`.
- **Thêm `for:` vào Watchdog.** Watchdog phải firing *liên tục* (deadman switch). Thêm `for:` phá cơ chế phát hiện "Prometheus/Alertmanager đã chết" (ADR-026, `doc_v2/16 §12`).
- **Over-engineering sớm.** Dựng Terraform + ArgoCD cho 1 VPS là tốn công vô ích — đúng tinh thần ADR-042: thủ công + runbook trước, Terraform khi *thật sự* lên multi-env/K8s.

## 7. Lộ trình luyện tập (🥉→🥈→🥇)

Vì chủ đề phần lớn **planned**, các task là **thiết kế/POC ngay trong repo LogMon** — vẫn phải tạo file/PR cụ thể, không nói chung chung.

**🥉 Cơ bản — đóng khoảng trống đã biết (I2 trong `doc_v2/16 §15`)**
1. Viết `infra/scripts/verify.sh` đúng 4 lệnh ở `doc_v2/10 §2`: `curl /health`, đếm Prometheus targets không `up` (phải = 0), đếm ES data streams `logs-*` (> 0), check Alertmanager `/api/v2/status`. Trả exit code khác 0 nếu bất kỳ check fail (để làm gate deploy).
2. Soạn **1 trang wiki runbook** cho `ServiceDown` theo khung `doc_v2/16 §11`, lấy nguyên liệu từ dòng `ServiceDown` ở `§12`. Commit dưới dạng markdown trong repo trước (sau đẩy lên wiki).

**🥈 Trung cấp — declarative hoá phần còn treo**
3. Viết `infra/docker/docker-compose.prod.yml` (override): thêm `deploy.resources.limits` theo bảng capacity `doc_v2/10 §5`, logging driver, và khai báo `networks: { frontend: {}, backend: { internal: true } }` cho ADR-040. Verify bằng `docker compose -f docker-compose.yml -f docker-compose.prod.yml config -q`.
4. Viết `infra/scripts/restore.sh` (POC restore drill): dựng Postgres tạm, `pg_restore` từ `pg_dump -Fc` mới nhất, chạy query đếm bảng để xác minh, dọn, in RTO đo được. Map vào bảng RTO/RPO `doc_v2/10 §4`.

**🥇 Nâng cao — POC con đường GĐ4**
5. **POC Terraform module** `modules/backup-bucket/` (S3/B2 + lifecycle 30d theo `doc_v2/10 §4`), state ở remote backend có lock, env `staging`/`prod` là 2 thư mục. Chạy `terraform validate` + `plan`, KHÔNG `apply` lên hạ tầng thật.
6. **POC GitOps:** viết một ArgoCD `Application` (app-of-apps root) + manifest sinh `PrometheusRule` từ `infra/prometheus/rules/base-alerts.yml`. Đối chiếu output với file rule mà `promfile.Syncer` sinh ra — chứng minh `ports.RuleSyncer` swap được mà interface không đổi.
7. Viết ADR đề xuất kích hoạt một trong các mục ⬜ ở `§15`/`§16` (vd Terraform GĐ4), nêu trade-off và tiêu chí "khi nào nên bật".

## 8. Skill/agent ECC nên dùng

- **`ecc:deployment-patterns`** — khi thiết kế `compose.prod.yml`, chiến lược deploy/rollback, pipeline gate (verify.sh). Dùng cho task 🥈#3 và quy trình Deploy/Rollback ở `doc_v2/16 §13`.
- **`ecc:kubernetes-patterns`** — khi POC đường lên K8s (PrometheusRule CR, Deployment/HPA/DaemonSet, ECK/kube-prometheus-stack). Dùng cho task 🥇#6 và `doc_v2/10 §7`. Bổ trợ cho module K8S-2.
- **`ecc:production-audit`** — rà soát "production-readiness": healthcheck, resource limits, secrets, backup đã test, runbook đủ chưa. Dùng để chấm điểm trước khi coi một mục ⬜/📐 là "đã xong" và chuyển trạng thái trong `§15`.
- Phụ trợ: **`ecc:docker-patterns`** (kiện toàn compose), **`ecc:go-review`** (review adapter `RuleSyncer` khi swap), **`/cso`** (security gate trước commit hạ tầng).

## 9. Tài nguyên học thêm (link đã research)

- [Terraform — Module Composition (dependency inversion)](https://developer.hashicorp.com/terraform/language/modules/develop/composition) — HashiCorp official.
- [Terraform — Recommended Practices](https://developer.hashicorp.com/terraform/cloud-docs/recommended-practices) — workflow, remote state, môi trường.
- [Google Cloud — Terraform best practices: style & structure](https://docs.cloud.google.com/docs/terraform/best-practices/general-style-structure) — cấu trúc module chuẩn.
- [ArgoCD — Best Practices](https://argo-cd.readthedocs.io/en/stable/user-guide/best_practices/) — tách repo config, pin version, drift.
- [OpenGitOps — Principles](https://opengitops.dev/) & [Akuity — GitOps Best Practices](https://akuity.io/blog/gitops-best-practices-whitepaper) — 4 nguyên tắc, app-of-apps.
- [Google SRE Workbook — On-call](https://sre.google/workbook/on-call/) & [Incident Response](https://sre.google/workbook/incident-response/) — runbook/playbook, MTTR.
- [Druva — Quy tắc 3-2-1 (2026)](https://www.druva.com/learning-center/glossary/3-2-1-backup-rule) & [Cohesity — RTO vs RPO](https://www.cohesity.com/deep-dives/role-of-rto-rpo-in-disaster-recovery/) — backup & DR.
- Nội bộ: `doc_v2/16-iac-runbooks.md` (source of truth), `doc_v2/10-deployment-operations.md`, `doc_v2/13-adr.md` (ADR-040…043).

## 10. Checklist "đã hiểu"

- [ ] Phân biệt được **imperative vs declarative vs reconcile (GitOps)** và chỉ ra LogMon đang ở bậc nào cho từng phần.
- [ ] Giải thích vì sao **Terraform state** phải ở remote backend có lock, và điều gì xảy ra nếu mất state.
- [ ] Áp dụng **dependency inversion** cho một module (truyền dependency qua biến, không tự dò/tạo) và liên hệ với port/adapter của LogMon.
- [ ] Mô tả **app-of-apps** và 4 nguyên tắc OpenGitOps; chỉ ra `ports.RuleSyncer` swap sang PrometheusRule CR thế nào.
- [ ] Viết được **runbook** theo khung 7 phần và biết tại sao Watchdog không được thêm `for:`.
- [ ] Phân biệt **RTO vs RPO**, đọc đúng bảng `doc_v2/10 §4`, và giải thích tại sao "backup chưa restore = chưa có backup".
- [ ] Liệt kê đúng **3 trạng thái** (✅/📐/⬜) cho từng trụ cột IaC của LogMon và nguồn doc_v2 tương ứng.
- [ ] Biết khi nào **KHÔNG** nên dùng Terraform/ArgoCD (1 VPS — ADR-042) để tránh over-engineering.
