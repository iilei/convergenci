package cli

import (
	"bytes"
	"flag"
	"testing"
)

func TestPrintVersionVariants(t *testing.T) {
	tests := []struct {
		name    string
		version Version
		want    string
	}{
		{name: "development build", version: Version{}, want: "dev\n"},
		{name: "version without commit", version: Version{Version: "1.2.3"}, want: "1.2.3\n"},
		{name: "version with none commit", version: Version{Version: "1.2.3", Commit: "none"}, want: "1.2.3\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := NewRootCommand(test.version)
			cmd.SetOut(&out)
			cmd.printVersion()
			if got := out.String(); got != test.want {
				t.Fatalf("printVersion() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseFlagsWithPositionals(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Bool("verbose", false, "verbose output")
	fs.String("name", "", "name")

	positionals, err := parseFlagsWithPositionals(
		fs,
		[]string{"first.json", "--verbose", "second.json", "--name", "example", "--", "literal", "--name"},
	)
	if err != nil {
		t.Fatalf("parseFlagsWithPositionals returned error: %v", err)
	}
	if got, want := fs.Lookup("verbose").Value.String(), "true"; got != want {
		t.Fatalf("verbose flag = %q, want %q", got, want)
	}
	if got, want := fs.Lookup("name").Value.String(), "example"; got != want {
		t.Fatalf("name flag = %q, want %q", got, want)
	}
	wantPositionals := []string{"first.json", "second.json", "literal", "--name"}
	if len(positionals) != len(wantPositionals) {
		t.Fatalf("positionals = %#v, want %#v", positionals, wantPositionals)
	}
	for index := range wantPositionals {
		if positionals[index] != wantPositionals[index] {
			t.Fatalf("positionals = %#v, want %#v", positionals, wantPositionals)
		}
	}
}

func TestParseFlagsWithPositionalsRejectsInvalidFlag(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	_, err := parseFlagsWithPositionals(fs, []string{"--unknown"})
	if err == nil {
		t.Fatal("parseFlagsWithPositionals returned nil error, want flag error")
	}
}
