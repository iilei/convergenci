// Package main provides the convergenci CLI entrypoint.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/iilei/convergenci-cli/internal/cli"
)

// These variables are replaced by -ldflags during the build.
var (
	version     = "dev"
	commit      = "none"
	date        = "unknown"
	exitProcess = os.Exit
)

// GetVersion returns the build metadata injected by GoReleaser.
func GetVersion() cli.Version {
	return cli.Version{
		Version: version,
		Commit:  commit,
		Date:    date,
	}
}

func main() {
	exitProcess(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out, errOut io.Writer) int {
	command := cli.NewRootCommand(GetVersion())
	command.SetOut(out)
	command.SetErr(errOut)
	if err := command.ExecuteArgs(args); err != nil {
		var exitErr *cli.ExitCodeError
		if errors.As(err, &exitErr) && exitErr.Code > 0 {
			fmt.Fprintln(errOut, err)
			return exitErr.Code
		}
		fmt.Fprintln(errOut, err)
		return 1
	}
	return 0
}
