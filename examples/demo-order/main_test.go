// main_test.go — Tests cho config loading và helper functions.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvOr(t *testing.T) {
	tests := []struct {
		give     string
		key      string
		envVal   string
		fallback string
		want     string
	}{
		{
			give:     "biến không set → trả fallback",
			key:      "TEST_ENVKEY_NOTSET_XYZ",
			envVal:   "",
			fallback: "default",
			want:     "default",
		},
		{
			give:     "biến có giá trị → trả env value",
			key:      "TEST_ENVKEY_SET_XYZ",
			envVal:   "custom",
			fallback: "default",
			want:     "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv(tt.key, tt.envVal)
			}
			got := envOr(tt.key, tt.fallback)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		give      string
		envs      map[string]string
		wantPort  string
		wantErr   float64
		wantLatMS int
		wantLevel string
	}{
		{
			give:      "mặc định khi không set env",
			envs:      map[string]string{},
			wantPort:  "8081",
			wantErr:   0.02,
			wantLatMS: 0,
			wantLevel: "info",
		},
		{
			give:      "override tất cả env",
			envs:      map[string]string{"PORT": "9090", "ERROR_RATE": "0.5", "EXTRA_LATENCY_MS": "100", "LOG_LEVEL": "debug"},
			wantPort:  "9090",
			wantErr:   0.5,
			wantLatMS: 100,
			wantLevel: "debug",
		},
		{
			give:      "ERROR_RATE không hợp lệ → giữ mặc định",
			envs:      map[string]string{"ERROR_RATE": "not-a-float"},
			wantPort:  "8081",
			wantErr:   0.02,
			wantLatMS: 0,
			wantLevel: "info",
		},
		{
			give:      "ERROR_RATE > 1.0 → giữ mặc định",
			envs:      map[string]string{"ERROR_RATE": "1.5"},
			wantPort:  "8081",
			wantErr:   0.02,
			wantLatMS: 0,
			wantLevel: "info",
		},
		{
			give:      "EXTRA_LATENCY_MS âm → giữ mặc định",
			envs:      map[string]string{"EXTRA_LATENCY_MS": "-10"},
			wantPort:  "8081",
			wantErr:   0.02,
			wantLatMS: 0,
			wantLevel: "info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			// Xóa các env liên quan trước mỗi test case.
			for _, k := range []string{"PORT", "ERROR_RATE", "EXTRA_LATENCY_MS", "LOG_LEVEL"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.envs {
				t.Setenv(k, v)
			}

			cfg := loadConfig()
			require.Equal(t, tt.wantPort, cfg.port)
			require.InDelta(t, tt.wantErr, cfg.errorRate, 1e-9)
			require.Equal(t, tt.wantLatMS, cfg.extraLatencyMS)
			require.Equal(t, tt.wantLevel, cfg.logLevel)
		})
	}
}

func TestLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	log := newLogger(&buf, "info")
	log.Info("test message", "key1", "val1")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	require.Equal(t, "test message", entry["message"])
	require.Equal(t, "demo-order", entry["service"])
	require.Equal(t, "val1", entry["key1"])
}

func TestLogger_Error(t *testing.T) {
	tests := []struct {
		give    string
		err     error
		wantKey bool // có field "error" trong JSON không
	}{
		{
			give:    "err nil → log không có error field",
			err:     nil,
			wantKey: false,
		},
		{
			give:    "err có giá trị → log có error field",
			err:     errors.New("lỗi giả"),
			wantKey: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			var buf bytes.Buffer
			log := newLogger(&buf, "info")
			log.Error("lỗi test", tt.err)

			var entry map[string]any
			require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
			require.Equal(t, "lỗi test", entry["message"])
			require.Equal(t, "demo-order", entry["service"])
			if tt.wantKey {
				require.Contains(t, entry, "error")
			}
		})
	}
}

func TestLogger_NilWriter(t *testing.T) {
	// newLogger với nil writer không panic.
	require.NotPanics(t, func() {
		_ = newLogger(nil, "info")
	})
}

func TestLogger_InvalidLevel(t *testing.T) {
	var buf bytes.Buffer
	// Level không hợp lệ → fallback info, không panic.
	log := newLogger(&buf, "not-a-level")
	log.Info("ok")
	require.Contains(t, buf.String(), "ok")
}

func TestLogger_Request(t *testing.T) {
	var buf bytes.Buffer
	log := newLogger(&buf, "info")
	log.Request("GET", "/api/v1/orders", 200, 15)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	require.Equal(t, "GET", entry["method"])
	require.Equal(t, "/api/v1/orders", entry["path"])
	require.Equal(t, float64(200), entry["status"])
	require.Equal(t, float64(15), entry["duration_ms"])
}

func TestLogger_ServiceField(t *testing.T) {
	var buf bytes.Buffer
	log := newLogger(&buf, "info")
	log.Info("check service field")

	require.True(t, strings.Contains(buf.String(), `"service":"demo-order"`))
}
