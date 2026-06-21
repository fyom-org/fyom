package app

import (
	"context"
	"testing"
)

func TestDesktopRuntimeShutdownNil(t *testing.T) {
	// Shutdown on nil runtime must not panic.
	var rt *DesktopRuntime
	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatalf("expected nil error for nil runtime shutdown, got: %v", err)
	}
}

func TestDesktopRuntimeShutdownIdempotent(t *testing.T) {
	// Calling Shutdown twice must not panic.
	rt, err := NewDesktopRuntime(context.Background(), t.TempDir()+"/test.db")
	if err != nil {
		// If we can't create a runtime (e.g., no config), skip.
		t.Skipf("could not create desktop runtime: %v", err)
	}

	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatalf("first shutdown failed: %v", err)
	}

	// Second shutdown must not panic.
	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown failed: %v", err)
	}
}

func TestDesktopRuntimeShutdownPartialStartup(t *testing.T) {
	// Shutdown after a failed startup must not panic.
	rt, err := NewDesktopRuntime(context.Background(), t.TempDir()+"/partial.db")
	if err != nil {
		t.Skipf("could not create desktop runtime: %v", err)
	}

	// Simulate a context cancellation after creation.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Startup with canceled context should fail, but Shutdown must still work.
	_ = rt.Startup(ctx)

	// Shutdown must not panic even if startup failed.
	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown after failed startup failed: %v", err)
	}
}

func TestDesktopRuntimeRouterNil(t *testing.T) {
	// Router on nil runtime must return nil.
	var rt *DesktopRuntime
	if r := rt.Router(); r != nil {
		t.Fatalf("expected nil router for nil runtime, got: %v", r)
	}
}
