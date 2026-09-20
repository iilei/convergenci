# Convergenci

[![codecov](https://codecov.io/gh/iilei/convergenci-cli/graph/badge.svg?token=RMVFXWV6SQ)](https://codecov.io/gh/iilei/convergenci-cli)

Convergenci is a small AWS-bound CLI that verifies that an infrastructure change has converged to the intended runtime state.

The tool does not perform infrastructure changes itself. Terraform remains responsible for applying changes. Convergenci observes AWS after the change and determines whether the expected postcondition has been reached.

## Design principle

> Plan intent first, observe runtime state afterward, and determine convergence from explicit postconditions.

Terraform answers: what should change?
AWS answers: what actually happened?
Convergenci answers: has the intended runtime state converged?

```mermaid
flowchart LR
    subgraph P1["1 . plan"]
        A["terraform plan -out=tfplan"] -->|"terraform CLI"| A2["tfplan.json"]
    end

    subgraph P2["2 . scan"]
        B["convergenci scan\n(optional --assert-all-settled)"] -->|"AWS CLI"| B2["AWS runtime state"]
        B --> C["convergence.json"]
    end

    subgraph P3["3 . apply"]
        D["terraform apply tfplan"] -->|"terraform CLI"| D2["AWS"]
    end

    subgraph P4["4 . await"]
        E["convergenci await"] -->|"AWS CLI"| E2["AWS runtime state"]
        E --> F["convergence-report.json"]
    end

    subgraph P5["5 . report"]
        G["convergenci report"] --> H["human-readable summary"]
    end

    A2 --> B
    C --> D
    D2 --> E
    F --> G
```

## Scope

The initial implementation is intentionally AWS-specific and focuses on:

- EC2 Auto Scaling Groups (ASG)

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

convergenci scan --output-json convergence.json tfplan.json

convergenci scan --assert-all-settled tfplan.json

terraform apply tfplan

convergenci await convergence.json
```

## Billable AWS tests

The repository includes opt-in end-to-end tests that create a small Auto Scaling
Group in `eu-central-1`, exercise the Terraform plan, convergence scan,
`assert-all-settled`, await, and report flow, and destroy the resources on exit.
They are separate from the local fake-AWS benches because they create billable
AWS resources.

### Authentication and prerequisites

Install the repository-managed tools first:

```bash
mise install
```

The live tests also require the AWS CLI. Use
an isolated AWS account or sandbox role with permission to manage the test
resources. The test uses the normal AWS credential chain; it does not read
credentials from repository files.

For AWS SSO, configure a profile interactively and log in before running the
test:

```bash
aws configure sso --profile convergenci-sandbox
aws sso login --profile convergenci-sandbox
export AWS_PROFILE=convergenci-sandbox
```

For an assumed role or CI identity, configure the standard AWS environment
variables or web-identity credential variables instead. Do not commit access
keys, session tokens, or backend credentials.

The test is fixed to `eu-central-1` and requires an explicit opt-in:

```bash
export ALLOW_BILLABLE_TEST=true
mise run billable-terraform-test
```

The live instance-refresh warmup defaults to one second for a quick test. Set
`CONVERGENCI_LIVE_INSTANCE_WARMUP_SECONDS` to a larger non-negative integer to
make polling and refresh progress easier to observe:

```bash
CONVERGENCI_LIVE_INSTANCE_WARMUP_SECONDS=30 \
  mise run billable-terraform-test
```

To verify timeout handling, use the dedicated scenario. It intentionally sets
the await timeout below the instance warmup and succeeds only when timeout is
reported as expected:

```bash
mise run billable-terraform-test-timeout
```

The live-test controls are:

- `CONVERGENCI_LIVE_INSTANCE_WARMUP_SECONDS`: ASG instance-refresh warmup;
  defaults to `1`
- `CONVERGENCI_LIVE_AWAIT_TIMEOUT`: maximum time passed to `await`; defaults to
  `5m`
- `CONVERGENCI_LIVE_AWAIT_INTERVAL`: polling interval passed to `await`;
  defaults to `5s`
- `CONVERGENCI_LIVE_EXPECT_TIMEOUT`: when `true`, the harness succeeds only if
  `await` exits with timeout code `230`

The dedicated timeout task fixes warmup at `30` seconds and timeout at `5`
seconds. Seeing one `Pending` observation followed by exit code `230` is its
expected success condition, not a failed convergence test. Use
`billable-terraform-test` or `billable-terragrunt-test` when the expected result
is convergence.

Run the Terragrunt variant with:

```bash
export ALLOW_BILLABLE_TEST=true
export AWS_PROFILE=convergenci-sandbox
mise run billable-terragrunt-test
```

The test uses the default VPC and one small `t3.micro` instance. It does not
create a NAT gateway, load balancer, database, or other supporting service.
Every resource receives a unique `convergenci-test-id` tag. Cleanup is installed
as an exit trap and runs Terraform or Terragrunt destroy even when the test
fails or is interrupted. A failed process should still be checked manually in
the AWS console before the sandbox is reused.

`AWS_PROFILE` and the other standard AWS credential variables are inherited by
the AWS CLI subprocess. Debug output such as `profile=""` means no explicit
`--aws-profile-name` option was supplied; it does not mean that `AWS_PROFILE`
was ignored. Use `--aws-profile-name` only when an explicit per-command profile
override is desired.

`scan` is the stateless plan-to-contract step: it resolves the relevant AWS
resources from the Terraform plan and emits a convergence contract.

`scan --assert-all-settled` uses the same plan-based resource resolution as plain
`scan` and adds one preflight guard: it exits non-zero if any relevant resource
is already in progress. It behaves like the plain scan path for resource
selection and any scan-phase side effects, and it only adds the extra fail-fast
check before returning.

`scan --assert-all-settled` assumes the caller holds a Terraform state lock for
the duration of `apply`, so at most one relevant change can be in flight.
Under that assumption, correlation between an apply and its runtime effect no
longer needs to resolve concurrent/superseding changes (see the ADR for
details) — it only needs a pre-apply/post-apply boundary.

### Refresh correlation edge case

The state lock does not cover the complete shell sequence from `plan` through
`scan`, `apply`, and `await`. Use an external CI/job lock when multiple
deployments could otherwise run concurrently. Always apply the exact saved plan
that was scanned, and invoke `scan --assert-all-settled` immediately before
that apply.

For ASGs, a previous successful instance refresh can still be visible when
`await` starts. A successful observation must therefore be understood as the
refresh produced by the just-applied plan, not merely as any historical
successful refresh. Avoid reusing an old convergence contract or report, keep
the plan, contract, and report together, and treat a preflight failure as a
reason to stop rather than to continue with `await`.

The invariant to verify in integration tests is:

```text
assert-all-settled succeeds
apply the saved plan
await does not accept the previous refresh as this apply's result
await accepts the new refresh only after its expected trigger converges
```

If the workflow cannot guarantee that boundary, do not infer convergence from
an already-successful runtime state; serialize the deployment externally and
rerun the plan/scan/apply sequence.

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
- AWS CLI path can also be set with `CONVERGENCI_AWS_CLI_PATH`; an explicit `--aws-cli-path` flag takes precedence
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
- for each matched resource, observes AWS and exits non-zero if a relevant
  runtime operation (an ASG instance refresh) is currently in progress
- differs from plain `scan` only in the terminal outcome: plain `scan` emits a
  contract, while `scan --assert-all-settled` performs a preflight assertion and
  returns a failing exit status when any relevant resource is not settled
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
- contracts currently emitted by `scan` use `schema_version: 2`; `await` does
  not negotiate schema compatibility, so regenerate the plan and contract
  after upgrading rather than reusing an artifact created by another version

Example:

```bash
convergenci await --timeout 10m --interval 15s ./artifacts/asg-plan.convergence.json
convergenci await --force ./artifacts/asg-plan.convergence.json
```

#### `report`

Render a convergence report file (produced by `await`) as human-readable text.

```bash
convergenci report <convergence-report.json>
```

Behavior:

- uses the built-in `templates/report-as-text.tmpl` template, embedded into the
  binary at build time, so no external template file or `gomplate` install is
  required
- reads the JSON report given as a positional argument and prints the
  rendered text to stdout
- enables ANSI colors automatically only when stdout is an interactive terminal
  and `NO_COLOR` is unset
- `CONVERGENCI_COLOR=always` (or `true`) forces color, overriding automatic
  detection; `CONVERGENCI_COLOR=never` (or `false`) disables it

Example:

```bash
convergenci report ./artifacts/asg-plan.convergence-report.json
```

#### `doctor`

Diagnose AWS access before running convergence checks. `doctor` verifies that
the configured AWS CLI is available and that the active credentials can call
`sts get-caller-identity`. This makes it useful for checking the selected
profile, region-independent credential setup, and executable path before
debugging a failed `scan` or `await` run.

It can also render an existing report as a secondary convenience, using an
optional filesystem template.

```bash
convergenci doctor [--template PATH] [--aws-cli-path PATH] [--aws-profile-name NAME] [report.json]
```

Without a report argument, `doctor` performs only the AWS diagnostic. With a
report argument, it performs the diagnostic first and then renders that JSON
file using `--template`. An explicitly supplied template path is read from the
filesystem; the default path is `templates/report-as-text.tmpl` relative to the
current working directory. For ordinary report rendering without an external
template file, use `report` instead.

Example:

```bash
convergenci doctor
convergenci doctor --template ./templates/report-as-text.tmpl ./artifacts/asg-plan.convergence-report.json
```

### Exit codes

Commands return `0` on success. Structured CLI failures use these exit codes:

|  Code | Meaning                                                               |
| ----: | --------------------------------------------------------------------- |
| `201` | convergence contract exceeds the supported resource limit             |
| `210` | invalid command configuration or arguments                            |
| `220` | input/output or artifact error                                        |
| `230` | convergence timeout                                                   |
| `240` | convergence, AWS observation, rendering, or other operational failure |

The billable timeout task explicitly treats `230` as its expected outcome.

## End-to-end example

The repository ships a fake AWS CLI and a multi-stack Terragrunt-style fixture
so the full `scan` → `--assert-all-settled` → `await` → `report` flow can be
exercised locally, without touching real AWS. This is the output of
`mise run fake-aws-scenarios-terragrunt`:

```text
$ convergenci scan stubs/terragrunt-multi-stack.json \
    --output-json .bench/terragrunt-multi-stack/.convergence.json \
    --aws-cli-path ./testdata/fake-aws-bin/aws
aws-cli-path: /path/to/testdata/fake-aws-bin/aws
output-json: .bench/terragrunt-multi-stack/.convergence.json

$ FAKE_AWS_SCENARIO=success convergenci scan stubs/terragrunt-multi-stack.json \
    --assert-all-settled --aws-cli-path ./testdata/fake-aws-bin/aws
aws-cli-path: /path/to/testdata/fake-aws-bin/aws
all resources settled

$ FAKE_AWS_SCENARIO=in-progress convergenci scan stubs/terragrunt-multi-stack.json \
    --assert-all-settled --aws-cli-path ./testdata/fake-aws-bin/aws
aws-cli-path: /path/to/testdata/fake-aws-bin/aws
not settled: prod-us-east-1-web, prod-us-east-1-worker, prod-eu-west-1-web, prod-eu-west-1-worker (exit code 240)

$ FAKE_AWS_SCENARIO=success convergenci await \
    --timeout 3s --interval 100ms \
    .bench/terragrunt-multi-stack/.convergence.json \
    --aws-cli-path ./testdata/fake-aws-bin/aws
aws-cli-path: /path/to/testdata/fake-aws-bin/aws
timeout: 3s
interval: 100ms

$ convergenci report .bench/terragrunt-multi-stack/.convergence.convergence-report.json
Status: successful
Message: all resources converged
Contract: .bench/terragrunt-multi-stack/.convergence.json
Expected resources: 4
Pending resources: 0
Converged resources: 4
Started at: 2026-09-20T16:17:14.759948941Z
Finished at: 2026-09-20T16:17:16.065673125Z

---
Resources:
 * module.terragrunt["prod-eu-west-1"].module.platform.aws_autoscaling_group.web
   status: converged
   arn: arn:aws:autoscaling:us-east-1:123456789012:autoScalingGroup:::prod-eu-west-1-web
   strategy: instance_refresh
   desired: tag rotation=green launch_template version=8
 * module.terragrunt["prod-eu-west-1"].module.platform.aws_autoscaling_group.worker
   status: converged
   arn: arn:aws:autoscaling:us-east-1:123456789012:autoScalingGroup:::prod-eu-west-1-worker
   strategy: instance_refresh
   desired: tag rotation=green launch_template version=12
 * module.terragrunt["prod-us-east-1"].module.platform.aws_autoscaling_group.web
   status: converged
   arn: arn:aws:autoscaling:us-east-1:123456789012:autoScalingGroup:::prod-us-east-1-web
   strategy: instance_refresh
   desired: tag rotation=green launch_template version=8
 * module.terragrunt["prod-us-east-1"].module.platform.aws_autoscaling_group.worker
   status: converged
   arn: arn:aws:autoscaling:us-east-1:123456789012:autoScalingGroup:::prod-us-east-1-worker
   strategy: instance_refresh
   desired: tag rotation=green launch_template version=12
---
```

Run it yourself with:

```bash
mise run fake-aws-scenarios-terragrunt
```

## Debug logging

Debug logging is controlled by the global `DEBUG` environment variable and the `--debug` flag.

When debug logging is enabled, the default format is:

```text
[DEBUG 2026-09-19T12:00:00Z] status=InProgress pct=45
```

Override it with `CONVERGENCI_DEBUG_FORMAT` or the `--debug-format` flag.

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

For a fuller example of what a Go text template can do here — conditionals,
loops, and custom helper functions — see the built-in
[`templates/report-as-text.tmpl`](templates/report-as-text.tmpl). It is the
template `report` renders by default, and it is a good starting point for
power users who want to build a more elaborate `--debug-format`/
`CONVERGENCI_DEBUG_FORMAT` template than the one-liners above.

## Version output

The CLI version output follows:

```text
1.2.3 (commit: abc123, built at: 2026-09-19)
```

## Development checks

Install the pinned tools and run both project validation tasks before opening a
change:

```bash
mise install
mise run check
mise run lint
mise run lint-markdown
```

`mise run check` formats with the pinned `gofumpt`, runs `go vet`, builds the
fake AWS CLI, and executes the Go tests. Go and Markdown linting are separate
tasks and are not included in `check`.

## Notes

- The CLI uses the AWS CLI as its observation boundary rather than embedding AWS SDK code.
- The design intentionally keeps the implementation small and AWS-specific until concrete requirements demand more abstraction.
- The manifest produced by `scan` is the contract consumed by `await`.
