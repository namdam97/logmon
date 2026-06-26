package system

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestArgon2idHasher_HashProducesPHCFormat(t *testing.T) {
	h := NewArgon2idHasher()

	hash, err := h.Hash("correct horse battery staple")

	require.NoError(t, err)
	require.True(t, strings.HasPrefix(hash, "$argon2id$v=19$"),
		"hash phải ở PHC format argon2id, got %q", hash)
	require.Contains(t, hash, "m=19456,t=2,p=1")
}

func TestArgon2idHasher_HashIsSalted(t *testing.T) {
	h := NewArgon2idHasher()

	first, err := h.Hash("same-password")
	require.NoError(t, err)
	second, err := h.Hash("same-password")
	require.NoError(t, err)

	require.NotEqual(t, first, second, "salt ngẫu nhiên → hai hash khác nhau")
}

func TestArgon2idHasher_VerifyAcceptsCorrectPassword(t *testing.T) {
	h := NewArgon2idHasher()
	hash, err := h.Hash("s3cr3t-pass")
	require.NoError(t, err)

	require.NoError(t, h.Verify(hash, "s3cr3t-pass"))
}

func TestArgon2idHasher_VerifyRejectsWrongPassword(t *testing.T) {
	h := NewArgon2idHasher()
	hash, err := h.Hash("s3cr3t-pass")
	require.NoError(t, err)

	require.Error(t, h.Verify(hash, "wrong-pass"))
}

func TestArgon2idHasher_VerifyAcceptsLegacyBcrypt(t *testing.T) {
	// Lazy migration: hash bcrypt cũ trong DB vẫn verify được.
	h := NewArgon2idHasher()
	bcryptHash, err := bcrypt.GenerateFromPassword([]byte("legacy-pass"), bcrypt.MinCost)
	require.NoError(t, err)

	require.NoError(t, h.Verify(string(bcryptHash), "legacy-pass"))
	require.Error(t, h.Verify(string(bcryptHash), "wrong-pass"))
}

func TestArgon2idHasher_VerifyRejectsUnknownFormat(t *testing.T) {
	h := NewArgon2idHasher()

	require.Error(t, h.Verify("not-a-real-hash", "whatever"))
}

func TestArgon2idHasher_NeedsRehash(t *testing.T) {
	h := NewArgon2idHasher()

	bcryptHash, err := bcrypt.GenerateFromPassword([]byte("p"), bcrypt.MinCost)
	require.NoError(t, err)
	argonHash, err := h.Hash("p")
	require.NoError(t, err)

	tests := []struct {
		name string
		give string
		want bool
	}{
		{name: "bcrypt cần nâng cấp", give: string(bcryptHash), want: true},
		{name: "argon2id hiện hành không cần", give: argonHash, want: false},
		{name: "định dạng lạ cần re-hash", give: "garbage", want: true},
		{
			name: "argon2id tham số yếu cần nâng cấp",
			give: "$argon2id$v=19$m=8,t=1,p=1$YWJjZGVmZ2hpamtsbW5vcA$qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, h.NeedsRehash(tt.give))
		})
	}
}
