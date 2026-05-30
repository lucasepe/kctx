package engine

import (
	"fmt"

	"github.com/lucasepe/kctx/internal/model"
	corev1 "k8s.io/api/core/v1"
)

// traceSignals reports factual Service backend conditions without inferring
// root cause.
func traceSignals(service *corev1.Service, pods []model.PodBackend, endpoints []model.EndpointSummary) []model.Signal {
	var signals []model.Signal
	if len(endpoints) == 0 {
		signals = append(signals, model.Signal{Severity: "warning", Reason: "no_endpoints_found", Message: fmt.Sprintf("service %s/%s has no endpoints", service.Namespace, service.Name), Source: "endpoints"})
	}

	endpointPods := map[string]bool{}
	readyEndpoints := 0
	for _, endpoint := range endpoints {
		if endpoint.Pod != "" {
			endpointPods[endpoint.Pod] = true
		} else {
			signals = append(signals, model.Signal{Severity: "warning", Reason: "endpoint_without_pod", Message: fmt.Sprintf("endpoint %s does not resolve to a Pod", endpoint.ID), Source: "endpoints"})
		}
		if endpoint.Ready {
			readyEndpoints++
		} else {
			signals = append(signals, model.Signal{Severity: "warning", Reason: "endpoint_not_ready", Message: fmt.Sprintf("endpoint %s is not Ready", endpoint.ID), Source: "endpoints"})
		}
	}

	readyPods := 0
	for _, pod := range pods {
		if pod.Ready {
			readyPods++
		} else {
			signals = append(signals, model.Signal{Severity: "warning", Reason: "selected_pod_not_ready", Message: fmt.Sprintf("Pod %s/%s is selected by the Service but is not Ready", pod.Namespace, pod.Name), Source: "pod"})
		}
		if len(endpoints) > 0 && !endpointPods[pod.Name] && pod.IP != "" {
			signals = append(signals, model.Signal{Severity: "warning", Reason: "selected_pod_missing_from_endpoints", Message: fmt.Sprintf("Pod %s/%s is selected by the Service but is missing from endpoints", pod.Namespace, pod.Name), Source: "endpoints"})
		}
	}
	if len(pods) > 0 && readyPods == 0 {
		signals = append(signals, model.Signal{Severity: "critical", Reason: "all_backends_unready", Message: fmt.Sprintf("all selected Pods for service %s/%s are unready", service.Namespace, service.Name), Source: "pod"})
	}
	if service.Spec.Type != corev1.ServiceTypeExternalName && readyEndpoints == 0 {
		signals = append(signals, model.Signal{Severity: "critical", Reason: "service_has_no_usable_backends", Message: fmt.Sprintf("service %s/%s has no ready endpoints", service.Namespace, service.Name), Source: "endpoints"})
	}

	return signals
}
