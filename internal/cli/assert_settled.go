package cli

import (
	"encoding/json"
	"strings"

	"github.com/iilei/convergenci-cli/internal/awscmd"
)

// asgInstanceRefreshInProgress reports whether the named ASG currently has an
// instance refresh with status InProgress or Pending.
func asgInstanceRefreshInProgress(asgName string, cfg awscmd.Config) (bool, error) {
	resp, err := cfg.Run("autoscaling", "describe-instance-refreshes", "--auto-scaling-group-name", asgName)
	if err != nil {
		return false, err
	}
	var payload struct {
		InstanceRefreshes []struct {
			Status string `json:"Status"`
		} `json:"InstanceRefreshes"`
	}
	if err := json.Unmarshal(resp, &payload); err != nil {
		return false, err
	}
	for _, refresh := range payload.InstanceRefreshes {
		switch strings.ToUpper(refresh.Status) {
		case awsStatusInProgress, "PENDING":
			return true, nil
		}
	}
	return false, nil
}
