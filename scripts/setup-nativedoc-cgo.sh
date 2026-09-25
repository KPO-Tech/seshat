#!/usr/bin/env bash
# setup-nativedoc-cgo.sh — fetch the native libraries pkg/nativedoc needs
# (pdf_oxide, pdfium, onnxruntime) and print the exact CGO_CFLAGS/CGO_LDFLAGS
# (and, on Windows, PATH/env additions) to build and run with.
#
# nativedoc is opt-in (see pkg/nativedoc's own doc comment) - a plain
# `make build`/`go build ./...` never needs this. Only run it if you're
# building something that actually wires pkg/nativedoc in (via
# sdk.ClientConfig.DocumentConverter).
#
# Usage:
#   source <(./scripts/setup-nativedoc-cgo.sh)     # exports everything into your shell
#   go build -tags "$NATIVEDOC_BUILD_TAGS" ./your/package/...
#
# Or capture the exports into a file to reuse across shells:
#   ./scripts/setup-nativedoc-cgo.sh > .cache/nativedoc-cgo-env.sh
#   source .cache/nativedoc-cgo-env.sh
#
# $NATIVEDOC_BUILD_TAGS is exported by this script because it differs by
# platform: Linux/macOS use "static nativedoc" (`static` selects
# onnxruntime_go's dlopen(NULL)-based symbol resolution needed for the
# RAGFlow-patched static onnxruntime build - see
# internal/nativedoc/session.go's WeightSharingEnabled doc comment); Windows
# uses just "nativedoc" (no patched onnxruntime build is published for
# Windows, so this script uses the stock dynamic onnxruntime.dll instead -
# weight sharing stays off there, which is safe, just a bit more memory per
# worker - see the same doc comment).
#
# Platform support: Linux, macOS, and Windows (x86_64/arm64) are all
# covered end-to-end - confirmed by actually building and running
# internal/nativedoc/parser's real test suite on each.

set -euo pipefail

CACHE_DIR="${NATIVEDOC_CACHE_DIR:-$HOME/.cache/seshat-nativedoc}"
PDF_OXIDE_VERSION="${PDF_OXIDE_VERSION:-v0.3.78}"
ONNXRUNTIME_VERSION="${ONNXRUNTIME_VERSION:-1.29.0}"
ONNXRUNTIME_PATCH_TAG="${ONNXRUNTIME_PATCH_TAG:-onnxruntime-v${ONNXRUNTIME_VERSION}-patch1}"

log() { echo "# $*" >&2; }
die() { echo "# ERROR: $*" >&2; exit 1; }

mkdir -p "$CACHE_DIR"

# ── Platform detection ────────────────────────────────────────────────────────
os="$(uname -s)"
arch="$(uname -m)"
is_windows=false

case "$os" in
    Linux)  pdfium_plat="linux" ;;
    Darwin) pdfium_plat="mac" ;;
    MINGW*|MSYS*|CYGWIN*) pdfium_plat="win"; is_windows=true ;;
    *) die "unsupported OS for this script: $os" ;;
esac
case "$arch" in
    x86_64|amd64) pdfium_arch="x64"; ort_arch="x86_64" ;;
    arm64|aarch64) pdfium_arch="arm64"; ort_arch="aarch64" ;;
    *) die "unsupported architecture: $arch" ;;
esac

if $is_windows && ! command -v gcc &>/dev/null; then
    die "no C compiler on PATH - CGO needs one even on Windows (MSYS2 + 'pacman -S mingw-w64-x86_64-gcc' is one way to get it, then add its bin/ to PATH)"
fi

# ── 1. pdf_oxide - has its own installer, which prints the CGO_* exports itself ──
log "Installing pdf_oxide native lib (${PDF_OXIDE_VERSION})..."
PDF_OXIDE_OUT="$(cd "$(mktemp -d)" && go run "github.com/yfedoseev/pdf_oxide/go/cmd/install@${PDF_OXIDE_VERSION}" 2>&1)" \
    || die "pdf_oxide installer failed:\n$PDF_OXIDE_OUT"
PDF_OXIDE_CFLAGS="$(echo "$PDF_OXIDE_OUT" | grep -m1 -i '^\(export \|set \)\?CGO_CFLAGS=' | sed -E 's/^(export |set )?CGO_CFLAGS=//' | tr -d '"')"
PDF_OXIDE_LDFLAGS="$(echo "$PDF_OXIDE_OUT" | grep -m1 -i '^\(export \|set \)\?CGO_LDFLAGS=' | sed -E 's/^(export |set )?CGO_LDFLAGS=//' | tr -d '"')"
[ -n "$PDF_OXIDE_CFLAGS" ] && [ -n "$PDF_OXIDE_LDFLAGS" ] \
    || die "could not parse pdf_oxide installer output:\n$PDF_OXIDE_OUT"
