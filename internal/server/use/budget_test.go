package use

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lucasepe/kctx/internal/limits"
)

func TestKubeAPIBudgetAddsBudget(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		budget := limits.BudgetFromContext(r.Context())
		if budget == nil {
			t.Fatal("budget from context is nil")
		}
		if got := budget.Remaining(); got != 3 {
			t.Fatalf("remaining budget = %d, want 3", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	KubeAPIBudget(3)(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestKubeAPIBudgetCanBeDisabled(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if budget := limits.BudgetFromContext(r.Context()); budget != nil {
			t.Fatalf("budget from context = %#v, want nil", budget)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	KubeAPIBudget(0)(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
