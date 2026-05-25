package tunnelstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTunnelStore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TunnelStore Suite")
}

func newBDDStore() *Store {
	dir := GinkgoT().TempDir()
	storePath := filepath.Join(dir, "tunnels.json")
	return &Store{path: storePath, lockPath: storePath + ".lock"}
}

var _ = Describe("Tunnel store", func() {
	It("creates default store metadata", func() {
		store, err := NewStore()
		Expect(err).NotTo(HaveOccurred())
		Expect(store).NotTo(BeNil())
		Expect(store.path).NotTo(BeEmpty())
	})

	It("saves, gets, overwrites and removes entries", func() {
		store := newBDDStore()
		entry := TunnelEntry{PID: os.Getpid(), Port: 15555, CreatedAt: time.Now().Truncate(time.Second)}
		Expect(store.Save("sandbox-aaa", entry)).To(Succeed())
		got, ok, err := store.Get("sandbox-aaa")
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		Expect(got.Port).To(Equal(15555))

		Expect(store.Save("sandbox-aaa", TunnelEntry{PID: os.Getpid(), Port: 15556, CreatedAt: time.Now()})).To(Succeed())
		got, ok, err = store.Get("sandbox-aaa")
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())
		Expect(got.Port).To(Equal(15556))

		Expect(store.Remove("sandbox-aaa")).To(Succeed())
		_, ok, err = store.Get("sandbox-aaa")
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
		Expect(store.Remove("missing")).To(Succeed())
	})

	It("lists live entries and cleans zombies", func() {
		store := newBDDStore()
		Expect(store.Save("zombie", TunnelEntry{PID: 99999999, Port: 1, CreatedAt: time.Now()})).To(Succeed())
		Expect(store.Save("live", TunnelEntry{PID: os.Getpid(), Port: 2, CreatedAt: time.Now()})).To(Succeed())
		entries, err := store.List()
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveKey("live"))
		Expect(entries).NotTo(HaveKey("zombie"))
	})

	It("cleans entries and writes valid JSON", func() {
		store := newBDDStore()
		Expect(store.Save("sandbox", TunnelEntry{PID: 99999998, Port: 15555, CreatedAt: time.Now()})).To(Succeed())
		Expect(store.Cleanup("sandbox")).To(Succeed())
		Expect(store.Cleanup("missing")).To(Succeed())
		Expect(store.Save("sandbox", TunnelEntry{PID: os.Getpid(), Port: 15555, CreatedAt: time.Now()})).To(Succeed())
		data, err := os.ReadFile(store.path)
		Expect(err).NotTo(HaveOccurred())
		var entries map[string]TunnelEntry
		Expect(json.Unmarshal(data, &entries)).To(Succeed())
		Expect(entries).To(HaveKey("sandbox"))
		Expect(store.Remove("sandbox")).To(Succeed())
		Expect(store.Save("dead-a", TunnelEntry{PID: 99999997, Port: 1, CreatedAt: time.Now()})).To(Succeed())
		Expect(store.CleanupAll()).To(Succeed())
	})
})
