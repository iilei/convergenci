# Fake AWS CLI for hand-rolled test benches

This directory contains a tiny Go-based AWS CLI shim intended only for local test benches and debugging.

It is not part of the production CLI binary and should not be used as a deploy-time dependency.

## Build

```bash
mise run build-fake-aws
```

This produces a binary named `aws` under `testdata/fake-aws-bin/`.

## Usage

Convergenci already accepts an AWS CLI path via `--aws-cli-path`, so you do not need to alter `PATH` for local bench runs.

```bash
export FAKE_AWS_SCENARIO=success
./convergenci await .convergence.json --timeout 2s --interval 100ms --aws-cli-path "$(pwd)/testdata/fake-aws-bin/aws"
```

If you still want to put the shim on `PATH`, that is optional, but it is not required.

## Supported scenarios

- `success` — nothing is in progress
- `failed` — a failed runtime state is reported
- `in-progress` — returns a realistic `InProgress` payload with partial completion such as `45%`
- `late-success` — returns `InProgress` for a short sequence of polls and then resolves to `Successful` once the delayed transition is reached
- `in-progress-then-success` — a transitional mode that stays in progress before eventually succeeding
- `terragrunt-multi-stack` — a transitional mode for the multi-stack Terragrunt-style bench
- `in-progress-then-failed` — a transitional mode that stays in progress before eventually failing

These scenarios are meant to exercise the exact guardrail and polling behavior used by the `scan --assert-all-settled` and `await` flows, without introducing a real AWS dependency.

Example:

```bash
export FAKE_AWS_SCENARIO=late-success
./convergenci await .convergence.json --timeout 2s --interval 100ms --aws-cli-path "$(pwd)/testdata/fake-aws-bin/aws"
```

The response fields mimic AWS roughly: `Status` reflects the lifecycle state and
`PercentageComplete` exposes how far along the rollout is. That makes the bench useful for
testing timeout, retry, and late-success behavior without introducing actual AWS dependencies.
