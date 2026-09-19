package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func resetScenarioState(scenario string) {
	_ = os.Remove(scenarioStatePath(scenario))
}

func TestFakeAWSVersionOutput(t *testing.T) {
	if got := fakeAWSVersion(); got == "" {
		t.Fatal("fakeAWSVersion returned empty output")
	}
}

func TestFakeAWSCallerIdentityOutput(t *testing.T) {
	got := fakeAWSCallerIdentity()
	for _, want := range []string{"Account", "Arn", "UserId"} {
		if !strings.Contains(got, want) {
			t.Fatalf("fakeAWSCallerIdentity = %q, want it to contain %q", got, want)
		}
	}
}

func TestSupportedScenarioNames(t *testing.T) {
	for _, scenario := range []string{"success", "failed", "in-progress", "in-progress-then-success", "in-progress-then-failed", "complex-report"} {
		if got := scenarioName(scenario); got == "" {
			t.Fatalf("scenarioName(%q) returned empty string", scenario)
		}
	}
}

func TestComplexReportScenarioReturnsMultipleASGs(t *testing.T) {
	response := map[string]any{"AutoScalingGroups": complexReportAutoscalingGroups()}
	groups, ok := response["AutoScalingGroups"].([]map[string]any)
	if !ok {
		t.Fatal("AutoScalingGroups should be a slice of maps")
	}
	if len(groups) != 3 {
		t.Fatalf("len(groups) = %d, want 3", len(groups))
	}
	seen := map[string]bool{}
	for _, group := range groups {
		seen[group["AutoScalingGroupName"].(string)] = true
	}
	for _, name := range []string{"web-frontend", "api-backend", "worker-batch"} {
		if !seen[name] {
			t.Fatalf("missing expected ASG %q in complex report", name)
		}
	}
}

func TestScenarioNameDefaultsToSuccess(t *testing.T) {
	if got := scenarioName(""); got != "success" {
		t.Fatalf("scenarioName(\"\") = %q, want success", got)
	}
}

func TestDescribeProgressRisesAcrossPolls(t *testing.T) {
	resetScenarioState("in-progress")
	defer resetScenarioState("in-progress")

	for _, want := range []int{30, 55, 75, 90, 90} {
		status, got, completed := describeProgress("in-progress")
		if status != "InProgress" {
			t.Fatalf("describeProgress status = %q, want InProgress", status)
		}
		if got != want {
			t.Fatalf("describeProgress(%d) = %d, want %d", len([]int{30, 55, 75, 90, 90}), got, want)
		}
		if completed != nil {
			t.Fatalf("describeProgress completedAt = %v, want nil", completed)
		}
	}
}

func TestProgressPercentRisesAcrossPolls(t *testing.T) {
	resetScenarioState("in-progress")
	defer resetScenarioState("in-progress")

	for _, want := range []int{30, 55, 75, 90} {
		incrementScenarioCount("in-progress")
		if got := progressPercent("in-progress", 30, 55, 75, 90); got != want {
			t.Fatalf("progressPercent(%d) = %d, want %d", len([]int{30, 55, 75, 90}), got, want)
		}
	}
}

func TestDelayedOutcomeUsesScenarioScopedState(t *testing.T) {
	resetScenarioState("late-success")
	defer resetScenarioState("late-success")

	if got := delayedOutcome("late-success", 3); got != "in-progress" {
		t.Fatalf("first delayedOutcome = %q, want in-progress", got)
	}
	if got := delayedOutcome("late-success", 3); got != "in-progress" {
		t.Fatalf("second delayedOutcome = %q, want in-progress", got)
	}
	if got := delayedOutcome("late-success", 3); got != "success" {
		t.Fatalf("third delayedOutcome = %q, want success", got)
	}

	statePath := scenarioStatePath("late-success")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) returned error: %v", statePath, err)
	}
	if got, want := strconv.Itoa(3), string(data); got != want {
		t.Fatalf("state count = %q, want %q", got, want)
	}
}

func TestTransitionalScenariosStayInProgressLongerThanSinglePoll(t *testing.T) {
	for _, tc := range []struct {
		scenario  string
		threshold int
	}{
		{scenario: "in-progress-then-success", threshold: 3},
		{scenario: "in-progress-then-failed", threshold: 3},
	} {
		resetScenarioState(tc.scenario)
		defer resetScenarioState(tc.scenario)

		if got := delayedOutcome(tc.scenario, tc.threshold); got != "in-progress" {
			t.Fatalf("first %s delayedOutcome = %q, want in-progress", tc.scenario, got)
		}
		if got := delayedOutcome(tc.scenario, tc.threshold); got != "in-progress" {
			t.Fatalf("second %s delayedOutcome = %q, want in-progress", tc.scenario, got)
		}
		if got := delayedOutcome(tc.scenario, tc.threshold); got != "success" {
			t.Fatalf("third %s delayedOutcome = %q, want success", tc.scenario, got)
		}
	}
}
