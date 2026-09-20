package cli

import (
	"fmt"
	"io"
)

func writeBestEffort(writer io.Writer, values ...any) {
	_, _ = fmt.Fprint(writer, values...)
}

func writeBestEffortf(writer io.Writer, format string, values ...any) {
	_, _ = fmt.Fprintf(writer, format, values...)
}

func writeBestEffortln(writer io.Writer, values ...any) {
	_, _ = fmt.Fprintln(writer, values...)
}
