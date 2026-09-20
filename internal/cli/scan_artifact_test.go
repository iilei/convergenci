package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iilei/convergenci-cli/internal/scan"
)

func TestSafeOutputFilePathRejectsInvalidPaths(t *testing.T) {
	for _, path := range []string{"", "   ", ".", string(filepath.Separator)} {
		t.Run(path, func(t *testing.T) {
			if _, err := safeOutputFilePath(path, false); err == nil {
				t.Fatalf("safeOutputFilePath(%q) returned nil error", path)
			}
		})
	}
}

func TestSafeOutputFilePathForceAllowsOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	got, err := safeOutputFilePath(path, true)
	if err != nil {
		t.Fatalf("safeOutputFilePath returned error: %v", err)
	}
	if got != path {
		t.Fatalf("safeOutputFilePath = %q, want %q", got, path)
	}
}

func TestWriteScanArtifactJSONLinesCreatesParentDirectories(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "nested", "contract.jsonlines")
	contract := scan.Contract{Resources: []scan.ContractItem{
		{Address: "asg.one", Kind: "aws_asg", DesiredGeneration: map[string]any{"rotation": "blue"}},
		{Address: "asg.two", Kind: "aws_asg", DesiredGeneration: map[string]any{"version": "7"}},
	}}

	if err := writeScanArtifact(outputPath, contract, true, false); err != nil {
		t.Fatalf("writeScanArtifact returned error: %v", err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("JSON Lines output has %d lines, want 2: %q", len(lines), data)
	}
	for index, line := range lines {
		var item scan.ContractItem
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			t.Fatalf("line %d is not valid JSON: %v", index, err)
		}
		if item.Address == "" {
			t.Fatalf("line %d has empty address", index)
		}
	}
}

func TestWriteScanArtifactForceOverwritesExistingFile(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "contract.json")
	if err := os.WriteFile(outputPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	contract := scan.Contract{SchemaVersion: 1}

	if err := writeScanArtifact(outputPath, contract, false, true); err != nil {
		t.Fatalf("writeScanArtifact returned error: %v", err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if strings.Contains(string(data), "old") {
		t.Fatalf("output still contains old content: %q", data)
	}
}
