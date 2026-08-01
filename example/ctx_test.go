package example

import (
	"context"
	"testing"
)

func TestDoWork(t *testing.T) {
	ctx := DoWork(context.Background())
	if ctx.Err() == nil {
		t.Error("context should be cancelled")
	}

	cause := context.Cause(ctx)
	if cause == nil {
		t.Error("cause should not be nil")
	}

	if cause != ErrTimeout {
		t.Errorf("cause = %v, want ErrTimeout", cause)
	}
}

// Weak test: only checks context is done, not the cause.
// cancel(err) -> cancel(nil) mutation survives because ctx.Err() is
// still non-nil.
func TestDoWorkUnchecked_Weak(t *testing.T) {
	ctx := DoWorkUnchecked(context.Background())
	if ctx.Err() == nil {
		t.Error("context should be cancelled")
	}
}
