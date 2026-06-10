# syntax=docker/dockerfile:1
# Multi-stage build cho userservice — binary tĩnh, image runtime tối giản.
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/userservice ./cmd/userservice

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/userservice /app/userservice
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/userservice"]
