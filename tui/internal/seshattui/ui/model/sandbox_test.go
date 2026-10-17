package model

import (
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/tools/bash"
)

func TestSandboxLabelNeverShowsAnUnconfinedHostAsConfined(t *testing.T) {
	for backend, wantConfined := range map[string]bool{
		bash.BackendLandlock: true,
		bash.BackendSrt:      true,
		bash.BackendNone:     false,
		"":                   false,
		"something-new":      false,
	} {
		label, confined := sandboxLabel(backend)
		if confined != wantConfined || label == "" {
			t.Errorf("%q: got (%q, %v), want confined=%v", backend, label, confined, wantConfined)
		}
	}
}
