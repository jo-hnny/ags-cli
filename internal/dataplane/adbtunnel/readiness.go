package adbtunnel

import "github.com/TencentCloudAgentRuntime/ags-cli/internal/output"

// ReadyMessage is the private readiness protocol between the same CLI binary's
// mobile connect parent and tunnel child. Error payloads are sanitized by the child.
type ReadyMessage struct {
	Status   string          `json:"status"`
	Port     int             `json:"port,omitzero"`
	PID      int             `json:"pid,omitzero"`
	Failure  *output.Failure `json:"failure,omitempty"`
	ExitCode int             `json:"exit_code,omitzero"`
}
