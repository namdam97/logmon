// Package domain chứa read model + value object cho logpipeline BC (read side).
// GĐ2.8 chỉ có truy vấn log (CQRS query); write side (Mode switch, DLQ, ILM) ở
// các giai đoạn sau. Domain chỉ import Go stdlib + shared/errors.
package domain

import "time"

// LogEntry là read model một dòng log lấy từ Elasticsearch (data stream logs-*,
// doc shape OTel-native). Chỉ phơi các field cần cho UI/API.
type LogEntry struct {
	Timestamp time.Time
	Severity  string
	Body      string
	Service   string
	TraceID   string
	SpanID    string
}

// SearchResult gói kết quả truy vấn: các entry trang hiện tại + tổng số khớp.
type SearchResult struct {
	Entries []LogEntry
	Total   int
}
