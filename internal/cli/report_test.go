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
			{"address": "module.app.aws_autoscaling_group.main", "status": "converged", "observation": {"strategy": "instance_refresh"}, "desired_generation": [{"type": "tag", "key": "rotation", "value": "bb"}]}
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

func TestReportCommandColorForcedOn(t *testing.T) {
	t.Setenv("CONVERGENCI_COLOR", "always")

	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	tempDir := t.TempDir()
	reportPath := filepath.Join(tempDir, "report.json")
	if err := os.WriteFile(
		reportPath,
		[]byte(`{"status": "converged", "resources": [{"address": "asg.app", "status": "converged"}]}`),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	if err := cmd.ExecuteArgs([]string{"report", reportPath}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	if got := out.String(); !strings.Contains(got, "\x1b[32mconverged\x1b[0m") {
		t.Fatalf("report output = %q, want ANSI-colored converged status", got)
	}
}

func TestReportCommandColorForcedOff(t *testing.T) {
	t.Setenv("CONVERGENCI_COLOR", "never")

	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	tempDir := t.TempDir()
	reportPath := filepath.Join(tempDir, "report.json")
	if err := os.WriteFile(
		reportPath,
		[]byte(`{"status": "pending", "resources": [{"address": "asg.app", "status": "pending"}]}`),
		0o644,
	); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	if err := cmd.ExecuteArgs([]string{"report", reportPath}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	if got := out.String(); strings.Contains(got, "\x1b[") {
		t.Fatalf("report output = %q, want no ANSI escapes with CONVERGENCI_COLOR=never", got)
	}
}

func TestStatusColorMapsKnownStatuses(t *testing.T) {
	t.Setenv("CONVERGENCI_COLOR", "always")
	funcs := reportTemplateFuncs(nil)
	statusColor := funcs["statusColor"].(func(string) string)

	tests := map[string]string{
		"converged":   ansiGreen,
		"successful":  ansiGreen,
		"pending":     ansiYellow,
		"failed":      ansiRed,
		"timeout":     ansiRed,
		"in-progress": ansiCyan,
		"InProgress":  ansiCyan,
		"unknown":     ansiMagenta,
	}
	for status, code := range tests {
		want := code + status + ansiReset
		if got := statusColor(status); got != want {
			t.Fatalf("statusColor(%q) = %q, want %q", status, got, want)
		}
	}
	if got := statusColor("some-other-status"); got != "some-other-status" {
		t.Fatalf("statusColor(\"some-other-status\") = %q, want unchanged text", got)
	}
}

func TestColorEnabledAcceptsTrueFalseAliases(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "true", want: true},
		{value: "TRUE", want: true},
		{value: "always", want: true},
		{value: "false", want: false},
		{value: "FALSE", want: false},
		{value: "never", want: false},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv("CONVERGENCI_COLOR", test.value)
			if got := colorEnabled(); got != test.want {
				t.Fatalf("colorEnabled() with CONVERGENCI_COLOR=%q = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

func TestSortResourcesGroupsSuccessFirstThenAlphabetizes(t *testing.T) {
	resources := []any{
		map[string]any{"address": "zebra.pending", "status": "pending"},
		map[string]any{"address": "beta.converged", "status": "converged"},
		map[string]any{"address": "alpha.pending", "status": "pending"},
		map[string]any{"address": "alpha.converged", "status": "converged"},
	}

	got := sortResources(resources)
	want := []string{"alpha.converged", "beta.converged", "alpha.pending", "zebra.pending"}
	for i, address := range want {
		if got[i].(map[string]any)["address"] != address {
			t.Fatalf("sortResources()[%d] address = %v, want %q", i, got[i].(map[string]any)["address"], address)
		}
	}
}

func TestStatusBulletDiffersBySuccess(t *testing.T) {
	funcs := reportTemplateFuncs(nil)
	statusBullet := funcs["statusBullet"].(func(string) string)

	if got := statusBullet("converged"); got != "*" {
		t.Fatalf("statusBullet(\"converged\") = %q, want \"*\"", got)
	}
	if got := statusBullet("successful"); got != "*" {
		t.Fatalf("statusBullet(\"successful\") = %q, want \"*\"", got)
	}
	if got := statusBullet("pending"); got != "-" {
		t.Fatalf("statusBullet(\"pending\") = %q, want \"-\"", got)
	}
	if got := statusBullet("failed"); got != "-" {
		t.Fatalf("statusBullet(\"failed\") = %q, want \"-\"", got)
	}
}

func TestReportCommandOrdersResourcesSuccessFirstAlphabetically(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{Version: "1.2.3", Commit: "abc123", Date: "2026-09-19"})
	cmd.SetOut(&out)

	tempDir := t.TempDir()
	reportPath := filepath.Join(tempDir, "report.json")
	reportJSON := `{
		"resources": [
			{"address": "zebra.pending", "status": "pending"},
			{"address": "beta.converged", "status": "converged"},
			{"address": "alpha.pending", "status": "pending"},
			{"address": "alpha.converged", "status": "converged"}
		]
	}`
	if err := os.WriteFile(reportPath, []byte(reportJSON), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	if err := cmd.ExecuteArgs([]string{"report", reportPath}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	got := out.String()
	wantOrder := []string{"alpha.converged", "beta.converged", "alpha.pending", "zebra.pending"}
	lastIndex := -1
	for _, address := range wantOrder {
		index := strings.Index(got, address)
		if index < 0 {
			t.Fatalf("report output = %q, missing address %q", got, address)
		}
		if index < lastIndex {
			t.Fatalf("report output = %q, want %q to appear after previous address", got, address)
		}
		lastIndex = index
	}
	if !strings.Contains(got, "* "+"alpha.converged") {
		t.Fatalf("report output = %q, want '*' bullet before converged resource", got)
	}
	if !strings.Contains(got, "- "+"alpha.pending") {
		t.Fatalf("report output = %q, want '-' bullet before pending resource", got)
	}
}
