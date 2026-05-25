package file_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/tests/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInstanceFileCommands(t *testing.T) {
	testutil.RunSpecs(t, "AGR instance file command live smoke")
}

var _ = BeforeSuite(testutil.SetupSuite)
var _ = AfterSuite(testutil.CleanupSuite)

var _ = Describe("instance file commands", Ordered, func() {
	var cli *testutil.CLI
	var tracker *testutil.ResourceTracker
	var instanceID string
	var remotePath string

	BeforeAll(func() {
		cli = testutil.NewCLI()
		cli.InitConfig()
		tracker = testutil.NewResourceTracker(cli)
		instanceID = testutil.CreateInstance(context.Background(), cli, tracker)
		remotePath = "/tmp/agr-live-file.txt"
	})

	AfterAll(func() {
		tracker.Cleanup()
		cli.Cleanup()
	})

	It("executes agr instance file upload", func() {
		localPath := testutil.TempFile("agr-live-file")
		result := cli.Run(context.Background(), "--output", "json", "instance", "file", "upload", instanceID, localPath, remotePath)
		result.ExpectSuccess()
		Expect(result.Envelope().Command).To(Equal("instance.file.upload"))
	})

	It("executes agr instance file download", func() {
		downloadPath := filepath.Join(GinkgoT().TempDir(), "download.txt")
		result := cli.Run(context.Background(), "--output", "json", "instance", "file", "download", instanceID, remotePath, downloadPath)
		result.ExpectSuccess()
		Expect(result.Envelope().Command).To(Equal("instance.file.download"))
		data, err := os.ReadFile(downloadPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("agr-live-file"))
	})
})
