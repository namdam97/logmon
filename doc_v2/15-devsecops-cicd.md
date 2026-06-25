# 15 — DevSecOps & CI/CD

> Hợp nhất và làm cụ thể phần **pipeline + tự động hóa bảo mật** vốn nằm rải ở [09 §9](09-security.md) (checklist thủ công) và [10 §3](10-deployment-operations.md) (pattern CI/CD). File này KHÔNG lặp lại quy tắc security coding (đã ở [09](09-security.md)) — chỉ mô tả **CI/CD pipeline, security gates tự động, supply-chain, secret rotation, quản lý môi trường**.
>
> **Quy ước trạng thái** (dùng xuyên suốt — vì đây là tài liệu tham chiếu khi triển khai, mỗi mục ghi rõ đang ở đâu):
> - **✅ Đã có** — tồn tại trong repo (kèm đường dẫn).
> - **📐 Đã chốt** — đã quyết trong doc_v2 nhưng **chưa triển khai**.
> - **⬜ Chưa quyết** — khoảng trống thật; **cần ADR trước khi làm**, doc này KHÔNG tự ý chọn.

---

## 1. Nguyên Tắc

- **Shift-left:** kiểm tra (test, lint, scan) chạy trên mọi PR, fail sớm trước khi merge.
- **Fail the build:** gate đã chốt là điều kiện chặn merge/deploy, không phải cảnh báo bỏ qua được.
- **Mọi cấu hình trong git:** pipeline, compose, rule, dashboard — versioned, review qua PR (liên quan [16](16-iac-runbooks.md)).
- **Defense-in-depth:** security ở cả code ([09](09-security.md)) lẫn pipeline (file này); không dựa vào một lớp duy nhất.

---

## 2. Hiện Trạng CI (`.github/workflows/ci.yml`)

Workflow `CI` chạy trên `pull_request` và `push` vào `main`; `concurrency` hủy run cũ cùng ref. Các job:

| Job | Nội dung thực tế | Trạng thái |
|-----|------------------|-----------|
| `backend` | `go build` → `go test -race -covermode=atomic -coverprofile` → in coverage tổng → `golangci-lint` v2.12.2 → `govulncheck ./...` | ✅ |
| `demo-order` | `go build` + `go test -race -cover` + `go vet` cho `examples/demo-order` | ✅ |
| `frontend` | pnpm install (frozen) → `vitest run --passWithNoTests` → `pnpm build` | ✅ |
| `secrets-scan` | `gitleaks/gitleaks-action@v2` (fetch-depth 0) | ✅ |
| `validate-configs` | `promtool check config` + `promtool check rules` (từng file) + `docker compose config -q` | ✅ |
| `build-images` | `needs: [backend, frontend, demo-order]`; build backend + demo-order images; **push ghcr chỉ khi `main`**, tag `${sha}` + `latest`; cache GHA | ✅ |

**Đã chốt nhưng CHƯA có trong `ci.yml`:**
- Coverage gate cứng ≥80% domain/app — `ci.yml` hiện chỉ *in* coverage tổng (comment trong file ghi rõ "gate cứng… sẽ thêm bằng script"). 📐 ([11 §2.4](11-coding-testing-standards.md))
- `pnpm audit` ([09 §9](09-security.md)) — chưa có bước trong job `frontend`. 📐
- `pnpm lint` — `ci.yml` có TODO "thêm khi có eslint config (frontend chưa có)". ⬜
- Integration test (`go test -tags integration`) trên PR vào main — chỉ có target `make test-integration` local, chưa nối CI. 📐 ([11 §2.4](11-coding-testing-standards.md))
- E2E Playwright + pipeline smoke (nightly/staging) — chỉ có `make e2e` / `make ci-local` local, chưa có job CI nightly. 📐 ([11 §2.3-2.4](11-coding-testing-standards.md))
- Frontend image build — `build-images` hiện chỉ build backend + demo-order. ⬜

> **Lưu ý nhất quán:** `ci.yml` tag image `${sha}` + `latest`; [10 §3](10-deployment-operations.md) ghi "tag = sha + semver". Khi thêm release semver cần đồng bộ hai nơi (xem §6).

---

## 3. Pipeline Mục Tiêu (theo [10 §3](10-deployment-operations.md))

Sơ đồ đã chốt trong [10 §3](10-deployment-operations.md). Phần **PR + main build** đã có (§2); phần **deploy** là 📐 (chưa có job trong `ci.yml`):

