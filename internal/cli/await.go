package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iilei/convergenci-cli/internal/awscmd"
	"github.com/iilei/convergenci-cli/internal/scan"
)

const (
	defaultAwaitTimeout  = 5 * time.Minute
	defaultAwaitInterval = 5 * time.Second

	codeResourceLimitExceeded = 201
	codeConfigError           = 210
	codeIOError               = 220
	codeTimeout               = 230
	codeGenericFailure        = 240
)

// ExitCodeError indicates a process exit code with an optional message.
type ExitCodeError struct {
	Code    int
	Message string
}

func (e *ExitCodeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return fmt.Sprintf("exit code %d", e.Code)
	}
	return fmt.Sprintf("%s (exit code %d)", e.Message, e.Code)
}

type awaitReport struct {
	SchemaVersion      int       `json:"schema_version"`
	Status             string    `json:"status"`
	Message            string    `json:"message,omitempty"`
	ContractPath       string    `json:"contract_path"`
	StartedAt          time.Time `json:"started_at"`
	FinishedAt         time.Time `json:"finished_at"`
	ExpectedResources  int       `json:"expected_resources"`
	PendingResources   int       `json:"pending_resources"`
	ConvergedResources int       `json:"converged_resources"`
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func (c *Command) executeAwait(args []string) error {
	fs := flag.NewFlagSet("convergenci await", flag.ContinueOnError)
	fs.SetOutput(c.out)

	defaultTimeout := envDuration("CONVERGENCI_AWAIT_TIMEOUT", defaultAwaitTimeout)
	defaultInterval := envDuration("CONVERGENCI_AWAIT_INTERVAL", defaultAwaitInterval)
	defaultDebugFormat := strings.TrimSpace(os.Getenv("CONVERGENCI_DEBUG_FORMAT"))
	timeout := fs.Duration("timeout", defaultTimeout, "maximum time to wait before failing")
	interval := fs.Duration("interval", defaultInterval, "polling interval between checks")
	force := fs.Bool("force", false, "overwrite an existing report file")
	debugFormat := fs.String("debug-format", defaultDebugFormat, "template for debug progress output; e.g. '{{ .Status }} {{ .PercentageComplete }}%' ")
	awsCLIPath, awsProfileName := awscmd.RegisterFlags(fs)
	fs.Usage = func() {
		fmt.Fprintf(c.out, "Usage: convergenci await <convergence.json>\n\n")
		fmt.Fprintf(c.out, "Wait for the runtime state in a convergence contract to converge.\n\n")
		fmt.Fprintf(c.out, "Flags:\n")
		fmt.Fprintf(c.out, "  --timeout          Maximum time to wait before failing\n")
		fmt.Fprintf(c.out, "  --interval         Polling interval between checks\n")
		fmt.Fprintf(c.out, "  --force            Overwrite an existing report file\n")
		fmt.Fprintf(c.out, "  --debug-format     Template for debug progress output, e.g. '{{ .Status }} {{ .PercentageComplete }}%%'\n")
		fmt.Fprint(c.out, awscmd.UsageText())
		fmt.Fprintf(c.out, "  -h, --help         Show help\n")
	}

	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fs.Usage()
		return nil
	}
	positionals, err := parseFlagsWithPositionals(fs, args)
	if err != nil {
		return &ExitCodeError{Code: codeConfigError, Message: err.Error()}
	}
	if *debugFormat != "" {
		if err := ConfigureDebugFormat(*debugFormat); err != nil {
			return &ExitCodeError{Code: codeConfigError, Message: fmt.Sprintf("invalid debug-format: %v", err)}
		}
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
		return nil
	}

	path := positionals[0]
	if len(positionals) > 1 {
		fmt.Fprintf(c.errOut, "convergence contract: %s\n", path)
	}
	if *timeout != 0 {
		fmt.Fprintf(c.out, "timeout: %s\n", timeout.String())
	}
	if *interval != 0 {
		fmt.Fprintf(c.out, "interval: %s\n", interval.String())
	}

	contract, err := loadAwaitContract(path)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
			return &ExitCodeError{Code: codeIOError, Message: fmt.Sprintf("convergence contract not found: %s", path)}
		}
		return &ExitCodeError{Code: codeIOError, Message: fmt.Sprintf("unable to read convergence contract: %v", err)}
	}
	if len(contract.Resources) > 200 {
		if err := writeAwaitReport(path, contract, "failed", *force); err != nil {
			return &ExitCodeError{Code: codeIOError, Message: fmt.Sprintf("unable to write summary report: %v", err)}
		}
		return &ExitCodeError{Code: codeResourceLimitExceeded, Message: "expected resources exceeds 200"}
	}

	result, err := pollAwait(contract, *timeout, *interval, cmdCfg)
	if err != nil {
		if reportErr := writeAwaitReport(path, contract, result.Status, *force); reportErr != nil {
			return &ExitCodeError{Code: codeIOError, Message: fmt.Sprintf("unable to write summary report: %v", reportErr)}
		}
		return err
	}
	if result.Status == "converged" {
		if err := writeAwaitReport(path, contract, result.Status, *force); err != nil {
			return &ExitCodeError{Code: codeIOError, Message: fmt.Sprintf("unable to write summary report: %v", err)}
		}
		return nil
	}
	if err := writeAwaitReport(path, contract, result.Status, *force); err != nil {
		return &ExitCodeError{Code: codeIOError, Message: fmt.Sprintf("unable to write summary report: %v", err)}
	}
	return &ExitCodeError{Code: result.ExitCode, Message: result.Message}
}

