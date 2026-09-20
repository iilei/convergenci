package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"github.com/iilei/convergenci-cli/internal/awscmd"
)

func (c *Command) executeAssertAllSettled(args []string) error {
	fs := flag.NewFlagSet("convergenci --assert-all-settled", flag.ContinueOnError)
	fs.SetOutput(c.out)

	awsCLIPath, awsProfileName := awscmd.RegisterFlags(fs)
	fs.Usage = func() {
		fmt.Fprintf(c.out, "Usage: convergenci --assert-all-settled <asg-name> [asg-name...]\n\n")
		fmt.Fprintf(c.out, "Assert that nothing relevant is currently converging for the given\n")
		fmt.Fprintf(c.out, "resources, across all resource kinds supported by convergenci (currently\n")
		fmt.Fprintf(c.out, "only ASG instance refreshes). Intended to run right before terraform\n")
		fmt.Fprintf(c.out, "apply, while holding a Terraform state lock, so a subsequent await can\n")
		fmt.Fprintf(c.out, "assume any in-progress operation it observes was caused by that apply.\n\n")
		fmt.Fprintf(c.out, "Flags:\n")
		fmt.Fprint(c.out, awscmd.UsageText())
		fmt.Fprintf(c.out, "  -h, --help            Show help\n")
	}

	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fs.Usage()
		return nil
	}
	positionals, err := parseFlagsWithPositionals(fs, args)
	if err != nil {
		return &ExitCodeError{Code: codeConfigError, Message: err.Error()}
	}

	cmdCfg := awscmd.DefaultConfig()
	if *awsCLIPath != "aws" {
		cmdCfg.BinaryPath = *awsCLIPath
		fmt.Fprintf(c.out, "aws-cli-path: %s\n", *awsCLIPath)
	}
	if *awsProfileName != "" {
		cmdCfg.Profile = *awsProfileName
		fmt.Fprintf(c.out, "aws-profile-name: %s\n", *awsProfileName)
	}
	if DebugEnabled() {
		Debugf("aws command config: binary=%s profile=%q", cmdCfg.BinaryPath, cmdCfg.Profile)
	}

	if len(positionals) == 0 {
		return &ExitCodeError{Code: codeConfigError, Message: "at least one resource name is required"}
	}

	// ASG is the only supported resource kind today; future kinds (e.g. ECS) would be
	// dispatched here as well once convergenci understands their runtime observation.
	var notSettled []string
	for _, name := range positionals {
		inProgress, err := asgInstanceRefreshInProgress(name, cmdCfg)
		if err != nil {
			return &ExitCodeError{Code: codeGenericFailure, Message: err.Error()}
		}
		if inProgress {
			notSettled = append(notSettled, name)
		}
	}
	if len(notSettled) > 0 {
		return &ExitCodeError{Code: codeGenericFailure, Message: fmt.Sprintf("not settled: %s", strings.Join(notSettled, ", "))}
	}

	fmt.Fprintln(c.out, "all resources settled")
	return nil
}

// asgInstanceRefreshInProgress reports whether the named ASG currently has an
// instance refresh with status InProgress or Pending.
func asgInstanceRefreshInProgress(asgName string, cfg awscmd.Config) (bool, error) {
	resp, err := cfg.Run("autoscaling", "describe-instance-refreshes", "--auto-scaling-group-name", asgName)
	if err != nil {
		return false, err
	}
	var payload struct {
		InstanceRefreshes []struct {
			Status string `json:"Status"`
		} `json:"InstanceRefreshes"`
	}
	if err := json.Unmarshal(resp, &payload); err != nil {
		return false, err
	}
	for _, refresh := range payload.InstanceRefreshes {
		switch strings.ToUpper(refresh.Status) {
		case "INPROGRESS", "PENDING":
			return true, nil
		}
	}
	return false, nil
}
