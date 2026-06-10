package postgres

import (
	"fmt"
	"time"

	"github.com/namdam97/logmon/backend/internal/user/domain"
)

// reconstruct dựng lại domain.User từ dữ liệu thô trong DB, đi qua các value
// object để dữ liệu lưu trữ vẫn được validate khi đọc ra.
func reconstruct(rawID, rawEmail, hash string, createdAt time.Time) (domain.User, error) {
	id, err := domain.NewUserID(rawID)
	if err != nil {
		return domain.User{}, fmt.Errorf("reconstruct id: %w", err)
	}
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return domain.User{}, fmt.Errorf("reconstruct email: %w", err)
	}
	user, err := domain.NewUser(id, email, hash, createdAt.UTC())
	if err != nil {
		return domain.User{}, fmt.Errorf("reconstruct user: %w", err)
	}
	return user, nil
}
