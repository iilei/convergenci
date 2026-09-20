package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/iilei/convergenci-cli/internal/awscmd"
)

func (c *Command) executeDoctor(args []string) error {
	fs := flag.NewFlagSet("convergenci doctor", flag.ContinueOnError)
	fs.SetOutput(c.out)

	templatePath := fs.String(
		"template",
		filepath.FromSlash("templates/report-as-text.tmpl"),
		"path to a text template used to render a report file",
	)
	awsCLIPath, awsProfileName := awscmd.RegisterFlags(fs)
	fs.Usage = func() {
		writeBestEffortf(c.out, "Usage: convergenci doctor [--template PATH] [report.json]\n\n")
		writeBestEffortf(c.out, "Run a small AWS CLI sanity check and render a human-friendly report summary.\n\n")
		writeBestEffortf(c.out, "Flags:\n")
		writeBestEffortf(c.out, "  --template PATH      Template used to render a report file\n")
		writeBestEffort(c.out, awscmd.UsageText())
		writeBestEffortf(c.out, "  -h, --help           Show help\n")
	}

	if len(args) > 0 && (args[0] == shortHelpFlag || args[0] == longHelpFlag) {
		fs.Usage()
		return nil
	}
	positionals, err := parseFlagsWithPositionals(fs, args)
	if err != nil {
		return err
	}

	cfg := awscmd.DefaultConfig()
	if *awsCLIPath != defaultAWSExecutable {
		cfg.BinaryPath = *awsCLIPath
		writeBestEffortf(c.out, "aws-cli-path: %s\n", *awsCLIPath)
	}
	if *awsProfileName != "" {
		cfg.Profile = *awsProfileName
		writeBestEffortf(c.out, "aws-profile-name: %s\n", *awsProfileName)
	}
	if err := checkAWSCLI(cfg); err != nil {
		return err
	}

	if len(positionals) == 0 {
		writeBestEffortln(c.out, "doctor: AWS CLI is available")
		return nil
	}

	if *templatePath == "" {
		return errors.New("template path is empty")
	}

	reportPath := positionals[0]
	// #nosec G304 G703 -- reportPath is the report file explicitly selected by the CLI user.
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("read report: %w", err)
	}

	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("decode report: %w", err)
	}

	tpl, err := template.ParseFiles(*templatePath)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}
	if err := tpl.Execute(c.out, report); err != nil {
		return fmt.Errorf("render report: %w", err)
	}
	_, _ = fmt.Fprintln(c.out)
	return nil
}

func checkAWSCLI(cfg awscmd.Config) error {
	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		binaryPath = defaultAWSExecutable
	}
	if _, err := exec.LookPath(binaryPath); err != nil {
		return fmt.Errorf("AWS CLI not found: %w", err)
	}

	// #nosec G204 -- binaryPath is the explicit AWS CLI path accepted by this diagnostic command.
	versionCmd := exec.CommandContext(context.Background(), binaryPath, "--version")
	versionOut, err := versionCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("AWS CLI check failed: %w: %s", err, strings.TrimSpace(string(versionOut)))
	}
	writeBestEffortf(os.Stdout, "aws version: %s\n", strings.TrimSpace(string(versionOut)))

	// #nosec G204 -- binaryPath is the explicit AWS CLI path accepted by this diagnostic command.
	callerCmd := exec.CommandContext(context.Background(), binaryPath, "sts", "get-caller-identity")
	if cfg.Profile != "" {
		callerCmd.Args = append(
			callerCmd.Args[:2],
			append([]string{"--profile", cfg.Profile}, callerCmd.Args[2:]...)...,
		)
	}
	callerOut, err := callerCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("AWS caller identity check failed: %w: %s", err, strings.TrimSpace(string(callerOut)))
	}
	writeBestEffortf(os.Stdout, "aws caller identity: %s\n", strings.TrimSpace(string(callerOut)))
	return nil
}
