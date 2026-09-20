package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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
	SchemaVersion      int                 `json:"schema_version"`
	Status             string              `json:"status"`
	Message            string              `json:"message,omitempty"`
	ContractPath       string              `json:"contract_path"`
	StartedAt          time.Time           `json:"started_at"`
	FinishedAt         time.Time           `json:"finished_at"`
	ExpectedResources  int                 `json:"expected_resources"`
	PendingResources   int                 `json:"pending_resources"`
	ConvergedResources int                 `json:"converged_resources"`
	Resources          []scan.ContractItem `json:"resources"`
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
	envDebugFormat := strings.TrimSpace(os.Getenv("CONVERGENCI_DEBUG_FORMAT"))
	if envDebugFormat == "" {
		envDebugFormat = defaultDebugFormat
	}
	timeout := fs.Duration("timeout", defaultTimeout, "maximum time to wait before failing")
	interval := fs.Duration("interval", defaultInterval, "polling interval between checks")
	force := fs.Bool("force", false, "overwrite an existing report file")
	debugFormat := fs.String(
		"debug-format",
		envDebugFormat,
		"template for debug progress output; e.g. '{{ .Status }} {{ .PercentageComplete }}%' ",
	)
	awsCLIPath, awsProfileName := awscmd.RegisterFlags(fs)
	fs.Usage = func() {
		fmt.Fprintf(c.out, "Usage: convergenci await <convergence.json>\n\n")
		fmt.Fprintf(c.out, "Wait for the runtime state in a convergence contract to converge.\n\n")
		fmt.Fprintf(c.out, "Flags:\n")
		fmt.Fprintf(c.out, "  --timeout          Maximum time to wait before failing\n")
		fmt.Fprintf(c.out, "  --interval         Polling interval between checks\n")
		fmt.Fprintf(c.out, "  --force            Overwrite an existing report file\n")
		fmt.Fprintf(
			c.out,
			"  --debug-format     Template for debug progress output, e.g. '{{ .Status }} {{ .PercentageComplete }}%%'\n",
		)
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
		if err := writeAwaitReport(path, contract, "failed", *force, nil); err != nil {
			return &ExitCodeError{Code: codeIOError, Message: fmt.Sprintf("unable to write summary report: %v", err)}
		}
		return &ExitCodeError{Code: codeResourceLimitExceeded, Message: "expected resources exceeds 200"}
	}

	result, err := pollAwait(contract, *timeout, *interval, cmdCfg)
	if result.Contract.Resources != nil {
		contract = result.Contract
	}
	if err != nil {
		if reportErr := writeAwaitReport(
			path,
			contract,
			result.Status,
			*force,
			result.ResourceWaits,
			result.StartedAt,
			result.FinishedAt,
		); reportErr != nil {
			return &ExitCodeError{
				Code:    codeIOError,
				Message: fmt.Sprintf("unable to write summary report: %v", reportErr),
			}
		}
		return err
	}
	if result.Status == "converged" {
		if err := writeAwaitReport(
			path,
			contract,
			result.Status,
			*force,
			result.ResourceWaits,
			result.StartedAt,
			result.FinishedAt,
		); err != nil {
			return &ExitCodeError{Code: codeIOError, Message: fmt.Sprintf("unable to write summary report: %v", err)}
		}
		return nil
	}
	if err := writeAwaitReport(
		path,
		contract,
		result.Status,
		*force,
		result.ResourceWaits,
		result.StartedAt,
		result.FinishedAt,
	); err != nil {
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
	if baseline, err := loadAwaitBaseline(path); err == nil && len(baseline.Resources) > 0 {
		contract = preferAwaitBaseline(contract, baseline)
	}
	return contract, nil
}

func baselineFilePath(contractPath string) string {
	dir := filepath.Dir(contractPath)
	base := filepath.Base(contractPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" {
		stem = "baseline"
	}
	return filepath.Join(dir, stem+".baseline.json")
}

func loadAwaitBaseline(path string) (scan.Contract, error) {
	baselinePath := baselineFilePath(path)
	data, err := os.ReadFile(baselinePath)
	if err != nil {
		return scan.Contract{}, err
	}
	var baseline scan.Contract
	if err := json.Unmarshal(data, &baseline); err != nil {
		return scan.Contract{}, err
	}
	if baseline.Resources == nil {
		baseline.Resources = []scan.ContractItem{}
	}
	return baseline, nil
}

