package utils

import (
	"runtime"
	"testing"
)

func TestBrowserCommandDiscovery(t *testing.T) {
	available := IsBrowserAvailable()
	cmd, args, err := GetBrowserCommand()

	switch runtime.GOOS {
	case "windows":
		if !available || err != nil || cmd != "rundll32" || len(args) != 1 {
			t.Fatalf("windows browser discovery mismatch: available=%v cmd=%q args=%v err=%v", available, cmd, args, err)
		}
	case "darwin":
		if !available || err != nil || cmd != "open" || len(args) != 0 {
			t.Fatalf("darwin browser discovery mismatch: available=%v cmd=%q args=%v err=%v", available, cmd, args, err)
		}
	case "linux":
		if available {
			if err != nil || cmd == "" {
				t.Fatalf("linux browser should have command when available: cmd=%q args=%v err=%v", cmd, args, err)
			}
		} else if err == nil {
			t.Fatalf("linux browser unavailable should return error")
		}
	default:
		if available || err == nil {
			t.Fatalf("unsupported platform should be unavailable: available=%v cmd=%q args=%v err=%v", available, cmd, args, err)
		}
	}
}
