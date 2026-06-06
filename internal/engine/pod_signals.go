package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lucasepe/kctx/internal/model"
)

const highRestartThreshold int32 = 5

// buildSignals turns Pod status and related Events into factual diagnostic
// signals without attempting root-cause inference.
func buildSignals(status model.PodStatus, events []model.Event) []model.Signal {
	var signals []model.Signal
	if !status.Ready {
		signals = append(signals, model.Signal{Severity: "warning", Reason: "pod_not_ready", Message: "pod is not ready", Source: "status"})
	}
	for _, container := range status.Containers {
		if container.RestartCount > 0 && (container.Reason != "" || container.LastStateReason != "") {
			signals = append(signals, model.Signal{
				Severity: containerRestartCycleSeverity(container),
				Reason:   "container_restart_cycle",
				Message:  containerRestartCycleMessage(container),
				Source:   "status",
			})
		}
		if container.Reason == "CrashLoopBackOff" {
			signals = append(signals, model.Signal{Severity: "error", Reason: "container_crashloop", Message: "container " + container.Name + " is restarting repeatedly", Source: "status"})
		}
		if container.Reason == "ImagePullBackOff" || container.Reason == "ErrImagePull" {
			signals = append(signals, model.Signal{Severity: "error", Reason: "image_pull_error", Message: "container " + container.Name + " cannot pull its image: " + container.Reason, Source: "status"})
		}
		if container.LastStateReason == "OOMKilled" {
			signals = append(signals, model.Signal{Severity: "error", Reason: "container_oom_killed", Message: "container " + container.Name + " was previously OOMKilled", Source: "status"})
		} else if container.LastState == "terminated" && container.LastStateReason != "" && container.LastStateReason != "Completed" {
			signals = append(signals, model.Signal{Severity: "warning", Reason: "container_last_terminated", Message: "container " + container.Name + " previously terminated: " + container.LastStateReason, Source: "status"})
		}
		if container.RestartCount >= highRestartThreshold {
			signals = append(signals, model.Signal{Severity: "warning", Reason: "high_restart_count", Message: fmt.Sprintf("container %s has %d restarts", container.Name, container.RestartCount), Source: "status"})
		} else if container.RestartCount > 0 {
			signals = append(signals, model.Signal{Severity: "info", Reason: "container_restarted", Message: fmt.Sprintf("container %s has restarted %d time(s)", container.Name, container.RestartCount), Source: "status"})
		}
	}
	signals = append(signals, podEventSignals(status.Ready, events)...)
	sortSignals(signals)
	return signals
}

func containerRestartCycleSeverity(container model.ContainerStatus) string {
	if container.Reason == "CrashLoopBackOff" || container.LastStateReason == "OOMKilled" {
		return "error"
	}
	if container.RestartCount >= highRestartThreshold {
		return "warning"
	}
	return "info"
}

func containerRestartCycleMessage(container model.ContainerStatus) string {
	message := fmt.Sprintf("container %s state=%s restarts=%d", container.Name, container.State, container.RestartCount)
	if container.Reason != "" {
		message += " reason=" + container.Reason
	}
	if container.LastState != "" {
		message += " lastState=" + container.LastState
	}
	if container.LastStateReason != "" {
		message += " lastStateReason=" + container.LastStateReason
	}
	return message
}

func podEventSignals(podReady bool, events []model.Event) []model.Signal {
	var signals []model.Signal
	backoffs := podEventGroup{}
	probeFailures := podEventGroup{}
	schedulingWarnings := podEventGroup{}
	otherWarnings := map[string]podEventGroup{}

	for _, event := range events {
		switch event.Reason {
		case "BackOff":
			backoffs = addPodEvent(backoffs, event)
		case "FailedScheduling":
			schedulingWarnings = addPodEvent(schedulingWarnings, event)
		case "Unhealthy":
			probeFailures = addPodEvent(probeFailures, event)
		default:
			if event.Type == "Warning" {
				otherWarnings[event.Reason] = addPodEvent(otherWarnings[event.Reason], event)
			}
		}
	}

	if backoffs.count > 0 {
		severity := "warning"
		if !podReady {
			severity = "error"
		}
		signals = append(signals, model.Signal{Severity: severity, Reason: "pod_backoff_event", Message: fmt.Sprintf("%d BackOff warning event(s); latest: %s", backoffs.count, backoffs.latest.Message), Source: "event"})
	}
	if probeFailures.count > 0 {
		severity := "info"
		reason := "transient_probe_warnings"
		if !podReady {
			severity = "warning"
			reason = "probe_failures"
		}
		signals = append(signals, model.Signal{Severity: severity, Reason: reason, Message: fmt.Sprintf("%d probe warning event(s); latest: %s", probeFailures.count, probeFailures.latest.Message), Source: "event"})
	}
	if schedulingWarnings.count > 0 {
		severity := "info"
		reason := "resolved_scheduling_warnings"
		if !podReady {
			severity = "warning"
			reason = "scheduling_warnings"
		}
		signals = append(signals, model.Signal{Severity: severity, Reason: reason, Message: fmt.Sprintf("%d scheduling warning event(s); latest: %s", schedulingWarnings.count, schedulingWarnings.latest.Message), Source: "event"})
	}
	for reason, group := range otherWarnings {
		signals = append(signals, model.Signal{Severity: "warning", Reason: podEventSignalReason(reason), Message: fmt.Sprintf("%d %s warning event(s); latest: %s", group.count, reason, group.latest.Message), Source: "event"})
	}

	return signals
}

type podEventGroup struct {
	count  int32
	latest model.Event
}

func addPodEvent(group podEventGroup, event model.Event) podEventGroup {
	group.count += event.Count
	if group.count == 0 {
		group.count = 1
	}
	if group.latest.LastTimestamp.IsZero() || event.LastTimestamp.After(group.latest.LastTimestamp) {
		group.latest = event
	}
	return group
}

func podEventSignalReason(reason string) string {
	var out strings.Builder
	for i, r := range reason {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out.WriteByte('_')
		}
		out.WriteRune(r)
	}
	return strings.ToLower(out.String()) + "_warning_events"
}

func sortSignals(signals []model.Signal) {
	sort.Slice(signals, func(i, j int) bool {
		if severityRank(signals[i].Severity) != severityRank(signals[j].Severity) {
			return severityRank(signals[i].Severity) > severityRank(signals[j].Severity)
		}
		if signals[i].Reason != signals[j].Reason {
			return signals[i].Reason < signals[j].Reason
		}
		if signals[i].Source != signals[j].Source {
			return signals[i].Source < signals[j].Source
		}
		return signals[i].Message < signals[j].Message
	})
}
