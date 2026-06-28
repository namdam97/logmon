// Package promk8s implement ports.SLOSyncer bằng cách apply PrometheusRule CR
// (thay promfile ghi file + reload). Dùng trên Kubernetes — Prometheus Operator
// nạp rule tự động (Phase III C3, ADR-024). Mỗi SLO → một rule group.
package promk8s

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/namdam97/logmon/backend/internal/shared/promrule"
	"github.com/namdam97/logmon/backend/internal/slo/domain"
	"github.com/namdam97/logmon/backend/internal/slo/ports"
)

type applier interface {
	Apply(ctx context.Context, obj *unstructured.Unstructured) error
}

// Syncer apply SLO trong DB thành một PrometheusRule CR (nhiều group).
type Syncer struct {
	reader    ports.SLOReader
	status    ports.SLOSyncStatusWriter
	clock     ports.Clock
	applier   applier
	name      string
	namespace string
}

var _ ports.SLOSyncer = (*Syncer)(nil)

// NewSyncer tạo Syncer. name/namespace là metadata PrometheusRule sinh ra.
func NewSyncer(reader ports.SLOReader, status ports.SLOSyncStatusWriter, clock ports.Clock, applier applier, name, namespace string) *Syncer {
	return &Syncer{reader: reader, status: status, clock: clock, applier: applier, name: name, namespace: namespace}
}

// Sync apply SLO → PrometheusRule, rồi ghi sync_status (đóng vòng như promfile).
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
	slos, err := s.reader.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("list slos: %w", err)
	}
	obj := promrule.BuildObject(s.name, s.namespace,
		map[string]string{"app.kubernetes.io/part-of": "logmon", "app.kubernetes.io/managed-by": "logmon-slo"},
		toGroups(slos))
	return s.applier.Apply(ctx, obj)
}

// toGroups convert mỗi SLO → một promrule.Group (recording + alerting). Pure.
func toGroups(slos []domain.SLO) []promrule.Group {
	groups := make([]promrule.Group, 0, len(slos))
	for _, s := range slos {
		g := s.GenerateRuleGroup()
		grp := promrule.Group{Name: g.Name}
		for _, rec := range g.Recording {
			grp.Rules = append(grp.Rules, promrule.Rule{
				Record: rec.Record,
				Expr:   rec.Expr,
				Labels: rec.Labels,
			})
		}
		for _, al := range g.Alerting {
			grp.Rules = append(grp.Rules, promrule.Rule{
				Alert:       al.Alert,
				Expr:        al.Expr,
				For:         al.For,
				Labels:      al.Labels,
				Annotations: al.Annotations,
			})
		}
		groups = append(groups, grp)
	}
	return groups
}
