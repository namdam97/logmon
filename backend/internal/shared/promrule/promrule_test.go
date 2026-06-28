package promrule_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/promrule"
)

func TestBuildObject(t *testing.T) {
	tests := []struct {
		name   string
		give   []promrule.Group
		labels map[string]string
		want   func(t *testing.T, spec map[string]any)
	}{
		{
			name: "alerting rule giữ alert/expr/for/labels/annotations",
			give: []promrule.Group{{
				Name: "logmon-generated",
				Rules: []promrule.Rule{{
					Alert:       "HighErrorRate",
					Expr:        "rate(errors[5m]) > 0.1",
					For:         "5m",
					Labels:      map[string]string{"severity": "critical"},
					Annotations: map[string]string{"summary": "boom"},
				}},
			}},
			want: func(t *testing.T, spec map[string]any) {
				groups := spec["groups"].([]any)
				require.Len(t, groups, 1)
				g := groups[0].(map[string]any)
				require.Equal(t, "logmon-generated", g["name"])
				r := g["rules"].([]any)[0].(map[string]any)
				require.Equal(t, "HighErrorRate", r["alert"])
				require.Equal(t, "rate(errors[5m]) > 0.1", r["expr"])
				require.Equal(t, "5m", r["for"])
				require.NotContains(t, r, "record")
			},
		},
		{
			name: "recording rule chỉ record/expr/labels, không alert/for",
			give: []promrule.Group{{
				Name: "logmon-slo",
				Rules: []promrule.Rule{{
					Record: "job:slo:ratio",
					Expr:   "sum(rate(ok[5m]))",
					Labels: map[string]string{"slo": "api"},
				}},
			}},
			want: func(t *testing.T, spec map[string]any) {
				r := spec["groups"].([]any)[0].(map[string]any)["rules"].([]any)[0].(map[string]any)
				require.Equal(t, "job:slo:ratio", r["record"])
				require.NotContains(t, r, "alert")
				require.NotContains(t, r, "for")
				require.NotContains(t, r, "annotations")
			},
		},
		{
			name: "groups rỗng → spec.groups là slice rỗng (không nil)",
			give: nil,
			want: func(t *testing.T, spec map[string]any) {
				require.Equal(t, []any{}, spec["groups"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := promrule.BuildObject("rules", "logmon", tt.labels, tt.give)

			require.Equal(t, "monitoring.coreos.com/v1", obj.GetAPIVersion())
			require.Equal(t, "PrometheusRule", obj.GetKind())
			require.Equal(t, "rules", obj.GetName())
			require.Equal(t, "logmon", obj.GetNamespace())

			spec := obj.Object["spec"].(map[string]any)
			tt.want(t, spec)
		})
	}
}

func TestBuildObjectMergesLabels(t *testing.T) {
	obj := promrule.BuildObject("rules", "logmon", map[string]string{"app": "logmon"}, nil)
	require.Equal(t, "logmon", obj.GetLabels()["app"])
}
