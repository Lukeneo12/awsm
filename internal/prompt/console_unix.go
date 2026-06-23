//go:build !windows

package prompt

import (
	"io"
	"os"
)

// EOFKey is the keystroke that signals end-of-input when pasting into stdin.
const EOFKey = "Ctrl+D"

// openConsole opens the controlling terminal for interactive prompts.
func openConsole() (r io.Reader, w io.Writer, closeFn func(), err error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, nil, err
	}
	return f, f, func() { _ = f.Close() }, nil
}
