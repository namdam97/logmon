# Docker, Compose & Nginx reverse proxy
> Module DEPLOY-2 · multi-stage build, compose profiles, reverse proxy/TLS · Độ khó: 🥉→🥇 · Prereqs: BE-1

## 1. Vì sao kỹ năng này quan trọng trong LogMon

LogMon là nền tảng observability gồm hàng chục process: `userservice` (Go), Postgres, Prometheus, Grafana, Elasticsearch, hai OTel Collector (agent + gateway), Alertmanager, Jaeger, các exporter, cộng demo workload. Không ai cài tay ngần đó thứ trên máy dev. Docker đóng gói mỗi thứ thành image bất biến; Docker Compose dựng cả đồ thị phụ thuộc đó bằng **một** lệnh — chính là `make up` / `make up-full` (`Makefile:32-39`).

Cụ thể, kỹ năng này quyết định ba thứ trong repo:
- **Build image nhỏ và an toàn** cho service Go: `backend/Dockerfile` build ra binary tĩnh chạy trên distroless, không shell, chạy user `nonroot`.
- **Điều phối stack theo nhu cầu**: profile `observability` và `demo` cho phép `make up` chỉ dựng phần nhẹ (3 service) còn `make up-full` mới kéo cả monitoring stack — tiết kiệm RAM/thời gian khi dev.
- **Đưa hệ thống ra Internet an toàn** qua reverse proxy + TLS (Nginx, ADR-041) — đây là phần **planned**, chưa có file config trong repo.

Hiểu sai một mắt xích (ví dụ thứ tự khởi động, healthcheck, hay header proxy) là nguyên nhân số một của lỗi "chạy được trên máy tôi nhưng chết ở staging".

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Bắt đầu từ vấn đề gốc: một chương trình cần *môi trường* (thư viện, biến môi trường, file config, cổng mạng). Trên hai máy khác nhau, môi trường khác nhau → hành vi khác nhau.

- **Container** = một process chạy bị "cách ly" bằng tính năng của Linux kernel (namespaces cho cách ly tiến trình/mạng, cgroups cho giới hạn CPU/RAM). Nó **không** phải máy ảo — không có kernel riêng, nhẹ hơn nhiều.
- **Image** = ảnh chụp **chỉ-đọc** của filesystem + metadata (lệnh chạy, cổng, user). Container = một instance đang chạy của image, cộng thêm một lớp ghi-được ở trên cùng.
- **Layer** = mỗi dòng trong `Dockerfile` (`COPY`, `RUN`...) tạo một lớp được cache. Sửa dòng nào thì rebuild từ dòng đó trở xuống. Đây là lý do ta `COPY go.mod go.sum` rồi `go mod download` **trước** khi `COPY . .` (xem `backend/Dockerfile:6-9`): code thay đổi liên tục nhưng dependency thì hiếm, nên lớp download được tái dùng.
- **Docker Compose** = file YAML khai báo *nhiều* container, mạng và volume cùng quan hệ giữa chúng, để dựng/xoá như một đơn vị. Tư duy: **khai báo trạng thái mong muốn**, để Compose lo phần "làm sao đạt được".
- **Reverse proxy** = một process đứng trước nhiều backend, nhận request từ ngoài rồi *chuyển tiếp* vào trong. Khác "forward proxy" (đại diện cho client). Reverse proxy là nơi gom TLS, security header, routing — client không bao giờ nói chuyện trực tiếp với app.

Quy tắc nền: **bất biến + cách ly + khai báo**. Image bất biến (không sửa container đang chạy, mà build lại); service cách ly nhau qua mạng riêng; toàn bộ topology được khai báo trong file đọc được, versioned trong Git.

## 3. Khái niệm cốt lõi (tăng dần độ khó)

### 3.1 Dockerfile & multi-stage build
Mỗi instruction tạo layer. **Multi-stage** dùng nhiều `FROM`: stage đầu có đủ toolchain để build, stage cuối chỉ `COPY --from` artifact ra một base tối giản. Toàn bộ compiler/SDK bị bỏ lại → image cuối nhỏ và ít CVE.

