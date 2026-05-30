package server

import (
	"context"
	"errors"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func errorResponse(err error) (int, ErrorResponse) {
	if err == nil {
		return http.StatusInternalServerError, ErrorResponse{Error: "internal_error"}
	}
	if apierrors.IsNotFound(err) {
		return http.StatusNotFound, ErrorResponse{Error: "not_found", Message: err.Error()}
	}
	if errors.Is(err, errBadRequest) {
		return http.StatusBadRequest, ErrorResponse{Error: "bad_request", Message: err.Error()}
	}
	if errors.Is(err, errNotFound) {
		return http.StatusNotFound, ErrorResponse{Error: "not_found", Message: err.Error()}
	}
	if errors.Is(err, errMethodNotAllowed) {
		return http.StatusMethodNotAllowed, ErrorResponse{Error: "method_not_allowed", Message: err.Error()}
	}

	// Il client ha annullato la richiesta (es. chiuso il browser o fatto Ctrl+C su curl)
	if errors.Is(err, context.Canceled) {
		return 499, ErrorResponse{Error: "client_closed_request", Message: "The client canceled the request"}
	}

	// È scattato un timeout di contesto prima che l'operazione terminasse
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, ErrorResponse{Error: "gateway_timeout", Message: "The operation took too long to complete"}
	}

	return http.StatusInternalServerError, ErrorResponse{Error: "internal_error"}
}
