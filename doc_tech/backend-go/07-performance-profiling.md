# Performance & Profiling (Go) trong LogMon
> Module BE-7 · pprof, benchmark, latency budget, load test, p99 · Độ khó: 🥉→🥇 · Prereqs: BE-1, OBS-1

---

## 1. Vì sao kỹ năng này quan trọng trong LogMon

LogMon là **nền tảng observability** — sản phẩm bán chính sự đo lường. Nếu chính `userservice`, `alerting`, hay `logpipeline` của ta *chậm* hoặc *ngốn RAM*, ta vừa làm hỏng sản phẩm vừa **bơm thêm tải** vào hệ thống mà khách hàng đang cố giám sát. Một observability platform chậm là một nghịch lý không bán được.

Ba lý do cụ thể, neo vào kiến trúc:

1. **LogMon nằm trên hot path của khách hàng.** Mọi log/metric/trace đều đi qua pipeline của ta. `doc_v2/12` §GĐ4 đặt mục tiêu đo được: **"Load test 10K logs/s duy trì 1h không mất log"**. Đạt được con số đó không phải bằng phỏng đoán — phải **profiling + benchmark + load test** thật.

2. **Latency là một trong Four Golden Signals — và ta phải đo bằng phân vị, không phải trung bình.** Code đã có sẵn histogram `logmon_http_request_duration_seconds` (xem §4). Histogram tồn tại chính là để tính **p95/p99**, vì trung bình *nói dối* về tail latency (Google SRE — "The Tail at Scale").

3. **Go runtime đã đổi luật chơi (1.25/1.26).** `doc_v2/11` ghi rõ stack chạy **Go 1.26.x** với **Green Tea GC** mặc định và **container-aware GOMAXPROCS**. Hiểu GC, escape analysis, GOMAXPROCS/GOMEMLIMIT là điều kiện để service chạy đúng trong container K8s (GĐ4) thay vì bị kernel throttle.

> **Thực trạng (quan trọng để không hiểu lầm):** Repo **chưa có** benchmark (`grep "func Benchmark"` = 0 file) và **chưa wire** `net/http/pprof`. Đây **không phải thiếu sót** — đó là phần *planned* mà bài này hướng dẫn bạn thêm vào, đúng vào lúc roadmap cần (đo cho DoD GĐ4). Phần *implemented* (histogram metrics, rate limiter) được chỉ rõ ở §4.

---

## 2. Mô hình tư duy (first principles) — giải thích từ con số 0

Hiệu năng quy về **bốn câu hỏi**, đo bằng **bốn loại số**:

| Câu hỏi | Loại số | Đơn vị | Công cụ |
|---|---|---|---|
| Một thao tác mất bao lâu? | Latency | ns/op, ms | benchmark, histogram |
| Bao nhiêu thao tác/giây? | Throughput | req/s, logs/s | load test |
| CPU đốt ở đâu? | CPU profile | % thời gian/hàm | pprof CPU |
| RAM cấp phát ở đâu? | Memory profile | bytes/op, allocs/op | pprof heap, `b.ReportAllocs()` |

Ba nguyên lý nền tảng:

