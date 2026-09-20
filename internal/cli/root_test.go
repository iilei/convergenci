package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iilei/convergenci-cli/internal/awscmd"
	"github.com/iilei/convergenci-cli/internal/scan"
)

func TestRootCommandVersionFlag(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"--version"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	want := "1.2.3 (commit: abc123, built at: 2026-09-19)\n"
	if got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestRootCommandVersionSubcommand(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"version"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	want := "1.2.3 (commit: abc123, built at: 2026-09-19)\n"
	if got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestRootCommandHelpIncludesSubcommandDocs(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"--help"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"scan",
		"await",
		"convergenci scan <tfplan.json>",
		"convergenci await <convergence.json>",
	} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("help output = %q, want it to contain %q", got, want)
		}
	}
}

func TestScanCommandHelpIncludesPlaceholders(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"scan", "--help"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Usage: convergenci scan <tfplan.json>",
		"--output-json",
		"--jsonlines",
		"--aws-cli-path",
	} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("scan help output = %q, want it to contain %q", got, want)
		}
	}
}

func TestScanCommandDefaultOutputJSONPath(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"scan"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	want := "output-json: .convergence.json\n"
	if got != want {
		t.Fatalf("scan output = %q, want %q", got, want)
	}
}

func TestScanCommandJSONLinesDefaultOutputPath(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"scan", "--jsonlines"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	want := "output-json: .convergence.jsonlines\njsonlines: true\n"
	if got != want {
		t.Fatalf("scan output = %q, want %q", got, want)
	}
}

func TestScanCommandUsesPlanDerivedDefaultOutputPath(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd returned error: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("os.Chdir returned error: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	planPath := filepath.Join(oldWD, "..", "..", "stubs", "asg-default-rotation.json")
	if err := cmd.ExecuteArgs([]string{"scan", planPath}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	want := "output-json: asg-default-rotation.convergence.json\n"
	if got != want {
		t.Fatalf("scan output = %q, want %q", got, want)
	}
}

func TestScanCommandExplicitOutputJSONPath(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"scan", "--output-json", "custom/out.json"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	want := "output-json: custom/out.json\n"
	if got != want {
		t.Fatalf("scan output = %q, want %q", got, want)
	}
}

func TestAwaitCommandHelpIncludesPlaceholders(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"await", "--help"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Usage: convergenci await <convergence.json>",
		"--timeout",
		"--interval",
		"--aws-cli-path",
	} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("await help output = %q, want it to contain %q", got, want)
		}
	}
}

func TestScanCommandWritesConvergenceContract(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "convergence.json")
	planPath := filepath.Join("..", "..", "stubs", "asg-default-rotation.json")

	if err := cmd.ExecuteArgs([]string{"scan", "--output-json", outputPath, planPath}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		"\"schema_version\": 1",
		"module.app.aws_autoscaling_group.main",
		"\"rotation\": \"bb\"",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("contract output = %q, want it to contain %q", got, want)
		}
	}
}

func TestWriteScanArtifactRejectsOverwrite(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "existing.json")
	if err := os.WriteFile(outputPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	contract := scan.Contract{Resources: []scan.ContractItem{{Address: "module.app.aws_autoscaling_group.main", Kind: "aws_asg"}}}
	if err := writeScanArtifact(outputPath, contract, false, false); err == nil {
		t.Fatal("writeScanArtifact accepted an existing file and should have rejected it")
	}
}

func TestWriteScanArtifactAllowsAbsolutePath(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "abs-output.json")
	contract := scan.Contract{Resources: []scan.ContractItem{{Address: "module.app.aws_autoscaling_group.main", Kind: "aws_asg"}}}
	if err := writeScanArtifact(outputPath, contract, false, false); err != nil {
		t.Fatalf("writeScanArtifact rejected an absolute path: %v", err)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("os.Stat returned error after absolute-path write: %v", err)
	}
}

func TestWriteScanArtifactRejectsPathTraversal(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd returned error: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("os.Chdir returned error: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	contract := scan.Contract{Resources: []scan.ContractItem{{Address: "module.app.aws_autoscaling_group.main", Kind: "aws_asg"}}}
	if err := writeScanArtifact(filepath.Join("..", "escape.json"), contract, false, false); err == nil {
		t.Fatal("writeScanArtifact accepted a path traversal attempt and should have rejected it")
	}
}

func TestScanCommandAWSCLIPathFlag(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"scan", "--aws-cli-path", "/custom/bin/aws"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"output-json: .convergence.json\n",
		"aws-cli-path: /custom/bin/aws\n",
	} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("scan output = %q, want it to contain %q", got, want)
		}
	}
}

func TestAwaitCommandAWSCLIPathFlag(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"await", "--aws-cli-path", "/custom/bin/aws"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	want := "aws-cli-path: /custom/bin/aws\n"
	if got != want {
		t.Fatalf("await output = %q, want %q", got, want)
	}
}