func loadAwaitContract(path string) (scan.Contract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return scan.Contract{}, err
	}
	var contract scan.Contract
	if err := json.Unmarshal(data, &contract); err != nil {
		return scan.Contract{}, err
	}
	if contract.Resources == nil {
		contract.Resources = []scan.ContractItem{}
	}
	return contract, nil
}

type awaitPollResult struct {
	Status    string
	Message   string
	ExitCode  int
	Pending   int
	Converged int
}

func pollAwait(contract scan.Contract, timeout, interval time.Duration, cmdCfg awscmd.Config) (awaitPollResult, error) {
	if len(contract.Resources) == 0 {
		return awaitPollResult{Status: "converged", Message: "no resources to check", ExitCode: 0, Pending: 0, Converged: 0}, nil
	}
	start := time.Now()
	maxAttempts := 1
	if timeout > 0 && interval > 0 {
		maxAttempts = int((timeout + interval - 1) / interval)
		if maxAttempts <= 0 {
			maxAttempts = 1
		}
	}
	for attempt := 1; ; attempt++ {
		pending, converged, status, message, err := awsContractState(contract, cmdCfg)
		if err != nil {
			return awaitPollResult{Status: "failed", Message: err.Error(), ExitCode: codeGenericFailure, Pending: pending, Converged: converged}, err
		}
		if status == "converged" {
			return awaitPollResult{Status: "converged", Message: message, ExitCode: 0, Pending: 0, Converged: converged}, nil
		}
		if status == "failed" {
			return awaitPollResult{Status: "failed", Message: message, ExitCode: codeGenericFailure, Pending: pending, Converged: converged}, &ExitCodeError{Code: codeGenericFailure, Message: message}
		}
		if DebugEnabled() {
			Debugf("retry %d/%d (elapsed %s)", attempt, maxAttempts, time.Since(start).Round(time.Second))
		}
		if timeout > 0 && time.Since(start) >= timeout {
			fmt.Fprintf(os.Stderr, "timeout reached after %s\n", time.Since(start).Round(time.Second))
			return awaitPollResult{Status: "timeout", Message: fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending), ExitCode: codeTimeout, Pending: pending, Converged: converged}, &ExitCodeError{Code: codeTimeout, Message: fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending)}
		}
		if timeout > 0 && attempt >= maxAttempts {
			time.Sleep(interval)
			if elapsed := time.Since(start); elapsed >= timeout {
				fmt.Fprintf(os.Stderr, "timeout reached after %s\n", elapsed.Round(time.Second))
				return awaitPollResult{Status: "timeout", Message: fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending), ExitCode: codeTimeout, Pending: pending, Converged: converged}, &ExitCodeError{Code: codeTimeout, Message: fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending)}
			}
			continue
		}
		if attempt >= maxAttempts {
			return awaitPollResult{Status: "pending", Message: fmt.Sprintf("%d resource(s) remain pending", pending), ExitCode: codeGenericFailure, Pending: pending, Converged: converged}, &ExitCodeError{Code: codeGenericFailure, Message: fmt.Sprintf("%d resource(s) remain pending", pending)}
		}
		time.Sleep(interval)
		if timeout > 0 && time.Since(start) >= timeout {
			fmt.Fprintf(os.Stderr, "timeout reached after %s\n", time.Since(start).Round(time.Second))
			return awaitPollResult{Status: "timeout", Message: fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending), ExitCode: codeTimeout, Pending: pending, Converged: converged}, &ExitCodeError{Code: codeTimeout, Message: fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending)}
		}
	}
}

