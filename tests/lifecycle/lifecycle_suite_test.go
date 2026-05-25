package lifecycle

import (
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/tests/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLifecycle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AGR CLI lifecycle tests")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	testutil.SetupSuite()
	return nil
}, func(_ []byte) {})

var _ = SynchronizedAfterSuite(func() {}, func() {
	testutil.CleanupSuite()
})
