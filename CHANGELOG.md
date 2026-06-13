# Changelog

All notable changes to nexus-engine are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).  
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- Session auto-title generation feature: AI auto-titles sessions after the first successful turn using the user message.
- `OnSessionTitled` callback and `DisableTitleGeneration` configuration options in `ClientConfig`.
- `CredentialResolver` interface in `pkg/sdk` — allows per-request API key injection without touching `ClientConfig.APIKey`
- `generate_image` tool backed by `image.Generation` interface (OpenAI DALL-E 3 and Google Gemini Imagen providers)
- `text_to_speech` and `speech_to_text` tools backed by pluggable audio providers (OpenAI TTS-1, Whisper)
- OpenTelemetry tracing (`internal/monitoring/tracer.go`) — OTLP gRPC export, no-op when `OTEL_EXPORTER_OTLP_ENDPOINT` is unset
- `pkg/monitoring` public package exposing `InitTracer` and `Tracer`
- `.github/workflows/ci.yml` — build, test (race), lint on push/PR
- `.github/workflows/release.yml` — cross-platform binary release on `v*` tags
- `.githooks/pre-commit` — gofmt + go vet + golangci-lint (install with `make hooks`)
- `SetDefault` and `GetDefaultByUserID` on `ProviderSettingStore`
- Community files: `SECURITY.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `AGENTS.md`
- Architecture diagrams in `docs/vision/diagrams.md` (Mermaid)
- Project vision and 3-level roadmap in `docs/vision/`

### Changed
- Makefile: replaced `build-api` (removed) with `build-grpc`; added `fmt`, `vet`, `hooks` targets
- `docs/architecture.md`: removed `cmd/api` from entry points (it lives in nexus-product)
- `docs/transports.md`: translated to English, removed HTTP API section (nexus-product), fixed absolute paths

### Fixed
- `internal/tools/files/patch/patch.go`: replaced `if HasSuffix` with `strings.TrimSuffix` (golangci-lint S1017)
- `internal/providers/auth.go`: removed OAuth credential values from log output (security: S1)

### Removed
- `cmd/api` — moved to `nexus-product` (the open-source engine does not own the HTTP product layer)
- `docs/backend-boundary.md` and `docs/internal-boundary-audit.md` — monorepo-era documents, no longer relevant
- `docs/archive/` — speculative archived docs removed

---

## [0.1.0] — Initial public release (pending)

*This release has not been cut yet. The engine is in active development on `main`.*

[Unreleased]: https://github.com/EngineerProjects/nexus-engine/compare/HEAD...HEAD
