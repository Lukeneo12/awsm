// Package clipboard reads the system clipboard cross-platform by shelling out
// to the native tool (pbpaste on macOS; wl-paste/xclip/xsel on Linux). All
// execution goes through runner.CommandRunner so it is unit-testable.
package clipboard

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/Lukeneo12/awsm/internal/runner"
)

// candidate is a clipboard-read command: the binary plus its args.
type candidate struct {
	bin  string
	args []string
}

// candidatesFor returns the ordered list of clipboard tools to try for an OS.
func candidatesFor(goos string) []candidate {
	switch goos {
	case "darwin":
		return []candidate{{"pbpaste", nil}}
	case "linux":
		return []candidate{
			{"wl-paste", []string{"--no-newline"}},
			{"xclip", []string{"-selection", "clipboard", "-o"}},
			{"xsel", []string{"--clipboard", "--output"}},
		}
	case "windows":
		return []candidate{{"powershell", []string{"-NoProfile", "-Command", "Get-Clipboard"}}}
	default:
		return nil
	}
}

// Read returns the clipboard contents using the first available native tool.
func Read(r runner.CommandRunner) (string, error) {
	return read(r, runtime.GOOS)
}

func read(r runner.CommandRunner, goos string) (string, error) {
	cands := candidatesFor(goos)
	if len(cands) == 0 {
		return "", fmt.Errorf("clipboard not supported on %s", goos)
	}
	var tried []string
	for _, c := range cands {
		tried = append(tried, c.bin)
		if _, err := r.LookPath(c.bin); err != nil {
			continue
		}
		res := r.Run(context.Background(), c.bin, c.args...)
		if res.Err != nil {
			return "", fmt.Errorf("reading clipboard with %s: %w", c.bin, res.Err)
		}
		if res.ExitCode != 0 {
			return "", fmt.Errorf("%s failed: %s", c.bin, strings.TrimSpace(string(res.Stderr)))
		}
		return string(res.Stdout), nil
	}
	return "", fmt.Errorf("no clipboard tool found (install one of: %s)", strings.Join(tried, ", "))
}
