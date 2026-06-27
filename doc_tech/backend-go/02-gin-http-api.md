# REST API với Gin trong LogMon

> Module BE-2 · router, middleware chain, validator/v10, response envelope · Độ khó: 🥉→🥇 · Prereqs: BE-1

## 1. Vì sao kỹ năng này quan trọng trong LogMon

LogMon là nền tảng observability cho Go microservices. Mọi thứ người dùng và service khác chạm vào — đăng nhập, tạo alert rule, query log, nhận webhook từ Alertmanager — đều đi qua một lớp HTTP API duy nhất do `userservice` phục vụ (xem `backend/cmd/userservice/main.go`). Lớp này là **biên hệ thống**: nơi dữ liệu chưa tin cậy từ ngoài đi vào, nơi mọi quyết định bảo mật (auth, CSRF, rate limit) được thực thi, và nơi mọi lỗi nội bộ phải được "làm sạch" trước khi trả ra ngoài.

Gin là HTTP framework LogMon chọn (CLAUDE.md, Tech Stack). Nếu bạn không nắm chắc cách Gin ráp router, xâu chuỗi middleware, validate input và đóng gói response, bạn sẽ vô tình leak chi tiết lỗi nội bộ, mở lỗ hổng IDOR/CSRF, hoặc tạo metric high-cardinality làm sập Prometheus. Module này dạy bạn làm đúng cả bốn việc đó theo đúng cách LogMon đã làm.

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Một HTTP request là một thông điệp text: dòng đầu `POST /api/v1/users HTTP/1.1`, vài header (`Content-Type`, `Cookie`, ...), một body (thường JSON). Server đọc thông điệp đó, làm gì đó, rồi trả về một thông điệp khác: status code (200, 400, 500), header, body.

Việc của một web framework như Gin chỉ gồm 3 phần:

1. **Routing** — nhìn vào `method + path` để chọn đúng hàm xử lý (handler). `POST /users` → hàm `register`, `GET /users/:id` → hàm `get`.
2. **Middleware chain** — trước/sau handler, chạy một chuỗi hàm dùng chung: log, đo latency, kiểm tra auth... Mỗi middleware nhận `c *gin.Context`, làm việc của nó, rồi gọi `c.Next()` để nhường cho mắt xích kế tiếp. Đây là pattern **onion** (vỏ hành): request đi vào qua từng lớp, response đi ra ngược lại qua đúng các lớp đó.
3. **Context** — `*gin.Context` là "cặp tài liệu" đi kèm suốt vòng đời request: chứa `Request`, cho phép đọc param/cookie/body, ghi response, và mang giá trị giữa các middleware (vd `c.Set("auth_user_id", ...)`).

Ba nguyên tắc nền mà LogMon áp lên ba phần trên:

- **Không tin input** — mọi field từ client phải validate trước khi dùng (CLAUDE.md §Security).
- **Một format trả về duy nhất** — mọi response (thành công hay lỗi) cùng một "phong bì" (envelope) để frontend xử lý nhất quán.
- **Lỗi ra ngoài phải generic** — user nhận message chung chung; chi tiết log nội bộ kèm `trace_id`.

## 3. Khái niệm cốt lõi (tăng dần độ khó)

### 3.1 Engine, RouterGroup và route

`gin.New()` tạo một `*gin.Engine` (router rỗng, không kèm middleware mặc định — khác `gin.Default()`). Bạn nhóm route bằng `RouterGroup` để chia sẻ prefix + middleware:

```go
r := gin.New()
api := r.Group("/api/v1")          // mọi route dưới đây có prefix /api/v1
api.POST("/users", h.register)     // → POST /api/v1/users
api.GET("/users/:id", h.get)       // :id là path param, đọc bằng c.Param("id")
```

### 3.2 Middleware và `c.Next()`

Middleware là `func(c *gin.Context)`. Code **trước** `c.Next()` chạy lúc request đi vào; code **sau** chạy lúc response đi ra:

```go
func Metrics(m *metrics.Metrics) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        c.Next()                       // chạy các handler phía sau
        m.ObserveRequest(/* ... */, time.Since(start)) // đo sau khi xong
    }
}
```

