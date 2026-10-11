package sdk_test

import (
	"context"
	"testing"

	"github.com/KPO-Tech/seshat/pkg/sdk"
)

// TestAddToolHook_UsableFromOutsideModule guards H5 in TODO.md: pkg/sdk
// aliased ToolHook, ToolHookInput and ToolHookStage, but not ToolHookResult
// or ToolHookStop - the types ToolHook.Execute itself must return - so a
// host in another module could name the hook's input type but not
// construct the result its own Execute function has to produce, making
// AddToolHook effectively unusable from outside this module.
//
// This file deliberately uses package sdk_test (external, not sdk): it must
// compile and run using nothing but the public API, exactly like a real
// consumer in a different module would write it.
func TestAddToolHook_UsableFromOutsideModule(t *testing.T) {
	client, err := sdk.NewClient(&sdk.ClientConfig{PersistSessions: false})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	hook := sdk.ToolHook{
		ID:    "external-hook",
		Stage: sdk.ToolHookStagePre,
		Execute: func(ctx context.Context, input sdk.ToolHookInput) sdk.ToolHookResult {
			return sdk.ToolHookResult{
				Stop: &sdk.ToolHookStop{Content: "blocked by external hook", IsError: true},
			}
		},
	}

	// The point of this test is that the line above and this call compile
	// and run without a host needing any internal package - that was the
	// actual bug. AddToolHook itself never errors; a running tool call is
	// not needed to prove the gap is closed.
	client.AddToolHook(hook)
}
