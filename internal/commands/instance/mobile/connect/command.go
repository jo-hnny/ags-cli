package connect

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/cli"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/dataplane/mobileadb"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/dataplane/tunnelstore"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

// Store is the tunnel registry dependency used to replace stale connections and
// persist the new tunnel.
type Store interface {
	Get(string) (tunnelstore.TunnelEntry, bool, error)
	Cleanup(string) error
	Save(string, tunnelstore.TunnelEntry) error
}

// TunnelReady is the daemon readiness payload emitted by the hidden tunnel
// command and consumed by connect.
type TunnelReady struct {
	Port    int
	PID     int
	ExePath string
	LogPath string
}

// RuntimeDeps contains adb, config, tunnel, and store hooks that tests can
// replace without spawning a daemon process.
type RuntimeDeps struct {
	RequireADB     func() (string, error)
	ValidateConfig func() error
	NewStore       func() (Store, error)
	DisconnectADB  func(adbPath, addr string) error
	StartTunnel    func(ctx context.Context, instanceID string, port int) (TunnelReady, error)
	ConnectADB     func(adbPath, addr string, maxRetries int, out io.Writer) error
}

// Module returns this package's command module.
func Module() command.Module {
	return mobileModule(command.Spec{
		ID:           "instance.mobile.connect",
		Path:         []string{"instance", "mobile", "connect"},
		Use:          "connect <instance-id>",
		Short:        "Connect to mobile instance (background tunnel + adb connect)",
		Args:         []command.ArgSpec{{Name: "instance-id", Required: true}},
		Flags:        []command.FlagSpec{{Name: "port", Shorthand: "p", Usage: "Local port to listen on (0 = auto-assign)", Type: command.FlagInt, Default: 0}},
		SupportsJSON: true,
		Output:       command.OutputSpec{DataType: "MobileConnection"},
	})
}

func mobileModule(spec command.Spec) command.Module {
	return command.Module{
		Descriptor: command.Descriptor{
			Spec: spec,
			Groups: []command.GroupSpec{
				{
					Path:    []string{"instance"},
					Use:     "instance",
					Short:   "Manage sandbox instances",
					Long:    "Manage sandbox instances and related data-plane workflows.",
					Aliases: []string{"i"},
				},
				{
					Path:  []string{"instance", "mobile"},
					Use:   "mobile",
					Short: "Mobile sandbox ADB commands",
					Long: `Manage ADB connections to mobile sandbox instances.

Examples:
  agr instance mobile connect <instance-id>
  agr instance mobile list
  agr instance mobile adb <instance-id> -- shell ls /sdcard
  agr instance mobile disconnect <instance-id>`,
				},
			},
			Source: command.SourceWorkflow,
		},
		Build: func(deps command.Deps) (command.Runtime, error) {
			deps = deps.WithDefaults()
			rt := runtimeDeps(deps.DataPlane)
			return command.Runtime{
				Handler: command.HandlerFunc(func(ctx context.Context, req command.Request) (*command.Result, error) {
					return runConnect(ctx, req, deps, rt)
				}),
			}, nil
		},
	}
}

func runtimeDeps(injected any) RuntimeDeps {
	rt, _ := injected.(RuntimeDeps)
	if rt.RequireADB == nil {
		rt.RequireADB = mobileadb.Require
	}
	if rt.ValidateConfig == nil {
		rt.ValidateConfig = config.Validate
	}
	if rt.NewStore == nil {
		rt.NewStore = func() (Store, error) { return tunnelstore.NewStore() }
	}
	if rt.DisconnectADB == nil {
		rt.DisconnectADB = func(adbPath, addr string) error {
			return mobileadb.Run(adbPath, "disconnect", addr)
		}
	}
	if rt.StartTunnel == nil {
		rt.StartTunnel = startTunnelDaemon
	}
	if rt.ConnectADB == nil {
		rt.ConnectADB = mobileadb.ConnectWithRetry
	}
	return rt
}

