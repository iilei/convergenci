package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iilei/convergenci-cli/internal/awscmd"
	"github.com/iilei/convergenci-cli/internal/scan"
)

func TestObserveAWSResourceWithoutStrategyUsesContractStatus(t *testing.T) {
	cfg := awscmd.Config{BinaryPath: fakeAWSCLIPath(t)}
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "missing status", want: "pending"},
		{name: "pending status", status: "pending", want: "pending"},
		{name: "converged status", status: " CONVERGED ", want: "converged"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := scan.ContractItem{Address: "asg.example", Status: test.status}
			got, arn, err := observeAWSResource(&item, cfg)
			if err != nil {
				t.Fatalf("observeAWSResource returned error: %v", err)
			}
			if got != test.want || arn != "" {
				t.Fatalf("observeAWSResource = (%q, %q), want (%q, empty ARN)", got, arn, test.want)
			}
		})
	}
}

func TestObserveAWSResourceFakeScenarios(t *testing.T) {
	cfg := awscmd.Config{BinaryPath: fakeAWSCLIPath(t)}
	tests := []struct {
		name     string
		scenario string
		want     string
	}{
		{name: "successful refresh", scenario: "success", want: "converged"},
		{name: "in-progress refresh", scenario: "in-progress", want: "pending"},
		{name: "failed refresh", scenario: "failed", want: "failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("FAKE_AWS_SCENARIO", test.scenario)
			item := scan.ContractItem{
				Address: "aws_autoscaling_group.example",
				Name:    "example-asg",
				Observation: scan.Observation{
					Strategy: "instance_refresh",
				},
			}
			got, arn, err := observeAWSResource(&item, cfg)
			if err != nil {
				t.Fatalf("observeAWSResource returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("status = %q, want %q", got, test.want)
			}
			if test.want != "pending" && !strings.Contains(arn, "example-asg") {
				t.Fatalf("ARN = %q, want example-asg", arn)
			}
		})
	}
}

