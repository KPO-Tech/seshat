package workspace

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/db"
	"github.com/KPO-Tech/seshat-runtime/tui/internal/seshattui/config"
)

// storedModel reads back the model the workspace wrote to the credentials DB.
func storedModel(t *testing.T, dbPath string) (string, bool) {
	t.Helper()
	database, err := db.Open(context.Background(), db.DefaultSQLiteConfig(dbPath))
	if err != nil {
		t.Fatalf("open credentials db: %v", err)
	}
	defer database.Close()
	value, ok, err := database.GetCredential(context.Background(), credKeyModel)
	if err != nil {
		t.Fatalf("read credential: %v", err)
	}
	return value, ok
}

func TestUpdatePreferredModelKeepsTheLargeChoiceAcrossRestarts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "seshat.db")
	t.Setenv("SESHAT_DB_PATH", dbPath)
	t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())

	w := NewSeshatWorkspace(nil, t.TempDir(), "")
	large := config.SelectedModel{Provider: "z-ai", Model: "glm-5.3"}
	if err := w.UpdatePreferredModel(config.ScopeGlobal, config.SelectedModelTypeLarge, large); err != nil {
		t.Fatal(err)
	}
	if w.model != "z-ai:glm-5.3" {
		t.Fatalf("session model = %q", w.model)
	}
	if got, ok := storedModel(t, dbPath); !ok || got != "z-ai:glm-5.3" {
		t.Fatalf("stored model = %q (found %v), want z-ai:glm-5.3", got, ok)
	}

	// Right after a large model is picked the interface sets a small one. That
	// must neither replace the session model nor the remembered choice.
	small := config.SelectedModel{Provider: "z-ai", Model: "glm-5.3-flash"}
	if err := w.UpdatePreferredModel(config.ScopeGlobal, config.SelectedModelTypeSmall, small); err != nil {
		t.Fatal(err)
	}
	if w.model != "z-ai:glm-5.3" {
		t.Fatalf("a small model replaced the session model: %q", w.model)
	}
	if got, _ := storedModel(t, dbPath); got != "z-ai:glm-5.3" {
		t.Fatalf("a small model replaced the stored choice: %q", got)
	}
}

func TestUpdatePreferredModelIgnoresAnEmptySelection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "seshat.db")
	t.Setenv("SESHAT_DB_PATH", dbPath)
	t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())

	w := NewSeshatWorkspace(nil, t.TempDir(), "openai:gpt-6.1-sol")
	if err := w.UpdatePreferredModel(config.ScopeGlobal, config.SelectedModelTypeLarge, config.SelectedModel{Provider: "openai"}); err != nil {
		t.Fatal(err)
	}
	if w.model != "openai:gpt-6.1-sol" {
		t.Fatalf("an empty model replaced the session model: %q", w.model)
	}
}
