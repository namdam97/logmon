# Identity, RBAC & Multi-tenancy trong LogMon
> Module SEC-2 · workspace · role · permission · tenant isolation · Độ khó: 🥉→🥇 · Prereqs: SEC-1, ARCH-1

## 1. Vì sao kỹ năng này quan trọng trong LogMon

LogMon là nền tảng observability **dùng chung** (multi-tenant): nhiều team/khách hàng cùng đẩy logs, metrics, traces, alert rules, SLO vào một cụm hạ tầng. Câu hỏi sống còn không phải "user này có đăng nhập không" (đó là authentication — SEC-1), mà là: **"user này được phép thấy/sửa cái gì, và tuyệt đối không được chạm vào dữ liệu của tenant khác."**

Hỏng phần này thì hậu quả nặng hơn hầu hết lỗi khác:
- **Cross-tenant leak** — team A đọc được log/metrics của team B. Với observability, log thường chứa PII, secret rò rỉ, hostname nội bộ → đây là sự cố compliance (GDPR/SOC2), không chỉ là bug.
- **Privilege escalation** — một `viewer` xoá được alert rule hoặc đổi pipeline mode.
- **Broken access control** đứng **A01 trong OWASP Top 10:2021** — hạng mục bị khai thác nhiều nhất.

Trong LogMon, "ai-thấy-gì" được neo vào khái niệm **workspace** (đơn vị tenant). Mọi resource có business — alert rule, SLO, incident, log stream — đều thuộc về đúng một workspace, và RBAC quyết định trong workspace đó user làm được gì.

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Tách bạch 3 câu hỏi, đừng trộn lẫn:

1. **Authentication (AuthN) — "Bạn là ai?"** Xác minh danh tính. LogMon: JWT trong cookie `logmon_token`, subject = `userID` (đã implemented, xem SEC-1).
2. **Authorization (AuthZ) — "Bạn được làm gì?"** Sau khi biết danh tính, quyết định cho phép thao tác hay không. Đây là RBAC.
3. **Tenant isolation — "Bạn thuộc về vùng dữ liệu nào?"** Một biên giới *dữ liệu*, độc lập với role. Một `admin` của workspace A vẫn **không** được thấy dữ liệu workspace B.

Điểm dễ sai nhất của người mới: tưởng "isolation chỉ là một dạng quyền". Không. Hãy hình dung **hai trục vuông góc**:

```
            quyền cao  ┆
   (role)   admin ─────┼──────  một admin của WS-A
            editor     ┆        KHÔNG được leo sang WS-B
            viewer ────┼──────
                       ┆
            WS-A      WS-B   WS-C   ← biên giới tenant (workspace)
```

Role di chuyển user lên/xuống theo trục dọc *trong một workspace*. Tenant isolation là vách ngăn dọc giữa các cột — không role nào (trừ `platform_admin`) được xuyên qua. Hệ quả thiết kế: **mọi truy vấn dữ liệu phải mang theo `workspace_id`**, và điều đó phải được *ép buộc bởi tầng dưới*, không phụ thuộc lập trình viên nhớ thêm `WHERE`.

Nguyên tắc nền: **least privilege** (mặc định từ chối, chỉ cấp đúng quyền cần) + **defense in depth** (chặn ở nhiều tầng: middleware + app + DB) + **fail closed** (nghi ngờ thì từ chối).

## 3. Khái niệm cốt lõi (tăng dần)

- **Identity / User** — chủ thể được xác thực. Trong LogMon là aggregate `User` (`backend/internal/user/domain/user.go`): hiện chỉ có `id`, `email`, `passwordHash`, `createdAt`. Chưa có role/workspace — đúng như thiết kế GĐ hiện tại.
- **Tenant / Workspace** — đơn vị cô lập dữ liệu. doc_v2 chọn từ "workspace" làm tenant. Mỗi workspace có `slug` (dùng làm namespace cho ES data stream).
- **Membership** — quan hệ nhiều-nhiều giữa user và workspace, *mang theo role*. Một user có thể là `admin` ở WS-A và `viewer` ở WS-B.
- **Role** — nhóm quyền đặt tên sẵn. LogMon dùng 4 role phân cấp: `viewer` < `editor` < `admin` < `platform_admin`.
- **Permission** — hành động cụ thể trên resource (`alert_rule.create`, `pipeline.set_mode`). RBAC ánh xạ role → tập permission.
- **RBAC vs ABAC** — RBAC gán quyền theo *vai trò*; ABAC quyết định theo *thuộc tính* (resource, môi trường, thời gian). LogMon dùng **RBAC làm xương sống**, cộng một thuộc tính tenant (`workspace_id`) — tức một mô hình hybrid RBAC + tenant-scoping nhẹ, đúng khuyến nghị của AWS cho SaaS multi-tenant.
- **PEP/PDP** — Policy Enforcement Point (nơi *chặn*, vd middleware) tách khỏi Policy Decision Point (nơi *quyết định*). Tách 2 thứ giúp policy nhất quán và test được.
- **Tenant isolation models** — (a) database-per-tenant, (b) schema-per-tenant, (c) **shared schema + cột `tenant_id`** (rẻ, scale tốt). LogMon chọn (c) cho Postgres, cộng data-stream-per-workspace cho Elasticsearch.

