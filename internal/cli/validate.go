package cli

import (
	"fmt"
	"net"
	"sort"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

func isNotFoundCLIError(err error) bool {
	cliErr := classifyCLIError(err)
	return cliErr != nil && cliErr.Failure != nil && cliErr.Failure.Kind == output.KindNotFound
}

func validateListenAddress(address string) error {
	if address == "localhost" {
		return nil
	}
	if net.ParseIP(address) == nil {
		return output.NewUsageError("INVALID_ADDRESS", fmt.Sprintf("invalid address %q: must be a valid IP address or \"localhost\"", address), "Use localhost, 127.0.0.1, ::1, or another valid IP address.")
	}
	return nil
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
