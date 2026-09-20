package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestLoggerRespectsEnvVar(t *testing.T) {
	old := os.Getenv("DEBUG")
	if err := os.Setenv("DEBUG", "true"); err != nil {
		t.Fatalf("Setenv DEBUG=true: %v", err)
	}
	defer func() {
		if old == "" {
			_ = os.Unsetenv("DEBUG")
			return
		}
		_ = os.Setenv("DEBUG", old)
	}()

	SetDebug(false)
	ConfigureLoggerFromEnv()
	if !DebugEnabled() {
		t.Fatal("expected DEBUG=true to enable debug logging")
	}
}

func TestLoggerCanBeEnabledExplicitly(t *testing.T) {
	old := os.Getenv("DEBUG")
	if err := os.Unsetenv("DEBUG"); err != nil {
		t.Fatalf("Unsetenv DEBUG: %v", err)
	}
	defer func() {
		if old == "" {
			_ = os.Unsetenv("DEBUG")
			return
		}
		_ = os.Setenv("DEBUG", old)
	}()

	SetDebug(false)
	ConfigureLogger(true)
	if !DebugEnabled() {
		t.Fatal("expected ConfigureLogger(true) to enable debug logging")
	}
}

func TestRootCommandSetDebugFlag(t *testing.T) {
	old := os.Getenv("DEBUG")
	if err := os.Unsetenv("DEBUG"); err != nil {
		t.Fatalf("Unsetenv DEBUG: %v", err)
	}
	defer func() {
		if old == "" {
			_ = os.Unsetenv("DEBUG")
			return
		}
		_ = os.Setenv("DEBUG", old)
	}()

	SetDebug(false)
	cmd := NewRootCommand(Version{Version: "dev"})
	if err := cmd.ExecuteArgs([]string{"--debug"}); err != nil {
		t.Fatalf("ExecuteArgs returned error: %v", err)
	}
	if !DebugEnabled() {
		t.Fatal("expected --debug to enable debug logging")
	}
}

func TestConfigureDebugFormatRendersProgress(t *testing.T) {
	SetDebug(true)
	defer SetDebug(false)

	if err := ConfigureDebugFormat("{{ .Status }} {{ .PercentageComplete }}%"); err != nil {
		t.Fatalf("ConfigureDebugFormat returned error: %v", err)
	}

	got := renderDebugTemplate(struct {
		Status             string
		PercentageComplete int
	}{Status: "InProgress", PercentageComplete: 42})
	if got != "InProgress 42%" {
		t.Fatalf("renderDebugTemplate = %q, want %q", got, "InProgress 42%")
	}
}

func TestConfigureDebugFormatRendersRetryFields(t *testing.T) {
	SetDebug(true)
	defer SetDebug(false)

	if err := ConfigureDebugFormat("{{ .Event }} {{ .RetryIteration }}/{{ .RetryLimit }} {{ .Elapsed }}"); err != nil {
		t.Fatalf("ConfigureDebugFormat returned error: %v", err)
	}

	got := renderDebugTemplate(map[string]any{
		"Event":          "retry",
		"RetryIteration": 2,
		"RetryLimit":     10,
		"Elapsed":        "5s",
	})
	if got != "retry 2/10 5s" {
		t.Fatalf("renderDebugTemplate = %q, want %q", got, "retry 2/10 5s")
	}
}

func TestConfigureDebugFormatRendersRetryLifecycleFields(t *testing.T) {
	SetDebug(true)
	defer SetDebug(false)

	if err := ConfigureDebugFormat("{{ .Event }} {{ .RetryIteration }}/{{ .RetryLimit }} {{ join \",\" .ResourceNames }} {{ .Elapsed }}"); err != nil {
		t.Fatalf("ConfigureDebugFormat returned error: %v", err)
	}

	for _, event := range []string{"retry_started", "retry_satisfied"} {
		got := renderDebugTemplate(map[string]any{
			"Event":          event,
			"RetryIteration": 1,
			"RetryLimit":     10,
			"ResourceNames":  []string{"web-asg", "worker-asg"},
			"Elapsed":        "0s",
		})
		want := event + " 1/10 web-asg,worker-asg 0s"
		if got != want {
			t.Fatalf("renderDebugTemplate(%q) = %q, want %q", event, got, want)
		}
	}
}

