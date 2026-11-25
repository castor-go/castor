package middleware

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	"github.com/castor-go/castor"
)

var recovererErrorWriter io.Writer = os.Stderr

// Recoverer recovers panics, logs them, and responds with 500 when possible.
// http.ErrAbortHandler is re-panicked so the connection is aborted.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			if recovered == http.ErrAbortHandler {
				panic(recovered)
			}

			stack := debug.Stack()
			if entry := castor.GetLog(r); entry != nil {
				entry.Panic(recovered, stack)
			} else {
				printPrettyStackBytes(recovered, stack)
			}
			if r.Header.Get("Connection") != "Upgrade" {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// PrintPrettyStack prints a formatted panic and stack trace to stderr.
func PrintPrettyStack(recovered any) {
	printPrettyStackBytes(recovered, debug.Stack())
}

func printPrettyStackBytes(recovered any, stack []byte) {
	output, err := (prettyStack{}).parse(stack, recovered)
	if err != nil {
		_, _ = fmt.Fprintf(recovererErrorWriter, "panic: %v\n\n%s", recovered, stack)
		return
	}
	_, _ = recovererErrorWriter.Write(output)
}

type prettyStack struct{}

func (s prettyStack) parse(stack []byte, recovered any) ([]byte, error) {
	buf := &bytes.Buffer{}
	castor.Write(buf, "\n")
	castor.Write(buf, " panic: ")
	castor.Write(buf, "%v", recovered)
	castor.Write(buf, "\n \n")

	lines := strings.Split(string(stack), "\n")
	start := 0
	for i := len(lines) - 1; i > 0; i-- {
		if strings.HasPrefix(lines[i], "panic(") {
			start = min(i+2, len(lines))
			break
		}
	}

	var err error
	for i, line := range lines[start:] {
		line, err = s.decorateLine(line, i)
		if err != nil {
			return nil, err
		}
		buf.WriteString(line)
	}
	return buf.Bytes(), nil
}

func (s prettyStack) decorateLine(line string, num int) (string, error) {
	line = strings.TrimSpace(line)
	switch {
	case strings.Contains(line, ".go:"):
		return s.decorateSourceLine(line, num)
	case strings.HasSuffix(line, ")"):
		return s.decorateFuncCallLine(line, num)
	default:
		return fmt.Sprintf("    %s\n", line), nil
	}
}

func (s prettyStack) decorateFuncCallLine(line string, num int) (string, error) {
	idx := strings.LastIndex(line, "(")
	if idx < 0 {
		return "", errors.New("middleware: not a function call line")
	}

	buf := &bytes.Buffer{}
	pkg := line[:idx]
	method := ""

	if idx := strings.LastIndex(pkg, string(os.PathSeparator)); idx < 0 {
		if idx := strings.Index(pkg, "."); idx > 0 {
			method = pkg[idx:]
			pkg = pkg[:idx]
		}
	} else {
		method = pkg[idx+1:]
		pkg = pkg[:idx+1]
		if idx := strings.Index(method, "."); idx > 0 {
			pkg += method[:idx]
			method = method[idx:]
		}
	}

	if num == 0 {
		castor.Write(buf, " -> ")
	} else {
		castor.Write(buf, "    ")
	}
	castor.Write(buf, "pkg=%s", pkg)
	castor.Write(buf, "method=%s\n", method)
	return buf.String(), nil
}

func (s prettyStack) decorateSourceLine(line string, num int) (string, error) {
	idx := strings.LastIndex(line, ".go:")
	if idx < 0 {
		return "", errors.New("middleware: not a source line")
	}

	buf := &bytes.Buffer{}
	path := line[:idx+3]
	lineNumber := line[idx+3:]

	idx = strings.LastIndex(path, string(os.PathSeparator))
	if idx < 0 {
		return "", errors.New("middleware: source line has no directory")
	}
	dir := path[:idx+1]
	file := path[idx+1:]

	if idx = strings.Index(lineNumber, " "); idx > 0 {
		lineNumber = lineNumber[:idx]
	}

	if num == 1 {
		castor.Write(buf, " ->   ")
	} else {
		castor.Write(buf, "      ")
	}
	castor.Write(buf, "%s", dir)
	castor.Write(buf, "file=%s", file)
	castor.Write(buf, "line=%s", lineNumber)
	castor.Write(buf, "\n")
	return buf.String(), nil
}
