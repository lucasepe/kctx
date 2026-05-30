package engine

import (
	"sort"

	"github.com/lucasepe/kctx/internal/model"
)

// namespaceSummary computes aggregate counts from the normalized health
// response.
func namespaceSummary(resp *NamespaceHealthResponse) model.HealthSummary {
	summary := resp.Summary
	summary.PodsTotal = len(resp.Pods)
	for _, pod := range resp.Pods {
		if pod.Ready {
			summary.PodsReady++
		} else {
			summary.PodsNotReady++
		}
		if pod.Restarts > 0 {
			summary.PodsRestarting++
		}
	}
	summary.WorkloadsTotal = len(resp.Workloads)
	for _, workload := range resp.Workloads {
		if workload.Healthy {
			summary.WorkloadsHealthy++
		} else {
			summary.WorkloadsUnhealthy++
		}
	}
	summary.ServicesTotal = len(resp.Services)
	for _, service := range resp.Services {
		if service.ReadyEndpoints == 0 {
			summary.ServicesWithoutEndpoints++
		}
	}
	summary.PVCsTotal = len(resp.PVCs)
	for _, pvc := range resp.PVCs {
		if pvc.Phase == "Pending" || pvc.Phase == "Lost" {
			summary.PVCsPending++
		}
	}
	for _, signal := range resp.Signals {
		if signal.Severity == "critical" {
			summary.CriticalSignals++
		}
	}
	return summary
}

// sortNamespaceHealth makes the health response deterministic for JSON output
// and tests.
func sortNamespaceHealth(resp *NamespaceHealthResponse) {
	sort.Slice(resp.Pods, func(i, j int) bool {
		if resp.Pods[i].Namespace != resp.Pods[j].Namespace {
			return resp.Pods[i].Namespace < resp.Pods[j].Namespace
		}
		return resp.Pods[i].Name < resp.Pods[j].Name
	})
	sort.Slice(resp.Workloads, func(i, j int) bool {
		return workloadHealthKey(resp.Workloads[i]) < workloadHealthKey(resp.Workloads[j])
	})
	sort.Slice(resp.Services, func(i, j int) bool {
		if resp.Services[i].Namespace != resp.Services[j].Namespace {
			return resp.Services[i].Namespace < resp.Services[j].Namespace
		}
		return resp.Services[i].Name < resp.Services[j].Name
	})
	sort.Slice(resp.PVCs, func(i, j int) bool {
		if resp.PVCs[i].Namespace != resp.PVCs[j].Namespace {
			return resp.PVCs[i].Namespace < resp.PVCs[j].Namespace
		}
		return resp.PVCs[i].Name < resp.PVCs[j].Name
	})
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

// workloadHealthKey returns the stable sort key for workload health summaries.
func workloadHealthKey(workload model.WorkloadHealth) string {
	return workload.Kind + "/" + workload.Namespace + "/" + workload.Name
}
