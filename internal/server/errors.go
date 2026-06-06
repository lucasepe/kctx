package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/lucasepe/kctx/internal/apperror"
	"github.com/lucasepe/kctx/internal/model"
)

func errorResponse(err error) (int, model.ErrorEnvelope) {
	if err == nil {
		return http.StatusInternalServerError, model.NewErrorEnvelope(model.ErrorInternal, "internal error")
	}
	if errors.Is(err, errBadRequest) {
		return http.StatusBadRequest, model.NewErrorEnvelope(model.ErrorBadRequest, err.Error())
	}
	if errors.Is(err, errNotFound) {
		return http.StatusNotFound, model.NewErrorEnvelope(model.ErrorNotFound, err.Error())
	}
	if errors.Is(err, errMethodNotAllowed) {
		return http.StatusMethodNotAllowed, model.NewErrorEnvelope(model.ErrorMethodNotAllowed, err.Error())
	}

	// Il client ha annullato la richiesta (es. chiuso il browser o fatto Ctrl+C su curl)
	if errors.Is(err, context.Canceled) {
		return 499, model.NewErrorEnvelope(model.ErrorClientClosedRequest, "client canceled the request")
	}

	// È scattato un timeout di contesto prima che l'operazione terminasse
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, model.NewErrorEnvelope(model.ErrorTimeout, "operation took too long to complete")
	}

	return apperror.HTTPStatusAndEnvelope(err)
}
