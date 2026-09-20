terraform {
  source = "${get_repo_root()}/testdata/aws-live/terraform"
}

inputs = {
  region     = get_env("AWS_REGION", "eu-central-1")
  test_id    = get_env("CONVERGENCI_LIVE_TEST_ID")
  generation = get_env("CONVERGENCI_LIVE_GENERATION", "blue")
  instance_warmup_seconds = tonumber(get_env("CONVERGENCI_LIVE_INSTANCE_WARMUP_SECONDS", "1"))
}
