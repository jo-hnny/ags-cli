package api_test

import (
	"context"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/tests/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAPICommands(t *testing.T) { testutil.RunSpecs(t, "AGR api command live smoke") }

var _ = BeforeSuite(testutil.SetupSuite)
var _ = AfterSuite(testutil.CleanupSuite)

var _ = Describe("api call", func() {
	It("executes agr api call", func() {
		cli := testutil.NewCLI()
		DeferCleanup(cli.Cleanup)
		cli.InitConfig()

		result := cli.Run(context.Background(), "--output", "json", "api", "call", "DescribeSandboxToolList", "--request", `{"Limit":1}`)
		result.ExpectSuccess()
		env := result.Envelope()
		Expect(env.Command).To(Equal("api.call"))
		Expect(env.Data).To(HaveKey("Response"))
	})
})
