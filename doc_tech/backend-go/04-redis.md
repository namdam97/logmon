# Redis trong LogMon (cache & rate limit)
> Module BE-4 · caching, rate limiting, refresh-token store · Độ khó: 🥉→🥇 · Prereqs: BE-3

## 1. Vì sao kỹ năng này quan trọng trong LogMon

LogMon là nền tảng observability chạy **nhiều replica** sau Nginx (xem `CLAUDE.md` — Kubernetes prod). Có ba bài toán mà bộ nhớ trong process (in-memory) **không** giải được khi scale ngang:

1. **Rate limiting công bằng.** Hiện tại `RateLimiter` dùng map in-memory (`backend/internal/shared/middleware/ratelimit.go:16`). Comment ở dòng 12-13 nói thẳng: *"Phù hợp single-instance; multi-instance prod nên dùng store tập trung (Redis)."* Với 3 replica, mỗi replica chỉ thấy 1/3 traffic của một IP → giới hạn thực tế bị nhân 3, sai hoàn toàn.
2. **Cache read-model (CQRS).** Các BC `alerting`/`slo`/`logpipeline` dùng CQRS; read side cần "denormalized views, cache Redis" (`doc_v2/02-backend-architecture.md:58`). Query Postgres lặp lại cho mỗi dashboard refresh sẽ giết DB.
3. **Token/session store có TTL.** Refresh token rotation (ADR-023) cần một store nhanh, có expiry tự động và revoke tức thì cho cả family.

Redis là cơ sở dữ liệu key-value in-memory, **dùng chung cho mọi replica**, mỗi key có thể gắn TTL (tự hết hạn). Đây là công cụ chuẩn để giải cả ba bài toán trên. ADR-028 đã chốt `go-redis/redis_rate/v10` (GCRA) cho rate limiting; roadmap GĐ3 chốt Redis làm delivery queue cho `notification/`.

> **Trạng thái thực tế (rất quan trọng):** Redis **CHƯA** được implement trong repo. `go-redis` không có trong `backend/go.mod`; không có service redis trong `infra/docker/docker-compose.yml`; rate limit đang là in-memory; refresh token đang lưu **Postgres** (`backend/internal/user/adapters/postgres/refresh_repository.go:16`). Toàn bộ phần Redis trong bài là **planned** theo doc_v2/ADR. Service Redis xuất hiện sớm nhất ở GĐ1.4 (`doc_v2/12-roadmap.md:31` — redis exporter cho Prometheus), rate limit GCRA per-IP gắn với GĐ1 (`doc_v2/02-backend-architecture.md:77` — "1 ✅ per-IP, 3 per-workspace"), còn cache/queue Redis trải ở GĐ3 (notification queue 3.2, per-workspace rate limit 3.6) và GĐ4 (topology cache 4.4). Bài này dạy bạn để khi các giai đoạn đó tới, bạn implement đúng.

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Hình dung Redis như **một cái `map[string]value` khổng lồ sống trong RAM của một process riêng**, mà mọi service của bạn đều kết nối tới qua mạng (TCP, mặc định cổng 6379). Khác với một map Go thường:

- **Chia sẻ toàn cục.** 3 replica `userservice` cùng đọc/ghi một map duy nhất → trạng thái nhất quán giữa các instance.
- **TTL.** Mỗi key có thể "tự xóa" sau N giây. Đây là tính năng nền tảng cho cache, denylist token, rate-limit window.
- **Nguyên tử (atomic) + đơn luồng logic.** Redis thực thi lệnh tuần tự; `INCR`, `SET ... NX`, hay một Lua script chạy "all-or-nothing". Không cần mutex phân tán cho các thao tác cơ bản.
- **Đánh đổi:** dữ liệu nằm trong RAM, có thể mất khi restart (trừ khi bật persistence). Vì vậy Redis hợp với dữ liệu **phái sinh / tạm thời** (cache, counter, session) — **không** thay thế Postgres làm nguồn sự thật.

Quy tắc vàng để quyết định *có nên đưa dữ liệu vào Redis không*: "Mất key này có làm hỏng tính đúng đắn không?" Nếu chỉ làm chậm (phải tính lại từ Postgres) → hợp với Redis. Nếu mất là mất tiền/mất dữ liệu nghiệp vụ → để ở Postgres.