`c.Abort()` (hoặc `c.AbortWithStatusJSON`) dừng chuỗi — handler phía sau **không** chạy. Đây là cách middleware auth chặn request: trả 401 rồi abort.

### 3.3 Binding vs Validation — hai bước khác nhau

Đây là điểm dễ nhầm. LogMon tách rõ làm hai bước:

| Bước | Làm gì | API Gin/lib | Lỗi → |
|------|--------|-------------|-------|
| **Binding** | Parse JSON body vào struct (đúng kiểu dữ liệu chưa?) | `c.ShouldBindJSON(&req)` | 400 "invalid request body" |
| **Validation** | Áp luật nghiệp vụ trên struct (email hợp lệ? password đủ dài?) | `validator/v10` qua tag `validate:"..."` | 400 "invalid request fields" |

Gin **có** validator tích hợp (qua tag `binding:"..."`), nhưng LogMon cố ý gọi `validator/v10` **tường minh** thành bước riêng (`h.validate.Struct(req)`). Lý do: tách lỗi parse khỏi lỗi nghiệp vụ, và CLAUDE.md §Security yêu cầu dùng `go-playground/validator/v10` làm chuẩn validate ở mọi biên.

```go
type registerRequest struct {
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=8,max=72"`
}
```

### 3.4 Response envelope

Thay vì mỗi handler tự `c.JSON(...)` một hình dạng khác nhau, LogMon gói mọi response trong một struct `Envelope` (xem §4). Frontend luôn đọc `success` rồi rẽ nhánh `data` hay `error`.

### 3.5 Error mapping (domain error → HTTP status)

Tầng `app/domain` trả về **error nghiệp vụ** (vd `ErrUserNotFound`), không biết gì về HTTP. Tầng adapter HTTP có một hàm `failDomain` dùng `errors.Is` để dịch error đó sang status code đúng (404/409/401...) và một message generic. Nguyên tắc CLAUDE.md: "handle once" — log HOẶC return, không cả hai; và không leak raw error.

## 4. LogMon dùng nó thế nào (bám code thật — path:line, ghi rõ implemented/planned)

![Gin request lifecycle](../diagrams/gin-request-lifecycle.png)

Tất cả dưới đây **đã implemented** trừ chỗ ghi rõ "(planned)".

**Composition root — ráp router.** `backend/cmd/userservice/main.go:347-358` dựng engine và đăng ký global middleware chain theo đúng thứ tự onion:

```
Recovery → otelgin → TraceID → CORS → SecurityHeaders → Metrics → Logging
```

`Recovery` đứng ngoài cùng để bắt panic của mọi lớp trong; `otelgin` tạo span sớm để các lớp sau nằm trong trace. Route `/healthz` (`main.go:360`) và `/metrics` (`main.go:369`) đăng ký ngoài group `/api/v1`.

**Middleware dùng chung** ở `backend/internal/shared/middleware/middleware.go`:
- `TraceID()` (`:30`) — gắn `X-Trace-Id` cho mọi response; nếu đã có span W3C thì dùng trace_id của span, ngược lại sinh bằng `crypto/rand` (`:122`).
- `Recovery()` (`:79`) — `gin.CustomRecovery`, panic → log + trả 500 generic, không crash service.
- `SecurityHeaders()` (`:110`) — set `X-Content-Type-Options`, `X-Frame-Options: DENY`, HSTS, CSP `default-src 'none'`.
- `CORS()` (`:91`) — chỉ echo đúng một origin kèm credentials (không bao giờ `*` + credentials).
- `Metrics()` (`:66`) — đo theo **route template** `c.FullPath()` (vd `/users/:id`), tránh high-cardinality; fallback `"unmatched"`.

**Rate limit** ở `backend/internal/shared/middleware/ratelimit.go`: token bucket theo IP (`golang.org/x/time/rate`), vượt ngưỡng → 429 (`:53`). `main.go:386-387` áp limiter này riêng cho register/login/refresh. Lưu ý chính code tự ghi (`ratelimit.go:14`): map client chưa có evict/TTL → chấp nhận cho skeleton, **production cần store tập trung như Redis (planned)**.

**Auth & CSRF & Bearer** ở `backend/internal/shared/auth/`:
- `RequireAuth()` (`middleware.go:21`) — đọc access token từ cookie `logmon_token`, parse JWT, set `auth_user_id` vào context.
- `CSRFProtector.Middleware()` (`csrf.go:73`) — signed double-submit: bỏ qua safe method (GET/HEAD), so cookie `lm_csrf` với header `X-CSRF-Token` bằng `subtle.ConstantTimeCompare` + verify HMAC. `main.go:375-380` miễn trừ login/register/refresh/webhook.
- `RequireBearerToken()` (`bearer.go:16`) — bảo vệ webhook Alertmanager, fail-closed khi token rỗng.

**Envelope** ở `backend/internal/shared/httpx/response.go`:

```go
type Envelope struct {
    Success bool   `json:"success"`
    Data    any    `json:"data"`
    Error   string `json:"error,omitempty"`
    Meta    *Meta  `json:"meta,omitempty"`
}
func OK(c, status, data)   // success=true
func Fail(c, status, msg)  // success=false, message generic
func FailFromError(c, err) // map sentinel error (errors.go) → status (:41)
```

**Handler thật.** `backend/internal/user/adapters/http/handler.go`:
- `register` (`:127`) — `ShouldBindJSON` → `validate.Struct` → gọi use case → `httpx.OK(201)`.
- `get` (`:220`) — kiểm tra `authUserID == c.Param("id")` để **chống IDOR** (chỉ đọc chính mình); thiếu role admin nên trả 403.
- `failDomain` (`:252`) — `errors.Is` map `ErrUserNotFound→404`, `ErrEmailTaken→409`, `ErrInvalidCredentials→401`, `ValidationError→400`, còn lại 500.

Pattern này lặp lại đồng nhất ở `backend/internal/alerting/adapters/http/handler.go` (CRUD alert rule, `validate:"oneof=critical warning info"` ở `:92`) và ở read API `backend/internal/logpipeline/adapters/http/handler.go` — đáng chú ý handler log dùng **query param** thủ công (`parseSearchInput` `:59`, `parseTimeParam` `:94`, `parseIntParam` `:107`) thay vì bind JSON, vì là `GET /logs` với filter (chỉ bật khi `ELASTICSEARCH_URL` có giá trị — `main.go:286`).

**Khác biệt implemented vs doc_v2 (planned/target).** `doc_v2/07-api-specification.md` §1.1 mô tả envelope mục tiêu `{data, error:{code,message}, meta:{per_page,...}}` và cookie tên `lm_access`. Code hiện tại dùng envelope `{success, data, error(string)}` và cookie `logmon_token`. Đây là khác biệt skeleton-vs-target có chủ đích; khi triển khai theo doc_v2 cần align lại. Về HTTP surface: chỉ ba BC `user`, `alerting`, `logpipeline` có handler (`internal/*/adapters/http/`). BC `slo` mới có lớp `domain/` (chưa có `app`/`adapters`/HTTP handler); `incident` và `notification` **chưa có code** — tất cả là **planned (GĐ3)** về mặt API. Multi-tenancy đầy đủ cũng planned GĐ3: hiện code dùng hằng `_defaultWorkspaceID` (`main.go:60-61`), chưa có middleware parse header `X-Workspace-ID`.

## 5. Best practices (mỗi mục kèm 1 nguồn đã research)

- **Tách binding khỏi validation, gọi validator tường minh.** Gin suy luận binder theo `Content-Type`; hãy dùng `ShouldBindJSON` rồi validate riêng để kiểm soát thông điệp lỗi ([Gin — Model binding and validation](https://gin-gonic.com/en/docs/binding/binding-and-validation/)).
- **Cân nhắc error-handling middleware tập trung.** Handler `c.Error(err)`, một middleware sau `c.Next()` đọc `c.Errors.Last()` để trả format đồng nhất — giảm lặp code. LogMon hiện đạt sự đồng nhất bằng `httpx` + `failDomain` thay vì `c.Errors`; cả hai đều hợp lệ ([Gin — Error handling middleware](https://gin-gonic.com/en/docs/middleware/error-handling-middleware/)).
- **Phân biệt 400 (parse fail) và 422 (field invalid); cân nhắc RFC 9457 Problem Details cho error body.** Chuẩn `application/problem+json` với `type/title/status/detail/instance` là hướng đi hiện đại cho error envelope ([RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html)).
- **Giới hạn kích thước request, validate server-side mọi tham số phân trang, trả 429 khi quá quota kèm `Retry-After`.** ([OWASP REST Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html)).
- **Bật security header và TLS đúng cách, không tin client.** Hướng dẫn bảo mật chính thức của Gin về header, binding an toàn, recovery ([Gin — Security Best Practices](https://gin-gonic.com/en/docs/middleware/security-guide/)).
- **Metric theo route template, không theo URL thật.** Path biến (`/users/123`) tạo cardinality bùng nổ — dùng `c.FullPath()` (`/users/:id`) như `middleware.Metrics`, phù hợp khuyến nghị tránh high-cardinality label ([Gin docs / pkg.go.dev](https://pkg.go.dev/github.com/gin-gonic/gin)).

## 6. Lỗi thường gặp & anti-patterns

- **Leak raw error ra client.** `c.JSON(500, gin.H{"error": err.Error()})` lộ chi tiết DB/stacktrace. Đúng: `httpx.Fail` message generic, log chi tiết nội bộ kèm trace_id.
- **Quên `return` sau khi `Fail`.** Sau `httpx.Fail(...)` phải `return` ngay, nếu không handler chạy tiếp và ghi response lần hai ("superfluous WriteHeader").
- **Dùng `c.JSON` đa hình dạng.** Mỗi handler một format → frontend khổ. Luôn qua `httpx.OK/Fail`.
- **Validate sau khi đã dùng input.** Phải validate trước khi gọi use case; đừng để query DB rồi mới phát hiện email rỗng.
- **Metric/label high-cardinality.** Đừng đưa `user_id`, `request_id`, hay URL thật vào label Prometheus (CLAUDE.md §General).
- **CORS `*` + credentials.** Vi phạm bảo mật; `middleware.CORS` cố ý chỉ echo một origin.
- **Bỏ qua IDOR.** `GET /users/:id` không kiểm tra chủ sở hữu → ai cũng đọc được hồ sơ người khác. Xem `handler.go:223`.
- **Đặt `Recovery` sai vị trí.** Nếu không phải middleware ngoài cùng, panic ở lớp sớm không được bắt.
- **Rate limiter in-memory tưởng đủ cho prod.** Multi-instance cần store chung (Redis) — hiện là hạn chế đã biết.

## 7. Lộ trình luyện tập NGAY trong repo LogMon (🥉 cơ bản → 🥈 trung cấp → 🥇 nâng cao)

### 🥉 Cơ bản
1. Thêm route `GET /api/v1/system/info` trả `{version, uptime}` qua `httpx.OK`, đăng ký trong `buildRouter` (`main.go`), bảo vệ bằng `auth.RequireAuth`.
2. Thêm field `validate:"max=255"` cho một field mới trong `registerRequest` (`user/adapters/http/handler.go`) và viết test bảng kiểm 400 khi vượt giới hạn.
3. Đọc `middleware.SecurityHeaders` rồi thêm header `Referrer-Policy: no-referrer`; chạy `curl -I` để xác nhận xuất hiện.
4. Thêm một sentinel error mới vào `shared/errors/errors.go` và một nhánh `errors.Is` tương ứng trong `httpx.FailFromError`.

### 🥈 Trung cấp
1. Viết middleware `MaxBodyBytes(n)` trong `shared/middleware` dùng `http.MaxBytesReader`, trả 413 khi body vượt ngưỡng (theo OWASP); ráp vào group `/api/v1`.
2. Thêm metric histogram mới (vd `logmon_http_request_body_bytes`) vào `shared/metrics/metrics.go`, observe trong `middleware.Metrics`, và xác nhận nó xuất hiện ở `/metrics`.
3. Thêm pagination `?page=&limit=` (max 100) cho `GET /alert-rules` ở `alerting/adapters/http/handler.go`, set `httpx.Meta`, validate server-side limit.
4. Viết handler test cho `failDomain` bao đủ các nhánh 404/409/401/400/500 bằng table-driven test + `httptest`.

### 🥇 Nâng cao
1. Refactor sang error-handling middleware tập trung: handler gọi `c.Error(err)`, một middleware cuối chain đọc `c.Errors.Last()` và gọi `failDomain` một chỗ duy nhất; giữ test xanh.
2. Triển khai error body theo RFC 9457 (`application/problem+json`) như một option, align với `doc_v2/07` §1.1 (envelope `error:{code,message}`), kèm migration note.
3. Thay rate limiter in-memory bằng abstraction `ports.RateStore` để có thể cắm Redis (planned), giữ middleware không đổi.
4. Thêm middleware `X-Workspace-ID` (planned GĐ3): parse header, validate user là member, đẩy workspaceID vào context thay cho hằng `_defaultWorkspaceID`.

## 8. Skill/agent ECC nên dùng khi luyện

- **`ecc:go-review`** (go-reviewer) — chạy ngay sau khi viết/sửa handler hoặc middleware: bắt lỗi idiom, thiếu `return` sau `Fail`, error wrapping sai, concurrency của rate limiter.
- **`ecc:api-design`** — khi thiết kế route mới, status code, pagination, hoặc khi align envelope với RFC 9457 / `doc_v2/07`. Dùng trước khi code.
- **`ecc:security-reviewer`** (qua `/cso` hoặc trực tiếp) — bắt buộc khi đụng auth/CSRF/input handling: kiểm IDOR, leak error, CORS, header bảo mật, fail-closed.
- **`ecc:go-test`** — ép TDD table-driven cho handler test (`httptest`) và xác minh coverage ≥ 80% theo testing.md.
- **`ecc:go-build`** — khi build/vet/lint fail sau refactor middleware chain.

## 9. Tài nguyên học thêm (link đã research, có chú thích 1 dòng)

- [Gin — Model binding and validation](https://gin-gonic.com/en/docs/binding/binding-and-validation/) — cách `ShouldBind*` và tag validate hoạt động, chính thức.
- [Gin — Error handling middleware](https://gin-gonic.com/en/docs/middleware/error-handling-middleware/) — pattern `c.Error()` + `c.Errors` để tập trung hoá lỗi.
- [Gin — Security Best Practices](https://gin-gonic.com/en/docs/middleware/security-guide/) — header, binding an toàn, recovery theo khuyến nghị Gin.
- [RFC 9457 — Problem Details for HTTP APIs](https://www.rfc-editor.org/rfc/rfc9457.html) — chuẩn error body hiện đại (kế nhiệm RFC 7807).
- [OWASP REST Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html) — input validation, giới hạn size, rate limiting 429.
- [pkg.go.dev — gin-gonic/gin](https://pkg.go.dev/github.com/gin-gonic/gin) — API reference Engine/RouterGroup/Context/binding.

## 10. Checklist "đã hiểu" (self-assessment)

- [ ] Giải thích được thứ tự middleware chain trong `buildRouter` và tại sao `Recovery` ngoài cùng.
- [ ] Phân biệt được binding (`ShouldBindJSON`) với validation (`validator/v10`) và status code tương ứng.
- [ ] Biết tại sao mọi response đi qua `httpx.Envelope` và mọi error qua `failDomain`/`FailFromError`.
- [ ] Chỉ ra được cơ chế chống IDOR ở `user` handler và chống CSRF ở `CSRFProtector`.
- [ ] Hiểu vì sao metric dùng `c.FullPath()` thay vì URL thật.
- [ ] Phân biệt được phần đã implemented (envelope `success`, cookie `logmon_token`) với target trong `doc_v2/07` (RFC-9457-style, `lm_access`).
- [ ] Nêu được hạn chế của rate limiter in-memory và hướng khắc phục (Redis, planned).
- [ ] Viết được một handler test bằng `httptest` phủ cả nhánh thành công và lỗi.
