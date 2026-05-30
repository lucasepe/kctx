package engine

import (
	"context"

	"github.com/lucasepe/kctx/internal/model"
)

// NamespaceHealthRequest identifies the namespace to summarize.
type NamespaceHealthRequest struct {
	Namespace string
}

// NamespaceHealthResponse is a factual health snapshot for one namespace.
type NamespaceHealthResponse struct {
	Namespace string                 `json:"namespace"`
	Summary   model.HealthSummary    `json:"summary"`
	Workloads []model.WorkloadHealth `json:"workloads"`
	Pods      []model.PodHealth      `json:"pods"`
	Services  []model.ServiceHealth  `json:"services"`
	PVCs      []model.PVCHealth      `json:"pvcs"`
	Events    []model.EventSummary   `json:"events"`
	Signals   []model.Signal         `json:"signals"`
}

// NamespaceHealth summarizes namespace-level health from supported workload,
// service, storage, Pod, and Event resources.
func (e *TypedEngine) NamespaceHealth(ctx context.Context, req NamespaceHealthRequest) (*NamespaceHealthResponse, error) {
	if req.Namespace == "" {
		req.Namespace = "default"
	}

	pods, err := e.kube.ListPods(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	deployments, err := e.kube.ListDeployments(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	replicaSets, err := e.kube.ListReplicaSets(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	statefulSets, err := e.kube.ListStatefulSets(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	daemonSets, err := e.kube.ListDaemonSets(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	jobs, err := e.kube.ListJobs(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	cronJobs, err := e.kube.ListCronJobs(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	services, err := e.kube.ListServices(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	slices, err := e.kube.ListEndpointSlices(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	pvcs, err := e.kube.ListPersistentVolumeClaims(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}
	events, err := e.kube.ListEvents(ctx, req.Namespace)
	if err != nil {
		return nil, err
	}

	resp := &NamespaceHealthResponse{Namespace: req.Namespace}
	resp.Pods = namespacePodHealth(pods)
	resp.Workloads = namespaceWorkloadHealth(deployments, replicaSets, statefulSets, daemonSets, jobs, cronJobs)
	resp.Services = namespaceServiceHealth(services, slices)
	resp.PVCs = namespacePVCHealth(pvcs)
	resp.Events, resp.Summary.WarningEvents = namespaceWarningEvents(events, 20)
	resp.Signals = namespaceSignals(req.Namespace, resp.Pods, resp.Workloads, resp.Services, resp.PVCs, resp.Events, len(pods), len(resp.Workloads), len(services), len(pvcs))
	resp.Summary = namespaceSummary(resp)
	sortNamespaceHealth(resp)
	return resp, nil
}
