package awscmd

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigArgsIncludesProfile(t *testing.T) {
	cfg := Config{BinaryPath: "/usr/local/bin/aws", Profile: "prod"}
	got := cfg.Args("autoscaling", "describe-auto-scaling-groups")
	want := []string{"--profile", "prod", "autoscaling", "describe-auto-scaling-groups"}
	if len(got) != len(want) {
		t.Fatalf("Args length = %d, want %d; got %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Args[%d] = %q, want %q; got %v", i, got[i], want[i], got)
		}
	}
}

func TestConfigArgsWithoutProfile(t *testing.T) {
	cfg := Config{BinaryPath: "/usr/local/bin/aws"}
	got := cfg.Args("ecs", "describe-services")
	want := []string{"ecs", "describe-services"}
	if len(got) != len(want) {
		t.Fatalf("Args length = %d, want %d; got %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Args[%d] = %q, want %q; got %v", i, got[i], want[i], got)
		}
	}
}

func TestDefaultConfigUsesAWSCLIPathEnvironment(t *testing.T) {
	t.Setenv("CONVERGENCI_AWS_CLI_PATH", "/tmp/fake-aws")

	if got := DefaultConfig().BinaryPath; got != "/tmp/fake-aws" {
		t.Fatalf("DefaultConfig().BinaryPath = %q, want %q", got, "/tmp/fake-aws")
	}
}

func TestDefaultConfigFallsBackToAWS(t *testing.T) {
	t.Setenv("CONVERGENCI_AWS_CLI_PATH", "")

	if got := DefaultConfig().BinaryPath; got != "aws" {
		t.Fatalf("DefaultConfig().BinaryPath = %q, want %q", got, "aws")
	}
}

func TestRegisterFlagsAndUsageText(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	path, profile := RegisterFlags(fs)

	if err := fs.Parse([]string{"--aws-cli-path", "/custom/aws", "--aws-profile-name", "staging"}); err != nil {
		t.Fatalf("FlagSet.Parse returned error: %v", err)
	}
	if *path != "/custom/aws" {
		t.Fatalf("aws-cli-path = %q, want %q", *path, "/custom/aws")
	}
	if *profile != "staging" {
		t.Fatalf("aws-profile-name = %q, want %q", *profile, "staging")
	}
	for _, want := range []string{"--aws-cli-path", "--aws-profile-name"} {
		if !strings.Contains(UsageText(), want) {
			t.Fatalf("UsageText() = %q, want %q", UsageText(), want)
		}
	}
}

func TestConfigRunUsesConfiguredBinary(t *testing.T) {
	cfg := Config{BinaryPath: "/bin/echo", Profile: "prod"}
	out, err := cfg.Run("service", "operation")
	if err != nil {
		t.Fatalf("Config.Run returned error: %v", err)
	}
	if got, want := string(out), "--profile prod service operation\n"; got != want {
		t.Fatalf("Config.Run output = %q, want %q", got, want)
	}
}

func TestConfigRunUsesAWSFallbackBinary(t *testing.T) {
	binDir := t.TempDir()
	awsPath := filepath.Join(binDir, "aws")
	if err := os.WriteFile(awsPath, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0o755); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	t.Setenv("PATH", binDir)

	out, err := (Config{}).Run("service")
	if err != nil {
		t.Fatalf("Config.Run returned error: %v", err)
	}
	if got, want := string(out), "service"; got != want {
		t.Fatalf("Config.Run output = %q, want %q", got, want)
	}
}

func TestConfigRunReturnsCommandError(t *testing.T) {
	_, err := Config{BinaryPath: "/bin/sh"}.Run("-c", "exit 7")
	if err == nil {
		t.Fatal("Config.Run returned nil error, want command error")
	}
}
