package db

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openTemp(t *testing.T, path string) *DB {
	t.Helper()
	database, err := Open(context.Background(), DefaultSQLiteConfig(path))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func countRows(t *testing.T, database *DB, table string) int {
	t.Helper()
	var n int
	if err := database.SQL().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestOpenRefusesAnIncompleteConfig(t *testing.T) {
	t.Parallel()
	if _, err := Open(context.Background(), Config{DSN: "x.db"}); err == nil {
		t.Error("a config with no driver must be refused")
	}
	if _, err := Open(context.Background(), Config{Driver: DriverSQLite}); err == nil {
		t.Error("a config with no DSN must be refused")
	}
}

// Migrations are recorded, and opening the same file again applies none of them a second time.
func TestMigrationsAreAppliedOnceAndRecorded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "seshat.db")
	first := openTemp(t, path)
	if err := first.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	recorded := countRows(t, first, migrationTableName)
	if recorded == 0 {
		t.Fatal("opening a new database must record the migrations it applied")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second := openTemp(t, path)
	if got := countRows(t, second, migrationTableName); got != recorded {
		t.Errorf("reopening applied migrations again: %d recorded, then %d", recorded, got)
	}
}

// A credential is stored encrypted, comes back as it was written, and can be listed and deleted.
func TestCredentialsAreEncryptedAtRest(t *testing.T) {
	t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())
	database := openTemp(t, filepath.Join(t.TempDir(), "seshat.db"))
	ctx := context.Background()
	const secret = "sk-this-must-not-be-stored-in-clear-1234567890"

	if err := database.UpsertCredential(ctx, "openai", secret); err != nil {
		t.Fatalf("UpsertCredential: %v", err)
	}
	var stored string
	if err := database.SQL().QueryRowContext(ctx, "SELECT cipher_text FROM credentials WHERE key = ?", "openai").Scan(&stored); err != nil {
		t.Fatalf("read the row: %v", err)
	}
	if stored == "" || strings.Contains(stored, secret) || strings.Contains(stored, "this-must-not-be") {
		t.Fatalf("the value must be stored encrypted, got %q", stored)
	}

	got, ok, err := database.GetCredential(ctx, "openai")
	if err != nil || !ok || got != secret {
		t.Fatalf("GetCredential = (%q, %v, %v), want the secret back", got, ok, err)
	}

	if err := database.UpsertCredential(ctx, "openai", "replaced"); err != nil {
		t.Fatalf("second UpsertCredential: %v", err)
	}
	if got, _, _ := database.GetCredential(ctx, "openai"); got != "replaced" {
		t.Errorf("an upsert must replace the value, got %q", got)
	}

	if _, ok, err := database.GetCredential(ctx, "missing"); ok || err != nil {
		t.Errorf("a missing key is (\"\", false, nil), got ok=%v err=%v", ok, err)
	}

	keys, err := database.ListCredentialKeys(ctx)
	if err != nil || len(keys) != 1 || keys[0] != "openai" {
		t.Errorf("ListCredentialKeys = %v, %v", keys, err)
	}
	if err := database.DeleteCredential(ctx, "openai"); err != nil {
		t.Fatalf("DeleteCredential: %v", err)
	}
	if _, ok, _ := database.GetCredential(ctx, "openai"); ok {
		t.Error("a deleted credential must be gone")
	}

	keyFile := filepath.Join(os.Getenv("SESHAT_RUNTIME_ROOT"), "secret.key")
	data, err := os.ReadFile(keyFile)
	if err != nil || len(data) != 32 {
		t.Fatalf("the encryption key is written to %s with 32 bytes: %v, %d bytes", keyFile, err, len(data))
	}
}

func TestAESGCMRefusesATamperedOrForeignCiphertext(t *testing.T) {
	t.Parallel()
	key := bytes.Repeat([]byte{7}, 32)
	other := bytes.Repeat([]byte{9}, 32)

	encoded, err := encryptAESGCM(key, []byte("hello"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	again, _ := encryptAESGCM(key, []byte("hello"))
	if encoded == again {
		t.Error("two encryptions of the same text must differ (random nonce)")
	}
	if plain, err := decryptAESGCM(key, encoded); err != nil || string(plain) != "hello" {
		t.Fatalf("round trip = (%q, %v)", plain, err)
	}
	if _, err := decryptAESGCM(other, encoded); err == nil {
		t.Error("another key must not decrypt it")
	}
	flipped := []byte(encoded)
	if flipped[len(flipped)-2] == 'A' {
		flipped[len(flipped)-2] = 'B'
	} else {
		flipped[len(flipped)-2] = 'A'
	}
	if _, err := decryptAESGCM(key, string(flipped)); err == nil {
		t.Error("a modified ciphertext must be refused")
	}
	if _, err := decryptAESGCM(key, "not base64 !!"); err == nil {
		t.Error("garbage must be refused")
	}
}
