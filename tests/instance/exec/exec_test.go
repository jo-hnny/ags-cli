package exec_test

import (
	"context"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/tests/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInstanceExecCommand(t *testing.T) {
	testutil.RunSpecs(t, "AGR instance exec command live smoke")
}

var _ = BeforeSuite(testutil.SetupSuite)
var _ = AfterSuite(testutil.CleanupSuite)

var _ = Describe("instance exec", func() {
	It("executes agr instance exec", func() {
		cli := testutil.NewCLI()
		DeferCleanup(cli.Cleanup)
		cli.InitConfig()
		tracker := testutil.NewResourceTracker(cli)
		instanceID := testutil.CreateInstance(context.Background(), cli, tracker)

		result := cli.Run(context.Background(), "--output", "json", "instance", "exec", instanceID, "--", "sh", "-lc", "printf agr-live-exec")
		result.ExpectSuccess()
		env := result.Envelope()
		Expect(env.Command).To(Equal("instance.exec"))
		Expect(testutil.StringField(env.Data, "Stdout")).To(ContainSubstring("agr-live-exec"))
	})
})
