# Dockerfile — multi-stage build cho demo-order service.
# Stage builder: biên dịch binary tĩnh không phụ thuộc CGO.
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o demo-order .

# Stage runtime: image tối giản, chạy với user không có quyền root.
FROM alpine:3.22
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
WORKDIR /app
COPY --from=builder /app/demo-order .
USER appuser
EXPOSE 8081
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s \
    CMD wget -qO- http://localhost:8081/healthz || exit 1
ENTRYPOINT ["./demo-order"]
