// Package httpx chứa response envelope chuẩn cho mọi HTTP API và helper map
// domain error sang HTTP status. Mọi response đều dùng cùng một format.
package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/namdam97/logmon/backend/internal/shared/errors"
)

// Meta chứa metadata cho response phân trang.
type Meta struct {
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

// Envelope là format thống nhất cho mọi response.
type Envelope struct {
	Success bool   `json:"success"`
	Data    any    `json:"data"`
	Error   string `json:"error,omitempty"`
	Meta    *Meta  `json:"meta,omitempty"`
}

// OK ghi response thành công với data payload.
func OK(c *gin.Context, status int, data any) {
	c.JSON(status, Envelope{Success: true, Data: data})
}

// Fail ghi response lỗi với message generic (không leak chi tiết nội bộ).
func Fail(c *gin.Context, status int, message string) {
	c.JSON(status, Envelope{Success: false, Data: nil, Error: message})
}

// FailFromError map domain error sang HTTP status + message an toàn cho user.
// Chi tiết lỗi phải được log riêng ở tầng gọi, KHÔNG trả raw error ra ngoài.
func FailFromError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		Fail(c, http.StatusNotFound, "resource not found")
	case errors.Is(err, apperrors.ErrConflict):
		Fail(c, http.StatusConflict, "resource already exists")
	case errors.Is(err, apperrors.ErrUnauthorized):
		Fail(c, http.StatusUnauthorized, "unauthorized")
	default:
		if ve, ok := apperrors.AsValidationError(err); ok {
			Fail(c, http.StatusBadRequest, ve.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, "internal server error")
	}
}
