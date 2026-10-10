package cli

import (
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/client"
	sdkerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
)

// The error output points users at `agr explain <Code>`, so explain must agree
// with the classifier rather than describe a client error as a server one.
func TestExplainCloudNetworkErrorMatchesClassification(t *testing.T) {
	const code = "ClientError.NetworkError"
	classified := client.ClassifyError(sdkerrors.NewTencentCloudSDKError(code, "Fail to get response", ""))
	explained, ok := explainCodeData(strings.ToUpper(code))
	if !ok || explained.Kind != classified.Failure.Kind || explained.ExitCode != classified.ExitCode || explained.Retryable != classified.Failure.Retryable {
		t.Fatalf("explain=%#v classified=%#v exit=%d", explained, classified.Failure, classified.ExitCode)
	}
	if strings.Contains(explained.Meaning, "server-side") {
		t.Fatalf("client network error explained as server error: %s", explained.Meaning)
	}
}
