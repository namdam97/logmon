package promfile

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/slo/domain"
)

var update = flag.Bool("update", false, "cập nhật golden files")

func mustSLO(t *testing.T, in domain.NewSLOInput) domain.SLO {
	t.Helper()
	s, err := domain.NewSLO(in)
	require.NoError(t, err)
	return s
}

// TestRenderSLOsGolden render SLO availability + latency và đối chiếu golden file;
// đồng thời chạy rulefmt.validate để bảo đảm PromQL/YAML sinh ra hợp lệ (lưới an
// toàn khử rủi ro sai công thức — điều kiện hội đồng GĐ3).
func TestRenderSLOsGolden(t *testing.T) {
	ws := "00000000-0000-0000-0000-000000000001"
	idA, _ := domain.NewSLOID("11111111-1111-1111-1111-111111111111")
	idL, _ := domain.NewSLOID("22222222-2222-2222-2222-222222222222")
	created := time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC)

	slos := []domain.SLO{
		mustSLO(t, domain.NewSLOInput{
			ID: idA, WorkspaceID: ws, Name: "checkout availability", Service: "checkout",
			SLIType: domain.SLIAvailability, Target: 0.999, WindowDays: 28, CreatedAt: created,
		}),
		mustSLO(t, domain.NewSLOInput{
			ID: idL, WorkspaceID: ws, Name: "api latency", Service: "api",
			SLIType: domain.SLILatency, LatencyThresholdMs: 250, Target: 0.99, WindowDays: 28, CreatedAt: created,
		}),
	}

	got, err := renderSLOs(slos)
	require.NoError(t, err)

	// Lưới an toàn: output phải pass rulefmt (PromQL + cấu trúc hợp lệ).
	require.NoError(t, validate(got))

	golden := filepath.Join("testdata", "slo_rules.golden.yml")
	if *update {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(golden, got, 0o644))
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err, "thiếu golden file — chạy: go test -run TestRenderSLOsGolden -update")
	require.Equal(t, string(want), string(got))
}
