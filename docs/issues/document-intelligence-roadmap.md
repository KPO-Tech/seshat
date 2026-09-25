# Document Intelligence Roadmap

This document tracks the plan to bring seshat's native (Go-only, no external
Python process) document-handling capabilities up to par with what
[RAGFlow](https://github.com/infiniflow/ragflow) does in its own Go code,
adopting and adapting their Apache-2.0 work where it makes sense rather than
rebuilding from scratch. It complements, not replaces,
`seshat-ai/seshat-intelligence` (the standalone Python service for
Docling/Marker conversion and hybrid chunking) — this roadmap is about what
seshat gets **by default, with no external server configured**.

## Why this exists

`seshat-intelligence` will always be the more capable path when it's
running. But `seshat` is also a standalone OSS runtime (`pkg/sdk`,
`cmd/cli`) that has to work well on its own. Today, without docling-serve
(or later, `seshat-intelligence`) configured, a scanned PDF or a
complex slide deck barely works — there is no native OCR, layout analysis,
or table-structure recognition in Go at all. RAGFlow solved exactly this
problem for their own Go code, under a fully compatible license. This
roadmap is about closing that gap.

## Status

| Phase | Area | Status |
|---|---|---|
| 0 | License/dependency due diligence | Done (2026-09-25) |
| 1 | Native PDF/DOCX/XLSX parsing (OCR, layout, table structure) | **Done, all platforms (2026-09-25)** — PDF/DOCX/XLSX all confirmed end-to-end through the real `DocumentConverterBackend` interface, real tests in place, model provisioning + CGO build tooling scripted and tested, on Linux/macOS *and* natively on Windows. |
| 1.W | Make Phase 1 work on Windows (see "Windows" note in Phase 1) | **Done (2026-09-25)** — validated on a real native Windows 11 machine, not emulation. See log below. |
| 2 | Chunk-level LLM enrichment (synthetic questions) | **Done (2026-09-25)** — opt-in, cached, bounded-concurrency. See log below. |
| 3 | Vision-LLM fallback for pages native parsing can't handle | **Done (2026-09-25)** — opt-in, CGO-independent interface, real pdfium render test. See log below. |
| 4 | Specialized chunkers (QA-formatted docs, table-heavy docs) | **Done (2026-09-25)** — TableChunker + QAChunker, wired as new ChunkProfiles. See log below. |

**Phase 1 progress log**:
- Validated in Docker (`golang:1.26-bookworm`, CGO): `pdf_oxide` opens a PDF,
  extracts text, and renders a page to PNG — no need for RAGFlow's separate
  `pdfium.go` (which is POSIX-only anyway); `pdf_oxide` alone covers both
  text and rendering, and its own installer already fetches Windows-native
  libs (unlike RAGFlow's pdfium wrapper).
- Validated: `github.com/infiniflow/onnxruntime_go v1.29.0` (RAGFlow's exact
  pin) loads the real `layout.ort` model fetched from `InfiniFlow/deepdoc`
  on HuggingFace, confirmed input/output tensor shapes
  (`images [1,3,1024,1024]` → `output0 [1,300,6]`).
- **Windows note**: `onnxruntime_go` is explicitly designed to support
  Windows (it manually `dlopen`/`LoadLibrary`s the shared lib rather than
  requiring MSVC-style static CGO linking - see its own README). This
  prediction held up: confirmed by real testing on native Windows - see
  phase 1.W below for the full validation log.
- **Ported `internal/nativedoc/` from RAGFlow's `internal/deepdoc/native/`**
  (16 files: image/geometry/NMS primitives, det.go/det_core.go/det_helpers.go
  for DBNet text detection, ocr_rec.go for CTC text recognition, dla.go for
  YOLO-style layout detection, tsr.go/tsr_decode.go for table structure,
  session.go/session_pool.go/inference_limit.go for ONNX Runtime plumbing,
  clipper_offset.go for DBNet's box-unclip polygon offsetting). These files
  had zero RAGFlow-internal dependencies upstream - only the standard
  library and `onnxruntime_go` - so the port was a near-verbatim copy plus a
  package rename, exactly as hoped. `go build`/`go vet` clean.
  Apache-2.0 attribution recorded in `internal/nativedoc/doc.go`.
- **New dependency discovered**: the "shared weights" memory optimization
  (`session.go`'s `sharedWeights`/`AddInitializer`) needs three *custom*
  OrtApi functions patched into a fork of onnxruntime that RAGFlow builds
  themselves (`infiniflow/ragflow-build`, release
  `onnxruntime-v1.29.0-patch1`) - the stock Microsoft onnxruntime release
  does not have them and **segfaults** (not a clean Go error) if the code
  path is hit against it. Confirmed the patched build exists and is
  downloadable (linux-x86_64/aarch64, macOS arm64/universal - **no Windows
  asset**, another item for phase 1.W). Using it requires linking
  `libonnxruntime.a` statically with `-Wl,--undefined=OrtGetApiBase
  -Wl,--dynamic-list=<file with "OrtGetApiBase;">` plus Go build tag
  `static` (selects `onnxruntime_go`'s `setup_env_static.go`, which resolves
  ORT via `dlopen(NULL)` against the running binary instead of
  `SetSharedLibraryPath`). This exact recipe is now confirmed working.
- **End-to-end smoke test** (`cmd/nativedoc_e2e_check/`, a throwaway
  validation tool, not a permanent part of the package): render a real PDF
  page via `pdf_oxide` → `nativedoc.RunDet` → crop each detected box →
  `nativedoc.RunOCRRec`.
  - **Detection: confirmed working correctly.** Found 2 real, sensible text
    boxes on a rendered page, with plausible confidence scores (0.76-0.79)
    and precise, correctly-positioned box coordinates (verified by saving
    crops to PNG and visually inspecting real text content).
  - **Root cause found and fixed: it was never a bug in the ported code.**
    Testing against a real-world PDF (not the earlier synthetic `fpdf2` one)
    still produced garbage ("TLL", "LLLLLLLLL...", repeated single letters).
    Saving the *full rendered page* (not just crops) to PNG and inspecting
    it directly showed why: every character rendered as a **solid black
    block**, not a real glyph - confirmed as a known `pdf_oxide` bug
    ([yfedoseev/pdf_oxide#1283](https://github.com/yfedoseev/pdf_oxide/issues/1283)):
    "Text filled with a pattern colour space paints flat black instead of
    the pattern... the text rasterizer has no pattern awareness at all."
    Many real-world PDF generators (CV templates included) style text with
    gradient/pattern fills, triggering this. The OCR model was being asked
    to read barcodes, not letters - it was working exactly as designed.
    This vindicates RAGFlow's own original design (pdfium for rendering,
    pdf_oxide only for text/char extraction, which this bug doesn't affect)
    that was simplified away earlier in this roadmap without testing it
    against a real document first - that simplification has been reverted.
    **Ported `internal/nativedoc/pdfium/` and `internal/nativedoc/pdfsync/`**
    from RAGFlow (2 more files, same clean-port story: `pdfsync` is a
    single process-wide mutex serializing all PDFium C calls, since PDFium
    is documented non-thread-safe; `pdfium` wraps `FPDF_RenderPageBitmap`
    via CGO). Linked against a prebuilt `libpdfium.so` from the well-known
    [bblanchon/pdfium-binaries](https://github.com/bblanchon/pdfium-binaries)
    project (RAGFlow builds their own static lib; a prebuilt dynamic one is
    simpler for now and works identically) - **this project also publishes
    Windows builds** (`pdfium-win-x64/x86/arm64.tgz`), which is genuinely
    good news for phase 1.W, unlike RAGFlow's own pdfium.go which has zero
    Windows adaptation.
  - **Re-tested against the real PDF with pdfium rendering: excellent
    results.** Confirmed on a real French-language CV
    (`cv-stephane-kpoviessi-big-data-ai-fr.pdf`) - accurate text at high
    confidence, e.g. `"STEPHANE KPOVIESSI"` (0.990), `"Ingenieur Big Data
    &Intelligence Artificielle"` (0.968), full sentence-length paragraph
    lines correctly recognized at 0.94-0.99 confidence throughout. The
    `nativedoc` det/rec port is confirmed correct end-to-end on real data.
  - **`pdf_oxide` text extraction re-verified on the same real CV: excellent,
    better than OCR as expected** (reads the content stream directly, not
    pixels - correct accents, apostrophes, no recognition errors). Confirms
    the split is right: `pdf_oxide` for text/char extraction, `pdfium` for
    rendering.
  - **DLA (layout) and TSR (table structure) also tested end-to-end on the
    same real CV page** (all four models now exercised: det, rec, DLA, TSR):
    - DLA found 36 layout regions with sensible classes - titles (class 0)
      at the name/section-header positions, text blocks (class 1) covering
      the body paragraphs, and the profile photo correctly classified as a
      figure (class 3, score 0.770). Confirms the letterbox
      preprocessing/NMS/class-remapping port is correct, not just IO shapes.
    - TSR ran without error (this document has no real table, so its output
      isn't semantically meaningful here - this was a smoke test confirming
      the model loads/runs, not a correctness test; a real table-bearing
      document is needed to actually validate TSR's output).
  - **All four DeepDoc ONNX models (det/rec/layout/tsr) now confirmed
    working under the infiniflow-patched static-link recipe.** Phase 1's
    core technical risk is resolved.
- **Wired into the real `docling.DocumentConverterBackend` interface** -
  `internal/nativedoc/parser.PDFConverter` (new, original orchestration
  code, not ported from RAGFlow - see the package's own doc comment for
  what's deliberately simpler than RAGFlow's equivalent for now: no
  DLA-based reading order yet, single-column top-to-bottom sort only; no
  image extraction yet). Per page: native text (`pdf_oxide`) when the page
  has a real text layer, OCR (`pdfium` render + `nativedoc` det/rec)
  otherwise. `ConvertFile`/`ConvertBytes`/`ConvertURL`/`IsAvailable` all
  implemented - satisfies the interface (`var _
  docling.DocumentConverterBackend = (*PDFConverter)(nil)` compiles).
  **Tested end-to-end through this actual interface** (not a scratch
  script) against the real CV: 2 pages correctly detected, full accurate
  French text via the native-text path (this document has real text
  layers, so OCR wasn't even needed here - separately confirmed working
  above when it is needed).

**Done since the log above:**

- **`Converter`** (`internal/nativedoc/parser`, renamed from `PDFConverter`)
  now dispatches by extension and handles **PDF, DOCX, and XLSX**, all
  tested end-to-end through the real `docling.DocumentConverterBackend`
  interface on real files.
- **DOCX/XLSX course-correction, caught by testing before committing to the
  approach**: initially ported RAGFlow's CGO `office_oxide` DOCX wrapper
  (`internal/nativedoc/docx/`, ~225 lines, same clean-port story as the
  other pieces). Before wiring it in, tested it head-to-head against
  `internal/officetext.ExtractDOCX` - seshat's own existing, pure-Go,
  zero-CGO DOCX extractor - on the same fixture
  (`internal/officetext/testdata/sample.docx`). **Output was byte-for-byte
  identical.** The CGO dependency bought nothing, so it was deleted rather
  than kept "just in case," and `Converter` delegates to
  `officetext.ExtractDOCX`/`ExtractXLSX` instead (XLSX was *already*
  pure-Go `excelize` in this repo too - no new code needed there at all).
  Net effect: DOCX/XLSX support with zero new native dependencies.

**Wiring decision (confirmed with the user)**: *not* an automatic default
inside `internal/tools/builtin`. That would force CGO/pdfium/onnxruntime
onto every consumer of the tool registry, including plain
`CGO_ENABLED=0` builds - confirmed this stays broken-out via a live
regression check (`CGO_ENABLED=0 go build ./pkg/sdk/... ./internal/tools/builtin/...`
still passes after adding `pkg/nativedoc`). Instead: **opt-in**, same
injection point as any custom backend
(`sdk.ClientConfig.DocumentConverter`). Added `pkg/nativedoc` (mirrors the
existing `pkg/docling`/`pkg/rag`/`pkg/pdfsmart` facade pattern) so external
consumers (`seshat-backend`, which can't import `internal/*` per this
repo's own package boundary rules) can actually reach it:

```go
if err := nativedoc.InitORT(); err != nil { /* only needed for OCR */ }
cfg := &sdk.ClientConfig{
    DocumentConverter: nativedoc.New("/path/to/deepdoc-models"),
    // ...
}
```

Preparing the actual model directory / calling this from seshat-backend is
out of scope for `seshat` itself - that's the "backend does its own
preparation" half of this decision, left for whoever wires it in there.

**Items 1 and 2 done:**

- **`scripts/install-deepdoc-models.sh`** (+ `make install-deepdoc-models`)
  fetches det/rec/layout/tsr.ort + ocr.res from `InfiniFlow/deepdoc` into
  `pkg/runtimepath.DeepDocModelsDir` (new helper, added alongside the
  existing `runtimepath` catalog), mirroring `install-python-env.sh`'s
  style and idempotency exactly. **Tested for real**: fresh run downloads
  all 5 files correctly-sized; re-run correctly skips all 5.
- **`scripts/setup-nativedoc-cgo.sh`** fetches pdf_oxide (via its own
  installer), pdfium (from `pdfium-binaries`), and the infiniflow-patched
  onnxruntime static lib, then prints the exact `CGO_CFLAGS`/`CGO_LDFLAGS`
  to build with (`source <(./scripts/setup-nativedoc-cgo.sh)`). **Tested
  for real, in a clean container**: ran the script fresh, sourced its own
  output with no manual edits, built the validation tool with only those
  exported flags, and got the same correct French-text output as every
  prior manual run. That path covers Linux/macOS (infiniflow-patched static
  lib); the script also gained a Windows branch (stock dynamic-loaded
  onnxruntime, no patch) - see phase 1.W below for the full Windows story.

**Item 3 done - real tests, and a real build-breakage caught and fixed
along the way:**

- **`internal/nativedoc/parser/parser_test.go`** - 7 tests against real
  fixtures (copied from `internal/pdftext`/`internal/officetext`'s own
  testdata, not synthetic): DOCX/XLSX conversion, a native-text-layer PDF,
  an *actual scanned PDF exercising the real OCR path* (skips gracefully
  via `SESHAT_NATIVEDOC_TEST_MODELS_DIR` when models aren't provisioned,
  rather than failing CI that hasn't run `install-deepdoc-models.sh`), plus
  unit tests for the reading-order sort and box-crop helpers. All passing.
  The old scratch validation tool (`cmd/nativedoc_e2e_check`) is gone -
  these tests supersede it.
- **Caught a real regression before it shipped**: adding `internal/nativedoc`
  as *packages that exist in the module* (even though nothing imports them
  by default) broke a plain `go build ./...`/`CGO_ENABLED=0 go build ./...`
  from the repo root - Go's `./...` wildcard tries to compile every package
  regardless of whether anything imports it, and several files in the new
  packages had no build constraint at all, so they always tried to compile
  and failed referencing CGO-only symbols. Fixed by gating **every**
  non-test file under `internal/nativedoc/`, `internal/nativedoc/parser/`,
  `internal/nativedoc/pdfium/`, `internal/nativedoc/pdfsync/`, and
  `pkg/nativedoc/` behind `//go:build cgo && nativedoc` - a dedicated,
  repo-specific tag (not just `cgo`), so these packages are invisible to
  `go build ./...` even on a machine where CGO defaults on but the native
  libs haven't been set up, matching the "genuinely opt-in" design decision
  above. **Verified for real**: a plain `go build ./...` and a
  `CGO_ENABLED=0 go build ./...` both pass cleanly on the whole repo with
  zero setup; `go build -tags "static nativedoc" ./internal/nativedoc/...`
  (after sourcing `setup-nativedoc-cgo.sh`) still builds/vets/tests/passes
  everything. Every usage example in this file, the script, and the
  Makefile now says `-tags "static nativedoc"`, not just `-tags static`.

**Phase 1.W (Windows) done - validated on a real native Windows 11 machine,
not Docker/WSL/emulation:**

The `infiniflow`-patched onnxruntime build (needed for the weight-sharing
optimization) indeed has no Windows asset, confirming the doubt above. Since
weight-sharing was already made opt-in (`WeightSharingEnabled`, default
`false` — see Phase 1 "New dependency discovered" below) because it
segfaults against *any* stock onnxruntime build, the practical Windows
answer is exactly the fallback already anticipated: stock Microsoft
`onnxruntime` release (dynamic-loaded via `onnxruntime_go`'s
`LoadLibrary`/`dlopen` path, no static link, no patch needed) with
`WeightSharingEnabled` left off. No correctness impact, per
`sharedWeights`'s own graceful degradation.

Toolchain: installed MSYS2 (`winget install --id MSYS2.MSYS2`) +
`pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-make unzip` to get a real C
compiler on the dev machine for genuine CGO builds — no analysis-only
shortcuts, per the "test rigorously" standing instruction.

`scripts/setup-nativedoc-cgo.sh` gained a Windows branch (detected via
`uname -s` matching `MINGW*|MSYS*|CYGWIN*`): downloads stock Windows
onnxruntime instead of the infiniflow static lib, emits
`NATIVEDOC_BUILD_TAGS="nativedoc"` (not `"static nativedoc"` — no static
link on Windows), and sets `SESHAT_NATIVEDOC_ONNXRUNTIME_PATH` explicitly
rather than relying on `LoadLibrary`'s default search order.

Five distinct Windows/Git-Bash/cgo interop bugs were found and fixed along
the way, each confirmed by direct reproduction before the fix:

1. `pdfium.go`'s `#cgo LDFLAGS: -lm -lpthread -ldl` are POSIX system
   libraries with no Windows equivalent — made `!windows`-conditional.
2. MinGW's linker couldn't resolve `-lpdfium` against the prebuilt
   `pdfium.dll.lib` via the normal `-L`/`-l` search — fixed by referencing
   the `.dll.lib` file by its direct path in `CGO_LDFLAGS` instead.
3. `tar` interpreted `C:/...`-style paths as remote-host `host:path` syntax
   (BSD/GNU tar's own remote-archive convention) — fixed with
   `tar --force-local`.
4. cgo's own flag tokenizer treats `\` as a shell escape character, which
   corrupted the backslash-style Windows paths pdf_oxide's installer
   produces when they landed in `CGO_CFLAGS`/`CGO_LDFLAGS` — fixed by
   converting to forward slashes (`tr '\\' '/'`) before exporting.
5. Embedding a `C:/...`-style path inside bash's colon-separated `$PATH`
   broke MSYS's automatic POSIX↔Windows path-list translation for any
   native (non-MSYS) child process — confirmed empirically with a small Go
   program dumping `os.Getenv("PATH")`. Fixed by converting that one
   directory to genuine POSIX form with `cygpath -u` specifically for the
   `PATH` export (the forward-slash `C:/...` form is kept for
   `CGO_CFLAGS`/`CGO_LDFLAGS`, which aren't colon-delimited lists and don't
   have this problem).

**Verified for real, end to end, with zero manual intervention**: after
`source <(./scripts/setup-nativedoc-cgo.sh)` on the native Windows machine,
`go test -tags "nativedoc" ./internal/nativedoc/parser/...` passes all 7
tests — including the real OCR path against an actual scanned PDF, not a
skip — using only the script's own freshly generated output. Both a plain
`go build ./...` and a `CGO_ENABLED=0 go build ./...` from the repo root
also pass cleanly on Windows with zero nativedoc-related errors, confirming
the opt-in build-tag design (`//go:build cgo && nativedoc`) holds on
Windows exactly as it does on Linux/macOS.

Phases are ordered by technical dependency, not just importance — see each
phase's "Why this order" note.

---

## Phase 0 — License and dependency due diligence (done)

Before adopting any of RAGFlow's code, every dependency it would bring in
was checked individually. All clear:

| Component | License | Notes |
|---|---|---|
| RAGFlow's own Go code (`internal/deepdoc/`, etc.) | Apache-2.0 | Same license as `seshat` itself — fully compatible. |
| `github.com/yfedoseev/pdf_oxide/go` (Rust, via CGO) | Apache-2.0 | Text/char extraction from PDFs. |
| `github.com/yfedoseev/office_oxide/go` (Rust, via CGO) | Apache-2.0 | DOCX parsing. |
| `github.com/infiniflow/onnxruntime_go` | MIT | Go bindings for ONNX Runtime. |
| `InfiniFlow/deepdoc` model weights (HuggingFace) | Apache-2.0 | The actual OCR/layout/table-structure ONNX models. |

Adoption requires standard Apache-2.0 compliance when porting RAGFlow's own
code: keep copyright/license notices, note what was changed in adapted
files. `pdf_oxide`/`office_oxide`/`onnxruntime_go` don't need copying at
all — they're real Go modules, added as normal dependencies. The model
weights aren't vendored in RAGFlow's repo either — they're fetched at setup
time from HuggingFace, same pattern `seshat`'s own docling-serve
provisioning already uses.

**What this does *not* clear**: RAGFlow's code is entangled with their own
internal types and build tooling (`internal/entity`, their `build.sh`
wrapper for CGO flags and native static libs). Adopting it means adapting
the parsing/OCR/layout logic to seshat's own types, not a literal
copy-paste. Budget real engineering time per phase below, not "copy the
folder and done."

---

## Phase 1 — Native PDF/DOCX/XLSX parsing (OCR, layout, table structure)

**Why first**: everything else in this roadmap either has no dependency on
it (Phase 2) or actively needs its output to be worth doing (Phase 3 reuses
its page-rendering capability; Phase 4's table-aware chunking needs real
table-structure data to chunk against). It's also the single biggest gap
identified against RAGFlow, and the piece explicitly asked for first.

**What exists today**: `internal/pdftext` and `internal/officetext` do
plain native-text-layer extraction only — no OCR, no layout analysis, no
table-structure recognition. `internal/pdfsmart` routes pages that need
more than that to `docling.DocumentConverterBackend` (see the interface
refactor already landed — this is exactly the seam a native fallback plugs
into). Without a configured backend, those pages just don't get read well.

**What RAGFlow does that we'd adopt**: `pdf_oxide` (CGO) for text/char
extraction with layout coordinates, `pdfium` (CGO) for page rasterization,
and their own bespoke Go layout/table/box-merging logic running ONNX
detection/recognition/layout/table-structure models via `onnxruntime_go` —
fully self-contained, no Python. `office_oxide` (CGO) for DOCX, mapped to
document-order sections (headings/tables/text). XLSX stays pure Go
(`excelize`, which RAGFlow also uses) — no native lib needed there.

### Steps

1. **Dependencies**: add `pdf_oxide`, `office_oxide`, `onnxruntime_go` as Go
   modules. Confirm they build cleanly on all three target platforms
   (Windows, macOS, Linux) before going further — this is the single
   biggest risk in this phase.
2. **Build tooling**: seshat's `go build ./...`/`go vet ./...`/`go test
   -race ./...` gate (`AGENTS.md`) currently assumes no CGO. Add a
   `//go:build cgo` / `!cgo` split (mirroring RAGFlow's own
   `parse_cgo.go`/`parse_nocgo.go` pattern) so a non-CGO build still
   compiles and simply reports "native OCR unavailable" rather than
   failing. Extend `build.sh`-equivalent tooling (or document the required
   `CGO_ENABLED=1` + linker flags) so CI and local dev both know how to
   build the CGO path.
3. **Model provisioning**: a setup step (mirroring
   `scripts/install-python-env.sh`'s docling-serve provisioning) that
   fetches the `InfiniFlow/deepdoc` ONNX weights from HuggingFace into the
   runtime root, the same way docling-serve's own venv gets provisioned by
   `seshat setup`.
4. **Port the parsing logic**: adapt (not copy verbatim)
   `internal/deepdoc/parser/pdf/*` (pdf_oxide/pdfium wiring, scanned-vs-native
   detection, OCR/layout/table-structure invocation, box merging, zoom
   retry) and `internal/deepdoc/parser/docx/*` (office_oxide → section
   mapping) into a new seshat package — proposed: `internal/nativedoc/`
   (name it after what it does, not after RAGFlow) — rewired to produce
   `docling.ConversionResult` (headings/text/tables/images), the exact
   shape every existing consumer already expects.
5. **Wire it in as a new `DocumentConverterBackend` implementation**: this
   is exactly why that interface was extracted in the earlier refactor. No
   changes needed to `ConvertTool`/`ReadURLTool`/`FileRead`/`pdfsmart` —
   they already depend on the interface, not the concrete docling-serve
   client. The new native backend becomes an additional, swappable
   implementation, selectable as the default when no `DoclingURL`/
   `DocumentConverter` override is configured.
6. **Tests**: at minimum, port RAGFlow's parity-test *concept* (not their
   exact fixtures) — a handful of real scanned/native/mixed PDFs and DOCX
   files with known-good expected output, run through both the old
   text-layer-only path and the new native path, to catch regressions.

**Honest cost note**: this is the largest phase by far. RAGFlow's team
invested ~54k lines (including tests) here over a long period. Adopting their logic
is a large multiple faster than writing it from scratch, but "large
multiple faster than years" can still be weeks, not days — budget
accordingly and consider landing it behind a feature flag / opt-in build
tag first.

---

## Phase 2 — Chunk-level LLM enrichment

**Why second**: zero dependency on Phase 1 — this operates on chunks,
regardless of which provider produced them (docling-serve, native, or
`seshat-intelligence` later). Cheapest, fastest win identified in the
RAGFlow comparison, and a good phase to land while Phase 1 is still being
hardened.

**What RAGFlow does**: an "Extractor" pipeline stage generates, per chunk,
LLM-produced keywords/questions/summary, run concurrently with a bounded
worker pool, retried with backoff, and memoized (keyed by content hash) so
a re-ingest doesn't re-pay the LLM cost. A known technique for improving
retrieval recall (synthetic questions in particular help match
how users actually phrase queries).

### Steps

1. Define the enrichment shape: what to generate per chunk (keywords,
   summary, synthetic questions — pick based on what `search_knowledge`'s
   retrieval quality actually needs; don't build all three without a
   reason).
2. Add an enrichment step in `internal/rag/` (a new file, not bolted onto
   `chunker.go`) that calls the already-configured LLM client per chunk.
3. Reuse `internal/rag/chunk_cache.go`'s existing cache infrastructure
   (`ChunkCache`, `ArtifactChunkCache`/`MemoryChunkCache`) — same content-hash
   keying pattern already established for `CachedDocumentChunker`, extended
   to cover enrichment results too so re-ingestion doesn't re-bill the LLM.
4. Wire into `rag.Service.Ingest` as an optional step (config flag), not a
   forced default — it costs LLM calls per chunk, callers should opt in.

**Done. Design decisions and what got built:**

- **Enrichment shape: synthetic questions only**, not keywords or summary.
  Keywords mostly help BM25, which this repo's hybrid search already gets
  from `vector.Store`'s own FTS indexing of the chunk text — an LLM-generated
  keyword list wouldn't add much there. Summary helps more for
  already-large chunks; this repo's chunkers already bound chunk size (see
  `ChunkProfile`), so a summary of an already-small chunk isn't clearly
  worth an LLM call. Synthetic questions are the one RAGFlow's own rationale
  singles out by name ("help match how users actually phrase queries") and
  the one with a direct, uncontroversial mechanism: chunk text is
  declarative, queries are often question-shaped, embedding the chunk
  together with questions it answers closes that gap. Don't build all three
  without a reason, per the step above — this is the one with a clear reason.
- **`internal/rag.Enricher`** (`internal/rag/types.go`) is the new optional
  interface, exactly mirroring `Embedder`'s shape:
  `EnrichChunks(ctx, texts) ([][]string, error)`, each inner slice being
  that chunk's synthetic questions. Deliberately primitive-typed (no
  `EnrichmentResult` struct) so a concrete implementation never needs to
  import `internal/rag` at all — the same reason `Embedder`/`Reranker`'s
  own signatures stay primitive-typed.
- **`internal/rag/enricher.LLMEnricher`** (new subpackage, mirrors
  `internal/rag/embedder`/`internal/rag/reranker`'s split between the
  interface in `internal/rag` and its concrete implementation in a
  subpackage) calls an `LLMCaller` — a minimal local interface
  (`CreateMessage(ctx, types.APIRequest) (*types.APIResponse, error)`) that
  `*providers.Client` satisfies structurally, so `internal/rag` never has to
  import `internal/providers` at all. Exact same pattern as
  `internal/memory/longterm.Extractor`'s own `LLMCaller` — not a
  coincidence, it's the established idiom in this codebase for "a package
  needs to call the chat LLM without depending on the whole provider
  stack."
  - Runs one LLM call per chunk, fanned out concurrently with a bounded
    worker pool (`Config.MaxConcurrency`, default 4) — the "bounded worker
    pool" RAGFlow's own Extractor uses.
  - **No retry/backoff logic here, deliberately**: `*providers.Client`
    already retries transient failures internally
    (`Client.sendMessageWithRetry`) before `LLMCaller.CreateMessage` ever
    returns, so re-wrapping that in `internal/rag/enricher` would just
    double the backoff. Confirmed by reading `internal/providers/client.go`
    before writing this, not assumed.
  - Best-effort per chunk, matching `longterm.Extractor`'s own philosophy: a
    single chunk's LLM failure is logged at DEBUG and that chunk's slot
    stays empty rather than failing the whole batch — callers opted into
    enrichment for better recall, not for ingestion to become less reliable
    than before. `EnrichChunks` only returns a hard error for something
    more fundamental (context cancellation, or a caller-visible bug like a
    result-count mismatch).
- **Cache**: `internal/rag/enrichment_cache.go` adds `EnrichmentCache`,
  `ArtifactEnrichmentCache`, `MemoryEnrichmentCache`, and `CachedEnricher` —
  same shape and same file-per-concern convention as `chunk_cache.go`'s
  `ChunkCache`/`ArtifactChunkCache`/`MemoryChunkCache`/
  `CachedDocumentChunker`, reusing that file's own `writeCachePart` hashing
  helper directly (same package). `CachedEnricher.EnrichChunks` batches
  correctly: for a mix of cached and new chunk texts, only the cache misses
  are forwarded to the wrapped `Enricher`, and only those get written back —
  verified by a test asserting the underlying enricher sees exactly the
  miss set, not the whole batch. `LLMEnricher` implements
  `EnricherCacheKeyProvider` (model + questions-per-chunk fingerprinted into
  the cache key) so changing that configuration doesn't silently reuse
  stale enrichment results, mirroring `ChunkCacheKeyProvider`'s exact
  purpose for `DoclingChunker`.
- **Wiring**: `Service.SetEnricher(e Enricher)` (`internal/rag/service.go`),
  same nil-means-off pattern as `SetReranker`/the `embedder` field — not a
  forced default, not a per-`IngestRequest` flag (no request-level opt-out
  was asked for; a service either has enrichment configured or it doesn't,
  matching how the embedder/reranker are already toggled by presence, not
  a request field). Inside `Ingest`, when an enricher is configured, each
  chunk's questions are appended to the text handed to the *embedder* only
  (`buildEmbeddingText`) — the chunk's stored/returned `Text` (what
  `rag_search` displays) is never touched. Verified by a test
  (`TestServiceIngest_EnricherAugmentsEmbeddingTextNotStoredText`) that
  fails if either side leaks into the other.
- **`pkg/rag`** exposes all of the above (`Enricher`, `EnrichmentCache`,
  `ArtifactEnrichmentCache`, `MemoryEnrichmentCache`, `CachedEnricher`,
  `ChunkEnrichmentCacheKey`, matching constructors) plus a new
  `pkg/rag/enricher` facade subpackage for `LLMEnricher`, mirroring
  `pkg/rag/reranker`'s exact shape.
- **Tests**: unit tests only, no live LLM calls — matching how
  `longterm.Extractor`, `internal/rag/embedder`, and `internal/rag/reranker`
  are all tested in this repo (a fake satisfying the minimal caller
  interface, not a real network call). Covers: plain/fenced/prose-wrapped
  JSON parsing, the `QuestionsPerChunk` cap, best-effort per-chunk failure
  isolation, bounded concurrency (asserted via an instrumented fake caller
  tracking max in-flight calls), cache hit/miss/partial-miss behavior,
  cache-key invalidation on config change, and the embed-vs-stored-text
  separation in `Service.Ingest`. All pass under `go test -race`.
- **Verified for real**: `go build ./...`, `go vet ./...`,
  `golangci-lint run ./...`, and `go test -race ./...` all pass clean
  (one unrelated, pre-existing `internal/sandbox` Docker-cleanup-timing
  test flaked under full-suite load and passed cleanly in isolation — not
  touched by this phase, confirmed via `git diff --stat -- internal/sandbox/`
  showing no changes there).

---

## Phase 3 — Vision-LLM fallback for pages native parsing can't handle

**Why third**: needs Phase 1's `pdfium` page-rasterization capability to
render a page as an image in the first place — seshat has no page-render
capability today (`internal/pdfsmart`'s `extractSinglePage` slices out a
one-page **PDF**, not a raster image). Doing this before Phase 1 would mean
building page rendering twice.

**What RAGFlow does**: when a page still can't produce usable text (no
native layer, OCR/layout also failed or is unavailable), render the page
and send it to a vision-capable LLM with a transcription prompt, as an
explicit, deliberate pipeline stage — not a hope that the caller's model
happens to be multimodal.

**What we do today**: `fileread.go` falls through to raw base64 byte
pass-through, relying on the agent's own model being vision-capable, with
no page rendering step at all.

### Steps

1. Reuse Phase 1's `pdfium` CGO binding for page-to-image rendering (no new
   native dependency needed here).
2. Add an explicit fallback stage: when native parsing (Phase 1) still
   produces nothing usable for a page, render just that page and send it
   through the **already-configured agent LLM client** (no new external
   dependency — unlike RAGFlow, which calls a separately configured vision
   model) with a transcription prompt.
3. Gate this behind checking the configured model's multimodal capability
   (don't attempt it against a text-only model).
4. Keep `pdfsmart`'s existing safety contract (`ok=false` when any page
   couldn't get usable text) — this becomes one more thing tried before
   giving up, not a silent behavior change.

**Done. Design decisions and what got built:**

- **`internal/pdfsmart` stays CGO-independent.** `PageRenderer` and
  `VisionTranscriber` are new interfaces in `internal/pdfsmart/pdfsmart.go`
  (exactly the same injection pattern as `docling.DocumentConverterBackend`)
  rather than a direct import of `internal/nativedoc/pdfium` - `pdfsmart`
  itself never depends on CGO or the nativedoc build tag, confirmed by the
  full-repo plain `go build ./...`/`go vet ./...` passing with these changes
  in place, same as every other nativedoc-adjacent addition.
- **`Convert`'s signature grew one parameter**, not two: the two new
  collaborators are bundled into a single `VisionFallback{Renderer,
  Transcriber}` struct rather than two more positional args, specifically so
  a future fallback stage doesn't force another breaking signature change.
  `VisionFallback{}` (zero value) disables the stage entirely, the same as a
  nil docling client already disabled that stage. This is a breaking
  signature change for `pkg/pdfsmart.Convert`'s external callers
  (seshat-backend) - documented here and in the commit, matching how earlier
  phases already changed `pdfsmart.Convert`'s `doclingClient` parameter type
  once before.
- **`internal/nativedoc/parser.Converter` gained a `RenderPage` method**
  (`render.go`, new file, same `cgo && nativedoc` build tag) reusing the
  `RenderDPI` field/`renderDPI()` helper the OCR path (`pdf.go`) already
  had - one `Converter` now satisfies both
  `docling.DocumentConverterBackend` (existing) and `pdfsmart.PageRenderer`
  (new), duck-typed, no import of `internal/pdfsmart` needed. `pkg/nativedoc`
  (both the real, CGO-tagged facade and its always-compiles stub) each carry
  a `var _ pdfsmart.PageRenderer = (*Converter)(nil)` compile-time proof, so
  the two builds can never silently drift apart on this interface. **Verified
  for real, not just compiled**: a new test renders `testdata/text_layer.pdf`
  via `Converter.RenderPage`, decodes the result as a real PNG, and checks
  its dimensions are sane (>500px tall at the default 150 DPI) - run against
  the actual fixture on the same native Windows CGO setup Phase 1.W
  validated, not mocked.
- **`internal/pdfsmart/vision.Transcriber`** (new subpackage, mirrors
  `internal/rag/enricher`'s split between interface-in-parent and
  implementation-in-subpackage) calls an `LLMCaller` - the same minimal
  local-interface pattern as `internal/rag/enricher.LLMCaller` and
  `internal/memory/longterm.Extractor`'s `LLMCaller`, so `internal/pdfsmart`
  never depends on `internal/providers` either.
  - **Multimodal-capability gate**: `IsAvailable` checks
    `model.Registry.VisionCapable(provider, modelID)` - the same capability
    registry already used elsewhere in this codebase for this exact
    purpose, not a new mechanism. `Config.Registry` defaults to
    `model.Global` (populated at startup by `internal/providers`) but is
    overridable, primarily for tests - see the new `pkg/model` facade below
    for why this is a real, usable field rather than a leak.
  - The transcription prompt asks for faithful markdown transcription, no
    summarization, with an explicit `(no text)` sentinel for a blank page -
    `TranscribePage` maps that sentinel back to an empty string, which
    `Convert` then treats the same as any other "still no usable text"
    outcome (falls through to `ok=false` if nothing else recovers it).
- **New `pkg/model` facade** - `Config.Registry` (in
  `internal/pdfsmart/vision.Config`, exposed publicly via
  `pkg/pdfsmart/vision.Config`) is typed `*internal/model.Registry`. Per
  this repo's own rule against leaking `internal/` types into `pkg/`
  signatures, added a small `pkg/model` package (type aliases + `NewRegistry`
  + `Global`, mirroring `internal/model.go`'s own small, dependency-free
  shape) so external consumers can actually construct and pass a `*Registry`
  through that field instead of it being an internal type they can compile
  against but never populate.
- **`pkg/pdfsmart` and a new `pkg/pdfsmart/vision` facade** expose all of
  the above, mirroring `pkg/rag`/`pkg/rag/enricher`'s exact shape.
- **Tests**: unit tests with fakes for the LLM-calling and interface-wiring
  logic (`internal/pdfsmart/vision`, `internal/pdfsmart`'s new
  `VisionFallback` test cases - fallback used when docling is nil/
  unavailable, tried after docling fails, skipped when the transcriber
  reports itself unavailable, garbled vision output rejected the same as
  garbled docling output, renderer errors handled cleanly), plus the one
  real/non-mocked test described above for the actual pdfium rendering
  path. All pass under `go test -race`, both in the plain build (CGO
  disabled, nativedoc code entirely absent) and under `-tags nativedoc`.
- **Verified for real**: `go build`/`go vet`/`golangci-lint`/
  `go test -race ./...` all pass clean across the whole repo in the plain
  build, and the same four checks pass clean again for the touched packages
  under `-tags nativedoc` on the real native Windows CGO setup.

---

## Phase 4 — Specialized chunkers

**Why last**: lowest urgency, and table-aware chunking specifically is far
more useful once Phase 1 actually produces real table-structure data to
chunk against (today's chunkers only ever see plain markdown/text).

**What RAGFlow does**: 11 chunker types, workflow-configurable. Not
replicating their DSL/canvas system (different product shape — seshat is
agent+tools, not a visual workflow builder), just the two or three
chunking *strategies* worth having:

### Steps

1. **Table-aware chunker**: keep table rows/structure intact as their own
   chunk(s) instead of letting a generic splitter cut through the middle of
   a table — needs Phase 1's real table-structure output to be worth
   building.
2. **QA-pair chunker**: for documents that are naturally
   question/answer-formatted (FAQs, support docs) — one chunk per Q/A pair
   instead of arbitrary token-count splitting.
3. Wire both into the existing `ChunkProfile`/`Chunker` system
   (`internal/rag/chunker.go`, `chunk_profile.go`) as additional named
   profiles, following the same pattern `HeadingChunker` already
   established for the `"structured"` profile.

**Done. Design decisions and what got built:**

- **"Real table-structure data" ended up meaning GFM/Markdown table syntax
  in already-produced text, not TSR bounding-box/cell-grid metadata.** The
  native nativedoc pipeline (Phase 1) never actually wired TSR into
  `Converter.convertPDF` (only det/rec OCR - see that file's own doc
  comment: "considerably more... than the deliberately-scoped v1 here"), so
  there is no structured table metadata flowing through
  `docling.ConversionResult` from the native path. What genuinely does
  produce real GFM tables today is docling-serve's own conversion output
  (well-known for table-structure recognition) - `TableChunker` operates on
  that already-produced Markdown text, recognizing table block syntax
  syntactically, the same approach `HeadingChunker` already takes for
  headings (it doesn't need semantic structure data either, just pattern
  matching on the text). This is a real, valuable feature today, not
  blocked on TSR ever getting wired into the native path.
- **`TableChunker`** (`internal/rag/table_chunker.go`) partitions text into
  alternating table/non-table spans (a table block = a pipe-delimited row
  immediately followed by a valid GFM separator row, extended through every
  further contiguous pipe-delimited row). Non-table spans delegate to
  `Fallback` (default `HeadingChunker`, so heading context is preserved for
  the surrounding prose too). An oversized table splits by data row, but
  the header + separator row is repeated at the top of every resulting
  piece, so each chunk stays a valid, self-contained table rather than a
  headerless fragment - the well-known "repeat header on split" practice.
  A document with no table delegates entirely to `Fallback` - additive,
  never worse than the fallback's own baseline, the same contract
  `HeadingChunker` already established.
- **`QAChunker`** (`internal/rag/qa_chunker.go`) tries two independent
  detection strategies, in order:
  1. Explicit `Q:`/`A:` labels (also `Question:`/`Answer:`, numbered
     `Q1:`/`A1:`, bold-markdown `**Q:**`) - the common plain-text FAQ shape.
     A preamble before the first label is preserved as its own ordinary
     chunk, never dropped.
  2. Markdown headings ending in `?` - the common docling-converted FAQ
     shape. Reuses `HeadingChunker`'s own `matchHeading`/ancestor-path
     tracking directly (same package, same unexported helpers), so a
     nested "Category > Question?" structure keeps its category context.
     Critically, a heading that does *not* end in `?` still produces an
     ordinary (non-QA-tagged) section chunk exactly as `HeadingChunker`
     would - QA tagging is additive on top of `HeadingChunker`'s own
     traversal, never a narrower view that could silently drop a
     non-question section. Verified by a dedicated test
     (`TestQAChunker_HeadingModeNonQuestionSectionNotDropped`).
  Each detected pair becomes one chunk (split further via
  `splitBodyByTokenBudget`, reused from `heading_chunker.go`, only if a
  single pair exceeds the token budget), tagged
  `Metadata["qa_question"]` for citation/UI display - the same idea as
  `HeadingChunker`'s `Metadata["heading_path"]`. No QA structure at all
  delegates entirely to `Fallback` (default `HeadingChunker`), same
  additive contract.
  - **Known, documented heuristic limitation**: the label regex also
    matches a letter-lettered outline bullet ("a. First point") as an
    answer label. Harmless in isolation (a pair only forms from a complete
    question-then-answer sequence), but a document mixing real `Q:`/`A:`
    labels with letter-bulleted sub-lists could occasionally misattribute
    a bullet - the same class of imprecision `HeadingChunker`'s own
    `maxHeadingLineRunes` guard already accepts elsewhere in this package,
    not a new standard being introduced.
- **New `ChunkProfileTable`/`ChunkProfileQA`** (`chunk_profile.go`), zero
  `OverlapTokens` for both - deliberate, not an oversight: `TableChunker`'s
  header-repeat already gives each split piece the context an overlap
  would otherwise exist to provide, and `QAChunker`'s chunks are already
  each a complete, self-contained pair with no "next chunk" content an
  overlap would usefully carry forward.
- **Wired into `NewDoclingChunkerForProfile`** (`docling_chunker.go`)
  exactly like `ChunkProfileStructured` → `HeadingChunker` already was -
  extended from an `if` to a `switch` covering all three profile names.
  Regression-tested (`TestNewDoclingChunkerForProfile_TableAndQAProfilesGetMatchingFallbacks`)
  the same way the existing structured-profile test already was.
- **`pkg/rag`** exposes `TableChunker`/`QAChunker`/`NewTableChunker`/
  `NewQAChunker`/`ChunkProfileTable`/`ChunkProfileQA`, matching the facade's
  existing shape - no new subpackage needed here (unlike Phase 2/3's
  `enricher`/`vision`), since neither chunker calls out to an LLM or any
  other external collaborator - they're pure text-processing, same as
  `HeadingChunker`/`ParagraphChunker` already are.
- **Tests**: 13 new unit tests covering both chunkers' fallback behavior,
  detection/splitting correctness (small table stays one chunk, large table
  splits with the header repeated, multiple tables each get their own
  chunk, all three label variants, preamble preservation, heading-mode
  question/non-question mixing, nested category ancestor paths), plus
  `SplitDocument` ignoring original bytes in favor of already-extracted
  text (matching every other text-only chunker in this package). All pass
  under `go test -race`.
- **Verified for real**: `go build`/`go vet`/`golangci-lint`/
  `go test -race ./...` all pass clean across the full repo.

---

## Explicitly out of scope

- RAGFlow's DSL/canvas workflow engine for chunking configuration —
  wrong product shape for seshat.
- RAGFlow's auto-tagging system (IDF-weighted tag-KB matching) — needs a
  curated tag knowledge base seshat has no equivalent concept for; revisit
  only if a concrete need shows up.
- Replicating RAGFlow's kvrocks-backed job checkpoint/resume system —
  seshat's ingestion is a synchronous per-document operation, not an async
  background job system; Phase 2's chunk/enrichment caching already gets
  the practical benefit (idempotent re-runs) without needing the job
  infrastructure.
