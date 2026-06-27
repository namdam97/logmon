# Go production-grade trong LogMon
> Module BE-1 · Uber style, run() pattern, error wrap, goroutine an toàn · Độ khó: 🥉→🥇 · Prereqs: không

## 1. Vì sao kỹ năng này quan trọng trong LogMon

LogMon là nền tảng observability cho Go microservices — bản thân backend cũng *là* một Go service production phải tự dogfood logging/metrics/tracing của chính nó. Một service như `userservice` (composition root tại `backend/cmd/userservice/main.go`) phải làm đúng vài việc "boring" nhưng sống còn: khởi tạo tài nguyên đúng thứ tự, nhận tín hiệu shutdown, đóng connection pool sạch, không rò goroutine, không panic làm sập process, và trả lỗi có ngữ cảnh để debug được trong production.

Nếu sai mấy điểm này thì: service crash giữa request (panic không recover), mất event khi deploy (goroutine bị giết giữa chừng), hoặc log/metric vô dụng vì lỗi bị nuốt. Toàn bộ `CLAUDE.md` mục "Go Style Guide" và `doc_v2/11-coding-testing-standards.md` được viết để chặn các lỗi đó. Module này dạy bạn *vì sao* các quy tắc đó tồn tại và *LogMon đã áp dụng ở đâu*.

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Một Go service production, ở mức trừu tượng nhất, là: `main()` → dựng đồ thị phụ thuộc (dependency graph) → chạy → nhận tín hiệu dừng → tháo dỡ ngược lại. Bốn nguyên lý nền:

1. **Một process là một cây tài nguyên có vòng đời.** Pool DB, tracer, HTTP server, background worker đều phải *mở* được và *đóng* được. Mở theo thứ tự, đóng theo thứ tự ngược (LIFO) — đúng ngữ nghĩa `defer`.
2. **Lỗi là giá trị (values), không phải exception.** Go không có try/catch. Hàm trả `(T, error)`; caller quyết định xử lý. Lỗi mang ngữ cảnh bằng cách *wrap* (`%w`), và caller match bằng `errors.Is`/`errors.As`.
3. **Goroutine không tự dọn.** Bạn tạo nó thì bạn phải có cách (a) báo nó dừng và (b) chờ nó dừng hẳn. Không có cơ chế này = goroutine leak.
4. **Phụ thuộc trỏ vào trong (dependency inversion).** Business logic định nghĩa *interface* (cái nó cần); hạ tầng *implement*. Service không "biết" nó đang nói chuyện với Postgres hay mock.

`main()` của LogMon thể hiện gọn nguyên lý 1: `main()` chỉ gọi `run()` rồi exit một lần (`backend/cmd/userservice/main.go:67-72`). Mọi việc thật nằm trong `run() error` để dùng `defer` được và test được.

## 3. Khái niệm cốt lõi (tăng dần độ khó)

### 3.1 `run()` pattern — `main()` mỏng

`main()` không nên chứa logic. Lý do: `defer` trong `main()` *không chạy* nếu bạn gọi `os.Exit()`, và `main()` khó test. Giải pháp: dồn hết vào `run() error`.

```go
func main() {
    if err := run(); err != nil {        // main chỉ định tuyến lỗi + exit code
        fmt.Fprintln(os.Stderr, "userservice:", err)
        os.Exit(1)
    }
}
```

Bên trong `run()`, mọi `defer` (đóng pool, flush tracer) chạy bình thường vì `run()` *return* chứ không `os.Exit()`.

### 3.2 Error wrapping & sentinel errors

| Cú pháp | Khi nào dùng | Ý nghĩa |
|---|---|---|
| `fmt.Errorf("get alert: %w", err)` | caller cần match lỗi gốc | giữ chain, `errors.Is/As` xuyên qua được |
| `fmt.Errorf("...: %v", err)` | ẩn implementation (biên adapter) | thông điệp giữ lại, chain *không* lộ |
| `var ErrNotFound = errors.New(...)` | trạng thái nghiệp vụ phổ biến | sentinel, so bằng `errors.Is` |
| `type ValidationError struct{...}` | lỗi mang dữ liệu (field nào sai) | type, trích bằng `errors.As` |

LogMon dùng đúng cả 4: sentinel + ValidationError ở `backend/internal/shared/errors/errors.go:12-45`. Quy ước **không dùng "failed to"** (context đã đủ rõ) và **handle-once** (log *hoặc* return, không cả hai).

### 3.3 Interface nhỏ, "accept interfaces, return structs"

Theo ISP, định nghĩa interface *nơi dùng* (tầng `app`/`ports`), không nơi implement. Interface nhỏ dễ mock, dễ thay. Verify tại compile time bằng dòng assertion:

```go
var _ ports.UserRepository = (*Repository)(nil) // sai chữ ký → fail lúc build
```