```
PR  ──▶ backend (test-race + lint + govulncheck)          ✅
    ──▶ frontend (vitest + build)                          ✅
    ──▶ demo-order (build + test + vet)                    ✅
    ──▶ gitleaks                                           ✅
    ──▶ validate-configs (promtool + compose config)       ✅

main ─▶ build & push images (ghcr, tag sha)                ✅
     ─▶ deploy staging (SSH: compose pull + up -d)          📐 chưa có
     ─▶ verify staging (./verify.sh — fail thì dừng)        📐 chưa có (script cũng chưa có — xem 16)
     ─▶ deploy production (GH environment: manual approval) 📐 chưa có
     ─▶ verify prod + watch error rate 10' (Prometheus API) 📐 chưa có
     ─▶ docker system prune (until=168h)                    📐 chưa có
```

Chi tiết môi trường & rollback: §8.

---

## 4. Security Gates Tự Động

| Gate | Phạm vi | Nguồn quyết định | Trạng thái |
|------|---------|------------------|-----------|
| `gitleaks` | Secret scan toàn lịch sử | [09 §9](09-security.md) | ✅ job `secrets-scan` |
| `govulncheck` | CVE trong dependency Go (gọi thật) | [09 §9](09-security.md), [11 §1](11-coding-testing-standards.md) | ✅ job `backend` |
| `golangci-lint` v2 | Lint + một phần static analysis Go | [11 §1](11-coding-testing-standards.md) | ✅ job `backend` |
| `promtool check` | Cú pháp Prometheus config + rules | — (đã có) | ✅ job `validate-configs` |
| `docker compose config` | Cú pháp compose | — (đã có) | ✅ job `validate-configs` |
| `pnpm audit` | CVE trong dependency JS | [09 §9](09-security.md) | 📐 chưa nối CI |
| Quét lỗ hổng **image container** trước push | CVE trong base image + layer | — | ⬜ chưa quyết (ứng viên ecosystem: Trivy/Grype) |
| **SBOM** cho image | Kê khai thành phần (CycloneDX/SPDX) | — | ⬜ chưa quyết |
| **Ký image + provenance** | Chống giả mạo artifact (cosign/SLSA) | — | ⬜ chưa quyết |
| **SAST** chuyên dụng (ngoài linter) | Mẫu lỗ hổng nguồn (gosec/semgrep) | — | ⬜ chưa quyết — cần đánh giá có dư thừa với `golangci-lint` không |
| **DAST** (quét động trên staging) | OWASP-style runtime scan | — | ⬜ chưa quyết |
| Lint Dockerfile | Best-practice image (hadolint) | — | ⬜ chưa quyết |

> Các dòng ⬜ là **khoảng trống thật** — chưa có quyết định trong doc_v2 lẫn repo. Tổng hợp lại ở §12 để chốt qua ADR; doc này không mặc định chọn công cụ.

---

## 5. Quality & Coverage Gates

| Gate | Ngưỡng/quy tắc | Nguồn | Trạng thái |
|------|----------------|-------|-----------|
| `go test -race` | Luôn bật | [11 §2.4](11-coding-testing-standards.md) | ✅ job `backend`/`demo-order` |
| Unit coverage domain+app | **≥ 80%** | [11 §2.4](11-coding-testing-standards.md) | 📐 `ci.yml` mới *in* coverage, chưa enforce |
| Adapters | Coverage qua integration (không ép 80% unit) | [11 §2.4](11-coding-testing-standards.md) | 📐 integration chưa nối CI |
| CI xanh bắt buộc mọi PR | unit + lint + govulncheck + gitleaks | [11 §2.4](11-coding-testing-standards.md) | ✅ (trừ coverage-enforce) |

**Việc cần làm để đóng (📐):** thêm script tính coverage chỉ trên package `domain`/`app` và fail < 80% (comment trong `ci.yml` đã ghi chủ ý này); nối job integration (`go test -tags integration`, cần Postgres service) cho PR vào `main`; thêm job e2e nightly.

---

## 6. Image & Supply-Chain

**✅ Đã có (sự kiện trong repo):**
- [backend/Dockerfile](../backend/Dockerfile): multi-stage `golang:1.26-alpine` → `gcr.io/distroless/static-debian12:nonroot`; `CGO_ENABLED=0`, `-trimpath`, `-ldflags="-s -w"`; `USER nonroot`. Image runtime tối giản, không shell, chạy non-root.
- [examples/demo-order/Dockerfile](../examples/demo-order/Dockerfile) tương tự.
- Push `ghcr.io/<repo>-backend` + `-demo-order`, tag `${sha}` + `latest`, chỉ khi `main` (`ci.yml` job `build-images`).

