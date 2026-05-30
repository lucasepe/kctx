package model

import "time"

type PodStatus struct {
	Phase      string            `json:"phase"`
	Ready      bool              `json:"ready"`
	Restarts   int32             `json:"restarts"`
	Conditions []PodCondition    `json:"conditions,omitempty"`
	Containers []ContainerStatus `json:"containers,omitempty"`
}

type PodCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type ContainerStatus struct {
	Name             string `json:"name"`
	Ready            bool   `json:"ready"`
	RestartCount     int32  `json:"restartCount"`
	State            string `json:"state,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Message          string `json:"message,omitempty"`
	LastState        string `json:"lastState,omitempty"`
	LastStateReason  string `json:"lastStateReason,omitempty"`
	LastStateMessage string `json:"lastStateMessage,omitempty"`
}

type VolumeRef struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type Event struct {
	Type           string    `json:"type,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	Message        string    `json:"message,omitempty"`
	Source         string    `json:"source,omitempty"`
	FirstTimestamp time.Time `json:"firstTimestamp,omitempty"`
	LastTimestamp  time.Time `json:"lastTimestamp,omitempty"`
	Count          int32     `json:"count,omitempty"`
}