## 4. LogMon dùng/sẽ dùng nó thế nào (bám doc_v2 + code)

**Đã implemented (đọc thấy trong code):**
- AuthN nền: `shared/auth` phát hành/parse JWT HS256, subject = `userID`; `RequireAuth` middleware gắn `userID` vào `gin.Context` qua `UserIDFromContext` (`backend/internal/shared/auth/middleware.go`).
- Một số handler đã *mang theo* `workspaceID`, nhưng đó là **workspace mặc định static** tiêm lúc khởi tạo, chưa suy ra từ membership của user. Ví dụ `InstanceHandler` nhận `workspaceID string` trong constructor và gán cứng vào mỗi command (`backend/internal/alerting/adapters/http/instance_handler.go`); SLO handler tương tự. Đây đúng là trạng thái "GĐ2 dùng workspace mặc định" mà doc_v2 mô tả.
- Bảng `users` (`backend/migrations/000001_init.up.sql`) chỉ có `id/email/password_hash/created_at`. **Chưa có** `workspaces`, `workspace_members`, `audit_logs`.
- Rate limit: in-memory token bucket theo IP (`golang.org/x/time/rate`) ở `shared/middleware/ratelimit.go` — comment trong code ghi rõ "single-instance; multi-instance prod nên dùng Redis". Per-workspace rate limit là việc của GĐ3.

**Planned — đích thiết kế chính thức của doc_v2 (GĐ3, task 3.6 trong `doc_v2/12-roadmap.md`):**

> ADR-029: *"`internal/user/` tiến hóa thành `internal/identity/` (auth + workspaces + RBAC — BC platform thật)."*

- **Schema multi-tenancy** (`doc_v2/08-database-schema.md`, mục GĐ3):
  ```sql
  CREATE TABLE workspaces (
      id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      name VARCHAR(100) NOT NULL,
      slug VARCHAR(100) NOT NULL UNIQUE,   -- namespace ES data stream
      ...
  );
  CREATE TABLE workspace_members (
      workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
      user_id      UUID NOT NULL REFERENCES users(id)      ON DELETE CASCADE,
      role         VARCHAR(20) NOT NULL DEFAULT 'viewer',  -- viewer|editor|admin|platform_admin
      PRIMARY KEY (workspace_id, user_id)
  );
  ```
- **Ma trận RBAC** (`doc_v2/09-security.md §2`): `viewer` (read + log search) → `editor` (+ CRUD rules/SLO/incident, ack/silence) → `admin` (+ members, channels, pipeline mode/ILM, oncall) → `platform_admin` (mọi workspace + tạo/xoá workspace).
- **Role KHÔNG nhét vào JWT** (`doc_v2/09-security.md §1.2`): access claims chỉ là `sub/iss/aud/exp/iat`. Lý do trích nguyên văn: *"role đổi phải có hiệu lực ngay — đọc từ DB/cache."* Đây là quyết định kiến trúc chính, không phải thiếu sót.
- **Enforcement 2 tầng** (defense in depth): middleware `rbac` chặn theo min-role của route **và** app layer kiểm lại cho thao tác nhạy cảm.
- **Repo enforced workspace filter**: *"Mọi query repository bắt buộc có `workspace_id` filter — constructor repo nhận workspace từ context, không có đường đi nào bỏ qua."* Resource khác workspace trả **404** (không phải 403 — tránh lộ tồn tại).
- **Isolation theo từng store** (`doc_v2/09-security.md §3`): Postgres = cột `workspace_id` + composite index (RLS cân nhắc làm tầng 2 ở GĐ4); Elasticsearch = data-stream-per-workspace `logs-{service}-{slug}`, backend tự build query DSL, không forward DSL thô; Prometheus/Jaeger = inject matcher `{workspace="..."}` kiểu prom-label-proxy (label tự khai **không** phải biên giới cứng — tenancy cứng để GĐ4+ với Mimir/Thanos); Redis = key prefix `ws:{id}:`.
- **Audit logs** immutable cho admin actions + authz failures (bảng `audit_logs`).

## 5. Best practices (mỗi mục kèm 1 nguồn đã research)

