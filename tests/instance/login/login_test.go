package login_test

import (
	"context"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/tests/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInstanceLoginCommand(t *testing.T) {
	testutil.RunSpecs(t, "AGR instance login command live smoke")
}

var _ = BeforeSuite(testutil.SetupSuite)
var _ = AfterSuite(testutil.CleanupSuite)

var _ = Describe("instance login", func() {
	It("executes agr instance login validation path", func() {
		cli := testutil.NewCLI()
		DeferCleanup(cli.Cleanup)
		cli.InitConfig()

		result := cli.Run(context.Background(), "instance", "login", "ins-live-placeholder", "--non-interactive")
		result.ExpectExit(2)
		Expect(result.Stderr).To(ContainSubstring("requires"), result.Diagnostics())
	})
})
