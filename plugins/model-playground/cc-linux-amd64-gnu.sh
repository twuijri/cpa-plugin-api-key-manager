#!/bin/sh
exec "$(dirname "$0")/../zig-aarch64-macos-0.17.0/zig" cc -target x86_64-linux-gnu.2.17 "$@"
