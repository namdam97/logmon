package promql_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/adapters/promql"
)

func TestValidatorValidateExpression(t *testing.T) {
	v := promql.NewValidator()
	tests := []struct {
		name    string
		give    string
		wantErr bool
	}{
		{name: "valid rate threshold", give: `rate(logmon_http_requests_total{status=~"5.."}[5m]) > 0.05`},
		{name: "valid up", give: "up == 0"},
		{name: "valid histogram_quantile", give: `histogram_quantile(0.95, sum by (le) (rate(x_bucket[5m]))) > 1`},
		{name: "empty", give: "", wantErr: true},
		{name: "unbalanced paren", give: "rate(x[5m]", wantErr: true},
		{name: "garbage", give: "this is not promql !!!", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateExpression(tt.give)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
