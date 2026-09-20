package cli

import (
	"strings"
	"testing"

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
			got, arn, err := observeAWSResource(item, cfg)
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
			got, arn, err := observeAWSResource(item, cfg)
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

func TestObserveAWSResourceFallsBackToContractStatusOnAWSFailure(t *testing.T) {
	item := scan.ContractItem{
		Address: "aws_autoscaling_group.example",
		Status:  "converged",
		Observation: scan.Observation{
			Strategy: "instance_refresh",
		},
	}
	got, arn, err := observeAWSResource(item, awscmd.Config{BinaryPath: "/missing/aws"})
	if err != nil {
		t.Fatalf("observeAWSResource returned error: %v", err)
	}
	if got != "converged" || arn != "" {
		t.Fatalf("observeAWSResource = (%q, %q), want (converged, empty ARN)", got, arn)
	}
}
