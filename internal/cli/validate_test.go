package cli

import (
	"fmt"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("validation helpers", func() {
	errorCode := func(err error) string {
		cliErr := output.ClassifyError(err)
		if cliErr == nil || cliErr.Failure == nil {
			return ""
		}
		return cliErr.Failure.Code
	}

	It("validates listen address values", func() {
		Expect(validateListenAddress("localhost")).To(Succeed())
		Expect(validateListenAddress("127.0.0.1")).To(Succeed())
		Expect(validateListenAddress("::1")).To(Succeed())
		Expect(errorCode(validateListenAddress("not a host"))).To(Equal("INVALID_ADDRESS"))
	})

	It("sorts map keys and detects not found errors", func() {
		Expect(sortedKeys(map[string]bool{"b": true, "a": true})).To(Equal([]string{"a", "b"}))
		Expect(isNotFoundCLIError(output.NewNotFoundError("NOPE", "missing", "hint"))).To(BeTrue())
		Expect(isNotFoundCLIError(fmt.Errorf("other"))).To(BeFalse())
	})
})
