package browser_test

import (
	"context"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/tests/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInstanceBrowserCommands(t *testing.T) {
	testutil.RunSpecs(t, "AGR instance browser command live smoke")
}

var _ = BeforeSuite(testutil.SetupSuite)
var _ = AfterSuite(testutil.CleanupSuite)

var _ = Describe("instance browser vnc", func() {
	It("executes agr instance browser vnc", func() {
		cli := testutil.NewCLI()
		DeferCleanup(cli.Cleanup)
		cli.InitConfig()
		tracker := testutil.NewResourceTracker(cli)
		instanceID := testutil.CreateInstance(context.Background(), cli, tracker)

		result := cli.Run(context.Background(), "--output", "json", "instance", "browser", "vnc", instanceID)
		result.ExpectSuccess()
		env := result.Envelope()
		Expect(env.Command).To(Equal("instance.browser.vnc"))
		Expect(testutil.StringField(env.Data, "VncUrl")).To(ContainSubstring(instanceID))
	})
})
