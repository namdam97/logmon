package system_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/namdam97/logmon/backend/internal/user/adapters/system"
)

func TestBcryptHasher(t *testing.T) {
	h := system.NewBcryptHasher(bcrypt.MinCost)

	hash, err := h.Hash("password123")
	require.NoError(t, err)
	require.NotEqual(t, "password123", hash)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(hash), []byte("password123")))
}

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
