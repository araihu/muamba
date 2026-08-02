package cli

import (
	"fmt"
	"io"
)

const helpText = "Usage: muamba <command>\n\n" +
	"Commands:\n" +
	"  lock\n" +
	"  sync\n" +
	"  verify\n" +
	"  update\n" +
	"  generate-go\n" +
	"  help\n"

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	if len(args) == 0 {
		_, _ = fmt.Fprint(stdout, helpText)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		_, _ = fmt.Fprint(stdout, helpText)
		return 0
	case "lock", "sync", "verify", "update", "generate-go":
		_, _ = fmt.Fprintf(stderr, "%s: not implemented\n", args[0])
		return 1
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		_, _ = fmt.Fprint(stderr, helpText)
		return 2
	}
}
