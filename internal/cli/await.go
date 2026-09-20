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
	maxContractResources = 200

	codeResourceLimitExceeded = 201
	codeConfigError           = 210
	codeIOError               = 220
	codeTimeout               = 230
	codeGenericFailure        = 240
)

// ExitCodeError indicates a process exit code with an optional message.
type ExitCodeError struct {
	Message string
	Code    int
}

type awaitReport struct {
	StartedAt          time.Time           `json:"started_at"`
	FinishedAt         time.Time           `json:"finished_at"`
	Status             string              `json:"status"`
	Message            string              `json:"message,omitempty"`
	ContractPath       string              `json:"contract_path"`
	Resources          []scan.ContractItem `json:"resources"`
	SchemaVersion      int                 `json:"schema_version"`
	ExpectedResources  int                 `json:"expected_resources"`
	PendingResources   int                 `json:"pending_resources"`
	ConvergedResources int                 `json:"converged_resources"`
}

type (
	awaitResourceWait struct {
		Address string
		Name    string
		Status  string
		Wait    time.Duration
	}

	awaitPollResult struct {
		StartedAt     time.Time
		FinishedAt    time.Time
		Status        string
		Message       string
		ResourceWaits []awaitResourceWait
		Contract      scan.Contract
		ExitCode      int
		Pending       int
		Converged     int
	}

	awsContractStateResult struct {
		Status    string
		Message   string
		Waits     []awaitResourceWait
		Pending   int
		Converged int
	}
)

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

func (e *ExitCodeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return fmt.Sprintf("exit code %d", e.Code)
	}
	return fmt.Sprintf("%s (exit code %d)", e.Message, e.Code)
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
		writeBestEffortf(c.out, "Usage: convergenci await <convergence.json>\n\n")
		writeBestEffortf(c.out, "Wait for the runtime state in a convergence contract to converge.\n\n")
		writeBestEffortf(c.out, "Flags:\n")
		writeBestEffortf(c.out, "  --timeout          Maximum time to wait before failing\n")
		writeBestEffortf(c.out, "  --interval         Polling interval between checks\n")
		writeBestEffortf(c.out, "  --force            Overwrite an existing report file\n")
		writeBestEffortf(
			c.out,
			"  --debug-format     Template for debug progress output, e.g. '{{ .Status }} {{ .PercentageComplete }}%%'\n",
		)

		writeBestEffort(c.out, awscmd.UsageText())
		writeBestEffortf(c.out, "  -h, --help         Show help\n")
	}

	if len(args) > 0 && (args[0] == shortHelpFlag || args[0] == longHelpFlag) {
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

	cmdCfg := c.commandAWSConfig(*awsCLIPath, *awsProfileName)

	if len(positionals) == 0 {
		return nil
	}

	path := positionals[0]
	if len(positionals) > 1 {
		writeBestEffortf(c.errOut, "convergence contract: %s\n", path)
	}
	if *timeout != 0 {
		writeBestEffortf(c.out, "timeout: %s\n", timeout.String())
	}
	if *interval != 0 {
		writeBestEffortf(c.out, "interval: %s\n", interval.String())
	}

	contract, err := prepareAwaitContract(path, *force)
	if err != nil {
		return err
	}

	result, err := pollAwait(contract, *timeout, *interval, cmdCfg)
	return finishAwait(path, contract, &result, *force, err)
}

func prepareAwaitContract(path string, force bool) (scan.Contract, error) {
	contract, err := loadAwaitContract(path)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
			return scan.Contract{}, &ExitCodeError{
				Code:    codeIOError,
				Message: "convergence contract not found: " + path,
			}
		}
		return scan.Contract{}, &ExitCodeError{
			Code:    codeIOError,
			Message: fmt.Sprintf("unable to read convergence contract: %v", err),
		}
	}
	if len(contract.Resources) <= maxContractResources {
		return contract, nil
	}
	if err := writeAwaitReport(path, contract, statusFailed, force, nil); err != nil {
		return scan.Contract{}, &ExitCodeError{
			Code:    codeIOError,
			Message: fmt.Sprintf("unable to write summary report: %v", err),
		}
	}
	return scan.Contract{}, &ExitCodeError{
		Code:    codeResourceLimitExceeded,
		Message: "expected resources exceeds 200",
	}
}

func finishAwait(path string, contract scan.Contract, result *awaitPollResult, force bool, pollErr error) error {
	if result.Contract.Resources != nil {
		contract = result.Contract
	}
	if err := writeAwaitReport(
		path,
		contract,
		result.Status,
		force,
		result.ResourceWaits,
		result.StartedAt,
		result.FinishedAt,
	); err != nil {
		return &ExitCodeError{Code: codeIOError, Message: fmt.Sprintf("unable to write summary report: %v", err)}
	}
	if pollErr != nil {
		return pollErr
	}
	if result.Status == statusConverged {
		return nil
	}
	return &ExitCodeError{Code: result.ExitCode, Message: result.Message}
}

