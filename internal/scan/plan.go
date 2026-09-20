package scan

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// TerraformPlan is the minimal Terraform JSON plan shape required by the ASG scan implementation.
type TerraformPlan struct {
	ResourceChanges []ResourceChange `json:"resource_changes"`
}

// ResourceChange is a single Terraform resource change entry.
type ResourceChange struct {
	Address string `json:"address"`
	Type    string `json:"type"`
	Change  Change `json:"change"`
}

// Change contains the before/after values for the resource.
type Change struct {
	Before map[string]any `json:"before"`
	After  map[string]any `json:"after"`
}

// RecordPolicy defines the built-in matching behavior for a convergence record type.
type RecordPolicy struct {
	Name         string
	ResourceType string
	DefaultRegex string
	DefaultPaths []string
}

// ASGDefaultPolicy is the built-in policy for the current ASG instance refresh implementation.
var ASGDefaultPolicy = RecordPolicy{
	Name:         "asg",
	ResourceType: "aws_autoscaling_group",
	DefaultRegex: `(?i).*aws_autoscaling_group.*`,
	DefaultPaths: []string{
		"tags.rotation",
		"tags.convergenci_rotation",
		"launch_template.version",
		"mixed_instances_policy.launch_template.version",
	},
}

// Contract is the generated convergence contract written by the scan command.
type Contract struct {
	SchemaVersion int            `json:"schema_version"`
	Resources     []ContractItem `json:"resources"`
}

// ContractItem is a single resource expectation in the convergence contract.
type ContractItem struct {
	Address           string         `json:"address"`
	Kind              string         `json:"kind"`
	Status            string         `json:"status,omitempty"`
	DesiredGeneration map[string]any `json:"desired_generation"`
	Observation       Observation    `json:"observation"`
}

// Observation captures the AWS runtime strategy expected for the resource.
type Observation struct {
	Strategy  string   `json:"strategy"`
	Fulfilled *bool    `json:"fulfilled"`
	TimeSpent *float64 `json:"timeSpent"`
	ARN       string   `json:"arn,omitempty"`
}

// LoadPlan reads a Terraform plan JSON file from disk.
func LoadPlan(path string) (TerraformPlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TerraformPlan{}, err
	}

	var plan TerraformPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return TerraformPlan{}, err
	}
	return plan, nil
}

// BuildContract converts a Terraform plan into a convergence contract using the provided policy.
func BuildContract(plan TerraformPlan, policy RecordPolicy, addressRegex string, extraIndicators []string) (Contract, error) {
	if addressRegex == "" {
		addressRegex = policy.DefaultRegex
	}

	compiled, err := regexp.Compile(addressRegex)
	if err != nil {
		return Contract{}, err
	}

	paths := append([]string{}, policy.DefaultPaths...)
	paths = append(paths, extraIndicators...)

	contract := Contract{SchemaVersion: 1}
	for _, change := range plan.ResourceChanges {
		if change.Type != policy.ResourceType {
			continue
		}
		if !compiled.MatchString(change.Address) {
			continue
		}

		value, key, ok := firstIndicatorValue(change.Change.After, paths)
		if !ok {
			continue
		}

		entry := ContractItem{
			Address: change.Address,
			Kind:    "aws_asg",
			DesiredGeneration: map[string]any{
				key: value,
			},
			Observation: Observation{Strategy: "instance_refresh"},
		}
		contract.Resources = append(contract.Resources, entry)
	}
	return contract, nil
}

// MatchedResourceNames returns the AWS resource names (e.g. ASG names) for plan resource
// changes matching the given policy, using the same matching rules as BuildContract. This
// lets callers like assert-all-settled identify AWS resources directly from a plan without
// requiring a previously generated convergence contract or explicit resource names.
func MatchedResourceNames(plan TerraformPlan, policy RecordPolicy, addressRegex string) ([]string, error) {
	if addressRegex == "" {
		addressRegex = policy.DefaultRegex
	}

	compiled, err := regexp.Compile(addressRegex)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, change := range plan.ResourceChanges {
		if change.Type != policy.ResourceType {
			continue
		}
		if !compiled.MatchString(change.Address) {
			continue
		}
		if name, ok := resourceName(change); ok {
			names = append(names, name)
		}
	}
	return names, nil
}

// resourceName resolves the AWS resource name from a plan resource change, preferring the
// after state and falling back to before (e.g. for destroy-only changes).
func resourceName(change ResourceChange) (string, bool) {
	if name, ok := stringValue(change.Change.After, "name"); ok {
		return name, true
	}
	if name, ok := stringValue(change.Change.Before, "name"); ok {
		return name, true
	}
	return "", false
}

func stringValue(obj map[string]any, path string) (string, bool) {
	value, ok := nestedValue(obj, path)
	if !ok {
		return "", false
	}
	s, ok := value.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

// firstIndicatorValue walks indicator paths in order and returns the first non-empty value.
func firstIndicatorValue(obj map[string]any, paths []string) (any, string, bool) {
	for _, p := range paths {
		value, ok := nestedValue(obj, p)
		if !ok {
			continue
		}
		return value, indicatorKey(p), true
	}
	return nil, "", false
}

func indicatorKey(path string) string {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return path
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	switch last {
	case "rotation", "convergenci_rotation":
		return "rotation"
	case "version":
		return "version"
	default:
		return last
	}
}

// nestedValue resolves a dotted path into a nested value from the JSON object.
func nestedValue(obj map[string]any, path string) (any, bool) {
	if obj == nil {
		return nil, false
	}
	curr := any(obj)
	for _, part := range strings.Split(path, ".") {
		v, ok := curr.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := v[part]
		if !ok {
			return nil, false
		}
		curr = next
	}

	if curr == nil {
		return nil, false
	}
	if s, ok := curr.(string); ok && s == "" {
		return nil, false
	}
	return curr, true
}
