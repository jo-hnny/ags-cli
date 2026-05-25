package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

// ResourceTracker records live instances created by a spec so they can be
// cleaned up automatically.
type ResourceTracker struct {
	cli       *CLI
	instances []string
}

// NewResourceTracker creates a tracker and registers its cleanup with Ginkgo.
func NewResourceTracker(cli *CLI) *ResourceTracker {
	tracker := &ResourceTracker{cli: cli}
	ginkgo.DeferCleanup(tracker.Cleanup)
	return tracker
}

// AddInstance records an instance id for cleanup.
func (r *ResourceTracker) AddInstance(id string) {
	if id != "" {
		r.instances = append(r.instances, id)
	}
}

// ForgetInstance removes an instance id after the test deletes it explicitly.
func (r *ResourceTracker) ForgetInstance(id string) {
	out := r.instances[:0]
	for _, value := range r.instances {
		if value != id {
			out = append(out, value)
		}
	}
	r.instances = out
}

// Cleanup deletes tracked instances unless the live-test config keeps resources.
func (r *ResourceTracker) Cleanup() {
	if r.cli == nil || r.cli.Config.KeepResources {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for i := len(r.instances) - 1; i >= 0; i-- {
		id := r.instances[i]
		result := r.cli.Run(ctx, "--output", "json", "instance", "delete", id, "--ignore-not-found")
		if result.ExitCode != 0 {
			ginkgo.GinkgoWriter.Printf("Warning: cleanup instance %s failed:\n%s\n", id, result.Diagnostics())
		}
	}
}

// CreateInstance creates a sandbox instance through the CLI and waits until it
// is running.
func CreateInstance(ctx context.Context, cli *CLI, tracker *ResourceTracker) string {
	toolID := State().EnsureToolID(ctx)
	result := cli.Run(ctx, "--output", "json", "instance", "create", "--tool-id", toolID, "--timeout", "300s")
	result.ExpectSuccess()
	env := result.Envelope()
	gomega.ExpectWithOffset(1, env.Command).To(gomega.Equal("instance.create"))
	id := StringField(env.Data, "InstanceId")
	tracker.AddInstance(id)
	WaitForInstanceStatus(ctx, id, "RUNNING")
	return id
}

// DeleteInstance deletes a sandbox instance through the CLI and removes it from
// the tracker.
func DeleteInstance(ctx context.Context, cli *CLI, tracker *ResourceTracker, instanceID string) {
	result := cli.Run(ctx, "--output", "json", "instance", "delete", instanceID, "--ignore-not-found")
	result.ExpectSuccess()
	tracker.ForgetInstance(instanceID)
}

// WaitForInstanceStatus polls the Cloud API until the instance reaches one of
// the expected statuses.
func WaitForInstanceStatus(ctx context.Context, instanceID string, statuses ...string) {
	allowed := map[string]bool{}
	for _, status := range statuses {
		allowed[status] = true
	}
	gomega.EventuallyWithOffset(1, func(g gomega.Gomega) string {
		req := ags.NewDescribeSandboxInstanceListRequest()
		req.InstanceIds = []*string{&instanceID}
		resp, err := State().sdk.DescribeSandboxInstanceListWithContext(ctx, req)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(resp.Response).NotTo(gomega.BeNil())
		g.Expect(resp.Response.InstanceSet).NotTo(gomega.BeEmpty())
		status := ""
		if resp.Response.InstanceSet[0].Status != nil {
			status = *resp.Response.InstanceSet[0].Status
		}
		return status
	}, State().Config.EventuallyTimeout, 10*time.Second).Should(gomega.Satisfy(func(status string) bool {
		return allowed[status]
	}))
}

// TempFile writes contents to a spec-scoped temporary file and returns its path.
func TempFile(contents string) string {
	path := filepath.Join(ginkgo.GinkgoT().TempDir(), fmt.Sprintf("agr-live-%d.txt", time.Now().UnixNano()))
	gomega.ExpectWithOffset(1, os.WriteFile(path, []byte(contents), 0o600)).To(gomega.Succeed())
	return path
}
