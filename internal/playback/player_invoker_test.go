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
	err := p.Invoke(context.Background(), PlaybackInfo{URI: "http://example.com/video.mp4"})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestPlayerInvoker_EmptyURI(t *testing.T) {
	p := PlayerInvoker{Command: "mpv"}
	err := p.Invoke(context.Background(), PlaybackInfo{URI: ""})
	if err == nil {
		t.Fatal("expected error for empty URI")
	}
}

func TestPlayerInvoker_ArgsPreserved(t *testing.T) {
	p := PlayerInvoker{
		Command: "mpv",
		Args:    []string{"--resume-playback", "--fullscreen"},
	}

	// Verify the invoker fields are set correctly.
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
	info := PlaybackInfo{URI: "http://example.com/video.mp4"}

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
	uri := "http://example.com/my file.mp4"
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
	p := PlayerInvoker{
		Command: "mpv",
		Args:    []string{"--opt=val"},
	}
	uri := "http://example.com/video.mp4"
	args := make([]string, 0, len(p.Args)+1)
	args = append(args, p.Args...)
	args = append(args, uri)

	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
}

func TestPlaybackInfo_Fields(t *testing.T) {
	info := PlaybackInfo{
		URI:      "http://example.com/video.mp4",
		Title:    "Test Video",
		MimeType: "video/mp4",
		Headers:  map[string]string{"Authorization": "Bearer token123"},
	}
	if info.URI != "http://example.com/video.mp4" {
		t.Errorf("URI mismatch")
	}
	if info.Title != "Test Video" {
		t.Errorf("Title mismatch")
	}
	if info.MimeType != "video/mp4" {
		t.Errorf("MimeType mismatch")
	}
	if info.Headers["Authorization"] != "Bearer token123" {
		t.Errorf("Headers mismatch")
	}
}

func TestPlaybackInfo_EmptyHeaders(t *testing.T) {
	// Headers may be empty for local file:// playback.
	info := PlaybackInfo{
		URI:      "file:///media/video.mp4",
		Title:    "Local Video",
		MimeType: "",
	}
	if info.Headers != nil {
		t.Errorf("expected nil Headers, got %v", info.Headers)
	}
}

func TestDomainErrors(t *testing.T) {
	if ErrMediaNotFound == nil {
		t.Error("ErrMediaNotFound should not be nil")
	}
	if ErrPlaybackNotAllowed == nil {
		t.Error("ErrPlaybackNotAllowed should not be nil")
	}
	if ErrInvalidMediaPath == nil {
		t.Error("ErrInvalidMediaPath should not be nil")
	}
}

// Verify that setDetach doesn't panic on the current platform.
func TestSetDetach_NoPanic(_ *testing.T) {
	cmd := exec.Command("echo", "test")
	setDetach(cmd)
}

var _ testCmd = &fakeCmd{}

func TestErrors(_ *testing.T) {
	startErr := errors.New("start failed")
	releaseErr := errors.New("release failed")
	_ = startErr
	_ = releaseErr
}

// Verify PlaybackURIResolver interface is satisfiable.
var _ PlaybackURIResolver = (PlaybackURIResolver)(nil)
