package model

type ServiceSummary struct {
	Kind         string            `json:"kind"`
	Namespace    string            `json:"namespace"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	ClusterIP    string            `json:"clusterIP,omitempty"`
	ExternalName string            `json:"externalName,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
}

type ServicePort struct {
	Name       string `json:"name,omitempty"`
	Protocol   string `json:"protocol"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort,omitempty"`
}

type EndpointSummary struct {
	ID       string `json:"id"`
	Address  string `json:"address"`
	Port     int32  `json:"port,omitempty"`
	PortName string `json:"portName,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Ready    bool   `json:"ready"`
	Pod      string `json:"pod,omitempty"`
	Node     string `json:"node,omitempty"`
	Source   string `json:"source"`
}

type PodBackend struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
	IP        string `json:"ip,omitempty"`
	Ready     bool   `json:"ready"`
	Restarts  int32  `json:"restarts"`
	Node      string `json:"node,omitempty"`
	Phase     string `json:"phase,omitempty"`
}
