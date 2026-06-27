package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseMode(t *testing.T) {
	tests := []struct {
		give    string
		want    Mode
		wantErr bool
	}{
		{give: "A", want: ModeA},
		{give: "b", want: ModeB},
		{give: " B ", want: ModeB},
		{give: "C", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			got, err := ParseMode(tt.give)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.want.String(), got.String())
		})
	}
}

func TestILMPolicyValidation(t *testing.T) {
	now := time.Unix(1, 0).UTC()
	cfg := DefaultPipelineConfig("ws-1", now)
	require.Equal(t, ModeA, cfg.Mode())
	require.Equal(t, 7, cfg.ILM().HotDays)

	tests := []struct {
		name    string
		give    ILMPolicy
		wantErr bool
	}{
		{name: "valid", give: ILMPolicy{HotDays: 7, WarmDays: 30, DeleteDays: 90}},
		{name: "hot zero", give: ILMPolicy{HotDays: 0, WarmDays: 30, DeleteDays: 90}, wantErr: true},
		{name: "warm <= hot", give: ILMPolicy{HotDays: 7, WarmDays: 7, DeleteDays: 90}, wantErr: true},
		{name: "delete <= warm", give: ILMPolicy{HotDays: 7, WarmDays: 30, DeleteDays: 30}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cfg.WithILM(tt.give, "u-1", now)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestWithModeImmutable(t *testing.T) {
	now := time.Unix(1, 0).UTC()
	cfg := DefaultPipelineConfig("ws-1", now)
	switched, err := cfg.WithMode(ModeB, "u-9", now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, ModeB, switched.Mode())
	require.Equal(t, "u-9", switched.UpdatedBy())
	require.Equal(t, ModeA, cfg.Mode()) // bản gốc không đổi
}

func TestDLQMarkRetried(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	e := ReconstructDLQEntry(1, "ws-1", "raw", "es reject", "svc", 0, DLQPending, now, nil)
	r, err := e.MarkRetried(now.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, DLQRetried, r.Status())
	require.Equal(t, 1, r.RetryCount())
	require.NotNil(t, r.RetriedAt())
	require.Equal(t, DLQPending, e.Status()) // bất biến

	// retry lần nữa entry đã retried → lỗi
	_, err = r.MarkRetried(now)
	require.ErrorIs(t, err, ErrDLQNotRetryable)
}

func TestParseDLQStatus(t *testing.T) {
	for _, s := range []DLQStatus{DLQPending, DLQRetried, DLQDiscarded} {
		got, err := ParseDLQStatus(s.String())
		require.NoError(t, err)
		require.Equal(t, s, got)
	}
	_, err := ParseDLQStatus("bogus")
	require.Error(t, err)
}
