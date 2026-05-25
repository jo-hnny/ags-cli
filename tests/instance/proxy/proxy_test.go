package proxy_test

import (
	"context"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/tests/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInstanceProxyCommand(t *testing.T) {
	testutil.RunSpecs(t, "AGR instance proxy command live smoke")
}

var _ = BeforeSuite(testutil.SetupSuite)
var _ = AfterSuite(testutil.CleanupSuite)

var _ = Describe("instance proxy", func() {
	It("executes agr instance proxy validation path", func() {
		cli := testutil.NewCLI()
		DeferCleanup(cli.Cleanup)
		cli.InitConfig()

		result := cli.Run(context.Background(), "instance", "proxy", "ins-live-placeholder", "0")
		result.ExpectExit(2)
		Expect(result.Stderr).To(ContainSubstring("invalid port"), result.Diagnostics())
	})
})
