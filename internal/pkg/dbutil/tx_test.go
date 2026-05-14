package dbutil

import (
	"context"
	"errors"
	"testing"
)

func TestWithTransactionRunner(t *testing.T) {
	wantErr := errors.New("callback failed")
	var runnerCalled bool
	var callbackCalled bool

	ctx := WithTransactionRunner(context.Background(), func(ctx context.Context, fn func(context.Context) error) error {
		runnerCalled = true
		return fn(ctx)
	})

	err := WithTransaction(ctx, func(context.Context) error {
		callbackCalled = true
		return wantErr
	})
	if !runnerCalled {
		t.Fatal("expected transaction runner to be called")
	}
	if !callbackCalled {
		t.Fatal("expected transaction callback to be called")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}
