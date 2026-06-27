package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/shared/crypto"
)

func newTestCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	c, err := crypto.NewCipher("v1", crypto.KeyFromPassphrase("test-passphrase"))
	require.NoError(t, err)
	return c
}

func TestEncryptDecryptConfigRoundTrip(t *testing.T) {
	r := &ChannelRepository{cipher: newTestCipher(t)}
	config := map[string]string{"webhook_url": "https://hooks.slack.com/secret", "token": "xoxb-1"}

	enc, err := r.encryptConfig(config)
	require.NoError(t, err)
	require.NotContains(t, enc, "xoxb-1", "secret not stored in plaintext")
	require.NotContains(t, enc, "hooks.slack.com")

	got, err := r.decryptConfig(enc)
	require.NoError(t, err)
	require.Equal(t, config, got)
}

func TestDecryptConfigRejectsTampered(t *testing.T) {
	r := &ChannelRepository{cipher: newTestCipher(t)}
	enc, err := r.encryptConfig(map[string]string{"k": "v"})
	require.NoError(t, err)

	_, err = r.decryptConfig(enc + "tampered")
	require.Error(t, err)
}

func TestDecryptConfigWrongKeyFails(t *testing.T) {
	r1 := &ChannelRepository{cipher: newTestCipher(t)}
	enc, err := r1.encryptConfig(map[string]string{"k": "v"})
	require.NoError(t, err)

	other, err := crypto.NewCipher("v1", crypto.KeyFromPassphrase("different"))
	require.NoError(t, err)
	r2 := &ChannelRepository{cipher: other}

	_, err = r2.decryptConfig(enc)
	require.Error(t, err)
}