func TestAwaitCommandFlagAfterPositionalPath(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	t.Setenv("FAKE_AWS_SCENARIO", "success")
	tempDir := t.TempDir()
	contractPath := filepath.Join(tempDir, "contract.json")
	contract := scan.Contract{Resources: []scan.ContractItem{{Address: "module.app.aws_autoscaling_group.main", Kind: "aws_asg", Name: "app-asg", DesiredGeneration: map[string]any{"rotation": "v2"}, Observation: scan.Observation{Strategy: "instance_refresh"}}}}
	payload, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	if err := os.WriteFile(contractPath, payload, 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	if err := cmd.ExecuteArgs([]string{"await", contractPath, "--aws-cli-path", filepath.Join("..", "..", "testdata", "fake-aws-bin", "aws")}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	if got := out.String(); !strings.Contains(got, "aws-cli-path: ") {
		t.Fatalf("await output = %q, want it to contain aws-cli-path", got)
	}
	reportData, err := os.ReadFile(filepath.Join(tempDir, "contract.convergence-report.json"))
	if err != nil {
		t.Fatalf("ReadFile report returned error: %v", err)
	}
	if got, want := string(reportData), "arn:aws:autoscaling:us-east-1:123456789012:autoScalingGroup:::app-asg"; !strings.Contains(got, want) {
		t.Fatalf("await report = %q, want it to contain ARN %q", got, want)
	}
}

func TestAwaitCommandUsesEnvDefaultTimeoutAndInterval(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	t.Setenv("CONVERGENCI_AWAIT_TIMEOUT", "250ms")
	t.Setenv("CONVERGENCI_AWAIT_INTERVAL", "50ms")
	t.Setenv("FAKE_AWS_SCENARIO", "success")

	tempDir := t.TempDir()
	contractPath := filepath.Join(tempDir, "contract.json")
	contract := scan.Contract{Resources: []scan.ContractItem{{Address: "module.app.aws_autoscaling_group.main", Kind: "aws_asg", Status: "converged", DesiredGeneration: map[string]any{"rotation": "v2"}, Observation: scan.Observation{Strategy: "instance_refresh"}}}}
	payload, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	if err := os.WriteFile(contractPath, payload, 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	if err := cmd.ExecuteArgs([]string{"await", "--aws-cli-path", filepath.Join("..", "..", "testdata", "fake-aws-bin", "aws"), contractPath}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{"timeout: 250ms\n", "interval: 50ms\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("await output = %q, want it to contain %q", got, want)
		}
	}
}

func TestAwaitSummaryReportCountsConvergedResources(t *testing.T) {
	contract := scan.Contract{Resources: []scan.ContractItem{{Address: "module.app.aws_autoscaling_group.main", Kind: "aws_asg", DesiredGeneration: map[string]any{"rotation": "v2"}, Observation: scan.Observation{Strategy: "instance_refresh"}}}}

	report := awaitSummaryReport("/tmp/example.convergence.json", "converged", "all expected resources converged", contract)
	if report.ExpectedResources != 1 {
		t.Fatalf("ExpectedResources = %d, want 1", report.ExpectedResources)
	}
	if report.PendingResources != 0 {
		t.Fatalf("PendingResources = %d, want 0 for a converged report", report.PendingResources)
	}
	if report.ConvergedResources != 1 {
		t.Fatalf("ConvergedResources = %d, want 1 for a converged report", report.ConvergedResources)
	}
}

func TestAwaitReportWriting(t *testing.T) {
	contract := scan.Contract{Resources: []scan.ContractItem{{Address: "module.app.aws_autoscaling_group.main", Kind: "aws_asg", DesiredGeneration: map[string]any{"rotation": "v2"}, Observation: scan.Observation{Strategy: "instance_refresh"}}}}

	tempDir := t.TempDir()
	contractPath := filepath.Join(tempDir, "example.convergence.json")
	updated := withObservationFulfilled(contract, "pending")
	if len(updated.Resources) != 1 {
		t.Fatalf("updated resources = %d, want 1", len(updated.Resources))
	}
	if err := writeAwaitReport(contractPath, contract, "pending", false, nil); err != nil {
		t.Fatalf("writeAwaitReport returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "example.convergence-report.json")); err != nil {
		t.Fatalf("summary report not created: %v", err)
	}
}

func TestLoadAwaitBaselineReadsSidecarFile(t *testing.T) {
	tempDir := t.TempDir()
	contractPath := filepath.Join(tempDir, "example.convergence.json")
	baselinePath := baselineFilePath(contractPath)
	baseline := scan.Contract{Resources: []scan.ContractItem{{Address: "asg.web", Kind: "aws_asg", Observation: scan.Observation{Strategy: "instance_refresh"}}}}
	payload, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent returned error: %v", err)
	}
	if err := os.WriteFile(baselinePath, payload, 0o600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	loaded, err := loadAwaitBaseline(contractPath)
	if err != nil {
		t.Fatalf("loadAwaitBaseline returned error: %v", err)
	}
	if len(loaded.Resources) != 1 {
		t.Fatalf("loaded resources = %d, want 1", len(loaded.Resources))
	}
	if loaded.Resources[0].Address != "asg.web" {
		t.Fatalf("loaded resource address = %q, want %q", loaded.Resources[0].Address, "asg.web")
	}
}

func TestWithObservationFulfilledSetsEffectiveStatus(t *testing.T) {
	contract := scan.Contract{Resources: []scan.ContractItem{{Address: "module.app.aws_autoscaling_group.main", Kind: "aws_asg", Status: "pending", DesiredGeneration: map[string]any{"rotation": "v2"}, Observation: scan.Observation{Strategy: "instance_refresh"}}}}

	updated := withObservationFulfilled(contract, "converged")
	if len(updated.Resources) != 1 {
		t.Fatalf("updated resources = %d, want 1", len(updated.Resources))
	}
	if updated.Resources[0].Status != "converged" {
		t.Fatalf("updated resource status = %q, want %q", updated.Resources[0].Status, "converged")
	}
	if updated.Resources[0].Observation.Fulfilled == nil || !*updated.Resources[0].Observation.Fulfilled {
		t.Fatalf("fulfilled flag = %v, want true", updated.Resources[0].Observation.Fulfilled)
	}
}

func TestWithObservationMetadataSetsTimeSpent(t *testing.T) {
	contract := scan.Contract{Resources: []scan.ContractItem{{Address: "module.app.aws_autoscaling_group.main", Kind: "aws_asg", Status: "pending", DesiredGeneration: map[string]any{"rotation": "v2"}, Observation: scan.Observation{Strategy: "instance_refresh"}}}}

	updated := withObservationMetadata(contract, []awaitResourceWait{{Address: "module.app.aws_autoscaling_group.main", Status: "converged", Wait: 250 * time.Millisecond}}, "converged")
	if updated.Resources[0].Observation.TimeSpent == nil {
		t.Fatalf("Observation.TimeSpent = nil, want a pointer to a positive float value")
	}
	if got, want := *updated.Resources[0].Observation.TimeSpent, 0.25; got != want {
		t.Fatalf("Observation.TimeSpent = %v, want %v", got, want)
	}
}

func TestWithObservationFulfilledPreservesMixedResourceState(t *testing.T) {
	trueValue := true
	falseValue := false
	contract := scan.Contract{Resources: []scan.ContractItem{
		{Address: "asg.web_frontend", Kind: "aws_asg", Status: "pending", DesiredGeneration: map[string]any{"rotation": 2}, Observation: scan.Observation{Strategy: "instance_refresh", Fulfilled: &trueValue}},
		{Address: "asg.api_backend", Kind: "aws_asg", Status: "pending", DesiredGeneration: map[string]any{"rotation": 1}, Observation: scan.Observation{Strategy: "instance_refresh", Fulfilled: &trueValue}},
		{Address: "asg.worker_batch", Kind: "aws_asg", Status: "pending", DesiredGeneration: map[string]any{"rotation": 3}, Observation: scan.Observation{Strategy: "instance_refresh", Fulfilled: &falseValue}},
	}}

	updated := withObservationFulfilled(contract, "pending")
	if len(updated.Resources) != 3 {
		t.Fatalf("updated resources = %d, want 3", len(updated.Resources))
	}
	if updated.Resources[0].Status != "converged" || updated.Resources[1].Status != "converged" || updated.Resources[2].Status != "pending" {
		t.Fatalf("resource statuses = %q, %q, %q, want converged, converged, pending", updated.Resources[0].Status, updated.Resources[1].Status, updated.Resources[2].Status)
	}
}

func TestAwaitTimeoutReportsToStderr(t *testing.T) {
	contract := scan.Contract{Resources: []scan.ContractItem{{Address: "module.app.aws_autoscaling_group.main", Kind: "aws_asg", DesiredGeneration: map[string]any{"rotation": "v2"}, Observation: scan.Observation{Strategy: "instance_refresh"}}}}

	old := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe returned error: %v", err)
	}
	os.Stderr = writer
	defer func() { os.Stderr = old }()

	ConfigureLogger(true)
	defer ConfigureLogger(false)

	result, err := pollAwait(contract, 20*time.Millisecond, 5*time.Millisecond, awscmd.DefaultConfig())
	if err == nil {
		t.Fatalf("pollAwait returned nil error, want timeout-style error")
	}
	if result.Pending != 1 {
		t.Fatalf("result.Pending = %d, want 1", result.Pending)
	}

	writer.Close()
	text, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatalf("ReadAll returned error: %v", readErr)
	}
	if !strings.Contains(string(text), "timeout reached") {
		t.Fatalf("stderr = %q, want it to contain timeout message", string(text))
	}
}

func TestScanCommandAWSProfileFlag(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"scan", "--aws-profile-name", "prod"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"output-json: .convergence.json\n",
		"aws-profile-name: prod\n",
	} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("scan output = %q, want it to contain %q", got, want)
		}
	}
}

func TestAwaitCommandAWSProfileFlag(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"await", "--aws-profile-name", "prod"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	want := "aws-profile-name: prod\n"
	if got != want {
		t.Fatalf("await output = %q, want %q", got, want)
	}
}
