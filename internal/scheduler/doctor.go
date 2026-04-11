package scheduler

import (
	"fmt"
	"os/exec"
)

// RunnerBinaries maps each runner name to the CLI binary it requires.
var RunnerBinaries = map[string]string{
	"claude_code": "claude",
	"codex":       "codex",
	"gemini":      "gemini",
	"copilot":     "gh",
	"opencode":    "opencode",
}

// BinaryStatus holds the result of checking a single runner binary.
type BinaryStatus struct {
	Runner string
	Binary string
	Found  bool
	Path   string // resolved path when found
}

// CheckBinaries verifies that the CLI binary for each known runner is
// available on PATH via exec.LookPath. It always checks git as well,
// since every runner depends on it for diff capture.
func CheckBinaries() []BinaryStatus {
	var results []BinaryStatus

	// Always check git first — all runners depend on it.
	gitPath, err := exec.LookPath("git")
	results = append(results, BinaryStatus{
		Runner: "*",
		Binary: "git",
		Found:  err == nil,
		Path:   gitPath,
	})

	for runner, binary := range RunnerBinaries {
		path, err := exec.LookPath(binary)
		results = append(results, BinaryStatus{
			Runner: runner,
			Binary: binary,
			Found:  err == nil,
			Path:   path,
		})
	}

	return results
}

// CheckRunnerBinary returns a non-nil error if the CLI binary required
// by the given runner is not found on PATH.
func CheckRunnerBinary(runner string) error {
	binary, ok := RunnerBinaries[runner]
	if !ok {
		return nil // unknown runner — let the switch-default handle it
	}
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf("runner %q requires the %s CLI but it was not found in PATH", runner, binary)
	}
	return nil
}
