package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
	"github.com/namdam97/logmon/backend/internal/notification/ports"
)

const uniqueViolationCode = "23505"

const channelColumns = `id, workspace_id, name, channel_type, config_encrypted, events, enabled, created_at, updated_at`

// Cipher mã hóa/giải mã blob config (shared/crypto.Cipher thỏa interface này).
type Cipher interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// ChannelRepository lưu trữ + đọc channel; config mã hóa at-rest qua Cipher.
type ChannelRepository struct {
	pool   *pgxpool.Pool
	cipher Cipher
}

var (
	_ ports.ChannelRepository = (*ChannelRepository)(nil)
	_ ports.ChannelReader     = (*ChannelRepository)(nil)
)

// NewChannelRepository tạo repository với pool + cipher.
func NewChannelRepository(pool *pgxpool.Pool, cipher Cipher) *ChannelRepository {
	return &ChannelRepository{pool: pool, cipher: cipher}
}

// Save chèn channel mới; vi phạm UNIQUE(ws,name) → ErrChannelNameTaken.
func (r *ChannelRepository) Save(ctx context.Context, c domain.Channel) error {
	enc, err := r.encryptConfig(c.Config())
	if err != nil {
		return err
	}
	const q = `INSERT INTO notification_channels
		(id, workspace_id, name, channel_type, config_encrypted, events, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err = dbFrom(ctx, r.pool).Exec(ctx, q,
		c.ID().String(), c.WorkspaceID(), c.Name(), c.Type().String(), enc,
		c.Events(), c.IsEnabled(), c.CreatedAt(), c.UpdatedAt())
	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrChannelNameTaken
		}
		return fmt.Errorf("insert channel: %w", err)
	}
	return nil
}

// Update ghi đè channel theo id.
func (r *ChannelRepository) Update(ctx context.Context, c domain.Channel) error {
	enc, err := r.encryptConfig(c.Config())
	if err != nil {
		return err
	}
	const q = `UPDATE notification_channels SET
		name = $2, channel_type = $3, config_encrypted = $4, events = $5, enabled = $6, updated_at = $7
		WHERE id = $1`
	tag, err := dbFrom(ctx, r.pool).Exec(ctx, q,
		c.ID().String(), c.Name(), c.Type().String(), enc, c.Events(), c.IsEnabled(), c.UpdatedAt())
	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrChannelNameTaken
		}
		return fmt.Errorf("update channel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrChannelNotFound
	}
	return nil
}

// Delete xóa channel theo workspace + id; ErrChannelNotFound nếu không có.
func (r *ChannelRepository) Delete(ctx context.Context, workspaceID string, id domain.ChannelID) error {
	const q = `DELETE FROM notification_channels WHERE workspace_id = $1 AND id = $2`
	tag, err := dbFrom(ctx, r.pool).Exec(ctx, q, workspaceID, id.String())
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrChannelNotFound
	}
	return nil
}

// ExistsByName kiểm trùng tên trong workspace.
func (r *ChannelRepository) ExistsByName(ctx context.Context, workspaceID, name string) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM notification_channels WHERE workspace_id = $1 AND name = $2)`
	var exists bool
	if err := dbFrom(ctx, r.pool).QueryRow(ctx, q, workspaceID, name).Scan(&exists); err != nil {
		return false, fmt.Errorf("exists by name: %w", err)
	}
	return exists, nil
}

// ByID đọc channel theo workspace + id (config đã giải mã).
func (r *ChannelRepository) ByID(ctx context.Context, workspaceID string, id domain.ChannelID) (domain.Channel, error) {
	const q = `SELECT ` + channelColumns + ` FROM notification_channels WHERE workspace_id = $1 AND id = $2`
	c, err := r.scanChannel(dbFrom(ctx, r.pool).QueryRow(ctx, q, workspaceID, id.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Channel{}, domain.ErrChannelNotFound
	}
	return c, err
}

// List đọc các channel của workspace (sắp theo name).
func (r *ChannelRepository) List(ctx context.Context, workspaceID string) ([]domain.Channel, error) {
	const q = `SELECT ` + channelColumns + ` FROM notification_channels WHERE workspace_id = $1 ORDER BY name`
	return r.queryChannels(ctx, q, workspaceID)
}

// SubscribedTo trả các channel đang bật đăng ký eventType (config đã giải mã).
func (r *ChannelRepository) SubscribedTo(ctx context.Context, workspaceID, eventType string) ([]domain.Channel, error) {
	const q = `SELECT ` + channelColumns + ` FROM notification_channels
		WHERE workspace_id = $1 AND enabled = true AND $2 = ANY(events) ORDER BY name`
	return r.queryChannels(ctx, q, workspaceID, eventType)
}

func (r *ChannelRepository) queryChannels(ctx context.Context, q string, args ...any) ([]domain.Channel, error) {
	rows, err := dbFrom(ctx, r.pool).Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query channels: %w", err)
	}
	defer rows.Close()

	var channels []domain.Channel
	for rows.Next() {
		c, err := r.scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate channels: %w", err)
	}
	return channels, nil
}

type scanRow interface {
	Scan(dest ...any) error
}

func (r *ChannelRepository) scanChannel(row scanRow) (domain.Channel, error) {
	var (
		rawID, workspaceID, name, typeStr, enc string
		events                                 []string
		enabled                                bool
		createdAt, updatedAt                   time.Time
	)
	if err := row.Scan(&rawID, &workspaceID, &name, &typeStr, &enc, &events, &enabled, &createdAt, &updatedAt); err != nil {
		return domain.Channel{}, err
	}
	id, err := domain.NewChannelID(rawID)
	if err != nil {
		return domain.Channel{}, fmt.Errorf("reconstruct id: %w", err)
	}
	ct, err := domain.NewChannelType(typeStr)
	if err != nil {
		return domain.Channel{}, fmt.Errorf("reconstruct type: %w", err)
	}
	config, err := r.decryptConfig(enc)
	if err != nil {
		return domain.Channel{}, err
	}
	return domain.Reconstruct(domain.ReconstructInput{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        name,
		ChannelType: ct,
		Config:      config,
		Events:      events,
		Enabled:     enabled,
		CreatedAt:   createdAt.UTC(),
		UpdatedAt:   updatedAt.UTC(),
	}), nil
}

// encryptConfig JSON-marshal map rồi mã hóa thành một blob.
func (r *ChannelRepository) encryptConfig(config map[string]string) (string, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	enc, err := r.cipher.Encrypt(string(data))
	if err != nil {
		return "", fmt.Errorf("encrypt config: %w", err)
	}
	return enc, nil
}

// decryptConfig giải mã blob rồi unmarshal về map.
func (r *ChannelRepository) decryptConfig(enc string) (map[string]string, error) {
	plain, err := r.cipher.Decrypt(enc)
	if err != nil {
		return nil, fmt.Errorf("decrypt config: %w", err)
	}
	var config map[string]string
	if err := json.Unmarshal([]byte(plain), &config); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return config, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode
}
