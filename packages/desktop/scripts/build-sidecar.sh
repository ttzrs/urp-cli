#!/bin/bash
# Build urp sidecar binary for Tauri bundling
# Tauri expects: binaries/urp-{target_triple}

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DESKTOP_DIR="$(dirname "$SCRIPT_DIR")"
GO_DIR="$DESKTOP_DIR/../../go"
BINARIES_DIR="$DESKTOP_DIR/src-tauri/binaries"

# Detect target triple
detect_target() {
    local os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch=$(uname -m)

    case "$os" in
        linux)
            case "$arch" in
                x86_64)  echo "x86_64-unknown-linux-gnu" ;;
                aarch64) echo "aarch64-unknown-linux-gnu" ;;
                *)       echo "unknown-linux" ;;
            esac
            ;;
        darwin)
            case "$arch" in
                x86_64)  echo "x86_64-apple-darwin" ;;
                arm64)   echo "aarch64-apple-darwin" ;;
                *)       echo "unknown-darwin" ;;
            esac
            ;;
        mingw*|msys*|cygwin*)
            echo "x86_64-pc-windows-msvc"
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

TARGET="${1:-$(detect_target)}"
echo "Building urp for target: $TARGET"

# Create binaries directory
mkdir -p "$BINARIES_DIR"

# Build Go binary
cd "$GO_DIR"

case "$TARGET" in
    *linux*)
        GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$BINARIES_DIR/urp-$TARGET" ./cmd/urp
        ;;
    *darwin*)
        if [[ "$TARGET" == *aarch64* ]]; then
            GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o "$BINARIES_DIR/urp-$TARGET" ./cmd/urp
        else
            GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o "$BINARIES_DIR/urp-$TARGET" ./cmd/urp
        fi
        ;;
    *windows*)
        GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o "$BINARIES_DIR/urp-$TARGET.exe" ./cmd/urp
        ;;
    *)
        go build -ldflags="-s -w" -o "$BINARIES_DIR/urp-$TARGET" ./cmd/urp
        ;;
esac

echo "Built: $BINARIES_DIR/urp-$TARGET"
ls -lh "$BINARIES_DIR/"