## 3. Khái niệm cốt lõi (tăng dần độ khó)

### 3.1 Client & connection pool (`go-redis`)

`go-redis` (module maintained: `github.com/redis/go-redis/v9`) quản lý một **pool kết nối**. Bạn tạo client một lần lúc khởi động, dùng lại suốt vòng đời service:

```go
import "github.com/redis/go-redis/v9"

rdb := redis.NewClient(&redis.Options{
    Addr:         "redis:6379",
    DialTimeout:  1 * time.Second,
    ReadTimeout:  500 * time.Millisecond, // khớp "Redis 500ms" — doc_v2/02:190
    WriteTimeout: 500 * time.Millisecond,
    PoolSize:     10 * runtime.GOMAXPROCS(0),
})
```

Mọi lệnh đều nhận `context.Context` làm tham số đầu (đúng style guide LogMon): `rdb.Get(ctx, key)`.

### 3.2 Các thao tác nền tảng

| Lệnh | Ý nghĩa | Dùng cho |
|------|---------|----------|
| `SET k v EX 60` | ghi kèm TTL 60s | cache, session |
| `GET k` | đọc (miss → `redis.Nil`) | cache-aside |
| `SETNX` / `SET NX` | chỉ set nếu chưa tồn tại | lock, single-flight |
| `INCR` / `EXPIRE` | đếm + đặt hạn | rate limit thô |
| `DEL k` | xóa | invalidation, revoke |
| `EVAL <lua>` | chạy script atomic | GCRA, claim token |

Phân biệt `redis.Nil` (key không tồn tại — *không phải lỗi*) với lỗi thật: `if errors.Is(err, redis.Nil)` → cache miss; lỗi khác → lỗi hạ tầng.

### 3.3 Cache-aside (lazy loading)

Mẫu phổ biến nhất: đọc cache trước; miss thì đọc nguồn (Postgres) rồi ghi ngược lại cache với TTL.

```go
v, err := rdb.Get(ctx, key).Bytes()
if errors.Is(err, redis.Nil) {        // miss
    v = loadFromPostgres(ctx, id)
    rdb.Set(ctx, key, v, ttlWithJitter()) // jitter chống đồng loạt hết hạn
}
```

### 3.4 Rate limiting: từ thô tới GCRA

- **Thô (fixed window):** `INCR key` + `EXPIRE 60`. Đơn giản nhưng có "biên cửa sổ" (burst gấp đôi ở ranh giới phút).
- **GCRA (redis_rate v10 — ADR-028):** thuật toán "leaky bucket" mượt, atomic qua **một Lua script**, chính xác hơn fixed window, rẻ hơn sliding-window-log. Hoạt động đúng dù nhiều replica vì state nằm ở Redis.

```go
import "github.com/go-redis/redis_rate/v10"

limiter := redis_rate.NewLimiter(rdb)
res, err := limiter.Allow(ctx, "ip:"+ip, redis_rate.PerMinute(60))
// res.Allowed (>0 = cho qua), res.Remaining, res.RetryAfter, res.ResetAfter
```

### 3.5 Token store / denylist với TTL

Đặt **TTL của key Redis bằng thời gian sống còn lại của token** → khi token hết hạn tự nhiên, key tự biến mất, không cần dọn rác. Hai chiến lược:

| Chiến lược | Cơ chế | Ưu/nhược |
|-----------|--------|----------|
| Refresh rotation (OAuth2) | mỗi refresh sinh token mới, vô hiệu cũ; reuse → revoke cả family | Chuẩn LogMon (ADR-023); access token ngắn hạn |
| jti denylist | lưu `jti` đã revoke, TTL = exp còn lại | Revoke access token tức thì nhưng mọi verify phải hỏi Redis |

## 4. LogMon dùng nó thế nào (bám code thật — path:line, ghi rõ implemented/planned)

