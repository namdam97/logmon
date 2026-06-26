package system_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/user/adapters/system"
)

func TestUUIDGeneratorProducesUnique(t *testing.T) {
	g := system.NewUUIDGenerator()
	a, b := g.NewID(), g.NewID()
	require.NotEmpty(t, a)
	require.NotEqual(t, a, b)
}

func TestClockReturnsUTC(t *testing.T) {
	c := system.NewClock()
	now := c.Now()
	require.Equal(t, "UTC", now.Location().String())
}
