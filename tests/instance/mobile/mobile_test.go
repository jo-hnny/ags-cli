package mobile_test

import (
	"context"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/tests/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInstanceMobileCommands(t *testing.T) {
	testutil.RunSpecs(t, "AGR instance mobile command live smoke")
}

var _ = BeforeSuite(testutil.SetupSuite)
var _ = AfterSuite(testutil.CleanupSuite)

var _ = Describe("instance mobile commands", func() {
	var cli *testutil.CLI

	BeforeEach(func() {
		cli = testutil.NewCLI()
		cli.InitConfig()
		DeferCleanup(cli.Cleanup)
	})

	It("executes agr instance mobile list", func() {
		result := cli.Run(context.Background(), "--output", "json", "instance", "mobile", "list")
		result.ExpectSuccess()
		Expect(result.Envelope().Command).To(Equal("instance.mobile.list"))
	})

	It("executes agr instance mobile disconnect --all", func() {
		result := cli.Run(context.Background(), "--output", "json", "instance", "mobile", "disconnect", "--all")
		result.ExpectSuccess()
		Expect(result.Envelope().Command).To(Equal("instance.mobile.disconnect"))
	})

	It("executes agr instance mobile connect validation path", func() {
		result := cli.Run(context.Background(), "--output", "json", "instance", "mobile", "connect", "ins-live-placeholder")
		Expect(result.ExitCode).NotTo(Equal(0), result.Diagnostics())
		env := result.Envelope()
		Expect(env.Command).To(Equal("instance.mobile.connect"))
		Expect(env.Status).To(Equal("failed"))
	})

	It("executes agr instance mobile adb validation path", func() {
		result := cli.Run(context.Background(), "--output", "json", "instance", "mobile", "adb", "ins-live-placeholder")
		result.ExpectExit(2)
		env := result.Envelope()
		Expect(env.Command).To(Equal("instance.mobile.adb"))
		Expect(env.Status).To(Equal("failed"))
	})
})
