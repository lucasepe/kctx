package apperror_test

import (
	"net/http"
	"testing"

	"github.com/lucasepe/kctx/internal/apperror"
	"github.com/lucasepe/kctx/internal/engine"
	"github.com/lucasepe/kctx/internal/kube"
	"github.com/lucasepe/kctx/internal/model"
)

func TestUnsupportedResourceErrorEnvelopeIncludesDetails(t *testing.T) {
	err := engine.UnsupportedResourceError{
		APIVersion: "argoproj.io/v1alpha1",
		Kind:       "AppProject",
		Resource:   kube.ResourceRef{Group: "argoproj.io", Version: "v1alpha1", Resource: "appprojects"},
	}

	status, envelope := apperror.HTTPStatusAndEnvelope(err)

	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", status, http.StatusUnprocessableEntity)
	}
	if envelope.Error.Code != model.ErrorUnsupportedResource {
		t.Fatalf("code = %q, want %q", envelope.Error.Code, model.ErrorUnsupportedResource)
	}
	wantDetails := map[string]string{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "AppProject",
		"resource":   "argoproj.io/v1alpha1, Resource=appprojects",
	}
	for key, want := range wantDetails {
		if got := envelope.Error.Details[key]; got != want {
			t.Fatalf("details[%q] = %q, want %q", key, got, want)
		}
	}
}
