package domain

import (
	"regexp"
	"strings"
)

// emailPattern là kiểm tra cú pháp email tối giản, đủ cho biên nhập liệu;
// việc xác thực thật sự do luồng verification đảm nhận (ngoài phạm vi skeleton).
var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

const maxEmailLength = 254 // RFC 5321 giới hạn độ dài đường dẫn email.

// Email là value object: bất biến và luôn hợp lệ sau khi khởi tạo thành công.
type Email struct {
	value string
}

// NewEmail chuẩn hoá (trim + lowercase) và validate email. Trả về
// *ValidationError nếu rỗng, quá dài hoặc sai cú pháp.
func NewEmail(raw string) (Email, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case v == "":
		return Email{}, newValidationError("email", "must not be empty")
	case len(v) > maxEmailLength:
		return Email{}, newValidationError("email", "exceeds maximum length")
	case !emailPattern.MatchString(v):
		return Email{}, newValidationError("email", "invalid format")
	}
	return Email{value: v}, nil
}

// String trả về biểu diễn chuỗi đã chuẩn hoá của email.
func (e Email) String() string { return e.value }
