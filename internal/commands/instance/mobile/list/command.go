package list

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/dataplane/tunnelstore"
)

// Store is the tunnel registry reader used to list active mobile connections.
type Store interface {
	List() (map[string]tunnelstore.TunnelEntry, error)
}

// RuntimeDeps contains the tunnel store factory so tests can provide in-memory
// tunnel state.
type RuntimeDeps struct {
	NewStore func() (Store, error)
}

// Module returns this package's command module.
func Module() command.Module {
	spec := command.Spec{
		ID:           "instance.mobile.list",
		Path:         []string{"instance", "mobile", "list"},
		Use:          "list",
		Short:        "List active mobile instance connections",
		SupportsJSON: true,
		Output:       command.OutputSpec{DataType: "MobileConnectionList"},
	}
	return command.Module{
		Descriptor: command.Descriptor{
			Spec: spec,
			Groups: []command.GroupSpec{
				{Path: []string{"instance"}, Use: "instance", Short: "Manage sandbox instances", Long: "Manage sandbox instances and related data-plane workflows.", Aliases: []string{"i"}},
				{Path: []string{"instance", "mobile"}, Use: "mobile", Short: "Mobile sandbox ADB commands", Long: `Manage ADB connections to mobile sandbox instances.

Examples:
  agr instance mobile connect <instance-id>
  agr instance mobile list
  agr instance mobile adb <instance-id> -- shell ls /sdcard
  agr instance mobile disconnect <instance-id>`},
			},
			Source: "workflow",
		},
		Build: func(deps command.Deps) (command.Runtime, error) {
			deps = deps.WithDefaults()
			rt := runtimeDeps(deps.DataPlane)
			return command.Runtime{Handler: command.HandlerFunc(func(ctx context.Context, req command.Request) (*command.Result, error) {
				return runList(ctx, deps, rt)
			})}, nil
		},
	}
}

func runtimeDeps(injected any) RuntimeDeps {
	rt, _ := injected.(RuntimeDeps)
	if rt.NewStore == nil {
		rt.NewStore = func() (Store, error) { return tunnelstore.NewStore() }
	}
	return rt
}

func runList(_ context.Context, deps command.Deps, rt RuntimeDeps) (*command.Result, error) {
	store, err := rt.NewStore()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tunnel store: %w", err)
	}
	entries, err := store.List()
	if err != nil {
		var recovered *tunnelstore.CorruptStoreRecoveredError
		if errors.As(err, &recovered) {
			data := map[string]any{"Items": []map[string]any{}, "Total": 0}
			return &command.Result{
				Data:     data,
				Warnings: []string{recovered.Error()},
				Text: func(w io.Writer) {
					fmt.Fprintln(deps.IO.ErrOut, "No active connections")
					fmt.Fprintf(deps.IO.ErrOut, "Warning: %s\n", recovered.Error())
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to list tunnels: %w", err)
	}
	items := make([]map[string]any, 0, len(entries))
	for id, entry := range entries {
		addr := fmt.Sprintf("127.0.0.1:%d", entry.Port)
		items = append(items, map[string]any{
			"InstanceId": id,
			"AdbAddress": addr,
			"Port":       entry.Port,
			"Pid":        entry.PID,
			"CreatedAt":  entry.CreatedAt.Format(time.RFC3339),
			"Status":     "connected",
		})
	}
	data := map[string]any{"Items": items, "Total": len(items)}
	return &command.Result{Data: data, Text: func(w io.Writer) {
		if len(entries) == 0 {
			fmt.Fprintln(deps.IO.ErrOut, "No active connections.")
			fmt.Fprintln(deps.IO.ErrOut, "Use 'agr instance mobile connect <instance-id>' to connect.")
			return
		}
		headers := []string{"INSTANCE", "ADB ADDRESS", "STATUS"}
		rows := make([][]string, 0, len(entries))
		for id, entry := range entries {
			addr := fmt.Sprintf("127.0.0.1:%d", entry.Port)
			rows = append(rows, []string{id, addr, "connected"})
		}
		printTable(w, headers, rows)
	}}, nil
}

func printTable(w io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	_ = tw.Flush()
}
