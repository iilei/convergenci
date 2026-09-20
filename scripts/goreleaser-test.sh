#!/usr/bin/env sh
set -eu

mkdir -p ./testdata/fake-aws-bin
go build -o ./testdata/fake-aws-bin/aws ./testdata/fake-aws
CONVERGENCI_AWS_CLI_PATH="$PWD/testdata/fake-aws-bin/aws" go test -timeout 5m ./...
