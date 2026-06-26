package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/alerting/app/command"
	"github.com/namdam97/logmon/backend/internal/alerting/domain"
)

type fakeSilenceCreator struct {
	got command.CreateSilenceInput
	id  string
	err error
}

func (f *fakeSilenceCreator) Handle(_ context.Context, in command.CreateSilenceInput) (string, error) {
	f.got = in
	return f.id, f.err
}

type fakeSilenceDeleter struct {
	gotID string
	err   error
}

func (f *fakeSilenceDeleter) Handle(_ context.Context, id string) error {
	f.gotID = id
	return f.err
}

type fakeSilenceLister struct {
	views []domain.SilenceView
	err   error
}

func (f *fakeSilenceLister) List(context.Context) ([]domain.SilenceView, error) {
	return f.views, f.err
}

func newSilenceEngine(cr *fakeSilenceCreator, del *fakeSilenceDeleter, ls *fakeSilenceLister) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewSilenceHandler(cr, del, ls)
	h.Register(r.Group("/api/v1"), authStubMW)
	return r
}

func TestCreateSilence_Success(t *testing.T) {
	cr := &fakeSilenceCreator{id: "sil-123"}
	r := newSilenceEngine(cr, &fakeSilenceDeleter{}, &fakeSilenceLister{})

	body := `{"matchers":[{"name":"alertname","value":"HighErrorRate"}],
		"endsAt":"2026-01-01T02:00:00Z","comment":"điều tra"}`
	rec := doJSON(t, r, http.MethodPost, "/api/v1/alerts/silences", body)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, ackActor, cr.got.CreatedBy, "createdBy lấy từ JWT, không từ body")
	require.Len(t, cr.got.Matchers, 1)
	require.True(t, cr.got.Matchers[0].IsEqual, "isEqual mặc định true khi client bỏ trống")

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			SilenceID string `json:"silenceId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "sil-123", resp.Data.SilenceID)
}

func TestCreateSilence_NegativeMatcherRespected(t *testing.T) {
	cr := &fakeSilenceCreator{id: "sil-1"}
	r := newSilenceEngine(cr, &fakeSilenceDeleter{}, &fakeSilenceLister{})

	body := `{"matchers":[{"name":"env","value":"prod","isEqual":false}],
		"endsAt":"2026-01-01T02:00:00Z","comment":"c"}`
	rec := doJSON(t, r, http.MethodPost, "/api/v1/alerts/silences", body)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.False(t, cr.got.Matchers[0].IsEqual, "isEqual=false (phủ định) được giữ nguyên")
}

func TestCreateSilence_MissingFieldsReturns400(t *testing.T) {
	r := newSilenceEngine(&fakeSilenceCreator{}, &fakeSilenceDeleter{}, &fakeSilenceLister{})

	rec := doJSON(t, r, http.MethodPost, "/api/v1/alerts/silences",
		`{"matchers":[],"comment":""}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateSilence_ValidationErrorFromDomainReturns400(t *testing.T) {
	cr := &fakeSilenceCreator{err: &domain.ValidationError{Field: "endsAt", Message: "must be after startsAt"}}
	r := newSilenceEngine(cr, &fakeSilenceDeleter{}, &fakeSilenceLister{})

	body := `{"matchers":[{"name":"alertname","value":"x"}],"endsAt":"2026-01-01T02:00:00Z","comment":"c"}`
	rec := doJSON(t, r, http.MethodPost, "/api/v1/alerts/silences", body)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateSilence_GatewayErrorReturns502(t *testing.T) {
	cr := &fakeSilenceCreator{err: errors.New("dial tcp: connection refused")}
	r := newSilenceEngine(cr, &fakeSilenceDeleter{}, &fakeSilenceLister{})

	body := `{"matchers":[{"name":"alertname","value":"x"}],"endsAt":"2026-01-01T02:00:00Z","comment":"c"}`
	rec := doJSON(t, r, http.MethodPost, "/api/v1/alerts/silences", body)

	require.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestListSilences_ReturnsViews(t *testing.T) {
	view := domain.SilenceView{
		ID:        "sil-1",
		Status:    "active",
		Matchers:  []domain.SilenceMatcher{domain.NewSilenceMatcher("alertname", "HighErrorRate", false, true)},
		StartsAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndsAt:    time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC),
		CreatedBy: "user-1",
		Comment:   "c",
	}
	r := newSilenceEngine(&fakeSilenceCreator{}, &fakeSilenceDeleter{}, &fakeSilenceLister{views: []domain.SilenceView{view}})

	rec := doJSON(t, r, http.MethodGet, "/api/v1/alerts/silences", "")

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Success bool              `json:"success"`
		Data    []silenceResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	require.Equal(t, "sil-1", resp.Data[0].ID)
	require.Equal(t, "active", resp.Data[0].Status)
	require.Len(t, resp.Data[0].Matchers, 1)
	require.True(t, resp.Data[0].Matchers[0].IsEqual)
}

func TestDeleteSilence_Success(t *testing.T) {
	del := &fakeSilenceDeleter{}
	r := newSilenceEngine(&fakeSilenceCreator{}, del, &fakeSilenceLister{})

	rec := doJSON(t, r, http.MethodDelete, "/api/v1/alerts/silences/sil-123", "")

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, "sil-123", del.gotID)
}

func TestDeleteSilence_NotFoundReturns404(t *testing.T) {
	del := &fakeSilenceDeleter{err: domain.ErrSilenceNotFound}
	r := newSilenceEngine(&fakeSilenceCreator{}, del, &fakeSilenceLister{})

	rec := doJSON(t, r, http.MethodDelete, "/api/v1/alerts/silences/nope", "")

	require.Equal(t, http.StatusNotFound, rec.Code)
}
