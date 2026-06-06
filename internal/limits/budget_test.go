package limits

import (
	"context"
	"errors"
	"testing"
)

func TestBudgetTake(t *testing.T) {
	budget := NewBudget(2)

	if err := budget.Take(); err != nil {
		t.Fatalf("first Take() error = %v", err)
	}
	if err := budget.Take(); err != nil {
		t.Fatalf("second Take() error = %v", err)
	}
	if err := budget.Take(); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("third Take() error = %v, want ErrBudgetExceeded", err)
	}
	if got := budget.Remaining(); got != 0 {
		t.Fatalf("Remaining() = %d, want 0", got)
	}
}

func TestTakeBudgetWithoutBudgetIsNoop(t *testing.T) {
	if err := TakeBudget(context.Background()); err != nil {
		t.Fatalf("TakeBudget() error = %v", err)
	}
}
