// Package promrule build + server-side-apply PrometheusRule CR
// (monitoring.coreos.com/v1) cho Prometheus Operator (Phase III C3, ADR-024).
//
// Shared kernel: chỉ nhận DTO Group/Rule thuần (KHÔNG import BC) → alerting và
// slo BC tự convert domain của mình sang Group rồi gọi Applier. BuildObject là
// pure (test không cần cluster); Apply là I/O mỏng qua dynamic client.
package promrule

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GVR của PrometheusRule (CRD do prometheus-operator cài).
var gvr = schema.GroupVersionResource{
	Group:    "monitoring.coreos.com",
	Version:  "v1",
	Resource: "prometheusrules",
}

// Rule là một dòng rule (recording HOẶC alerting). Field rỗng được bỏ khi build
// để spec khớp schema PrometheusRule (record xOR alert).
type Rule struct {
	Record      string
	Alert       string
	Expr        string
	For         string
	Labels      map[string]string
	Annotations map[string]string
}

// Group là một rule group (tên + danh sách rule).
type Group struct {
	Name  string
	Rules []Rule
}

// BuildObject tạo unstructured PrometheusRule. Pure — không chạm cluster.
func BuildObject(name, namespace string, labels map[string]string, groups []Group) *unstructured.Unstructured {
	outGroups := make([]any, 0, len(groups))
	for _, g := range groups {
		rules := make([]any, 0, len(g.Rules))
		for _, r := range g.Rules {
			rules = append(rules, ruleMap(r))
		}
		outGroups = append(outGroups, map[string]any{
			"name":  g.Name,
			"rules": rules,
		})
	}

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "monitoring.coreos.com/v1",
		"kind":       "PrometheusRule",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{"groups": outGroups},
	}}
	if len(labels) > 0 {
		obj.SetLabels(labels)
	}
	return obj
}

func ruleMap(r Rule) map[string]any {
	m := map[string]any{"expr": r.Expr}
	if r.Record != "" {
		m["record"] = r.Record
	}
	if r.Alert != "" {
		m["alert"] = r.Alert
	}
	if r.For != "" {
		m["for"] = r.For
	}
	if len(r.Labels) > 0 {
		m["labels"] = toAnyMap(r.Labels)
	}
	if len(r.Annotations) > 0 {
		m["annotations"] = toAnyMap(r.Annotations)
	}
	return m
}

func toAnyMap(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Applier server-side-apply PrometheusRule qua dynamic client (idempotent,
// declarative — apply ghi đè trọn vẹn group do LogMon quản lý).
type Applier struct {
	client       dynamic.Interface
	fieldManager string
}

// NewApplier dùng cho test (inject fake dynamic client).
func NewApplier(client dynamic.Interface) *Applier {
	return &Applier{client: client, fieldManager: "logmon-rule-syncer"}
}

// NewInClusterApplier tạo Applier từ in-cluster config (ServiceAccount của pod).
func NewInClusterApplier() (*Applier, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}
	return NewApplier(client), nil
}

// Apply server-side-apply obj vào namespace của nó. Force:true để giành field
// ownership (LogMon là chủ duy nhất của group này).
func (a *Applier) Apply(ctx context.Context, obj *unstructured.Unstructured) error {
	_, err := a.client.Resource(gvr).Namespace(obj.GetNamespace()).Apply(
		ctx, obj.GetName(), obj,
		metav1.ApplyOptions{FieldManager: a.fieldManager, Force: true},
	)
	if err != nil {
		return fmt.Errorf("apply prometheusrule %s: %w", obj.GetName(), err)
	}
	return nil
}
