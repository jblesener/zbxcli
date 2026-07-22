#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:?release version is required}"
DIST_DIR="dist"

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

package_target() {
  local goos="$1"
  local goarch="$2"
  local target="$3"
  local archive="$4"
  local binary="zbxcli"

  if [ "$goos" = "windows" ]; then
    binary="${binary}.exe"
  fi

  GOOS="$goos" GOARCH="$goarch" go build -trimpath -o "$DIST_DIR/$binary" .

  if [ "$archive" = "zip" ]; then
    (
      cd "$DIST_DIR"
      zip "zbxcli_${VERSION}_${target}.zip" "$binary"
    )
  else
    tar -C "$DIST_DIR" -czf "$DIST_DIR/zbxcli_${VERSION}_${target}.tar.gz" "$binary"
  fi

  rm -f "$DIST_DIR/$binary"
}

package_target linux amd64 linux-amd64 tar.gz
package_target darwin amd64 darwin-amd64 tar.gz
package_target windows amd64 windows-amd64 zip
