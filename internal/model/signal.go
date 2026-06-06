package model

type Signal struct {
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
	Message  string `json:"message"`
	Source   string `json:"source,omitempty"`
}
