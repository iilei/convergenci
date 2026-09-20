package cli

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"text/template"
	"time"
)

var logger = newLogger(false)

const defaultDebugFormat = `[{{ .Level }} {{ .Timestamp }}]{{- " " -}}
{{- if or (eq .Event "retry_started") (eq .Event "retry_satisfied") -}}
    === {{ .Event }} {{ .RetryIteration }}/{{ .RetryLimit }} resources={{ join ", " .ResourceNames }} converged={{ join ", " .ConvergedResources }} elapsed={{ .Elapsed }} ===
{{- else if eq .Event "retry" -}}
	retry={{ .RetryIteration }}/{{ .RetryLimit }} pending={{ join ", " .PendingResources }} elapsed={{- .Elapsed }}
{{- else if eq .Event "desired_generation_pending" -}}
    >>> waiting for {{ .RequirementType }} {{ .RequirementKey }} on {{ .Resource }}: want={{ .Wanted }} actual={{ .Actual }} <<<
{{- else if eq .Event "desired_generation_met" -}}
    +++ {{ .RequirementType }} {{ .RequirementKey }} on {{ .Resource }} satisfied: value={{ .Wanted }} +++
{{- else -}}
    source={{ .Source }} resource={{ .Resource }} status={{ .Status }} pct={{ .PercentageComplete }}
{{- end -}}`

func newLogger(enabled bool) *loggerState {
	return &loggerState{enabled: enabled}
}

type loggerState struct {
	enabled    bool
	format     *template.Template
	rawFormat  string
	prefixMode bool
}

func (l *loggerState) logf(format string, args ...any) {
	if !l.enabled {
		return
	}
	message := fmt.Sprintf(format, args...)
	if l.format == nil {
		fmt.Fprintln(os.Stderr, message)
		return
	}
	if l.prefixMode {
		fmt.Fprintf(os.Stderr, "%s %s\n", renderLogPrefix(), message)
		return
	}
	fmt.Fprintln(os.Stderr, message)
}

func (l *loggerState) logLine(line string) {
	if !l.enabled {
		return
	}
	if strings.TrimSpace(line) == "" {
		return
	}
	if l.format == nil {
		fmt.Fprintln(os.Stderr, line)
		return
	}
	if l.prefixMode && strings.Contains(line, "[") && strings.Contains(line, "]") {
		fmt.Fprintln(os.Stderr, line)
		return
	}
	if l.prefixMode {
		fmt.Fprintf(os.Stderr, "%s %s\n", renderLogPrefix(), line)
		return
	}
	fmt.Fprintln(os.Stderr, line)
}

func ConfigureLogger(enabled bool) {
	logger.enabled = enabled
}

func ConfigureLoggerFromEnv() {
	ConfigureLogger(debugFromEnv())
}

func SetDebug(enabled bool) {
	ConfigureLogger(enabled)
}

func ConfigureDebugFormat(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		logger.format = nil
		logger.rawFormat = ""
		logger.prefixMode = false
		return nil
	}
	tpl, err := template.New("debug").Funcs(template.FuncMap{
		"join": func(separator string, values []string) string {
			return strings.Join(values, separator)
		},
	}).Option("missingkey=zero").Parse(raw)
	if err != nil {
		return err
	}
	logger.format = tpl
	logger.rawFormat = raw
	logger.prefixMode = strings.Contains(raw, ".Level") || strings.Contains(raw, ".Timestamp") ||
		strings.Contains(raw, ".Prefix") ||
		strings.Contains(raw, ".Time")
	return nil
}

func DebugEnabled() bool {
	return logger.enabled
}

func Debugf(format string, args ...any) {
	logger.logf(format, args...)
}

func renderDebugTemplate(value any) string {
	if !logger.enabled || logger.format == nil {
		return ""
	}
	var out bytes.Buffer
	if err := logger.format.Execute(&out, buildDebugTemplateData(value)); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

func renderLogPrefix() string {
	if !logger.enabled {
		return ""
	}
	data := buildDebugTemplateData(nil)
	return fmt.Sprintf("[%s %s]", data["Level"], data["Timestamp"])
}

func buildDebugTemplateData(value any) map[string]any {
	level := "DEBUG"
	if !DebugEnabled() {
		level = "INFO"
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	data := map[string]any{
		"Level":     level,
		"Timestamp": timestamp,
		"Time":      time.Now().UTC(),
		"Prefix":    fmt.Sprintf("[%s %s]", level, timestamp),
	}
	if value == nil {
		return data
	}
	if m, ok := value.(map[string]any); ok {
		for k, v := range m {
			data[k] = v
		}
		return data
	}
	if m, ok := value.(map[string]string); ok {
		for k, v := range m {
			data[k] = v
		}
		return data
	}
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return data
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			data[field.Name] = v.Field(i).Interface()
		}
		return data
	}
	data["Value"] = value
	return data
}

func debugFromEnv() bool {
	value := strings.TrimSpace(os.Getenv("DEBUG"))
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}
	return parsed
}

func init() {
	ConfigureLoggerFromEnv()
	format := strings.TrimSpace(os.Getenv("CONVERGENCI_DEBUG_FORMAT"))
	if format == "" {
		format = defaultDebugFormat
	}
	_ = ConfigureDebugFormat(format)
}
