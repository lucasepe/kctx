package engine

import (
	"context"
	"fmt"
	"sort"

	"github.com/lucasepe/kctx/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const serviceNameLabel = "kubernetes.io/service-name"

// TraceServiceRequest identifies the Service whose backend wiring should be
// traced.
type TraceServiceRequest struct {
	Namespace string
	Name      string
}

// TraceServiceResponse summarizes a Service, its endpoints, selected Pods,
// backend ownership, and factual usability signals.
type TraceServiceResponse struct {
	SchemaVersion string                  `json:"schemaVersion"`
	Kind          string                  `json:"kind"`
	Service       model.ServiceSummary    `json:"service"`
	Selector      map[string]string       `json:"selector,omitempty"`
	Ports         []model.ServicePort     `json:"ports"`
	Endpoints     []model.EndpointSummary `json:"endpoints"`
	Pods          []model.PodBackend      `json:"pods"`
	Owners        []model.Entity          `json:"owners"`
	Relations     []model.Relation        `json:"relations"`
	Signals       []model.Signal          `json:"signals"`
}

// TraceService correlates a Service to selected Pods, EndpointSlices or legacy
// Endpoints, backend owners, and Nodes.
func (e *TypedEngine) TraceService(ctx context.Context, req TraceServiceRequest) (*TraceServiceResponse, error) {
	if req.Namespace == "" {
		req.Namespace = "default"
	}

	service, err := e.kube.GetService(ctx, req.Namespace, req.Name)
	if err != nil {
		return nil, err
	}

	resp := &TraceServiceResponse{
		SchemaVersion: model.SchemaVersion,
		Kind:          "ServiceTrace",
		Service:       serviceSummary(service),
		Selector:      copyMap(service.Spec.Selector),
		Ports:         servicePorts(service),
	}
	serviceEntity := model.Entity{APIVersion: "v1", Kind: "Service", Namespace: service.Namespace, Name: service.Name, UID: string(service.UID), Labels: copyMap(service.Labels)}

	if service.Spec.Type == corev1.ServiceTypeExternalName {
		resp.Signals = append(resp.Signals, model.Signal{
			Severity: "info",
			Reason:   "external_name_service",
			Message:  fmt.Sprintf("service %s/%s points to external name %s", service.Namespace, service.Name, service.Spec.ExternalName),
			Source:   "service",
		})
		normalizeTraceServiceSlices(resp)
		return resp, nil
	}

	selectedPods, err := e.selectedPods(ctx, service)
	if err != nil {
		return nil, err
	}
	resp.Pods = podBackends(selectedPods)
	podByName, podByIP := podIndexes(selectedPods)

	if len(service.Spec.Selector) == 0 {
		resp.Signals = append(resp.Signals, model.Signal{Severity: "warning", Reason: "service_has_no_selector", Message: fmt.Sprintf("service %s/%s has no selector", service.Namespace, service.Name), Source: "service"})
	} else if len(selectedPods) == 0 {
		resp.Signals = append(resp.Signals, model.Signal{Severity: "warning", Reason: "selector_matches_no_pods", Message: fmt.Sprintf("service %s/%s selector matches no pods", service.Namespace, service.Name), Source: "selector"})
	}

	for _, pod := range selectedPods {
		podEntity := podEntity(&pod)
		resp.Relations = append(resp.Relations, model.Relation{Type: "selects", Source: serviceEntity, Target: podEntity})
		if pod.Spec.NodeName != "" {
			resp.Relations = append(resp.Relations, model.Relation{Type: "runs_on", Source: podEntity, Target: model.Entity{Kind: "Node", Name: pod.Spec.NodeName}})
		}
		owners, relations, err := e.traceOwnerChain(ctx, pod.Namespace, pod.OwnerReferences, podEntity, map[string]bool{})
		if err != nil {
			return nil, err
		}
		resp.Owners = append(resp.Owners, owners...)
		resp.Relations = append(resp.Relations, relations...)
	}

	endpoints, err := e.serviceEndpoints(ctx, service, podByIP)
	if err != nil {
		return nil, err
	}
	resp.Endpoints = endpoints
	resp.Relations = append(resp.Relations, endpointRelations(serviceEntity, endpoints, podByName)...)
	resp.Signals = append(resp.Signals, traceSignals(service, resp.Pods, endpoints)...)

	resp.Owners = dedupeEntities(resp.Owners)
	resp.Relations = dedupeRelations(resp.Relations)
	normalizeTraceServiceSlices(resp)
	sortTraceResponse(resp)
	return resp, nil
}

func normalizeTraceServiceSlices(resp *TraceServiceResponse) {
	if resp.Ports == nil {
		resp.Ports = []model.ServicePort{}
	}
	if resp.Endpoints == nil {
		resp.Endpoints = []model.EndpointSummary{}
	}
	if resp.Pods == nil {
		resp.Pods = []model.PodBackend{}
	}
	if resp.Owners == nil {
		resp.Owners = []model.Entity{}
	}
	if resp.Relations == nil {
		resp.Relations = []model.Relation{}
	}
	if resp.Signals == nil {
		resp.Signals = []model.Signal{}
	}
}

// selectedPods returns Pods in the Service namespace that match its selector.
func (e *TypedEngine) selectedPods(ctx context.Context, service *corev1.Service) ([]corev1.Pod, error) {
	if len(service.Spec.Selector) == 0 {
		return nil, nil
	}
	pods, err := e.kube.ListPods(ctx, service.Namespace)
	if err != nil {
		return nil, err
	}
	var selected []corev1.Pod
	for _, pod := range pods {
		if selectorMatches(service.Spec.Selector, pod.Labels) {
			selected = append(selected, pod)
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].Name < selected[j].Name
	})
	return selected, nil
}

