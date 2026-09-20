#!/usr/bin/env bash
set -euo pipefail

mode="${1:-}"
if [[ "$mode" != "terraform" && "$mode" != "terragrunt" ]]; then
  echo "usage: $0 terraform|terragrunt" >&2
  exit 2
fi

if [[ "$mode" == "terragrunt" ]] && ! command -v terragrunt >/dev/null; then
  echo "terragrunt is required for billable-terragrunt-test" >&2
  exit 2
fi

if [[ "${ALLOW_BILLABLE_TEST:-}" != "true" ]]; then
  echo "refusing billable test: set ALLOW_BILLABLE_TEST=true explicitly" >&2
  exit 2
fi

repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
work_dir="$repo_root/.bench/billable-$mode"
convergenci="$repo_root/convergenci"
test_id="convergenci-${mode}-$(date +%Y%m%d%H%M%S)-$$"
region="eu-central-1"
warmup_seconds="${CONVERGENCI_LIVE_INSTANCE_WARMUP_SECONDS:-1}"
await_timeout="${CONVERGENCI_LIVE_AWAIT_TIMEOUT:-5m}"
await_interval="${CONVERGENCI_LIVE_AWAIT_INTERVAL:-5s}"
expect_timeout="${CONVERGENCI_LIVE_EXPECT_TIMEOUT:-false}"
if [[ ! "$warmup_seconds" =~ ^[0-9]+$ ]]; then
  echo "CONVERGENCI_LIVE_INSTANCE_WARMUP_SECONDS must be a non-negative integer" >&2
  exit 2
fi

mkdir -p "$work_dir"
rm -f "$work_dir"/change.tfplan "$work_dir"/tfplan.json "$work_dir"/.convergence.json "$work_dir"/.convergence.convergence-report.json "$work_dir"/*.tf
cp "$repo_root/testdata/aws-live/terraform/main.tf" "$work_dir/main.tf"
if [[ -f "$repo_root/testdata/aws-live/terraform/.terraform.lock.hcl" ]]; then
  cp "$repo_root/testdata/aws-live/terraform/.terraform.lock.hcl" "$work_dir/.terraform.lock.hcl"
fi
if [[ "$mode" == "terragrunt" ]]; then
  cp "$repo_root/testdata/aws-live/terragrunt/terragrunt.hcl" "$work_dir/terragrunt.hcl"
fi

cleanup() {
  set +e
  if [[ "$mode" == "terraform" ]]; then
    terraform -chdir="$work_dir" destroy -auto-approve -input=false \
      -var "region=$region" -var "test_id=$test_id" -var "generation=green" \
      -var "instance_warmup_seconds=$warmup_seconds"
  else
    (cd "$work_dir" && \
      CONVERGENCI_LIVE_TEST_ID="$test_id" CONVERGENCI_LIVE_GENERATION=green \
      CONVERGENCI_LIVE_INSTANCE_WARMUP_SECONDS="$warmup_seconds" \
      AWS_REGION="$region" AWS_DEFAULT_REGION="$region" \
      terragrunt destroy --auto-approve --non-interactive)
  fi
}
trap cleanup EXIT INT TERM

run_tool() {
  if [[ "$mode" == "terraform" ]]; then
    terraform -chdir="$work_dir" "$@"
  else
    (cd "$work_dir" && \
      CONVERGENCI_LIVE_TEST_ID="$test_id" CONVERGENCI_LIVE_GENERATION="${CONVERGENCI_LIVE_GENERATION:-blue}" \
      CONVERGENCI_LIVE_INSTANCE_WARMUP_SECONDS="$warmup_seconds" \
      AWS_REGION="$region" AWS_DEFAULT_REGION="$region" terragrunt "$@")
  fi
}

if [[ "$mode" == "terraform" ]]; then
  run_tool init -input=false
  run_tool apply -auto-approve -input=false \
    -var "region=$region" -var "test_id=$test_id" -var "generation=blue" \
    -var "instance_warmup_seconds=$warmup_seconds"
else
  run_tool init --non-interactive
  run_tool apply --auto-approve --non-interactive
fi

if [[ "$mode" == "terraform" ]]; then
  run_tool plan -input=false \
    -var "region=$region" -var "test_id=$test_id" -var "generation=green" \
    -var "instance_warmup_seconds=$warmup_seconds" \
    -out=change.tfplan
else
  CONVERGENCI_LIVE_GENERATION=green run_tool plan --non-interactive -out=change.tfplan
fi
run_tool show -json change.tfplan >"$work_dir/tfplan.json"

AWS_REGION="$region" AWS_DEFAULT_REGION="$region" \
  "$convergenci" scan --assert-all-settled "$work_dir/tfplan.json"
"$convergenci" scan --output-json "$work_dir/.convergence.json" "$work_dir/tfplan.json"

if [[ "$mode" == "terraform" ]]; then
  run_tool apply -auto-approve -input=false change.tfplan
else
  CONVERGENCI_LIVE_GENERATION=green run_tool apply --auto-approve --non-interactive change.tfplan
fi

set +e
AWS_REGION="$region" AWS_DEFAULT_REGION="$region" \
  "$convergenci" await --force --timeout "$await_timeout" --interval "$await_interval" "$work_dir/.convergence.json"
await_exit=$?
set -e
if [[ "$expect_timeout" == "true" ]]; then
  if [[ "$await_exit" -ne 230 ]]; then
    echo "expected await timeout (exit code 230), got exit code $await_exit" >&2
    exit 1
  fi
elif [[ "$await_exit" -ne 0 ]]; then
  exit "$await_exit"
fi
AWS_REGION="$region" AWS_DEFAULT_REGION="$region" \
  "$convergenci" report "$work_dir/.convergence.convergence-report.json"
