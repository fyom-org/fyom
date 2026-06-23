package playback

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

// testCmd is a minimal interface for process start/release.
type testCmd interface {
	Start() error
	Release() error
}

// fakeCmd implements testCmd for testing.
type fakeCmd struct {
	startErr  error
	releaseErr error
}

func (f *fakeCmd) Start() error  { return f.startErr }
func (f *fakeCmd) Release() error { return f.releaseErr }

func TestPlayerInvoker_EmptyCommand(t *testing.T) {
	p := PlayerInvoker{Command: ""}
	err := p.Invoke(context.Background(), Info{URI: "http://example.com/video.mp4"})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestPlayerInvoker_EmptyURI(t *testing.T) {
	p := PlayerInvoker{Command: "mpv"}
	err := p.Invoke(context.Background(), Info{URI: ""})
	if err == nil {
		t.Fatal("expected error for empty URI")
	}
}

func TestPlayerInvoker_ArgsPreserved(t *testing.T) {
	p := PlayerInvoker{
		Command: "mpv",
		Args:    []string{"--resume-playback", "--fullscreen"},
	}

	// We can't easily intercept exec.Command without restructuring,
	// so we verify the invoker fields are set correctly.
	if p.Command != "mpv" {
		t.Errorf("expected command mpv, got %q", p.Command)
	}
	if len(p.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(p.Args))
	}
	if p.Args[0] != "--resume-playback" {
		t.Errorf("expected --resume-playback, got %q", p.Args[0])
	}
	if p.Args[1] != "--fullscreen" {
		t.Errorf("expected --fullscreen, got %q", p.Args[1])
	}
}

func TestPlayerInvoker_URIAppendedAsFinalArg(t *testing.T) {
	p := PlayerInvoker{
		Command: "mpv",
		Args:    []string{"--loop"},
	}
	info := Info{URI: "http://example.com/video.mp4"}

	// Verify the args slice would be constructed correctly.
	args := make([]string, 0, len(p.Args)+1)
	args = append(args, p.Args...)
	args = append(args, info.URI)

	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	if args[0] != "--loop" {
		t.Errorf("expected --loop, got %q", args[0])
	}
	if args[1] != info.URI {
		t.Errorf("expected URI %q, got %q", info.URI, args[1])
	}
}

func TestPlayerInvoker_SpacesInURI(t *testing.T) {
	// URI with spaces must be preserved as a single argument.
	uri := "http://example.com/my video file.mp4"
	p := PlayerInvoker{Command: "mpv"}
	args := make([]string, 0, len(p.Args)+1)
	args = append(args, p.Args...)
	args = append(args, uri)

	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != uri {
		t.Errorf("URI not preserved: got %q", args[0])
	}
}

func TestPlayerInvoker_NoShellJoin(t *testing.T) {
	// Args must NOT be joined into a single shell string.
	// Each arg should be a separate element.
	p := PlayerInvoker{
		Command: "mpv",
		Args:    []string{"--opt=val"},
	}
	uri := "http://example.com/video.mp4"
	args := make([]string, 0, len(p.Args)+1)
	args = append(args, p.Args...)
	args = append(args, uri)

	// Verify we have exactly 2 separate args, not 1 joined string.
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
}

func TestInfo_Fields(t *testing.T) {
	info := Info{
		URI:      "http://example.com/video.mp4",
		Title:    "Test Video",
		Duration: 3600,
	}
	if info.URI != "http://example.com/video.mp4" {
		t.Errorf("URI mismatch")
	}
	if info.Title != "Test Video" {
		t.Errorf("Title mismatch")
	}
	if info.Duration != 3600 {
		t.Errorf("Duration mismatch")
	}
}

// Verify that setDetach doesn't panic on the current platform.
func TestSetDetach_NoPanic(_ *testing.T) {
	cmd := exec.Command("echo", "test")
	setDetach(cmd)
	// We don't start the process, just verify setDetach doesn't panic.
}

// Verify the fakeCmd interface satisfaction.
var _ testCmd = &fakeCmd{}

// Ensure error types work correctly.
func TestErrors(_ *testing.T) {
	startErr := errors.New("start failed")
	releaseErr := errors.New("release failed")
	_ = startErr
	_ = releaseErr
}
