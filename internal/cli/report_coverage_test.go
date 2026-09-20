package cli

import "testing"

func TestIsEmptyValue(t *testing.T) {
	value := 42
	nilPointer := (*int)(nil)
	nonNilPointer := &value
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "nil", value: nil, want: true},
		{name: "empty string", value: "", want: true},
		{name: "string", value: "value", want: false},
		{name: "empty slice", value: []string{}, want: true},
		{name: "slice", value: []string{"value"}, want: false},
		{name: "empty map", value: map[string]string{}, want: true},
		{name: "map", value: map[string]string{"key": "value"}, want: false},
		{name: "zero array", value: [0]string{}, want: true},
		{name: "array", value: [1]string{}, want: false},
		{name: "zero integer", value: 0, want: true},
		{name: "integer", value: 1, want: false},
		{name: "nil pointer", value: nilPointer, want: true},
		{name: "pointer", value: nonNilPointer, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isEmptyValue(test.value); got != test.want {
				t.Fatalf("isEmptyValue(%#v) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

func TestReportTemplateFuncs(t *testing.T) {
	report := map[string]any{"status": "pending"}
	functions := reportTemplateFuncs(report)

	if got := functions["ds"].(func(string) map[string]any)("report"); got["status"] != "pending" {
		t.Fatalf("ds() = %#v, want original report", got)
	}
	if got := functions["add"].(func(int, int) int)(2, 3); got != 5 {
		t.Fatalf("add(2, 3) = %d, want 5", got)
	}
	defaultValue := functions["default"].(func(any, any) any)
	if got := defaultValue("fallback", ""); got != "fallback" {
		t.Fatalf("default() for empty value = %#v, want fallback", got)
	}
	if got := defaultValue("fallback", "present"); got != "present" {
		t.Fatalf("default() for present value = %#v, want present", got)
	}
	dict := functions["dict"].(func(...any) map[string]any)
	got := dict("status", "pending", 123, "ignored", "message")
	if got["status"] != "pending" {
		t.Fatalf("dict() = %#v, want status entry", got)
	}
	if _, ok := got["message"]; ok {
		t.Fatalf("dict() = %#v, want trailing key without value ignored", got)
	}
}
