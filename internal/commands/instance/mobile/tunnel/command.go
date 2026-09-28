package tunnel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/cli"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/client"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/dataplane/adbtunnel"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/dataplane/tunnelstore"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

// Tunnel is the foreground or daemon ADB tunnel process managed by this command.
type Tunnel interface {
	Start() (string, error)
	Probe() error
	Stop()
}

// RuntimeDeps contains token, config, tunnel construction, and signal-wait hooks
// that tests can replace without opening a real tunnel.
type RuntimeDeps struct {
	AcquireToken   func(ctx context.Context, instanceID string) (string, error)
	ValidateConfig func() error
	NewTunnel      func(adbtunnel.TunnelOptions) (Tunnel, error)
	Wait           func(context.Context)
	StopTimeout    time.Duration
}

// Module returns this package's command module.
func Module() command.Module {
	spec := command.Spec{
		ID:     "instance.mobile.tunnel",
		Path:   []string{"instance", "mobile", "tunnel"},
		Use:    "tunnel <instance-id>",
		Short:  "Run ADB tunnel in foreground (used internally by connect)",
		Hidden: true,
		Args:   []command.ArgSpec{{Name: "instance-id", Required: true}},
		Flags:  []command.FlagSpec{{Name: "daemon", Usage: "Run in daemon mode (used by connect)", Type: command.FlagBool}, {Name: "port", Usage: "Local port to listen on (0 = auto-assign)", Type: command.FlagInt, Default: 0}},
		Output: command.OutputSpec{DataType: "MobileTunnel"},
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
			Source: command.SourceWorkflow,
		},
		Build: func(deps command.Deps) (command.Runtime, error) {
			deps = deps.WithDefaults()
			rt := runtimeDeps(deps.DataPlane)
			return command.Runtime{Handler: command.HandlerFunc(func(ctx context.Context, req command.Request) (*command.Result, error) {
				return runTunnel(ctx, req, deps, rt)
			})}, nil
		},
	}
}

func runtimeDeps(injected any) RuntimeDeps {
	rt, _ := injected.(RuntimeDeps)
	if rt.AcquireToken == nil {
		rt.AcquireToken = cli.AcquireInstanceToken
	}
	if rt.ValidateConfig == nil {
		rt.ValidateConfig = config.Validate
	}
	if rt.NewTunnel == nil {
		rt.NewTunnel = func(opts adbtunnel.TunnelOptions) (Tunnel, error) {
			return adbtunnel.New(opts)
		}
	}
	if rt.Wait == nil {
		rt.Wait = waitForSignal
	}
	if rt.StopTimeout == 0 {
		rt.StopTimeout = 5 * time.Second
	}
	return rt
}

func runTunnel(ctx context.Context, req command.Request, deps command.Deps, rt RuntimeDeps) (result *command.Result, retErr error) {
	instanceID := req.ArgValues["instance-id"]
	if instanceID == "" && len(req.Args) > 0 {
		instanceID = req.Args[0]
	}
	daemon := boolFlag(req, "daemon")
	readyWritten := false
	defer func() {
		if daemon && !readyWritten && retErr != nil {
			classified := cli.ClassifyCLIError(retErr)
			_ = json.NewEncoder(deps.IO.Out).Encode(adbtunnel.ReadyMessage{Status: "error", Failure: classified.Failure, ExitCode: classified.ExitCode})
		}
	}()
	logger := log.New(cli.DiagnosticWriter(deps.IO.ErrOut), "", log.LstdFlags)
	port := intFlag(req, "port")
	if port < 0 || port > 65535 {
		return nil, output.NewUsageError("INVALID_PORT", "--port must be between 0 and 65535", "Use 0 for an automatically assigned port, or a port from 1 to 65535.")
	}
	if err := rt.ValidateConfig(); err != nil {
		return nil, err
	}

	cfg := config.Get()
	listenAddr := "127.0.0.1:0"
	if port > 0 {
		listenAddr = fmt.Sprintf("127.0.0.1:%d", port)
	}
	tunnel, err := rt.NewTunnel(adbtunnel.TunnelOptions{
		InstanceID: instanceID,
		Domain:     cfg.DataPlaneRegionDomain(),
		TokenProvider: func() (string, error) {
			return rt.AcquireToken(ctx, instanceID)
		},
		ListenAddress: listenAddr,
		Insecure:      false,
		OnStateChange: makeStateChangeHandler(instanceID, logger),
		Logger:        logger,
	})
	if err != nil {
		return nil, classifyTunnelError(fmt.Errorf("failed to create tunnel: %w", err))
	}

	addr, err := tunnel.Start()
	if err != nil {
		if isAddressInUse(err) {
			return nil, output.NewUsageError("PORT_IN_USE", fmt.Sprintf("local port %d is already in use", port), "Choose another --port or use --port 0.").WithCause(err)
		}
		return nil, classifyTunnelError(fmt.Errorf("failed to start tunnel: %w", err))
	}

	if err := tunnel.Probe(); err != nil {
		tunnel.Stop()
		return nil, classifyProbeError(fmt.Errorf("upstream probe failed: %w", err))
	}

	_, portStr, _ := strings.Cut(addr, ":")
	if daemon {
		msg := adbtunnel.ReadyMessage{Status: "ready", Port: mustAtoi(portStr), PID: os.Getpid()}
		if err := json.NewEncoder(deps.IO.Out).Encode(msg); err != nil {
			tunnel.Stop()
			return nil, classifyTunnelError(fmt.Errorf("failed to write ready message: %w", err))
		}
		readyWritten = true
	} else {
		fmt.Fprintf(deps.IO.Out, "[Ready] ADB Tunnel established at %s\n", addr)
		fmt.Fprintln(deps.IO.Out, "[Ready] Press Ctrl+C to disconnect.")
	}

	rt.Wait(ctx)

	if !daemon {
		fmt.Fprintln(deps.IO.Out, "\n[INFO] Shutting down ADB tunnel...")
	}

	shutdownDone := make(chan struct{})
	go func() {
		tunnel.Stop()
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
	case <-time.After(rt.StopTimeout):
		if !daemon {
			fmt.Fprintln(deps.IO.Out, "[WARN] Graceful shutdown timed out. Forcing exit.")
		}
	}

	return &command.Result{StreamDone: true}, nil
}