### 3.4 Goroutine an toàn: stop + wait

Mọi goroutine **phải** có (1) tín hiệu dừng (thường là `ctx` huỷ) và (2) cơ chế chờ (done channel / `WaitGroup`). Mẫu chuẩn:

```go
func (r *Relay) Run(ctx context.Context) {
    defer close(r.done)              // báo "tôi đã thoát"
    ticker := time.NewTicker(r.interval)
    defer ticker.Stop()
    for {
        r.tick(ctx)
        select {
        case <-ctx.Done():           // tín hiệu dừng
            return
        case <-ticker.C:
        }
    }
}
func (r *Relay) Wait() { <-r.done }  // caller chờ thoát hẳn
```

### 3.5 Functional options

Cấu hình tuỳ chọn không dùng struct config khổng lồ mà dùng `Option func(*T)`. Bắt buộc thì là tham số constructor; tuỳ chọn thì là `opts ...Option`.

### 3.6 Graceful shutdown & context

`signal.NotifyContext` biến SIGINT/SIGTERM thành một `ctx` huỷ. Khi huỷ: dừng nhận request mới (`srv.Shutdown`), chờ in-flight xong, chờ worker thoát.

## 4. LogMon dùng nó thế nào (bám code thật — implemented/planned)

Tất cả dưới đây **đã implemented** trừ chỗ ghi rõ `(planned)`.

- **`run()` pattern**: `backend/cmd/userservice/main.go:67-72` (`main`) gọi `run()` tại `:194-328`. Hằng số cấu hình gom đầu file dạng `_serviceName`, `_shutdownTimeout` (`:49-65`) — đúng quy ước "unexported globals tiền tố `_`".
- **Config qua env, fail-fast**: `loadConfig()` (`:96-120`) đọc env; `run()` kiểm tra bắt buộc và trả lỗi sớm: `if cfg.databaseURL == "" { return errors.New("DATABASE_URL not configured") }` (`:198-200`).
- **Khởi tạo + tháo dỡ theo LIFO**: tracer dựng ở `:223-237` với `defer tp.Shutdown(...)` flush span; pool ở `:250-254` với `defer pool.Close()`; mỗi bước lỗi → `fmt.Errorf("...: %w", err)`.
- **Graceful shutdown**: `signal.NotifyContext(... SIGINT, SIGTERM)` (`:218`), server chạy trong goroutine (`:307-312`), `select` chờ lỗi *hoặc* ctx huỷ (`:314-319`), rồi `srv.Shutdown` (`:323`) và `alerting.relay.Wait()` (`:326`) chờ worker thoát.
- **HTTP server không dùng default**: tạo `&http.Server{... ReadHeaderTimeout: _readHeaderTimeout}` (`:300-304`) — đặt `ReadHeaderTimeout` chống Slowloris (đúng khuyến nghị "không dùng `http.DefaultServer`").
- **Goroutine stop+wait**: outbox `Relay` ở `backend/internal/shared/outbox/relay.go:59-75` (`Run`/`Wait`/`done`). Khởi chạy `go alerting.relay.Run(ctx)` ở `main.go:281`, chờ ở `:326`.
- **Error types dùng chung**: sentinel + ValidationError ở `backend/internal/shared/errors/errors.go:12-45`; helper `AsValidationError` bọc `errors.As`.
- **Interface nhỏ + DIP**: `backend/internal/user/ports/` khai báo `UserRepository`, `PasswordHasher`, `TokenIssuer`, `Clock`... mỗi cái 1-4 method; adapter assert compile-time `var _ ports.UserRepository = (*Repository)(nil)` (`backend/internal/user/adapters/postgres/repository.go:27`). Tổng cộng 16 assertion như vậy trong repo.
- **Functional options**: `userapp.WithLogger(log)` truyền vào `NewService(...)` (`main.go:262-269`); định nghĩa tại `backend/internal/user/app/service.go:25-28`. Relay cũng có `WithInterval`/`WithBatchSize`/`WithObserver` (`relay.go:31-40`).
- **Clock injection (không global thời gian)**: `usersys.NewClock()` truyền vào service (`main.go:261`) để test xác định — đúng `doc_v2/11` §2.1.
- **Logger là wrapper zerolog duy nhất**: `backend/internal/shared/logger/logger.go`; `withCtx` (`:52-60`) tự gắn `trace_id`/`span_id` từ `trace.SpanContextFromContext(ctx)` — đúng delta 2026 ở `doc_v2/11` §1.
- **Metrics naming**: `logmon_http_requests_total` / `..._duration_seconds` ở `backend/internal/shared/metrics/metrics.go:24-38`; path dùng route template (`/users/:id`) tránh high-cardinality.
- **Panic recovery**: `middleware.Recovery` (`backend/internal/shared/middleware/middleware.go:79-87`) trả 500 generic thay vì sập process.
- **Value-object enum**: `Severity` ở `backend/internal/alerting/domain/severity.go:6-23` dùng struct bọc string + constructor validate (lưu ý: đây là VO pattern, *không* phải `iota+1`; quy ước `iota+1` trong CLAUDE.md áp cho enum số — repo hiện chưa có chỗ nào dùng).
- **`(planned)`**: rate limit hiện tự viết token-bucket (`backend/internal/shared/middleware/ratelimit.go`) với ghi chú cần TTL/LRU; `doc_v2/11` §1 đặt mục tiêu đổi sang `go-redis/redis_rate/v10` (GCRA) — **chưa có `go-redis` trong `backend/go.mod`** (cũng chưa có Redis adapter nào). BC `slo` mới có **domain layer** (`backend/internal/slo/domain/` — `slo.go`/`rules.go`/`events.go` + test), **chưa có app/ports/adapters và chưa wire vào `main.go`**. Các BC `incident`/`notification` và k8s manifests vẫn là target roadmap, **chưa có code**.