// traceOwnerChain recursively follows owner references for backend Pods while
// avoiding cycles.
func (e *TypedEngine) traceOwnerChain(ctx context.Context, namespace string, refs []metav1.OwnerReference, child model.Entity, seen map[string]bool) ([]model.Entity, []model.Relation, error) {
	var owners []model.Entity
	var relations []model.Relation
	for _, ref := range refs {
		ownerNode, ownerRefs, err := e.ownerNodeAndRefs(ctx, namespace, ref)
		if err != nil {
			return nil, nil, err
		}
		owner := model.Entity{Kind: ownerNode.Kind, Namespace: ownerNode.Namespace, Name: ownerNode.Name, Labels: copyMap(ownerNode.Labels)}
		owners = append(owners, owner)
		relations = append(relations, model.Relation{Type: "owned_by", Source: child, Target: owner})
		key := owner.Kind + "/" + owner.Namespace + "/" + owner.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		nextOwners, nextRelations, err := e.traceOwnerChain(ctx, namespace, ownerRefs, owner, seen)
		if err != nil {
			return nil, nil, err
		}
		owners = append(owners, nextOwners...)
		relations = append(relations, nextRelations...)
	}
	return owners, relations, nil
}

// endpointRelations builds Service-to-endpoint and endpoint-to-Pod relations.
func endpointRelations(service model.Entity, endpoints []model.EndpointSummary, podByName map[string]corev1.Pod) []model.Relation {
	var relations []model.Relation
	for _, endpoint := range endpoints {
		endpointEntity := model.Entity{Kind: "Endpoint", Namespace: service.Namespace, Name: endpoint.ID}
		relations = append(relations, model.Relation{Type: "has_endpoint", Source: service, Target: endpointEntity})
		if endpoint.Pod != "" {
			if pod, ok := podByName[endpoint.Pod]; ok {
				relations = append(relations, model.Relation{Type: "endpoint_targets", Source: endpointEntity, Target: podEntity(&pod)})
			} else {
				relations = append(relations, model.Relation{Type: "endpoint_targets", Source: endpointEntity, Target: model.Entity{Kind: "Pod", Namespace: service.Namespace, Name: endpoint.Pod}})
			}
		}
	}
	return relations
}

// serviceSummary normalizes the Service fields needed by trace output.
func serviceSummary(service *corev1.Service) model.ServiceSummary {
	return model.ServiceSummary{
		Kind:         "Service",
		Namespace:    service.Namespace,
		Name:         service.Name,
		Type:         string(service.Spec.Type),
		ClusterIP:    service.Spec.ClusterIP,
		ExternalName: service.Spec.ExternalName,
		Labels:       copyMap(service.Labels),
	}
}

// servicePorts normalizes Service ports and sorts them deterministically.
func servicePorts(service *corev1.Service) []model.ServicePort {
	ports := make([]model.ServicePort, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		ports = append(ports, model.ServicePort{Name: port.Name, Protocol: string(port.Protocol), Port: port.Port, TargetPort: targetPortString(port.TargetPort)})
	}
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Port != ports[j].Port {
			return ports[i].Port < ports[j].Port
		}
		return ports[i].Name < ports[j].Name
	})
	return ports
}

// podBackends converts selected Pods into compact backend summaries.
func podBackends(pods []corev1.Pod) []model.PodBackend {
	out := make([]model.PodBackend, 0, len(pods))
	for _, pod := range pods {
		out = append(out, model.PodBackend{Namespace: pod.Namespace, Name: pod.Name, UID: string(pod.UID), IP: pod.Status.PodIP, Ready: isPodReady(&pod), Restarts: podRestarts(&pod), Node: pod.Spec.NodeName, Phase: string(pod.Status.Phase)})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// podIndexes builds lookup maps used to correlate endpoints back to Pods.
func podIndexes(pods []corev1.Pod) (map[string]corev1.Pod, map[string]corev1.Pod) {
	byName := map[string]corev1.Pod{}
	byIP := map[string]corev1.Pod{}
	for _, pod := range pods {
		byName[pod.Name] = pod
		if pod.Status.PodIP != "" {
			byIP[pod.Status.PodIP] = pod
		}
	}
	return byName, byIP
}

// podRestarts totals restart counts across all containers in a Pod.
func podRestarts(pod *corev1.Pod) int32 {
	var restarts int32
	for _, container := range pod.Status.ContainerStatuses {
		restarts += container.RestartCount
	}
	return restarts
}

// sortTraceResponse makes trace output stable across client-go list ordering.
func sortTraceResponse(resp *TraceServiceResponse) {
	sortEndpoints(resp.Endpoints)
	sort.Slice(resp.Pods, func(i, j int) bool { return resp.Pods[i].Name < resp.Pods[j].Name })
	sort.Slice(resp.Owners, func(i, j int) bool { return entityKey(resp.Owners[i]) < entityKey(resp.Owners[j]) })
	sort.Slice(resp.Relations, func(i, j int) bool { return relationKey(resp.Relations[i]) < relationKey(resp.Relations[j]) })
	sort.Slice(resp.Signals, func(i, j int) bool {
		if severityRank(resp.Signals[i].Severity) != severityRank(resp.Signals[j].Severity) {
			return severityRank(resp.Signals[i].Severity) > severityRank(resp.Signals[j].Severity)
		}
		if resp.Signals[i].Reason != resp.Signals[j].Reason {
			return resp.Signals[i].Reason < resp.Signals[j].Reason
		}
		return resp.Signals[i].Message < resp.Signals[j].Message
	})
}
