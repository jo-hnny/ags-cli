package webshell

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWebshell(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Webshell Suite")
}