- **Đo trước, sửa sau (measure, don't guess).** Trực giác về "chỗ chậm" gần như luôn sai. Profiler dừng chương trình ~100 lần/giây và chụp stack đang chạy ([Go blog — Profiling Go Programs](https://go.dev/blog/pprof)); dữ liệu thắng phỏng đoán.

- **Trung bình che giấu, phân vị phơi bày.** Nếu p50 = 5ms nhưng p99 = 800ms, *trung bình* (~12ms) khiến bạn yên tâm sai trong khi 1% người dùng đang khổ. Luôn nghĩ theo **phân phối**, không nghĩ theo một con số ([One2N — Why averages lie about latency](https://one2n.io/blog/sre-math-percentiles-in-sre-why-averages-lie-about-latency)).

- **Allocation = chi phí kép.** Mỗi lần cấp phát trên heap vừa tốn lúc cấp phát, vừa tạo việc cho GC sau này. Trong Go, "nhanh hơn" thường đồng nghĩa "cấp phát ít hơn" — nên `allocs/op` quan trọng ngang `ns/op`.

**Latency budget** là cách biến mấy câu trên thành kỷ luật kỹ thuật: bạn *phân bổ* tổng độ trễ cho phép của một request xuống từng tầng. Ví dụ ngân sách 200ms cho `POST /alert-rules`: validate 5ms + PromQL check (promtool) 120ms + ghi Postgres 20ms + render+reload file 50ms + overhead 5ms. Khi một tầng vượt phần của nó, bạn biết *chính xác* phải profile chỗ nào.

---

## 3. Khái niệm cốt lõi (tăng dần)

**🥉 Mức nền — benchmark.** Hàm `func BenchmarkXxx(b *testing.B)` chạy code `b.N` lần (Go tự tăng `b.N` tới khi đủ ~1s để có ý nghĩa thống kê). Luôn gọi `b.ReportAllocs()` để thấy `allocs/op` ([pkg.go.dev/testing](https://pkg.go.dev/testing)).

```go
func BenchmarkValidatePromQL(b *testing.B) {
    expr := `rate(logmon_http_requests_total[5m]) > 0.5`
    b.ReportAllocs()
    b.ResetTimer() // loại chi phí setup khỏi phép đo
    for i := 0; i < b.N; i++ {
        _ = validate(expr)
    }
}
```

Chạy & đọc: `go test -bench=. -benchmem -count=10 ./...` → cột `ns/op`, `B/op`, `allocs/op`.

**🥈 So sánh có ý nghĩa — benchstat.** Một lần chạy là vô nghĩa (nhiễu). Chạy `-count=10` rồi dùng `benchstat old.txt new.txt`: nó tính trung vị + khoảng tin cậy và báo thay đổi có **đáng kể về mặt thống kê** không. Variance <5% là tốt, >10% phải điều tra ([Better Stack — Go benchmarking](https://betterstack.com/community/guides/scaling-go/golang-benchmarking/)).

**🥈 pprof — bốn loại profile.** `runtime/pprof` (trong test) hoặc `net/http/pprof` (server đang chạy) xuất file định dạng pprof:
- **CPU**: thời gian CPU theo hàm — tìm hot function.
- **Heap (`-inuse_space`)**: object còn sống — tìm leak / RAM thường trú.
- **Heap (`-alloc_space`)**: tổng đã cấp phát — tìm chỗ tạo rác cho GC.
- **goroutine / mutex / block**: tìm goroutine leak, contention khoá.

Xem bằng `go tool pprof -http=:8080 cpu.prof` (flamegraph trong trình duyệt).

**🥇 Escape analysis.** Compiler quyết định biến nằm stack (rẻ, tự thu hồi) hay heap (tốn, cần GC). `go build -gcflags='-m'` in lý do "escapes to heap". Đây là cầu nối giữa profile heap và *nguyên nhân gốc*: profile nói "hàm này cấp phát nhiều", `-m` nói "vì biến X escape do bị trả qua interface / con trỏ thoát scope" ([JetBrains — Profiling guide](https://blog.jetbrains.com/go/2026/05/20/golang-profiling-guide/)).

**🥇 GC & runtime knobs.** **GOGC** (mặc định 100) điều khiển tần suất GC theo tăng trưởng heap. **GOMEMLIMIT** (1.19+) đặt trần mềm — bắt buộc khi chạy container để runtime *biết* có giới hạn RAM. **GOMAXPROCS** từ Go 1.25 **tự nhận CPU limit của container** ([Go blog — Container-aware GOMAXPROCS](https://go.dev/blog/container-aware-gomaxprocs)). **Green Tea GC** (1.26 mặc định) tối ưu cho workload nhiều object nhỏ — đúng kiểu của log/metric pipeline.

**🥇 p50/p95/p99 từ histogram.** Histogram chia thời gian vào các bucket; PromQL `histogram_quantile()` nội suy ra phân vị. p99 = ngưỡng mà 99% request nhanh hơn — số bảo vệ tail latency ([OneUptime — P50/P95/P99](https://oneuptime.com/blog/post/2025-09-15-p50-vs-p95-vs-p99-latency-percentiles/view)).

---

## 4. LogMon dùng/sẽ dùng nó thế nào (implemented vs planned)

**✅ Implemented — histogram latency đã có.** `backend/internal/shared/metrics/metrics.go` đã đăng ký:

```go
requestDuration := prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name:    "logmon_http_request_duration_seconds",
        Buckets: prometheus.DefBuckets, // 5ms…10s
    },
    []string{"method", "path"}, // path = route template, KHÔNG high-cardinality
)
```

Đây chính là dữ liệu thô để Prometheus tính p99 (`histogram_quantile(0.99, ...)`). Việc dùng *route template* `"/users/:id"` thay vì path thật là kỷ luật chống cardinality nổ — đúng `doc_v2/04` §"CẤM high-cardinality labels".

**✅ Implemented — rate limiter (in-memory).** `backend/internal/shared/middleware/ratelimit.go` dùng token bucket `golang.org/x/time/rate`, per-IP. Comment trong file tự thừa nhận: map `clients` **chưa có evict** (rò rỉ bộ nhớ chậm theo số IP) và **chỉ hợp single-instance**. `doc_v2/11` §bảng đặt *đích planned*: chuyển sang **`go-redis/redis_rate/v10` (GCRA)** cho multi-instance. Đây là một bài tập profiling thực tế ở §7 (đo heap growth của map).

**✅ Implemented — tracing đã wire.** `internal/shared/tracing/` + OTel SDK (xem `go.mod`: `otelgin`, `otlptracegrpc`). Pipeline OTel Collector → ES → Jaeger v2 đã dựng trong `infra/`. `doc_v2/04` §2.3 mô tả **span metrics (RED tự động)** sinh `duration`/`calls` per endpoint từ trace — nghĩa là latency được đo *hai đường* (HTTP histogram + spanmetrics) với **exemplars** để click từ panel p99 thẳng sang trace của chính request chậm đó.

**🎯 Planned — đích chính thức của doc_v2:**
- **`net/http/pprof` endpoint** sau auth/internal-only: hiện **chưa wire** (`grep pprof` = 0). Đây là việc cần thêm để profile service đang chạy trong GĐ4.
- **Native histograms**: `doc_v2/04` §"Native histograms" (stable Prometheus v3.8) — histogram mới dùng exponential bucket, giữ classic bucket song song giai đoạn chuyển tiếp. Cho phân vị chính xác hơn ở tail.
- **Load test 10K logs/s × 1h** (`doc_v2/12` DoD GĐ4) — đo end-to-end, đếm log mất.
- **Burn-rate p99**: `internal/slo/domain/` (rules.go, slo.go — **đã tồn tại**) tính error budget; SLO latency-based (vd "99% < 1s") chính là p99 budget; `doc_v2/04` §SLO config có `latency: { threshold_ms: 1000 }` cho tail-sampling — slow request luôn được giữ trace.

---

## 5. Best practices (mỗi mục kèm 1 nguồn đã research)

1. **Đo bằng phân vị, không bằng trung bình.** Báo cáo p50/p95/p99, không bao giờ chỉ "avg latency". Histogram đã có sẵn — dùng `histogram_quantile`. ([One2N — averages lie](https://one2n.io/blog/sre-math-percentiles-in-sre-why-averages-lie-about-latency))

2. **Luôn `b.ReportAllocs()` + `-count≥10` + `benchstat`.** Một lần chạy không kết luận được gì; benchstat cho biết thay đổi có ý nghĩa thống kê. ([Better Stack](https://betterstack.com/community/guides/scaling-go/golang-benchmarking/))

3. **Profile trước khi tối ưu.** Lấy CPU profile, mở flamegraph, sửa đúng hot path — đừng đoán. ([Go blog — pprof](https://go.dev/blog/pprof))

4. **Ghép heap profile với `-gcflags='-m'`.** Profile cho biết *cái gì* cấp phát; escape analysis cho biết *tại sao* — sửa nguyên nhân, không sửa triệu chứng. ([JetBrains profiling guide](https://blog.jetbrains.com/go/2026/05/20/golang-profiling-guide/))

5. **Đặt `GOMEMLIMIT` khi chạy container, tin tưởng GOMAXPROCS container-aware của 1.25+.** Không set GOMEMLIMIT → runtime không biết trần RAM → OOMKill. Go 1.25 đã tự khớp GOMAXPROCS với CPU limit, tránh kernel throttle gây tail latency. ([Go blog — Container-aware GOMAXPROCS](https://go.dev/blog/container-aware-gomaxprocs))

6. **Load test focus vào tail dưới tải.** Khi tăng RPS, theo dõi p99 — nếu p99 tách xa median nghĩa là tail đang xấu đi (contention/GC pause/queueing). ([RadView — P99 latency in load testing](https://www.radview.com/blog/p99-latency-why-matters-how-measure-load-testing/))

---

## 6. Lỗi thường gặp & anti-patterns

- **Tối ưu theo cảm tính.** Sửa "chỗ trông chậm" mà chưa profile → thường vô ích hoặc làm tệ hơn. Profile trước.
- **Quên `b.ResetTimer()` / không exclude setup.** Setup nặng (tạo client, seed data) bị tính vào phép đo → số sai. Dùng `b.ResetTimer()` sau setup, `b.StopTimer()/StartTimer()` quanh per-iteration setup.
- **Kết luận từ một lần chạy benchmark.** Nhiễu 10–20% là bình thường; không có benchstat thì "nhanh hơn 8%" có thể chỉ là noise.
- **`histogram_quantile` trên bucket sai.** `DefBuckets` cao nhất ~10s; nếu endpoint thật dưới 50ms thì các bucket thưa khiến p99 nội suy lệch. Chỉnh bucket theo *latency budget thực* của endpoint (đây là lý do native histogram được planned).
- **Để pprof endpoint public.** `net/http/pprof` lộ stack/heap nội bộ — phải đặt sau auth hoặc cổng internal-only, không bao giờ mở ra internet (đúng tinh thần `doc_v2/09`).
- **High-cardinality label.** Thêm `user_id`/`request_id` vào histogram → series nổ, Prometheus OOM. `CLAUDE.md` cấm tuyệt đối; dùng exemplar để gắn trace, không gắn ID vào label.
- **Bỏ qua `allocs/op`.** CPU nhanh nhưng cấp phát nhiều → GC pressure → tail latency xấu dưới tải. Object nhỏ nhiều lần là sát thủ thầm lặng (đúng workload Green Tea GC nhắm tới).
- **Để rate-limiter map rò rỉ.** Map `clients` không evict tăng vô hạn theo IP — heap profile sẽ chỉ ra; đây là lý do `doc_v2/11` planned chuyển sang Redis GCRA.

---

## 7. Lộ trình luyện tập NGAY trong repo LogMon (🥉→🥈→🥇)

**🥉 Task 1 — Benchmark đầu tiên + đọc allocs.**
Viết `ratelimit_bench_test.go` trong `backend/internal/shared/middleware/`:
```go
func BenchmarkLimiterFor(b *testing.B) {
    l := NewPerMinuteLimiter(100, 20)
    b.ReportAllocs(); b.ResetTimer()
    for i := 0; i < b.N; i++ { _ = l.limiterFor("10.0.0.1") }
}
```
Chạy `cd backend && go test -bench=. -benchmem ./internal/shared/middleware/`. Quan sát `allocs/op` cho IP đã tồn tại (nên gần 0) vs IP mới (cấp phát limiter). Ghi lại con số.

**🥈 Task 2 — pprof + escape analysis trên domain thật.**
Lấy benchmark của `internal/slo/domain` (burn-rate math đã có code), chạy:
```bash
cd backend && go test -bench=. -cpuprofile=cpu.prof -memprofile=mem.prof ./internal/slo/domain/
go tool pprof -http=:8080 mem.prof          # flamegraph alloc
go test -gcflags='-m' ./internal/slo/domain/ 2>&1 | grep escapes
```
Mục tiêu: tìm 1 biến "escapes to heap" không cần thiết, thử sửa (vd tránh trả interface, preallocate slice với capacity), rồi `benchstat` trước/sau với `-count=10`.

**🥈 Task 3 — Wire `net/http/pprof` an toàn.**
Trong `cmd/userservice/main.go`, mount `net/http/pprof` trên một mux **internal-only** (không qua Gin public router, không qua reverse proxy). Verify: `go tool pprof http://localhost:<internal>/debug/pprof/heap`. Đây là tiền đề cho profiling GĐ4.

**🥇 Task 4 — Latency budget + p99 thật.**
Chạy `make up`, bắn tải vào `userservice` (k6 hoặc `hey`), rồi query Prometheus:
```promql
histogram_quantile(0.99, sum by (le,path) (rate(logmon_http_request_duration_seconds_bucket[5m])))
```
Viết latency budget cho 1 endpoint (vd login: argon2id verify là phần lớn ngân sách — đo thật bằng benchmark hàm hash). So p99 đo được với budget; nếu vượt, profile chỗ vượt.

**🥇 Task 5 — Mini load test theo hướng DoD GĐ4.**
Dựng kịch bản đếm end-to-end qua `logpipeline` (OTel→ES), tăng dần RPS, vẽ p99 theo RPS. Tìm điểm p99 bắt đầu tách median (knee point) — đó là throughput ceiling hiện tại. Đây là bản thu nhỏ của "10K logs/s × 1h".

---

## 8. Skill/agent ECC nên dùng

- **`ecc:performance-optimizer`** (qua skill `ecc:benchmark-optimization-loop` / `ecc:latency-critical-systems`) — vòng lặp "make it faster": sinh nhiều biến thể, benchmark, chọn bản nhanh nhất có đo lường. Dùng cho Task 2/4 khi tối ưu hot path.
- **`ecc:golang-testing`** — pattern viết benchmark table-driven, `b.Run` subbench, helper. Dùng cho Task 1.
- **`ecc:benchmark`** — đo baseline + phát hiện regression trước/sau PR (đúng triết lý benchstat). Gắn vào CI để chặn regression hiệu năng.
- Bổ trợ: **`ecc:go-review`** (go-reviewer) soát concurrency/alloc anti-pattern; **`ecc:latency-critical-systems`** cho tư duy tail latency ở mức hệ thống.

---

## 9. Tài nguyên học thêm

- [Go blog — Profiling Go Programs](https://go.dev/blog/pprof) — bài gốc về pprof, cách profiler lấy mẫu.
- [Go — Diagnostics](https://go.dev/doc/diagnostics) — tổng quan profiling/tracing/debugging chính thức.
- [pkg.go.dev — testing (BenchmarkXxx, ReportAllocs)](https://pkg.go.dev/testing) — API benchmark chuẩn.
- [Better Stack — Benchmarking in Go (benchstat, variance)](https://betterstack.com/community/guides/scaling-go/golang-benchmarking/) — thực hành so sánh có ý nghĩa thống kê.
- [Go blog — Container-aware GOMAXPROCS](https://go.dev/blog/container-aware-gomaxprocs) — runtime 1.25 trong container.
- [JetBrains — A Practical Guide to Profiling in Go (2026)](https://blog.jetbrains.com/go/2026/05/20/golang-profiling-guide/) — pprof + escape analysis kèm flamegraph.
- [One2N — Percentiles in SRE: why averages lie](https://one2n.io/blog/sre-math-percentiles-in-sre-why-averages-lie-about-latency) + [OneUptime — P50/P95/P99](https://oneuptime.com/blog/post/2025-09-15-p50-vs-p95-vs-p99-latency-percentiles/view) — nền tảng phân vị/tail.
- LogMon nội bộ: `doc_v2/04-metrics-tracing.md` (histogram, RED, native histogram), `doc_v2/11-coding-testing-standards.md` (Go 1.26/Green Tea GC), `doc_v2/12-roadmap.md` §GĐ4 (load test DoD).

---

## 10. Checklist "đã hiểu"

- [ ] Giải thích được vì sao **trung bình nói dối** và khi nào dùng p95 vs p99.
- [ ] Viết được một `BenchmarkXxx`, gọi `b.ReportAllocs()` + `b.ResetTimer()`, đọc đúng `ns/op`/`B/op`/`allocs/op`.
- [ ] Biết chạy `-count=10` + `benchstat` để khẳng định thay đổi có **ý nghĩa thống kê**.
- [ ] Phân biệt được 4 loại pprof profile và biết khi nào dùng cái nào (CPU vs inuse vs alloc vs goroutine).
- [ ] Đọc được output `go build -gcflags='-m'` và nối nó với heap profile để tìm **nguyên nhân** escape.
- [ ] Biết vai trò **GOGC / GOMEMLIMIT / GOMAXPROCS** trong container và vì sao Go 1.25+ quan trọng cho K8s.
- [ ] Chỉ ra trong repo: histogram `logmon_http_request_duration_seconds` (✅ implemented) và viết được PromQL p99 từ nó.
- [ ] Nêu được phần **planned** theo doc_v2: `net/http/pprof` endpoint, native histograms, load test 10K logs/s (GĐ4), Redis GCRA rate limit.
- [ ] Biết vì sao **không** để pprof public và **không** thêm high-cardinality label vào histogram.
