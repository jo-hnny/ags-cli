package adbtunnel

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAdbtunnel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Adbtunnel Suite")
}
