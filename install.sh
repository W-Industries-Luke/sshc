#!/bin/sh
# Installs the latest sshc release for this machine:
#   curl -fsSL https://raw.githubusercontent.com/W-Industries-Luke/sshc/main/install.sh | sh
# Downloads the right binary, checks it against the release's SHA256SUMS, and
# runs "sshc --install", which copies it to ~/.local/bin and puts that on PATH.
set -eu

base=https://github.com/W-Industries-Luke/sshc/releases/latest/download

case $(uname -s) in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) echo "sshc: no prebuilt binary for $(uname -s); build from source instead" >&2; exit 1 ;;
esac
case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) echo "sshc: no prebuilt binary for $(uname -m); build from source instead" >&2; exit 1 ;;
esac
file=sshc-$os-$arch

if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -q -O "$2" "$1"; }
else
	echo "sshc: need curl or wget to download" >&2
	exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum <"$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 <"$1" | cut -d' ' -f1; }
else
	echo "sshc: need sha256sum or shasum to verify the download" >&2
	exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $file ..."
fetch "$base/$file" "$tmp/$file"
fetch "$base/SHA256SUMS" "$tmp/SHA256SUMS"
want=$(awk -v f="$file" '$2 == f { print $1 }' "$tmp/SHA256SUMS")
got=$(sha256 "$tmp/$file")
if [ -z "$want" ] || [ "$want" != "$got" ]; then
	echo "sshc: checksum mismatch for $file; not installing" >&2
	exit 1
fi

chmod +x "$tmp/$file"
# A script cannot change the shell that started it, so --install ends by
# naming the one command that brings this terminal up to date.
"$tmp/$file" --install
