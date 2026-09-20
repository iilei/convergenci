package cli

import (
	"os"
	"testing"
)

func fakeAWSCLIPath(t *testing.T) string {
	t.Helper()
	path := os.Getenv("CONVERGENCI_AWS_CLI_PATH")
	if path == "" {
		t.Fatal("CONVERGENCI_AWS_CLI_PATH is required for tests that use fake AWS")
	}
	return path
}
