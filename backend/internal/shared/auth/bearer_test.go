package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/auth"
)

func newBearerEngine(token string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/internal", auth.RequireBearerToken(token), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	return r
}

func TestRequireBearerToken(t *testing.T) {
	tests := []struct {
		name       string
		token      string // token cấu hình ở server
		header     string // header Authorization client gửi
		wantStatus int
	}{
		{name: "valid token", token: "s3cret", header: "Bearer s3cret", wantStatus: http.StatusOK},
		{name: "wrong token", token: "s3cret", header: "Bearer nope", wantStatus: http.StatusUnauthorized},
		{name: "missing header", token: "s3cret", header: "", wantStatus: http.StatusUnauthorized},
		{name: "no bearer prefix", token: "s3cret", header: "s3cret", wantStatus: http.StatusUnauthorized},
		{name: "fail closed when unconfigured", token: "", header: "Bearer anything", wantStatus: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/internal", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}

			newBearerEngine(tt.token).ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