**Khoảng trống supply-chain (⬜ — chưa quyết, cần ADR §13):**
- **Pin base image theo digest.** Hiện pin theo tag (`golang:1.26-alpine`, `distroless/...:nonroot`). [10 §1](10-deployment-operations.md) nêu "lý tưởng: digest" cho compose — cùng nguyên tắc nên áp cho Dockerfile.
- **Quét image** (CVE) trước khi push.
- **SBOM** sinh + lưu kèm release.
- **Ký image + attestation provenance**; verify chữ ký tại bước deploy.
- **Frontend image** chưa được build/push.
- **Tag release semver** song song `sha` (đồng bộ với [10 §3](10-deployment-operations.md) — xem ghi chú §2).

---

## 7. Secrets Trong Pipeline & Rotation

**✅/📐 Đã chốt (cơ chế trong [09 §5](09-security.md)):**
- App secrets qua env (`.env` KHÔNG commit) hoặc Compose `secrets:` file-based (ưu tiên file cho production). ✅ repo có `infra/docker/secrets/` + `.gitignore` + `*.example` ([16 §5](16-iac-runbooks.md)).
- Notification channel secrets: mã hóa AES-256-GCM với `LOGMON_ENCRYPTION_KEY`, nonce/bản ghi. 📐 (notification BC GĐ3).
- JWT secret ≥32 bytes, có `kid` để rotate không downtime. 📐
- Encryption key versioned (`v1:` prefix) để re-encrypt dần. 📐
- `config.Load()` fail-fast khi thiếu secret bắt buộc. (theo [09 §5](09-security.md))

**Trong CI:** secret duy nhất hiện dùng là `GITHUB_TOKEN` (gitleaks + push ghcr). Deploy qua SSH (📐) sẽ cần SSH key + host secret trong GitHub Secrets — chưa cấu hình.

**Rotation:**
- **Cadence đã chốt:** rotate secrets **hàng quý** ([10 §6](10-deployment-operations.md)).
- **Cơ chế hỗ trợ đã chốt:** JWT `kid`, encryption key versioned ([09 §5](09-security.md)).
- ⬜ **Chưa quyết:** cách *lưu trữ + tự động hóa* rotation cho production (file secrets thủ công vs secret manager). Cần ADR; doc không mặc định chọn.

---

## 8. Môi Trường, Deploy & Rollback (theo [10 §3](10-deployment-operations.md))

| Môi trường | Cách triển khai (đã chốt) | Trạng thái |
|-----------|----------------------------|-----------|
| dev (local) | `make up` / `up-full` / `up-demo` ([Makefile](../Makefile)) | ✅ |
| staging (VPS) | main → SSH `compose pull && up -d` → `verify.sh` gate | 📐 chưa có job/script |
| production (VPS) | GH `environment: production` (manual approval) → deploy → verify + watch error rate 10' → prune | 📐 chưa có |

- **Rollback:** image tag theo `sha` → `BACKEND_TAG=<sha-trước> docker compose pull && up -d` ([10 §3](10-deployment-operations.md)). 📐 (runbook chi tiết: [16 §13](16-iac-runbooks.md)).
- **DB migration:** phải backward-compatible 1 version (quy tắc zero-downtime [08](08-database-schema.md)); migrate chạy qua container one-shot ([16 §6](16-iac-runbooks.md)).

---

## 9. Branch Protection & Repo Hygiene

| Hạng mục | Nguồn | Trạng thái |
|----------|-------|-----------|
| Required status checks trước merge `main` (các job §2) | nguyên tắc §1 | ⬜ chưa quyết/cấu hình (cài ở GitHub settings, không trong repo) |
| Cập nhật dependency tự động (**Renovate**) | [10 §6](10-deployment-operations.md) | 📐 đã chốt, chưa cấu hình (`renovate.json` chưa có) |
| `CODEOWNERS`, required reviewers | — | ⬜ chưa quyết |
| Signed commits | — | ⬜ chưa quyết |

---

## 10. Bề Mặt Tấn Công & Biện Pháp Đã Có (hợp nhất từ [09](09-security.md))

Mục này **chỉ trỏ tới** quyết định đã có ở [09](09-security.md) (không thêm mới), để pipeline biết phải bảo vệ gì:

| Bề mặt | Biện pháp đã chốt | Nguồn |
|--------|-------------------|-------|
| Auth/session | argon2id, access 15m + refresh rotation + reuse detection, CSRF double-submit | [09 §1](09-security.md) |
| AuthZ/tenancy | RBAC middleware + app-layer; mọi query filter `workspace_id`; resource khác workspace → 404 | [09 §2-3](09-security.md) |
| Input/injection | validator/v10; PromQL parse; ES DSL build từ struct; SQL parameterized; SSRF block private IP | [09 §4](09-security.md) |
| Inter-component | ES security ON + TLS; bearer token webhook; Kafka SASL/SCRAM; network `internal` | [09 §6](09-security.md) |
| HTTP | HSTS, nosniff, X-Frame DENY, CSP, TLS ≥1.2 | [09 §7](09-security.md) |
| Audit | Security events log; audit_logs immutable, giữ 2 năm | [09 §8](09-security.md) |

⬜ **Chưa có:** tài liệu **threat model** chính quy (STRIDE/đánh giá rủi ro per surface) và **quy trình ứng phó sự cố bảo mật** (breach protocol, rotate-on-exposure, disclosure). Cần bổ sung — xem §12.

---

## 11. Map Theo Giai Đoạn ([12](12-roadmap.md))

| GĐ | Hạng mục DevSecOps | Nguồn |
|----|--------------------|-------|
| 1 | CI pipeline (test + lint v2 + govulncheck + gitleaks + build images) — **làm ĐẦU TIÊN** | [12 GĐ1.1](12-roadmap.md) ✅ phần lớn |
| 1 | TLS reverse proxy + deploy staging VPS | [12 GĐ1.8](12-roadmap.md) 📐 |
| 2 | Auth nâng cấp (refresh rotation + reuse detection + CSRF) trong code | [12 GĐ2.5](12-roadmap.md) 📐 |
| 2 | E2E pipeline smoke nightly xanh | [12 DoD GĐ2](12-roadmap.md) 📐 |
| 3 | Notification secrets AES-GCM; RBAC; per-workspace rate limit; audit đầy đủ | [12 GĐ3](12-roadmap.md) 📐 |
| 4 | Restore drill pass (gắn backup/DR) | [12 DoD GĐ4](12-roadmap.md) 📐 |

---

## 12. Khoảng Trống Cần Chốt (register — CHƯA quyết)

> Đây là phần trung thực về những gì **chưa có quyết định**. Không triển khai mục nào ở đây cho tới khi có ADR tương ứng.

| # | Khoảng trống | Câu hỏi cần chốt |
|---|--------------|------------------|
| G1 | Quét lỗ hổng image | Công cụ? Ngưỡng fail (CRITICAL/HIGH)? Chặn push hay chỉ cảnh báo? |
| G2 | SBOM | Định dạng (CycloneDX/SPDX)? Lưu ở đâu (release asset/registry)? |
| G3 | Ký image + provenance | Có ký không? Keyless (OIDC) hay key? Verify ở deploy? |
| G4 | SAST chuyên dụng | Có cần ngoài `golangci-lint` v2 không, hay trùng lặp? |
| G5 | DAST | Quét động staging tự động hay pentest định kỳ thủ công? |
| G6 | Secret rotation automation | Giữ file thủ công + cadence quý, hay secret manager? Cơ chế tự động? |
| G7 | Branch protection/hygiene | Required checks, CODEOWNERS, signed commits — bật cái nào? |
| G8 | Threat model + security incident response | Có tài liệu riêng không? Phạm vi? |
| G9 | Coverage-gate + integration/e2e trong CI | Script gate 80% domain/app; service Postgres cho integration; lịch nightly e2e |
| G10 | Lint Dockerfile + pin digest | Có thêm hadolint? Pin base image theo digest? |

---

## 13. ADR Đề Xuất (cần quyết, doc này KHÔNG tự quyết)

Đã chốt & ghi vào [13](13-adr.md) (2026-06-23):
- **ADR-044** (← DS-1): Image scanning **Trivy** + fail CRITICAL/HIGH (G1, **GĐ1 CI**).
- **ADR-045** (← DS-2): SBOM (Syft) + ký image **cosign** (G2/G3, GĐ sau).
- **ADR-046** (← DS-3): SAST **gosec + govulncheck**; DAST GĐ sau (G4/G5).
- **ADR-047** (← DS-4): Secret mgmt **SOPS/Docker secrets** + rotation (G6, prod).
- **ADR-048** (← DS-5): Threat model **STRIDE** + security IR (G8, GĐ sau).

> **Nhất quán:** mọi thay đổi `ci.yml`/deploy lệch file này → cập nhật doc trong cùng PR ([11 §4](11-coding-testing-standards.md)). Khi một mục ⬜ được chốt, chuyển trạng thái và xóa khỏi register §12.