func TestDesiredGenerationUnmet(t *testing.T) {
	group := autoScalingGroupObservation{
		Tags: []awsTag{{Key: "rotation", Value: "green"}},
		LaunchTemplate: &struct {
			Version string `json:"Version"`
		}{Version: "4"},
	}

	tests := []struct {
		name string
		item scan.ContractItem
		want bool
	}{
		{
			name: "matching tag and launch template",
			item: scan.ContractItem{DesiredGeneration: []scan.GenerationRequirement{
				{Type: "tag", Key: "rotation", Value: "green"},
				{Type: "launch_template", Value: "4"},
			}},
			want: false,
		},
		{
			name: "mismatched tag",
			item: scan.ContractItem{DesiredGeneration: []scan.GenerationRequirement{
				{Type: "tag", Key: "rotation", Value: "blue"},
			}},
			want: true,
		},
		{
			name: "mismatched launch template version",
			item: scan.ContractItem{DesiredGeneration: []scan.GenerationRequirement{
				{Type: "launch_template", Value: "5"},
			}},
			want: true,
		},
		{
			name: "unverifiable tag grants grace period",
			item: scan.ContractItem{DesiredGeneration: []scan.GenerationRequirement{
				{Type: "tag", Key: "missing-tag", Value: "green"},
			}},
			want: false,
		},
		{
			name: "unknown requirement type is trusted",
			item: scan.ContractItem{DesiredGeneration: []scan.GenerationRequirement{
				{Type: "indicator", Key: "custom", Value: "anything"},
			}},
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := desiredGenerationUnmet(&test.item, &group); got != test.want {
				t.Fatalf("desiredGenerationUnmet() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestObserveAWSResourceGrantsGracePeriodUntilTagMatches(t *testing.T) {
	awsPath := filepath.Join(t.TempDir(), "aws")
	stub := `#!/bin/sh
if [ "$2" = "describe-instance-refreshes" ]; then
    printf '%s\n' '{"InstanceRefreshes":[]}'
    exit 0
fi
printf '%s\n' "$FAKE_AWS_GROUP_RESPONSE"
`
	if err := os.WriteFile(awsPath, []byte(stub), 0o755); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	item := scan.ContractItem{
		Address: "aws_autoscaling_group.example",
		Name:    "example-asg",
		DesiredGeneration: []scan.GenerationRequirement{
			{Type: "tag", Key: "rotation", Value: "green"},
		},
		Observation: scan.Observation{Strategy: "instance_refresh"},
	}
	cfg := awscmd.Config{BinaryPath: awsPath}

	t.Setenv(
		"FAKE_AWS_GROUP_RESPONSE",
		`{"AutoScalingGroups":[{"AutoScalingGroupARN":"arn:example","Tags":[{"Key":"rotation","Value":"blue"}],"Activities":[{"StatusCode":"Successful","Progress":100}],"Instances":[{"LifecycleState":"InService"}]}]}`,
	)
	status, _, err := observeAWSResource(&item, cfg)
	if err != nil {
		t.Fatalf("observeAWSResource returned error: %v", err)
	}
	if status != "pending" {
		t.Fatalf("status = %q, want pending while rotation tag has not rolled over", status)
	}

	SetDebug(true)
	t.Cleanup(func() { SetDebug(false) })
	if err := ConfigureDebugFormat(
		"{{ .Event }} {{ .RequirementType }} {{ .RequirementKey }} value={{ .Wanted }}",
	); err != nil {
		t.Fatalf("ConfigureDebugFormat returned error: %v", err)
	}
	oldStderr := os.Stderr
	readPipe, writePipe, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("os.Pipe returned error: %v", pipeErr)
	}
	os.Stderr = writePipe

	t.Setenv(
		"FAKE_AWS_GROUP_RESPONSE",
		`{"AutoScalingGroups":[{"AutoScalingGroupARN":"arn:example","Tags":[{"Key":"rotation","Value":"green"}],"Activities":[{"StatusCode":"Successful","Progress":100}],"Instances":[{"LifecycleState":"InService"}]}]}`,
	)
	status, _, err = observeAWSResource(&item, cfg)
	_ = writePipe.Close()
	os.Stderr = oldStderr
	output, readErr := io.ReadAll(readPipe)
	_ = readPipe.Close()
	if readErr != nil {
		t.Fatalf("io.ReadAll returned error: %v", readErr)
	}
	if err != nil {
		t.Fatalf("observeAWSResource returned error: %v", err)
	}
	if status != "converged" {
		t.Fatalf("status = %q, want converged once rotation tag matches", status)
	}
	if !strings.Contains(string(output), "desired_generation_met tag rotation value=green") {
		t.Fatalf("debug output = %q, want desired_generation_met event once tag matches", output)
	}
}

func TestObserveAWSResourceFallsBackToContractStatusOnAWSFailure(t *testing.T) {
	item := scan.ContractItem{
		Address: "aws_autoscaling_group.example",
		Status:  "converged",
		Observation: scan.Observation{
			Strategy: "instance_refresh",
		},
	}
	got, arn, err := observeAWSResource(&item, awscmd.Config{BinaryPath: "/missing/aws"})
	if err != nil {
		t.Fatalf("observeAWSResource returned error: %v", err)
	}
	if got != "converged" || arn != "" {
		t.Fatalf("observeAWSResource = (%q, %q), want (converged, empty ARN)", got, arn)
	}
}

func TestPollAwaitLogsSatisfiedRetry(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "refresh-count")
	awsPath := filepath.Join(t.TempDir(), "aws")
	stub := `#!/bin/sh
state="$FAKE_AWS_TEST_STATE"
if [ "$2" = "describe-instance-refreshes" ]; then
    count=0
    if [ -f "$state" ]; then count=$(cat "$state"); fi
    count=$((count + 1))
    printf '%s' "$count" > "$state"
    if [ "$count" -eq 1 ]; then
        printf '%s\n' '{"InstanceRefreshes":[{"Status":"InProgress","PercentageComplete":50}]}'
    else
        printf '%s\n' '{"InstanceRefreshes":[{"Status":"Successful","PercentageComplete":100}]}'
    fi
    exit 0
fi
printf '%s\n' '{"AutoScalingGroups":[{"AutoScalingGroupARN":"arn:example","Activities":[{"StatusCode":"Successful","Progress":100}],"Instances":[{"LifecycleState":"InService"}]}]}'
`
	if err := os.WriteFile(awsPath, []byte(stub), 0o755); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	t.Setenv("FAKE_AWS_TEST_STATE", statePath)
	SetDebug(true)
	t.Cleanup(func() { SetDebug(false) })
	if err := ConfigureDebugFormat(
		"{{ .Event }} {{ .RetryIteration }}/{{ .RetryLimit }} {{ .Status }} resources={{ join \",\" .ResourceNames }} converged={{ join \",\" .ConvergedResources }}",
	); err != nil {
		t.Fatalf("ConfigureDebugFormat returned error: %v", err)
	}

	oldStderr := os.Stderr
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe returned error: %v", err)
	}
	os.Stderr = writePipe
	t.Cleanup(func() { os.Stderr = oldStderr })

	contract := scan.Contract{Resources: []scan.ContractItem{{
		Address: "aws_autoscaling_group.example",
		Name:    "example-asg",
		Observation: scan.Observation{
			Strategy: "instance_refresh",
		},
	}}}
	result, err := pollAwait(contract, time.Second, time.Millisecond, awscmd.Config{BinaryPath: awsPath})
	_ = writePipe.Close()
	os.Stderr = oldStderr
	output, readErr := io.ReadAll(readPipe)
	_ = readPipe.Close()
	if readErr != nil {
		t.Fatalf("io.ReadAll returned error: %v", readErr)
	}
	if err != nil || result.Status != "converged" {
		t.Fatalf("pollAwait returned (%#v, %v), want converged result", result, err)
	}
	debugOutput := string(output)
	startedIndex := strings.Index(debugOutput, "retry_started 1/")
	satisfiedIndex := strings.Index(debugOutput, "retry_satisfied 2/")
	if startedIndex < 0 || satisfiedIndex < 0 || startedIndex > satisfiedIndex ||
		!strings.Contains(debugOutput, "resources=example-asg") ||
		!strings.Contains(debugOutput, "converged=example-asg") {
		t.Fatalf("debug output = %q, want retry lifecycle events with resource names", output)
	}
}
