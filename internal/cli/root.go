// Package cli defines the convergenci command-line interface.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// Version contains build metadata for the CLI.
type Version struct {
	Version string
	Commit  string
	Date    string
}

// Command is the root CLI command.
type Command struct {
	version Version
	out     io.Writer
	errOut  io.Writer
	fs      *flag.FlagSet
}

// NewRootCommand returns a root command configured with build metadata.
func NewRootCommand(v Version) *Command {
	c := &Command{
		version: v,
		out:     os.Stdout,
		errOut:  os.Stderr,
	}
	c.initFlagSet()
	return c
}

func (c *Command) initFlagSet() {
	debugDefault := debugFromEnv()

	fs := flag.NewFlagSet("convergenci", flag.ContinueOnError)
	fs.SetOutput(c.errOut)
	fs.Bool("version", false, "show version information and exit")
	fs.Bool("v", false, "show version information and exit")
	fs.Bool("debug", debugDefault, "enable debug logging")
	fs.Usage = func() {
		fmt.Fprintf(c.out, "Usage: convergenci [flags] [command]\n\n")
		fmt.Fprintf(c.out, "Commands:\n")
		fmt.Fprintf(c.out, "  version        Show version information\n")
		fmt.Fprintf(c.out, "  scan           Scan a Terraform plan and emit a convergence contract\n")
		fmt.Fprintf(c.out, "  await          Wait for the runtime state in a convergence contract to converge\n")
		fmt.Fprintf(c.out, "  doctor         Check AWS CLI availability and render a human-friendly report\n\n")
		fmt.Fprintf(c.out, "Examples:\n")
		fmt.Fprintf(c.out, "  convergenci scan <tfplan.json>\n")
		fmt.Fprintf(c.out, "  convergenci --assert-all-settled <asg-name> [asg-name...]\n")
		fmt.Fprintf(c.out, "  convergenci await <convergence.json>\n")
		fmt.Fprintf(c.out, "  convergenci doctor ./artifacts/report.json\n\n")
		fmt.Fprintf(c.out, "Flags:\n")
		fmt.Fprintf(c.out, "  -v, --version  Show version information and exit\n")
		fmt.Fprintf(c.out, "  --debug       Enable debug logging\n")
		fmt.Fprintf(c.out, "  --assert-all-settled  Assert nothing relevant is currently converging (takes resource names)\n")
		fmt.Fprintf(c.out, "  -h, --help     Show help\n")
	}
	c.fs = fs
}

// SetOut sets the writer used for normal command output.
func (c *Command) SetOut(w io.Writer) {
	c.out = w
	if c.fs != nil {
		c.fs.SetOutput(c.errOut)
	}
}

// SetErr sets the writer used for errors and usage text.
func (c *Command) SetErr(w io.Writer) {
	c.errOut = w
	if c.fs != nil {
		c.fs.SetOutput(c.errOut)
	}
}

// Execute executes the root command with os.Args[1:].
func (c *Command) Execute() error {
	return c.ExecuteArgs(os.Args[1:])
}

// ExecuteArgs executes the root command with the provided arguments.
func parseFlagsWithPositionals(fs *flag.FlagSet, args []string) ([]string, error) {
	flagArgs := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				name := strings.TrimLeft(arg, "-")
				if name == "" {
					continue
				}
				if f := fs.Lookup(name); f != nil && f.Value.String() == "false" && f.DefValue == "false" {
					continue
				}
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
			continue
		}
		positionals = append(positionals, arg)
	}

	if len(flagArgs) > 0 {
		if err := fs.Parse(flagArgs); err != nil {
			return positionals, err
		}
	}
	return positionals, nil
}

func hasRootHelpFlag(args []string) bool {
	return len(args) > 0 && (args[0] == "-h" || args[0] == "--help")
}

func (c *Command) ExecuteArgs(args []string) error {
	if c.fs == nil {
		c.initFlagSet()
	}

	if len(args) == 0 {
		c.fs.Usage()
		return nil
	}

	if hasRootHelpFlag(args) {
		c.fs.Usage()
		return nil
	}

	switch args[0] {
	case "version":
		c.printVersion()
		return nil
	case "scan":
		return c.executeScan(args[1:])
	case "await":
		return c.executeAwait(args[1:])
	case "doctor":
		return c.executeDoctor(args[1:])
	case "--assert-all-settled":
		return c.executeAssertAllSettled(args[1:])
	}

	if err := c.fs.Parse(args); err != nil {
		return err
	}

	if c.fs.Lookup("debug") != nil {
		ConfigureLogger(c.fs.Lookup("debug").Value.String() == "true")
	}

	if c.fs.Lookup("version").Value.String() == "true" || c.fs.Lookup("v").Value.String() == "true" {
		c.printVersion()
		return nil
	}

	if c.fs.NArg() > 0 {
		fmt.Fprintf(c.errOut, "unknown command: %s\n", c.fs.Arg(0))
		c.fs.Usage()
		return nil
	}

	c.fs.Usage()
	return nil
}

func (c *Command) printVersion() {
	if c.version.Version == "" {
		fmt.Fprintln(c.out, "dev")
		return
	}
	if c.version.Commit == "" || c.version.Commit == "none" {
		fmt.Fprintf(c.out, "%s\n", c.version.Version)
		return
	}
	fmt.Fprintf(c.out, "%s (commit: %s, built at: %s)\n", c.version.Version, c.version.Commit, c.version.Date)
}