func runConnect(ctx context.Context, req command.Request, deps command.Deps, rt RuntimeDeps) (*command.Result, error) {
	instanceID := req.ArgValues["instance-id"]
	if instanceID == "" && len(req.Args) > 0 {
		instanceID = req.Args[0]
	}

	port := intFlag(req, "port")
	if port < 0 || port > 65535 {
		return nil, output.NewUsageError("INVALID_PORT", "--port must be between 0 and 65535", "Use a valid port number (1-65535) or 0 for auto-assign.")
	}

	adbPath, err := rt.RequireADB()
	if err != nil {
		return nil, output.NewUsageError("ADB_NOT_FOUND", err.Error(), "Install Android SDK Platform-Tools or set ADB_PATH to a valid adb binary.")
	}
	if err := rt.ValidateConfig(); err != nil {
		return nil, err
	}

	store, err := rt.NewStore()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tunnel store: %w", err)
	}

	if oldEntry, ok, _ := store.Get(instanceID); ok {
		oldAddr := fmt.Sprintf("127.0.0.1:%d", oldEntry.Port)
		_ = rt.DisconnectADB(adbPath, oldAddr)
	}
	if err := store.Cleanup(instanceID); err != nil {
		fmt.Fprintf(deps.IO.ErrOut, "Warning: failed to cleanup existing tunnel: %v\n", err)
	}

	ready, err := rt.StartTunnel(ctx, instanceID, port)
	if err != nil {
		return nil, err
	}

	if err := store.Save(instanceID, tunnelstore.TunnelEntry{
		PID:       ready.PID,
		Port:      ready.Port,
		CreatedAt: deps.Now(),
		ExePath:   ready.ExePath,
	}); err != nil {
		fmt.Fprintf(deps.IO.ErrOut, "Warning: failed to save tunnel mapping: %v\n", err)
	}

	adbAddr := fmt.Sprintf("127.0.0.1:%d", ready.Port)
	adbConnectErr := rt.ConnectADB(adbPath, adbAddr, 3, deps.IO.ErrOut)

	data := map[string]any{
		"InstanceId": instanceID,
		"AdbAddress": adbAddr,
		"Port":       ready.Port,
		"Pid":        ready.PID,
	}
	if ready.LogPath != "" {
		data["LogPath"] = ready.LogPath
	}

	return &command.Result{Data: data, Text: func(w io.Writer) {
		if adbConnectErr != nil {
			fmt.Fprintf(deps.IO.ErrOut, "tunnel ready for %s at %s (adb connect failed: %v; use 'adb connect %s' manually)\n",
				instanceID, adbAddr, adbConnectErr, adbAddr)
		} else {
			fmt.Fprintf(deps.IO.ErrOut, "connected to %s (%s)\n", instanceID, adbAddr)
		}
		if ready.LogPath != "" {
			fmt.Fprintf(deps.IO.ErrOut, "tunnel log: %s\n", ready.LogPath)
		}
	}}, nil
}

func openTunnelLog(instanceID string) (string, *os.File) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", nil
	}
	logDir := filepath.Join(homeDir, ".agr")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return "", nil
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("tunnel-%s.log", instanceID))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", nil
	}
	return logPath, logFile
}

func tunnelEnv() []string {
	env := os.Environ()
	cfg := config.Get()
	if cli.SecretIDFlag() != "" {
		env = append(env, "TENCENTCLOUD_SECRET_ID="+cli.SecretIDFlag())
	} else if cfg.Auth.SecretID != "" {
		env = append(env, "TENCENTCLOUD_SECRET_ID="+cfg.Auth.SecretID)
	}
	if cli.SecretKeyFlag() != "" {
		env = append(env, "TENCENTCLOUD_SECRET_KEY="+cli.SecretKeyFlag())
	} else if cfg.Auth.SecretKey != "" {
		env = append(env, "TENCENTCLOUD_SECRET_KEY="+cfg.Auth.SecretKey)
	}
	if token := config.GetToken(); token != "" {
		env = append(env, "TENCENTCLOUD_TOKEN="+token)
	}
	return env
}

func intFlag(req command.Request, name string) int {
	flag, ok := req.Flags[name]
	if !ok {
		return 0
	}
	return flag.Int
}

func closeIfOpen(file *os.File) error {
	if file == nil {
		return nil
	}
	return file.Close()
}
