# Storage & Session Artifacts

Reference for tool developers who need to persist files produced during agent execution.

---

## Directory layout

All data lives under `~/.config/seshat-cli/` (or `$NEXUS_RUNTIME_ROOT`).

```
~/.config/seshat-cli/
├── seshat.db                  ← SQLite: sessions, transcripts, credentials
├── secret.key                ← AES-256 encryption key (0600)
├── logs/
│   └── app.log
├── documents/                ← user-uploaded PDFs and docs  (global, persistent)
├── rag/                      ← RAG-indexed documents        (global, persistent)
└── sessions/
    └── {session_id}/
        ├── screenshots/      ← browser screenshots
        ├── plans/            ← plan-mode markdown files
        ├── tools/            ← browser downloads
        └── artifacts/
            ├── web/          ← web-scraped / fetched content
            ├── images/       ← AI-generated images
            └── audio/        ← TTS / STT audio files
```

**Rule of thumb:**
- Content produced **by the agent for a session** → `sessions/{id}/…`
- Content **uploaded intentionally by the user** as a knowledge base → `documents/` or `rag/`

---

## Artifact store

The `storage.ArtifactStore` interface is the single entry point for persisting binary files. It supports both local filesystem and S3 backends transparently.

The store is injected into tools at construction time — never instantiate it directly inside a tool.

### Key builders (`internal/storage/keys.go`)

Each key builder returns a deterministic path string that encodes the session, type, and timestamp. Use these instead of building paths by hand.

| Function | Output path | Use for |
|---|---|---|
| `ScreenshotKey(sessionID, pageID, now)` | `sessions/{id}/screenshots/{page}/{date}/{ts}-screenshot.png` | Browser screenshots |
| `DownloadKey(sessionID, pageID, filename, now)` | `sessions/{id}/tools/{page}/{date}/{ts}-{file}` | Browser downloads |
| `WebArtifactKey(sessionID, filename, now)` | `sessions/{id}/artifacts/web/{date}/{ts}-{file}` | Web-fetched content |
| `GeneratedImageKey(sessionID, filename, now)` | `sessions/{id}/artifacts/images/{date}/{ts}-{file}` | AI-generated images |
| `AudioKey(sessionID, filename, now)` | `sessions/{id}/artifacts/audio/{date}/{ts}-{file}` | TTS / STT audio |
| `PDFKey(title, now)` | `documents/{date}/{ts}-{title}.pdf` | Global PDF documents |
| `DocumentKey(filename, now)` | `documents/{date}/{ts}-{file}` | Global user documents |

### Store helpers (`internal/storage/artifacts.go`)

Higher-level wrappers that pick the right key and call `store.Put` for you:

```go
// Browser screenshots — called by the browser tool internally.
StoreScreenshotRef(ctx, store, data, sessionID, pageID string) (ArtifactRef, error)

// Web-fetched binary content — called by the fetch service internally.
StoreWebArtifactRef(ctx, store, data []byte, sessionID, filename, contentType string) (ArtifactRef, error)

// AI-generated image — call this from your image-generation tool.
StoreGeneratedImageRef(ctx, store, data []byte, sessionID, filename, contentType string) (ArtifactRef, error)

// TTS/STT audio — call this from your audio tool.
StoreAudioRef(ctx, store, data []byte, sessionID, filename, contentType string) (ArtifactRef, error)

// Global PDFs (not session-scoped).
StorePDFRef(ctx, store, data []byte, title string) (ArtifactRef, error)
```

Pass `nil` as `store` and the default process-wide store is used automatically.

### Example: image generation tool

```go
func (t *ImageGenTool) Execute(ctx context.Context, input Input) (Output, error) {
    imageData, err := t.provider.Generate(ctx, input.Prompt)
    if err != nil {
        return Output{}, err
    }
    ref, err := storage.StoreGeneratedImageRef(
        ctx,
        t.artifactStore,      // injected at construction
        imageData,
        string(t.sessionID),  // from tool context
        "generated.png",
        "image/png",
    )
    if err != nil {
        return Output{}, err
    }
    return Output{URL: ref.URL}, nil
}
```

### Example: audio (TTS) tool

```go
ref, err := storage.StoreAudioRef(ctx, store, audioBytes, sessionID, "speech.mp3", "audio/mpeg")
```

---

## Adding a new artifact type

1. Add a key builder in `internal/storage/keys.go`:
   ```go
   func MyTypeKey(sessionID, filename string, now time.Time) string {
       // Layout: sessions/{sessionID}/artifacts/mytype/{date}/{ts}-{file}
       parts := []string{"sessions", sanitizePathSegment(sessionID), "artifacts", "mytype", ...}
       ...
   }
   ```

2. Add a store helper in `internal/storage/artifacts.go`:
   ```go
   func StoreMyTypeRef(ctx context.Context, store ArtifactStore, data []byte, sessionID, filename, contentType string) (ArtifactRef, error) {
       key := MyTypeKey(sessionID, filename, time.Now().UTC())
       return store.Put(ctx, key, data, contentType)
   }
   ```

3. Add a `SessionMyTypeDir` function in `pkg/runtimepath/runtimepath.go` and expose it in `cmd/cli/appdir/appdir.go`.

4. Add the new directory to `appdir.EnsureSessionDir` so it's created when a session opens.

No changes to the deletion logic — `appdir.DeleteSessionDir` removes the entire `sessions/{id}/` tree.

---

## Session lifecycle

```
Session open / resume
  └── appdir.EnsureSessionDir(id)          ← creates all subdirs

Agent runs
  └── tools write artifacts via store helpers

Session deleted (user presses 'd' in session browser)
  ├── store.DeleteSession(id)              ← removes DB rows (cascade)
  └── appdir.DeleteSessionDir(id)          ← os.RemoveAll(sessions/{id}/)
      covers: screenshots, plans, tools, artifacts/web, artifacts/images, artifacts/audio
```

For S3 storage, `client.DeleteSession` additionally calls `store.List("sessions/{id}") + store.Delete` for each key, since `os.RemoveAll` only works on the local filesystem.

---

## Path helpers

Access paths from application code via `cmd/cli/appdir`:

```go
appdir.Root()                          // ~/.config/seshat-cli/
appdir.SessionDir(id)                  // sessions/{id}/
appdir.SessionScreenshotsDir(id)       // sessions/{id}/screenshots/
appdir.SessionPlansDir(id)             // sessions/{id}/plans/
appdir.SessionToolsDir(id)             // sessions/{id}/tools/
appdir.SessionArtifactsDir(id)         // sessions/{id}/artifacts/
appdir.SessionArtifactsWebDir(id)      // sessions/{id}/artifacts/web/
appdir.SessionArtifactsImagesDir(id)   // sessions/{id}/artifacts/images/
appdir.SessionArtifactsAudioDir(id)    // sessions/{id}/artifacts/audio/
appdir.SessionLogPath(id)              // sessions/{id}/session.log
```

Internal packages use `pkg/runtimepath` directly (same functions, with an explicit `root` param).

> **Note:** `appdir` is application-level — only `cmd/cli` imports it. Internal packages (`internal/…`) receive paths as explicit parameters and do not import `appdir`.