func loadAwaitContract(path string) (scan.Contract, error) {
	// #nosec G304 G703 -- path is the contract file explicitly selected by the CLI user.
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
	// #nosec G304 -- baselinePath is derived from the user-selected contract path.
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
	for i := range baseline.Resources {
		item := &baseline.Resources[i]
		byAddress[item.Address] = *item
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

func pollAwait(
	contract scan.Contract,
	timeout, interval time.Duration,
	cmdCfg awscmd.Config,
) (awaitPollResult, error) {
	startedAt := time.Now().UTC()
	result, err := pollAwaitUntilDone(contract, timeout, interval, cmdCfg)
	result.StartedAt = startedAt
	result.FinishedAt = time.Now().UTC()
	return result, err
}

func pollAwaitUntilDone(
	contract scan.Contract,
	timeout, interval time.Duration,
	cmdCfg awscmd.Config,
) (awaitPollResult, error) {
	if len(contract.Resources) == 0 {
		return awaitPollResult{
			Status:    statusConverged,
			Message:   "no resources to check",
			ExitCode:  0,
			Pending:   0,
			Converged: 0,
			Contract:  contract,
		}, nil
	}
	start := time.Now().UTC()
	maxAttempts := 1
	if timeout > 0 && interval > 0 {
		maxAttempts = int((timeout + interval - 1) / interval)
		if maxAttempts <= 0 {
			maxAttempts = 1
		}
	}
	for attempt := 1; ; attempt++ {
		logRetryStarted(contract, attempt, maxAttempts, start)
		state := awsContractState(contract, cmdCfg)
		if result, err, done := terminalPollResult(contract, state, attempt, maxAttempts, start); done {
			return result, err
		}
		pending, converged, waits := state.Pending, state.Converged, state.Waits
		logRetryPending(state, attempt, maxAttempts, start)
		if timeout > 0 && time.Since(start) >= timeout {
			writeBestEffortf(os.Stderr, "timeout reached after %s\n", time.Since(start).Round(time.Second))
			result := awaitPollResult{
				Status:        statusTimeout,
				Message:       fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending),
				ExitCode:      codeTimeout,
				Pending:       pending,
				Converged:     converged,
				ResourceWaits: waits,
				Contract:      contract,
			}
			return result, &ExitCodeError{
				Code:    codeTimeout,
				Message: fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending),
			}
		}
		if timeout > 0 && attempt >= maxAttempts {
			time.Sleep(interval)
			if elapsed := time.Since(start); elapsed >= timeout {
				writeBestEffortf(os.Stderr, "timeout reached after %s\n", elapsed.Round(time.Second))
				result := awaitPollResult{
					Status:        statusTimeout,
					Message:       fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending),
					ExitCode:      codeTimeout,
					Pending:       pending,
					Converged:     converged,
					ResourceWaits: waits,
					Contract:      contract,
				}
				return result, &ExitCodeError{
					Code:    codeTimeout,
					Message: fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending),
				}
			}
			continue
		}
		if attempt >= maxAttempts {
			result := awaitPollResult{
				Status:        statusPending,
				Message:       fmt.Sprintf("%d resource(s) remain pending", pending),
				ExitCode:      codeGenericFailure,
				Pending:       pending,
				Converged:     converged,
				ResourceWaits: waits,
				Contract:      contract,
			}
			return result, &ExitCodeError{
				Code:    codeGenericFailure,
				Message: fmt.Sprintf("%d resource(s) remain pending", pending),
			}
		}
		time.Sleep(interval)
		if timeout > 0 && time.Since(start) >= timeout {
			writeBestEffortf(os.Stderr, "timeout reached after %s\n", time.Since(start).Round(time.Second))
			result := awaitPollResult{
				Status:        statusTimeout,
				Message:       fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending),
				ExitCode:      codeTimeout,
				Pending:       pending,
				Converged:     converged,
				ResourceWaits: waits,
				Contract:      contract,
			}
			return result, &ExitCodeError{
				Code:    codeTimeout,
				Message: fmt.Sprintf("timed out waiting for %d resource(s) to converge", pending),
			}
		}
	}
}

