package secret

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Source struct {
	File  string
	Stdin bool
	Exec  string
}

func (s Source) IsZero() bool {
	return s.File == "" && !s.Stdin && s.Exec == ""
}

func Resolve(s Source) (string, error) {
	switch {
	case s.File != "":
		data, err := os.ReadFile(s.File)
		if err != nil {
			return "", fmt.Errorf("read secret file %s: %w", s.File, err)
		}
		return strings.TrimSpace(string(data)), nil

	case s.Stdin:
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read secret from stdin: %w", err)
		}
		return strings.TrimSpace(string(data)), nil

	case s.Exec != "":
		parts := strings.Fields(s.Exec)
		if len(parts) == 0 {
			return "", fmt.Errorf("invalid exec command: %q", s.Exec)
		}
		cmd := exec.Command(parts[0], parts[1:]...)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("execute secret command %q: %w", s.Exec, err)
		}
		return strings.TrimSpace(stdout.String()), nil

	default:
		return "", fmt.Errorf("no secret source configured: use --keyring-password-file, --keyring-password-stdin, or --keyring-password-exec")
	}
}
