package output

import (
	"context"
	"fmt"
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("format helpers", func() {
	It("formats byte sizes", func() {
		Expect(FormatSize(0)).To(Equal("0 B"))
		Expect(FormatSize(42)).To(Equal("42 B"))
		Expect(FormatSize(1024)).To(Equal("1.0 KB"))
		Expect(FormatSize(1536)).To(Equal("1.5 KB"))
		Expect(FormatSize(1024 * 1024)).To(Equal("1.0 MB"))
	})

	It("truncates long strings", func() {
		Expect(TruncateString("short", 10)).To(Equal("short"))
		Expect(TruncateString("abcdefghij", 10)).To(Equal("abcdefghij"))
		Expect(TruncateString("abcdefghijk", 10)).To(Equal("abcdefg..."))
	})
})

var _ = Describe("error classification", func() {
	It("classifies timeout and cancellation", func() {
		timeout := ClassifyError(context.DeadlineExceeded)
		Expect(timeout.Failure.Code).To(Equal("TIMEOUT"))
		Expect(timeout.Failure.Kind).To(Equal(KindTimeout))
		Expect(timeout.Failure.Retryable).To(BeTrue())

		canceled := ClassifyError(context.Canceled)
		Expect(canceled.Failure.Code).To(Equal("CANCELED"))
		Expect(canceled.Failure.Kind).To(Equal(KindGenericError))
	})

	It("classifies network errors", func() {
		dns := ClassifyError(&net.DNSError{Err: "no such host", Name: "example.invalid"})
		Expect(dns.Failure.Code).To(Equal("DNS_ERROR"))
		Expect(dns.Failure.Kind).To(Equal(KindNetwork))
		Expect(dns.Failure.Retryable).To(BeTrue())

		netErr := ClassifyError(&net.OpError{Op: "dial", Net: "tcp", Err: fmt.Errorf("connection refused")})
		Expect(netErr.Failure.Code).To(Equal("NETWORK_ERROR"))
		Expect(netErr.Failure.Kind).To(Equal(KindNetwork))
	})

	It("hides unknown internal error details", func() {
		classified := ClassifyError(fmt.Errorf("secret detail"))
		Expect(classified.Failure.Code).To(Equal("INTERNAL_ERROR"))
		Expect(classified.Failure.Message).To(Equal("internal error"))
	})
})
