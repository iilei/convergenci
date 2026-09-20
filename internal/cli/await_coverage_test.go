package cli

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/iilei/convergenci-cli/internal/scan"
)

func TestPreferAwaitBaseline(t *testing.T) {
	baselineItem := scan.ContractItem{
		Address: "asg.one",
		Status:  "pending",
		Observation: scan.Observation{
			Strategy: "instance_refresh",
		},
	}

	tests := []struct {
		name     string
		contract scan.Contract
		baseline scan.Contract
		want     scan.Contract
	}{
		{
			name:     "empty contract uses baseline",
			contract: scan.Contract{},
			baseline: scan.Contract{Resources: []scan.ContractItem{baselineItem}},
			want:     scan.Contract{Resources: []scan.ContractItem{baselineItem}},
		},
		{
			name:     "empty baseline keeps contract",
			contract: scan.Contract{Resources: []scan.ContractItem{baselineItem}},
			baseline: scan.Contract{},
			want:     scan.Contract{Resources: []scan.ContractItem{baselineItem}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := preferAwaitBaseline(test.contract, test.baseline)
			if len(got.Resources) != len(test.want.Resources) {
				t.Fatalf("resources = %#v, want %#v", got.Resources, test.want.Resources)
			}
			if len(got.Resources) > 0 {
				gotResource := got.Resources[0]
				wantResource := test.want.Resources[0]
				if gotResource.Address != wantResource.Address || gotResource.Status != wantResource.Status || gotResource.Observation.Strategy != wantResource.Observation.Strategy {
					t.Fatalf("resource = %#v, want %#v", gotResource, wantResource)
				}
			}
		})
	}
}

func TestPreferAwaitBaselineMergesMatchingMetadata(t *testing.T) {
	contract := scan.Contract{Resources: []scan.ContractItem{
		{Address: "asg.one", Observation: scan.Observation{}},
		{Address: "asg.two", Status: "already-pending", Observation: scan.Observation{Strategy: "existing"}},
	}}
	baseline := scan.Contract{Resources: []scan.ContractItem{
		{Address: "asg.one", Status: "baseline-pending", Observation: scan.Observation{Strategy: "instance_refresh"}},
		{Address: "asg.two", Status: "baseline-status", Observation: scan.Observation{Strategy: "baseline"}},
	}}

	got := preferAwaitBaseline(contract, baseline)
	if got.Resources[0].Status != "baseline-pending" || got.Resources[0].Observation.Strategy != "instance_refresh" {
		t.Fatalf("merged first resource = %#v, want baseline status and strategy", got.Resources[0])
	}
	if got.Resources[1].Status != "already-pending" || got.Resources[1].Observation.Strategy != "existing" {
		t.Fatalf("merged second resource = %#v, want existing metadata preserved", got.Resources[1])
	}
}

func TestRelativePathForCurrentWorkingDir(t *testing.T) {
	absPath, err := filepath.Abs("contract.json")
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "empty", path: "", want: "."},
		{name: "absolute", path: absPath, want: absPath},
		{name: "relative", path: filepath.Join("nested", "..", "contract.json"), want: "contract.json"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := relativePathForCurrentWorkingDir(test.path); got != test.want {
				t.Fatalf("relativePathForCurrentWorkingDir(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}

func TestEnvDuration(t *testing.T) {
	const name = "CONVERGENCI_TEST_DURATION"
	fallback := 7 * time.Second
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{name: "unset", value: "", want: fallback},
		{name: "whitespace", value: "  ", want: fallback},
		{name: "invalid", value: "not-a-duration", want: fallback},
		{name: "valid", value: "250ms", want: 250 * time.Millisecond},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(name, test.value)
			if got := envDuration(name, fallback); got != test.want {
				t.Fatalf("envDuration(%q, %s) = %s, want %s", name, fallback, got, test.want)
			}
		})
	}
}

func TestExitCodeErrorFormatting(t *testing.T) {
	var nilError *ExitCodeError
	if got := nilError.Error(); got != "" {
		t.Fatalf("nil ExitCodeError.Error() = %q, want empty string", got)
	}
	if got := (&ExitCodeError{Code: codeTimeout}).Error(); got != "exit code 230" {
		t.Fatalf("empty-message Error() = %q, want %q", got, "exit code 230")
	}
	if got := (&ExitCodeError{Code: codeIOError, Message: "unable to read"}).Error(); got != "unable to read (exit code 220)" {
		t.Fatalf("message Error() = %q, want %q", got, "unable to read (exit code 220)")
	}
}
