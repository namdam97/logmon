// Package promk8s implement ports.RuleSyncer bằng cách apply PrometheusRule CR
// (thay vì ghi rule file + reload như promfile). Dùng khi chạy trên Kubernetes:
// Prometheus Operator watch PrometheusRule → nạp vào Prometheus, không cần
// /-/reload hay shared volume (Phase III C3, ADR-024).
package promk8s

import (
	"context"
	"fmt"

	"github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/namdam97/logmon/backend/internal/alerting/domain"
	"github.com/namdam97/logmon/backend/internal/alerting/ports"
	"github.com/namdam97/logmon/backend/internal/shared/promrule"
)

// _groupName trùng với promfile để rule group ổn định khi đổi mode file↔k8s.
const _groupName = "logmon-generated"

// applier là I/O server-side-apply (promrule.Applier hoặc fake trong test).
type applier interface {
	Apply(ctx context.Context, obj *unstructured.Unstructured) error
}

// Syncer apply alert rule trong DB thành một PrometheusRule CR.
type Syncer struct {
	reader    ports.RuleReader
	status    ports.RuleSyncStatusWriter
	clock     ports.Clock
	applier   applier
	name      string
	namespace string
}

var _ ports.RuleSyncer = (*Syncer)(nil)

// NewSyncer tạo Syncer. name/namespace là metadata của PrometheusRule sinh ra.
func NewSyncer(reader ports.RuleReader, status ports.RuleSyncStatusWriter, clock ports.Clock, applier applier, name, namespace string) *Syncer {
	return &Syncer{reader: reader, status: status, clock: clock, applier: applier, name: name, namespace: namespace}
}

// Sync apply rule hiện tại → PrometheusRule, rồi ghi sync_status (đóng vòng như
// promfile). Lỗi bất kỳ → MarkSyncError + trả lỗi.
func (s *Syncer) Sync(ctx context.Context) error {
	if err := s.apply(ctx); err != nil {
		_ = s.status.MarkSyncError(ctx, err.Error(), s.clock.Now())
		return err
	}
	if err := s.status.MarkSynced(ctx, s.clock.Now()); err != nil {
		return fmt.Errorf("persist sync status: %w", err)
	}
	return nil
}

func (s *Syncer) apply(ctx context.Context) error {
	rules, err := s.reader.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("list rules: %w", err)
	}
	obj := promrule.BuildObject(s.name, s.namespace,
		map[string]string{"app.kubernetes.io/part-of": "logmon", "app.kubernetes.io/managed-by": "logmon-alerting"},
		toGroups(rules))
	return s.applier.Apply(ctx, obj)
}

// toGroups convert domain.AlertRule (enabled) thành một group promrule. Pure —
// gắn severity + service vào labels giống promfile.render.
func toGroups(rules []domain.AlertRule) []promrule.Group {
	group := promrule.Group{Name: _groupName}
	for _, r := range rules {
		if !r.IsEnabled() {
			continue
		}
		labels := r.Labels()
		labels["severity"] = r.Severity().String()
		labels["service"] = r.Service()
		group.Rules = append(group.Rules, promrule.Rule{
			Alert:       r.Name(),
			Expr:        r.Expression(),
			For:         model.Duration(r.ForDuration()).String(),
			Labels:      labels,
			Annotations: r.Annotations(),
		})
	}
	return []promrule.Group{group}
}