func waitForSignal(ctx context.Context) {
	waitCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-waitCtx.Done()
}

func boolFlag(req command.Request, name string) bool {
	flag, ok := req.Flags[name]
	return ok && flag.Bool
}

func intFlag(req command.Request, name string) int {
	flag, ok := req.Flags[name]
	if !ok {
		return 0
	}
	return flag.Int
}

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func classifyTunnelError(err error) error {
	classified := client.ClassifyError(err)
	if classified.Failure.Code == "INTERNAL_ERROR" {
		return output.NewCLIError(&output.Failure{Code: "TUNNEL_ERROR", Kind: output.KindGenericError, Message: err.Error(), Hint: "Inspect the reported tunnel operation; use --debug for the underlying cause."}).WithCause(err)
	}
	return classified
}

func classifyProbeError(err error) error {
	classified := client.ClassifyError(err)
	var handshake *adbtunnel.HandshakeError
	if !errors.As(err, &handshake) {
		return classifyTunnelError(err)
	}
	if classified.Failure.Code == "INTERNAL_ERROR" && (handshake.HTTPStatus == http.StatusUnauthorized || handshake.HTTPStatus == http.StatusForbidden) {
		classified = output.NewCLIError(&output.Failure{Code: "TUNNEL_AUTH_FAILED", Kind: output.KindAuthOrPermission, Message: err.Error(), Hint: "Check the tunnel access token and permissions, then reconnect."}).WithCause(err)
	}
	if classified.Failure.Code == "INTERNAL_ERROR" {
		classified = output.NewCLIError(&output.Failure{
			Code: "NETWORK_ERROR", Kind: output.KindNetwork, Message: err.Error(),
			Hint: "Check the tunnel endpoint, network, and access token. Inspect Failure.Details.HTTPStatus when present.",
		}).WithCause(err)
	}
	failure := *classified.Failure
	failure.Details = maps.Clone(classified.Failure.Details)
	if failure.Details == nil {
		failure.Details = map[string]any{}
	}
	failure.Details["Stage"] = "websocket_handshake"
	if handshake.HTTPStatus != 0 {
		failure.Details["HTTPStatus"] = handshake.HTTPStatus
	}
	copy := *classified
	copy.Failure = &failure
	return &copy
}

// makeStateChangeHandler returns an OnStateChange callback that updates the
// tunnel's status in the persistent tunnel store. This allows `mobile list`
// to display the real health state without probing each tunnel.
func makeStateChangeHandler(instanceID string, logger *log.Logger) func(adbtunnel.TunnelState) {
	return func(state adbtunnel.TunnelState) {
		store, err := tunnelstore.NewStore()
		if err != nil {
			logger.Printf("[WARN] Failed to open tunnel store for status update: %v", err)
			return
		}
		var status string
		var degradedAt *time.Time
		switch state {
		case adbtunnel.StateDegraded:
			status = "unreachable"
			now := time.Now()
			degradedAt = &now
		default:
			status = "connected"
		}
		if err := store.UpdateStatus(instanceID, status, degradedAt); err != nil {
			logger.Printf("[WARN] Failed to update tunnel status in store: %v", err)
		}
	}
}
