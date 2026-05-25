package proxy

import (
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func requireLocalListen() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		Skip("local listener unavailable: " + err.Error())
	}
	_ = ln.Close()
}

var _ = Describe("Proxy", func() {
	It("validates constructor options", func() {
		p, err := New(Options{InstanceID: "sandbox-test", Domain: "ap-guangzhou.tencentags.com", RemotePort: 3000, Token: "token"})
		Expect(err).NotTo(HaveOccurred())
		Expect(p).NotTo(BeNil())
		Expect(p.targetHost).To(Equal("3000-sandbox-test.ap-guangzhou.tencentags.com"))

		_, err = New(Options{Domain: "ap-guangzhou.tencentags.com", RemotePort: 3000, Token: "token"})
		Expect(err).To(MatchError(ContainSubstring("instanceID")))
		_, err = New(Options{InstanceID: "sandbox-test", RemotePort: 3000, Token: "token"})
		Expect(err).To(MatchError(ContainSubstring("domain")))
		_, err = New(Options{InstanceID: "sandbox-test", Domain: "ap-guangzhou.tencentags.com", RemotePort: 3000})
		Expect(err).To(MatchError(ContainSubstring("token")))
		_, err = New(Options{InstanceID: "sandbox-test", Domain: "ap-guangzhou.tencentags.com", RemotePort: 0, Token: "token"})
		Expect(err).To(MatchError(ContainSubstring("remotePort")))
	})

	It("constructs target host for internal domains", func() {
		p, err := New(Options{InstanceID: "sandbox-bbb", Domain: "ap-guangzhou.internal.tencentags.com", RemotePort: 5173, Token: "t"})
		Expect(err).NotTo(HaveOccurred())
		Expect(p.targetHost).To(Equal("5173-sandbox-bbb.ap-guangzhou.internal.tencentags.com"))
	})

	It("can reserve a local listener in this environment", func() {
		requireLocalListen()
	})
})
