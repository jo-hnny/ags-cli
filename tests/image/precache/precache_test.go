package precache_test

import (
	"context"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/tests/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestImagePrecacheCommands(t *testing.T) {
	testutil.RunSpecs(t, "AGR pre-cache-image-task command live smoke")
}

var _ = BeforeSuite(testutil.SetupSuite)
var _ = AfterSuite(testutil.CleanupSuite)

var _ = Describe("pre-cache-image-task commands", func() {
	var cli *testutil.CLI

	BeforeEach(func() {
		cli = testutil.NewCLI()
		cli.InitConfig()
		DeferCleanup(cli.Cleanup)
	})

	It("executes agr pre-cache-image-task create through skeleton mode", func() {
		result := cli.Run(context.Background(), "--output", "json", "pre-cache-image-task", "create", "--generate-skeleton")
		result.ExpectSuccess()
		Expect(result.Envelope().Command).To(Equal("pre-cache-image-task.create"))
	})

	It("executes agr pre-cache-image-task get through skeleton mode", func() {
		result := cli.Run(context.Background(), "--output", "json", "pre-cache-image-task", "get", "--generate-skeleton")
		result.ExpectSuccess()
		Expect(result.Envelope().Command).To(Equal("pre-cache-image-task.get"))
	})
})
