// Package prompt asks the user a yes/no question on the console, independently
// of stdin. The block being confirmed is typically read from stdin until EOF,
// so the confirmation must come from a separate channel — the controlling
// terminal (/dev/tty on Unix, CONIN$/CONOUT$ on Windows). All console access
// lives behind a build-tagged seam (openConsole) so the decision logic stays
// unit-testable.
package prompt

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// ErrNoTTY is returned by Confirm when no console can be opened (e.g. a headless
// CI run). Callers decide the non-interactive policy.
var ErrNoTTY = errors.New("no console available")

// Confirm asks question on the console and reports whether the user agreed.
// It returns ErrNoTTY when the console cannot be opened.
func Confirm(question string) (bool, error) {
	r, w, closeFn, err := openConsole()
	if err != nil {
		return false, ErrNoTTY
	}
	defer closeFn()
	return confirm(r, w, question)
}

// confirm writes the prompt to w and reads one line from r, returning true only
// for an explicit yes (y/yes, case-insensitive). An empty line (just Enter) or
// anything else is false. It holds the testable logic, free of console I/O.
func confirm(r io.Reader, w io.Writer, question string) (bool, error) {
	if _, err := io.WriteString(w, question+" [y/N]: "); err != nil {
		return false, err
	}
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