- **Rate limit — IMPLEMENTED (in-memory, chưa Redis).** `backend/internal/shared/middleware/ratelimit.go:16-21` định nghĩa `RateLimiter` với `clients map[string]*rate.Limiter` dùng `golang.org/x/time/rate`. Middleware trả 429 tại dòng 50-60. Wiring trong `backend/cmd/userservice/main.go:386-387`: `authRate := middleware.NewPerMinuteLimiter(...)`. Comment dòng 12-15 ghi rõ hạn chế multi-instance và thiếu evict TTL.
- **Rate limit Redis — PLANNED.** ADR-028 (`doc_v2/13-adr.md:204-208`): `go-redis/redis_rate/v10` GCRA, **fail-open khi Redis down** (kèm log + metric). Bảng middleware order `doc_v2/02-backend-architecture.md:77`: ratelimit per-IP GĐ1, per-workspace GĐ3.
- **Refresh token store — IMPLEMENTED ở Postgres, KHÔNG phải Redis.** `backend/internal/user/adapters/postgres/refresh_repository.go:16`. Đáng chú ý `ClaimByHash` (dòng 42-56) dùng `UPDATE ... RETURNING ... WHERE used_at IS NULL` để claim **atomic** — đúng tinh thần "atomic" mà Redis cũng đạt được, nhưng ở đây làm bằng Postgres. Service rotation + reuse-detection: `backend/internal/user/app/refresh_service.go:13-16`.
- **Cache read-model — PLANNED.** `doc_v2/02-backend-architecture.md:58` mô tả read side CQRS dùng "cache Redis". Topology cache GĐ4: `doc_v2/12-roadmap.md:103` ("materialized 30s, Redis cache").
- **Notification delivery queue — PLANNED (GĐ3).** `doc_v2/12-roadmap.md:78`: `notification/` BC dùng "Redis delivery queue".
- **redisotel tracing — PLANNED (GĐ2.6).** `doc_v2/12-roadmap.md:57` liệt kê `redisotel` cùng otelgin/otelpgx. Tham chiếu timeout chuẩn: `doc_v2/02-backend-architecture.md:190` ("Redis 500ms"). Key prefix bảo mật: `doc_v2/09-security.md:79` (`ws:{id}:`).

Quy tắc kiến trúc: theo `doc_v2/02-backend-architecture.md:31`, `domain/` **KHÔNG** import redis; client Redis chỉ sống ở `adapters/redis/`, lộ ra qua interface trong `ports/` (Repository/Cache). Đây là lý do refresh token store đặt ở `adapters/postgres/` thỏa interface `ports.RefreshTokenRepository` — đổi sang Redis chỉ cần thêm `adapters/redis/` implement cùng port, không đụng `app/`.

## 5. Best practices (mỗi mục kèm 1 nguồn đã research)

