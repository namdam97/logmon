# 09 — Security

> Cập nhật lớn so v1: **argon2id thay bcrypt** (ADR-022), **refresh token rotation + reuse detection + CSRF token** (ADR-023), bảo mật giữa các component (ES security on-by-default), mã hóa secrets at-rest.

---

## 1. Authentication

### 1.1 Password hashing — argon2id (ADR-022)

OWASP hiện hành khuyến nghị argon2id; bcrypt chỉ cho hệ legacy.

```go
import "golang.org/x/crypto/argon2"

// Tham số OWASP minimum: m=19456 KiB (19 MiB), t=2, p=1
hash := argon2.IDKey([]byte(password), salt, 2, 19456, 1, 32)
// Lưu PHC string: $argon2id$v=19$m=19456,t=2,p=1$<salt-b64>$<hash-b64>
```

- Salt 16 bytes từ `crypto/rand`. So sánh bằng `subtle.ConstantTimeCompare`.
- **Migration từ bcrypt** (code hiện tại): verify bằng thuật toán cũ khi login thành công → re-hash argon2id → update. Không cần reset password hàng loạt.
- Login lỗi → message generic "invalid credentials" (không phân biệt email/password sai); rate limit 10 req/min/IP; log security event cả success lẫn failure.

### 1.2 Session — Access + Refresh rotation (ADR-023)

```
Login OK ──▶ Access JWT (15 phút)   → cookie lm_access  (HttpOnly, Secure, SameSite=Strict, Path=/)
         ──▶ Refresh token (14 ngày) → cookie lm_refresh (HttpOnly, Secure, SameSite=Strict,
                                                          Path=/api/v1/auth)  ← scope hẹp
         ──▶ CSRF token              → cookie lm_csrf (KHÔNG HttpOnly — JS đọc để gắn header)
```

| Quy tắc | Chi tiết |
|---------|----------|
| JWT lib | `golang-jwt/jwt/v5`; alg pin HS256/EdDSA, **từ chối `alg=none`**; validate `iss`, `aud`, `exp` |
| Access claims | `sub` (user id), `iss`, `aud`, `exp` (15m), `iat`. KHÔNG nhét role vào token ở GĐ3 (role đổi phải có hiệu lực ngay — đọc từ DB/cache) |
| Refresh rotation | Mỗi lần `/auth/refresh`: token cũ đánh dấu `used_at`, phát token mới cùng `family_id` |
| **Reuse detection** | Refresh token có `used_at != NULL` bị dùng lại → **revoke toàn bộ family** + log security event + (GĐ3) notify user |
| Lưu trữ | DB chỉ lưu SHA-256 của refresh token, không plaintext |
| Logout | Revoke family + clear cả 3 cookies |
| JWT secret | ≥ 32 bytes random từ secrets manager/env; có `kid` để rotate key không downtime |

### 1.3 CSRF — double-submit cookie

SameSite=Strict giúp nhiều nhưng **không đủ làm phòng tuyến duy nhất** (OWASP — hành vi khác nhau giữa browsers). Áp dụng defense-in-depth:

```
Login → server phát lm_csrf = HMAC(session_id, csrf_secret)
Mọi POST/PUT/DELETE → client gửi header X-CSRF-Token = giá trị cookie lm_csrf
Middleware csrf: so khớp header vs cookie (signed double-submit) → lệch = 403
Ngoại lệ: /auth/login, /auth/register, /alerts/webhook (internal token riêng)
```

---

## 2. Authorization (RBAC — GĐ 3)

| Role | Quyền |
|------|-------|
| viewer | Read mọi resource trong workspace, log search |
| editor | + CRUD rules/SLO/incidents, ack/silence, timeline |
| admin | + members, notification channels, pipeline mode/ILM, oncall schedule |
| platform_admin | Mọi workspace + tạo/xóa workspace |

- Check tại middleware `rbac` (route-level min role) **và** tại app layer cho thao tác nhạy cảm (defense in depth).
- Mọi query repository **bắt buộc** có `workspace_id` filter — constructor repo nhận workspace từ context, không có đường đi nào bỏ qua. Resource khác workspace trả **404** (không phải 403 — tránh lộ tồn tại).

---

## 3. Multi-Tenancy Isolation (theo từng store)

| Store | Cơ chế | Ghi chú |
|-------|--------|---------|
| PostgreSQL | `workspace_id` column + composite index; repo enforced filter | Cân nhắc RLS (Row-Level Security) của Postgres làm tầng chặn thứ 2 ở GĐ4 |
| Elasticsearch | **Data-stream-per-workspace** (`logs-{service}-{workspace_slug}`); backend chỉ query đúng namespace | KHÔNG để client tự truyền index pattern |
| Prometheus | Label `workspace` trên metrics | ⚠️ Label tự khai từ service — **không phải security boundary thật**. Backend inject matcher `{workspace="..."}` vào mọi PromQL từ user (pattern prom-label-proxy). Tenancy cứng cần Mimir/Thanos Receive — để GĐ4+ |
| Jaeger | Tag `workspace` + backend filter | Tương tự Prometheus |
| Redis | Key prefix `ws:{id}:` | |

