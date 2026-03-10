package shimmer

import (
	"os"
	"os/exec"
)

// Git runs a git command against the overlay clone, with stdin/stdout/stderr
// connected to the parent process.
func (s *Shimmer) Git(args []string) error {
	clonePath, err := s.findClone()
	if err != nil {
		return err
	}

	fullArgs := append([]string{"-C", clonePath}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
