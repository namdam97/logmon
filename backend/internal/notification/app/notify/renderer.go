// Package notify chứa use case gửi thông báo (write-side application service)
// của notification BC: render template theo event type rồi enqueue cho từng kênh
// đăng ký. Worker (cùng package) tiêu thụ queue và gọi Sender.
package notify

import (
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
)

// _templates ánh xạ event type → (subject, body). Dùng text/template (KHÔNG
// html — payload là text/JSON cho Slack/webhook/email, không render vào DOM).
// Data là map[string]string truy cập qua {{.field}}.
var _templates = map[string]struct{ subject, body string }{
	domain.EventAlertFired: {
		subject: "🔥 [{{.severity}}] {{.alertName}}",
		body:    "Cảnh báo {{.alertName}} đã kích hoạt cho service {{.service}}.\nGiá trị: {{.value}} (ngưỡng {{.threshold}}).\nThời điểm: {{.firedAt}}.",
	},
	domain.EventAlertResolved: {
		subject: "✅ [RESOLVED] {{.alertName}}",
		body:    "Cảnh báo {{.alertName}} cho service {{.service}} đã được giải quyết lúc {{.resolvedAt}}.",
	},
	domain.EventIncidentCreated: {
		subject: "🚨 [{{.severity}}] Sự cố: {{.title}}",
		body:    "Sự cố mới: {{.title}}\nMức độ: {{.severity}}\nTrạng thái: {{.status}}\nMô tả: {{.description}}",
	},
	domain.EventIncidentResolved: {
		subject: "✅ Sự cố đã đóng: {{.title}}",
		body:    "Sự cố {{.title}} đã được giải quyết. MTTR: {{.mttr}}.",
	},
	domain.EventIncidentEscalated: {
		subject: "📟 Escalation [{{.target}}] sự cố: {{.title}}",
		body:    "Sự cố {{.title}} (service {{.service}}, {{.severity}}) chưa được ack.\nEscalate tới {{.target}}: {{.recipient}} (bậc {{.level}}).",
	},
	domain.EventSLOBudgetWarning: {
		subject: "⚠️ Error budget thấp: SLO {{.sloName}}",
		body:    "SLO {{.sloName}} (service {{.service}}) còn {{.budgetRemaining}} error budget.\nMục tiêu: {{.target}}. Hãy xem xét trước khi cạn ngân sách.",
	},
}

// renderer biên dịch sẵn template (compile-once) và render subject/body.
type renderer struct {
	compiled map[string]struct{ subject, body *template.Template }
}

// newRenderer biên dịch toàn bộ template tĩnh. Panic-free: template tĩnh hợp lệ
// đã kiểm ở test; lỗi parse trả về để constructor service fail-fast.
func newRenderer() (*renderer, error) {
	compiled := make(map[string]struct{ subject, body *template.Template }, len(_templates))
	for evt, t := range _templates {
		st, err := template.New(evt + ".subject").Option("missingkey=zero").Parse(t.subject)
		if err != nil {
			return nil, fmt.Errorf("parse subject %s: %w", evt, err)
		}
		bt, err := template.New(evt + ".body").Option("missingkey=zero").Parse(t.body)
		if err != nil {
			return nil, fmt.Errorf("parse body %s: %w", evt, err)
		}
		compiled[evt] = struct{ subject, body *template.Template }{st, bt}
	}
	return &renderer{compiled: compiled}, nil
}

// render trả subject + body cho eventType. Event type không có template → fallback
// generic (liệt kê data) để không bao giờ nuốt thông báo.
func (r *renderer) render(eventType string, data map[string]string) (subject, body string) {
	t, ok := r.compiled[eventType]
	if !ok {
		return "Thông báo: " + eventType, fallbackBody(eventType, data)
	}
	var sb, bb strings.Builder
	if err := t.subject.Execute(&sb, data); err != nil {
		sb.Reset()
		sb.WriteString("Thông báo: " + eventType)
	}
	if err := t.body.Execute(&bb, data); err != nil {
		bb.Reset()
		bb.WriteString(fallbackBody(eventType, data))
	}
	return sb.String(), bb.String()
}

// fallbackBody liệt kê data theo key đã sort (ổn định, dễ debug).
func fallbackBody(eventType string, data map[string]string) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("Sự kiện " + eventType + ":")
	for _, k := range keys {
		b.WriteString("\n- " + k + ": " + data[k])
	}
	return b.String()
}
