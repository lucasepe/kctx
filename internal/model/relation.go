package model

type Relation struct {
	Type       string `json:"type"`
	Source     Entity `json:"source"`
	Target     Entity `json:"target"`
	Confidence string `json:"confidence,omitempty"`
	Reason     string `json:"reason,omitempty"`
}
