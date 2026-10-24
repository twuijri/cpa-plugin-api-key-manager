#!/bin/sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
target_arch=${1:-amd64}
go_bin=${GO_BIN:-go}

case "$target_arch" in
  amd64) zig_target=x86_64-linux-gnu.2.17 ;;
  arm64) zig_target=aarch64-linux-gnu.2.17 ;;
  *) echo "Usage: $0 [amd64|arm64]" >&2; exit 2 ;;
esac

if [ -n "${ZIG_BIN:-}" ]; then
  c_compiler="$ZIG_BIN cc -target $zig_target"
elif [ -n "${CC:-}" ]; then
  c_compiler=$CC
elif [ "$(uname -s)" = Linux ]; then
  c_compiler=gcc
else
  echo "Cross-compilation requires ZIG_BIN or a matching Linux C compiler in CC." >&2
  exit 2
fi

out_dir="$project_dir/build/linux_$target_arch"
mkdir -p "$out_dir" "$project_dir/build/cache/go" "$project_dir/build/cache/mod"

GOCACHE="${GOCACHE:-$project_dir/build/cache/go}" \
GOMODCACHE="${GOMODCACHE:-$project_dir/build/cache/mod}" \
GOOS=linux GOARCH="$target_arch" CGO_ENABLED=1 CC="$c_compiler" \
  "$go_bin" -C "$project_dir" build -trimpath -buildmode=c-shared \
  -o "$out_dir/gemini-desktop.so" ./cmd/plugin

archive_name="gemini-desktop_0.1.2-preview_linux_$target_arch.zip"
versioned_library="gemini-desktop-v0.1.2-preview.so"
cp "$out_dir/gemini-desktop.so" "$out_dir/$versioned_library"
if command -v zip >/dev/null 2>&1; then
  (cd "$out_dir" && zip -q -j "$archive_name" "$versioned_library")
else
  echo "zip is unavailable; shared library built, archive skipped." >&2
fi

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$out_dir" && sha256sum gemini-desktop.so > checksums.txt)
  if [ -f "$out_dir/$archive_name" ]; then
    (cd "$out_dir" && sha256sum "$archive_name" >> checksums.txt)
  fi
else
  (cd "$out_dir" && shasum -a 256 gemini-desktop.so > checksums.txt)
  if [ -f "$out_dir/$archive_name" ]; then
    (cd "$out_dir" && shasum -a 256 "$archive_name" >> checksums.txt)
  fi
fi

echo "Build output: $out_dir"
