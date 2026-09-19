// Package main provides the convergenci CLI entrypoint.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/iilei/convergenci-cli/internal/cli"
)

// These variables are replaced by -ldflags during the build.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
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
	if err := cli.NewRootCommand(GetVersion()).Execute(); err != nil {
		var exitErr *cli.ExitCodeError
		if errors.As(err, &exitErr) && exitErr.Code > 0 {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
