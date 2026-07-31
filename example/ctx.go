package example

import (
	"context"
	"errors"
)

var ErrTimeout = errors.New("operation timed out")

func DoWork(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancelCause(ctx)
	cancel(ErrTimeout)

	return ctx
}

func DoWorkUnchecked(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancelCause(ctx)
	cancel(ErrTimeout)

	return ctx
}
