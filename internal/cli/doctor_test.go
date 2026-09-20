package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iilei/convergenci-cli/internal/awscmd"
)

func TestDoctorCommandReportsAWSAvailability(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{})
	cmd.SetOut(&out)

	if err := cmd.ExecuteArgs([]string{"doctor", "--aws-cli-path", fakeAWSCLIPath(t)}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	if got := out.String(); !strings.Contains(got, "aws-cli-path: ") {
		t.Fatalf("doctor output = %q, want configured CLI path", got)
	}
	if !strings.Contains(out.String(), "doctor: AWS CLI is available") {
		t.Fatalf("doctor output = %q, want availability message", out.String())
	}
}

func TestDoctorCommandReportsProfileCheckFailure(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand(Version{})
	cmd.SetOut(&out)

	err := cmd.ExecuteArgs([]string{"doctor", "--aws-cli-path", fakeAWSCLIPath(t), "--aws-profile-name", "test-profile"})
	if err == nil || !strings.Contains(err.Error(), "AWS caller identity check failed") {
		t.Fatalf("ExecuteArgs error = %v, want caller identity failure", err)
	}
	if !strings.Contains(out.String(), "aws-profile-name: test-profile") {
		t.Fatalf("doctor output = %q, want configured profile", out.String())
	}
}

func TestDoctorCommandRendersReportWithCustomTemplate(t *testing.T) {
	tempDir := t.TempDir()
	reportPath := filepath.Join(tempDir, "report.json")
	if err := os.WriteFile(reportPath, []byte(`{"status":"converged"}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile report returned error: %v", err)
	}
	templatePath := filepath.Join(tempDir, "report.tmpl")
	if err := os.WriteFile(templatePath, []byte("status={{.status}}"), 0o644); err != nil {
		t.Fatalf("os.WriteFile template returned error: %v", err)
	}

	var out bytes.Buffer
	cmd := NewRootCommand(Version{})
	cmd.SetOut(&out)
	if err := cmd.ExecuteArgs([]string{"doctor", "--aws-cli-path", fakeAWSCLIPath(t), "--template", templatePath, reportPath}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}

	if got := out.String(); !strings.Contains(got, "status=converged\n") {
		t.Fatalf("doctor output = %q, want rendered report", got)
	}
}

func TestDoctorCommandRejectsInvalidReportInputs(t *testing.T) {
	awsPath := fakeAWSCLIPath(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "empty template path",
			args: []string{"doctor", "--aws-cli-path", awsPath, "--template", "", "report.json"},
			want: "template path is empty",
		},
		{
			name: "missing report",
			args: []string{"doctor", "--aws-cli-path", awsPath, filepath.Join(t.TempDir(), "missing.json")},
			want: "read report:",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := NewRootCommand(Version{})
			cmd.SetOut(&out)
			if err := cmd.ExecuteArgs(test.args); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ExecuteArgs error = %v, want it to contain %q", err, test.want)
			}
		})
	}
}

func TestDoctorCommandRejectsInvalidReportJSONAndTemplate(t *testing.T) {
	tempDir := t.TempDir()
	invalidReport := filepath.Join(tempDir, "invalid.json")
	if err := os.WriteFile(invalidReport, []byte("{"), 0o644); err != nil {
		t.Fatalf("os.WriteFile invalid report returned error: %v", err)
	}

	missingTemplate := filepath.Join(tempDir, "missing.tmpl")
	var out bytes.Buffer
	cmd := NewRootCommand(Version{})
	cmd.SetOut(&out)
	if err := cmd.ExecuteArgs([]string{"doctor", "--aws-cli-path", fakeAWSCLIPath(t), "--template", missingTemplate, invalidReport}); err == nil || !strings.Contains(err.Error(), "decode report") {
		t.Fatalf("invalid report error = %v, want decode report error", err)
	}

	validReport := filepath.Join(tempDir, "valid.json")
	if err := os.WriteFile(validReport, []byte(`{"status":"pending"}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile valid report returned error: %v", err)
	}
	if err := cmd.ExecuteArgs([]string{"doctor", "--aws-cli-path", fakeAWSCLIPath(t), "--template", missingTemplate, validReport}); err == nil || !strings.Contains(err.Error(), "parse template") {
		t.Fatalf("missing template error = %v, want parse template error", err)
	}
}

func TestCheckAWSCLIRejectsMissingBinary(t *testing.T) {
	err := checkAWSCLI(awscmd.Config{BinaryPath: filepath.Join(t.TempDir(), "missing-aws")})
	if err == nil || !strings.Contains(err.Error(), "AWS CLI not found") {
		t.Fatalf("checkAWSCLI error = %v, want missing CLI error", err)
	}
}
