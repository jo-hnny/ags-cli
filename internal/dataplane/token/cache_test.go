package token

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("token cache", func() {
	It("rejects symlink cache files", func() {
		dir := GinkgoT().TempDir()
		target := filepath.Join(dir, "real.json")
		Expect(os.WriteFile(target, []byte(`{"version":1,"tokens":{}}`), 0o600)).To(Succeed())
		link := filepath.Join(dir, "tokens.json")
		Expect(os.Symlink(target, link)).To(Succeed())
		cache := &Cache{path: link, lockPath: link + ".lock"}
		_, err := cache.loadLocked()
		Expect(err).To(HaveOccurred())
	})

	It("quarantines corrupted cache files", func() {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "tokens.json")
		Expect(os.WriteFile(path, []byte("{bad json"), 0o600)).To(Succeed())
		cache := &Cache{path: path, lockPath: path + ".lock"}
		_, err := cache.loadLocked()
		Expect(err).To(HaveOccurred())
		matches, err := filepath.Glob(path + ".corrupt-*")
		Expect(err).NotTo(HaveOccurred())
		Expect(matches).To(HaveLen(1))
	})
})
