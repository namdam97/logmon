package crypto_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/crypto"
)

func newCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	c, err := crypto.NewCipher("v1", crypto.KeyFromPassphrase("test-passphrase"))
	require.NoError(t, err)
	return c
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c := newCipher(t)

	enc, err := c.Encrypt("https://hooks.slack.com/services/SECRET")
	require.NoError(t, err)

	dec, err := c.Decrypt(enc)
	require.NoError(t, err)
	require.Equal(t, "https://hooks.slack.com/services/SECRET", dec)
}

func TestEncryptHasKeyIDPrefix(t *testing.T) {
	c := newCipher(t)

	enc, err := c.Encrypt("secret")
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(enc, "v1:"), "got %q", enc)
}

func TestEncryptNonDeterministic(t *testing.T) {
	c := newCipher(t)

	a, err := c.Encrypt("same")
	require.NoError(t, err)
	b, err := c.Encrypt("same")
	require.NoError(t, err)

	require.NotEqual(t, a, b, "nonce ngẫu nhiên → ciphertext phải khác nhau")
}

func TestEmptyPlaintextRoundTrips(t *testing.T) {
	c := newCipher(t)

	enc, err := c.Encrypt("")
	require.NoError(t, err)
	dec, err := c.Decrypt(enc)
	require.NoError(t, err)
	require.Equal(t, "", dec)
}

func TestDecryptTamperedFails(t *testing.T) {
	c := newCipher(t)
	enc, err := c.Encrypt("secret")
	require.NoError(t, err)

	tampered := enc[:len(enc)-2] + "xy"
	_, err = c.Decrypt(tampered)
	require.Error(t, err)
}

func TestDecryptUnknownKeyIDFails(t *testing.T) {
	c := newCipher(t)

	_, err := c.Decrypt("v999:YWJjZA==")
	require.Error(t, err)
}

func TestDecryptMalformedFails(t *testing.T) {
	c := newCipher(t)

	_, err := c.Decrypt("no-prefix-no-colon")
	require.Error(t, err)
}

func TestNewCipherRejectsBadKeyLength(t *testing.T) {
	_, err := crypto.NewCipher("v1", []byte("too-short"))
	require.Error(t, err)
}

func TestRotationDecryptsOldKey(t *testing.T) {
	// Mã hóa bằng key cũ (v1), thêm key mới (v2) làm primary, vẫn giải mã được v1.
	old, err := crypto.NewCipher("v1", crypto.KeyFromPassphrase("old"))
	require.NoError(t, err)
	enc, err := old.Encrypt("secret")
	require.NoError(t, err)

	rotated, err := crypto.NewCipher("v2", crypto.KeyFromPassphrase("new"))
	require.NoError(t, err)
	require.NoError(t, rotated.AddKey("v1", crypto.KeyFromPassphrase("old")))

	dec, err := rotated.Decrypt(enc)
	require.NoError(t, err)
	require.Equal(t, "secret", dec)
}
