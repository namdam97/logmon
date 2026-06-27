package audit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNullStr(t *testing.T) {
	require.Nil(t, nullStr(""))
	got := nullStr("x")
	require.NotNil(t, got)
	require.Equal(t, "x", *got)
}

func TestNopRecorder(t *testing.T) {
	var r Recorder = NopRecorder{}
	require.NoError(t, r.Record(context.Background(), Entry{Action: "x", ResourceType: "y", ResourceID: "z"}))
}
