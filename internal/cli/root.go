// Package cli defines the convergenci command-line interface.
package cli

import (
	"flag"
	"io"
	"os"
	"strings"
)

const (
	generatedDirectoryMode = 0o750
	generatedFileMode      = 0o600
	defaultAWSExecutable   = "aws"
	shortHelpFlag          = "-h"
	longHelpFlag           = "--help"
	boolStringTrue         = "true"
	boolStringFalse        = "false"
	statusConverged        = "converged"
	statusFailed           = "failed"
	statusPending          = "pending"
	statusTimeout          = "timeout"
	awsStatusInProgress    = "INPROGRESS"
	debugFieldEvent        = "Event"
	debugFieldIteration    = "RetryIteration"
	debugFieldLimit        = "RetryLimit"
	debugFieldElapsed      = "Elapsed"
	debugFieldStatus       = "Status"
	debugFieldSource       = "Source"
	debugFieldResource     = "Resource"
)

// Version contains build metadata for the CLI.
type Version struct {
	Version string
	Commit  string
	Date    string
}

// Command is the root CLI command.
type Command struct {
	out     io.Writer
	errOut  io.Writer
	fs      *flag.FlagSet
	version Version
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

func parseFlagsWithPositionals(fs *flag.FlagSet, args []string) ([]string, error) {
	flagArgs := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for len(args) > 0 {
		arg := args[0]
		args = args[1:]
		if arg == "--" {
			positionals = append(positionals, args...)
			break
		}
		if !strings.HasPrefix(arg, "-") {
			positionals = append(positionals, arg)
			continue
		}
		flagArgs = append(flagArgs, arg)
		if len(args) == 0 {
			continue
		}
		nextArg := args[0]
		if strings.HasPrefix(nextArg, "-") {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if name == "" {
			continue
		}
		if f := fs.Lookup(name); f != nil && f.Value.String() == boolStringFalse && f.DefValue == boolStringFalse {
			continue
		}
		flagArgs = append(flagArgs, nextArg)
		args = args[1:]
	}

	if len(flagArgs) > 0 {
		if err := fs.Parse(flagArgs); err != nil {
			return positionals, err
		}
	}
	return positionals, nil
}

func hasRootHelpFlag(args []string) bool {
	return len(args) > 0 && (args[0] == shortHelpFlag || args[0] == longHelpFlag)
}

// ExecuteArgs executes the root command with the provided arguments.
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
	case "report":
		return c.executeReport(args[1:])
	case "doctor":
		return c.executeDoctor(args[1:])
	}

	if err := c.fs.Parse(args); err != nil {
		return err
	}

	if c.fs.Lookup("debug") != nil {
		ConfigureLogger(c.fs.Lookup("debug").Value.String() == boolStringTrue)
	}

	if c.fs.Lookup("version").Value.String() == boolStringTrue || c.fs.Lookup("v").Value.String() == boolStringTrue {
		c.printVersion()
		return nil
	}

	if c.fs.NArg() > 0 {
		writeBestEffortf(c.errOut, "unknown command: %s\n", c.fs.Arg(0))
		c.fs.Usage()
		return nil
	}

	c.fs.Usage()
	return nil
}

func (c *Command) initFlagSet() {
	debugDefault := debugFromEnv()

	fs := flag.NewFlagSet("convergenci", flag.ContinueOnError)
	fs.SetOutput(c.errOut)
	fs.Bool("version", false, "show version information and exit")
	fs.Bool("v", false, "show version information and exit")
	fs.Bool("debug", debugDefault, "enable debug logging")
	fs.Usage = func() {
		writeBestEffortf(c.out, "Usage: convergenci [flags] [command]\n\n")
		writeBestEffortf(c.out, "Commands:\n")
		writeBestEffortf(c.out, "  version        Show version information\n")
		writeBestEffortf(c.out, "  scan           Scan a Terraform plan and emit a convergence contract\n")
		writeBestEffortf(c.out, "  await          Wait for the runtime state in a convergence contract to converge\n")
		writeBestEffortf(c.out, "  report         Render a convergence report file as human-readable text\n")
		writeBestEffortf(c.out, "  doctor         Check AWS CLI availability and render a human-friendly report\n\n")
		writeBestEffortf(c.out, "Examples:\n")
		writeBestEffortf(c.out, "  convergenci scan <tfplan.json>\n")
		writeBestEffortf(c.out, "  convergenci scan --assert-all-settled <tfplan.json>\n")
		writeBestEffortf(c.out, "  convergenci await <convergence.json>\n")
		writeBestEffortf(c.out, "  convergenci report <convergence-report.json>\n")
		writeBestEffortf(c.out, "  convergenci doctor ./artifacts/report.json\n\n")
		writeBestEffortf(c.out, "Flags:\n")
		writeBestEffortf(c.out, "  -v, --version  Show version information and exit\n")
		writeBestEffortf(c.out, "  --debug       Enable debug logging\n")
		writeBestEffortf(c.out, "  -h, --help     Show help\n")
	}
	c.fs = fs
}

func (c *Command) printVersion() {
	if c.version.Version == "" {
		writeBestEffortln(c.out, "dev")
		return
	}
	if c.version.Commit == "" || c.version.Commit == "none" {
		writeBestEffortf(c.out, "%s\n", c.version.Version)
		return
	}
	writeBestEffortf(c.out, "%s (commit: %s, built at: %s)\n", c.version.Version, c.version.Commit, c.version.Date)
}
