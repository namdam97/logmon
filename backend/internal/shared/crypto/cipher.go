// Package crypto cung cấp mã hóa đối xứng at-rest cho secret (vd config kênh
// thông báo) bằng AES-256-GCM (doc_v2/09; hội đồng GĐ3: single key từ env +
// nonce 96-bit ngẫu nhiên/record + prefix key-id để xoay key dần). Thuần stdlib.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// _keyLen là độ dài key AES-256 (byte).
const _keyLen = 32

// ErrDecrypt là lỗi giải mã chung (không lộ chi tiết — chống oracle).
var ErrDecrypt = errors.New("decrypt failed")

// Cipher mã hóa/giải mã chuỗi. Định dạng ciphertext: "<keyID>:<base64(nonce|ct|tag)>".
// keyID cho phép xoay key: encrypt dùng key primary, decrypt tra theo prefix.
type Cipher struct {
	primaryID string
	byID      map[string]cipher.AEAD
}

// KeyFromPassphrase suy ra key 32 byte từ passphrase qua SHA-256 (cho phép env
// passphrase độ dài bất kỳ). Prod nên dùng passphrase entropy cao.
func KeyFromPassphrase(passphrase string) []byte {
	sum := sha256.Sum256([]byte(passphrase))
	return sum[:]
}

// NewCipher tạo Cipher với key primary (keyID + key 32 byte).
func NewCipher(keyID string, key []byte) (*Cipher, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	if keyID == "" || strings.Contains(keyID, ":") {
		return nil, fmt.Errorf("invalid key id %q", keyID)
	}
	return &Cipher{primaryID: keyID, byID: map[string]cipher.AEAD{keyID: aead}}, nil
}

// AddKey thêm một key cũ (chỉ dùng để GIẢI MÃ ciphertext đã tạo bằng key đó) —
// hỗ trợ xoay key không downtime.
func (c *Cipher) AddKey(keyID string, key []byte) error {
	aead, err := newAEAD(key)
	if err != nil {
		return err
	}
	if keyID == "" || strings.Contains(keyID, ":") {
		return fmt.Errorf("invalid key id %q", keyID)
	}
	c.byID[keyID] = aead
	return nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != _keyLen {
		return nil, fmt.Errorf("key must be %d bytes, got %d", _keyLen, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

// Encrypt mã hóa plaintext bằng key primary; trả "<keyID>:<base64>".
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	aead := c.byID[c.primaryID]
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	sealed := aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return c.primaryID + ":" + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt giải mã chuỗi "<keyID>:<base64>". Lỗi định dạng/key/auth → ErrDecrypt.
func (c *Cipher) Decrypt(ciphertext string) (string, error) {
	keyID, b64, ok := strings.Cut(ciphertext, ":")
	if !ok {
		return "", ErrDecrypt
	}
	aead, ok := c.byID[keyID]
	if !ok {
		return "", ErrDecrypt
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", ErrDecrypt
	}
	nonceSize := aead.NonceSize()
	if len(raw) < nonceSize {
		return "", ErrDecrypt
	}
	nonce, ct := raw[:nonceSize], raw[nonceSize:]
	plain, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", ErrDecrypt
	}
	return string(plain), nil
}
