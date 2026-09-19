# Scan implementation plan

## Goal

`scan` reads a Terraform plan JSON and produces a small convergence contract for the AWS runtime state that should be observed later by `await`.

For the current iteration, the only built-in checker is the ASG instance-refresh path.

## Scope for v1

- Input: Terraform plan JSON from `terraform show -json tfplan`
- Resource type covered: `aws_autoscaling_group`
- Trigger semantics: rotation indicator is the desired generation signal for an instance refresh
- Default selectors:
  - `tags.rotation`
  - `tags.convergenci_rotation`
  - `launch_template.version`
  - `mixed_instances_policy.launch_template.version`
- Matching behavior:
  - default resource matcher is `aws_autoscaling_group`
  - optional `--record asg` selects the built-in ASG policy
  - optional `--address-regex` narrows the matched addresses
  - optional repeated `--indicator` values append additional selectors

## Implementation steps

1. Parse Terraform plan JSON
   - read the root object
   - locate `resource_changes`
   - handle missing or malformed content as an error

2. Define built-in ASG policy
   - `record_name: asg`
   - `resource_type: aws_autoscaling_group`
   - default `address_regex: (?i).*aws_autoscaling_group.*`
   - default `indicators`:
     - `tags.rotation`
     - `tags.convergenci_rotation`
     - `launch_template.version`
     - `mixed_instances_policy.launch_template.version`

3. Match candidate resources
   - iterate over `resource_changes[]`
   - keep only resources whose `type` is `aws_autoscaling_group`
   - if `--address-regex` is provided, apply it to `address`
   - if the resource is not matched, ignore it

4. Extract the rotation indicator
   - inspect `change.after` for each candidate
   - walk each configured indicator path in order
   - for each path, read the nested value if it exists
   - use the first non-empty value as the desired generation signal
   - treat non-existent or empty values as “not present”

5. Build the convergence contract entry
   - `address`: Terraform resource address
   - `kind`: `aws_asg`
   - `desired_generation`: the chosen rotation indicator value
   - `observation.strategy`: `instance_refresh`
   - optionally include `source_indicator` for debugging/audit output

6. Emit the manifest
   - top-level JSON object with `schema_version`, `resources`, and related metadata
   - default output path: `.convergence.json`
   - `--jsonlines` writes one JSON object per line instead of pretty JSON

7. Keep the design extensible
   - encode the ASG record as a small built-in policy object
   - add future policies by name (for example `asg`, `ecs`)
   - keep record selection explicit and additive

## Example output contract

```json
{
  "schema_version": 1,
  "resources": [
    {
      "address": "module.app.aws_autoscaling_group.main",
      "kind": "aws_asg",
      "desired_generation": {
        "rotation": "bb"
      },
      "observation": {
        "strategy": "instance_refresh"
      }
    }
  ]
}
```

## Test strategy

Use fixtures in the `stubs/` directory to cover:

- default ASG rotation via `tags.rotation`
- custom ASG rotation via `tags.convergenci_rotation`
- launch template drift via `launch_template.version`
- mixed instance policy rotation via `mixed_instances_policy.launch_template.version`
- no indicator present => resource is ignored or emitted as empty/unsupported
- regex filtering via `--address-regex`
- repeated `--indicator` selection ordering

## Notes

- The design is intentionally resource-specific and additive.
- The runtime `await` step should later interpret the generated contract and poll AWS EC2 Auto Scaling Group state, especially instance refresh status and generation tracking.
- The scanner should not attempt to infer AWS runtime state; it should only encode a stable desired postcondition.