func logRetryStarted(contract scan.Contract, attempt, maxAttempts int, start time.Time) {
	if !DebugEnabled() || attempt != 1 {
		return
	}
	if formatted := renderDebugTemplate(map[string]any{
		debugFieldEvent:      "retry_started",
		debugFieldIteration:  attempt,
		debugFieldLimit:      maxAttempts,
		debugFieldElapsed:    time.Since(start).Round(time.Second),
		"ResourceNames":      contractResourceNames(contract),
		"ConvergedResources": []string{},
	}); formatted != "" {
		logger.logLine(formatted)
	}
}

func logRetryPending(state awsContractStateResult, attempt, maxAttempts int, start time.Time) {
	if !DebugEnabled() {
		return
	}
	if formatted := renderDebugTemplate(map[string]any{
		debugFieldEvent:     "retry",
		debugFieldIteration: attempt,
		debugFieldLimit:     maxAttempts,
		debugFieldElapsed:   time.Since(start).Round(time.Second),
		"PendingResources":  waitResourceNames(state.Waits, statusPending),
	}); formatted != "" {
		logger.logLine(formatted)
	}
}

func terminalPollResult(
	contract scan.Contract,
	state awsContractStateResult,
	attempt, maxAttempts int,
	start time.Time,
) (awaitPollResult, error, bool) {
	switch state.Status {
	case statusConverged:
		logRetrySatisfied(contract, state, attempt, maxAttempts, start)
		return awaitPollResult{
			Status:        statusConverged,
			Message:       state.Message,
			Converged:     state.Converged,
			ResourceWaits: state.Waits,
			Contract:      contract,
		}, nil, true
	case statusFailed:
		result := awaitPollResult{
			Status:        statusFailed,
			Message:       state.Message,
			ExitCode:      codeGenericFailure,
			Pending:       state.Pending,
			Converged:     state.Converged,
			ResourceWaits: state.Waits,
			Contract:      contract,
		}
		return result, &ExitCodeError{Code: codeGenericFailure, Message: state.Message}, true
	default:
		return awaitPollResult{}, nil, false
	}
}

func logRetrySatisfied(
	contract scan.Contract,
	state awsContractStateResult,
	attempt, maxAttempts int,
	start time.Time,
) {
	if !DebugEnabled() || attempt <= 1 || attempt >= maxAttempts {
		return
	}
	if formatted := renderDebugTemplate(map[string]any{
		debugFieldEvent:         "retry_satisfied",
		debugFieldStatus:        state.Status,
		"Message":               state.Message,
		debugFieldIteration:     attempt,
		debugFieldLimit:         maxAttempts,
		debugFieldElapsed:       time.Since(start).Round(time.Second),
		"ResourceNames":         contractResourceNames(contract),
		"ConvergedResources":    waitResourceNames(state.Waits, statusConverged),
		"AllResourcesConverged": true,
	}); formatted != "" {
		logger.logLine(formatted)
	}
}

