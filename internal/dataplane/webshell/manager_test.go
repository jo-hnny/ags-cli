package webshell

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("validateTTYDBinary", func() {
	var tmpDir string
	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "ttyd-test")
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() { _ = os.RemoveAll(tmpDir) })

	It("rejects missing, too small, too large and directory paths", func() {
		Expect(validateTTYDBinary(filepath.Join(tmpDir, "missing"))).To(MatchError(ContainSubstring("does not exist")))
		small := filepath.Join(tmpDir, "small")
		Expect(os.WriteFile(small, []byte("small"), 0o644)).To(Succeed())
		Expect(validateTTYDBinary(small)).To(MatchError(ContainSubstring("too small")))
		large := filepath.Join(tmpDir, "large")
		Expect(os.WriteFile(large, make([]byte, 51*1024*1024), 0o644)).To(Succeed())
		Expect(validateTTYDBinary(large)).To(MatchError(ContainSubstring("too large")))
		dir := filepath.Join(tmpDir, "dir")
		Expect(os.Mkdir(dir, 0o755)).To(Succeed())
		Expect(validateTTYDBinary(dir)).To(MatchError(ContainSubstring("not a regular file")))
	})

	It("accepts valid file size", func() {
		valid := filepath.Join(tmpDir, "valid")
		Expect(os.WriteFile(valid, make([]byte, 2*1024*1024), 0o644)).To(Succeed())
		Expect(validateTTYDBinary(valid)).To(Succeed())
	})
})
