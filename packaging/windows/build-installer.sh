#!/usr/bin/env bash
# Builds the Windows installer: dist/release/raikiri-windows-amd64-setup.exe
#
#   packaging/windows/build-installer.sh v4.0.0
#
# Expects the tray build at dist/windows/raikiri.exe (see `mise run release`). Uses a local
# makensis when available, otherwise Debian's nsis package in a throwaway container.
set -euo pipefail

tag=${1:?usage: build-installer.sh <version tag, e.g. v4.0.0>}
version=${tag#v}
if [[ ! $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	echo "installer needs a release version like v4.0.0, got $tag" >&2
	exit 1
fi

root=$(cd "$(dirname "$0")/../.." && pwd)
stage=dist/installer
out=dist/release/raikiri-windows-amd64-setup.exe
cd "$root"

rm -rf "$stage"
mkdir -p "$stage" dist/release
cp dist/windows/raikiri.exe "$stage/raikiri.exe"
cp cmd/raikiri/raikiri.ico "$stage/raikiri.ico"
cp LICENSE "$stage/LICENSE.txt"
cp internal/tiktoklive/NOTICE "$stage/THIRD_PARTY_NOTICES.txt"

args=(-V2 -DVERSION="$version" -DSTAGE="$root/$stage" -DOUTFILE="$root/$out" packaging/windows/raikiri.nsi)
if command -v makensis >/dev/null; then
	makensis "${args[@]}"
else
	docker run --rm -v "$root:$root" -w "$root" debian:bookworm-slim sh -c "
		apt-get update -qq && apt-get install -y -qq nsis >/dev/null &&
		makensis ${args[*]@Q} &&
		chown $(id -u):$(id -g) '$out'"
fi
echo "built $out"