func awsContractState(
	contract scan.Contract,
	cfg awscmd.Config,
) awsContractStateResult {
	state := awsContractStateResult{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := range contract.Resources {
		item := &contract.Resources[i]
		wg.Go(func() {
			start := time.Now()
			observed, arn, runtimeErr := observeAWSResource(item, cfg)
			elapsed := time.Since(start)
			mu.Lock()
			defer mu.Unlock()
			if arn != "" {
				contract.Resources[i].Observation.ARN = arn
			}
			if runtimeErr != nil {
				state.Waits = append(
					state.Waits,
					awaitResourceWait{Address: item.Address, Name: item.Name, Status: statusFailed, Wait: elapsed},
				)
				return
			}
			state.Waits = append(
				state.Waits,
				awaitResourceWait{Address: item.Address, Name: item.Name, Status: observed, Wait: elapsed},
			)
			if observed == statusConverged {
				state.Converged++
				return
			}
			if observed == statusFailed {
				state.Pending++
				if state.Status == "" {
					state.Status = statusFailed
					state.Message = fmt.Sprintf("resource %s failed to converge", item.Address)
				}
				return
			}
			state.Pending++
		})
	}
	wg.Wait()
	if state.Status == statusFailed {
		return state
	}
	if runtimeErrs := countRuntimeErrors(state.Waits); runtimeErrs > 0 {
		state.Status = statusFailed
		state.Message = fmt.Sprintf("%d resource(s) failed to converge", runtimeErrs)
		return state
	}
	if state.Pending == 0 {
		state.Status = statusConverged
		state.Message = "all expected resources converged"
		return state
	}
	state.Status = statusPending
	state.Message = fmt.Sprintf("%d resource(s) remain pending", state.Pending)
	return state
}

func contractResourceNames(contract scan.Contract) []string {
	names := make([]string, 0, len(contract.Resources))
	for i := range contract.Resources {
		item := &contract.Resources[i]
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
		if wait.Status == statusFailed {
			count++
		}
	}
	return count
}

func observeAWSResource(item *scan.ContractItem, cfg awscmd.Config) (string, string, error) {
	if item.Observation.Strategy == "" {
		if item.Status == "" || !strings.EqualFold(strings.TrimSpace(item.Status), statusConverged) {
			return statusPending, "", nil
		}
		return statusConverged, "", nil
	}
	refreshStatus, refreshPending := observeInstanceRefresh(item, cfg)
	if refreshPending {
		return statusPending, "", nil
	}
	groupArgs := []string{"autoscaling", "describe-auto-scaling-groups"}
	if item.Name != "" {
		groupArgs = append(groupArgs, "--auto-scaling-group-name", item.Name)
	}
	resp, err := cfg.Run(groupArgs[0], groupArgs[1:]...)
	if err != nil {
		//nolint:nilerr // AWS command failures fall back to the contract/refresh state by design.
		return fallbackObservationStatus(item, refreshStatus), "", nil
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
		return statusPending, "", nil
	}
	group := payload.AutoScalingGroups[0]
	arn := group.AutoScalingGroupARN
	logAutoScalingActivities(item, &group)
	if activityStatus := autoScalingActivityStatus(&group); activityStatus != "" {
		return finalizeGroupStatus(item, &group, activityStatus), arn, nil
	}
	if !autoScalingInstancesReady(&group) {
		return statusPending, arn, nil
	}
	if refreshStatus != "" {
		return finalizeGroupStatus(item, &group, refreshStatus), arn, nil
	}
	return finalizeGroupStatus(item, &group, statusConverged), arn, nil
}

func fallbackObservationStatus(item *scan.ContractItem, refreshStatus string) string {
	if refreshStatus != "" {
		return refreshStatus
	}
	if item.Status == "" || !strings.EqualFold(strings.TrimSpace(item.Status), statusConverged) {
		return statusPending
	}
	return statusConverged
}

func logAutoScalingActivities(item *scan.ContractItem, group *autoScalingGroupObservation) {
	if !DebugEnabled() {
		return
	}
	for _, activity := range group.Activities {
		if formatted := renderDebugTemplate(map[string]any{
			debugFieldEvent:      "autoscaling_activity",
			debugFieldSource:     "autoscaling_activity",
			debugFieldResource:   item.Address,
			"AutoScalingGroup":   group.AutoScalingGroupName,
			debugFieldStatus:     activity.StatusCode,
			"PercentageComplete": activity.Progress,
			"Cause":              activity.Cause,
		}); formatted != "" {
			logger.logLine(formatted)
		}
	}
}

func autoScalingActivityStatus(group *autoScalingGroupObservation) string {
	for _, activity := range group.Activities {
		switch strings.ToUpper(activity.StatusCode) {
		case "SUCCESSFUL":
			return statusConverged
		case "FAILED":
			return statusFailed
		case awsStatusInProgress:
			return statusPending
		}
	}
	return ""
}

func autoScalingInstancesReady(group *autoScalingGroupObservation) bool {
	for _, instance := range group.Instances {
		if !strings.EqualFold(instance.LifecycleState, "InService") {
			return false
		}
	}
	return true
}

func observeInstanceRefresh(item *scan.ContractItem, cfg awscmd.Config) (string, bool) {
	refreshArgs := []string{"autoscaling", "describe-instance-refreshes"}
	if item.Name != "" {
		refreshArgs = append(refreshArgs, "--auto-scaling-group-name", item.Name)
	}
	resp, err := cfg.Run(refreshArgs[0], refreshArgs[1:]...)
	if err != nil {
		if item.Status == "" || !strings.EqualFold(strings.TrimSpace(item.Status), statusConverged) {
			return "", true
		}
		return statusConverged, false
	}
	var payload struct {
		InstanceRefreshes []struct {
			CompletedAt          any    `json:"CompletedAt"`
			AutoScalingGroupName string `json:"AutoScalingGroupName"`
			InstanceRefreshID    string `json:"InstanceRefreshId"`
			Status               string `json:"Status"`
			PercentageComplete   int    `json:"PercentageComplete"`
		} `json:"InstanceRefreshes"`
	}
	if json.Unmarshal(resp, &payload) != nil || len(payload.InstanceRefreshes) == 0 {
		return "", false
	}
	refresh := payload.InstanceRefreshes[0]
	if DebugEnabled() {
		if formatted := renderDebugTemplate(map[string]any{
			debugFieldEvent:      "instance_refresh",
			debugFieldSource:     "instance_refresh",
			debugFieldResource:   item.Address,
			"AutoScalingGroup":   refresh.AutoScalingGroupName,
			"InstanceRefreshID":  refresh.InstanceRefreshID,
			debugFieldStatus:     refresh.Status,
			"PercentageComplete": refresh.PercentageComplete,
			"CompletedAt":        refresh.CompletedAt,
		}); formatted != "" {
			logger.logLine(formatted)
		}
	}
	switch strings.ToUpper(refresh.Status) {
	case "SUCCESSFUL":
		return statusConverged, false
	case "FAILED":
		return statusFailed, false
	case awsStatusInProgress, "PENDING":
		return "", true
	default:
		return "", false
	}
}

// finalizeGroupStatus downgrades a candidate "converged" status to "pending" when a desired
// generation requirement is verifiably not yet applied, giving AWS a grace period to reflect
// an in-flight rollover rather than reporting premature or false convergence.
func finalizeGroupStatus(item *scan.ContractItem, group *autoScalingGroupObservation, status string) string {
	if status == statusConverged && desiredGenerationUnmet(item, group) {
		return statusPending
	}
	return status
}

// desiredGenerationUnmet reports whether a desired generation requirement can be verified
// against the observed ASG and does not yet match. Requirements we cannot verify (e.g. the
// runtime response omits tags or launch template data) are treated as satisfied, granting a
// grace period for AWS to reflect the rollover before it is treated as pending.
func desiredGenerationUnmet(item *scan.ContractItem, group *autoScalingGroupObservation) bool {
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
func logDesiredGenerationPending(item *scan.ContractItem, requirementType, key, wanted, actual string) {
	if !DebugEnabled() {
		return
	}
	name := item.Name
	if name == "" {
		name = item.Address
	}
	if formatted := renderDebugTemplate(map[string]any{
		debugFieldEvent:    "desired_generation_pending",
		debugFieldSource:   "desired_generation",
		debugFieldResource: name,
		"RequirementType":  requirementType,
		"RequirementKey":   key,
		"Wanted":           wanted,
		"Actual":           actual,
	}); formatted != "" {
		logger.logLine(formatted)
	}
}

// logDesiredGenerationMet makes a verified, matching desired generation requirement (e.g. a
// completed tag rollover) explicit in debug output, symmetric to logDesiredGenerationPending.
func logDesiredGenerationMet(item *scan.ContractItem, requirementType, key, value string) {
	if !DebugEnabled() {
		return
	}
	name := item.Name
	if name == "" {
		name = item.Address
	}
	if formatted := renderDebugTemplate(map[string]any{
		debugFieldEvent:    "desired_generation_met",
		debugFieldSource:   "desired_generation",
		debugFieldResource: name,
		"RequirementType":  requirementType,
		"RequirementKey":   key,
		"Wanted":           value,
		"Actual":           value,
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

func groupLaunchTemplateVersion(group *autoScalingGroupObservation) (string, bool) {
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
	for i := range contract.Resources {
		item := &contract.Resources[i]
		if item.Status == "" || !strings.EqualFold(strings.TrimSpace(item.Status), statusConverged) {
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
	if strings.EqualFold(strings.TrimSpace(status), statusConverged) {
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
	if strings.EqualFold(strings.TrimSpace(status), statusConverged) {
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
	if err := os.MkdirAll(filepath.Dir(reportPath), generatedDirectoryMode); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	// #nosec G703 -- reportPath is the user-selected output validated by safeOutputFilePath.
	return os.WriteFile(reportPath, data, generatedFileMode)
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
				updated.Resources[i].Status = statusConverged
			} else {
				updated.Resources[i].Status = statusPending
			}
			continue
		}
		if waitStatus, ok := waitStatusByAddress[updated.Resources[i].Address]; ok {
			switch waitStatus {
			case statusConverged:
				value := true
				updated.Resources[i].Status = statusConverged
				updated.Resources[i].Observation.Fulfilled = &value
			case statusFailed:
				value := false
				updated.Resources[i].Status = statusPending
				updated.Resources[i].Observation.Fulfilled = &value
			default:
				updated.Resources[i].Status = statusPending
				updated.Resources[i].Observation.Fulfilled = nil
			}
			continue
		}
		switch {
		case strings.EqualFold(status, statusConverged):
			value := true
			updated.Resources[i].Status = statusConverged
			updated.Resources[i].Observation.Fulfilled = &value
		case strings.EqualFold(status, statusFailed), strings.EqualFold(status, statusTimeout):
			value := false
			updated.Resources[i].Status = statusPending
			updated.Resources[i].Observation.Fulfilled = &value
		default:
			updated.Resources[i].Status = statusPending
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
