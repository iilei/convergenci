package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"text/template"

	"github.com/iilei/convergenci-cli/templates"
)

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
			result := make(map[string]any, len(pairs)/2)
			for i := 0; i+1 < len(pairs); i += 2 {
				key, ok := pairs[i].(string)
				if !ok {
					continue
				}
				result[key] = pairs[i+1]
			}
			return result
		},
	}
}

func isEmptyValue(val any) bool {
	if val == nil {
		return true
	}
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	default:
		return v.IsZero()
	}
}

func (c *Command) executeReport(args []string) error {
	fs := flag.NewFlagSet("convergenci report", flag.ContinueOnError)
	fs.SetOutput(c.out)
	fs.Usage = func() {
		fmt.Fprintf(c.out, "Usage: convergenci report <report.json>\n\n")
		fmt.Fprintf(c.out, "Render a convergence report file as human-readable text using the\n")
		fmt.Fprintf(c.out, "built-in report template, without requiring gomplate.\n\n")
		fmt.Fprintf(c.out, "Flags:\n")
		fmt.Fprintf(c.out, "  -h, --help    Show help\n")
	}

	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
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
		return &ExitCodeError{Code: codeGenericFailure, Message: fmt.Sprintf("unable to parse report template: %v", err)}
	}
	if err := tpl.Execute(c.out, report); err != nil {
		return &ExitCodeError{Code: codeGenericFailure, Message: fmt.Sprintf("unable to render report: %v", err)}
	}
	return nil
}
