package cli

import (
	requestio "github.com/TencentCloudAgentRuntime/ags-cli/internal/cli/request"
	"github.com/spf13/cobra"
)

// requestConflict returns a non-empty description when the user has set
// both --request and one of the generated long flags / aliases /
// shorthands for `commandID`. Empty string means no conflict.
//
// The conflict set is derived from the api generator output so that
// adding a new request field automatically extends the check.
func requestConflict(cmd *cobra.Command, commandID string) string {
	return requestio.Conflict(cmd, commandID)
}

// requestConflictDetail wraps requestConflict for callers that want a
// formatted message but no error type.
func requestConflictDetail(cmd *cobra.Command, commandID string) string {
	return requestio.ConflictDetail(cmd, commandID)
}
