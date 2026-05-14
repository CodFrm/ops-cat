package dbutil

import (
	"context"

	"github.com/cago-frame/cago/database/db"
	"gorm.io/gorm"
)

type transactionRunnerKey struct{}

// TransactionRunner allows tests to mock the transaction boundary without a real database.
type TransactionRunner func(context.Context, func(context.Context) error) error

// WithTransactionRunner returns a context that uses runner for dbutil.WithTransaction.
func WithTransactionRunner(ctx context.Context, runner TransactionRunner) context.Context {
	return context.WithValue(ctx, transactionRunnerKey{}, runner)
}

// WithTransaction runs fn with a transactional DB stored in ctx.
func WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	if runner, ok := ctx.Value(transactionRunnerKey{}).(TransactionRunner); ok {
		if runner != nil {
			return runner(ctx, fn)
		}
	}
	return db.Ctx(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(db.WithContextDB(ctx, tx))
	})
}
