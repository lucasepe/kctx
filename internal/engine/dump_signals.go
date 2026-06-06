package engine

import (
	"fmt"
	"strings"

	"github.com/lucasepe/kctx/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
)

// dumpSignals derives factual operational signals for the namespace dump.
func dumpSignals(entities []model.DumpEntity, events []model.DumpEventSummary, deployments []appsv1.Deployment, replicaSets []appsv1.ReplicaSet, statefulSets []appsv1.StatefulSet, daemonSets []appsv1.DaemonSet, jobs []batchv1.Job, services []corev1.Service, slices []discoveryv1.EndpointSlice, pvcs []corev1.PersistentVolumeClaim) []model.DumpSignal {
	var signals []model.DumpSignal
	for _, entity := range entities {
		if entity.Kind == "Pod" {
			switch entity.Status {
			case "CrashLoopBackOff":
				signals = append(signals, model.DumpSignal{Severity: "error", Reason: "pod_crashloop", Message: fmt.Sprintf("Pod %s/%s is restarting repeatedly", entity.Namespace, entity.Name), EntityID: entity.ID})
			case "ImagePullBackOff", "ErrImagePull":
				signals = append(signals, model.DumpSignal{Severity: "error", Reason: "image_pull_error", Message: fmt.Sprintf("Pod %s/%s has image pull error %s", entity.Namespace, entity.Name, entity.Status), EntityID: entity.ID})
			}
			if entity.Ready != nil && !*entity.Ready {
				signals = append(signals, model.DumpSignal{Severity: "warning", Reason: "pod_not_ready", Message: fmt.Sprintf("Pod %s/%s is not Ready", entity.Namespace, entity.Name), EntityID: entity.ID})
			}
		}
	}
	signals = append(signals, dumpWorkloadSignals(deployments, replicaSets, statefulSets, daemonSets, jobs)...)
	signals = append(signals, dumpServiceSignals(services, slices)...)
	for _, pvc := range pvcs {
		if pvc.Status.Phase == corev1.ClaimPending || pvc.Status.Phase == corev1.ClaimLost {
			signals = append(signals, model.DumpSignal{Severity: "warning", Reason: "pvc_pending", Message: fmt.Sprintf("PVC %s/%s is %s", pvc.Namespace, pvc.Name, pvc.Status.Phase), EntityID: dumpID("PersistentVolumeClaim", pvc.Namespace, pvc.Name)})
		}
	}
	signals = append(signals, dumpEventSignals(events)...)
	sortDumpSignals(signals)
	return signals
}

func dumpEventSignals(events []model.DumpEventSummary) []model.DumpSignal {
	type eventGroup struct {
		count  int
		latest model.DumpEventSummary
	}
	groups := map[string]eventGroup{}
	for _, event := range events {
		key := event.ObjectKind + "|" + event.Reason + "|" + eventMessageClass(event.Message)
		group := groups[key]
		group.count++
		if group.latest.Timestamp.IsZero() || event.Timestamp.After(group.latest.Timestamp) {
			group.latest = event
		}
		groups[key] = group
	}

	signals := make([]model.DumpSignal, 0, len(groups))
	for _, group := range groups {
		event := group.latest
		signals = append(signals, model.DumpSignal{
			Severity: dumpEventSeverity(event),
			Reason:   dumpEventSignalCode(event),
			Message:  fmt.Sprintf("%d %s warning event(s) for %s resources; latest %s/%s: %s", group.count, event.Reason, event.ObjectKind, event.ObjectKind, event.ObjectName, event.Message),
		})
	}
	return signals
}

func dumpEventSeverity(event model.DumpEventSummary) string {
	switch event.Reason {
	case "BackOff":
		return "error"
	default:
		return "warning"
	}
}

func dumpEventSignalCode(event model.DumpEventSummary) string {
	switch event.Reason {
	case "BackOff":
		return "pod_backoff_events"
	case "Unhealthy":
		return "probe_warning_events"
	case "FailedScheduling":
		return "scheduling_warning_events"
	case "FailedCreate":
		return "create_warning_events"
	default:
		return eventSignalCode(event.Reason)
	}
}

func eventSignalCode(reason string) string {
	var out strings.Builder
	for i, r := range reason {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out.WriteByte('_')
		}
		out.WriteRune(r)
	}
	return strings.ToLower(out.String()) + "_warning_events"
}

