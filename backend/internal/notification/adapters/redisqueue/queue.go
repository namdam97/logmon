// Package redisqueue cài đặt delivery queue cho notification BC bằng Redis Streams
// + consumer group (hội đồng GĐ3: at-least-once, ack tường minh). Retry có
// backoff dùng ZSET delayed: Enqueue(delay>0) → ZADD; Read promote item tới hạn
// sang stream trước khi XREADGROUP.
package redisqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
	"github.com/namdam97/logmon/backend/internal/notification/ports"
)

// Khóa Redis dùng chung cho delivery.
const (
	_streamKey  = "logmon:notify:stream"
	_delayedKey = "logmon:notify:delayed"
	_group      = "delivery"
	_payload    = "msg" // field name trong stream entry
)

// Queue implement ports.Enqueuer + ports.QueueConsumer trên Redis Streams.
type Queue struct {
	rdb      redis.Cmdable
	consumer string
	now      func() time.Time
}

var (
	_ ports.Enqueuer      = (*Queue)(nil)
	_ ports.QueueConsumer = (*Queue)(nil)
)

// New tạo Queue. consumer là tên consumer trong group (vd hostname). now nil →
// time.Now. Gọi EnsureGroup một lần khi khởi động.
func New(rdb redis.Cmdable, consumer string, now func() time.Time) *Queue {
	if now == nil {
		now = time.Now
	}
	return &Queue{rdb: rdb, consumer: consumer, now: now}
}

// EnsureGroup tạo consumer group (idempotent — bỏ qua BUSYGROUP). MKSTREAM tạo
// stream nếu chưa có; bắt đầu đọc từ "$" (chỉ message mới).
func (q *Queue) EnsureGroup(ctx context.Context) error {
	err := q.rdb.XGroupCreateMkStream(ctx, _streamKey, _group, "$").Err()
	if err != nil && !isBusyGroup(err) {
		return fmt.Errorf("create group: %w", err)
	}
	return nil
}

func isBusyGroup(err error) bool {
	return err != nil && err.Error() == "BUSYGROUP Consumer Group name already exists"
}

// Enqueue đẩy message vào stream (delay<=0) hoặc ZSET delayed (delay>0).
func (q *Queue) Enqueue(ctx context.Context, msg domain.Message, delay time.Duration) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	if delay <= 0 {
		return q.add(ctx, data)
	}
	readyAt := q.now().Add(delay).UnixMilli()
	if err := q.rdb.ZAdd(ctx, _delayedKey, redis.Z{Score: float64(readyAt), Member: data}).Err(); err != nil {
		return fmt.Errorf("zadd delayed: %w", err)
	}
	return nil
}

func (q *Queue) add(ctx context.Context, data []byte) error {
	if err := q.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: _streamKey,
		Values: map[string]any{_payload: data},
	}).Err(); err != nil {
		return fmt.Errorf("xadd stream: %w", err)
	}
	return nil
}

// Read promote item delayed tới hạn rồi XREADGROUP. Block tối đa block để chờ.
func (q *Queue) Read(ctx context.Context, max int, block time.Duration) ([]ports.QueueItem, error) {
	if err := q.promoteDue(ctx); err != nil {
		return nil, err
	}
	res, err := q.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    _group,
		Consumer: q.consumer,
		Streams:  []string{_streamKey, ">"},
		Count:    int64(max),
		Block:    block,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil // hết block, không có item
	}
	if err != nil {
		return nil, fmt.Errorf("xreadgroup: %w", err)
	}
	var items []ports.QueueItem
	for _, stream := range res {
		for _, m := range stream.Messages {
			raw, _ := m.Values[_payload].(string)
			var msg domain.Message
			if err := json.Unmarshal([]byte(raw), &msg); err != nil {
				// payload hỏng: ack để không kẹt, bỏ qua.
				_ = q.Ack(ctx, m.ID)
				continue
			}
			items = append(items, ports.QueueItem{ID: m.ID, Msg: msg})
		}
	}
	return items, nil
}

// promoteDue chuyển các item delayed có score <= now sang stream rồi xóa khỏi ZSET.
func (q *Queue) promoteDue(ctx context.Context) error {
	nowMs := q.now().UnixMilli()
	members, err := q.rdb.ZRangeByScore(ctx, _delayedKey, &redis.ZRangeBy{
		Min: "-inf", Max: fmt.Sprintf("%d", nowMs),
	}).Result()
	if err != nil {
		return fmt.Errorf("zrangebyscore: %w", err)
	}
	for _, member := range members {
		if err := q.add(ctx, []byte(member)); err != nil {
			return err
		}
		if err := q.rdb.ZRem(ctx, _delayedKey, member).Err(); err != nil {
			return fmt.Errorf("zrem delayed: %w", err)
		}
	}
	return nil
}

// Ack xác nhận + xóa entry khỏi stream (tránh phình PEL/stream).
func (q *Queue) Ack(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := q.rdb.XAck(ctx, _streamKey, _group, ids...).Err(); err != nil {
		return fmt.Errorf("xack: %w", err)
	}
	if err := q.rdb.XDel(ctx, _streamKey, ids...).Err(); err != nil {
		return fmt.Errorf("xdel: %w", err)
	}
	return nil
}
