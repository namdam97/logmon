# Persistence: pgx + migrations trong LogMon
> Module BE-3 · pgxpool, parameterized query, migrate up/down · Độ khó: 🥉→🥇 · Prereqs: BE-1

## 1. Vì sao kỹ năng này quan trọng trong LogMon

LogMon là nền tảng observability cho Go microservices. Mọi trạng thái bền vững — user, refresh token, alert rule, alert instance, outbox event — đều nằm trong PostgreSQL. Tầng `adapters/postgres` là nơi *duy nhất* code chạm vào DB, nên nó là ranh giới bảo mật và đúng đắn quan trọng nhất của backend:

- **Bảo mật**: CLAUDE.md (mục Security/OWASP) bắt buộc *parameterized query* (`$1, $2`) cho mọi truy vấn. Một chỗ nối chuỗi là một lỗ SQL injection.
- **Đúng kiến trúc**: theo layer direction `adapters → ports ← app → domain`, chỉ `adapters/postgres` được import `pgx`; `app/` và `domain/` không biết PostgreSQL tồn tại. Đổi DB hay mock test đều chỉ động vào adapter.
- **Đồng thời an toàn**: rotation refresh-token và relay outbox đều phụ thuộc claim nguyên tử ở mức row của PostgreSQL. Sai một transaction là sai bảo mật (token reuse) hoặc mất/nhân đôi event.

Hiểu cụm `pgx + repository + golang-migrate` là điều kiện cần để đụng vào bất kỳ BC nào của LogMon.

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Hình dung 3 lớp xếp chồng, từ thấp lên cao:

1. **PostgreSQL** — tiến trình lưu dữ liệu trên đĩa, nói chuyện qua giao thức wire trên cổng 5432. Nó hiểu SQL.
2. **Driver `pgx/v5`** — thư viện Go mở socket TCP tới PostgreSQL, mã hoá câu SQL + tham số thành byte, đọc kết quả về thành giá trị Go. `pgx` nói wire protocol *trực tiếp* (không qua `database/sql`), nên nhanh hơn và lộ đủ kiểu dữ liệu PostgreSQL (JSONB, array, timestamptz).
3. **Repository** — struct Go của bạn (`postgres.Repository`) nhận `pgxpool.Pool`, dịch khái niệm nghiệp vụ ("lấy user theo email") thành SQL cụ thể, rồi map kết quả thô về `domain.User`.

Hai ý cốt lõi:

- **Connection pool**: mở một TCP connection tới Postgres tốn ~20–100ms (TCP + auth + TLS) và mỗi connection ngốn 5–10MB RAM server. Mở/đóng mỗi request thì quá đắt. `pgxpool` giữ sẵn một *bể* connection, cho mượn khi cần, trả lại khi xong. Pool *thread-safe* — nhiều goroutine xài chung một `*pgxpool.Pool`.
- **Migration**: schema (bảng/cột/index) cũng là code và phải versioned. `golang-migrate` áp các file `.up.sql` theo thứ tự số tăng dần, ghi version đã chạy vào bảng `schema_migrations`. File `.down.sql` đảo ngược. Nhờ vậy mọi môi trường (dev, CI, prod) hội tụ về cùng schema một cách lặp lại được.

Mệnh đề nền tảng: **tham số không bao giờ là một phần của chuỗi SQL**. Câu SQL được parse trước, rồi `$1, $2…` được gắn giá trị sau — Postgres không bao giờ "diễn giải" dữ liệu người dùng như cú pháp. Đó là gốc rễ chống SQL injection.

## 3. Khái niệm cốt lõi (tăng dần độ khó)

### 3.1 Pool — `ParseConfig` rồi `NewWithConfig`
Cách an toàn nhất: parse DSN (chuỗi kết nối) thành config, tinh chỉnh trong code, rồi tạo pool.

```go
poolCfg, err := pgxpool.ParseConfig(databaseURL) // DSN: postgres://user:pass@host:5432/db?sslmode=disable
// chỉnh poolCfg ở đây nếu cần (MaxConns, Tracer...)
pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
defer pool.Close()
pool.Ping(ctx) // xác nhận kết nối ngay khi khởi động (fail fast)
```

