package model

type Entity struct {
	APIVersion string            `json:"apiVersion,omitempty"`
	Kind       string            `json:"kind"`
	Namespace  string            `json:"namespace,omitempty"`
	Name       string            `json:"name"`
	UID        string            `json:"uid,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}
