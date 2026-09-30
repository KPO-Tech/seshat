package builtin

import (
	"testing"

	"github.com/KPO-Tech/seshat/internal/tools/registry"
)

// wantRegistered lists a representative tool from each family wired in
// RegisterBuiltinToolsWithConfig: catching an accidental removal or a typo'd
// tool name here is cheap, versus discovering it only when a host runtime
// tries to resolve the tool at request time.
var wantRegistered = []string{
	"bash",
	"read_file",
	"convert_document",
	"read_document_url",
	"render_document_page",
	"glob",
	"grep",
	"generate_image",
	"text_to_speech",
	"speech_to_text",
	"calculator",
	"unit_convert",
	"statistics",
	"financial_calc",
	"rag_search",
	"web_search",
	"web_fetch",
}

func TestRegisterBuiltinTools_NilConfig(t *testing.T) {
	reg := registry.NewRegistry()
	if err := RegisterBuiltinTools(reg); err != nil {
		t.Fatalf("RegisterBuiltinTools: %v", err)
	}
	for _, name := range wantRegistered {
		if !reg.HasTool(name) {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}

func TestNewBuiltinRegistry_NoDuplicateRegistrations(t *testing.T) {
	reg, err := NewBuiltinRegistry()
	if err != nil {
		t.Fatalf("NewBuiltinRegistry: %v", err)
	}
	if reg.Count() == 0 {
		t.Fatal("expected at least one registered tool")
	}
}

func TestNewBuiltinRegistryWithConfig_EmptyConfig(t *testing.T) {
	reg, err := NewBuiltinRegistryWithConfig(&Config{})
	if err != nil {
		t.Fatalf("NewBuiltinRegistryWithConfig: %v", err)
	}
	// render_document_page must always register - it degrades to a
	// "not configured" text result at Call() time rather than being omitted,
	// same as convert_document/read_document_url.
	tl, ok := reg.Get("render_document_page")
	if !ok {
		t.Fatal("expected render_document_page to be registered even without DocumentPageRenderer configured")
	}
	if !tl.IsEnabled() {
		t.Error("expected render_document_page to report enabled regardless of renderer configuration")
	}
}