```dockerfile
FROM golang:1.26-alpine AS build      # stage build: có Go toolchain
RUN CGO_ENABLED=0 go build -o /out/app ./cmd/app
FROM gcr.io/distroless/static-debian12:nonroot   # stage runtime: chỉ binary
COPY --from=build /out/app /app/app
```

### 3.2 Base image — đánh đổi
| Base | Kích thước | Shell/pkg mgr | Khi nào dùng |
|------|-----------|---------------|--------------|
| `scratch` | ~0 | không | binary tĩnh thuần, không cần CA cert |
| `distroless/static` | ~2 MB | không | binary Go tĩnh + cần CA certs/tz (LogMon `userservice`) |
| `alpine` | ~7 MB | có (busybox) | cần shell để debug/healthcheck `wget` (LogMon `demo-order`) |
| `debian-slim` | ~75 MB | có | cần glibc / nhiều tool |

Distroless an toàn hơn (không shell → khó RCE) nhưng khó `docker exec` vào debug — đánh đổi mà OWASP và Docker khuyến nghị cho production.

### 3.3 Compose: service, network, volume, depends_on
- **service**: một container (hoặc nhóm replica) khai báo từ `image:` hoặc `build:`.
- **network**: mặc định Compose tạo một mạng bridge; service gọi nhau qua **tên service** làm hostname (vd `postgres:5432`).
- **volume**: lưu trữ bền vững nằm ngoài vòng đời container (`pgdata`, `esdata`...).
- **depends_on + condition**: thứ tự khởi động. `service_healthy` chờ healthcheck pass; `service_completed_successfully` chờ job one-shot xong (cần Compose v2.20+).

### 3.4 Profiles
Profile gắn nhãn service để bật/tắt theo nhóm. Service không có `profiles:` luôn chạy; service có profile chỉ chạy khi profile đó được kích hoạt. LogMon dùng để tách stack nhẹ vs nặng.

### 3.5 Reverse proxy & TLS termination
Nginx nhận HTTPS ở `:443`, giải mã (TLS termination), rồi `proxy_pass` HTTP vào backend nội bộ. Lợi ích: gom cert về một chỗ, offload CPU mã hoá khỏi app, cho phép routing `/` → frontend và `/api` → backend **cùng origin** (điều kiện để cookie `SameSite=Strict` của LogMon hoạt động — `doc_v2/14:297`).

## 4. LogMon dùng nó thế nào (bám code thật — path:line, ghi rõ implemented/planned)

**[Implemented] `backend/Dockerfile` — multi-stage distroless.** `FROM golang:1.26-alpine AS build` (`backend/Dockerfile:3`); cache dependency bằng `COPY go.mod go.sum` rồi `go mod download` trước `COPY . .` (`:6-9`); build tĩnh `CGO_ENABLED=0 GOOS=linux ... -trimpath -ldflags="-s -w"` (`:10-11`) — `-s -w` bỏ symbol/debug để binary nhỏ. Runtime là `gcr.io/distroless/static-debian12:nonroot` (`:13`) với `USER nonroot:nonroot` (`:17`). Uid của user này là **65532**, và đó chính là lý do `genrules-init` phải `chown -R 65532:65532` volume rule (`infra/docker/docker-compose.yml:44-49`) để `userservice` ghi được rule file cho Prometheus đọc.

**[Implemented] `examples/demo-order/Dockerfile` — multi-stage alpine.** Khác `userservice` ở chỗ runtime là `alpine:3.22` (`examples/demo-order/Dockerfile:11`) với user tạo thủ công (`addgroup/adduser`, `:12`) và có `HEALTHCHECK ... wget` ngay trong image (`:17-18`) — vì alpine có shell/wget còn distroless thì không.