---

## 4. Input Validation & Injection

- `go-playground/validator/v10` trên mọi request struct; fail → 400 generic, chi tiết log internally.
- **PromQL từ user** (alert rules, SLO): parse bằng `promql/parser` trước khi lưu; chặn query quá nặng khi proxy (timeout 30s, giới hạn range).
- **ES query từ user** (log search): backend build query DSL từ struct — **không bao giờ** forward JSON DSL thô từ client; mọi search inject filter workspace.
- SQL: parameterized 100% (`$1, $2` pgx). Cấm string concatenation.
- SSRF (generic webhook channel — GĐ3): validate URL scheme https, **chặn private IP ranges** (10/8, 172.16/12, 192.168/16, 169.254/16, localhost) khi resolve; re-validate tại thời điểm gửi (chống DNS rebinding).

---

## 5. Secrets Management

| Loại | Cách xử lý |
|------|-----------|
| App secrets (DB password, JWT key, internal tokens) | Env vars từ `.env` (KHÔNG commit) hoặc Docker Compose `secrets:` (file-based, không cần Swarm) — ưu tiên secrets file cho production |
| Notification channel secrets (webhook URLs, PagerDuty keys) | Lưu DB nhưng **mã hóa AES-256-GCM** với `LOGMON_ENCRYPTION_KEY` từ env; nonce riêng mỗi bản ghi |
| Token generation | `crypto/rand` — CẤM `math/rand` |
| Startup validation | `config.Load()` fail-fast khi thiếu secret bắt buộc |
| Rotation | JWT key có `kid`; encryption key versioned (`v1:...` prefix) để re-encrypt dần |

---

## 6. Bảo Mật Giữa Các Component (mới trong v2)

| Kết nối | Bảo vệ |
|---------|--------|
| ES ← Gateway/Backend/Grafana/Jaeger | ES 9.x security mặc định BẬT: basic auth (user riêng per client, least privilege) + TLS. **Không tắt** `xpack.security` như thói quen dev cũ |
| Alertmanager → LogMon webhook | Bearer token nội bộ (env), network internal-only |
| Prometheus reload endpoint | `--web.enable-lifecycle` nhưng port chỉ expose trong network internal; không qua reverse proxy |
| Kafka (Mode B) | SASL/SCRAM + network `internal: true`; GĐ4 cân nhắc mTLS |
| OTel Agent → Gateway | Cùng host/network nội bộ: plaintext chấp nhận được; multi-host: mTLS |
| Docker networks | Tách `frontend` (proxy↔app) / `backend` (app↔data, `internal: true`); chỉ reverse proxy publish port ra ngoài |

---

## 7. HTTP Hardening (giữ từ v1 + bổ sung)

```go
c.Header("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
c.Header("X-Content-Type-Options", "nosniff")
c.Header("X-Frame-Options", "DENY")                  // Grafana embed dùng route riêng nếu cần
c.Header("Content-Security-Policy", "default-src 'self'")  // frontend tune theo nhu cầu
c.Header("Referrer-Policy", "no-referrer")
```

- TLS: MinVersion 1.2 (ưu tiên 1.3), `InsecureSkipVerify: false` LUÔN LUÔN. TLS termination tại reverse proxy (Let's Encrypt auto-renew).
- Anti-patterns cấm tuyệt đối (giữ từ v1): `unsafe` package, `text/template` cho HTML, ignore errors `_ :=`, `log.Fatal` trong handlers.

---

## 8. Security Logging & Audit

**Bắt buộc log** (security events): auth attempts (success + failure), authorization failures, validation failures, admin actions (rule/SLO/channel/member changes — vào cả `audit_logs`), TLS failures, refresh-token reuse detection.

**Cấm log**: passwords, tokens (access/refresh/CSRF), connection strings, encryption keys, PII, request/response bodies, full stack trace ở production.

Audit log: immutable (không UPDATE/DELETE), giữ 2 năm (archive ra S3 sau 90 ngày — GĐ4).

---

## 9. Checklist Trước Mỗi Release

- [ ] Không hardcoded secrets (gitleaks scan trong CI)
- [ ] Input validation đủ trên endpoints mới
- [ ] Queries parameterized; ES query build từ struct
- [ ] AuthN/AuthZ + workspace filter trên route mới
- [ ] Rate limiting áp dụng
- [ ] Error responses không lộ internals
- [ ] Dependency scan (`govulncheck`, `pnpm audit`) sạch CRITICAL/HIGH
- [ ] Security events mới được log đúng quy tắc mục 8
