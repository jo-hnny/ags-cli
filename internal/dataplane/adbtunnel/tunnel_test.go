package adbtunnel

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

var _ = Describe("ADB tunnel", func() {
	It("validates tunnel options", func() {
		_, err := New(TunnelOptions{InstanceID: "sandbox-test", Domain: "ap-guangzhou.tencentags.com", TokenProvider: func() (string, error) { return "token", nil }})
		Expect(err).NotTo(HaveOccurred())
		_, err = New(TunnelOptions{Domain: "ap-guangzhou.tencentags.com", TokenProvider: func() (string, error) { return "token", nil }})
		Expect(err).To(MatchError(ContainSubstring("instanceID")))
		_, err = New(TunnelOptions{InstanceID: "sandbox-test", TokenProvider: func() (string, error) { return "token", nil }})
		Expect(err).To(MatchError(ContainSubstring("domain")))
		_, err = New(TunnelOptions{InstanceID: "sandbox-test", Domain: "ap-guangzhou.tencentags.com"})
		Expect(err).To(MatchError(ContainSubstring("tokenProvider")))
	})

	It("constructs websocket URL and host", func() {
		tunnel, err := New(TunnelOptions{InstanceID: "sandbox-aaa", Domain: "ap-guangzhou.tencentags.com", TokenProvider: func() (string, error) { return "token", nil }})
		Expect(err).NotTo(HaveOccurred())
		Expect(tunnel.wsURL).To(Equal("wss://5556-sandbox-aaa.ap-guangzhou.tencentags.com/adb/ws"))
		Expect(tunnel.e2bHost).To(Equal("5556-sandbox-aaa.ap-guangzhou.tencentags.com"))
	})

	It("can reserve a local listener", func() { requireLocalListen() })
})
