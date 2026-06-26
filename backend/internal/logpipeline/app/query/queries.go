// Package query chứa read-side use case của logpipeline BC (CQRS).
package query

import (
	"context"
	"fmt"

	"github.com/namdam97/logmon/backend/internal/logpipeline/domain"
	"github.com/namdam97/logmon/backend/internal/logpipeline/ports"
)

// LogQueries phục vụ truy vấn log (read side). Validate input thành SearchCriteria
// trước khi gọi searcher — adapter chỉ nhận tiêu chí đã hợp lệ.
type LogQueries struct {
	searcher ports.LogSearcher
}

// NewLogQueries tạo query service với read port.
func NewLogQueries(searcher ports.LogSearcher) *LogQueries {
	return &LogQueries{searcher: searcher}
}

// Search validate input rồi truy vấn. Lỗi validate trả nguyên (ValidationError)
// để handler map 400; lỗi searcher được bọc context.
func (q *LogQueries) Search(ctx context.Context, in domain.SearchInput) (domain.SearchResult, error) {
	c, err := domain.NewSearchCriteria(in)
	if err != nil {
		return domain.SearchResult{}, err
	}
	res, err := q.searcher.Search(ctx, c)
	if err != nil {
		return domain.SearchResult{}, fmt.Errorf("search logs: %w", err)
	}
	return res, nil
}
