// Package promfile implement ports.RuleSyncer: render alert rule trong DB thành
// Prometheus rule file, validate (rulefmt, in-process — không cần promtool
// binary), ghi atomic vào thư mục generated rồi reload Prometheus (ADR-024).
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

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/alerting/ports"
)

const (
	_groupName   = "logmon-generated"
	_fileName    = "logmon-generated.yml"
	_httpTimeout = 5 * time.Second
)

// Syncer render + ghi + reload rule file.
type Syncer struct {
	reader     ports.RuleReader
	rulesDir   string
	promURL    string
	httpClient *http.Client
}

var _ ports.RuleSyncer = (*Syncer)(nil)

// NewSyncer tạo Syncer. rulesDir là thư mục generated (Prometheus mount đọc);
// promURL là base URL Prometheus (cần --web.enable-lifecycle để /-/reload).
func NewSyncer(reader ports.RuleReader, rulesDir, promURL string) *Syncer {
	return &Syncer{
		reader:     reader,
		rulesDir:   rulesDir,
		promURL:    promURL,
		httpClient: &http.Client{Timeout: _httpTimeout},
	}
}

// Sync render mọi rule enabled, validate, ghi atomic, rồi reload Prometheus.
// Reload thất bại → rollback file về nội dung trước đó.
func (s *Syncer) Sync(ctx context.Context) error {
	rules, err := s.reader.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("list rules: %w", err)
	}
	content, err := render(rules)
	if err != nil {
		return fmt.Errorf("render rules: %w", err)
	}
	if err := validate(content); err != nil {
		return err // validate TRƯỚC khi ghi (ADR-024) — không phá file đang chạy
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
	Name  string      `yaml:"name"`
	Rules []alertRule `yaml:"rules"`
}

type alertRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

func render(rules []domain.AlertRule) ([]byte, error) {
	group := ruleGroup{Name: _groupName}
	for _, r := range rules {
		if !r.IsEnabled() {
			continue
		}
		labels := r.Labels()
		labels["severity"] = r.Severity().String()
		labels["service"] = r.Service()
		group.Rules = append(group.Rules, alertRule{
			Alert:       r.Name(),
			Expr:        r.Expression(),
			For:         model.Duration(r.ForDuration()).String(),
			Labels:      labels,
			Annotations: r.Annotations(),
		})
	}
	return yaml.Marshal(ruleFile{Groups: []ruleGroup{group}})
}

func validate(content []byte) error {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, errs := rulefmt.Parse(content, false, model.UTF8Validation, parser.NewParser(parser.Options{}), logger); len(errs) > 0 {
		return fmt.Errorf("invalid rule file: %w", errors.Join(errs...))
	}
	return nil
}

// --- file IO ---

func atomicWrite(dir, name string, content []byte) error {
	// 0o755/0o644: thư mục + rule file phải đọc được bởi Prometheus container.
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: shared config dir
		return err
	}
	tmp, err := os.CreateTemp(dir, name+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op nếu rename thành công

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
	b, err := os.ReadFile(path) //nolint:gosec // G304: path nội bộ (rulesDir cấu hình), không phải user input
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
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
