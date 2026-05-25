package root_test

import (
	"context"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/tests/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRootCommands(t *testing.T) { testutil.RunSpecs(t, "AGR root command live smoke") }

var _ = BeforeSuite(testutil.SetupSuite)
var _ = AfterSuite(testutil.CleanupSuite)

var _ = Describe("root and global commands", func() {
	var cli *testutil.CLI

	BeforeEach(func() {
		cli = testutil.NewCLI()
		DeferCleanup(cli.Cleanup)
	})

	It("executes root help and parent command help", func() {
		for _, args := range [][]string{
			{"--help"},
			{"instance", "--help"},
			{"instance", "code", "--help"},
			{"instance", "file", "--help"},
			{"instance", "browser", "--help"},
			{"instance", "mobile", "--help"},
			{"tool", "--help"},
			{"apikey", "--help"},
			{"api", "--help"},
			{"pre_cache_image_task", "--help"},
		} {
			result := cli.Run(context.Background(), args...)
			result.ExpectSuccess()
			Expect(result.Stdout).NotTo(BeEmpty(), result.Diagnostics())
		}
	})

	It("executes version, schema, completion, explain, init, status, and doctor", func() {
		version := cli.Run(context.Background(), "--output", "json", "version")
		version.ExpectSuccess()
		Expect(version.Envelope().Command).To(Equal("version"))

		schema := cli.Run(context.Background(), "--output", "json", "schema")
		schema.ExpectSuccess()
		Expect(schema.Envelope().Command).To(Equal("schema"))

		completion := cli.Run(context.Background(), "completion", "bash")
		completion.ExpectSuccess()
		Expect(completion.Stdout).To(ContainSubstring("complete"), completion.Diagnostics())

		explain := cli.Run(context.Background(), "--output", "json", "explain", "INVALID_REQUEST_JSON")
		explain.ExpectSuccess()
		Expect(explain.Envelope().Command).To(Equal("explain"))

		initEnv := cli.InitConfig()
		Expect(initEnv.Data).To(HaveKey("ConfigFile"))

		status := cli.Run(context.Background(), "--output", "json", "status")
		status.ExpectSuccess()
		Expect(status.Envelope().Command).To(Equal("status"))

		doctor := cli.Run(context.Background(), "--output", "json", "doctor")
		doctor.ExpectSuccess()
		Expect(doctor.Envelope().Command).To(Equal("doctor"))
	})
})
