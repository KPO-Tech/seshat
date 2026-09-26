package read

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/internal/documentreader"
)

// A real the configured document reader instance set up with the plain `the configured document reader` package
// (no DOCLING_EXTRAS=asr) fails WAV/MP3 conversion internally - openai-whisper
// isn't installed - and surfaces it as an opaque "404 task result not found"
// rather than mentioning whisper at all. This is reproduced here by having a
// fake server return a 404 for /v1/convert/file, and asserts the tool's error
// message adds the actionable DOCLING_EXTRAS=asr hint for audio formats
// specifically, rather than leaving the agent with only the opaque upstream text.
func TestReadDocumentReaderFile_AudioConversionFailureHintsASRExtra(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/convert/file":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"Task result not found. Please wait for a completion status."}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tool := &Tool{
		config:         DefaultToolConfig(),
		documentReader: documentreader.NewDoclingClient(server.URL),
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "voicenote.wav")
	if err := os.WriteFile(path, []byte("not real audio, only the extension matters here"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}

	result, err := tool.readDocumentReaderFile(context.Background(), path, info)
	if err != nil {
		t.Fatalf("readDocumentReaderFile: %v", err)
	}
	if result.Error == nil {
		t.Fatalf("expected an error result for a failed audio conversion, got: %+v", result)
	}
	if !strings.Contains(result.Error.Error(), "DOCLING_EXTRAS=asr") {
		t.Errorf("expected the ASR-extra hint in the error message, got:\n%s", result.Error.Error())
	}
}
