package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"text/template"

	"github.com/iilei/convergenci-cli/templates"
)

const (
	templateDictPairWidth = 2

	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiCyan    = "\033[36m"
	ansiMagenta = "\033[35m"
)

// colorEnabled reports whether ANSI color codes should be emitted in report output.
// CONVERGENCI_COLOR=always/true forces color on and never/false forces it off; otherwise
// color is enabled only when NO_COLOR (https://no-color.org) is unset and stdout is an
// interactive terminal, so piped or redirected output (files, CI logs) stays plain by default.
func colorEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CONVERGENCI_COLOR"))) {
	case "always", boolStringTrue:
		return true
	case "never", boolStringFalse:
		return false
	}
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// colorize wraps text in the given ANSI code, unless color output is disabled.
func colorize(code, text string) string {
	if !colorEnabled() || text == "" {
		return text
	}
	return code + text + ansiReset
}

// reportTemplateFuncs provides the small subset of gomplate-style helpers used by the
// built-in report template, so it can be rendered with the standard library alone.
func reportTemplateFuncs(report map[string]any) template.FuncMap {
	return template.FuncMap{
		"ds": func(string) map[string]any { return report },
		"add": func(a, b int) int {
			return a + b
		},
		"default": func(def, val any) any {
			if isEmptyValue(val) {
				return def
			}
			return val
		},
		"dict": func(pairs ...any) map[string]any {
			result := make(map[string]any, len(pairs)/templateDictPairWidth)
			for i := 0; i+1 < len(pairs); i += templateDictPairWidth {
				key, ok := pairs[i].(string)
				if !ok {
					continue
				}
				result[key] = pairs[i+1]
			}
			return result
		},
		"red":    func(text string) string { return colorize(ansiRed, text) },
		"green":  func(text string) string { return colorize(ansiGreen, text) },
		"yellow": func(text string) string { return colorize(ansiYellow, text) },
		"cyan":   func(text string) string { return colorize(ansiCyan, text) },
		"bold":   func(text string) string { return colorize(ansiBold, text) },
		"statusColor": func(status string) string {
			switch strings.ToLower(strings.TrimSpace(status)) {
			case statusConverged, "successful":
				return colorize(ansiGreen, status)
			case statusPending:
				return colorize(ansiYellow, status)
			case statusFailed, statusTimeout:
				return colorize(ansiRed, status)
			case "inprogress", "in-progress", "in_progress":
				return colorize(ansiCyan, status)
			case "unknown":
				return colorize(ansiMagenta, status)
			default:
				return status
			}
		},
		"statusBullet": func(status string) string {
			if isSuccessStatus(status) {
				return "*"
			}
			return "-"
		},
		"sortResources": sortResources,
	}
}

// isSuccessStatus reports whether a resource/report status counts as successful.
func isSuccessStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case statusConverged, "successful":
		return true
	default:
		return false
	}
}

// sortResources orders report resources with successful ones first, each group
// alphabetized by address, so the rendered report groups converged resources
// together ahead of resources that still need attention.
func sortResources(resources []any) []any {
	sorted := make([]any, len(resources))
	copy(sorted, resources)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, _ := sorted[i].(map[string]any)
		right, _ := sorted[j].(map[string]any)
		leftSuccess := isSuccessStatus(fmt.Sprintf("%v", left["status"]))
		rightSuccess := isSuccessStatus(fmt.Sprintf("%v", right["status"]))
		if leftSuccess != rightSuccess {
			return leftSuccess
		}
		return fmt.Sprintf("%v", left["address"]) < fmt.Sprintf("%v", right["address"])
	})
	return sorted
}

func isEmptyValue(val any) bool {
	if val == nil {
		return true
	}
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}

func (c *Command) executeReport(args []string) error {
	fs := flag.NewFlagSet("convergenci report", flag.ContinueOnError)
	fs.SetOutput(c.out)
	fs.Usage = func() {
		writeBestEffortf(c.out, "Usage: convergenci report <report.json>\n\n")
		writeBestEffortf(c.out, "Render a convergence report file as human-readable text using the\n")
		writeBestEffortf(c.out, "built-in report template, without requiring gomplate.\n\n")
		writeBestEffortf(c.out, "Flags:\n")
		writeBestEffortf(c.out, "  -h, --help    Show help\n")
	}

	if len(args) > 0 && (args[0] == shortHelpFlag || args[0] == longHelpFlag) {
		fs.Usage()
		return nil
	}
	positionals, err := parseFlagsWithPositionals(fs, args)
	if err != nil {
		return &ExitCodeError{Code: codeConfigError, Message: err.Error()}
	}
	if len(positionals) == 0 {
		return &ExitCodeError{Code: codeConfigError, Message: "a report file path is required"}
	}

	reportPath := positionals[0]
	// #nosec G304 G703 -- reportPath is the report file explicitly selected by the CLI user.
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return &ExitCodeError{Code: codeIOError, Message: fmt.Sprintf("unable to read report file: %v", err)}
	}

	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		return &ExitCodeError{Code: codeIOError, Message: fmt.Sprintf("unable to decode report file: %v", err)}
	}

	tpl, err := template.New("report-as-text").Funcs(reportTemplateFuncs(report)).Parse(templates.ReportAsText)
	if err != nil {
		return &ExitCodeError{
			Code:    codeGenericFailure,
			Message: fmt.Sprintf("unable to parse report template: %v", err),
		}
	}
	if err := tpl.Execute(c.out, report); err != nil {
		return &ExitCodeError{Code: codeGenericFailure, Message: fmt.Sprintf("unable to render report: %v", err)}
	}
	return nil
}
