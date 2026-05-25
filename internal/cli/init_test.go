package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("init", func() {
	It("does not write the removed internal config key", func() {
		tmpDir := GinkgoT().TempDir()
		cfgPath := filepath.Join(tmpDir, "config.toml")

		prevSecretID := secretID
		prevSecretKey := secretKey
		prevOverwrite := initOverwrite
		defer func() {
			secretID = prevSecretID
			secretKey = prevSecretKey
			initOverwrite = prevOverwrite
			config.SetConfigFile("")
		}()

		secretID = ""
		secretKey = ""
		initOverwrite = false
		config.SetConfigFile(cfgPath)

		Expect(os.Setenv("TENCENTCLOUD_SECRET_ID", "sid")).To(Succeed())
		Expect(os.Setenv("TENCENTCLOUD_SECRET_KEY", "skey")).To(Succeed())
		defer func() {
			Expect(os.Unsetenv("TENCENTCLOUD_SECRET_ID")).To(Succeed())
			Expect(os.Unsetenv("TENCENTCLOUD_SECRET_KEY")).To(Succeed())
		}()

		_, err := initFn(nil, nil)
		Expect(err).NotTo(HaveOccurred())

		content, readErr := os.ReadFile(cfgPath)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(content)).NotTo(ContainSubstring("internal = false"))
		Expect(strings.TrimSpace(string(content))).To(ContainSubstring(`cloud_endpoint = "ags.tencentcloudapi.com"`))
	})
})
