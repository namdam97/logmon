# Config & Secrets (12-factor) trong LogMon
> Module DSO-2 · env config, validate at startup, secret manager, fail-fast · Độ khó: 🥉→🥇 · Prereqs: BE-1

---

## 1. Vì sao kỹ năng này quan trọng trong LogMon

LogMon là nền tảng observability cho Go microservices — nó *cầm* những thứ nhạy cảm nhất của hệ thống: chuỗi kết nối Postgres, `JWT_SECRET` ký token đăng nhập, mật khẩu Elasticsearch (nơi chứa toàn bộ log production), token webhook giữa Alertmanager và service, và (ở GĐ3) cả webhook URL Slack/PagerDuty của khách. Một secret rò rỉ ở đây không chỉ hỏng một service — nó mở toang kho log + khả năng giả mạo alert.

Đồng thời, cùng một binary `userservice` phải chạy ở rất nhiều ngữ cảnh: máy dev (`make up`), CI E2E (`make e2e`), stack đầy đủ (`make up-full`), và production K8s — *không đổi một dòng code*. Cách duy nhất làm được điều đó sạch sẽ là tách cấu hình ra khỏi mã nguồn và nạp qua môi trường. Đây chính là Factor III của 12-factor app. Nếu cấu hình bị hardcode, mỗi môi trường lại cần một bản build riêng — và bí mật sẽ nằm chình ình trong git history.

Kỹ năng này quyết định ba thứ sống còn: (1) bí mật không bao giờ lọt vào git, (2) service *chết ngay lúc khởi động* nếu thiếu config bắt buộc thay vì lỗi mơ hồ lúc 3 giờ sáng, và (3) cùng artifact dùng lại được ở mọi môi trường.

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Bắt đầu từ một câu hỏi: *cái gì thay đổi giữa các lần deploy?* Mã nguồn thì **không** — bạn build một lần, chạy ở dev/staging/prod đều cùng binary. Nhưng địa chỉ database, mật khẩu, secret ký token thì **có** thay đổi. 12-factor gọi nhóm "thay đổi theo deploy" này là *config*, và nguyên tắc là: **tách config ra khỏi code, lưu trong môi trường** (biến môi trường — OS-agnostic, không cần file đặc thù ngôn ngữ).

Phép thử đơn giản của 12factor.net: *"Nếu repo bị public ngay lúc này, có credential nào lộ không?"* Nếu có → bạn đã trộn config vào code.

Tiếp theo, phân biệt hai loại:

- **Config thường** (PORT, LOG_LEVEL, PROMETHEUS_URL): không nhạy cảm, có thể có default hợp lý.
- **Secret** (JWT_SECRET, DB password, webhook token): nhạy cảm. 12-factor *không* nói cách lưu secret — đó là khoảng trống mà OWASP lấp: secret cần được đối xử đặc biệt (không commit, có thể rotate, có thể thu hồi mà không cần deploy lại code).

Nguyên tắc thứ ba là **fail-fast**: nếu lúc khởi động thiếu config bắt buộc, ứng dụng *không thể* chạy đúng, nên đừng cố chạy. Hãy validate ngay "dòng số 1", in lỗi rõ ràng, rồi `exit(1)`. Lỗi lúc boot rẻ hơn lỗi lúc runtime gấp trăm lần.

Mô hình tinh thần cuối: hình dung một cái phễu khởi động. Đầu phễu là `loadConfig()` đọc env → bộ lọc `validate` chặn nếu thiếu → chỉ khi qua được mới dựng DB pool, JWT service, HTTP server. Code sau bộ lọc *luôn* được phép giả định config đã hợp lệ.

## 3. Khái niệm cốt lõi (tăng dần)

