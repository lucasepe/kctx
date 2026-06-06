package limits

import (
	"context"
	"errors"
	"sync/atomic"
)

var ErrBudgetExceeded = errors.New("limit exceeded: Kubernetes API call budget exhausted")

type budgetContextKey struct{}

type Budget struct {
	remaining atomic.Int64
}

// NewBudget creates a simple token budget. Each guarded operation consumes one
// token; when no tokens remain, callers receive ErrBudgetExceeded.
func NewBudget(limit int) *Budget {
	b := &Budget{}
	b.remaining.Store(int64(limit))
	return b
}

// ContextWithBudget attaches a request-scoped budget to ctx. The concrete key
// stays private so only this package can read or write budget values.
func ContextWithBudget(ctx context.Context, budget *Budget) context.Context {
	if budget == nil {
		return ctx
	}
	return context.WithValue(ctx, budgetContextKey{}, budget)
}

func BudgetFromContext(ctx context.Context) *Budget {
	if ctx == nil {
		return nil
	}
	if budget, ok := ctx.Value(budgetContextKey{}).(*Budget); ok {
		return budget
	}
	return nil
}

func TakeBudget(ctx context.Context) error {
	budget := BudgetFromContext(ctx)
	if budget == nil {
		return nil
	}
	return budget.Take()
}

// Take consumes one token from the budget.
func (b *Budget) Take() error {
	if b == nil {
		return nil
	}
	for {
		remaining := b.remaining.Load()
		if remaining <= 0 {
			return ErrBudgetExceeded
		}
		if b.remaining.CompareAndSwap(remaining, remaining-1) {
			return nil
		}
	}
}

func (b *Budget) Remaining() int {
	if b == nil {
		return 0
	}
	remaining := b.remaining.Load()
	if remaining < 0 {
		return 0
	}
	return int(remaining)
}
