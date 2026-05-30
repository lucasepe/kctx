package engine

import (
	"fmt"
	"strings"

	"github.com/lucasepe/kctx/internal/model"
	corev1 "k8s.io/api/core/v1"
)

type namespaceEventGroup struct {
	count  int
	latest model.EventSummary
}

// namespaceSignals derives factual namespace health signals from already
// normalized resource summaries.
func namespaceSignals(namespace string, pods []model.PodHealth, workloads []model.WorkloadHealth, services []model.ServiceHealth, pvcs []model.PVCHealth, events []model.EventSummary, rawPodCount, workloadCount, serviceCount, pvcCount int) []model.Signal {
	var signals []model.Signal
	if rawPodCount == 0 && workloadCount == 0 && serviceCount == 0 && pvcCount == 0 {
		signals = append(signals, model.Signal{Severity: "info", Reason: "namespace_empty", Message: fmt.Sprintf("namespace %s has no supported resources", namespace)})
	}
	for _, pod := range pods {
		source := fmt.Sprintf("Pod/%s/%s", pod.Namespace, pod.Name)
		if !pod.Ready {
			signals = append(signals, model.Signal{Severity: "warning", Reason: "pod_not_ready", Message: fmt.Sprintf("Pod %s/%s is not Ready", pod.Namespace, pod.Name), Source: source})
		}
		switch pod.Reason {
		case "CrashLoopBackOff":
			signals = append(signals, model.Signal{Severity: "critical", Reason: "pod_crashloop", Message: fmt.Sprintf("Pod %s/%s is in CrashLoopBackOff", pod.Namespace, pod.Name), Source: source})
		case "ImagePullBackOff", "ErrImagePull":
			signals = append(signals, model.Signal{Severity: "critical", Reason: "pod_image_pull_error", Message: fmt.Sprintf("Pod %s/%s has image pull error %s", pod.Namespace, pod.Name, pod.Reason), Source: source})
		}
		if pod.Restarts >= highRestartThreshold {
			signals = append(signals, model.Signal{Severity: "warning", Reason: "high_restart_count", Message: fmt.Sprintf("Pod %s/%s has %d restarts", pod.Namespace, pod.Name, pod.Restarts), Source: source})
		}
	}
	for _, workload := range workloads {
		if !workload.Healthy {
			signals = append(signals, model.Signal{Severity: "warning", Reason: "workload_replicas_unavailable", Message: fmt.Sprintf("%s %s/%s is not fully available", workload.Kind, workload.Namespace, workload.Name), Source: fmt.Sprintf("%s/%s/%s", workload.Kind, workload.Namespace, workload.Name)})
		}
	}
	for _, service := range services {
		if service.ReadyEndpoints == 0 {
			signals = append(signals, model.Signal{Severity: "warning", Reason: "service_without_ready_endpoints", Message: fmt.Sprintf("Service %s/%s has no ready endpoints", service.Namespace, service.Name), Source: fmt.Sprintf("Service/%s/%s", service.Namespace, service.Name)})
		}
	}
	for _, pvc := range pvcs {
		if pvc.Phase == string(corev1.ClaimPending) || pvc.Phase == string(corev1.ClaimLost) {
			signals = append(signals, model.Signal{Severity: "warning", Reason: "pvc_pending", Message: fmt.Sprintf("PVC %s/%s is %s", pvc.Namespace, pvc.Name, pvc.Phase), Source: fmt.Sprintf("PersistentVolumeClaim/%s/%s", pvc.Namespace, pvc.Name)})
		}
	}
	signals = append(signals, namespaceEventSignals(pods, events)...)
	return signals
}

func namespaceEventSignals(pods []model.PodHealth, events []model.EventSummary) []model.Signal {
	var signals []model.Signal
	podReady := make(map[string]bool, len(pods))
	for _, pod := range pods {
		podReady[pod.Name] = pod.Ready
	}

	backoffs := map[string]namespaceEventGroup{}
	probeFailures := map[string]namespaceEventGroup{}
	otherWarnings := map[string]namespaceEventGroup{}
	readyProbeWarnings := namespaceEventGroup{}
	readySchedulingWarnings := namespaceEventGroup{}

	for _, event := range events {
		key := fmt.Sprintf("%s/%s", event.Kind, event.Name)
		switch event.Reason {
		case "BackOff":
			backoffs[key] = addEvent(backoffs[key], event)
		case "Unhealthy":
			if event.Kind == "Pod" && podReady[event.Name] {
				readyProbeWarnings = addEvent(readyProbeWarnings, event)
				continue
			}
			probeFailures[key] = addEvent(probeFailures[key], event)
		case "FailedScheduling":
			if event.Kind == "Pod" && podReady[event.Name] {
				readySchedulingWarnings = addEvent(readySchedulingWarnings, event)
				continue
			}
			otherWarnings[key+"|"+event.Reason] = addEvent(otherWarnings[key+"|"+event.Reason], event)
		default:
			otherWarnings[key+"|"+event.Reason] = addEvent(otherWarnings[key+"|"+event.Reason], event)
		}
	}

	for source, group := range backoffs {
		signals = append(signals, model.Signal{
			Severity: "critical",
			Reason:   "pod_backoff_event",
			Message:  fmt.Sprintf("%s has %d BackOff warning event(s); latest: %s", source, group.count, group.latest.Message),
			Source:   source,
		})
	}
	for source, group := range probeFailures {
		signals = append(signals, model.Signal{
			Severity: "warning",
			Reason:   "probe_failures",
			Message:  fmt.Sprintf("%s has %d probe failure warning event(s); latest: %s", source, group.count, group.latest.Message),
			Source:   source,
		})
	}
	for source, group := range otherWarnings {
		displaySource := strings.Split(source, "|")[0]
		signals = append(signals, model.Signal{
			Severity: "warning",
			Reason:   eventSignalReason(group.latest.Reason),
			Message:  fmt.Sprintf("%s has %d %s warning event(s); latest: %s", displaySource, group.count, group.latest.Reason, group.latest.Message),
			Source:   displaySource,
		})
	}
	if readyProbeWarnings.count > 0 {
		signals = append(signals, model.Signal{
			Severity: "info",
			Reason:   "transient_probe_warnings",
			Message:  fmt.Sprintf("%d probe warning event(s) are on Pods that are currently Ready; latest: %s/%s: %s", readyProbeWarnings.count, readyProbeWarnings.latest.Kind, readyProbeWarnings.latest.Name, readyProbeWarnings.latest.Message),
		})
	}
	if readySchedulingWarnings.count > 0 {
		signals = append(signals, model.Signal{
			Severity: "info",
			Reason:   "resolved_scheduling_warnings",
			Message:  fmt.Sprintf("%d scheduling warning event(s) are on Pods that are currently Ready; latest: %s/%s: %s", readySchedulingWarnings.count, readySchedulingWarnings.latest.Kind, readySchedulingWarnings.latest.Name, readySchedulingWarnings.latest.Message),
		})
	}
	return signals
}

func eventSignalReason(reason string) string {
	var out strings.Builder
	for i, r := range reason {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out.WriteByte('_')
		}
		out.WriteRune(r)
	}
	return strings.ToLower(out.String()) + "_warning_events"
}

func addEvent(group namespaceEventGroup, event model.EventSummary) namespaceEventGroup {
	group.count++
	if group.latest.Timestamp.IsZero() || event.Timestamp.After(group.latest.Timestamp) {
		group.latest = event
	}
	return group
}