if $is_windows; then
    # cgo's own CGO_CFLAGS/CGO_LDFLAGS tokenizer treats backslash as a
    # shell-style escape character regardless of host OS, so Windows
    # backslash paths (what pdf_oxide's installer prints, e.g.
    # "C:\Users\...\include") get silently corrupted when go build/test
    # splits the flag string - confirmed hitting this on real Windows
    # (paths came out with chunks eaten, e.g. "...\A" swallowed). Forward
    # slashes are accepted by the Windows API for paths just as well and
    # don't trigger this, so normalize before using these anywhere.
    PDF_OXIDE_CFLAGS="$(echo "$PDF_OXIDE_CFLAGS" | tr '\\' '/')"
    PDF_OXIDE_LDFLAGS="$(echo "$PDF_OXIDE_LDFLAGS" | tr '\\' '/')"
fi
log "pdf_oxide OK."

# ── 2. pdfium - prebuilt lib from bblanchon/pdfium-binaries ──────────────────────
PDFIUM_DIR="$CACHE_DIR/pdfium"
PDFIUM_ASSET="pdfium-${pdfium_plat}-${pdfium_arch}.tgz"
pdfium_present() {
    [ -f "$PDFIUM_DIR/lib/libpdfium.so" ] || [ -f "$PDFIUM_DIR/lib/libpdfium.dylib" ] \
        || [ -f "$PDFIUM_DIR/lib/pdfium.dll.lib" ]
}
if ! pdfium_present; then
    log "Downloading pdfium ($PDFIUM_ASSET)..."
    mkdir -p "$PDFIUM_DIR"
    curl -fSL "https://github.com/bblanchon/pdfium-binaries/releases/latest/download/$PDFIUM_ASSET" \
        -o "$PDFIUM_DIR/pdfium.tgz" \
        || die "pdfium download failed for $PDFIUM_ASSET"
    # --force-local: without it, GNU tar treats any "C:/..." path (a plain
    # Windows drive-letter path) as "connect to host C" remote-archive
    # syntax, since a colon before the first slash is its remote-shell
    # trigger - confirmed hitting this while validating on real Windows.
    tar --force-local -xzf "$PDFIUM_DIR/pdfium.tgz" -C "$PDFIUM_DIR"
fi
log "pdfium OK."

if $is_windows; then
    # ── 3 (Windows). Stock Microsoft onnxruntime - dynamically loaded at
    # runtime (onnxruntime_go's Windows path uses LoadLibrary, not a
    # build-time link), so there's nothing to add to CGO_LDFLAGS for it.
    # No RAGFlow-patched build exists for Windows (only linux/macOS assets
    # are published), so weight sharing stays off here - see
    # internal/nativedoc/session.go's WeightSharingEnabled doc comment;
    # this is a safe, deliberate tradeoff (a bit more memory per worker),
    # not a missing feature.
    ORT_DIR="$CACHE_DIR/onnxruntime-win"
    ORT_ASSET="onnxruntime-win-${pdfium_arch}-${ONNXRUNTIME_VERSION}.zip"
    if [ ! -f "$ORT_DIR/onnxruntime.dll" ]; then
        log "Downloading onnxruntime ($ORT_ASSET)..."
        mkdir -p "$ORT_DIR"
        curl -fSL "https://github.com/microsoft/onnxruntime/releases/download/v${ONNXRUNTIME_VERSION}/${ORT_ASSET}" \
            -o "$ORT_DIR/ort.zip" \
            || die "onnxruntime download failed for $ORT_ASSET"
        (cd "$ORT_DIR" && unzip -oq ort.zip)
        found="$(find "$ORT_DIR" -name 'onnxruntime.dll' | head -1)"
        [ -n "$found" ] || die "onnxruntime.dll not found after extracting $ORT_ASSET"
        cp "$found" "$ORT_DIR/onnxruntime.dll"
    fi
    log "onnxruntime OK."

    # ── Emit Windows exports ──────────────────────────────────────────────────
    # pdfium.dll is a hard (import-table) link-time dependency of
    # internal/nativedoc/pdfium, so it must be discoverable via PATH at
    # process start, not just at build time - hence PATH, not just CGO_LDFLAGS.
    #
    # SESHAT_NATIVEDOC_ONNXRUNTIME_PATH is NOT optional on Windows in
    # practice: LoadLibrary's search order checks the exe's own directory
    # and the system directory before PATH, so if anything else on this
    # machine already has an onnxruntime.dll reachable there (a stray copy
    # from an unrelated ML/Python install, for example - this is a real
    # thing that happened while validating this script, not a hypothetical),
    # the wrong one silently wins without this. See
    # internal/nativedoc/session.go's EnvSharedLibraryPath doc comment.
    # PATH specifically needs POSIX-style entries (/c/... not C:/...),
    # even though CGO_CFLAGS/CGO_LDFLAGS want the Windows-style forward-slash
    # form used above. Reason: this script runs under a bash on Windows
    # (git-bash/MSYS), which auto-translates $PATH into a real Windows PATH
    # when spawning a native child process (go.exe, etc.) - but that
    # translator splits on ':' the same way it does for the *outer* POSIX
    # path list, so a Windows-style "C:/Users/..." entry's own drive-letter
    # colon collides with it and comes out corrupted (confirmed: it produced
    # a bogus lone "C" entry, and mangled the rest by prefixing Git's own
    # install path onto it). cygpath -u avoids the whole ambiguity.
    PDFIUM_BIN_POSIX="$(cygpath -u "$PDFIUM_DIR/bin" 2>/dev/null || echo "$PDFIUM_DIR/bin")"
    echo "export NATIVEDOC_BUILD_TAGS=\"nativedoc\""
    echo "export CGO_CFLAGS=\"$PDF_OXIDE_CFLAGS -I$PDFIUM_DIR/include\""
    echo "export CGO_LDFLAGS=\"$PDF_OXIDE_LDFLAGS $PDFIUM_DIR/lib/pdfium.dll.lib\""
    echo "export PATH=\"$PDFIUM_BIN_POSIX:\$PATH\""
    echo "export SESHAT_NATIVEDOC_ONNXRUNTIME_PATH=\"$ORT_DIR/onnxruntime.dll\""
    log 'Done. Build with: go build -tags "$NATIVEDOC_BUILD_TAGS" ./your/package/...'
    log 'Copy $PDFIUM_DIR/bin/pdfium.dll next to any binary you ship (PATH covers `go run`/`go test`, not a built .exe moved elsewhere).'
    exit 0
