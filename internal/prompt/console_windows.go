//go:build windows

package prompt

import (
	"io"
	"os"
)

// EOFKey is the keystroke that signals end-of-input when pasting into stdin.
const EOFKey = "Ctrl+Z then Enter"

// openConsole opens the Windows console (CONIN$/CONOUT$), which reach the real
// console even when stdin/stdout are redirected.
func openConsole() (r io.Reader, w io.Writer, closeFn func(), err error) {
	in, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, nil, err
	}
	out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		_ = in.Close()
		return nil, nil, nil, err
	}
	return in, out, func() { _ = in.Close(); _ = out.Close() }, nil
}
