// Package promfile implement ports.SLOSyncer: render SLO trong DB thành recording
// + MWMB alerting rules (Prometheus rule file RIÊNG, tách khỏi alerting BC), validate
// in-process (rulefmt), ghi atomic rồi reload Prometheus (ADR-024 + hội đồng GĐ3:
// file/group riêng theo slo_id để không đè logmon-generated.yml của alerting).
package promfile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
	"gopkg.in/yaml.v3"

	"github.com/namdam97/logmon/backend/internal/slo/domain"
	"github.com/namdam97/logmon/backend/internal/slo/ports"
)

const (
	// _fileName tách biệt với logmon-generated.yml của alerting BC.
	_fileName    = "logmon-slo-generated.yml"
	_httpTimeout = 5 * time.Second
)

// Syncer render + ghi + reload SLO rule file, rồi ghi lại sync_status vào DB.
type Syncer struct {
	reader     ports.SLOReader
	status     ports.SLOSyncStatusWriter
	clock      ports.Clock
	rulesDir   string
	promURL    string
	httpClient *http.Client
}

var _ ports.SLOSyncer = (*Syncer)(nil)

// NewSyncer tạo Syncer. rulesDir là thư mục generated (Prometheus mount đọc);
// promURL là base URL Prometheus (cần --web.enable-lifecycle để /-/reload).
func NewSyncer(reader ports.SLOReader, status ports.SLOSyncStatusWriter, clock ports.Clock, rulesDir, promURL string) *Syncer {
	return &Syncer{
		reader:     reader,
		status:     status,
		clock:      clock,
		rulesDir:   rulesDir,
		promURL:    promURL,
		httpClient: &http.Client{Timeout: _httpTimeout},
	}
}

// Sync render mọi SLO → rules, validate, ghi atomic, reload, ghi sync_status.
func (s *Syncer) Sync(ctx context.Context) error {
	if err := s.render(ctx); err != nil {
		_ = s.status.MarkSyncError(ctx, err.Error(), s.clock.Now())
		return err
	}
	if err := s.status.MarkSynced(ctx, s.clock.Now()); err != nil {
		return fmt.Errorf("persist sync status: %w", err)
	}
	return nil
}

func (s *Syncer) render(ctx context.Context) error {
	slos, err := s.reader.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("list slos: %w", err)
	}
	content, err := renderSLOs(slos)
	if err != nil {
		return fmt.Errorf("render slo rules: %w", err)
	}
	if err := validate(content); err != nil {
		return err // validate TRƯỚC khi ghi — không phá file đang chạy
	}

	target := filepath.Join(s.rulesDir, _fileName)
	prev, hadPrev := readMaybe(target)
	if err := atomicWrite(s.rulesDir, _fileName, content); err != nil {
		return fmt.Errorf("write rule file: %w", err)
	}
	if err := s.reload(ctx); err != nil {
		s.rollback(target, prev, hadPrev)
		return fmt.Errorf("reload prometheus: %w", err)
	}
	return nil
}

// --- render ---

type ruleFile struct {
	Groups []ruleGroup `yaml:"groups"`
}

type ruleGroup struct {
	Name  string `yaml:"name"`
	Rules []rule `yaml:"rules"`
}

// rule là entry hợp nhất: recording (record) HOẶC alerting (alert).
type rule struct {
	Record      string            `yaml:"record,omitempty"`
	Alert       string            `yaml:"alert,omitempty"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

func renderSLOs(slos []domain.SLO) ([]byte, error) {
	groups := make([]ruleGroup, 0, len(slos))
	for _, s := range slos {
		g := s.GenerateRuleGroup()
		grp := ruleGroup{Name: g.Name}
		for _, rec := range g.Recording {
			grp.Rules = append(grp.Rules, rule{
				Record: rec.Record,
				Expr:   rec.Expr,
				Labels: rec.Labels,
			})
		}
		for _, al := range g.Alerting {
			grp.Rules = append(grp.Rules, rule{
				Alert:       al.Alert,
				Expr:        al.Expr,
				For:         al.For,
				Labels:      al.Labels,
				Annotations: al.Annotations,
			})
		}
		groups = append(groups, grp)
	}
	return yaml.Marshal(ruleFile{Groups: groups})
}

func validate(content []byte) error {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, errs := rulefmt.Parse(content, false, model.UTF8Validation, parser.NewParser(parser.Options{}), logger); len(errs) > 0 {
		return fmt.Errorf("invalid slo rule file: %w", errors.Join(errs...))
	}
	return nil
}

// --- file IO ---

func atomicWrite(dir, name string, content []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: shared config dir
		return err
	}
	tmp, err := os.CreateTemp(dir, name+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil { //nolint:gosec // G302: rule file đọc bởi Prometheus
		return err
	}
	return os.Rename(tmpName, filepath.Join(dir, name))
}

func readMaybe(path string) ([]byte, bool) {
	b, err := os.ReadFile(path) //nolint:gosec // G304: path nội bộ (rulesDir cấu hình)
	if err != nil {
		return nil, false
	}
	return b, true
}

func (s *Syncer) rollback(target string, prev []byte, hadPrev bool) {
	if hadPrev {
		_ = atomicWrite(s.rulesDir, _fileName, prev)
		return
	}
	_ = os.Remove(target)
}

// --- reload ---

func (s *Syncer) reload(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.promURL+"/-/reload", nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("prometheus reload status %d", resp.StatusCode)
	}
	return nil
}