fi

# ── 3 (Linux/macOS). RAGFlow-patched onnxruntime, statically linked ─────────────
ORT_DIR="$CACHE_DIR/onnxruntime"
ORT_ASSET="onnxruntime-v${ONNXRUNTIME_VERSION}-${pdfium_plat}-${ort_arch}.zip"
[ "$pdfium_plat" = "linux" ] && ORT_ASSET="onnxruntime-v${ONNXRUNTIME_VERSION}-linux-${ort_arch}.zip"
[ "$pdfium_plat" = "mac" ] && ORT_ASSET="onnxruntime-v${ONNXRUNTIME_VERSION}-osx-universal.zip"
if [ ! -f "$ORT_DIR/lib/libonnxruntime.a" ]; then
    log "Downloading infiniflow-patched onnxruntime ($ORT_ASSET)..."
    mkdir -p "$ORT_DIR"
    curl -fSL "https://github.com/infiniflow/ragflow-build/releases/download/${ONNXRUNTIME_PATCH_TAG}/${ORT_ASSET}" \
        -o "$ORT_DIR/ort.zip" \
        || die "onnxruntime download failed for $ORT_ASSET - if this is a new OS/arch, check https://github.com/infiniflow/ragflow-build/releases for the exact asset name"
    (cd "$ORT_DIR" && unzip -oq ort.zip)
    # The zip extracts to onnxruntime-v${VERSION}-${plat}-${arch}/lib/libonnxruntime.a -
    # flatten it so ORT_DIR/lib is stable regardless of the archive's inner folder name.
    found="$(find "$ORT_DIR" -name 'libonnxruntime.a' | head -1)"
    [ -n "$found" ] || die "libonnxruntime.a not found after extracting $ORT_ASSET"
    mkdir -p "$ORT_DIR/lib"
    cp "$found" "$ORT_DIR/lib/libonnxruntime.a"
fi
log "onnxruntime OK."

# ── Dynamic-list file (exports only OrtGetApiBase - see
# internal/nativedoc/session.go's own doc comment for why) ──────────────────────
DYNAMIC_LIST="$CACHE_DIR/ort_dynamic_list.txt"
printf '{\n  OrtGetApiBase;\n};\n' > "$DYNAMIC_LIST"

# ── Emit the combined CGO_CFLAGS/CGO_LDFLAGS ──────────────────────────────────
PDFIUM_LIB_FLAG="-lpdfium"
echo "export NATIVEDOC_BUILD_TAGS=\"static nativedoc\""
echo "export CGO_CFLAGS=\"$PDF_OXIDE_CFLAGS -I$PDFIUM_DIR/include\""
echo "export CGO_LDFLAGS=\"$PDF_OXIDE_LDFLAGS -L$PDFIUM_DIR/lib $PDFIUM_LIB_FLAG -Wl,--undefined=OrtGetApiBase -Wl,--dynamic-list=$DYNAMIC_LIST $ORT_DIR/lib/libonnxruntime.a -lstdc++\""
echo "export LD_LIBRARY_PATH=\"$PDFIUM_DIR/lib\${LD_LIBRARY_PATH:+:\$LD_LIBRARY_PATH}\""
log 'Done. Build with: go build -tags "$NATIVEDOC_BUILD_TAGS" ./your/package/...'