### 3.2 Bốn động từ truy vấn

| Hàm | Trả về | Dùng khi |
|-----|--------|----------|
| `Exec` | `CommandTag` (số dòng ảnh hưởng) | INSERT/UPDATE/DELETE không cần dữ liệu trả về |
| `QueryRow` | `pgx.Row` (scan 1 dòng) | SELECT đúng 1 dòng; lỗi `pgx.ErrNoRows` nếu rỗng |
| `Query` | `pgx.Rows` (lặp nhiều dòng) | SELECT nhiều dòng — phải `defer rows.Close()` + `rows.Err()` |
| `Begin` | `pgx.Tx` | gom nhiều câu vào 1 transaction |

### 3.3 Tham số hoá — `$1, $2`
Placeholder kiểu PostgreSQL là `$N` (không phải `?`). Giá trị truyền tách rời:

```go
const q = `SELECT id, email FROM users WHERE email = $1`
row := pool.QueryRow(ctx, q, email) // email vào đúng vị trí $1, không thể chèn cú pháp
```

### 3.4 Phân biệt lỗi DB
`pgx.ErrNoRows` (không có dòng) khác lỗi vi phạm ràng buộc. Lỗi ràng buộc bóc ra `*pgconn.PgError` rồi đọc `.Code` (mã SQLSTATE chuẩn): `23505` = unique violation, `23503` = foreign-key violation. Repository dịch các mã này thành *domain error* để app không phải biết Postgres.

### 3.5 Transaction & claim nguyên tử
`UPDATE ... RETURNING` trong một câu là nguyên tử: nó vừa đổi trạng thái vừa trả dữ liệu, đảm bảo chỉ một goroutine "thắng". `FOR UPDATE SKIP LOCKED` cho nhiều worker quét cùng bảng mà không khoá nhau.

## 4. LogMon dùng nó thế nào (bám code thật — implemented/planned)

![LogMon Database ERD](../diagrams/db-erd.png)

**Tạo pool (implemented)** — `backend/cmd/userservice/main.go:244-257`: `ParseConfig` → gắn `otelpgx.NewTracer()` vào `poolCfg.ConnConfig.Tracer` (mỗi query thành một trace span) → `NewWithConfig` với `connectCtx` timeout 10s (`_dbConnectTimeout`, dòng 54) → `Ping`. Pool được *truyền xuống* các constructor, không dùng global — đúng khuyến nghị của tác giả pgx.

**Repository pattern (implemented)** — `backend/internal/user/ports/ports.go:13-23` khai báo interface `UserRepository` ở tầng `ports`; `backend/internal/user/adapters/postgres/repository.go:22-32` là implementation. Compile-time check `var _ ports.UserRepository = (*Repository)(nil)` (dòng 27) đảm bảo adapter luôn khớp interface. Mọi query đều parameterized — ví dụ `Save` ở `repository.go:36-48`.

**Dịch lỗi sang domain (implemented)** — `repository.go:41-44`: `errors.As(err, &pgErr)` + so `pgErr.Code == "23505"` (`uniqueViolationCode`, dòng 19) → trả `domain.ErrEmailTaken`. `scanOne` (`repository.go:78-88`) bắt `pgx.ErrNoRows` → `domain.ErrUserNotFound`.

**Reconstruct qua value object (implemented)** — `backend/internal/user/adapters/postgres/reconstruct.go:12-26`: dữ liệu thô từ DB được dựng lại *đi qua* `NewUserID`/`NewEmail`/`NewUser`, nên dữ liệu lưu trữ vẫn được validate khi đọc ra.

**Claim nguyên tử cho refresh-token (implemented)** — `backend/internal/user/adapters/postgres/refresh_repository.go:42-56`: `UPDATE refresh_tokens SET used_at=$2 WHERE token_hash=$1 AND used_at IS NULL AND expires_at>$2 RETURNING ...` đảm bảo chỉ một request rotate được token; `pgx.ErrNoRows` → `ok=false`. Đây là nền của reuse detection (ADR-023).