1. **Biến môi trường** — cặp key=value OS cung cấp cho process. Trong Go: `os.Getenv("KEY")` (rỗng nếu không có) và `os.LookupEnv("KEY")` (trả thêm `bool` để phân biệt "không set" với "set rỗng").
2. **Config struct tập trung** — gom mọi giá trị vào *một* struct, nạp *một* chỗ. Phần còn lại của app chỉ nhận struct đó, không gọi `os.Getenv` rải rác. Tăng tính khám phá (đọc struct là biết app cần gì).
3. **Default an toàn vs bắt buộc** — config thường có fallback (`envOr("PORT", "8080")`); secret bắt buộc thì *không* fallback ngầm (thiếu = lỗi).
4. **Validate at startup (fail-fast)** — kiểm tra giá trị bắt buộc + ép kiểu (string→int/duration/bool) ngay khi load; gom hết lỗi báo một lần thay vì fail từng cái.
5. **`.env` file** — tiện cho dev: tập trung biến cho `docker compose` đọc. **KHÔNG commit** — chỉ commit `.env.example` làm template.
6. **Secret manager / file-based secrets** — production không dùng env trần cho secret (env lộ qua `/proc`, logs, crash dump). Dùng Docker Compose `secrets:` (file mount, không cần Swarm) hoặc Vault/AWS Secrets Manager với rotation + audit.
7. **Encryption at-rest** — secret lưu trong DB (vd webhook URL khách) phải mã hóa (AES-256-GCM), key lấy từ env, có key-id để rotate dần.

## 4. LogMon dùng/sẽ dùng nó thế nào (bám doc_v2 + code; ghi rõ implemented/planned)

**Đã implemented:**

- **Config struct tập trung + fail-fast.** `backend/cmd/userservice/main.go` có `type config struct` gom toàn bộ (`databaseURL`, `jwtSecret`, `csrfSecret`, `esPassword`...), nạp qua `loadConfig()`, và `run()` validate ngay: `if cfg.databaseURL == "" { return errors.New("DATABASE_URL not configured") }`. `NewJWTService` từ chối secret rỗng (`"jwt secret must not be empty"`). `main()` chỉ gọi `run()` rồi `os.Exit(1)` — đúng pattern fail-fast của doc_v2/09 §5 ("`config.Load()` fail-fast khi thiếu secret bắt buộc").
- **Helper ép kiểu an toàn.** `envOr(key, fallback string)` cho string có default, `envIntOr(key, fallback int)` parse int và reject giá trị ≤0 — đúng nguyên tắc "đừng truyền string khi cần int/duration".
- **Phân tầng default/bắt buộc.** Config thường có default (`PORT`→8080, `LOG_LEVEL`→info, `PROMETHEUS_URL`→`http://prometheus:9090`); secret thì `os.Getenv` trần không default. Một số tính năng *optional fail-soft*: `ELASTICSEARCH_URL` rỗng → tắt `/api/v1/logs` (log rõ ràng); `ALERTMANAGER_WEBHOOK_TOKEN` rỗng → webhook *fail-closed* (mọi POST trả 401) chứ không bỏ qua xác thực.
- **Domain separation cho secret.** `csrfSecret(cfg)` ưu tiên `CSRF_SECRET`, nếu rỗng thì *derive* qua HMAC của `jwtSecret` với nhãn miền `logmon-csrf-v1` — tách miền ký CSRF khỏi ký JWT mà không bắt buộc thêm một secret.
- **`.env` không commit.** Root `.env.example` là template (`JWT_SECRET=local-dev-insecure-change-me`, kèm chú thích "Production nạp qua secret manager"); `.gitignore` chặn `.env` và `.env.*` nhưng giữ `!.env.example`.
- **File-based secrets cho compose.** `infra/docker/secrets/` chứa `logmon_webhook_token.txt`, `slack_webhook_url.txt`... với `.gitignore` chỉ cho phép `*.example` (`*` bị ignore, `!*.example` whitelist). Compose có khối `secrets:` top-level (dòng 402) và service `elasticsearch` dùng `secrets: [elastic_password]` — đúng doc_v2/10 §5 ("Compose `secrets:` file-based cho credentials").
- **AES-256-GCM at-rest.** `backend/internal/shared/crypto/cipher.go` đã có `Cipher` mã hóa định dạng `<keyID>:<base64(nonce|ct|tag)>`, `KeyFromPassphrase` derive key từ env passphrase, prefix key-id cho phép rotate.

**Planned (đích thiết kế chính thức doc_v2):**

- doc_v2/09 §5 đặt mục tiêu: secret app (DB password, JWT key) ưu tiên **Compose `secrets:` file-based cho production** thay vì env trần; JWT secret "≥ 32 bytes random từ secrets manager/env; có `kid` để rotate key không downtime".
- **Notification channel secrets mã hóa AES-256-GCM** với `LOGMON_ENCRYPTION_KEY` từ env, nonce riêng mỗi record — đây là mục **GĐ3.2 trong roadmap** (doc_v2/12): `notification/` BC + secrets mã hóa AES-GCM. Hiện `internal/notification/` mới có lớp `domain/` (channel/message/events), chưa wire encryption vào adapter — `Cipher` đã sẵn sàng để cắm vào.
- doc_v2/10 §6: tách `docker-compose.prod.yml` (TLS, limits) khỏi base (ADR-040); rotate secrets định kỳ hàng quý (restore drill); gitleaks scan secret trong CI pipeline.

