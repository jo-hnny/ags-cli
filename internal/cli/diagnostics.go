package cli

import (
	"fmt"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

func collectConfigWarnings(warning string) []string {
	if warning == "" {
		return nil
	}
	return []string{warning}
}

func requireNonEmptyValue(value, field, code, hint string) error {
	if strings.TrimSpace(value) == "" {
		return output.NewUsageError(code, fmt.Sprintf("%s is required", field), hint)
	}
	return nil
}

type configIssue struct {
	Name string
	Hint string
}

func describeConfigIssue(err error) configIssue {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "invalid output format:"):
		return configIssue{
			Name: "output",
			Hint: "Set output to 'text', 'json', or 'ndjson'.",
		}
	case strings.Contains(msg, "ndjson is only supported"):
		return configIssue{
			Name: "output",
			Hint: "Set output to 'text' or 'json', or override with -o text/-o json for this command.",
		}
	case strings.Contains(msg, "invalid region:"):
		return configIssue{
			Name: "region",
			Hint: "Set region to a valid value such as 'ap-guangzhou'.",
		}
	case strings.Contains(msg, "invalid domain:"):
		return configIssue{
			Name: "domain",
			Hint: "Set domain to a hostname without scheme or path.",
		}
	default:
		return configIssue{
			Name: "ConfigFile",
			Hint: "Fix the reported configuration issue and rerun the command.",
		}
	}
}

func newConfigUsageError(err error) *output.CLIError {
	issue := describeConfigIssue(err)
	return output.NewUsageError("INVALID_CONFIG", err.Error(), issue.Hint)
}
