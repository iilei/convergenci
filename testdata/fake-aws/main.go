package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	case "failed":
		return "failed"
	case "in-progress":
		return "in-progress"
	case "in-progress-then-success":
		return "in-progress-then-success"
	case "in-progress-then-failed":
		return "in-progress-then-failed"
	case "late-success":
		return "late-success"
	case "complex-report":
		return "complex-report"
	default:
		return "success"
	}
}

func scenarioState(scenario string) string {
	switch scenario {
	case "in-progress":
		return delayedOutcome("in-progress", 6)
	case "in-progress-then-success":
		return delayedOutcome("in-progress-then-success", 2)
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
	return filepath.Join(os.TempDir(), "convergenci-fake-aws-state-"+name)
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
	switch scenario {
	case "in-progress":
		if scenarioState(scenario) == "success" {
			return "Successful", 100, "2026-09-19T00:00:00Z"
		}
		return "InProgress", progressPercent(scenario, 30, 55, 75, 90), nil
	case "in-progress-then-success":
		if scenarioState(scenario) == "success" {
			return "Successful", 100, "2026-09-19T00:00:00Z"
		}
		return "InProgress", progressPercent(scenario, 25, 60, 82, 95), nil
	case "in-progress-then-failed":
		if scenarioState(scenario) == "success" {
			return "Failed", 100, "2026-09-19T00:00:00Z"
		}
		if scenarioState(scenario) == "failed" {
			return "Failed", 100, "2026-09-19T00:00:00Z"
		}
		return "InProgress", progressPercent(scenario, 20, 45, 75, 90), nil
	case "late-success":
		if scenarioState(scenario) == "success" {
			return "Successful", 100, "2026-09-19T00:00:00Z"
		}
		return "InProgress", progressPercent(scenario, 35, 70, 96), nil
	default:
		return "Successful", 100, "2026-09-19T00:00:00Z"
	}
}

func main() {
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
		switch subcommand {
		case "describe-auto-scaling-groups":
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
					"AutoScalingGroupName": "fake-asg",
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
					"AutoScalingGroupName": "fake-asg",
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
					"AutoScalingGroupName": "fake-asg",
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
					"AutoScalingGroupName": "fake-asg",
					"InstanceRefreshId":    "ir-123",
					"Status":               status,
					"PercentageComplete":   percentage,
					"CompletedAt":          completedAt,
				}},
			}
			if scenario == "failed" {
				response["InstanceRefreshes"] = []map[string]any{{
					"AutoScalingGroupName": "fake-asg",
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
