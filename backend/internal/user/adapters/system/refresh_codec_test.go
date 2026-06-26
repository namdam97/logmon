package system_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/user/adapters/system"
)

func TestRefreshCodec_GenerateProducesUniqueTokens(t *testing.T) {
	c := system.NewRefreshCodec()

	a, err := c.Generate()
	require.NoError(t, err)
	b, err := c.Generate()
	require.NoError(t, err)

	require.NotEmpty(t, a)
	require.NotEqual(t, a, b, "mỗi token phải ngẫu nhiên")
}

func TestRefreshCodec_HashIsDeterministicHex(t *testing.T) {
	c := system.NewRefreshCodec()

	h1 := c.Hash("token-abc")
	h2 := c.Hash("token-abc")

	require.Equal(t, h1, h2, "hash phải tất định")
	require.Len(t, h1, 64, "sha256 hex = 64 ký tự")
	require.NotEqual(t, h1, c.Hash("token-xyz"))
}
