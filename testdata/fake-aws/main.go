package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func fakeAWSVersion() string {
	return "aws-cli/2.17.0 Python/3.12.0 Linux/6.0 exe/x86_64"
}

func fakeAWSCallerIdentity() string {
	return `{
  "UserId": "fake-user-123",
  "Account": "123456789012",
  "Arn": "arn:aws:iam::123456789012:user/fake-user"
}`
}

func scenarioName(scenario string) string {
	switch scenario {
	case "", "success":
		return "success"
	case "not-started":
		return "not-started"
	case "not-started-then-success":
		return "not-started-then-success"
	case "failed":
		return "failed"
	case "in-progress":
		return "in-progress"
	case "in-progress-then-success":
		return "in-progress-then-success"
	case "terragrunt-multi-stack":
		return "terragrunt-multi-stack"
	case "in-progress-then-failed":
		return "in-progress-then-failed"
	case "late-success":
		return "late-success"
	case "complex-report":
		return "complex-report"
	case "tag-rotation-lagging":
		return "tag-rotation-lagging"
	case "mixed-resource-outcomes":
		return "mixed-resource-outcomes"
	default:
		return "success"
	}
}

func scenarioState(scenario string) string {
	switch scenario {
	case "in-progress":
		incrementScenarioCount(scenario)
		return "in-progress"
	case "in-progress-then-success", "terragrunt-multi-stack":
		return delayedOutcome(scenario, 2)
	case "in-progress-then-failed":
		return delayedOutcome("in-progress-then-failed", 2)
	case "late-success":
		return delayedOutcome("late-success", 3)
	case "failed":
		return "failed"
	default:
		return scenario
	}
}

func scenarioStatePath(scenario string) string {
	name := strings.TrimSpace(scenario)
	if name == "" {
		name = "success"
	}
	name = strings.ReplaceAll(name, "-", "_")
	return filepath.Join(os.TempDir(), fmt.Sprintf("convergenci-fake-aws-state-%s", name))
}

func delayedOutcome(scenario string, threshold int) string {
	count := incrementScenarioCount(scenario)
	if count < threshold {
		return "in-progress"
	}
	return "success"
}

func incrementScenarioCount(scenario string) int {
	statePath := scenarioStatePath(scenario)
	data, err := os.ReadFile(statePath)
	count := 0
	if err == nil {
		count, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}
	count++
	_ = os.WriteFile(statePath, []byte(strconv.Itoa(count)), 0o600)
	return count
}

