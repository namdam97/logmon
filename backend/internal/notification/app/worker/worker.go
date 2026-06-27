package worker

import (
	"context"
	"time"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
	"github.com/namdam97/logmon/backend/internal/notification/ports"
)

// Tuning mặc định cho delivery.
const (
	_defaultReadBatch = 16
	_defaultReadBlock = 5 * time.Second
	_breakerThreshold = 5
	_breakerCooldown  = 30 * time.Second
)

// _retryBackoff: lịch retry theo số lần đã thử (hội đồng GĐ3: ngay→30s→2m). Hết
// danh sách → bỏ cuộc (history failed). len = số lần thử lại tối đa.
var _retryBackoff = []time.Duration{0, 30 * time.Second, 2 * time.Minute}

// Logger là cổng log tối thiểu của worker.
type Logger interface {
	Info(ctx context.Context, msg string)
	Error(ctx context.Context, err error, msg string)
}

// Worker tiêu thụ delivery queue, gọi Sender theo channel type, retry có backoff,
// circuit breaker per-channel, ghi history mỗi kết quả terminal.
type Worker struct {
	queue   ports.QueueConsumer
	enq     ports.Enqueuer
	senders map[string]ports.Sender
	history ports.HistoryWriter
	breaker *Breaker
	clock   ports.Clock
	log     Logger
}

// NewWorker tạo worker. senders map theo channel type (slack/webhook/...).
func NewWorker(queue ports.QueueConsumer, enq ports.Enqueuer, senders map[string]ports.Sender, history ports.HistoryWriter, clock ports.Clock, log Logger) *Worker {
	return &Worker{
		queue:   queue,
		enq:     enq,
		senders: senders,
		history: history,
		breaker: NewBreaker(_breakerThreshold, _breakerCooldown, clock.Now),
		clock:   clock,
		log:     log,
	}
}

// Run vòng lặp tiêu thụ tới khi ctx hủy. Lỗi Read tạm thời được log và thử lại.
func (w *Worker) Run(ctx context.Context) {
	if w.log != nil {
		w.log.Info(ctx, "notification delivery worker started")
	}
	for {
		if ctx.Err() != nil {
			return
		}
		items, err := w.queue.Read(ctx, _defaultReadBatch, _defaultReadBlock)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if w.log != nil {
				w.log.Error(ctx, err, "read delivery queue")
			}
			continue
		}
		for _, it := range items {
			w.process(ctx, it)
		}
	}
}

// process xử lý một item: gửi, retry/bỏ cuộc, ghi history; luôn Ack để không kẹt
// queue (retry được tái-enqueue như item mới — at-least-once).
func (w *Worker) process(ctx context.Context, it ports.QueueItem) {
	msg := it.Msg
	defer w.ack(ctx, it.ID)

	sender, ok := w.senders[msg.ChannelType]
	if !ok {
		w.record(ctx, msg, domain.StatusFailed, "unsupported channel type: "+msg.ChannelType)
		return
	}

	if !w.breaker.Allow(msg.ChannelID) {
		// Mạch mở: hoãn (re-enqueue với backoff) thay vì đốt budget retry.
		w.requeue(ctx, msg, _breakerCooldown)
		return
	}

	err := sender.Send(ctx, msg)
	if err == nil {
		w.breaker.Success(msg.ChannelID)
		w.record(ctx, msg, domain.StatusSent, "")
		return
	}

	w.breaker.Failure(msg.ChannelID)
	next := msg.Attempt + 1
	if next < len(_retryBackoff) {
		w.record(ctx, msg, domain.StatusRetrying, err.Error())
		w.requeue(ctx, msg, _retryBackoff[next])
		return
	}
	w.record(ctx, msg, domain.StatusFailed, err.Error())
}

func (w *Worker) requeue(ctx context.Context, msg domain.Message, delay time.Duration) {
	retry := msg
	retry.Attempt++
	if err := w.enq.Enqueue(ctx, retry, delay); err != nil && w.log != nil {
		w.log.Error(ctx, err, "re-enqueue retry")
	}
}

func (w *Worker) ack(ctx context.Context, id string) {
	if err := w.queue.Ack(ctx, id); err != nil && w.log != nil {
		w.log.Error(ctx, err, "ack delivery item")
	}
}

func (w *Worker) record(ctx context.Context, msg domain.Message, status domain.DeliveryStatus, errMsg string) {
	entry := domain.HistoryEntry{
		WorkspaceID:  msg.WorkspaceID,
		ChannelID:    msg.ChannelID,
		EventType:    msg.EventType,
		EventRef:     msg.EventRef,
		Status:       status,
		ErrorMessage: errMsg,
		SentAt:       w.clock.Now(),
	}
	if err := w.history.Save(ctx, entry); err != nil && w.log != nil {
		w.log.Error(ctx, err, "save delivery history")
	}
}
