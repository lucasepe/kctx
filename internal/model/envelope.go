package model

const SchemaVersion = "kctx.io/v1alpha1"

const (
	ErrorBadRequest          = "bad_request"
	ErrorNotFound            = "not_found"
	ErrorForbidden           = "forbidden"
	ErrorMethodNotAllowed    = "method_not_allowed"
	ErrorUnsupportedResource = "unsupported_resource"
	ErrorTimeout             = "timeout"
	ErrorLimitExceeded       = "limit_exceeded"
	ErrorClientClosedRequest = "client_closed_request"
	ErrorInternal            = "internal_error"
)

type ErrorEnvelope struct {
	SchemaVersion string      `json:"schemaVersion"`
	Kind          string      `json:"kind"`
	Error         ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

func NewErrorEnvelope(code, message string) ErrorEnvelope {
	return ErrorEnvelope{
		SchemaVersion: SchemaVersion,
		Kind:          "Error",
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	}
}

func NewErrorEnvelopeWithDetails(code, message string, details map[string]string) ErrorEnvelope {
	envelope := NewErrorEnvelope(code, message)
	if len(details) > 0 {
		envelope.Error.Details = make(map[string]string, len(details))
		for key, value := range details {
			envelope.Error.Details[key] = value
		}
	}
	return envelope
}
