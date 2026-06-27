package sender

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
	"github.com/namdam97/logmon/backend/internal/notification/ports"
)

// _defaultSMTPPort là cổng submission STARTTLS mặc định.
const _defaultSMTPPort = "587"

// smtpSendFunc cho phép inject để test (mặc định net/smtp.SendMail).
type smtpSendFunc func(addr string, a smtp.Auth, from string, to []string, msg []byte) error

// EmailSender gửi qua SMTP (config: smtp_host, from, to; optional smtp_port,
// username, password). Auth PLAIN khi có username (TLS do server/submission lo).
type EmailSender struct {
	send smtpSendFunc
}

var _ ports.Sender = (*EmailSender)(nil)

// NewEmailSender tạo sender email dùng net/smtp.SendMail.
func NewEmailSender() *EmailSender { return &EmailSender{send: smtp.SendMail} }

// Send dựng message RFC822 và gửi tới địa chỉ "to".
func (s *EmailSender) Send(_ context.Context, msg domain.Message) error {
	host := msg.Config["smtp_host"]
	from := msg.Config["from"]
	to := msg.Config["to"]
	if host == "" || from == "" || to == "" {
		return fmt.Errorf("email: missing smtp_host/from/to")
	}
	port := msg.Config["smtp_port"]
	if port == "" {
		port = _defaultSMTPPort
	}
	var auth smtp.Auth
	if user := msg.Config["username"]; user != "" {
		auth = smtp.PlainAuth("", user, msg.Config["password"], host)
	}
	addr := host + ":" + port
	body := buildEmail(from, to, msg.Subject, msg.Body)
	if err := s.send(addr, auth, from, []string{to}, body); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}

// buildEmail dựng message RFC822 tối thiểu (header + body). Pure → test được.
func buildEmail(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
	return []byte(b.String())
}
