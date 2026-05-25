package lifecycle

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/TencentCloudAgentRuntime/ags-cli/tests/testutil"
)

func newCLI() *testutil.CLI {
	cli := testutil.NewCLI()
	DeferCleanup(cli.Cleanup)
	return cli
}

func stringField(data map[string]any, key string) string {
	return testutil.StringField(data, key)
}

func numberField(data map[string]any, key string) int {
	return testutil.NumberField(data, key)
}

func itemsField(data map[string]any) []any {
	return testutil.ItemsField(data)
}

type ResourceTracker struct {
	cli       *testutil.CLI
	tools     []string
	instances []string
}

func NewResourceTracker(cli *testutil.CLI) *ResourceTracker {
	tracker := &ResourceTracker{cli: cli}
	DeferCleanup(tracker.Cleanup)
	return tracker
}

func (r *ResourceTracker) AddTool(id string) {
	if id != "" {
		r.tools = append(r.tools, id)
	}
}

func (r *ResourceTracker) AddInstance(id string) {
	if id != "" {
		r.instances = append(r.instances, id)
	}
}

func (r *ResourceTracker) ForgetTool(id string) {
	r.tools = removeString(r.tools, id)
}

func (r *ResourceTracker) ForgetInstance(id string) {
	r.instances = removeString(r.instances, id)
}

func (r *ResourceTracker) Cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	for i := len(r.instances) - 1; i >= 0; i-- {
		r.cli.Run(ctx, "--output", "json", "instance", "delete", r.instances[i], "--ignore-not-found")
	}
	for i := len(r.tools) - 1; i >= 0; i-- {
		r.cli.Run(ctx, "--output", "json", "tool", "delete", r.tools[i])
	}
}

func removeString(values []string, target string) []string {
	out := values[:0]
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

func waitForToolStatus(cli *testutil.CLI, toolID string, allowed ...string) testutil.Envelope {
	var last testutil.CommandResult
	var env testutil.Envelope
	Eventually(func(g Gomega) string {
		last = cli.Run(context.Background(), "--output", "json", "tool", "get", toolID)
		g.Expect(last.ExitCode).To(Equal(0), last.Diagnostics())
		env = last.Envelope()
		return stringField(env.Data, "Status")
	}, 8*time.Minute, 10*time.Second).Should(BeElementOf(allowed), func() string {
		return fmt.Sprintf("tool %s did not reach %v\n%s", toolID, allowed, last.Diagnostics())
	})
	return env
}

func waitForInstanceStatus(cli *testutil.CLI, instanceID string, allowed ...string) testutil.Envelope {
	var last testutil.CommandResult
	var env testutil.Envelope
	Eventually(func(g Gomega) string {
		last = cli.Run(context.Background(), "--output", "json", "instance", "get", instanceID)
		g.Expect(last.ExitCode).To(Equal(0), last.Diagnostics())
		env = last.Envelope()
		return stringField(env.Data, "Status")
	}, 8*time.Minute, 10*time.Second).Should(BeElementOf(allowed), func() string {
		return fmt.Sprintf("instance %s did not reach %v\n%s", instanceID, allowed, last.Diagnostics())
	})
	return env
}

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
