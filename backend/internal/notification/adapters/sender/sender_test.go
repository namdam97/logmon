package sender

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/notification/domain"
)

func TestSlackPayload(t *testing.T) {
	got := slackPayload(domain.Message{Subject: "S", Body: "B"})
	require.Equal(t, "*S*\nB", got["text"])
}

func TestWebhookPayload(t *testing.T) {
	msg := domain.Message{EventType: "alert_fired", EventRef: "a-1", Subject: "S", Body: "B"}
	got := webhookPayload(msg)
	require.Equal(t, "alert_fired", got["event_type"])
	require.Equal(t, "a-1", got["event_ref"])
}

func TestPagerDutyPayload(t *testing.T) {
	got := pagerDutyPayload(domain.Message{Subject: "boom", DedupKey: "k1"}, "rk-1")
	require.Equal(t, "rk-1", got["routing_key"])
	require.Equal(t, "trigger", got["event_action"])
	require.Equal(t, "k1", got["dedup_key"])
	payload := got["payload"].(map[string]string)
	require.Equal(t, "boom", payload["summary"])
}

func TestBuildEmailHeaders(t *testing.T) {
	got := string(buildEmail("a@x.com", "b@y.com", "Subj", "Body"))
	require.Contains(t, got, "From: a@x.com\r\n")
	require.Contains(t, got, "To: b@y.com\r\n")
	require.Contains(t, got, "Subject: Subj\r\n")
	require.Contains(t, got, "\r\n\r\nBody")
}

func TestSlackSenderPostsToWebhook(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewSlackSender()
	err := s.Send(context.Background(), domain.Message{
		Subject: "S", Body: "B", Config: map[string]string{"webhook_url": srv.URL},
	})

	require.NoError(t, err)
	require.Equal(t, "*S*\nB", gotBody["text"])
}

func TestSlackSenderErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := NewSlackSender().Send(context.Background(), domain.Message{
		Config: map[string]string{"webhook_url": srv.URL},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "500")
}

func TestSlackSenderMissingConfig(t *testing.T) {
	err := NewSlackSender().Send(context.Background(), domain.Message{Config: map[string]string{}})
	require.Error(t, err)
}

func TestEmailSenderBuildsAndSends(t *testing.T) {
	var gotAddr, gotFrom string
	var gotTo []string
	var gotMsg []byte
	s := &EmailSender{send: func(addr string, _ smtp.Auth, from string, to []string, msg []byte) error {
		gotAddr, gotFrom, gotTo, gotMsg = addr, from, to, msg
		return nil
	}}

	err := s.Send(context.Background(), domain.Message{
		Subject: "S", Body: "B",
		Config: map[string]string{"smtp_host": "mail.x.com", "from": "a@x.com", "to": "b@y.com"},
	})

	require.NoError(t, err)
	require.Equal(t, "mail.x.com:587", gotAddr)
	require.Equal(t, "a@x.com", gotFrom)
	require.Equal(t, []string{"b@y.com"}, gotTo)
	require.Contains(t, string(gotMsg), "Subject: S")
}

func TestRegistryHasAllTypes(t *testing.T) {
	reg := Registry()
	for _, ct := range []string{"slack", "teams", "webhook", "pagerduty", "email"} {
		require.Contains(t, reg, ct)
	}
}
