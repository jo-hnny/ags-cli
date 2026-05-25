package cli

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	// Version is the build version printed by "agr version".
	Version = "dev"
	// Commit identifies the source revision used to build the binary.
	Commit = "unknown"
	// BuildTime records when the binary was built.
	BuildTime = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print the version, commit hash, and build time of AGR CLI.`,
}

func init() {
	versionCmd.RunE = Wrap("version", versionFn)
	rootCmd.AddCommand(versionCmd)
}

func versionFn(cmd *cobra.Command, args []string) (*CmdResult, error) {
	version, commit, buildTime := resolvedVersionInfo()
	data := &output.VersionData{
		Version:   version,
		Commit:    commit,
		BuildTime: buildTime,
	}
	return OK(data, func(w io.Writer) {
		fmt.Fprintf(w, "agr version %s\n", version)
		fmt.Fprintf(w, "  commit: %s\n", commit)
		fmt.Fprintf(w, "  built:  %s\n", buildTime)
	}), nil
}

// SetVersionInfo overrides build metadata, usually from main via ldflags or
// tests that need deterministic version output.
func SetVersionInfo(version, commit, buildTime string) {
	if version != "" {
		Version = version
	}
	if commit != "" {
		Commit = commit
	}
	if buildTime != "" {
		BuildTime = buildTime
	}
}

func resolvedVersionInfo() (string, string, string) {
	version, commit, buildTime := Version, Commit, BuildTime
	if bi, ok := debug.ReadBuildInfo(); ok {
		if (version == "" || version == "dev") && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			version = bi.Main.Version
		}
		if commit == "" || commit == "unknown" {
			for _, setting := range bi.Settings {
				if setting.Key == "vcs.revision" && setting.Value != "" {
					commit = setting.Value
				}
				if setting.Key == "vcs.time" && setting.Value != "" && (buildTime == "" || buildTime == "unknown") {
					buildTime = setting.Value
				}
			}
		}
	}
	return version, commit, buildTime
}
