package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildContractDefaultASGRotation(t *testing.T) {
	plan, err := LoadPlan("../../stubs/asg-default-rotation.json")
	if err != nil {
		t.Fatalf("LoadPlan returned error: %v", err)
	}

	contract, err := BuildContract(plan, ASGDefaultPolicy, "", nil)
	if err != nil {
		t.Fatalf("BuildContract returned error: %v", err)
	}
	if len(contract.Resources) != 1 {
		t.Fatalf("len(resources) = %d, want 1", len(contract.Resources))
	}

	got := contract.Resources[0].DesiredGeneration
	if len(got) != 2 || got[0].Type != "tag" || got[0].Key != "rotation" || got[0].Value != "bb" ||
		got[1].Type != "launch_template" ||
		got[1].Key != "version" ||
		got[1].Value != "6" {
		t.Fatalf("desired generation = %#v, want tag rotation=bb and launch_template version=6", got)
	}
}

func TestBuildContractCustomTagRotation(t *testing.T) {
	plan, err := LoadPlan("../../stubs/asg-custom-tag-rotation.json")
	if err != nil {
		t.Fatalf("LoadPlan returned error: %v", err)
	}

	contract, err := BuildContract(plan, ASGDefaultPolicy, "", nil)
	if err != nil {
		t.Fatalf("BuildContract returned error: %v", err)
	}
	if len(contract.Resources) != 1 {
		t.Fatalf("len(resources) = %d, want 1", len(contract.Resources))
	}

	got := contract.Resources[0].DesiredGeneration
	if len(got) != 1 || got[0].Type != "tag" || got[0].Key != "rotation" || got[0].Value != "v2" {
		t.Fatalf("desired generation = %#v, want tag rotation=v2", got)
	}
}

func TestBuildContractMixedInstancePolicyVersion(t *testing.T) {
	plan, err := LoadPlan("../../stubs/asg-mixed-instance-policy.json")
	if err != nil {
		t.Fatalf("LoadPlan returned error: %v", err)
	}

	contract, err := BuildContract(plan, ASGDefaultPolicy, "", nil)
	if err != nil {
		t.Fatalf("BuildContract returned error: %v", err)
	}
	if len(contract.Resources) != 1 {
		t.Fatalf("len(resources) = %d, want 1", len(contract.Resources))
	}

	got := contract.Resources[0].DesiredGeneration
	if len(got) != 1 || got[0].Type != "launch_template" || got[0].Key != "version" || got[0].Value != "4" {
		t.Fatalf("desired generation = %#v, want launch_template version=4", got)
	}
}

func TestBuildContractNestedModuleComplexAddress(t *testing.T) {
	plan, err := LoadPlan("../../stubs/asg-nested-module-complex-name.json")
	if err != nil {
		t.Fatalf("LoadPlan returned error: %v", err)
	}

	contract, err := BuildContract(plan, ASGDefaultPolicy, "", nil)
	if err != nil {
		t.Fatalf("BuildContract returned error: %v", err)
	}
	if len(contract.Resources) != 3 {
		t.Fatalf("len(resources) = %d, want 3", len(contract.Resources))
	}

	if contract.Resources[0].Address != "module.shared.module.platform.module.asg.aws_autoscaling_group.worker_group[0]" {
		t.Fatalf("address = %q, want nested module ASG address", contract.Resources[0].Address)
	}

	got := contract.Resources[0].DesiredGeneration
	if len(got) != 2 || got[0].Type != "tag" || got[0].Key != "rotation" || got[0].Value != "green" ||
		got[1].Type != "launch_template" ||
		got[1].Key != "version" ||
		got[1].Value != "4" {
		t.Fatalf("desired generation = %#v, want tag rotation=green and launch_template version=4", got)
	}
}

func TestLoadPlanErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		if _, err := LoadPlan(filepath.Join(t.TempDir(), "missing.json")); err == nil {
			t.Fatal("LoadPlan returned nil error, want read error")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "invalid.json")
		if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
			t.Fatalf("os.WriteFile returned error: %v", err)
		}
		if _, err := LoadPlan(path); err == nil {
			t.Fatal("LoadPlan returned nil error, want decode error")
		}
	})
}

func TestBuildContractFiltersChangesAndUsesCustomIndicators(t *testing.T) {
	plan := TerraformPlan{ResourceChanges: []ResourceChange{
		{
			Address: "aws_instance.not_an_asg",
			Type:    "aws_instance",
			Change:  Change{After: map[string]any{"custom": "ignored"}},
		},
		{
			Address: "aws_autoscaling_group.unmatched",
			Type:    "aws_autoscaling_group",
			Change:  Change{After: map[string]any{"custom": "ignored"}},
		},
		{
			Address: "aws_autoscaling_group.no_indicator",
			Type:    "aws_autoscaling_group",
			Change:  Change{After: map[string]any{"name": "no-indicator"}},
		},
		{
			Address: "module.app.aws_autoscaling_group.custom",
			Type:    "aws_autoscaling_group",
			Change: Change{After: map[string]any{
				"name":   "custom-name",
				"custom": "generation-7",
			}},
		},
	}}

	contract, err := BuildContract(plan, ASGDefaultPolicy, `module\.app\..*`, []string{"custom"})
	if err != nil {
		t.Fatalf("BuildContract returned error: %v", err)
	}
	if len(contract.Resources) != 1 {
		t.Fatalf("len(resources) = %d, want 1", len(contract.Resources))
	}
	resource := contract.Resources[0]
	if resource.Name != "custom-name" {
		t.Fatalf("resource name = %q, want %q", resource.Name, "custom-name")
	}
	if len(resource.DesiredGeneration) != 1 || resource.DesiredGeneration[0].Type != "indicator" ||
		resource.DesiredGeneration[0].Key != "custom" ||
		resource.DesiredGeneration[0].Value != "generation-7" {
		t.Fatalf("desired generation = %#v, want indicator custom=generation-7", resource.DesiredGeneration)
	}
}

