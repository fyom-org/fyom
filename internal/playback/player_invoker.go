// Package playback handles external media player invocation.
package playback

import (
	"context"
	"fmt"
	"os/exec"
)

// Info holds the resolved URI and optional metadata needed to launch
// an external player.
type Info struct {
	URI      string
	Title    string
	Duration int // seconds, informational only
}

// PlayerInvoker launches an external media player with a resolved playback
// URI and immediately detaches. It does not monitor progress, open IPC
// sockets, or manage the external process lifecycle after launch.
type PlayerInvoker struct {
	Command string
	Args    []string
}

// Invoke starts the external player with the given playback info and
// immediately releases the process. It is a fire-and-forget operation:
// no goroutines are spawned, no progress is tracked, no IPC is opened.
func (p PlayerInvoker) Invoke(_ context.Context, info Info) error {
	if p.Command == "" {
		return fmt.Errorf("player command is empty")
	}
	if info.URI == "" {
		return fmt.Errorf("playback URI is empty")
	}

	// Build args: configured args first, then the URI as the final argument.
	args := make([]string, 0, len(p.Args)+1)
	args = append(args, p.Args...)
	args = append(args, info.URI)

	// Use exec.Command so the process is not killed when the context is
	// cancelled. We want true fire-and-forget semantics: once the process
	// is started and released, it outlives this context.
	cmd := exec.Command(p.Command, args...)

	// Detach the process from the parent so it is not killed when the
	// parent exits. Platform-specific setup is handled in build-tagged
	// files.
	setDetach(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start player %q: %w", p.Command, err)
	}

	// Release the process so we do not retain a reference and the child
	// can continue running independently.
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release player process: %w", err)
	}

	return nil
}
