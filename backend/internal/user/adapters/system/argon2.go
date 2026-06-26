package system

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"

	"github.com/namdam97/logmon/backend/internal/user/ports"
)

// Tham số argon2id theo OWASP minimum (ADR-022): m=19 MiB, t=2, p=1, output 32B,
// salt 16B. Đủ mạnh cho server-side, vẫn đáp ứng được latency login.
const (
	_argon2Memory    = 19456 // KiB (19 MiB)
	_argon2Time      = 2     // số lần lặp
	_argon2Threads   = 1     // parallelism
	_argon2KeyLen    = 32    // độ dài hash (bytes)
	_argon2SaltLen   = 16    // độ dài salt (bytes)
	_argon2MaxKeyLen = 1024  // chặn trên khi parse hash lạ (an toàn ép int→uint32)
	_argon2Version   = argon2.Version
)

// errUnknownHashFormat: hash trong DB không phải argon2id cũng không phải bcrypt.
var errUnknownHashFormat = errors.New("unknown password hash format")

// Argon2idHasher băm mật khẩu bằng argon2id và verify được cả hash bcrypt cũ
// (lazy migration — ADR-022). Tham số được nhúng trong PHC string nên hash cũ
// vẫn verify đúng kể cả khi tham số mặc định thay đổi về sau.
type Argon2idHasher struct {
	memory  uint32
	time    uint32
	threads uint8
	keyLen  uint32
	saltLen uint32
}

var _ ports.PasswordHasher = (*Argon2idHasher)(nil)

// NewArgon2idHasher tạo hasher với tham số OWASP minimum.
func NewArgon2idHasher() *Argon2idHasher {
	return &Argon2idHasher{
		memory:  _argon2Memory,
		time:    _argon2Time,
		threads: _argon2Threads,
		keyLen:  _argon2KeyLen,
		saltLen: _argon2SaltLen,
	}
}

// Hash sinh salt ngẫu nhiên (crypto/rand) và trả về PHC string argon2id.
func (h *Argon2idHasher) Hash(plain string) (string, error) {
	salt := make([]byte, h.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}
	key := argon2.IDKey([]byte(plain), salt, h.time, h.memory, h.threads, h.keyLen)
	return encodePHC(h.memory, h.time, h.threads, salt, key), nil
}

// Verify so khớp plain với hash. Hỗ trợ cả argon2id (PHC) và bcrypt (legacy) để
// phục vụ lazy migration. Mọi so khớp dùng constant-time để chống timing attack.
func (h *Argon2idHasher) Verify(hash, plain string) error {
	switch {
	case strings.HasPrefix(hash, "$argon2id$"):
		return verifyArgon2id(hash, plain)
	case strings.HasPrefix(hash, "$2"): // $2a$ / $2b$ / $2y$ — bcrypt
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)); err != nil {
			return fmt.Errorf("bcrypt compare: %w", err)
		}
		return nil
	default:
		return errUnknownHashFormat
	}
}

// NeedsRehash báo true khi hash không phải argon2id hiện hành (bcrypt cũ, định
// dạng lạ, hoặc tham số yếu hơn cấu hình hiện tại) — caller re-hash sau khi
// login thành công.
func (h *Argon2idHasher) NeedsRehash(hash string) bool {
	p, err := parseArgon2id(hash)
	if err != nil {
		return true // bcrypt hoặc định dạng không nhận diện được → nâng cấp
	}
	return p.memory < h.memory || p.time < h.time || p.threads != h.threads || p.keyLen != int(h.keyLen)
}

// argon2Params là tham số trích từ một PHC string.
type argon2Params struct {
	memory  uint32
	time    uint32
	threads uint8
	keyLen  int // dùng int để so khớp len(key) trực tiếp, tránh narrowing
}

func encodePHC(memory, time uint32, threads uint8, salt, key []byte) string {
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		_argon2Version, memory, time, threads,
		b64.EncodeToString(salt), b64.EncodeToString(key))
}

// parseArgon2id giải mã PHC string argon2id thành tham số + salt + key.
func parseArgon2id(hash string) (params argon2Params, err error) {
	parts := strings.Split(hash, "$")
	// ["", "argon2id", "v=19", "m=...,t=...,p=...", salt, key]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argon2Params{}, errUnknownHashFormat
	}
	var version int
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != _argon2Version {
		return argon2Params{}, errUnknownHashFormat
	}
	var memory, time uint32
	var threads uint8
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return argon2Params{}, errUnknownHashFormat
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2Params{}, errUnknownHashFormat
	}
	return argon2Params{memory: memory, time: time, threads: threads, keyLen: len(key)}, nil
}

func verifyArgon2id(hash, plain string) error {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		return errUnknownHashFormat
	}
	params, err := parseArgon2id(hash)
	if err != nil {
		return err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return errUnknownHashFormat
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return errUnknownHashFormat
	}
	// keyLen bị chặn trên để vừa hợp lệ hoá hash vừa an toàn khi ép int → uint32.
	if params.keyLen <= 0 || params.keyLen > _argon2MaxKeyLen {
		return errUnknownHashFormat
	}
	got := argon2.IDKey([]byte(plain), salt, params.time, params.memory, params.threads, uint32(params.keyLen))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return errors.New("argon2id mismatch")
	}
	return nil
}