**[Implemented] Compose — profiles, healthcheck, secrets, limits.**
- Stack nhẹ (không profile): `postgres`, `migrate`, `genrules-init`, `userservice`. Profile `[observability]` (`docker-compose.yml:151`, 181, 208, ...) bọc Prometheus/Grafana/ES/OTel/Jaeger/exporter; profile `[demo]` (`:108`, 126) bọc `demo-order` + `loadgen`.
- **Thứ tự khởi động**: `userservice` chờ `postgres: service_healthy`, `migrate: service_completed_successfully`, `genrules-init: service_completed_successfully` (`:97-103`).
- **Healthcheck** mọi service dài hạn, vd `userservice` `wget -qO- http://localhost:8080/healthz` (`:56-61`) có `start_period: 10s`.
- **Secrets file-based**: `slack_webhook_url`, `healthchecks_url`, `logmon_webhook_token` (`:402-408`), mount vào Alertmanager (`:190-193`); thư mục `infra/docker/secrets/` có `.gitignore` và các file `*.example`.
- **Resource limits** `deploy.resources.limits` cho hầu hết service (vd ES `memory: 2g, cpus: "1.0"`, `:295-299`) — hoạt động cả với `docker compose` thường.
- **Image pin theo minor**: `postgres:16-alpine`, `prom/prometheus:v3.12.0`, `migrate/migrate:v4.18.1`... không dùng `:latest`.

**[Implemented] `Makefile` điều phối.** `up` = stack nhẹ (`Makefile:32-33`); `up-full` set env `OTEL_EXPORTER_OTLP_ENDPOINT` + `ELASTICSEARCH_URL` rồi `--profile observability` (`:35-36`); `up-demo` thêm `--profile demo` (`:38-39`); `migrate` chạy `compose run --rm migrate` one-shot (`:58-59`).

**[Planned] Reverse proxy Nginx + TLS.** ADR-041 chốt **Nginx + certbot** làm TLS termination trước frontend/backend ở staging GĐ1.8 (`doc_v2/13-adr.md:318-324`). Hiện **chưa có file config nginx nào trong repo** và **chưa có `docker-compose.prod.yml`** — runbook còn ghi "chưa có trong compose; chưa chọn dứt khoát giữa Caddy/Nginx" (`doc_v2/16-iac-runbooks.md:77`), dù ADR-041 đã chọn Nginx. Mạng `backend: { internal: true }` để DB/ES không có đường ra Internet cũng là **planned** (`doc_v2/10:38-40,55`), compose dev hiện chưa khai báo network tùy biến.

> Lưu ý: nhiều thứ trong CLAUDE.md là target. Trong `backend/internal/` hiện chỉ có `user`, `alerting`, `slo`, `logpipeline`, `shared` (có code thật); các BC `incident`/`notification`, k8s manifests, `docker-compose.prod.yml` đều **planned** — đừng giả định chúng tồn tại.

## 5. Best practices (mỗi mục kèm 1 nguồn đã research)