func eventMessageClass(message string) string {
	switch {
	case strings.Contains(message, "Liveness probe failed"):
		return "liveness_probe_failed"
	case strings.Contains(message, "Readiness probe failed"):
		return "readiness_probe_failed"
	case strings.Contains(message, "untolerated taint"):
		return "untolerated_taint"
	case strings.Contains(message, "serviceaccount") && strings.Contains(message, "not found"):
		return "serviceaccount_not_found"
	case strings.Contains(message, "Back-off restarting failed container"):
		return "container_backoff"
	default:
		return message
	}
}

// dumpWorkloadSignals reports unavailable workload facts across built-in
// workload kinds.
func dumpWorkloadSignals(deployments []appsv1.Deployment, replicaSets []appsv1.ReplicaSet, statefulSets []appsv1.StatefulSet, daemonSets []appsv1.DaemonSet, jobs []batchv1.Job) []model.DumpSignal {
	var signals []model.DumpSignal
	for _, deploy := range deployments {
		desired := replicasValue(deploy.Spec.Replicas)
		if deploy.Status.AvailableReplicas < desired {
			id := dumpID("Deployment", deploy.Namespace, deploy.Name)
			signals = append(signals, model.DumpSignal{Severity: "warning", Reason: "workload_unavailable", Message: fmt.Sprintf("Deployment %s/%s available %d/%d", deploy.Namespace, deploy.Name, deploy.Status.AvailableReplicas, desired), EntityID: id})
		}
	}
	for _, rs := range replicaSets {
		desired := replicasValue(rs.Spec.Replicas)
		if rs.Status.ReadyReplicas < desired {
			id := dumpID("ReplicaSet", rs.Namespace, rs.Name)
			signals = append(signals, model.DumpSignal{Severity: "warning", Reason: "workload_unavailable", Message: fmt.Sprintf("ReplicaSet %s/%s ready %d/%d", rs.Namespace, rs.Name, rs.Status.ReadyReplicas, desired), EntityID: id})
		}
	}
	for _, sts := range statefulSets {
		desired := replicasValue(sts.Spec.Replicas)
		if sts.Status.ReadyReplicas < desired {
			id := dumpID("StatefulSet", sts.Namespace, sts.Name)
			signals = append(signals, model.DumpSignal{Severity: "warning", Reason: "workload_unavailable", Message: fmt.Sprintf("StatefulSet %s/%s ready %d/%d", sts.Namespace, sts.Name, sts.Status.ReadyReplicas, desired), EntityID: id})
		}
	}
	for _, ds := range daemonSets {
		if ds.Status.NumberAvailable < ds.Status.DesiredNumberScheduled {
			id := dumpID("DaemonSet", ds.Namespace, ds.Name)
			signals = append(signals, model.DumpSignal{Severity: "warning", Reason: "workload_unavailable", Message: fmt.Sprintf("DaemonSet %s/%s available %d/%d", ds.Namespace, ds.Name, ds.Status.NumberAvailable, ds.Status.DesiredNumberScheduled), EntityID: id})
		}
	}
	for _, job := range jobs {
		desired := jobCompletions(&job)
		if job.Status.Failed > 0 || job.Status.Succeeded < desired {
			id := dumpID("Job", job.Namespace, job.Name)
			signals = append(signals, model.DumpSignal{Severity: "warning", Reason: "workload_unavailable", Message: fmt.Sprintf("Job %s/%s succeeded %d/%d failed %d", job.Namespace, job.Name, job.Status.Succeeded, desired, job.Status.Failed), EntityID: id})
		}
	}
	return signals
}

// dumpServiceSignals reports Services that currently have no ready endpoints.
func dumpServiceSignals(services []corev1.Service, slices []discoveryv1.EndpointSlice) []model.DumpSignal {
	readyByService := map[string]int{}
	for _, slice := range slices {
		serviceName := slice.Labels[serviceNameLabel]
		for _, endpoint := range slice.Endpoints {
			for range endpoint.Addresses {
				if endpointReady(endpoint.Conditions.Ready) {
					readyByService[serviceName]++
				}
			}
		}
	}
	var signals []model.DumpSignal
	for _, service := range services {
		if service.Spec.Type == corev1.ServiceTypeExternalName {
			continue
		}
		if readyByService[service.Name] == 0 {
			id := dumpID("Service", service.Namespace, service.Name)
			signals = append(signals, model.DumpSignal{Severity: "warning", Reason: "service_without_ready_endpoints", Message: fmt.Sprintf("Service %s/%s has no ready endpoints", service.Namespace, service.Name), EntityID: id})
		}
	}
	return signals
}
