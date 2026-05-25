package cli

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("diagnostic helpers", func() {
	It("collects config warnings", func() {
		Expect(collectConfigWarnings("")).To(BeNil())
		Expect(collectConfigWarnings("warn")).To(Equal([]string{"warn"}))
	})

	It("requires non-empty values", func() {
		Expect(requireNonEmptyValue("x", "field", "CODE", "hint")).To(Succeed())
		err := requireNonEmptyValue("  ", "field", "CODE", "hint")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("field is required"))
	})

	It("describes config issues", func() {
		Expect(describeConfigIssue(fmt.Errorf("invalid output format: yaml")).Name).To(Equal("output"))
		Expect(describeConfigIssue(fmt.Errorf("ndjson is only supported for streaming")).Name).To(Equal("output"))
		Expect(describeConfigIssue(fmt.Errorf("invalid region: !!")).Name).To(Equal("region"))
		Expect(describeConfigIssue(fmt.Errorf("invalid domain: https://example.com")).Name).To(Equal("domain"))
		Expect(describeConfigIssue(fmt.Errorf("other")).Name).To(Equal("ConfigFile"))

		cliErr := newConfigUsageError(fmt.Errorf("invalid region: !!"))
		Expect(cliErr.Failure.Code).To(Equal("INVALID_CONFIG"))
		Expect(cliErr.Failure.Hint).To(ContainSubstring("valid value"))
	})
})
