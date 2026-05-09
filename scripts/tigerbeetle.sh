#!/usr/bin/env bash
set -euo pipefail

VERSION="0.17.8"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.tigerbeetle"
BIN="$DIR/tigerbeetle"
DATA="$DIR/0_0.tigerbeetle"
ADDR="${TB_ADDRESS:-3000}"

mkdir -p "$DIR"

download() {
  if [ -x "$BIN" ] && "$BIN" version 2>/dev/null | grep -q "$VERSION"; then
    return
  fi
  case "$(uname -s)-$(uname -m)" in
    Darwin-*)        asset="tigerbeetle-universal-macos.zip" ;;
    Linux-arm64|Linux-aarch64) asset="tigerbeetle-aarch64-linux.zip" ;;
    Linux-x86_64)    asset="tigerbeetle-x86_64-linux.zip" ;;
    *) echo "unsupported platform: $(uname -s)-$(uname -m)" >&2; exit 1 ;;
  esac
  echo "downloading TigerBeetle $VERSION ($asset)..."
  curl -fsSL -o "$DIR/tb.zip" \
    "https://github.com/tigerbeetle/tigerbeetle/releases/download/$VERSION/$asset"
  (cd "$DIR" && unzip -o tb.zip >/dev/null && rm -f tb.zip)
}

format() {
  rm -f "$DATA"
  "$BIN" format --cluster=0 --replica=0 --replica-count=1 --development "$DATA"
}

download
case "${1:-start}" in
  format) format ;;
  start)
    [ -f "$DATA" ] || format
    exec "$BIN" start --addresses="$ADDR" --development "$DATA"
    ;;
  *) echo "usage: $0 [start|format]" >&2; exit 1 ;;
esac
