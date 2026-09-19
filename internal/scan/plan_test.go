package scan

import "testing"

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

	got, ok := contract.Resources[0].DesiredGeneration["rotation"]
	if !ok {
		t.Fatalf("desired generation missing rotation key: %#v", contract.Resources[0].DesiredGeneration)
	}
	if got != "bb" {
		t.Fatalf("desired generation = %#v, want %q", got, "bb")
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

	got, ok := contract.Resources[0].DesiredGeneration["rotation"]
	if !ok {
		t.Fatalf("desired generation missing rotation key: %#v", contract.Resources[0].DesiredGeneration)
	}
	if got != "v2" {
		t.Fatalf("desired generation = %#v, want %q", got, "v2")
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

	got, ok := contract.Resources[0].DesiredGeneration["version"]
	if !ok {
		t.Fatalf("desired generation missing version key: %#v", contract.Resources[0].DesiredGeneration)
	}
	if got != "4" {
		t.Fatalf("desired generation = %#v, want %q", got, "4")
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

	got, ok := contract.Resources[0].DesiredGeneration["rotation"]
	if !ok {
		t.Fatalf("desired generation missing rotation key: %#v", contract.Resources[0].DesiredGeneration)
	}
	if got != "green" {
		t.Fatalf("desired generation = %#v, want %q", got, "green")
	}
}