1. **Multi-stage + base tối giản, chạy non-root.** Bỏ toolchain khỏi image cuối, dùng distroless/scratch cho binary Go tĩnh, đặt `USER` non-root — đúng như cả hai Dockerfile của LogMon. ([Sysdig — Dockerfile best practices](https://www.sysdig.com/learn-cloud-native/dockerfile-best-practices))
2. **Pin image, không `:latest`.** Lý tưởng pin theo digest `@sha256:...`; tối thiểu theo minor version để build tái lập và quét CVE ổn định. LogMon pin minor; nâng cấp qua PR. ([OWASP Docker Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html))
3. **Tách base + prod override.** Giữ `docker-compose.yml` làm base, thêm `docker-compose.prod.yml` cho limits/restart/logging/TLS, chạy `-f base -f prod`. (LogMon: planned, ADR-040.) ([Docker Docs — Use Compose in production](https://docs.docker.com/compose/how-tos/production/))
4. **Healthcheck + `depends_on: service_healthy`.** Đừng tin "container started" = "service ready"; dùng healthcheck có `start_period` và điều kiện khởi động — cần Compose v2.20+. ([freeCodeCamp — Compose for production](https://www.freecodecamp.org/news/how-to-use-docker-compose-for-production-workloads/))
5. **Secrets qua `secrets:`/vault, không nhúng vào image.** Credentials dùng Docker secrets file-based; `env_file`/`.env` chỉ cho config không nhạy cảm và không commit. ([Docker Docs — Use Compose in production](https://docs.docker.com/compose/how-tos/production/))
6. **Nginx: bốn header proxy là bắt buộc.** `Host`, `X-Real-IP`, `X-Forwarded-For`, `X-Forwarded-Proto`; với WebSocket thêm `Upgrade`/`Connection "upgrade"` và tăng `proxy_read_timeout`. Thiếu `X-Forwarded-Proto` gây redirect loop HTTP/HTTPS. ([GetPageSpeed — NGINX Reverse Proxy 2026](https://www.getpagespeed.com/server-setup/nginx/nginx-reverse-proxy))

## 6. Lỗi thường gặp & anti-patterns

- **`COPY . .` trước khi cài deps** → cache layer dependency vô dụng, mỗi lần sửa code đều `go mod download` lại. (LogMon đã tránh — copy go.mod trước.)
- **Dùng `:latest`** → build không tái lập, "hôm qua chạy hôm nay hỏng" khi upstream đẩy bản mới.
- **Chạy container bằng root** → leo thang đặc quyền nếu thoát container. Luôn `USER` non-root.
- **Coi `depends_on` không điều kiện = đã sẵn sàng.** `depends_on` trơn chỉ đảm bảo *started*, không phải *healthy*. `userservice` sẽ kết nối DB lỗi nếu thiếu `condition: service_healthy`.
- **Bind-mount source code ở prod.** Tiện cho dev nhưng phá tính bất biến; prod phải để code nằm trong image (Docker khuyến nghị bỏ volume binding code ở prod).
- **Hardcode secret trong compose/image** thay vì `secrets:`. Đặc biệt nguy hiểm khi image bị push lên registry.
- **Nginx thiếu header chuyển tiếp** → app thấy IP của Nginx, hoặc sinh redirect loop vì tưởng request là HTTP.
- **Lệ thuộc shell trong distroless** cho healthcheck → `CMD ["wget"...]` lỗi vì không có shell; với distroless phải dùng exec-form trỏ thẳng binary, hoặc healthcheck ở tầng compose như LogMon làm.
- **Mở mọi port ra host.** DB/ES nên ở network `internal`, chỉ proxy/frontend lộ ra ngoài (LogMon planned).

## 7. Lộ trình luyện tập NGAY trong repo LogMon

### 🥉 Cơ bản
1. Chạy `make doctor` rồi `make up`; dùng `make ps` và `make logs S=userservice` quan sát, `curl localhost:8080/healthz`.
2. Đọc `backend/Dockerfile` và giải thích bằng lời tại sao `COPY go.mod go.sum` đứng trước `COPY . .` — rồi thử đảo thứ tự, `make build`, đo lại thời gian build.
3. Chạy `make up-full`, mở Grafana ở `localhost:3001` và Prometheus `localhost:9090`; đối chiếu service nào thuộc profile `observability` trong `docker-compose.yml`.
4. So sánh runtime base của `backend/Dockerfile` (distroless) vs `examples/demo-order/Dockerfile` (alpine) và viết 3 dòng vì sao khác nhau.

### 🥈 Trung cấp
1. Thêm một biến môi trường mới cho `userservice` trong compose (vd `LOG_LEVEL=debug`) với cú pháp fallback `${VAR:-default}` giống các biến hiện có, rebuild và xác minh qua `make logs`.
2. Thêm `HEALTHCHECK` exec-form vào `backend/Dockerfile`? — *không làm được* vì distroless không có shell/wget; thay vào đó hãy chỉnh `start_period` healthcheck của `userservice` trong compose và quan sát `make ps` cột health.
3. Pin một image từ minor sang digest `@sha256:` (vd `postgres:16-alpine`) — lấy digest bằng `docker buildx imagetools inspect`, cập nhật compose, `make up` lại.
4. Thêm `deploy.resources.limits` cho `userservice` (hiện chưa có) và quan sát `docker stats` để thấy giới hạn áp dụng.

### 🥇 Nâng cao
1. **Hiện thực phần planned**: viết `infra/docker/docker-compose.prod.yml` (override) thêm network `backend: { internal: true }`, gắn `postgres`/`elasticsearch` vào đó, bỏ `ports` công khai của chúng; test bằng `docker compose -f ... -f docker-compose.prod.yml config`.
2. Viết một service `nginx` (image `nginx:1.27-alpine`) + file `infra/nginx/logmon.conf` reverse proxy `/api` → `userservice:8080`, `/` → frontend; bật đủ bốn header `X-Forwarded-*`; chạy chỉ HTTP trước (ADR-041 là TLS staging).
3. Thêm TLS self-signed cho Nginx local (mkcert/openssl), terminate `:443`, `proxy_pass` HTTP nội bộ; xác minh cookie auth còn hoạt động cùng-origin.
4. Tạo profile `[scale]` cho một service mới (vd Kafka, theo `doc_v2/10:34-36`) và thêm target Makefile `up-scale` tương tự `up-full`.
5. Thêm bước quét image vào quy trình build (Trivy, ADR-044) và fail khi có CVE CRITICAL/HIGH có bản vá.

## 8. Skill/agent ECC nên dùng khi luyện

- **`ecc:docker-patterns`** — khi viết/sửa Dockerfile và compose: kiểm tra multi-stage, thứ tự layer cache, non-root, healthcheck, pin image. Dùng ở task 🥉#2, 🥈#3, 🥇#5.
- **`ecc:deployment-patterns`** — khi dựng phần production: tách base/prod override, network segmentation, restart policy, reverse proxy/TLS, rollback. Dùng ở task 🥇#1–#3 (hiện thực `docker-compose.prod.yml` + Nginx).
- **`ecc:security-scan` / skill `cso`** — chạy trước khi commit cấu hình hạ tầng: phát hiện secret hardcode, port hở, image `:latest` (đúng checklist `doc_v2/09`).
- **`ecc:go-build`** — khi `make build` lỗi do thay đổi Dockerfile làm hỏng biên dịch Go.

## 9. Tài nguyên học thêm (link đã research, có chú thích 1 dòng)

- [Docker Docs — Use Compose in production](https://docs.docker.com/compose/how-tos/production/) — nguồn chính thức về override file, restart, bỏ volume binding code.
- [OWASP Docker Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html) — checklist bảo mật: non-root, pin digest, least privilege, supply chain.
- [Sysdig — Top Dockerfile best practices](https://www.sysdig.com/learn-cloud-native/dockerfile-best-practices) — multi-stage, base tối giản, giảm attack surface, scan.
- [freeCodeCamp — Docker Compose for production workloads](https://www.freecodecamp.org/news/how-to-use-docker-compose-for-production-workloads/) — profiles, watch mode, điều kiện `service_healthy`, version Compose tối thiểu.
- [GetPageSpeed — NGINX Reverse Proxy Setup Guide (2026)](https://www.getpagespeed.com/server-setup/nginx/nginx-reverse-proxy) — `proxy_pass`, bốn header bắt buộc, TLS termination.
- [GetPageSpeed — NGINX WebSocket Proxy Guide](https://www.getpagespeed.com/server-setup/nginx/nginx-websocket-proxy) — `Upgrade`/`Connection` header và timeout cho WebSocket/SSE.

## 10. Checklist "đã hiểu" (self-assessment)

- [ ] Giải thích được layer caching và vì sao `COPY go.mod go.sum` đứng trước `COPY . .` trong `backend/Dockerfile`.
- [ ] Phân biệt được khi nào chọn distroless vs alpine, và biết uid `nonroot` = 65532 ảnh hưởng tới quyền ghi volume.
- [ ] Biết `make up` dựng gì và `make up-full` thêm gì, mapping tới profile nào trong compose.
- [ ] Phân biệt `depends_on` trơn vs `condition: service_healthy` / `service_completed_successfully` và vì sao `userservice` cần cả ba.
- [ ] Biết secret trong LogMon được nạp bằng `secrets:` file-based, không nhúng vào image.
- [ ] Liệt kê được bốn header `X-Forwarded-*` Nginx phải set và lý do reverse proxy giúp cookie `SameSite=Strict` hoạt động cùng-origin.
- [ ] Chỉ ra được đâu là **implemented** (Dockerfiles, compose profiles, Makefile) vs **planned** (Nginx config, `docker-compose.prod.yml`, network `internal`).
