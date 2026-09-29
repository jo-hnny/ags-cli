package main

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/apimeta"
)

// Exercise the real CLI, signed dynamic transport and JSON envelope. The old
// SDK cannot represent the new fields, so they must survive the fallback route.
func TestPrecachePromotedContract(t *testing.T) {
	response := map[string]any{"PreCacheImageId": "cache-test", "SourceType": "EXPLICIT", "CachedImageSizeBytes": json.Number("9007199254740993"), "CreateTime": "created", "LastUsedTime": "used", "RequestId": "request-test"}
	for _, tc := range []struct {
		name   string
		args   []string
		stdin  string
		action string
	}{
		{"ID flag", []string{"get", "--pre-cache-image-id", "cache-test"}, "", "DescribePreCacheImageTask"},
		{"ID JSON", []string{"get", "--request", `{"PreCacheImageId":"cache-test"}`}, "", "DescribePreCacheImageTask"},
		{"ID stdin", []string{"get", "--request", "-"}, `{"PreCacheImageId":"cache-test"}`, "DescribePreCacheImageTask"},
		{"ID file", []string{"get", "--request", "@file"}, "", "DescribePreCacheImageTask"},
		{"legacy triple", []string{"get", "sha256:test", "--image", "nginx:latest", "--image-registry-type", "personal"}, "", "DescribePreCacheImageTask"},
		{"create", []string{"create", "--image", "nginx:latest", "--image-registry-type", "personal"}, "", "CreatePreCacheImageTask"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan map[string]any, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-TC-Action") != tc.action || r.Header.Get("Authorization") == "" {
					t.Error("wrong action or missing signature")
				}
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				requests <- request
				_ = json.NewEncoder(w).Encode(map[string]any{"Response": response})
			}))
			defer server.Close()
			if tc.name == "ID file" {
				path := filepath.Join(t.TempDir(), "request.json")
				if err := os.WriteFile(path, []byte(`{"PreCacheImageId":"cache-test"}`), 0600); err != nil {
					t.Fatal(err)
				}
				tc.args[2] = "@" + path
			}
			args := append([]string{"pre-cache-image-task"}, tc.args...)
			args = append(args, "-o", "json")
			cmd := exec.CommandContext(t.Context(), os.Args[0], append([]string{"-test.run=^TestTransportLifecycleHelper$", "--"}, args...)...)
			home := t.TempDir()
			cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})

			cmd.Env = []string{"HOME=" + home, "USERPROFILE=" + home, "PATH=" + os.Getenv("PATH"), "AGR_TRANSPORT_LIFECYCLE_HELPER=success", "GORACE=atexit_sleep_ms=0", "TENCENTCLOUD_SECRET_ID=fake", "TENCENTCLOUD_SECRET_KEY=fake", "AGR_REGION=ap-guangzhou", "AGR_CLOUD_ENDPOINT=" + strings.TrimPrefix(server.URL, "https://"), "AGR_TRANSPORT_TEST_CERT=" + string(cert)}
			cmd.Stdin = strings.NewReader(tc.stdin)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("CLI: %v %s", err, stderr.String())
			}
			var envelope struct {
				Status string
				Data   map[string]any
			}
			decoder := json.NewDecoder(bytes.NewReader(out))
			decoder.UseNumber()
			if err := decoder.Decode(&envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Status != "succeeded" {
				t.Fatalf("status=%s", envelope.Status)
			}
			for key, want := range response {
				if envelope.Data[key] != want {
					t.Errorf("%s=%v want %v", key, envelope.Data[key], want)
				}
			}
			select {
			case req := <-requests:
				if strings.HasPrefix(tc.name, "ID") && (len(req) != 1 || req["PreCacheImageId"] != "cache-test") {
					t.Fatalf("ID request: %v", req)
				}
				if tc.name == "legacy triple" && (len(req) != 3 || req["ImageDigest"] != "sha256:test") {
					t.Fatalf("triple: %v", req)
				}
			default:
				t.Fatal("no API request")
			}
		})
	}
}

// Derive required coverage from the canonical request, not a second field list.
// Deleting a generated field/schema property or adding a canonical member must
// fail until the command, help and schema expose it consistently.
func TestPromotedRequestCoverage(t *testing.T) {
	contract, err := apimeta.LoadContract("../../api/ags/v20250920", apimeta.BuildChannel)
	if err != nil {
		t.Fatal(err)
	}
	for _, mapping := range contract.Mapping.Actions {
		if !strings.HasPrefix(mapping.Command, "registry.") && !strings.HasPrefix(mapping.Command, "session.") && !strings.HasPrefix(mapping.Command, "session-space.") && !strings.HasPrefix(mapping.Command, "pre-cache-image-task.") {
			continue
		}
		t.Run(mapping.Command, func(t *testing.T) {
			schema := schemaForCommand(t, mapping.Command)
			cmd, ok := findCobraCommand(contractRoot(), mapping.Command)
			if !ok {
				t.Fatal("command absent")
			}
			members := map[string]bool{}
			for _, member := range contract.Spec.Object(mapping.Request).Members {
				if member.Disabled {
					continue
				}
				members[member.Name] = true
				property, ok := schema.RequestSchema[member.Name]
				if !ok {
					t.Errorf("canonical field %s absent from schema", member.Name)
					continue
				}
				field := mapping.Fields[member.Name]
				if field != nil && field.Excluded {
					t.Errorf("promoted field %s unexpectedly excluded", member.Name)
					continue
				}
				if field != nil && field.Positional {
					continue
				}
				flag := apimeta.KebabCase(member.Name)
				if field != nil {
					if field.Flag != "" {
						flag = field.Flag
					}
					if len(field.Inputs) > 0 {
						flag = field.Inputs[0].Flag
					}
				}
				if property.CliFlag == nil || *property.CliFlag != flag {
					t.Errorf("%s CliFlag=%v want %s", member.Name, property.CliFlag, flag)
				}
				if cmd.Flags().Lookup(flag) == nil || !strings.Contains(commandHelp(t, cmd), "--"+flag) {
					t.Errorf("%s missing runtime/help flag --%s", member.Name, flag)
				}
			}
			for name := range schema.RequestSchema {
				if !members[name] {
					t.Errorf("schema contains noncanonical field %s", name)
				}
			}
		})
	}
}
