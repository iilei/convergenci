package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func fakeAWSBinPath() string {
	return filepath.Join("..", "..", "testdata", "fake-aws-bin", "aws")
}

func defaultRotationPlanPath() string {
	return filepath.Join("..", "..", "stubs", "asg-default-rotation.json")
}

func TestScanAssertAllSettledSucceedsWhenNoRefreshInProgress(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	t.Setenv("FAKE_AWS_SCENARIO", "success")

	err := cmd.ExecuteArgs([]string{"scan", "--assert-all-settled", "--aws-cli-path", fakeAWSBinPath(), defaultRotationPlanPath()})
	if err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "all resources settled") {
		t.Fatalf("output = %q, want it to contain %q", got, "all resources settled")
	}
}

func TestScanAssertAllSettledFailsWhenRefreshInProgress(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	t.Setenv("FAKE_AWS_SCENARIO", "in-progress")

	err := cmd.ExecuteArgs([]string{"scan", "--assert-all-settled", "--aws-cli-path", fakeAWSBinPath(), defaultRotationPlanPath()})
	if err == nil {
		t.Fatalf("ExecuteArgs returned nil error, want failure for in-progress refresh")
	}
	exitErr, ok := err.(*ExitCodeError)
	if !ok {
		t.Fatalf("error = %T, want *ExitCodeError", err)
	}
	if !strings.Contains(exitErr.Message, "app-asg") {
		t.Fatalf("error message = %q, want it to mention %q", exitErr.Message, "app-asg")
	}
}

func TestScanAssertAllSettledRequiresPlanPath(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	err := cmd.ExecuteArgs([]string{"scan", "--assert-all-settled"})
	if err == nil {
		t.Fatalf("ExecuteArgs returned nil error, want a config error")
	}
	exitErr, ok := err.(*ExitCodeError)
	if !ok {
		t.Fatalf("error = %T, want *ExitCodeError", err)
	}
	if exitErr.Code != codeConfigError {
		t.Fatalf("exit code = %d, want %d", exitErr.Code, codeConfigError)
	}
}

func TestScanAssertAllSettledHelp(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"scan", "--help"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "--assert-all-settled") {
		t.Fatalf("help output = %q, want it to contain --assert-all-settled", got)
	}
}
