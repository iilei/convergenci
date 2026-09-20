package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iilei/convergenci-cli/internal/awscmd"
	"github.com/iilei/convergenci-cli/internal/scan"
)

func defaultScanOutputPath(planPath string, jsonlines bool) string {
	if planPath == "" {
		if jsonlines {
			return ".convergence.jsonlines"
		}
		return ".convergence.json"
	}
	base := strings.TrimSuffix(filepath.Base(planPath), filepath.Ext(planPath))
	if base == "" {
		base = "plan"
	}
	if jsonlines {
		return base + ".convergence.jsonlines"
	}
	return base + ".convergence.json"
}

func (c *Command) executeScan(args []string) error {
	fs := flag.NewFlagSet("convergenci scan", flag.ContinueOnError)
	fs.SetOutput(c.out)

	outputJSON := fs.String("output-json", "", "write the generated convergence contract to a JSON file")
	jsonlines := fs.Bool("jsonlines", false, "write the generated convergence contract as JSON Lines")
	force := fs.Bool("force", false, "overwrite an existing output file")
	assertAllSettled := fs.Bool(
		"assert-all-settled",
		false,
		"assert that no relevant AWS operation is currently in progress for the plan's resources, instead of emitting a contract",
	)
	awsCLIPath, awsProfileName := awscmd.RegisterFlags(fs)
	fs.Usage = func() {
		fmt.Fprintf(c.out, "Usage: convergenci scan <tfplan.json>\n")
		fmt.Fprintf(c.out, "       convergenci scan --assert-all-settled <tfplan.json>\n\n")
		fmt.Fprintf(c.out, "Scan a Terraform plan and emit a convergence contract.\n\n")
		fmt.Fprintf(c.out, "Flags:\n")
		fmt.Fprintf(c.out, "  --output-json PATH    Write the generated convergence contract to a JSON file at PATH\n")
		fmt.Fprintf(c.out, "  --jsonlines           Write the generated convergence contract as JSON Lines\n")
		fmt.Fprintf(c.out, "  --force              Overwrite an existing output file\n")
		fmt.Fprintf(
			c.out,
			"  --assert-all-settled Assert nothing relevant is currently converging for the plan's resources\n",
		)
		fmt.Fprint(c.out, awscmd.UsageText())
		fmt.Fprintf(c.out, "  -h, --help            Show help\n")
	}

	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fs.Usage()
		return nil
	}
	positionals, err := parseFlagsWithPositionals(fs, args)
	if err != nil {
		return err
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

	if *assertAllSettled {
		if len(positionals) == 0 {
			return &ExitCodeError{Code: codeConfigError, Message: "a terraform plan path is required"}
		}
		return c.executeScanAssertAllSettled(positionals[0], cmdCfg)
	}

	path := *outputJSON
	if path == "" {
		if len(positionals) > 0 {
			path = defaultScanOutputPath(positionals[0], *jsonlines)
		} else {
			path = defaultScanOutputPath("", *jsonlines)
		}
	}
	fmt.Fprintf(c.out, "output-json: %s\n", path)
	if *jsonlines {
		fmt.Fprintln(c.out, "jsonlines: true")
	}

	if len(positionals) == 0 {
		return nil
	}

	planPath := positionals[0]
	if len(positionals) > 1 {
		fmt.Fprintf(c.errOut, "terraform plan input: %s\n", positionals[0])
	}
	plan, err := scan.LoadPlan(planPath)
	if err != nil {
		return err
	}
	contract, err := scan.BuildContract(plan, scan.ASGDefaultPolicy, "", nil)
	if err != nil {
		return err
	}
	if err := writeScanArtifact(path, contract, *jsonlines, *force); err != nil {
		return err
	}
	return nil
}

// executeScanAssertAllSettled resolves AWS resource names from the plan using the same
// matching rules as BuildContract, then asserts none of them are currently converging.
func (c *Command) executeScanAssertAllSettled(planPath string, cmdCfg awscmd.Config) error {
	plan, err := scan.LoadPlan(planPath)
	if err != nil {
		return &ExitCodeError{Code: codeIOError, Message: fmt.Sprintf("unable to read terraform plan: %v", err)}
	}
	names, err := scan.MatchedResourceNames(plan, scan.ASGDefaultPolicy, "")
	if err != nil {
		return &ExitCodeError{Code: codeConfigError, Message: err.Error()}
	}
	if len(names) == 0 {
		fmt.Fprintln(c.out, "no matching resources in plan")
		return nil
	}

	var notSettled []string
	for _, name := range names {
		inProgress, err := asgInstanceRefreshInProgress(name, cmdCfg)
		if err != nil {
			return &ExitCodeError{Code: codeGenericFailure, Message: err.Error()}
		}
		if inProgress {
			notSettled = append(notSettled, name)
		}
	}
	if len(notSettled) > 0 {
		return &ExitCodeError{
			Code:    codeGenericFailure,
			Message: fmt.Sprintf("not settled: %s", strings.Join(notSettled, ", ")),
		}
	}

	fmt.Fprintln(c.out, "all resources settled")
	return nil
}

func safeOutputFilePath(path string, force bool) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("output path is empty")
	}
	cleaned := filepath.Clean(path)
	if cleaned == "." || cleaned == string(filepath.Separator) {
		return "", fmt.Errorf("invalid output path: %q", path)
	}
	if filepath.IsAbs(cleaned) {
		if !force {
			if _, err := os.Stat(cleaned); err == nil {
				return "", fmt.Errorf("refusing to overwrite existing file: %s", cleaned)
			} else if !os.IsNotExist(err) {
				return "", err
			}
		}
		return cleaned, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(cleaned)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(wd, absPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing to write outside the working directory: %q", path)
	}
	if !force {
		if _, err := os.Stat(absPath); err == nil {
			return "", fmt.Errorf("refusing to overwrite existing file: %s", absPath)
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	return cleaned, nil
}

func writeScanArtifact(path string, contract scan.Contract, jsonlines, force bool) error {
	var data []byte
	var err error
	if jsonlines {
		for _, item := range contract.Resources {
			entry, err := json.Marshal(item)
			if err != nil {
				return err
			}
			data = append(data, entry...)
			data = append(data, '\n')
		}
	} else {
		data, err = json.MarshalIndent(contract, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
	}

	path, err = safeOutputFilePath(path, force)
	if err != nil {
		return err
	}
	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return err
		}
	}
	if dir := filepath.Dir(resolved); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	return os.WriteFile(resolved, data, 0o644)
}
