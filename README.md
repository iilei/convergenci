# Convergenci

Convergenci is a small AWS-bound CLI that verifies that an infrastructure change has converged to the intended runtime state.

The tool does not perform infrastructure changes itself. Terraform remains responsible for applying changes. Convergenci observes AWS after the change and determines whether the expected postcondition has been reached.

## Design principle

> Plan intent first, observe runtime state afterward, and determine convergence from explicit postconditions.

Terraform answers: what should change?
AWS answers: what actually happened?
Convergenci answers: has the intended runtime state converged?

## Scope

The initial implementation is intentionally AWS-specific and focuses on:

- EC2 Auto Scaling Groups (ASG)
- Amazon ECS

It does not currently include:

- EKS
- generic cloud-provider abstractions
- Terraform apply orchestration
- AWS SDK integration
- arbitrary shell execution

## Workflow

```bash
terraform plan -out=tfplan
terraform show -json tfplan > tfplan.json

convergenci scan tfplan.json > convergence.json

convergenci scan --assert-all-settled tfplan.json

terraform apply tfplan

convergenci await convergence.json
```

`scan --assert-all-settled` assumes the caller holds a Terraform state lock for
the duration of `apply`, so at most one relevant change can be in flight.
Under that assumption, correlation between an apply and its runtime effect no
longer needs to resolve concurrent/superseding changes (see the ADR for
details) — it only needs a pre-apply/post-apply boundary.

## CLI

```text
convergenci [flags] [command]
```

### Global flags

- `-v, --version`: show the CLI version and build metadata
- `--debug`: enable debug logging
- `-h, --help`: show help output

### Commands

#### `scan`

Scan a Terraform plan and emit a convergence contract.

```bash
convergenci scan [--output-json PATH] [--jsonlines] [--aws-cli-path PATH] [--aws-profile-name NAME] <tfplan.json>
```

Behavior:

- defaults to a plan-derived output name such as `asg-default-rotation.convergence.json`
- with `--jsonlines` it writes `asg-default-rotation.convergence.jsonlines`
- if no plan input is supplied it falls back to `.convergence.json` or `.convergence.jsonlines`
- AWS CLI invocation can be directed with `--aws-cli-path` and `--aws-profile-name`
- output paths must be safe: absolute paths are allowed, but relative paths that escape the working directory via `..` are rejected
- existing output files are not overwritten unless `--force` is supplied

Example:

```bash
convergenci scan --output-json ./artifacts/asg-plan.convergence.json tfplan.json
convergenci scan --output-json /tmp/asg-plan.convergence.json --force tfplan.json
```

##### `scan --assert-all-settled`

Assert that nothing relevant is currently converging for the resources that
`scan` would otherwise turn into a contract, across all resource kinds
supported by convergenci (currently only ASG instance refreshes).

```bash
convergenci scan --assert-all-settled [--aws-cli-path PATH] [--aws-profile-name NAME] <tfplan.json>
```

Behavior:

- resolves AWS resource names (e.g. Auto Scaling Group names) from the
  Terraform plan, using the same matching rules `scan` uses to build a
  contract — no convergence contract or explicit resource names are required
- for each matched resource, observes AWS and fails if a relevant runtime
  operation (an ASG instance refresh) is currently in progress
- exits non-zero if any resource is not settled, so it can gate `terraform
  apply` in CI
- does not write a convergence contract or any other file in this mode; it is
  a point-in-time AWS check

This command assumes the caller holds a Terraform state lock (or otherwise
serializes applies) for the duration of `apply`, so it does not attempt to
resolve concurrent or superseding changes — it only establishes a
before/after boundary for correlation.

#### `await`

Wait for the runtime state described in a convergence contract to converge.

```bash
convergenci await [--timeout 5m] [--interval 5s] [--debug-format '{{ .Status }} {{ .PercentageComplete }}%'] [--aws-cli-path PATH] [--aws-profile-name NAME] <convergence.json>
```

Behavior:

- reads the contract produced by `scan`
- polls AWS until the expected resources converge or fail
- writes a sibling report next to the contract as `<stem>.convergence-report.json`
- defaults to refusing writes to an existing report file, unless `--force` is supplied
- supports timeout and polling interval overrides through environment variables:
  - `CONVERGENCI_AWAIT_TIMEOUT`
  - `CONVERGENCI_AWAIT_INTERVAL`

Example:

```bash
convergenci await --timeout 10m --interval 15s ./artifacts/asg-plan.convergence.json
convergenci await --force ./artifacts/asg-plan.convergence.json
```

## Debug logging

Debug logging is controlled by the global `DEBUG` environment variable and the `--debug` flag.

```bash
DEBUG=true convergenci --debug scan tfplan.json
```

The CLI uses a single package-scoped logger that is initialized once and then read from subsequent calls without re-passing configuration.

The debug output format can be customized with `CONVERGENCI_DEBUG_FORMAT` using a Go text template. The template receives the runtime object fields plus a few convenience values:

- `.Status`
- `.PercentageComplete`
- `.Prefix`
- `.Level`
- `.Timestamp`

Examples:

```bash
DEBUG=true \
CONVERGENCI_DEBUG_FORMAT='status={{ .Status }} pct={{ .PercentageComplete }}' \
convergenci await .convergence.json
```

```bash
DEBUG=true \
CONVERGENCI_DEBUG_FORMAT='[{{ .Level }} {{ .Timestamp }}] status={{ .Status }} pct={{ .PercentageComplete }}' \
convergenci await .convergence.json
```

This supports the common log prefix style:

```text
[DEBUG 2026-09-19T12:00:00Z] status=InProgress pct=45
```

## Version output

The CLI version output follows:

```text
1.2.3 (commit: abc123, built at: 2026-09-19)
```

## Notes

- The CLI uses the AWS CLI as its observation boundary rather than embedding AWS SDK code.
- The design intentionally keeps the implementation small and AWS-specific until concrete requirements demand more abstraction.
- The manifest produced by `scan` is the contract consumed by `await`.
