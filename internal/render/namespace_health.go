package render

import (
	"fmt"
	"io"

	"github.com/lucasepe/kctx/internal/engine"
)

func HumanNamespaceHealth(w io.Writer, health *engine.NamespaceHealthResponse) error {
	if _, err := fmt.Fprintf(w, "Namespace %s\n", health.Namespace); err != nil {
		return err
	}

	writeSection(w, "Summary")
	fmt.Fprintf(w, "  Pods:       %d total, %d ready, %d not ready\n", health.Summary.PodsTotal, health.Summary.PodsReady, health.Summary.PodsNotReady)
	fmt.Fprintf(w, "  Workloads:  %d total, %d healthy, %d unhealthy\n", health.Summary.WorkloadsTotal, health.Summary.WorkloadsHealthy, health.Summary.WorkloadsUnhealthy)
	fmt.Fprintf(w, "  Services:   %d total, %d without ready endpoints\n", health.Summary.ServicesTotal, health.Summary.ServicesWithoutEndpoints)
	fmt.Fprintf(w, "  PVCs:       %d total, %d pending\n", health.Summary.PVCsTotal, health.Summary.PVCsPending)
	fmt.Fprintf(w, "  Events:     %d recent warnings\n", health.Summary.WarningEvents)

	if len(health.Signals) > 0 {
		writeSection(w, "Top Signals")
		for _, signal := range health.Signals {
			fmt.Fprintf(w, "  %s %s: %s\n", signal.Severity, signal.Reason, signal.Message)
		}
	}

	if len(health.Workloads) > 0 {
		writeSection(w, "Unhealthy Workloads")
		for _, workload := range health.Workloads {
			if workload.Healthy {
				continue
			}
			fmt.Fprintf(w, "  %s %s/%s %s\n", workload.Kind, workload.Namespace, workload.Name, workload.Message)
		}
	}

	if len(health.Pods) > 0 {
		writeSection(w, "Not Ready Pods")
		for _, pod := range health.Pods {
			if pod.Ready {
				continue
			}
			fmt.Fprintf(w, "  Pod %s/%s Ready=%t Restarts=%d Reason=%s\n", pod.Namespace, pod.Name, pod.Ready, pod.Restarts, pod.Reason)
		}
	}

	if len(health.Services) > 0 {
		writeSection(w, "Services")
		for _, service := range health.Services {
			fmt.Fprintf(w, "  %s ReadyEndpoints=%d\n", service.Name, service.ReadyEndpoints)
		}
	}

	if len(health.PVCs) > 0 {
		writeSection(w, "PVCs")
		for _, pvc := range health.PVCs {
			fmt.Fprintf(w, "  PVC %s/%s Phase=%s\n", pvc.Namespace, pvc.Name, pvc.Phase)
		}
	}

	if len(health.Events) > 0 {
		writeSection(w, "Recent Warning Events")
		for _, event := range health.Events {
			fmt.Fprintf(w, "  %s %s %s/%s: %s\n", event.Type, event.Reason, event.Kind, event.Name, event.Message)
		}
	}

	return nil
}