func preferAwaitBaseline(contract, baseline scan.Contract) scan.Contract {
	if len(contract.Resources) == 0 {
		return baseline
	}
	if len(baseline.Resources) == 0 {
		return contract
	}
	byAddress := map[string]scan.ContractItem{}
	for _, item := range baseline.Resources {
		byAddress[item.Address] = item
	}
	merged := contract
	for i := range merged.Resources {
		if baselineItem, ok := byAddress[merged.Resources[i].Address]; ok {
			if merged.Resources[i].Observation.Strategy == "" && baselineItem.Observation.Strategy != "" {
				merged.Resources[i].Observation = baselineItem.Observation
			}
			if merged.Resources[i].Status == "" && baselineItem.Status != "" {
				merged.Resources[i].Status = baselineItem.Status
			}
		}
	}
	return merged
}

type awaitResourceWait struct {
	Address string
	Name    string
	Status  string
	Wait    time.Duration
}

type awaitPollResult struct {
	Status        string
	Message       string
	ExitCode      int
	Pending       int
	Converged     int
	ResourceWaits []awaitResourceWait
	Contract      scan.Contract
	StartedAt     time.Time
	FinishedAt    time.Time
}

func pollAwait(
	contract scan.Contract,
	timeout, interval time.Duration,
	cmdCfg awscmd.Config,
) (result awaitPollResult, err error) {
	start := time.Now().UTC()
	defer func() {
		result.StartedAt = start
		result.FinishedAt = time.Now().UTC()
	}()
	if len(contract.Resources) == 0 {
		return awaitPollResult{
			Status:    "converged",
			Message:   "no resources to check",
			ExitCode:  0,
			Pending:   0,
			Converged: 0,
			Contract:  contract,
		}, nil
	}
	start = time.Now().UTC()
	maxAttempts := 1
	if timeout > 0 && interval > 0 {
		maxAttempts = int((timeout + interval - 1) / interval)
		if maxAttempts <= 0 {
			maxAttempts = 1
		}
	}
	for attempt := 1; ; attempt++ {
		if DebugEnabled() && attempt == 1 {
			if formatted := renderDebugTemplate(map[string]any{
				"Event":              "retry_started",
				"RetryIteration":     attempt,
				"RetryLimit":         maxAttempts,
				"Elapsed":            time.Since(start).Round(time.Second),
				"ResourceNames":      contractResourceNames(contract),
				"ConvergedResources": []string{},
			}); formatted != "" {
				logger.logLine(formatted)
			}
		}
		pending, converged, status, message, waits, err := awsContractState(contract, cmdCfg)
		if err != nil {
			return awaitPollResult{
				Status:        "failed",
				Message:       err.Error(),
				ExitCode:      codeGenericFailure,
				Pending:       pending,
				Converged:     converged,
				ResourceWaits: waits,
				Contract:      contract,
			}, err
		}
		if status == "converged" {
			if DebugEnabled() && attempt > 1 && attempt < maxAttempts {
				if formatted := renderDebugTemplate(map[string]any{
					"Event":                 "retry_satisfied",
					"Status":                status,
					"Message":               message,
					"RetryIteration":        attempt,
					"RetryLimit":            maxAttempts,
					"Elapsed":               time.Since(start).Round(time.Second),
					"ResourceNames":         contractResourceNames(contract),
					"ConvergedResources":    waitResourceNames(waits, "converged"),
					"AllResourcesConverged": true,
				}); formatted != "" {
					logger.logLine(formatted)
				}
			}
			return awaitPollResult{
				Status:        "converged",
				Message:       message,
				ExitCode:      0,
				Pending:       0,
				Converged:     converged,
				ResourceWaits: waits,
				Contract:      contract,
			}, nil
		}
		if status == "failed" {
			return awaitPollResult{
					Status:        "failed",
					Message:       message,
					ExitCode:      codeGenericFailure,
					Pending:       pending,
					Converged:     converged,
					ResourceWaits: waits,
					Contract:      contract,
				}, &ExitCodeError{
					Code:    codeGenericFailure,
					Message: message,
				}
		}
		if DebugEnabled() {
			if formatted := renderDebugTemplate(map[string]any{
				"Event":            "retry",
				"RetryIteration":   attempt,
				"RetryLimit":       maxAttempts,
				"Elapsed":          time.Since(start).Round(time.Second),
				"PendingResources": waitResourceNames(waits, "pending"),
			}); formatted != "" {
				logger.logLine(formatted)
			}
		}
		if timeout > 0 && time.Since(start) >= timeout {
			fmt.Fprintf(os.Stderr, "timeout reached after %s\n", time.Since(start).Round(time.Second))
			return awaitPollResult{
					Status:        "timeout",
					Message:       fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending),
					ExitCode:      codeTimeout,
					Pending:       pending,
					Converged:     converged,
					ResourceWaits: waits,
					Contract:      contract,
				}, &ExitCodeError{
					Code:    codeTimeout,
					Message: fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending),
				}
		}
		if timeout > 0 && attempt >= maxAttempts {
			time.Sleep(interval)
			if elapsed := time.Since(start); elapsed >= timeout {
				fmt.Fprintf(os.Stderr, "timeout reached after %s\n", elapsed.Round(time.Second))
				return awaitPollResult{
						Status:        "timeout",
						Message:       fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending),
						ExitCode:      codeTimeout,
						Pending:       pending,
						Converged:     converged,
						ResourceWaits: waits,
						Contract:      contract,
					}, &ExitCodeError{
						Code:    codeTimeout,
						Message: fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending),
					}
			}
			continue
		}
		if attempt >= maxAttempts {
			return awaitPollResult{
					Status:        "pending",
					Message:       fmt.Sprintf("%d resource(s) remain pending", pending),
					ExitCode:      codeGenericFailure,
					Pending:       pending,
					Converged:     converged,
					ResourceWaits: waits,
					Contract:      contract,
				}, &ExitCodeError{
					Code:    codeGenericFailure,
					Message: fmt.Sprintf("%d resource(s) remain pending", pending),
				}
		}
		time.Sleep(interval)
		if timeout > 0 && time.Since(start) >= timeout {
			fmt.Fprintf(os.Stderr, "timeout reached after %s\n", time.Since(start).Round(time.Second))
			return awaitPollResult{
					Status:        "timeout",
					Message:       fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending),
					ExitCode:      codeTimeout,
					Pending:       pending,
					Converged:     converged,
					ResourceWaits: waits,
					Contract:      contract,
				}, &ExitCodeError{
					Code:    codeTimeout,
					Message: fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending),
				}
		}
	}
}

