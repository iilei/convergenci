package awscmd

import "testing"

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