- **Đừng nhét quyền vào JWT cho hệ nội bộ nhạy cảm; đọc từ DB rồi cache.** Latency lookup chỉ ~5–10ms — không đáng kể so với rủi ro không revoke được khi đổi role. LogMon theo đúng hướng này. ([OneUptime — JWT Revocation](https://oneuptime.com/blog/post/2026-02-02-jwt-revocation/view))
- **Mặc định từ chối (deny by default), enforce server-side mọi request.** Không tin client; ép kiểm soát ở tầng tin cậy. ([OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html))
- **Dùng RBAC làm nền, thêm thuộc tính tenant (hybrid).** RBAC thuần khó scale multi-tenant; gắn `tenant_id` + role có scope vào mỗi quyết định. ([AWS Prescriptive Guidance — Multi-tenant RBAC+ABAC](https://docs.aws.amazon.com/prescriptive-guidance/latest/saas-multitenant-api-access-authorization/avp-mt-abac-rbac-examples.html))
- **Coi RLS của Postgres là tầng chặn cuối ở DB** — kể cả khi app quên `WHERE`, DB vẫn không rò chéo tenant. doc_v2 xếp việc này vào GĐ4 như defense-in-depth. ([AWS — Multi-tenant data isolation with PostgreSQL RLS](https://aws.amazon.com/blogs/database/multi-tenant-data-isolation-with-postgresql-row-level-security/))
- **Chọn shared-schema + cột tenant cho chi phí/scale, nhưng test isolation tự động.** Rẻ và scale, đổi lại phải kiểm thử kỹ policy/filter. ([PlanetScale — Approaches to tenancy in Postgres](https://planetscale.com/blog/approaches-to-tenancy-in-postgres))
- **Chọn đúng model AuthZ ngay từ đầu cho SaaS multi-tenant** — embed tenant id + scoped role, validate mỗi request. ([Auth0 — Authorization model for multi-tenant SaaS](https://auth0.com/blog/how-to-choose-the-right-authorization-model-for-your-multi-tenant-saas-application/))

## 6. Lỗi thường gặp & anti-patterns

- **Quên `workspace_id` trong một query duy nhất** → leak. Phòng bằng *kiến trúc*: repo nhận workspace từ context trong constructor, không nhận qua tham số tuỳ ý mỗi method (đúng như doc_v2 yêu cầu) — bịt đường đi sai.
- **Trả 403 cho resource khác workspace** → lộ "resource này tồn tại". LogMon quy định trả **404**.
- **Tin `workspace_id` do client gửi trong body/param.** Phải suy từ membership của user đã xác thực, không lấy từ input.
- **Nhét role vào JWT 15 phút** → đổi/revoke role không có hiệu lực tới khi token hết hạn; với hệ nội bộ là rủi ro compliance.
- **Chỉ chặn ở UI/middleware** mà app layer không kiểm lại → bypass bằng cách gọi API trực tiếp. Cần defense in depth.
- **Coi label `workspace` trên Prometheus là biên giới bảo mật.** Nó tự khai từ service — doc_v2 nói rõ "không phải security boundary thật"; backend phải inject matcher.
- **Forward JSON ES query DSL thô từ client** → client tự đổi index/namespace, vượt tenant. Backend phải build DSL từ struct.
- **Role explosion**: đẻ role riêng cho từng khách hàng → bùng nổ tổ hợp. Giữ tập role nhỏ cố định + scope theo workspace.
- **SSRF qua generic webhook channel** (GĐ3): không validate scheme/chặn private IP → kẻ tấn công dùng LogMon làm proxy nội mạng.

## 7. Lộ trình luyện tập NGAY trong repo LogMon (🥉→🥈→🥇)

**🥉 Beginner — đọc & vạch đường dữ liệu**
1. Đọc `backend/internal/shared/auth/middleware.go`: `RequireAuth` lấy `userID` từ đâu và đặt vào context bằng key nào?
2. Grep `workspaceID` trong `backend/internal/alerting/adapters/http/` và `slo/adapters/http/`: xác nhận nó là *static default* tiêm lúc khởi tạo, chưa suy từ user.
3. Đối chiếu bảng `users` (`migrations/000001_init.up.sql`) với DDL `workspaces`/`workspace_members` trong `doc_v2/08-database-schema.md`. Liệt kê chính xác cột nào còn thiếu để lên GĐ3.

**🥈 Intermediate — viết migration & middleware (TDD)**
4. Viết migration `000007_workspaces.up.sql` + `.down.sql` đúng DDL doc_v2 (workspaces, workspace_members, audit_logs) — chạy `make migrate` rồi `make migrate-down` để xác minh round-trip.
5. TDD một middleware `rbac.RequireRole(min Role)` trong `shared/auth`: viết bảng test (`viewer` gọi route `editor` → 403; `admin` → pass) **trước**, rồi implement. Định nghĩa `Role` là enum bắt đầu từ `iota+1` (đúng Go style guide của repo), so sánh phân cấp.
6. TDD một `WorkspaceFromContext` + middleware resolve workspace của user (đọc membership) và gắn vào context; test rằng thiếu membership → 404.

**🥇 Advanced — isolation end-to-end**
7. Refactor một repo (vd SLO postgres) để nhận `workspaceID` từ context trong constructor và *bắt buộc* xuất hiện trong mọi câu lệnh — viết test chứng minh không có method nào query thiếu filter.
8. Viết **integration test isolation** (chạy với Postgres thật qua `make up`): seed 2 workspace, tạo SLO ở mỗi cái, assert user WS-A gọi API/list **không bao giờ** thấy SLO của WS-B (đáp ứng DoD GĐ3: *"User workspace A không thấy bất kỳ data nào của workspace B"*).
9. Thêm ghi `audit_logs` cho admin actions + authz failures, và xác nhận bảng immutable (không UPDATE/DELETE). Mở rộng nhẹ: phác thảo RLS policy Postgres dùng `current_setting('app.workspace_id')` như tầng chặn DB (đích GĐ4).

## 8. Skill/agent ECC nên dùng

- **`ecc:architect`** — trước khi code GĐ3: chốt ranh giới `internal/identity/` (rename từ `user/` theo ADR-029), nơi đặt `Role` enum (shared kernel vs identity BC), và hợp đồng "repo nhận workspace từ context".
- **`ecc:security-reviewer`** (hoặc skill `/cso`) — bắt buộc cho code AuthZ/tenant: soi broken access control (OWASP A01), 404-vs-403, SSRF webhook, tin-tưởng-input. Trigger ngay khi đụng auth/tenant.
- **`ecc:database-reviewer`** (hoặc `ecc:postgres-patterns`) — review migration workspaces/members: composite index `(workspace_id, ...)`, FK `ON DELETE CASCADE`, và thiết kế RLS policy cho GĐ4.
- **`ecc:go-review` + `ecc:go-test`** — enforce table-driven test (TDD) cho middleware RBAC và đạt coverage ≥ 80% (DoD GĐ3).

## 9. Tài nguyên học thêm

- [OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html) — deny-by-default, enforce server-side, least privilege.
- [AWS — Multi-tenant SaaS authorization: RBAC & ABAC examples](https://docs.aws.amazon.com/prescriptive-guidance/latest/saas-multitenant-api-access-authorization/avp-mt-abac-rbac-examples.html) — hybrid model, scoped roles.
- [AWS — Multi-tenant data isolation with PostgreSQL RLS](https://aws.amazon.com/blogs/database/multi-tenant-data-isolation-with-postgresql-row-level-security/) — RLS như tầng chặn DB.
- [PlanetScale — Approaches to tenancy in Postgres](https://planetscale.com/blog/approaches-to-tenancy-in-postgres) — so sánh DB-per-tenant / schema / shared-schema.
- [Auth0 — Authorization model for multi-tenant SaaS](https://auth0.com/blog/how-to-choose-the-right-authorization-model-for-your-multi-tenant-saas-application/) — chọn model, embed tenant trong token.
- [OneUptime — How to Handle JWT Revocation](https://oneuptime.com/blog/post/2026-02-02-jwt-revocation/view) — vì sao không nên nhét quyền vào JWT.
- LogMon internal: `doc_v2/09-security.md` (§1–4), `doc_v2/08-database-schema.md` (GĐ3 DDL), `doc_v2/13-adr.md` (ADR-029), `doc_v2/12-roadmap.md` (task 3.6 + DoD GĐ3).

## 10. Checklist "đã hiểu"

- [ ] Phân biệt rạch ròi AuthN / AuthZ / tenant isolation, và biết chúng là **các trục độc lập**.
- [ ] Giải thích được vì sao tenant isolation không phải "một loại role" (admin WS-A ≠ chạm WS-B).
- [ ] Đọc vanh vách ma trận 4 role LogMon và một vài permission tiêu biểu của mỗi role.
- [ ] Nói được vì sao LogMon **không** nhét role vào JWT, và đánh đổi (revoke tức thì vs ~5–10ms lookup).
- [ ] Biết hiện trạng: `workspaceID` đang là *default static*, bảng workspaces/members **chưa** tồn tại, rate limit là in-memory — phân biệt rõ với đích GĐ3.
- [ ] Giải thích "repo nhận workspace từ context trong constructor" bịt lỗi quên filter ra sao, và vì sao trả **404** thay vì 403.
- [ ] Kể được cơ chế isolation cho từng store (Postgres / ES data stream / Prometheus matcher / Redis prefix) và vì sao label Prometheus không phải biên giới cứng.
- [ ] Biết RLS Postgres là tầng defense-in-depth (đích GĐ4) chứ không thay thế filter ở app.
- [ ] Chọn được agent ECC phù hợp cho từng phần (architect / security-reviewer / database-reviewer / go-test).
