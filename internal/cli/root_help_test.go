package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestExplicitHelpTopicAllowsHelpCommand(t *testing.T) {
	if err := explicitHelpTopicError([]string{"help", "help"}); err != nil {
		t.Fatalf("help help returned error: %v", err)
	}
	if err := explicitHelpTopicError([]string{"help", "help", "-o", "json"}); err != nil {
		t.Fatalf("help help -o json returned error: %v", err)
	}
}

func TestExplicitHelpTopicRejectsUnknownCommand(t *testing.T) {
	err := explicitHelpTopicError([]string{"help", "not-a-command"})
	if err == nil {
		t.Fatal("expected unknown help topic error")
	}
	if !strings.Contains(err.Error(), "Unknown help topic [not-a-command]") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigCommandRejectsUnknownSubcommandArgument(t *testing.T) {
	if err := configCmd.Args(configCmd, []string{"nope"}); err == nil {
		t.Fatal("expected config command to reject extra args")
	}
}

func TestInferRequestedCommandID(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{args: []string{}, want: "agr"},
		{args: []string{"instance", "list", "-o", "json"}, want: "instance.list"},
		{args: []string{"instnace", "list", "-o", "json"}, want: "instnace.list"},
		{args: []string{"help", "instance", "list", "-o", "json"}, want: "help.instance.list"},
	}

	for _, tc := range cases {
		if got := inferRequestedCommandID(tc.args); got != tc.want {
			t.Fatalf("inferRequestedCommandID(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestIsNDJSONAllowedCommand(t *testing.T) {
	root := &cobra.Command{Use: "agr"}
	instance := &cobra.Command{Use: "instance"}
	code := &cobra.Command{Use: "code"}
	run := &cobra.Command{Use: "run"}
	exec := &cobra.Command{Use: "exec"}
	tool := &cobra.Command{Use: "tool"}
	toolExec := &cobra.Command{Use: "exec"}

	root.AddCommand(instance, tool)
	instance.AddCommand(code, exec)
	code.AddCommand(run)
	tool.AddCommand(toolExec)

	if !isNDJSONAllowedCommand(run) {
		t.Fatal("expected instance.code.run to allow ndjson")
	}
	if !isNDJSONAllowedCommand(exec) {
		t.Fatal("expected instance.exec to allow ndjson")
	}
	if isNDJSONAllowedCommand(toolExec) {
		t.Fatal("expected tool.exec to reject ndjson")
	}
}