func awsContractState(
	contract scan.Contract,
	cfg awscmd.Config,
) (pending, converged int, status, message string, waits []awaitResourceWait, err error) {
	pending = 0
	converged = 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i, item := range contract.Resources {
		idx := i
		item := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			observed, arn, runtimeErr := observeAWSResource(item, cfg)
			elapsed := time.Since(start)
			mu.Lock()
			defer mu.Unlock()
			if arn != "" {
				contract.Resources[idx].Observation.ARN = arn
			}
			if runtimeErr != nil {
				waits = append(
					waits,
					awaitResourceWait{Address: item.Address, Name: item.Name, Status: "failed", Wait: elapsed},
				)
				return
			}
			waits = append(
				waits,
				awaitResourceWait{Address: item.Address, Name: item.Name, Status: observed, Wait: elapsed},
			)
			if observed == "converged" {
				converged++
				return
			}
			if observed == "failed" {
				pending++
				if status == "" {
					status = "failed"
					message = fmt.Sprintf("resource %s failed to converge", item.Address)
				}
				return
			}
			pending++
		}()
	}
	wg.Wait()
	if status == "failed" {
		return pending, converged, status, message, waits, nil
	}
	if runtimeErrs := countRuntimeErrors(waits); runtimeErrs > 0 {
		return pending, converged, "failed", fmt.Sprintf("%d resource(s) failed to converge", runtimeErrs), waits, nil
	}
	if pending == 0 {
		return 0, converged, "converged", "all expected resources converged", waits, nil
	}
	return pending, converged, "pending", fmt.Sprintf("%d resource(s) remain pending", pending), waits, nil
}

