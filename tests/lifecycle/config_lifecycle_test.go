package lifecycle

import (
	"context"

	"github.com/TencentCloudAgentRuntime/ags-cli/tests/testutil"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CLI configuration", Ordered, func() {
	var cli *testutil.CLI

	BeforeAll(func() {
		cli = newCLI()
	})

	It("initializes an isolated config and reports status through the CLI", func() {
		env := cli.InitConfig()
		configFile := stringField(env.Data, "ConfigFile")
		Expect(configFile).To(Equal(filepath.Join(cli.Home, ".agr", "config.toml")))

		info, err := os.Stat(configFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))

		status := cli.Run(context.Background(), "--output", "json", "status")
		status.ExpectSuccess()
		Expect(status.Stderr).To(BeEmpty(), status.Diagnostics())
		statusEnv := status.Envelope()
		Expect(statusEnv.Command).To(Equal("status"))
		Expect(statusEnv.Status).To(Equal("succeeded"))
		Expect(statusEnv.Data["ConfigLoaded"]).To(Equal(true))
		Expect(statusEnv.Data["ConfigFound"]).To(Equal(true))
		Expect(stringField(statusEnv.Data, "Region")).To(Equal(testutil.State().Config.Region))

		auth := statusEnv.Data["Auth"].(map[string]any)
		secretID := auth["SecretId"].(map[string]any)
		secretKey := auth["SecretKey"].(map[string]any)
		Expect(secretID["Present"]).To(Equal(true))
		Expect(secretKey["Present"]).To(Equal(true))
	})

	It("supports --jq only with explicit JSON output and keeps failures on stderr", func() {
		good := cli.Run(context.Background(), "--output", "json", "--jq", ".Data.ConfigLoaded", "status")
		good.ExpectSuccess()
		Expect(strings.TrimSpace(good.Stdout)).To(Equal("true"), good.Diagnostics())
		Expect(good.Stderr).To(BeEmpty(), good.Diagnostics())

		bad := cli.Run(context.Background(), "--jq", ".Data.ConfigLoaded", "status")
		bad.ExpectExit(2)
		Expect(bad.Stdout).To(BeEmpty(), bad.Diagnostics())
		Expect(bad.Stderr).To(ContainSubstring("--jq can only be used with explicit -o json"), bad.Diagnostics())
	})

	It("runs doctor against the configured live service", func() {
		result := cli.Run(context.Background(), "--output", "json", "doctor")
		result.ExpectSuccess()
		Expect(result.Stderr).To(BeEmpty(), result.Diagnostics())
		env := result.Envelope()
		Expect(env.Command).To(Equal("doctor"))
		Expect(env.Status).To(Equal("succeeded"))
		Expect(env.Data["Checks"]).To(BeAssignableToTypeOf([]any{}))
	})
})
