#!/bin/sh
# Builds dist/sshc_<version>_<arch>.deb and .rpm from the Linux binaries
# already in dist/. Usage: packaging/build-packages.sh VERSION
# Needs nfpm on PATH, or Docker to run it from its image.
set -eu
version=$1
cd "$(dirname "$0")/.."

if command -v nfpm >/dev/null 2>&1; then
	nfpm() { command nfpm "$@"; }
else
	nfpm() { docker run --rm -u "$(id -u):$(id -g)" -v "$PWD":/src -w /src goreleaser/nfpm:latest "$@"; }
fi

for arch in amd64 arm64; do
	sed -e "s/@VERSION@/$version/" -e "s/@ARCH@/$arch/" packaging/nfpm.yaml >dist/nfpm-$arch.yaml
	for kind in deb rpm; do
		nfpm package --config dist/nfpm-$arch.yaml --packager $kind --target dist/
	done
	rm dist/nfpm-$arch.yaml
done
