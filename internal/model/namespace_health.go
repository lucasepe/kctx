package model

import "time"

type HealthSummary struct {
	PodsTotal                int `json:"podsTotal"`
	PodsReady                int `json:"podsReady"`
	PodsNotReady             int `json:"podsNotReady"`
	PodsRestarting           int `json:"podsRestarting"`
	WorkloadsTotal           int `json:"workloadsTotal"`
	WorkloadsHealthy         int `json:"workloadsHealthy"`
	WorkloadsUnhealthy       int `json:"workloadsUnhealthy"`
	ServicesTotal            int `json:"servicesTotal"`
	ServicesWithoutEndpoints int `json:"servicesWithoutEndpoints"`
	PVCsTotal                int `json:"pvcsTotal"`
	PVCsPending              int `json:"pvcsPending"`
	WarningEvents            int `json:"warningEvents"`
	ErrorSignals             int `json:"errorSignals"`
}

type WorkloadHealth struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Desired   int32  `json:"desired,omitempty"`
	Ready     int32  `json:"ready,omitempty"`
	Available int32  `json:"available,omitempty"`
	Succeeded int32  `json:"succeeded,omitempty"`
	Failed    int32  `json:"failed,omitempty"`
	Active    int32  `json:"active,omitempty"`
	Healthy   bool   `json:"healthy"`
	Message   string `json:"message,omitempty"`
}

type PodHealth struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Phase     string `json:"phase,omitempty"`
	Ready     bool   `json:"ready"`
	Restarts  int32  `json:"restarts"`
	Reason    string `json:"reason,omitempty"`
	Node      string `json:"node,omitempty"`
}

type ServiceHealth struct {
	Namespace      string `json:"namespace"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	ReadyEndpoints int    `json:"readyEndpoints"`
	TotalEndpoints int    `json:"totalEndpoints"`
}

type PVCHealth struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Phase     string `json:"phase"`
}

type EventSummary struct {
	Type      string    `json:"type,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	Message   string    `json:"message,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	Name      string    `json:"name,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}