## 5. Best practices (mỗi mục kèm 1 nguồn đã research)

1. **Lưu config trong môi trường, không trong code.** Mỗi biến độc lập, không gom thành "environments" cứng. ([12factor.net/config](https://12factor.net/config))
2. **Đối xử secret khác config thường — không dùng env trần nếu tránh được.** Env var truy cập được bởi mọi process con, lọt vào log/crash dump; ưu tiên secret manager hoặc file mount. ([OWASP Secrets Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html))
3. **Validate ngay lúc startup, fail-fast, gom lỗi báo một lần.** App không nạp được config thì không nên chạy tiếp — terminate với message rõ ràng. ([Alex Edwards — Managing Configuration in Go](https://www.alexedwards.net/blog/how-to-manage-configuration-settings-in-go-web-applications))
4. **Ép kiểu tường minh + phân biệt unset vs empty.** Dùng `os.LookupEnv` cho biến bắt buộc, helper parse int/bool/duration. ([reintech — Environment Variables in Go](https://reintech.io/blog/working-with-environment-variables-in-go))
5. **Secret phải rotate được, thu hồi được, không cần deploy lại code; có audit.** ([OWASP Secrets Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html))
6. **Mã hóa secret at-rest đúng chuẩn (AES-256-GCM, nonce/record).** ([OWASP Cryptographic Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html))

## 6. Lỗi thường gặp & anti-patterns

- **Hardcode secret trong code/Dockerfile.** `ENV JWT_SECRET=...` hay literal trong Go đều rò qua image/git. OWASP: không dùng `docker ENV`/`ARG` cho secret. Trong LogMon, literal `local-dev-insecure-change-me` *chỉ* là fallback dev có chú thích — production phải override.
- **`os.Getenv` rải rác khắp nơi.** Khó biết app cần gì, khó test. Chữa: một struct, một `loadConfig()`.
- **Default ngầm cho secret bắt buộc.** `envOr("JWT_SECRET", "dev")` ở production = thảm họa thầm lặng. Secret bắt buộc phải fail nếu rỗng (như `NewJWTService` đang làm).
- **Commit `.env`.** Một lần `git add .env` là secret nằm vĩnh viễn trong history. LogMon chặn bằng `.gitignore` + chỉ commit `.example`.
- **Bỏ qua lỗi ép kiểu.** `strconv.Atoi` lỗi mà nuốt lặng → giá trị 0 bất ngờ. `envIntOr` reject ≤0 và fallback rõ ràng.
- **Fail-open khi thiếu secret bảo mật.** Thiếu webhook token mà *vẫn nhận* webhook = lỗ hổng. LogMon chọn *fail-closed* (trả 401).
- **Log cả config struct.** `log.Info(cfg)` in luôn cả secret. Không bao giờ log secret; chỉ log "đã set / chưa set".
- **In-memory state nhầm là phân tán.** Rate limiter của LogMon là `x/time/rate` in-memory, single-instance — không phải Redis; đừng nhầm config của nó là chia sẻ giữa các pod (multi-instance prod cần store tập trung — đã ghi chú trong `ratelimit.go`).

## 7. Lộ trình luyện tập NGAY trong repo LogMon (🥉→🥈→🥇)

**🥉 Cấp đồng — đọc & chạy:**
1. Đọc `backend/cmd/userservice/main.go` từ `type config struct` → `loadConfig()` → đoạn validate trong `run()`. Liệt kê: biến nào có default, biến nào bắt buộc, biến nào optional fail-soft.
2. Chạy `JWT_SECRET="" DATABASE_URL=... go run ./cmd/userservice` và quan sát thông báo fail-fast. Sau đó bỏ `DATABASE_URL` để thấy lỗi đổi.
3. So `.env.example` với khối `environment:` của `userservice` trong `infra/docker/docker-compose.yml` — đối chiếu từng biến.

**🥈 Cấp bạc — thêm config có validate:**
4. Thêm một config mới (vd `HTTP_READ_TIMEOUT` dạng `time.Duration`): thêm field vào struct, viết helper `envDurationOr(key string, fallback time.Duration)` (mô phỏng `envIntOr`), wire vào `http.Server`. Viết table-driven test cho helper (`give`/`want`, dùng `testify/require`).
5. Refactor validate trong `run()` để **gom mọi lỗi config bắt buộc** vào một `[]string` rồi báo một lần (thay vì return ở lỗi đầu tiên) — đúng best practice #3.

**🥇 Cấp vàng — secret manager hardening:**
6. Theo doc_v2/09 §5: chuyển `JWT_SECRET` của `userservice` trong compose sang **file-based secret** (thêm vào khối `secrets:`, đọc qua `/run/secrets/jwt_secret`), thêm helper `secretFromFileOrEnv(key string)` ưu tiên đường dẫn `*_FILE`. Cập nhật `.example`.
7. Cắm `crypto.Cipher` vào `notification/` (GĐ3.2): mã hóa webhook URL trước khi lưu DB, key từ `LOGMON_ENCRYPTION_KEY`, fail-fast nếu thiếu khi notification bật. Test round-trip encrypt/decrypt + rotate key-id.

## 8. Skill/agent ECC nên dùng

- **`ecc:security-reviewer`** — quét hardcoded secret, fail-open, secret bị log. Chạy *trước mọi commit* đụng config/auth (đúng security checklist của project). Có thể gọi qua `/cso` cho rà soát định kỳ.
- **`ecc:deployment-patterns`** — chuẩn hóa `.env` vs `env_file` vs Compose `secrets:`, tách base/prod override (ADR-040), gitleaks trong CI.
- **`ecc:go-review`** (go-reviewer) — review helper ép kiểu, pattern `run()`/fail-fast, xử lý lỗi `os.LookupEnv`, đảm bảo không nuốt lỗi `strconv`.
- Bổ trợ: **`ecc:golang-patterns`** (functional options cho service config), **`ecc:go-test`** (TDD cho helper config, coverage ≥80%).

## 9. Tài nguyên học thêm

- [The Twelve-Factor App — Factor III: Config](https://12factor.net/config) — bản gốc, đọc trước tiên.
- [OWASP Secrets Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html) — env vs secret manager, sidecar, rotation, audit.
- [OWASP Cryptographic Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html) — mã hóa secret at-rest (nền cho `crypto.Cipher`).
- [Alex Edwards — How to Manage Configuration in Go Web Apps](https://www.alexedwards.net/blog/how-to-manage-configuration-settings-in-go-web-applications) — config struct tập trung + fail-fast.
- [Working with Environment Variables in Go (reintech)](https://reintech.io/blog/working-with-environment-variables-in-go) — `Getenv` vs `LookupEnv`, ép kiểu.
- Nội bộ: `doc_v2/09-security.md` §5 (Secrets Management), `doc_v2/10-deployment-operations.md` §5–6 (Compose secrets, rotation), `CLAUDE.md` mục Security.

## 10. Checklist "đã hiểu"

- [ ] Giải thích được Factor III và phép thử "repo public → có lộ secret không?"
- [ ] Phân biệt config thường (có default) vs secret bắt buộc (fail nếu rỗng) và cho ví dụ từ `config struct` của LogMon.
- [ ] Biết vì sao env trần *không* lý tưởng cho secret và doc_v2 hướng tới Compose `secrets:` / secret manager.
- [ ] Chỉ ra fail-fast trong `run()` và lý do `main()` chỉ gọi `run()` + `os.Exit(1)`.
- [ ] Phân biệt `os.Getenv` vs `os.LookupEnv`; biết khi nào dùng cái nào.
- [ ] Hiểu fail-closed (webhook 401) khác fail-soft (tắt `/logs`) và khi nào chọn cái nào.
- [ ] Giải thích `.gitignore` chặn `.env` nhưng giữ `.env.example` và `infra/docker/secrets/`.
- [ ] Biết `crypto.Cipher` (AES-256-GCM, key-id) dùng cho secret at-rest và đó là mục GĐ3.2 cho `notification/`.
- [ ] Viết được helper ép kiểu mới có test (theo bài tập 🥈).
- [ ] Biết chạy `ecc:security-reviewer` trước commit đụng config/auth.
