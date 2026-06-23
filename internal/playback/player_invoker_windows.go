package playback

import "os/exec"

// setDetach is a no-op on Windows. The Windows implementation uses
// CREATE_NEW_PROCESS_GROUP in the build-tagged file.
func setDetach(cmd *exec.Cmd) {
	// On Windows, process detachment is handled via creation flags
	// in the STARTUPINFO. This no-op satisfies the compiler when
	// the unix file is excluded.
}