func TestConfigureDebugFormatRendersSatisfiedRetryFields(t *testing.T) {
	SetDebug(true)
	defer SetDebug(false)

	if err := ConfigureDebugFormat("{{ .Event }} {{ .RetryIteration }}/{{ .RetryLimit }} {{ .Status }} {{ .Message }}"); err != nil {
		t.Fatalf("ConfigureDebugFormat returned error: %v", err)
	}

	got := renderDebugTemplate(map[string]any{
		"Event":          "retry_satisfied",
		"RetryIteration": 2,
		"RetryLimit":     10,
		"Status":         "converged",
		"Message":        "all expected resources converged",
	})
	if got != "retry_satisfied 2/10 converged all expected resources converged" {
		t.Fatalf("renderDebugTemplate = %q, want satisfied retry fields", got)
	}
}

func TestConfigureDebugFormatRendersObservationFields(t *testing.T) {
	SetDebug(true)
	defer SetDebug(false)

	if err := ConfigureDebugFormat("{{ .Event }} {{ .Resource }} {{ .Status }} {{ .PercentageComplete }} {{ .Cause }}"); err != nil {
		t.Fatalf("ConfigureDebugFormat returned error: %v", err)
	}

	got := renderDebugTemplate(map[string]any{
		"Event":              "autoscaling_activity",
		"Resource":           "asg.web",
		"Status":             "InProgress",
		"PercentageComplete": 62,
		"Cause":              "launching instance",
	})
	if got != "autoscaling_activity asg.web InProgress 62 launching instance" {
		t.Fatalf("renderDebugTemplate = %q, want observation fields", got)
	}
}

func TestConfigureDebugFormatSupportsPrefixAndTimestamp(t *testing.T) {
	SetDebug(true)
	defer SetDebug(false)

	if err := ConfigureDebugFormat("[{{ .Level }} {{ .Timestamp }}] status={{ .Status }} pct={{ .PercentageComplete }}"); err != nil {
		t.Fatalf("ConfigureDebugFormat returned error: %v", err)
	}

	got := renderDebugTemplate(struct {
		Status             string
		PercentageComplete int
	}{Status: "Failed", PercentageComplete: 100})
	if got == "" {
		t.Fatal("renderDebugTemplate returned empty output")
	}
	if !strings.Contains(got, "status=Failed") || !strings.Contains(got, "pct=100") {
		t.Fatalf("renderDebugTemplate = %q, want status and percentage fields", got)
	}
	if !strings.Contains(got, "[DEBUG ") {
		t.Fatalf("renderDebugTemplate = %q, want DEBUG prefix", got)
	}
}

func TestDebugfPrefixesPlainMessages(t *testing.T) {
	SetDebug(true)
	defer SetDebug(false)

	if err := ConfigureDebugFormat("[{{ .Level }} {{ .Timestamp }}]"); err != nil {
		t.Fatalf("ConfigureDebugFormat returned error: %v", err)
	}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe returned error: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	Debugf("retry %d/%d", 1, 3)
	_ = w.Close()
	out, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatalf("io.ReadAll returned error: %v", err)
	}
	if !strings.Contains(string(out), "retry 1/3") {
		t.Fatalf("Debugf output = %q, want plain retry message with prefix", string(out))
	}
	if !strings.Contains(string(out), "[DEBUG ") {
		t.Fatalf("Debugf output = %q, want DEBUG prefix", string(out))
	}
}

func TestLogLineDoesNotDoublePrefixRenderedOutput(t *testing.T) {
	SetDebug(true)
	defer SetDebug(false)

	if err := ConfigureDebugFormat("[{{ .Level }} {{ .Timestamp }}] status={{ .Status }} pct={{ .PercentageComplete }}"); err != nil {
		t.Fatalf("ConfigureDebugFormat returned error: %v", err)
	}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe returned error: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	logger.logLine("instance refresh: " + renderDebugTemplate(struct {
		Status             string
		PercentageComplete int
	}{Status: "InProgress", PercentageComplete: 42}))
	_ = w.Close()
	out, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatalf("io.ReadAll returned error: %v", err)
	}
	text := string(out)
	if strings.Count(text, "[DEBUG ") != 1 {
		t.Fatalf("logLine output = %q, want a single DEBUG prefix, got %d", text, strings.Count(text, "[DEBUG "))
	}
}
