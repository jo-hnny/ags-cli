package instanceview

import (
	"strings"
	"testing"

	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

func TestTimeout(t *testing.T) {
	for _, tc := range []struct {
		seconds uint64
		want    string
	}{
		{seconds: 7200, want: "2h"},
		{seconds: 300, want: "5m"},
		{seconds: 45, want: "45s"},
	} {
		if got := Timeout(tc.seconds); got != tc.want {
			t.Fatalf("Timeout(%d) = %q, want %q", tc.seconds, got, tc.want)
		}
	}
}

func TestMountOptionsDetailDefaults(t *testing.T) {
	name := "workspace"
	detail := MountOptionsDetail([]*ags.MountOption{{Name: &name}})
	if !strings.Contains(detail, "MountPath: (default)") || !strings.Contains(detail, "ReadOnly:  (default)") {
		t.Fatalf("detail = %q", detail)
	}
	if got := MountOptionsDetail(nil); got != "" {
		t.Fatalf("empty detail = %q", got)
	}
}

func TestCanonicalDataDereferencesScalars(t *testing.T) {
	id := "ins-unit"
	timeout := uint64(300)
	data := CanonicalData(&ags.SandboxInstance{InstanceId: &id, TimeoutSeconds: &timeout})
	if data["InstanceId"] != id || data["TimeoutSeconds"] != timeout {
		t.Fatalf("data = %#v", data)
	}
}
