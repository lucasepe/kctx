package render

import (
	"fmt"
	"io"
	"sort"

	"github.com/lucasepe/kctx/internal/engine"
)

func HumanServiceTrace(w io.Writer, trace *engine.TraceServiceResponse) error {
	if _, err := fmt.Fprintf(w, "Service %s/%s\n", trace.Service.Namespace, trace.Service.Name); err != nil {
		return err
	}

	writeSection(w, "Type")
	switch trace.Service.Type {
	case "ExternalName":
		fmt.Fprintf(w, "  ExternalName %s\n", trace.Service.ExternalName)
	default:
		fmt.Fprintf(w, "  %s %s\n", trace.Service.Type, trace.Service.ClusterIP)
	}

	if len(trace.Selector) > 0 {
		writeSection(w, "Selector")
		for _, key := range sortedKeys(trace.Selector) {
			fmt.Fprintf(w, "  %s = %s\n", key, trace.Selector[key])
		}
	}

	if len(trace.Ports) > 0 {
		writeSection(w, "Ports")
		for _, port := range trace.Ports {
			name := port.Name
			if name == "" {
				name = "-"
			}
			target := port.TargetPort
			if target == "" {
				target = "-"
			}
			fmt.Fprintf(w, "  %s %s %d -> %s\n", name, port.Protocol, port.Port, target)
		}
	}

	if len(trace.Pods) > 0 {
		writeSection(w, "Selected Pods")
		for _, pod := range trace.Pods {
			fmt.Fprintf(w, "  Pod %s/%s   Ready=%t   Restarts=%d   Node=%s\n", pod.Namespace, pod.Name, pod.Ready, pod.Restarts, pod.Node)
		}
	}

	if len(trace.Endpoints) > 0 {
		writeSection(w, "Endpoints")
		for _, endpoint := range trace.Endpoints {
			pod := endpoint.Pod
			if pod == "" {
				pod = "-"
			}
			fmt.Fprintf(w, "  %s   Ready=%t   Pod=%s\n", endpoint.ID, endpoint.Ready, pod)
		}
	}

	if len(trace.Owners) > 0 {
		writeSection(w, "Owners")
		for _, owner := range trace.Owners {
			fmt.Fprintf(w, "  %s %s/%s\n", owner.Kind, owner.Namespace, owner.Name)
		}
	}

	if len(trace.Signals) > 0 {
		writeSection(w, "Signals")
		for _, signal := range trace.Signals {
			fmt.Fprintf(w, "  %s %s: %s\n", signal.Severity, signal.Reason, signal.Message)
		}
	}

	return nil
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
