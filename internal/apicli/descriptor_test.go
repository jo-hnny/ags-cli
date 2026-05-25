package apicli

import (
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
)

func TestCommandSpecCanOmitRequestFlag(t *testing.T) {
	spec := APIDescriptor{
		Spec:               command.Spec{ID: "apikey.list"},
		DisableRequestFlag: true,
	}.CommandSpec()
	for _, flag := range spec.Flags {
		if flag.Name == "request" {
			t.Fatalf("CommandSpec unexpectedly added --request")
		}
	}
}

func TestCommandSpecAddsRequestFlagByDefault(t *testing.T) {
	spec := APIDescriptor{Spec: command.Spec{ID: "tool.list"}}.CommandSpec()
	for _, flag := range spec.Flags {
		if flag.Name == "request" {
			return
		}
	}
	t.Fatalf("CommandSpec did not add --request")
}