func scenarioCount(scenario string) int {
	data, err := os.ReadFile(scenarioStatePath(scenario))
	if err != nil {
		return 0
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return count
}

func asgResponse(asgName, rotationTag, launchTemplateVersion, activityStatus string, activityProgress int) map[string]any {
	return map[string]any{
		"AutoScalingGroups": []map[string]any{{
			"AutoScalingGroupName": asgName,
			"AutoScalingGroupARN":  fakeASGARN(asgName),
			"DesiredCapacity":      2,
			"MinSize":              1,
			"MaxSize":              3,
			"Tags": []map[string]any{
				{"Key": "rotation", "Value": rotationTag},
			},
			"LaunchTemplate": map[string]any{
				"Version": launchTemplateVersion,
			},
			"Instances": []map[string]any{
				{"LifecycleState": "InService", "HealthStatus": "Healthy", "InstanceId": "i-1234567890"},
				{"LifecycleState": "InService", "HealthStatus": "Healthy", "InstanceId": "i-0987654321"},
			},
			"Activities": []map[string]any{{"Cause": "None", "StatusCode": activityStatus, "Progress": activityProgress}},
		}},
	}
}

func refreshResponse(asgName, status string, percentage int, completedAt any) map[string]any {
	return map[string]any{
		"InstanceRefreshes": []map[string]any{{
			"AutoScalingGroupName": asgName,
			"InstanceRefreshId":    "ir-123",
			"Status":               status,
			"PercentageComplete":   percentage,
			"CompletedAt":          completedAt,
		}},
	}
}

func progressPercent(scenario string, steps ...int) int {
	count := 0
	data, err := os.ReadFile(scenarioStatePath(scenario))
	if err == nil {
		count, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}
	if count <= 0 {
		if len(steps) == 0 {
			return 0
		}
		return steps[0]
	}
	if count > len(steps) {
		if len(steps) == 0 {
			return 0
		}
		return steps[len(steps)-1]
	}
	return steps[count-1]
}

func complexReportAutoscalingGroups() []map[string]any {
	return []map[string]any{
		{
			"AutoScalingGroupName": "web-frontend",
			"AutoScalingGroupARN":  fakeASGARN("web-frontend"),
			"DesiredCapacity":      2,
			"MinSize":              1,
			"MaxSize":              4,
			"Instances": []map[string]any{
				{"LifecycleState": "InService", "HealthStatus": "Healthy", "InstanceId": "i-web-1"},
				{"LifecycleState": "Pending", "HealthStatus": "Healthy", "InstanceId": "i-web-2"},
			},
			"Activities": []map[string]any{{"Cause": "None", "StatusCode": "InProgress", "Progress": 62}},
		},
		{
			"AutoScalingGroupName": "api-backend",
			"AutoScalingGroupARN":  fakeASGARN("api-backend"),
			"DesiredCapacity":      1,
			"MinSize":              1,
			"MaxSize":              2,
			"Instances": []map[string]any{
				{"LifecycleState": "InService", "HealthStatus": "Healthy", "InstanceId": "i-api-1"},
			},
			"Activities": []map[string]any{{"Cause": "None", "StatusCode": "Successful", "Progress": 100}},
		},
		{
			"AutoScalingGroupName": "worker-batch",
			"AutoScalingGroupARN":  fakeASGARN("worker-batch"),
			"DesiredCapacity":      2,
			"MinSize":              1,
			"MaxSize":              3,
			"Instances": []map[string]any{
				{"LifecycleState": "Pending", "HealthStatus": "Unhealthy", "InstanceId": "i-worker-1"},
				{"LifecycleState": "Pending", "HealthStatus": "Unhealthy", "InstanceId": "i-worker-2"},
			},
			"Activities": []map[string]any{{"Cause": "None", "StatusCode": "Failed", "Progress": 100}},
		},
	}
}

func describeProgress(scenario string) (status string, percentage int, completedAt any) {
	state := scenarioState(scenario)
	switch scenario {
	case "in-progress":
		if state == "success" {
			return "Successful", 100, "2026-09-19T00:00:00Z"
		}
		return "InProgress", progressPercent(scenario, 30, 55, 75, 90), nil
	case "in-progress-then-success", "terragrunt-multi-stack":
		if state == "success" {
			return "Successful", 100, "2026-09-19T00:00:00Z"
		}
		return "InProgress", progressPercent(scenario, 25, 60, 82, 95), nil
	case "in-progress-then-failed":
		if state == "success" || state == "failed" {
			return "Failed", 100, "2026-09-19T00:00:00Z"
		}
		return "InProgress", progressPercent(scenario, 20, 45, 75, 90), nil
	case "late-success":
		if state == "success" {
			return "Successful", 100, "2026-09-19T00:00:00Z"
		}
		return "InProgress", progressPercent(scenario, 35, 70, 96), nil
	default:
		return "Successful", 100, "2026-09-19T00:00:00Z"
	}
}

func requestedASGName(rest []string) string {
	for i, arg := range rest {
		if arg == "--auto-scaling-group-name" && i+1 < len(rest) {
			return rest[i+1]
		}
	}
	return "fake-asg"
}

func fakeASGARN(name string) string {
	return fmt.Sprintf("arn:aws:autoscaling:us-east-1:123456789012:autoScalingGroup:::%s", name)
}

// mixedMovementResourceIsPending picks a single resource (by ASG name suffix) to remain
// pending indefinitely, so a single await run's report shows both pending and success
// resources at once, without relying on retries or timing.
func mixedMovementResourceIsPending(asgName string) bool {
	return strings.HasSuffix(asgName, "-1")
}

func mixedResourceOutcomeTag(asgName string) string {
	if mixedMovementResourceIsPending(asgName) {
		return "blue"
	}
	return "green"
}

func mixedResourceOutcomeActivity(asgName string) string {
	if mixedMovementResourceIsPending(asgName) {
		return "InProgress"
	}
	return "Successful"
}

func mixedResourceOutcomeRefreshStatus(asgName string) string {
	if mixedMovementResourceIsPending(asgName) {
		return "InProgress"
	}
	return "Successful"
}

func mixedResourceOutcomeProgress(asgName string) int {
	if mixedMovementResourceIsPending(asgName) {
		return 40
	}
	return 100
}

func mixedResourceOutcomeInstance(asgName string) map[string]any {
	if mixedMovementResourceIsPending(asgName) {
		return map[string]any{"LifecycleState": "Pending", "HealthStatus": "Healthy", "InstanceId": "i-mixed"}
	}
	return map[string]any{"LifecycleState": "InService", "HealthStatus": "Healthy", "InstanceId": "i-mixed"}
}

// laggingTagValue simulates AWS not yet reflecting a tag rollover: it returns the stale
// value for the first threshold-1 calls, then the rotated value, to exercise the
// desired-generation grace period against a real race condition rather than only status.
func laggingTagValue(scenario, stale, rotated string, threshold int) string {
	count := incrementScenarioCount(scenario)
	if count < threshold {
		return stale
	}
	return rotated
}

func applyFakeAWSDelay() {
	delay, err := time.ParseDuration(strings.TrimSpace(os.Getenv("FAKE_AWS_DELAY")))
	if err == nil && delay > 0 {
		time.Sleep(delay)
	}
}

func main() {
	applyFakeAWSDelay()
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "fake aws: no command supplied")
		os.Exit(2)
	}

	args := os.Args[1:]
	if len(args) >= 2 && args[0] == "--profile" {
		args = args[2:]
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "fake aws: missing AWS service")
		os.Exit(2)
	}
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println(fakeAWSVersion())
		return
	}
	if len(args) == 2 && args[0] == "sts" && args[1] == "get-caller-identity" {
		fmt.Println(fakeAWSCallerIdentity())
		return
	}

	service := args[0]
	rest := args[1:]

	scenario := scenarioName(os.Getenv("FAKE_AWS_SCENARIO"))

	switch service {
	case "autoscaling":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "fake aws: missing autoscaling subcommand")
			os.Exit(2)
		}
		subcommand := rest[0]
		asgName := requestedASGName(rest)
		switch subcommand {
		case "describe-auto-scaling-groups":
			if scenario == "not-started" {
				response := asgResponse(asgName, "aa", "5", "Successful", 100)
				if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(3)
				}
				return
			}
			if scenario == "not-started-then-success" {
				tag, launchTemplateVersion := "aa", "5"
				if scenarioCount(scenario) >= 3 {
					tag, launchTemplateVersion = "bb", "6"
				}
				response := asgResponse(asgName, tag, launchTemplateVersion, "Successful", 100)
				if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(3)
				}
				return
			}
			if scenario == "mixed-resource-outcomes" {
				response := map[string]any{
					"AutoScalingGroups": []map[string]any{{
						"AutoScalingGroupName": asgName,
						"AutoScalingGroupARN":  fakeASGARN(asgName),
						"Tags": []map[string]any{
							{"Key": "rotation", "Value": mixedResourceOutcomeTag(asgName)},
						},
						"Instances": []map[string]any{
							mixedResourceOutcomeInstance(asgName),
						},
						"Activities": []map[string]any{{"Cause": "None", "StatusCode": mixedResourceOutcomeActivity(asgName), "Progress": mixedResourceOutcomeProgress(asgName)}},
					}},
				}
				if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(3)
				}
				return
			}
			if scenario == "tag-rotation-lagging" {
				response := map[string]any{
					"AutoScalingGroups": []map[string]any{{
						"AutoScalingGroupName": asgName,
						"AutoScalingGroupARN":  fakeASGARN(asgName),
						"Tags": []map[string]any{
							{"Key": "rotation", "Value": laggingTagValue(scenario, "v1", "v2", 3)},
						},
						"Instances": []map[string]any{
							{"LifecycleState": "InService", "HealthStatus": "Healthy", "InstanceId": "i-1234567890"},
						},
						"Activities": []map[string]any{{"Cause": "None", "StatusCode": "Successful", "Progress": 100}},
					}},
				}
				if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(3)
				}
				return
			}
			if scenario == "complex-report" {
				response := map[string]any{"AutoScalingGroups": complexReportAutoscalingGroups()}
				if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(3)
				}
				return
			}
			progressStatus, progressPct, progressCompletedAt := describeProgress(scenario)
			response := map[string]any{
				"AutoScalingGroups": []map[string]any{{
					"AutoScalingGroupName": asgName,
					"AutoScalingGroupARN":  fakeASGARN(asgName),
					"DesiredCapacity":      2,
					"MinSize":              1,
					"MaxSize":              3,
					"Instances": []map[string]any{
						{"LifecycleState": "InService", "HealthStatus": "Healthy", "InstanceId": "i-1234567890"},
						{"LifecycleState": "InService", "HealthStatus": "Healthy", "InstanceId": "i-0987654321"},
					},
					"Activities": []map[string]any{{"Cause": "None", "StatusCode": progressStatus, "Progress": progressPct}},
				}},
			}
			if progressStatus == "InProgress" {
				response["AutoScalingGroups"] = []map[string]any{{
					"AutoScalingGroupName": asgName,
					"AutoScalingGroupARN":  fakeASGARN(asgName),
					"DesiredCapacity":      2,
					"MinSize":              1,
					"MaxSize":              3,
					"Instances": []map[string]any{
						{"LifecycleState": "Pending", "HealthStatus": "Healthy", "InstanceId": "i-1234567890"},
						{"LifecycleState": "Pending", "HealthStatus": "Healthy", "InstanceId": "i-0987654321"},
					},
					"Activities": []map[string]any{{"Cause": "None", "StatusCode": progressStatus, "Progress": progressPct}},
				}}
			}
			if scenario == "failed" {
				response["AutoScalingGroups"] = []map[string]any{{
					"AutoScalingGroupName": asgName,
					"AutoScalingGroupARN":  fakeASGARN(asgName),
					"DesiredCapacity":      2,
					"MinSize":              1,
					"MaxSize":              3,
					"Instances": []map[string]any{
						{"LifecycleState": "Pending", "HealthStatus": "Unhealthy", "InstanceId": "i-1234567890"},
						{"LifecycleState": "Pending", "HealthStatus": "Unhealthy", "InstanceId": "i-0987654321"},
					},
					"Activities": []map[string]any{{"Cause": "None", "StatusCode": "Failed", "Progress": 100}},
				}}
			}
			_ = progressCompletedAt
			if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(3)
			}
			return
		case "describe-instance-refreshes":
			if scenario == "not-started" {
				response := map[string]any{"InstanceRefreshes": []map[string]any{}}
				if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(3)
				}
				return
			}
			if scenario == "not-started-then-success" {
				switch count := incrementScenarioCount(scenario); {
				case count == 1:
					response := map[string]any{"InstanceRefreshes": []map[string]any{}}
					if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
						fmt.Fprintln(os.Stderr, err)
						os.Exit(3)
					}
				case count == 2:
					response := refreshResponse(asgName, "InProgress", 45, nil)
					if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
						fmt.Fprintln(os.Stderr, err)
						os.Exit(3)
					}
				default:
					response := refreshResponse(asgName, "Successful", 100, "2026-09-19T00:00:00Z")
					if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
						fmt.Fprintln(os.Stderr, err)
						os.Exit(3)
					}
				}
				return
			}
			if scenario == "mixed-resource-outcomes" {
				response := map[string]any{
					"InstanceRefreshes": []map[string]any{{
						"AutoScalingGroupName": asgName,
						"InstanceRefreshId":    "ir-mixed",
						"Status":               mixedResourceOutcomeRefreshStatus(asgName),
						"PercentageComplete":   mixedResourceOutcomeProgress(asgName),
						"CompletedAt":          "2026-09-20T00:00:00Z",
					}},
				}
				if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(3)
				}
				return
			}
			if scenario == "tag-rotation-lagging" {
				response := map[string]any{
					"InstanceRefreshes": []map[string]any{{
						"AutoScalingGroupName": asgName,
						"InstanceRefreshId":    "ir-tag-rotation",
						"Status":               "Successful",
						"PercentageComplete":   100,
						"CompletedAt":          "2026-09-20T00:00:00Z",
					}},
				}
				if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(3)
				}
				return
			}
			if scenario == "complex-report" {
				response := map[string]any{
					"InstanceRefreshes": []map[string]any{
						{"AutoScalingGroupName": "web-frontend", "InstanceRefreshId": "ir-web", "Status": "InProgress", "PercentageComplete": 62},
						{"AutoScalingGroupName": "api-backend", "InstanceRefreshId": "ir-api", "Status": "Successful", "PercentageComplete": 100},
						{"AutoScalingGroupName": "worker-batch", "InstanceRefreshId": "ir-worker", "Status": "Failed", "PercentageComplete": 100},
					},
				}
				if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(3)
				}
				return
			}
			status, percentage, completedAt := describeProgress(scenario)
			response := map[string]any{
				"InstanceRefreshes": []map[string]any{{
					"AutoScalingGroupName": asgName,
					"InstanceRefreshId":    "ir-123",
					"Status":               status,
					"PercentageComplete":   percentage,
					"CompletedAt":          completedAt,
				}},
			}
			if scenario == "failed" {
				response["InstanceRefreshes"] = []map[string]any{{
					"AutoScalingGroupName": asgName,
					"InstanceRefreshId":    "ir-123",
					"Status":               "Failed",
					"PercentageComplete":   100,
					"CompletedAt":          "2026-09-19T00:00:00Z",
				}}
			}
			if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(3)
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "fake aws: unsupported autoscaling subcommand: %s\n", subcommand)
			os.Exit(3)
		}
	default:
		fmt.Fprintf(os.Stderr, "fake aws: unsupported service: %s\n", service)
		os.Exit(3)
	}
}