**Transaction tx-in-context + outbox (implemented)** — `backend/internal/alerting/adapters/postgres/tx.go:37-48`: `TxManager.WithinTx` mở `pool.Begin`, gắn `pgx.Tx` vào `ctx` qua `txKey{}`, `defer tx.Rollback` (no-op sau Commit). `dbFrom` (dòng 51-56) trả tx nếu đang trong TX, ngược lại trả pool — nhờ vậy rule INSERT và outbox INSERT nằm chung một TX. Relay quét pending bằng `FOR UPDATE SKIP LOCKED` ở `backend/internal/shared/outbox/store.go:104-110`.

**Migrations (implemented)** — `backend/migrations/` có 5 cặp `NNNNNN_name.up/down.sql`. `000001_init.up.sql` tạo bảng `users`; `000005_refresh_tokens.up.sql` tạo `refresh_tokens` với FK `ON DELETE CASCADE`. Chạy qua container `migrate/migrate:v4.18.1` (`infra/docker/docker-compose.yml` service `migrate`, one-shot khi `up`), điều khiển bằng `make migrate` / `make migrate-down` (`Makefile:58-62`).

**Lệch implemented vs planned (quan trọng)** — schema *đã code* dùng `users.id TEXT PRIMARY KEY` và `refresh_tokens.token_hash VARCHAR(64)` (hex SHA-256). `doc_v2/08-database-schema.md` lại *đặc tả mục tiêu* `id UUID DEFAULT gen_random_uuid()`, `token_hash BYTEA`, thêm cột `display_name`, `updated_at`, `revoked_at`, và mọi bảng tenant-scoped có `workspace_id`. Những thứ này là **planned**. Về persistence: chỉ `internal/user` và `internal/alerting` có adapter `postgres` thật; `internal/slo` hiện *mới có tầng `domain/`* (chưa có adapter DB), còn `internal/incident`, `internal/notification`, go-redis, k8s manifests **chưa có code** — chỉ là target trong doc_v2/roadmap. `pgx.CopyFrom`/`pgx.Batch`/`RowToStructByName` cũng **chưa dùng** ở repo hiện tại.

## 5. Best practices (mỗi mục kèm 1 nguồn đã research)

