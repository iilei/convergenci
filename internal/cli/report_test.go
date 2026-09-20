package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportCommandRendersConvergedReport(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	tempDir := t.TempDir()
	reportPath := filepath.Join(tempDir, "report.json")
	reportJSON := `{
		"status": "converged",
		"resources": [
			{"address": "module.app.aws_autoscaling_group.main", "status": "converged", "observation": {"strategy": "instance_refresh"}, "desired_generation": {"rotation": "bb"}}
		]
	}`
	if err := os.WriteFile(reportPath, []byte(reportJSON), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	if err := cmd.ExecuteArgs([]string{"report", reportPath}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Status: successful",
		"Converged resources: 1",
		"module.app.aws_autoscaling_group.main",
		"rotation=bb",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("report output = %q, want it to contain %q", got, want)
		}
	}
}

func TestReportCommandRendersPendingReport(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	tempDir := t.TempDir()
	reportPath := filepath.Join(tempDir, "report.json")
	reportJSON := `{"resources": [{"address": "aws_autoscaling_group.app", "status": "pending"}]}`
	if err := os.WriteFile(reportPath, []byte(reportJSON), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	if err := cmd.ExecuteArgs([]string{"report", reportPath}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Status: pending") {
		t.Fatalf("report output = %q, want it to contain %q", got, "Status: pending")
	}
	if !strings.Contains(got, "1 resource(s) remain pending") {
		t.Fatalf("report output = %q, want it to mention pending resources", got)
	}
}

func TestReportCommandRequiresReportPath(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	err := cmd.ExecuteArgs([]string{"report"})
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

func TestReportCommandMissingFile(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	err := cmd.ExecuteArgs([]string{"report", filepath.Join(t.TempDir(), "missing.json")})
	if err == nil {
		t.Fatalf("ExecuteArgs returned nil error, want an IO error")
	}
	exitErr, ok := err.(*ExitCodeError)
	if !ok {
		t.Fatalf("error = %T, want *ExitCodeError", err)
	}
	if exitErr.Code != codeIOError {
		t.Fatalf("exit code = %d, want %d", exitErr.Code, codeIOError)
	}
}

func TestReportCommandHelp(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"report", "--help"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Usage: convergenci report") {
		t.Fatalf("help output = %q, want it to contain usage text", got)
	}
}
