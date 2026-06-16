# demo-order

Service mẫu instrument đầy đủ cho LogMon platform. Mục đích:

- **Demo workload** — nguồn telemetry thực tế cho dev/test/load test
- **Tài liệu sống** về tích hợp (~30 dòng/service) — xem phần bên dưới
- Quyết định kiến trúc: **ADR-029** (tách demo workload khỏi internal/)

## Cách chạy

```bash
# Chạy trực tiếp
cd examples/demo-order
go run .

# Chạy với chaos injection
ERROR_RATE=0.1 EXTRA_LATENCY_MS=50 go run .

# Build Docker
docker build -t demo-order .
docker run -p 8081:8081 demo-order

# Sinh traffic giả
bash scripts/loadgen.sh
```

## Env vars

| Biến               | Mặc định | Mô tả                                              |
|--------------------|----------|----------------------------------------------------|
| `PORT`             | `8081`   | Port lắng nghe                                     |
| `LOG_LEVEL`        | `info`   | Level log: debug/info/warn/error                   |
| `ERROR_RATE`       | `0.02`   | Tỉ lệ request `/api/v1/orders` trả 500 giả (0..1) |
| `EXTRA_LATENCY_MS` | `0`      | Latency ngẫu nhiên bổ sung tối đa (ms)            |

## Endpoints

| Method | Path              | Mô tả                    |
|--------|-------------------|--------------------------|
| GET    | `/healthz`        | Health check (no chaos)  |
| GET    | `/metrics`        | Prometheus metrics       |
| GET    | `/api/v1/orders`  | Danh sách orders         |
| POST   | `/api/v1/orders`  | Tạo order mới            |

## Tích hợp 1 service vào LogMon

Để một Go service bất kỳ xuất hiện trong LogMon, cần ~30 dòng:

```go
// 1. Tạo metrics registry riêng
reg := prometheus.NewRegistry()
requests := prometheus.NewCounterVec(prometheus.CounterOpts{
    Name: "logmon_http_requests_total",
    Help: "Tổng số HTTP requests.",
}, []string{"method", "path", "status"})
duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
    Name:    "logmon_http_request_duration_seconds",
    Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
}, []string{"method", "path"})
reg.MustRegister(requests, duration)

// 2. Gắn /metrics endpoint
r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(reg, promhttp.HandlerOpts{})))

// 3. Middleware observe (thêm vào router)
r.Use(func(c *gin.Context) {
    start := time.Now()
    c.Next()
    requests.WithLabelValues(
        c.Request.Method, c.FullPath(), strconv.Itoa(c.Writer.Status()),
    ).Inc()
    duration.WithLabelValues(c.Request.Method, c.FullPath()).
        Observe(time.Since(start).Seconds())
})

// 4. JSON logger với field service
log := zerolog.New(os.Stdout).With().
    Timestamp().Str("service", "my-service").Logger()
log.Info().Str("addr", ":8080").Msg("service khởi động")
```

Prometheus scrape config (thêm vào `prometheus.yml`):
```yaml
- job_name: my-service
  static_configs:
    - targets: ["my-service:8080"]
```
