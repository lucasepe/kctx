package certmanager

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lucasepe/kctx/internal/graph"
	"github.com/lucasepe/kctx/internal/model"
	"github.com/lucasepe/kctx/internal/redaction"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	certificateAPIVersion = "cert-manager.io/v1"
	certificateKind       = "Certificate"
	expiringSoonWindow    = 30 * 24 * time.Hour
)

// CertificateAdapter interprets cert-manager Certificate resources without
// importing cert-manager Go packages.
type CertificateAdapter struct{}

func (CertificateAdapter) Supports(obj *unstructured.Unstructured) bool {
	return obj.GetAPIVersion() == certificateAPIVersion && obj.GetKind() == certificateKind
}

func (CertificateAdapter) Entities(ctx context.Context, obj *unstructured.Unstructured) ([]model.Entity, error) {
	_ = ctx
	entities := []model.Entity{certificateEntity(obj)}
	if secret := certificateSecret(obj); secret != "" {
		entities = append(entities, model.Entity{ID: graph.NodeID("Secret", obj.GetNamespace(), secret), APIVersion: "v1", Kind: "Secret", Namespace: obj.GetNamespace(), Name: secret})
	}
	if issuer := certificateIssuer(obj); issuer.Name != "" {
		entities = append(entities, issuer.Entity(obj.GetNamespace()))
	}
	return entities, nil
}

func (CertificateAdapter) Relations(ctx context.Context, obj *unstructured.Unstructured) ([]model.Relation, error) {
	_ = ctx
	cert := certificateEntity(obj)
	var relations []model.Relation
	if secret := certificateSecret(obj); secret != "" {
		relations = append(relations, model.Relation{Type: "stores_certificate_in", Source: cert, Target: model.Entity{ID: graph.NodeID("Secret", obj.GetNamespace(), secret), APIVersion: "v1", Kind: "Secret", Namespace: obj.GetNamespace(), Name: secret}, Reason: "spec.secretName"})
	}
	if issuer := certificateIssuer(obj); issuer.Name != "" {
		relations = append(relations, model.Relation{Type: "issued_by", Source: cert, Target: issuer.Entity(obj.GetNamespace()), Reason: "spec.issuerRef"})
	}
	return relations, nil
}

func (CertificateAdapter) Signals(ctx context.Context, obj *unstructured.Unstructured) ([]model.Signal, error) {
	_ = ctx
	var signals []model.Signal
	ready := certificateCondition(obj, "Ready")
	if ready.Type != "" && ready.Status != "True" {
		message := ready.Message
		if message == "" {
			message = fmt.Sprintf("cert-manager reports Certificate Ready as %s", ready.Status)
		}
		signals = append(signals, model.Signal{Severity: "error", Reason: "CertificateReady" + ready.Status, Message: redaction.Text(message), Source: "status.conditions.Ready"})
	}

	issuing := certificateCondition(obj, "Issuing")
	if issuing.Type != "" && issuing.Status == "True" {
		message := issuing.Message
		if message == "" {
			message = "cert-manager is issuing the Certificate"
		}
		signals = append(signals, model.Signal{Severity: "warning", Reason: "CertificateIssuing", Message: redaction.Text(message), Source: "status.conditions.Issuing"})
	}

	if notAfter, ok := certificateTime(obj, "status", "notAfter"); ok {
		until := time.Until(notAfter)
		switch {
		case until < 0:
			signals = append(signals, model.Signal{Severity: "error", Reason: "CertificateExpired", Message: fmt.Sprintf("Certificate %s/%s expired at %s", obj.GetNamespace(), obj.GetName(), notAfter.Format(time.RFC3339)), Source: "status.notAfter"})
		case until <= expiringSoonWindow:
			signals = append(signals, model.Signal{Severity: "warning", Reason: "CertificateExpiringSoon", Message: fmt.Sprintf("Certificate %s/%s expires at %s", obj.GetNamespace(), obj.GetName(), notAfter.Format(time.RFC3339)), Source: "status.notAfter"})
		}
	}
	return signals, nil
}

func (CertificateAdapter) Status(ctx context.Context, obj *unstructured.Unstructured) (map[string]string, error) {
	_ = ctx
	status := map[string]string{}
	if secret := certificateSecret(obj); secret != "" {
		status["secretName"] = secret
	}
	if issuer := certificateIssuer(obj); issuer.Name != "" {
		status["issuerRef"] = issuer.ID(obj.GetNamespace())
	}
	if ready := certificateCondition(obj, "Ready"); ready.Type != "" {
		status["ready"] = ready.Status
		if ready.Reason != "" {
			status["readyReason"] = ready.Reason
		}
		if ready.Message != "" {
			status["readyMessage"] = ready.Message
		}
	}
	addNestedString(status, "notAfter", obj, "status", "notAfter")
	addNestedString(status, "renewalTime", obj, "status", "renewalTime")
	return redaction.StringMap(status), nil
}