func contractResourceNames(contract scan.Contract) []string {
	names := make([]string, 0, len(contract.Resources))
	for _, item := range contract.Resources {
		name := item.Name
		if name == "" {
			name = item.Address
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func waitResourceNames(waits []awaitResourceWait, status string) []string {
	names := make([]string, 0, len(waits))
	for _, wait := range waits {
		if wait.Status != status {
			continue
		}
		name := wait.Name
		if name == "" {
			name = wait.Address
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func countRuntimeErrors(waits []awaitResourceWait) int {
	count := 0
	for _, wait := range waits {
		if wait.Status == "failed" {
			count++
		}
	}
	return count
}

func observeAWSResource(item scan.ContractItem, cfg awscmd.Config) (status, arn string, err error) {
	if item.Observation.Strategy == "" {
		if item.Status == "" || !strings.EqualFold(strings.TrimSpace(item.Status), "converged") {
			return "pending", "", nil
		}
		return "converged", "", nil
	}
	refreshStatus := ""
	refreshArgs := []string{"autoscaling", "describe-instance-refreshes"}
	if item.Name != "" {
		refreshArgs = append(refreshArgs, "--auto-scaling-group-name", item.Name)
	}
	resp, err := cfg.Run(refreshArgs[0], refreshArgs[1:]...)
	if err == nil {
		var payload struct {
			InstanceRefreshes []struct {
				AutoScalingGroupName string `json:"AutoScalingGroupName"`
				InstanceRefreshID    string `json:"InstanceRefreshId"`
				Status               string `json:"Status"`
				PercentageComplete   int    `json:"PercentageComplete"`
				CompletedAt          any    `json:"CompletedAt"`
			} `json:"InstanceRefreshes"`
		}
		if json.Unmarshal(resp, &payload) == nil && len(payload.InstanceRefreshes) > 0 {
			if DebugEnabled() {
				refresh := payload.InstanceRefreshes[0]
				if formatted := renderDebugTemplate(map[string]any{
					"Event":              "instance_refresh",
					"Source":             "instance_refresh",
					"Resource":           item.Address,
					"AutoScalingGroup":   refresh.AutoScalingGroupName,
					"InstanceRefreshID":  refresh.InstanceRefreshID,
					"Status":             refresh.Status,
					"PercentageComplete": refresh.PercentageComplete,
					"CompletedAt":        refresh.CompletedAt,
				}); formatted != "" {
					logger.logLine(formatted)
				}
			}
			switch strings.ToUpper(payload.InstanceRefreshes[0].Status) {
			case "SUCCESSFUL":
				refreshStatus = "converged"
			case "FAILED":
				refreshStatus = "failed"
			case "INPROGRESS", "PENDING":
				return "pending", "", nil
			}
		}
	} else {
		if item.Status == "" || !strings.EqualFold(strings.TrimSpace(item.Status), "converged") {
			return "pending", "", nil
		}
		refreshStatus = "converged"
	}
	groupArgs := []string{"autoscaling", "describe-auto-scaling-groups"}
	if item.Name != "" {
		groupArgs = append(groupArgs, "--auto-scaling-group-name", item.Name)
	}
	resp, err = cfg.Run(groupArgs[0], groupArgs[1:]...)
	if err != nil {
		if refreshStatus != "" {
			return refreshStatus, "", nil
		}
		if item.Status == "" || !strings.EqualFold(strings.TrimSpace(item.Status), "converged") {
			return "pending", "", nil
		}
		return "converged", "", nil
	}
	var payload struct {
		AutoScalingGroups []autoScalingGroupObservation `json:"AutoScalingGroups"`
	}
	if err := json.Unmarshal(resp, &payload); err != nil {
		return "", "", err
	}
	if len(payload.AutoScalingGroups) == 0 {
		if refreshStatus != "" {
			return refreshStatus, "", nil
		}
		return "pending", "", nil
	}
	group := payload.AutoScalingGroups[0]
	arn = group.AutoScalingGroupARN
	if DebugEnabled() {
		for _, activity := range group.Activities {
			if formatted := renderDebugTemplate(map[string]any{
				"Event":              "autoscaling_activity",
				"Source":             "autoscaling_activity",
				"Resource":           item.Address,
				"AutoScalingGroup":   group.AutoScalingGroupName,
				"Status":             activity.StatusCode,
				"PercentageComplete": activity.Progress,
				"Cause":              activity.Cause,
			}); formatted != "" {
				logger.logLine(formatted)
			}
		}
	}
	for _, activity := range group.Activities {
		switch strings.ToUpper(activity.StatusCode) {
		case "SUCCESSFUL":
			return finalizeGroupStatus(item, group, "converged"), arn, nil
		case "FAILED":
			return "failed", arn, nil
		case "INPROGRESS":
			return "pending", arn, nil
		}
	}
	for _, instance := range group.Instances {
		if strings.EqualFold(instance.LifecycleState, "InService") {
			continue
		}
		return "pending", arn, nil
	}
	if refreshStatus != "" {
		return finalizeGroupStatus(item, group, refreshStatus), arn, nil
	}
	return finalizeGroupStatus(item, group, "converged"), arn, nil
}

// autoScalingGroupObservation is the subset of the AWS describe-auto-scaling-groups response
// used to observe convergence, including the runtime data needed to verify desired generation
// requirements (tags, launch template version) against the convergence contract.
type autoScalingGroupObservation struct {
	AutoScalingGroupName string   `json:"AutoScalingGroupName"`
	AutoScalingGroupARN  string   `json:"AutoScalingGroupARN"`
	Tags                 []awsTag `json:"Tags"`
	LaunchTemplate       *struct {
		Version string `json:"Version"`
	} `json:"LaunchTemplate"`
	MixedInstancesPolicy *struct {
		LaunchTemplate struct {
			LaunchTemplateSpecification struct {
				Version string `json:"Version"`
			} `json:"LaunchTemplateSpecification"`
		} `json:"LaunchTemplate"`
	} `json:"MixedInstancesPolicy"`
	Activities []struct {
		Cause      string `json:"Cause"`
		StatusCode string `json:"StatusCode"`
		Progress   int    `json:"Progress"`
	} `json:"Activities"`
	Instances []struct {
		LifecycleState string `json:"LifecycleState"`
	} `json:"Instances"`
}

type awsTag struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

// finalizeGroupStatus downgrades a candidate "converged" status to "pending" when a desired
// generation requirement is verifiably not yet applied, giving AWS a grace period to reflect
// an in-flight rollover rather than reporting premature or false convergence.
func finalizeGroupStatus(item scan.ContractItem, group autoScalingGroupObservation, status string) string {
	if status == "converged" && desiredGenerationUnmet(item, group) {
		return "pending"
	}
	return status
}

// desiredGenerationUnmet reports whether a desired generation requirement can be verified
// against the observed ASG and does not yet match. Requirements we cannot verify (e.g. the
// runtime response omits tags or launch template data) are treated as satisfied, granting a
// grace period for AWS to reflect the rollover before it is treated as pending.
func desiredGenerationUnmet(item scan.ContractItem, group autoScalingGroupObservation) bool {
	unmet := false
	for _, requirement := range item.DesiredGeneration {
		want := fmt.Sprintf("%v", requirement.Value)
		switch requirement.Type {
		case "tag":
			value, ok := groupTagValue(group.Tags, requirement.Key)
			if !ok {
				continue
			}
			if value != want {
				logDesiredGenerationPending(item, requirement.Type, requirement.Key, want, value)
				unmet = true
			} else {
				logDesiredGenerationMet(item, requirement.Type, requirement.Key, want)
			}
		case "launch_template":
			value, ok := groupLaunchTemplateVersion(group)
			if !ok {
				continue
			}
			if value != want {
				logDesiredGenerationPending(item, requirement.Type, requirement.Key, want, value)
				unmet = true
			} else {
				logDesiredGenerationMet(item, requirement.Type, requirement.Key, want)
			}
		}
	}
	return unmet
}

// logDesiredGenerationPending makes the otherwise-invisible grace-period wait for a specific
// desired generation requirement (e.g. a tag rollover) explicit in debug output.
func logDesiredGenerationPending(item scan.ContractItem, requirementType, key, wanted, actual string) {
	if !DebugEnabled() {
		return
	}
	name := item.Name
	if name == "" {
		name = item.Address
	}
	if formatted := renderDebugTemplate(map[string]any{
		"Event":           "desired_generation_pending",
		"Source":          "desired_generation",
		"Resource":        name,
		"RequirementType": requirementType,
		"RequirementKey":  key,
		"Wanted":          wanted,
		"Actual":          actual,
	}); formatted != "" {
		logger.logLine(formatted)
	}
}

// logDesiredGenerationMet makes a verified, matching desired generation requirement (e.g. a
// completed tag rollover) explicit in debug output, symmetric to logDesiredGenerationPending.
func logDesiredGenerationMet(item scan.ContractItem, requirementType, key, value string) {
	if !DebugEnabled() {
		return
	}
	name := item.Name
	if name == "" {
		name = item.Address
	}
	if formatted := renderDebugTemplate(map[string]any{
		"Event":           "desired_generation_met",
		"Source":          "desired_generation",
		"Resource":        name,
		"RequirementType": requirementType,
		"RequirementKey":  key,
		"Wanted":          value,
		"Actual":          value,
	}); formatted != "" {
		logger.logLine(formatted)
	}
}

func groupTagValue(tags []awsTag, key string) (string, bool) {
	for _, tag := range tags {
		if tag.Key == key {
			return tag.Value, true
		}
	}
	return "", false
}

func groupLaunchTemplateVersion(group autoScalingGroupObservation) (string, bool) {
	if group.LaunchTemplate != nil && group.LaunchTemplate.Version != "" {
		return group.LaunchTemplate.Version, true
	}
	if group.MixedInstancesPolicy != nil {
		if version := group.MixedInstancesPolicy.LaunchTemplate.LaunchTemplateSpecification.Version; version != "" {
			return version, true
		}
	}
	return "", false
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

func awaitSummaryReport(path, status, message string, contract scan.Contract, times ...time.Time) awaitReport {
	startedAt := time.Now().UTC().Add(-time.Second)
	finishedAt := time.Now().UTC()
	if len(times) >= 2 && !times[0].IsZero() && !times[1].IsZero() {
		startedAt = times[0]
		finishedAt = times[1]
	}
	if strings.EqualFold(strings.TrimSpace(status), "converged") {
		return awaitReport{
			SchemaVersion:      1,
			Status:             status,
			Message:            message,
			ContractPath:       path,
			StartedAt:          startedAt,
			FinishedAt:         finishedAt,
			ExpectedResources:  len(contract.Resources),
			PendingResources:   0,
			ConvergedResources: len(contract.Resources),
			Resources:          contract.Resources,
		}
	}
	pending := expectedPendingResources(contract)
	converged := len(contract.Resources) - pending
	return awaitReport{
		SchemaVersion:      1,
		Status:             status,
		Message:            message,
		ContractPath:       path,
		StartedAt:          startedAt,
		FinishedAt:         finishedAt,
		ExpectedResources:  len(contract.Resources),
		PendingResources:   pending,
		ConvergedResources: converged,
		Resources:          contract.Resources,
	}
}

func contractReportPath(contractPath string) string {
	name := filepath.Base(contractPath)
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	stem = strings.TrimSuffix(stem, ".convergence")
	if stem == "" {
		stem = ".convergence"
	}
	return filepath.Join(filepath.Dir(contractPath), stem+".convergence-report.json")
}

func writeAwaitReport(
	contractPath string,
	contract scan.Contract,
	status string,
	force bool,
	waits []awaitResourceWait,
	times ...time.Time,
) error {
	updated := withObservationMetadata(contract, waits, status)
	message := fmt.Sprintf("%d resource(s) remain pending", expectedPendingResources(updated))
	if strings.EqualFold(strings.TrimSpace(status), "converged") {
		message = "all resources converged"
	}
	report := awaitSummaryReport(contractPath, status, message, updated, times...)
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
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(reportPath, data, 0o644)
}

func withObservationMetadata(contract scan.Contract, waits []awaitResourceWait, status string) scan.Contract {
	updated := contract
	waitStatusByAddress := make(map[string]string, len(waits))
	for _, wait := range waits {
		waitStatusByAddress[wait.Address] = wait.Status
	}
	for i := range updated.Resources {
		if updated.Resources[i].Observation.Fulfilled != nil {
			if *updated.Resources[i].Observation.Fulfilled {
				updated.Resources[i].Status = "converged"
			} else {
				updated.Resources[i].Status = "pending"
			}
			continue
		}
		if waitStatus, ok := waitStatusByAddress[updated.Resources[i].Address]; ok {
			switch waitStatus {
			case "converged":
				value := true
				updated.Resources[i].Status = "converged"
				updated.Resources[i].Observation.Fulfilled = &value
			case "failed":
				value := false
				updated.Resources[i].Status = "pending"
				updated.Resources[i].Observation.Fulfilled = &value
			default:
				updated.Resources[i].Status = "pending"
				updated.Resources[i].Observation.Fulfilled = nil
			}
			continue
		}
		if strings.EqualFold(status, "converged") {
			value := true
			updated.Resources[i].Status = "converged"
			updated.Resources[i].Observation.Fulfilled = &value
		} else if strings.EqualFold(status, "failed") || strings.EqualFold(status, "timeout") {
			value := false
			updated.Resources[i].Status = "pending"
			updated.Resources[i].Observation.Fulfilled = &value
		} else {
			updated.Resources[i].Status = "pending"
			updated.Resources[i].Observation.Fulfilled = nil
		}
	}
	return updated
}

func withObservationFulfilled(contract scan.Contract, status string) scan.Contract {
	return withObservationMetadata(contract, nil, status)
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
