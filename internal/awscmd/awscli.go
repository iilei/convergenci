// Package awscmd runs AWS CLI commands with shared execution settings.
package awscmd

import (
	"context"
	"flag"
	"os"
	"os/exec"
)

const (
	awsCLIPathEnv           = "CONVERGENCI_AWS_CLI_PATH"
	awsProfileArgumentCount = 2
)

// Config is the thin execution configuration shared by scan and await.
type Config struct {
	BinaryPath string
	Profile    string
}

func defaultAWSCLIPath() string {
	if path := os.Getenv(awsCLIPathEnv); path != "" {
		return path
	}
	return "aws"
}

// DefaultConfig returns the default AWS CLI execution settings.
func DefaultConfig() Config {
	return Config{BinaryPath: defaultAWSCLIPath()}
}

// RegisterFlags adds the shared AWS CLI flags and returns the configured values.
func RegisterFlags(fs *flag.FlagSet) (*string, *string) {
	binaryPath := fs.String("aws-cli-path", "aws", "path to the AWS CLI binary")
	profileName := fs.String("aws-profile-name", "", "AWS profile name to use for AWS CLI calls")
	return binaryPath, profileName
}

// UsageText returns the shared AWS CLI usage block for command help.
func UsageText() string {
	return "  --aws-cli-path     Path to the AWS CLI binary (default: aws)\n" +
		"  --aws-profile-name AWS profile name to use for AWS CLI calls\n"
}

// Args builds the AWS CLI arguments with any explicit profile override.
func (c Config) Args(service string, args ...string) []string {
	cmdArgs := make([]string, 0, awsProfileArgumentCount+len(args))
	if c.Profile != "" {
		cmdArgs = append(cmdArgs, "--profile", c.Profile)
	}
	cmdArgs = append(cmdArgs, service)
	cmdArgs = append(cmdArgs, args...)
	return cmdArgs
}

// Run executes an AWS CLI command with the current process environment.
func (c Config) Run(service string, args ...string) ([]byte, error) {
	path := c.BinaryPath
	if path == "" {
		path = "aws"
	}

	cmdArgs := c.Args(service, args...)
	// #nosec G204 -- BinaryPath is an explicit CLI option for selecting the installed AWS CLI.
	cmd := exec.CommandContext(context.Background(), path, cmdArgs...)
	cmd.Env = os.Environ()
	return cmd.Output()
}