func (CertificateAdapter) Nodes(ctx context.Context, obj *unstructured.Unstructured) ([]graph.Node, error) {
	entities, err := CertificateAdapter{}.Entities(ctx, obj)
	if err != nil {
		return nil, err
	}
	nodes := make([]graph.Node, 0, len(entities))
	for _, entity := range entities {
		nodes = append(nodes, graph.Node{ID: entity.ID, Kind: entity.Kind, Namespace: entity.Namespace, Name: entity.Name, Labels: entity.Labels, Status: entity.Status})
	}
	return nodes, nil
}

func (CertificateAdapter) Edges(ctx context.Context, obj *unstructured.Unstructured) ([]graph.Edge, error) {
	relations, err := CertificateAdapter{}.Relations(ctx, obj)
	if err != nil {
		return nil, err
	}
	edges := make([]graph.Edge, 0, len(relations))
	for _, relation := range relations {
		edges = append(edges, graph.Edge{Type: relation.Type, Source: relation.Source.ID, Target: relation.Target.ID, Reason: relation.Reason})
	}
	return edges, nil
}

type issuerRef struct {
	Group string
	Kind  string
	Name  string
}

func (i issuerRef) Entity(namespace string) model.Entity {
	ns := namespace
	if i.Kind == "ClusterIssuer" {
		ns = ""
	}
	return model.Entity{ID: i.ID(namespace), APIVersion: i.APIVersion(), Kind: i.Kind, Namespace: ns, Name: i.Name}
}

func (i issuerRef) ID(namespace string) string {
	ns := namespace
	if i.Kind == "ClusterIssuer" {
		ns = ""
	}
	return graph.NodeID(i.Kind, ns, i.Name)
}

func (i issuerRef) APIVersion() string {
	group := i.Group
	if group == "" {
		group = "cert-manager.io"
	}
	return group + "/v1"
}

func certificateEntity(obj *unstructured.Unstructured) model.Entity {
	ready := ""
	if condition := certificateCondition(obj, "Ready"); condition.Type != "" {
		ready = condition.Status
	}
	return model.Entity{ID: graph.NodeID(obj.GetKind(), obj.GetNamespace(), obj.GetName()), APIVersion: obj.GetAPIVersion(), Kind: obj.GetKind(), Namespace: obj.GetNamespace(), Name: obj.GetName(), UID: string(obj.GetUID()), Labels: redaction.StringMap(obj.GetLabels()), Status: ready}
}

func certificateSecret(obj *unstructured.Unstructured) string {
	value, _, _ := unstructured.NestedString(obj.Object, "spec", "secretName")
	return strings.TrimSpace(value)
}

func certificateIssuer(obj *unstructured.Unstructured) issuerRef {
	ref, ok, _ := unstructured.NestedMap(obj.Object, "spec", "issuerRef")
	if !ok {
		return issuerRef{}
	}
	kind := stringValue(ref["kind"])
	if kind == "" {
		kind = "Issuer"
	}
	return issuerRef{Group: stringValue(ref["group"]), Kind: kind, Name: stringValue(ref["name"])}
}

func certificateCondition(obj *unstructured.Unstructured, conditionType string) condition {
	for _, condition := range conditions(obj) {
		if condition.Type == conditionType {
			return condition
		}
	}
	return condition{}
}

func conditions(obj *unstructured.Unstructured) []condition {
	items, ok, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !ok {
		return nil
	}
	out := make([]condition, 0, len(items))
	for _, item := range items {
		cond, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, condition{Type: stringValue(cond["type"]), Status: stringValue(cond["status"]), Reason: stringValue(cond["reason"]), Message: stringValue(cond["message"])})
	}
	return out
}

type condition struct {
	Type    string
	Status  string
	Reason  string
	Message string
}

func certificateTime(obj *unstructured.Unstructured, fields ...string) (time.Time, bool) {
	value, ok, _ := unstructured.NestedString(obj.Object, fields...)
	if !ok || value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func addNestedString(out map[string]string, key string, obj *unstructured.Unstructured, fields ...string) {
	value, ok, _ := unstructured.NestedString(obj.Object, fields...)
	if ok && value != "" {
		out[key] = value
	}
}

func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}