1. **Tạo pool tường minh, truyền xuống (không global), parse DSN rồi tinh chỉnh.** LogMon làm đúng ở `main.go:244-257`. Tác giả pgx khuyến nghị tạo pool trực tiếp thay vì trong `init()`. — [jackc/pgx Discussion #1700](https://github.com/jackc/pgx/discussions/1700)
2. **Một `*pgxpool.Pool` dùng chung, thread-safe, cho cả app web.** Đặt `MaxConns`/`MinConns` hợp lý (ví dụ 20/5) theo tài nguyên Postgres. — [pgxpool — pkg.go.dev](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool)
3. **Luôn parameterized query (`$1`), không nối chuỗi.** Tham số tách khỏi SQL = chống injection tận gốc; khớp checklist Security của CLAUDE.md. — [pgx — pkg.go.dev](https://pkg.go.dev/github.com/jackc/pgx/v5)
4. **Phân biệt `pgx.ErrNoRows` bằng `errors.Is`** và dịch sang domain error; `QueryRow` defer lỗi đến `Scan`. — [pgx — pkg.go.dev](https://pkg.go.dev/github.com/jackc/pgx/v5)
5. **Migration luôn có cặp up/down, nhỏ và reversible; commit vào Git.** LogMon tuân thủ ở `backend/migrations/`. — [Better Stack — golang-migrate guide](https://betterstack.com/community/guides/scaling-go/golang-migrate/)
6. **Zero-downtime: cột mới phải nullable hoặc có DEFAULT; index lớn tạo `CONCURRENTLY`; không rename trực tiếp (add→backfill→switch→drop).** Rule này đã ghi trong `doc_v2/08-database-schema.md` (đầu §"Nguyên tắc chung"). — [Atlas blog — golang-migrate error handling](https://atlasgo.io/blog/2025/04/06/atlas-and-golang-migrate)
7. **Repository chỉ chứa data access, biểu đạt ý định nghiệp vụ ("find by email"), inject qua constructor.** — [DEV — Mastering Data Access in Go with Repositories](https://dev.to/greyisheepai/clean-performant-and-testable-mastering-data-access-in-go-with-repositories-sqlc-2m9m)

## 6. Lỗi thường gặp & anti-patterns

- **Nối chuỗi SQL** (`"... WHERE email='" + email + "'"`) → SQL injection. Luôn `$1`.
- **Quên `rows.Close()` / `rows.Err()`** sau `Query` → rò connection khỏi pool, rồi pool cạn và treo. (Outbox `claimPending` làm đúng: `defer rows.Close()` + check `rows.Err()`.)
- **So `err.Error()` bằng chuỗi** thay vì `errors.Is(err, pgx.ErrNoRows)` / `errors.As(&pgErr)` — dễ vỡ khi đổi version.
- **Quên `defer tx.Rollback`** sau `Begin`: nếu return sớm giữa chừng, transaction treo và giữ lock. Rollback sau Commit là no-op nên luôn defer được.
- **Tin context tự rollback transaction**: context chỉ ảnh hưởng câu BEGIN/COMMIT/ROLLBACK, *không* tự rollback khi cancel — phải defer Rollback thủ công. — [pgx docs](https://pkg.go.dev/github.com/jackc/pgx/v5)
- **Pool quá to**: `MaxConns` vượt khả năng Postgres → context-switch và OOM phía server, không nhanh hơn.
- **Sửa file migration đã merge/đã chạy** thay vì thêm migration mới → version đã ghi không khớp nội dung, các môi trường lệch nhau.
- **Đụng "dirty state" rồi force bừa**: migrate đánh dấu dirty khi một migration fail giữa chừng; phải xác định nó áp một phần hay chưa, rồi `force` về đúng version — không đoán. — [golang-migrate guide](https://betterstack.com/community/guides/scaling-go/golang-migrate/)

## 7. Lộ trình luyện tập NGAY trong repo LogMon

### 🥉 Cơ bản
1. Chạy `make db` rồi `psql` vào container, `\d users` và `\d refresh_tokens`, đối chiếu với `000001_init.up.sql` + `000005_refresh_tokens.up.sql`.
2. Thêm method `CountAll(ctx) (int, error)` vào `postgres.Repository` (`repository.go`) dùng `QueryRow("SELECT count(*) FROM users")` + `.Scan(&n)`; thêm vào interface `ports.UserRepository`.
3. Viết migration mới `000006_users_last_login.up/down.sql` thêm cột `last_login_at TIMESTAMPTZ` (nullable — zero-downtime); chạy `make migrate` rồi `make migrate-down` để xác nhận đảo ngược sạch.
4. Cố tình tạo migration lỗi (SQL sai cú pháp), quan sát "dirty state" trong bảng `schema_migrations`, rồi recover bằng `migrate force`.

### 🥈 Trung cấp
1. Thêm `ByID` cho `RefreshRepository` (`refresh_repository.go`) trả `ErrRefreshTokenInvalid` khi `pgx.ErrNoRows`, theo đúng pattern `scanRefresh`.
2. Viết integration test (`//go:build integration`) cho method mới, theo khuôn `refresh_integration_test.go` (TRUNCATE + seed), chạy `make test-integration`.
3. Bóc lỗi FK: cố Insert refresh_token với `user_id` không tồn tại, `errors.As(&pgErr)` so `pgErr.Code == "23503"`, dịch thành một domain error mới.
4. Thêm `MaxConns`/`MinConns`/`MaxConnLifetime` vào `poolCfg` trong `main.go` (sau `ParseConfig`, dòng 244) và đọc từ env trong `loadConfig`.

### 🥇 Nâng cao
1. Viết một repository cho alerting dùng `TxManager.WithinTx` + `dbFrom` (theo `tx.go`) để 2 INSERT (rule + outbox) cùng commit/rollback; thêm integration test xác nhận rollback khi câu thứ 2 fail.
2. Thêm `RowToStructByName` (pgx v5) hoặc `pgx.CopyFrom` cho một batch insert (hiện repo chưa dùng) và benchmark so với loop `Exec`.
3. Viết migration nâng cấp `refresh_tokens` về hướng doc_v2/08 (`token_hash` → `BYTEA`, thêm `revoked_at`) theo pattern add→backfill→switch, kèm down migration; cập nhật `scanRefresh` tương ứng.
4. Mô phỏng pool exhaustion: tạm bỏ `defer rows.Close()` ở một chỗ, viết test bắn 50 query đồng thời và quan sát pool treo, rồi sửa lại.

## 8. Skill/agent ECC nên dùng khi luyện

- **`ecc:postgres-patterns`** — khi thiết kế query, index, transaction, hoặc tối ưu pgxpool: gợi ý pattern Postgres đúng (partial index, SKIP LOCKED, JSONB) trước khi code adapter.
- **`ecc:database-migrations`** — khi viết/đảo file migration: kiểm tra cặp up/down có thực sự reversible, có an toàn zero-downtime (nullable/DEFAULT, CONCURRENTLY), tránh dirty-state.
- **`ecc:database-reviewer`** *(nếu có trong bản cài)* — review sau khi viết adapter/migration: soi SQL injection, N+1, thiếu index, leak connection. Dùng *sau* khi code xong, trước commit, song song `ecc:go-review`.

Quy trình gợi ý: `ecc:postgres-patterns` (thiết kế) → code theo TDD (`ecc:go-test`) → `ecc:database-migrations` (nếu có migration) → `ecc:database-reviewer` + `ecc:go-review` (review) → commit.

## 9. Tài nguyên học thêm (link đã research)

- [pgx — pkg.go.dev](https://pkg.go.dev/github.com/jackc/pgx/v5) — API chính thức: QueryRow/Scan, ErrNoRows, transaction, statement cache.
- [pgxpool — pkg.go.dev](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool) — cấu hình pool, hooks BeforeConnect/AfterConnect, thread-safety.
- [jackc/pgx Discussion #1700](https://github.com/jackc/pgx/discussions/1700) — best practice long-running pool, vì sao tránh global/init.
- [Better Stack — Database migrations in Go with golang-migrate](https://betterstack.com/community/guides/scaling-go/golang-migrate/) — naming, up/down, dirty state, force.
- [Atlas blog — Handling Migration Errors vs golang-migrate (2025)](https://atlasgo.io/blog/2025/04/06/atlas-and-golang-migrate) — hạn chế golang-migrate và cách xử lý lỗi migration.
- [DEV — Mastering Data Access in Go with Repositories & sqlc](https://dev.to/greyisheepai/clean-performant-and-testable-mastering-data-access-in-go-with-repositories-sqlc-2m9m) — repository pattern: abstraction, intent-driven, DI.

## 10. Checklist "đã hiểu"

- [ ] Giải thích được vì sao `$1, $2` chống SQL injection (parse SQL trước, gắn tham số sau).
- [ ] Biết khi nào dùng `Exec` vs `QueryRow` vs `Query` vs `Begin`, và phải `defer rows.Close()` + check `rows.Err()`.
- [ ] Phân biệt `pgx.ErrNoRows` (qua `errors.Is`) với vi phạm ràng buộc (qua `errors.As(*pgconn.PgError)` + mã `23505`/`23503`), và dịch sang domain error.
- [ ] Hiểu pattern tx-in-context (`WithinTx` + `dbFrom`) cho phép nhiều adapter ghi chung một transaction, và vì sao `defer tx.Rollback` luôn an toàn.
- [ ] Giải thích claim nguyên tử bằng `UPDATE ... RETURNING` và `FOR UPDATE SKIP LOCKED` an toàn đồng thời như thế nào.
- [ ] Viết được cặp migration up/down reversible, biết zero-downtime rule và cách thoát dirty state bằng `force`.
- [ ] Chỉ ra được layer nào được import `pgx` (chỉ `adapters/postgres`) và vì sao `domain`/`app` không được.
- [ ] Phân biệt được schema *đã implement* (`users.id TEXT`, `token_hash VARCHAR(64)`) với schema *target* trong doc_v2/08 (`UUID`, `BYTEA`, `workspace_id`).