1. **Đặt cả 3 timeout (Dial/Read/Write) — đừng chỉ dựa vào context.** go-redis chạy vài background check không qua context và dựa vào connection timeout; tắt chúng dễ gây pool exhaustion. Với LogMon, neo theo `Redis 500ms` ở `doc_v2/02:190`. ([Uptrace — debugging go-redis](https://redis.uptrace.dev/guide/go-redis-debugging.html))
2. **Fail-open cho rate limit khi Redis down.** Nếu `limiter.Allow` lỗi hạ tầng, *cho request qua* + log + tăng metric — đúng ADR-028. Đừng để Redis chết kéo sập toàn bộ API. ([go-redis/redis_rate](https://github.com/go-redis/redis_rate))
3. **TTL + jitter để chống cache stampede.** Thêm ngẫu nhiên ±10% vào TTL để key không hết hạn đồng loạt; cân nhắc single-flight/XFetch cho hot key. ([Redis blog — taming the thundering herd](https://redis.io/blog/how-to-tame-the-thundering-herd-problem/))
4. **Token store: chỉ lưu `jti`/hash, TTL = exp còn lại.** Không lưu token thô (lớn + lộ bí mật); để key tự dọn khi token hết hạn. LogMon đã lưu **hash** token trong DB — giữ nguyên nguyên tắc này khi port sang Redis. ([oneuptime — JWT blacklist với Redis](https://oneuptime.com/blog/post/2026-03-31-redis-how-to-build-a-token-blacklist-for-jwt-revocation-with-redis/view))
5. **Ưu tiên refresh rotation + access token ngắn hạn hơn denylist mọi request.** Denylist tái tạo DB-lookup mà JWT muốn tránh; rotation (ADR-023) là mặc định. ([JWT Best Practices 2026 — JSONCraft](https://jsoncraft.dev/docs/jwt-best-practices-2026/))
6. **GCRA qua redis_rate thay vì tự viết sliding window.** Một Lua script atomic, đúng khi nhiều replica, cần Redis 3.2+. ([redis_rate v10 README](https://github.com/go-redis/redis_rate/blob/v10/README.md))

## 6. Lỗi thường gặp & anti-patterns

- **Coi `redis.Nil` là lỗi.** Cache miss là chuyện bình thường; phải `errors.Is(err, redis.Nil)` rồi fallback Postgres, đừng trả 500.
- **Rate limit in-memory trên nhiều replica.** Chính xác là tình trạng hiện tại của repo (`ratelimit.go:12`) — chấp nhận cho skeleton, nhưng **sai** khi prod multi-instance. Đừng quên migrate trước khi scale.
- **Không TTL → memory leak.** Key không expiry tích tụ tới khi Redis OOM. Cũng là bug tiềm ẩn của `clients map` hiện tại (không evict — `ratelimit.go:14`).
- **Dùng Redis làm nguồn sự thật cho dữ liệu nghiệp vụ.** Refresh token nếu chỉ ở Redis mà không bật persistence → restart mất hết session. LogMon hiện đặt ở Postgres là an toàn; nếu chuyển Redis phải bật AOF hoặc chấp nhận đánh đổi.
- **Cross-BC import qua Redis client.** Vi phạm `domain/` → import adapter. Luôn để client sau `ports/`.
- **TTL cố định + cache stampede.** Hàng loạt key hết hạn cùng lúc → "dog-pile" lên Postgres. Thiếu jitter là nguyên nhân kinh điển.
- **`InsecureSkipVerify` hoặc không TLS tới Redis managed.** Vi phạm mục Security của CLAUDE.md; dùng TLS + auth khi Redis không nằm trong mạng tin cậy.

## 7. Lộ trình luyện tập NGAY trong repo LogMon (🥉 cơ bản → 🥈 trung cấp → 🥇 nâng cao)

### 🥉 Cơ bản
1. Thêm service `redis:7` vào `infra/docker/docker-compose.yml` (cổng 6379, sau profile mặc định hoặc `observability`), kèm healthcheck `redis-cli ping`.
2. `cd backend && go get github.com/redis/go-redis/v9` và `github.com/go-redis/redis_rate/v10`; xác nhận chúng xuất hiện trong `backend/go.mod`.
3. Viết hàm `newRedisClient()` trong `backend/cmd/userservice/main.go` (đọc `REDIS_ADDR` từ env, set Dial/Read/Write timeout 500ms như `doc_v2/02:190`); log "redis connected" sau `PING`.
4. Đọc kỹ `backend/internal/shared/middleware/ratelimit.go` và viết ra giấy 3 lý do tại sao map in-memory sai trên 3 replica.

### 🥈 Trung cấp
1. Tạo `backend/internal/shared/middleware/ratelimit_redis.go`: middleware mới dùng `redis_rate.Limiter`, key = `"ratelimit:ip:" + c.ClientIP()`, giữ nguyên response 429 + JSON envelope như bản cũ.
2. Implement **fail-open** (ADR-028): khi `limiter.Allow` trả lỗi hạ tầng → `c.Next()` + `log.Warn` + tăng một counter mới trong `backend/internal/shared/metrics/metrics.go` (vd `logmon_ratelimit_failopen_total`), expose ở `/metrics`.
3. Viết test table-driven (`ratelimit_redis_test.go`) dùng `testify/require`: case cho-qua, case 429, case fail-open khi client trỏ tới Redis chết. Chạy `go test -race`.
4. Thay `authRate.Middleware()` ở `main.go:387` bằng middleware Redis sau cờ env `RATELIMIT_BACKEND=redis`, fallback in-memory nếu không set.

### 🥇 Nâng cao
1. Thêm `adapters/redis/refresh_repository.go` trong `internal/user/` implement đúng interface `ports.RefreshTokenRepository`; viết lại `ClaimByHash` bằng **Lua script `EVAL`** để giữ tính atomic (so sánh với bản Postgres `UPDATE ... RETURNING`). TTL key = `expires_at - now`.
2. Viết cache-aside cho một read-model `alerting` (vd danh sách alert active): key `alerting:active:{ws}`, TTL 30s **có jitter ±10%**, invalidation khi nhận domain event `AlertFired/AlertResolved` qua bus (`backend/internal/shared/outbox/bus.go`).
3. Bọc Redis client bằng `redisotel` (xem `doc_v2/12:57`) để span Redis nối vào trace Jaeger; thêm histogram độ trễ Redis vào `metrics.go`.
4. Mô phỏng cache stampede: bắn 200 request đồng thời vào một key vừa hết hạn, chứng minh single-flight (`SET NX` lock) giảm số query Postgres về 1; ghi kết quả vào một `/retro`.

## 8. Skill/agent ECC nên dùng khi luyện

- **`ecc:redis-patterns`** — gọi khi bắt đầu các task 🥈/🥇 (viết middleware GCRA, cache-aside, token store) để có mẫu connection pool, fail-open, key-naming chuẩn trước khi tự code.
- **`ecc:go-review`** (agent go-reviewer) — chạy **sau** mỗi khi viết xong middleware/adapter Redis: soát concurrency (goroutine có stop signal?), error wrapping `%w`, `redis.Nil` handling, interface nhỏ.
- **`ecc:go-test`** — ép TDD cho `ratelimit_redis_test.go` và adapter test (viết test đỏ trước), verify coverage ≥80% như rule testing của project.
- **`ecc:go-build`** — nếu `go get` redis làm hỏng build (version drift trong `go.mod`), dùng để fix tăng dần.
- **`ecc:security-review` / `/cso`** — trước khi commit code chạm auth/token store: kiểm TLS tới Redis, không lưu token thô, không hardcode `REDIS_PASSWORD`.

## 9. Tài nguyên học thêm (link đã research, có chú thích 1 dòng)

- [redis_rate v10 README](https://github.com/go-redis/redis_rate/blob/v10/README.md) — API GCRA chính thức (`NewLimiter`, `Allow`, `PerMinute`), thuật toán LogMon đã chốt ở ADR-028.
- [pkg.go.dev — redis_rate/v10](https://pkg.go.dev/github.com/go-redis/redis_rate/v10) — chi tiết struct `Result` (Allowed/Remaining/RetryAfter/ResetAfter) và các `Limit` constructor.
- [Uptrace — Debugging go-redis: pool size & timeouts](https://redis.uptrace.dev/guide/go-redis-debugging.html) — vì sao phải đặt Dial/Read/Write timeout dù đã có context.
- [Redis blog — How to tame the thundering herd](https://redis.io/blog/how-to-tame-the-thundering-herd-problem/) — TTL jitter, single-flight, XFetch chống cache stampede.
- [oneuptime — Token blacklist for JWT revocation with Redis](https://oneuptime.com/blog/post/2026-03-31-redis-how-to-build-a-token-blacklist-for-jwt-revocation-with-redis/view) — lưu `jti`, TTL = exp còn lại, namespace key.
- [JWT Best Practices 2026 — JSONCraft](https://jsoncraft.dev/docs/jwt-best-practices-2026/) — refresh rotation vs denylist; vì sao access token nên ngắn hạn.

## 10. Checklist "đã hiểu" (5-8 ý self-assessment)

- [ ] Giải thích được vì sao `RateLimiter` in-memory (`ratelimit.go:16`) sai trên multi-replica và Redis sửa thế nào.
- [ ] Phân biệt được `redis.Nil` (cache miss) với lỗi hạ tầng, và viết được cache-aside có fallback Postgres.
- [ ] Nói rõ trạng thái repo: rate limit = in-memory (implemented), refresh token = Postgres (implemented), Redis = planned (ADR-028 / GĐ3).
- [ ] Biết GCRA của redis_rate atomic nhờ Lua script và chính xác hơn fixed window.
- [ ] Hiểu "fail-open khi Redis down" của ADR-028 và biết phải kèm log + metric.
- [ ] Biết đặt TTL = exp còn lại cho token denylist và dùng jitter chống stampede.
- [ ] Biết Redis client phải nằm ở `adapters/redis/` sau `ports/`, không để `domain/` import (`doc_v2/02:31`).
- [ ] Đặt được đủ 3 timeout (Dial/Read/Write 500ms) theo `doc_v2/02:190` và giải thích lý do.