func awsContractState(contract scan.Contract, cfg awscmd.Config) (pending int, converged int, status string, message string, err error) {
	pending = 0
	converged = 0
	for _, item := range contract.Resources {
		observed, runtimeErr := observeAWSResource(item, cfg)
		if runtimeErr != nil {
			return pending, converged, "failed", runtimeErr.Error(), runtimeErr
		}
		switch observed {
		case "converged":
			converged++
		case "failed":
			pending++
			status = "failed"
			message = fmt.Sprintf("resource %s failed to converge", item.Address)
		default:
			pending++
		}
	}
	if status == "failed" {
		return pending, converged, status, message, nil
	}
	if pending == 0 {
		return 0, converged, "converged", "all expected resources converged", nil
	}
	return pending, converged, "pending", fmt.Sprintf("%d resource(s) remain pending", pending), nil
}

func observeAWSResource(item scan.ContractItem, cfg awscmd.Config) (string, error) {
	if item.Observation.Strategy == "" {
		if item.Status == "" || !strings.EqualFold(strings.TrimSpace(item.Status), "converged") {
			return "pending", nil
		}
		return "converged", nil
	}
	resp, err := cfg.Run("autoscaling", "describe-instance-refreshes")
	if err == nil {
		var payload struct {
			InstanceRefreshes []struct {
				Status             string `json:"Status"`
				PercentageComplete int    `json:"PercentageComplete"`
			} `json:"InstanceRefreshes"`
		}
		if json.Unmarshal(resp, &payload) == nil && len(payload.InstanceRefreshes) > 0 {
			if DebugEnabled() {
				if formatted := renderDebugTemplate(payload.InstanceRefreshes[0]); formatted != "" {
					logger.logLine(formatted)
				}
			}
			switch strings.ToUpper(payload.InstanceRefreshes[0].Status) {
			case "SUCCESSFUL":
				return "converged", nil
			case "FAILED":
				return "failed", nil
			case "INPROGRESS":
				return "pending", nil
			}
		}
	} else {
		if item.Status == "" || !strings.EqualFold(strings.TrimSpace(item.Status), "converged") {
			return "pending", nil
		}
		return "converged", nil
	}
	resp, err = cfg.Run("autoscaling", "describe-auto-scaling-groups")
	if err != nil {
		if item.Status == "" || !strings.EqualFold(strings.TrimSpace(item.Status), "converged") {
			return "pending", nil
		}
		return "converged", nil
	}
	var payload struct {
		AutoScalingGroups []struct {
			Activities []struct {
				StatusCode string `json:"StatusCode"`
				Progress   int    `json:"Progress"`
			} `json:"Activities"`
			Instances []struct {
				LifecycleState string `json:"LifecycleState"`
			} `json:"Instances"`
		} `json:"AutoScalingGroups"`
	}
	if err := json.Unmarshal(resp, &payload); err != nil {
		return "", err
	}
	if len(payload.AutoScalingGroups) == 0 {
		return "pending", nil
	}
	group := payload.AutoScalingGroups[0]
	if DebugEnabled() {
		for _, activity := range group.Activities {
			if formatted := renderDebugTemplate(activity); formatted != "" {
				logger.logLine(formatted)
			}
		}
	}
	for _, activity := range group.Activities {
		switch strings.ToUpper(activity.StatusCode) {
		case "SUCCESSFUL":
			return "converged", nil
		case "FAILED":
			return "failed", nil
		case "INPROGRESS":
			return "pending", nil
		}
	}
	for _, instance := range group.Instances {
		if strings.EqualFold(instance.LifecycleState, "InService") {
			continue
		}
		return "pending", nil
	}
	return "converged", nil
}

