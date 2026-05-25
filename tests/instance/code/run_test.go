package code_test

import (
	"context"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/tests/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInstanceCodeRunCommand(t *testing.T) {
	testutil.RunSpecs(t, "AGR instance code run command live smoke")
}

var _ = BeforeSuite(testutil.SetupSuite)
var _ = AfterSuite(testutil.CleanupSuite)

var _ = Describe("instance code run", func() {
	It("executes agr instance code run", func() {
		cli := testutil.NewCLI()
		DeferCleanup(cli.Cleanup)
		cli.InitConfig()
		tracker := testutil.NewResourceTracker(cli)
		instanceID := testutil.CreateInstance(context.Background(), cli, tracker)

		result := cli.Run(context.Background(), "--output", "json", "instance", "code", "run", instanceID, "--code", "print('agr-live-code-run')")
		result.ExpectSuccess()
		env := result.Envelope()
		Expect(env.Command).To(Equal("instance.code.run"))
		Expect(testutil.StringField(env.Data, "Stdout")).To(ContainSubstring("agr-live-code-run"))
	})
})