func TestBuildContractCollectsMultipleGenerationRequirements(t *testing.T) {
	plan := TerraformPlan{ResourceChanges: []ResourceChange{{
		Address: "aws_autoscaling_group.example",
		Type:    "aws_autoscaling_group",
		Change: Change{After: map[string]any{
			"tag":             map[string]any{"rotation": "green"},
			"launch_template": map[string]any{"version": "4"},
			"name":            "example",
		}},
	}}}

	contract, err := BuildContract(plan, ASGDefaultPolicy, "", nil)
	if err != nil {
		t.Fatalf("BuildContract returned error: %v", err)
	}
	got := contract.Resources[0].DesiredGeneration
	if len(got) != 2 {
		t.Fatalf("len(desired generation) = %d, want 2: %#v", len(got), got)
	}
	if got[0] != (GenerationRequirement{Type: "tag", Key: "rotation", Value: "green"}) {
		t.Fatalf("first requirement = %#v, want tag rotation=green", got[0])
	}
	if got[1] != (GenerationRequirement{Type: "launch_template", Key: "version", Value: "4"}) {
		t.Fatalf("second requirement = %#v, want launch_template version=4", got[1])
	}
}

func TestBuildContractRejectsInvalidRegex(t *testing.T) {
	_, err := BuildContract(TerraformPlan{}, ASGDefaultPolicy, "[", nil)
	if err == nil {
		t.Fatal("BuildContract returned nil error, want regex error")
	}
}

func TestMatchedResourceNames(t *testing.T) {
	plan := TerraformPlan{ResourceChanges: []ResourceChange{
		{
			Address: "aws_autoscaling_group.after",
			Type:    "aws_autoscaling_group",
			Change:  Change{Before: map[string]any{"name": "old"}, After: map[string]any{"name": "new"}},
		},
		{
			Address: "aws_autoscaling_group.before",
			Type:    "aws_autoscaling_group",
			Change:  Change{Before: map[string]any{"name": "destroyed"}},
		},
		{
			Address: "aws_autoscaling_group.missing",
			Type:    "aws_autoscaling_group",
			Change:  Change{After: map[string]any{"name": ""}},
		},
		{
			Address: "aws_instance.ignored",
			Type:    "aws_instance",
			Change:  Change{After: map[string]any{"name": "instance"}},
		},
	}}

	names, err := MatchedResourceNames(plan, ASGDefaultPolicy, "")
	if err != nil {
		t.Fatalf("MatchedResourceNames returned error: %v", err)
	}
	want := []string{"new", "destroyed"}
	if len(names) != len(want) {
		t.Fatalf("names = %#v, want %#v", names, want)
	}
	for index := range want {
		if names[index] != want[index] {
			t.Fatalf("names = %#v, want %#v", names, want)
		}
	}

	if _, err := MatchedResourceNames(plan, ASGDefaultPolicy, "["); err == nil {
		t.Fatal("MatchedResourceNames returned nil error, want regex error")
	}
}

func TestNestedValue(t *testing.T) {
	list := []any{
		"not an object",
		map[string]any{"key": "rotation", "value": "blue"},
		map[string]any{"version": "5"},
	}
	object := map[string]any{
		"nested": map[string]any{"value": "found"},
		"list":   list,
		"empty":  "",
		"nil":    nil,
	}

	tests := []struct {
		want any
		name string
		path string
		ok   bool
	}{
		{name: "nested object", path: "nested.value", want: "found", ok: true},
		{name: "list keyed value", path: "list.rotation", want: "blue", ok: true},
		{name: "list object field", path: "list.version", want: "5", ok: true},
		{name: "missing path", path: "missing", ok: false},
		{name: "scalar cannot be traversed", path: "empty.value", ok: false},
		{name: "empty value", path: "empty", ok: false},
		{name: "nil value", path: "nil", ok: false},
		{name: "nil object", path: "value", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := object
			if test.name == "nil object" {
				input = nil
			}
			got, ok := nestedValue(input, test.path)
			if ok != test.ok || got != test.want {
				t.Fatalf(
					"nestedValue(%#v, %q) = (%#v, %t), want (%#v, %t)",
					input,
					test.path,
					got,
					ok,
					test.want,
					test.ok,
				)
			}
		})
	}
}
