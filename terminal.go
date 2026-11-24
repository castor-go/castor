package castor

import (
	"fmt"
	"io"
	"os"
)

// IsTTY reports whether stdout is a character device.
var IsTTY bool

func init() {
	info, err := os.Stdout.Stat()
	if err != nil {
		return
	}
	mode := os.ModeDevice | os.ModeCharDevice
	IsTTY = info.Mode()&mode == mode
}

func Write(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
