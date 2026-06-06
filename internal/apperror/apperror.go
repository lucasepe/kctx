package apperror

import (
	"context"
	"errors"
	"net/http"

	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/limits"
	"github.com/lucasepe/kctx/internal/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func Envelope(err error) model.ErrorEnvelope {
	status, envelope := HTTPStatusAndEnvelope(err)
	_ = status
	return envelope
}

func HTTPStatusAndEnvelope(err error) (int, model.ErrorEnvelope) {
	if err == nil {
		return http.StatusInternalServerError, model.NewErrorEnvelope(model.ErrorInternal, "internal error")
	}

	var unsupported engine.UnsupportedResourceError
	if errors.As(err, &unsupported) {
		envelope := model.NewErrorEnvelopeWithDetails(model.ErrorUnsupportedResource, err.Error(), map[string]string{
			"apiVersion": unsupported.APIVersion,
			"kind":       unsupported.Kind,
			"resource":   unsupported.Resource.GVR().String(),
		})
		return http.StatusUnprocessableEntity, envelope
	}

	if apierrors.IsNotFound(err) {
		return http.StatusNotFound, model.NewErrorEnvelope(model.ErrorNotFound, err.Error())
	}
	if apierrors.IsForbidden(err) {
		return http.StatusForbidden, model.NewErrorEnvelope(model.ErrorForbidden, err.Error())
	}
	if errors.Is(err, context.Canceled) {
		return 499, model.NewErrorEnvelope(model.ErrorClientClosedRequest, "client canceled the request")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, model.NewErrorEnvelope(model.ErrorTimeout, "operation took too long to complete")
	}
	if errors.Is(err, limits.ErrBudgetExceeded) {
		return http.StatusTooManyRequests, model.NewErrorEnvelope(model.ErrorLimitExceeded, err.Error())
	}

	return http.StatusInternalServerError, model.NewErrorEnvelope(model.ErrorInternal, err.Error())
}
