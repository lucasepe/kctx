package model

import "time"

type DumpSummary struct {
	Pods           int `json:"pods"`
	Deployments    int `json:"deployments"`
	StatefulSets   int `json:"statefulSets"`
	DaemonSets     int `json:"daemonSets"`
	Jobs           int `json:"jobs"`
	CronJobs       int `json:"cronJobs"`
	Services       int `json:"services"`
	EndpointSlices int `json:"endpointSlices"`
	PVCs           int `json:"pvcs"`
	Nodes          int `json:"nodes"`
	WarningEvents  int `json:"warningEvents"`
	Signals        int `json:"signals"`
}

type DumpEntity struct {
	ID           string            `json:"id"`
	Kind         string            `json:"kind"`
	Namespace    string            `json:"namespace,omitempty"`
	Name         string            `json:"name"`
	UID          string            `json:"uid,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Status       string            `json:"status,omitempty"`
	Ready        *bool             `json:"ready,omitempty"`
	NodeName     string            `json:"nodeName,omitempty"`
	RestartCount int32             `json:"restartCount,omitempty"`
	LastState    string            `json:"lastState,omitempty"`
	LastReason   string            `json:"lastStateReason,omitempty"`
}

type DumpRelation struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Target string `json:"target"`
	Reason string `json:"reason,omitempty"`
}

type DumpSignal struct {
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
	Message  string `json:"message"`
	EntityID string `json:"entityId,omitempty"`
}

type DumpEventSummary struct {
	Type       string    `json:"type"`
	Reason     string    `json:"reason"`
	Message    string    `json:"message"`
	ObjectKind string    `json:"objectKind"`
	ObjectName string    `json:"objectName"`
	Timestamp  time.Time `json:"timestamp"`
}