## 5. Best practices (mỗi mục kèm 1 nguồn đã research)

1. **`main()` mỏng, logic trong `run()`** — main.go chỉ wiring + config + start; `defer` chạy được vì không `os.Exit` trong run. ([Effective Go](https://go.dev/doc/effective_go))
2. **Wrap lỗi khi thêm ngữ cảnh, dùng `%w` để giữ chain** — chỉ wrap khi sẵn sàng coi lỗi đó là phần API của bạn; match bằng `errors.Is`/`errors.As`. ([Go 1.13 errors blog](https://go.dev/blog/go1.13-errors))
3. **Goroutine lifetime phải hiển nhiên; quản lý bằng `context`** — ràng buộc số lượng và vòng đời goroutine qua `context.Context`, ưu tiên context hơn channel tự chế. ([Google Go Style Guide — decisions](https://google.github.io/styleguide/go/decisions.html))
4. **Không dùng `http.DefaultServer`/`DefaultClient` trong production** — các hàm package-level để timeout mặc định *off*, "unfit for public Internet"; luôn tạo `http.Server`/`http.Client` riêng có `ReadTimeout`/`WriteTimeout`/`ReadHeaderTimeout`. ([Cloudflare — net/http timeouts](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/))
5. **`/cmd` cho binary, `/internal` cho code không export** — đúng layout LogMon (`backend/cmd/...`, `backend/internal/...`). ([Standard Go Project Layout](https://github.com/golang-standards/project-layout))
6. **Luôn chạy `go test -race`**, dùng `context.WithTimeout` cho mọi I/O có thể treo. ([Effective Go](https://go.dev/doc/effective_go))

## 6. Lỗi thường gặp & anti-patterns

- **`defer` trong `main()` rồi `os.Exit()`** → defer không chạy, pool/tracer không đóng. *Sửa*: pattern `run()`.
- **Goroutine không có lối thoát** (`go func(){ for{...} }()`) → leak khi shutdown. *Sửa*: `select { case <-ctx.Done(): return ... }` + `Wait()` như `Relay`.
- **Nuốt lỗi** (`_ = err` hoặc log rồi return luôn) → mất dấu vết hoặc log trùng. *Sửa*: handle-once.
- **`%v` khi caller cần match** → `errors.Is` thất bại vì chain bị cắt. *Sửa*: `%w` khi cần inspectable; `%v` chỉ ở biên adapter để ẩn detail.
- **"failed to ..." lặp lại** → message dài dòng "failed to failed to". *Sửa*: context ngắn `"get alert: %w"`.
- **High-cardinality label** (`user_id`, `trace_id`, raw path) → nổ time-series Prometheus. *Sửa*: route template, đã làm ở `metrics.go`.
- **`gin.Default()`** kéo logger/recovery mặc định không kiểm soát. *Sửa*: `gin.New()` + middleware tự quản (`main.go:347-358`).
- **Mutable global cho thời gian/ID** → test không xác định. *Sửa*: inject `Clock`/`IDGenerator`.
- **Embed `sync.Mutex`** vào struct export → lộ `Lock()` ra API. *Sửa*: field có tên (`mu sync.RWMutex`) như `Bus` (`bus.go:16`).

## 7. Lộ trình luyện tập NGAY trong repo LogMon

### 🥉 Cơ bản
1. Đọc `backend/cmd/userservice/main.go`, vẽ ra giấy thứ tự *mở* tài nguyên (tracer→pool→service→relay→server) và đối chiếu thứ tự `defer` *đóng* ngược lại.
2. Thêm một hằng số `_healthzTimeout` (thay literal `2*time.Second` ở `:361`) vào block `const (...)` đầu file, dùng đúng quy ước `_` prefix; chạy `go build ./...`.
3. Thêm một sentinel `ErrForbidden = errors.New("forbidden")` vào `backend/internal/shared/errors/errors.go`, viết test `errors.Is` cho nó trong `errors_test.go`.
4. Chạy `cd backend && go test -race ./internal/shared/...` và đọc output coverage.

### 🥈 Trung cấp
1. Thêm một metric `Counter` mới `logmon_outbox_events_dispatched_total` vào package metrics (theo mẫu `metrics.go:24-40`), wire qua `outbox.Observer`, và expose ở `/metrics`; kiểm bằng `curl localhost:8080/metrics | grep dispatched`.
2. Viết một adapter `Clock` giả trong test cho `userapp.Service` để khẳng định token TTL tính từ thời điểm inject (table-driven, `give/want`).
3. Thêm `WithMaxRetries(n int) RelayOption` vào `outbox/relay.go` theo mẫu functional options hiện có, kèm test mặc định + override.
4. Thêm một interface nhỏ + `var _ Iface = (*Impl)(nil)` cho một adapter chưa có assertion, chạy `go vet ./...`.

### 🥇 Nâng cao
1. Viết integration test (build tag `//go:build integration`) cho `outbox.Relay`: ghi event trong TX → relay nhặt → subscriber nhận → status `published`; kill ctx giữa chừng → khẳng định không mất event (at-least-once), theo `doc_v2/11` §2.2.
2. Thêm một background worker thứ hai vào `run()` (vd dọn refresh-token hết hạn theo chu kỳ) tuân chuẩn stop+wait giống `Relay`, đảm bảo `make test` xanh và shutdown ≤ `_shutdownTimeout`.
3. Refactor `RateLimiter` (`ratelimit.go`) thêm eviction TTL cho map `clients` (sửa đúng `LƯU Ý` trong comment), giữ thread-safe, viết test concurrency với `-race`.

## 8. Skill/agent ECC nên dùng khi luyện

- **`ecc:golang-patterns`** — khi cần mẫu chuẩn (functional options, error wrapping, interface nhỏ, concurrency lifecycle) trước khi viết code mới. Dùng ở bước thiết kế task 🥈/🥇.
- **`ecc:golang-testing` / `ecc:go-test`** — khi viết test: `go-test` ép TDD (RED→GREEN), bảo đảm table-driven + `-race` + coverage 80% (đúng `doc_v2/11` §2.4). Dùng cho mọi task có chữ "viết test".
- **`ecc:go-review`** — sau khi viết/sửa code, review idiomatic + concurrency safety + error handling. Bắt buộc trước commit theo `CLAUDE.md` (mục code review).
- **`ecc:go-build`** — khi `go build`/`go vet`/lint đỏ, sửa tối thiểu và an toàn. Dùng khi task 🥈/🥇 gãy build.

## 9. Tài nguyên học thêm

- [Effective Go](https://go.dev/doc/effective_go) — tài liệu nền tảng về idiom Go (đặt tên, error, concurrency, slice/map).
- [Go 1.13 — Working with Errors](https://go.dev/blog/go1.13-errors) — chuẩn `%w`, `errors.Is`/`As`, khi nào nên wrap.
- [Google Go Style Guide — Decisions](https://google.github.io/styleguide/go/decisions.html) — quyết định cụ thể về goroutine lifetime, context, naming.
- [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md) — nguồn chính cho quy ước LogMon (functional options, mutex field, channel size, enum).
- [Standard Go Project Layout](https://github.com/golang-standards/project-layout) — quy ước `/cmd`, `/internal`, `/pkg`.
- [OWASP Go Secure Coding Practices](https://owasp.org/www-project-go-secure-coding-practices-guide/) — input validation, crypto/rand, query tham số hoá.

## 10. Checklist "đã hiểu"

- [ ] Giải thích được vì sao `defer pool.Close()` trong `main()` *không* chạy nếu gọi `os.Exit`, và `run()` pattern khắc phục thế nào.
- [ ] Phân biệt được khi nào dùng `%w` vs `%v`, và `errors.Is` vs `errors.As`.
- [ ] Chỉ ra trong `main.go` đúng 2 thứ làm cho mọi goroutine có stop *và* wait (`ctx.Done()` + `relay.Wait()`).
- [ ] Viết được một interface nhỏ + dòng assertion compile-time và biết nó bắt lỗi gì.
- [ ] Thêm được một functional option mới đúng mẫu repo.
- [ ] Biết vì sao path trong metric phải là route template, không phải URL thật.
- [ ] Phân biệt được phần đã implemented (outbox relay, argon2id, JWT) vs planned/dang dở (go-redis rate limit chưa có; `slo` mới có domain layer chưa wire; `incident`/`notification` BC chưa có code).
- [ ] Chạy được `go test -race ./...` và đọc hiểu coverage.
