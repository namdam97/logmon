package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/namdam97/logmon/backend/internal/slo/app/command"
	"github.com/namdam97/logmon/backend/internal/slo/domain"
)

// ── Fakes ──────────────────────────────────────────────────────────────────

type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fakeRepo struct {
	saved    map[string]domain.SLO
	names    map[string]bool
	deleted  []string
	updated  map[string]domain.SLO
	existErr error
}

func newRepo() *fakeRepo {
	return &fakeRepo{saved: map[string]domain.SLO{}, names: map[string]bool{}, updated: map[string]domain.SLO{}}
}

func (r *fakeRepo) Save(_ context.Context, s domain.SLO) error {
	r.saved[s.ID().String()] = s
	r.names[s.WorkspaceID()+"/"+s.Name()] = true
	return nil
}
func (r *fakeRepo) Update(_ context.Context, s domain.SLO) error {
	r.updated[s.ID().String()] = s
	return nil
}
func (r *fakeRepo) Delete(_ context.Context, id domain.SLOID) error {
	r.deleted = append(r.deleted, id.String())
	return nil
}
func (r *fakeRepo) ExistsByName(_ context.Context, ws, name string) (bool, error) {
	if r.existErr != nil {
		return false, r.existErr
	}
	return r.names[ws+"/"+name], nil
}

type fakeReader struct{ byID map[string]domain.SLO }

func (r fakeReader) ByID(_ context.Context, id domain.SLOID) (domain.SLO, error) {
	s, ok := r.byID[id.String()]
	if !ok {
		return domain.SLO{}, domain.ErrSLONotFound
	}
	return s, nil
}
func (r fakeReader) List(context.Context, string) ([]domain.SLO, error) { return nil, nil }
func (r fakeReader) ListAll(context.Context) ([]domain.SLO, error)      { return nil, nil }

type fakePublisher struct{ events []string }

func (p *fakePublisher) Publish(_ context.Context, _, _, eventType string, _ any) error {
	p.events = append(p.events, eventType)
	return nil
}

type fakeIDs struct{ id string }

func (f fakeIDs) NewID() string { return f.id }

type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }

func validCreate() command.CreateSLOInput {
	return command.CreateSLOInput{
		WorkspaceID: "ws-1",
		Name:        "checkout availability",
		Service:     "checkout",
		SLIType:     "availability",
		Target:      0.999,
		WindowDays:  28,
	}
}

// ── Tests ──────────────────────────────────────────────────────────────────

func TestCreateSLOPersistsAndPublishes(t *testing.T) {
	repo := newRepo()
	pub := &fakePublisher{}
	h := command.NewCreateSLOHandler(fakeTx{}, repo, pub, fakeIDs{"slo-x"}, fakeClock{time.Now()})

	got, err := h.Handle(context.Background(), validCreate())

	require.NoError(t, err)
	require.Equal(t, "slo-x", got.ID().String())
	require.Len(t, repo.saved, 1)
	require.Equal(t, []string{domain.EventSLODefined}, pub.events)
}

func TestCreateSLONameTaken(t *testing.T) {
	repo := newRepo()
	repo.names["ws-1/checkout availability"] = true
	h := command.NewCreateSLOHandler(fakeTx{}, repo, &fakePublisher{}, fakeIDs{"slo-x"}, fakeClock{time.Now()})

	_, err := h.Handle(context.Background(), validCreate())

	require.ErrorIs(t, err, domain.ErrSLONameTaken)
}

func TestCreateSLOInvalidSLIType(t *testing.T) {
	h := command.NewCreateSLOHandler(fakeTx{}, newRepo(), &fakePublisher{}, fakeIDs{"slo-x"}, fakeClock{time.Now()})
	in := validCreate()
	in.SLIType = "throughput"

	_, err := h.Handle(context.Background(), in)

	var ve *domain.ValidationError
	require.ErrorAs(t, err, &ve)
}

func TestUpdateSLOWorkspaceIsolation(t *testing.T) {
	id, _ := domain.NewSLOID("slo-1")
	existing, _ := domain.NewSLO(domain.NewSLOInput{
		ID: id, WorkspaceID: "ws-OTHER", Name: "n", Service: "svc",
		SLIType: domain.SLIAvailability, Target: 0.99, WindowDays: 28, CreatedAt: time.Now(),
	})
	reader := fakeReader{byID: map[string]domain.SLO{"slo-1": existing}}
	h := command.NewUpdateSLOHandler(fakeTx{}, newRepo(), reader, &fakePublisher{}, fakeClock{time.Now()})

	_, err := h.Handle(context.Background(), command.UpdateSLOInput{
		WorkspaceID: "ws-1", ID: "slo-1", Name: "n2", Service: "svc",
		SLIType: "availability", Target: 0.99, WindowDays: 28,
	})

	require.ErrorIs(t, err, domain.ErrSLONotFound)
}

func TestDeleteSLOPublishes(t *testing.T) {
	id, _ := domain.NewSLOID("slo-1")
	existing, _ := domain.NewSLO(domain.NewSLOInput{
		ID: id, WorkspaceID: "ws-1", Name: "n", Service: "svc",
		SLIType: domain.SLIAvailability, Target: 0.99, WindowDays: 28, CreatedAt: time.Now(),
	})
	reader := fakeReader{byID: map[string]domain.SLO{"slo-1": existing}}
	repo := newRepo()
	pub := &fakePublisher{}
	h := command.NewDeleteSLOHandler(fakeTx{}, repo, reader, pub)

	err := h.Handle(context.Background(), "ws-1", "slo-1")

	require.NoError(t, err)
	require.Equal(t, []string{"slo-1"}, repo.deleted)
	require.Equal(t, []string{domain.EventSLODeleted}, pub.events)
}
