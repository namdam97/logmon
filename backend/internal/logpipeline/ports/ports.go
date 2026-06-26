// Package ports khai báo interface tầng app của logpipeline BC phụ thuộc (DIP).
// Implementation ở adapters (Elasticsearch). GĐ2.8 chỉ có read side.
package ports

import (
	"context"

	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
)

// LogSearcher truy vấn log từ backend lưu trữ (Elasticsearch, data stream logs-*).
// Read side (CQRS) — không có ghi.
type LogSearcher interface {
	Search(ctx context.Context, c domain.SearchCriteria) (domain.SearchResult, error)
}