func expectedPendingResources(contract scan.Contract) int {
	pending := 0
	for _, item := range contract.Resources {
		if item.Status == "" || !strings.EqualFold(strings.TrimSpace(item.Status), "converged") {
			pending++
		}
	}
	return pending
}

func awaitSummaryReport(path, status, message string, contract scan.Contract) awaitReport {
	if strings.EqualFold(strings.TrimSpace(status), "converged") {
		return awaitReport{
			SchemaVersion:      1,
			Status:             status,
			Message:            message,
			ContractPath:       path,
			StartedAt:          time.Now().UTC().Add(-time.Second),
			FinishedAt:         time.Now().UTC(),
			ExpectedResources:  len(contract.Resources),
			PendingResources:   0,
			ConvergedResources: len(contract.Resources),
		}
	}
	pending := expectedPendingResources(contract)
	converged := len(contract.Resources) - pending
	now := time.Now().UTC()
	return awaitReport{
		SchemaVersion:      1,
		Status:             status,
		Message:            message,
		ContractPath:       path,
		StartedAt:          now.Add(-time.Second),
		FinishedAt:         now,
		ExpectedResources:  len(contract.Resources),
		PendingResources:   pending,
		ConvergedResources: converged,
	}
}

func contractReportPath(contractPath string) string {
	name := filepath.Base(contractPath)
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if strings.HasSuffix(stem, ".convergence") {
		stem = strings.TrimSuffix(stem, ".convergence")
	}
	if stem == "" {
		stem = ".convergence"
	}
	return filepath.Join(filepath.Dir(contractPath), stem+".convergence-report.json")
}

func writeAwaitReport(contractPath string, contract scan.Contract, status string, force bool) error {
	updated := withObservationFulfilled(contract, status)
	reportPath := contractReportPath(contractPath)
	if safePath, err := safeOutputFilePath(reportPath, force); err != nil {
		return err
	} else {
		reportPath = safePath
	}
	resolved := reportPath
	if !filepath.IsAbs(resolved) {
		resolved, err := filepath.Abs(resolved)
		if err != nil {
			return err
		}
		reportPath = resolved
	}
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(reportPath, data, 0o644)
}

func withObservationFulfilled(contract scan.Contract, status string) scan.Contract {
	updated := contract
	for i := range updated.Resources {
		if updated.Resources[i].Observation.Fulfilled != nil {
			if *updated.Resources[i].Observation.Fulfilled {
				updated.Resources[i].Status = "converged"
			} else {
				updated.Resources[i].Status = "pending"
			}
			continue
		}
		if strings.EqualFold(status, "converged") {
			value := true
			updated.Resources[i].Status = "converged"
			updated.Resources[i].Observation.Fulfilled = &value
			continue
		}
		if strings.EqualFold(status, "failed") || strings.EqualFold(status, "timeout") {
			value := false
			updated.Resources[i].Status = "pending"
			updated.Resources[i].Observation.Fulfilled = &value
			continue
		}
		updated.Resources[i].Status = "pending"
		updated.Resources[i].Observation.Fulfilled = nil
	}
	return updated
}

func relativePathForCurrentWorkingDir(path string) string {
	if path == "" {
		return "."
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Clean(path)
}

var errBadConfig = errors.New("bad configuration")
