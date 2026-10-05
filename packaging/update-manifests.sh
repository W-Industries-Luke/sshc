#!/bin/sh
# Points the Scoop bucket and the Homebrew tap at a published release.
# Usage: packaging/update-manifests.sh VERSION
# Needs the gh CLI, authenticated (GH_TOKEN) with write access to
# W-Industries-Luke/scoop-bucket and W-Industries-Luke/homebrew-tap.
set -eu
version=${1#v}
owner=W-Industries-Luke
rel=https://github.com/$owner/sshc/releases/download/v$version
who='W-Industries-Luke'
mail='172839406+W-Industries-Luke@users.noreply.github.com'

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -fsSL -o "$tmp/SHA256SUMS" "$rel/SHA256SUMS"
sum() {
	hash=$(awk -v f="$1" '$2 == f { print $1 }' "$tmp/SHA256SUMS")
	[ -n "$hash" ] || { echo "no checksum for $1 in release v$version" >&2; exit 1; }
	echo "$hash"
}

cat >"$tmp/sshc.json" <<JSON
{
    "version": "$version",
    "description": "ssh, scp, sftp, rsync and ssh-copy-id that answer the password or key passphrase prompt from a stored secret",
    "homepage": "https://github.com/$owner/sshc",
    "license": "MIT",
    "architecture": {
        "64bit": {
            "url": "$rel/sshc-windows-amd64.exe#/sshc.exe",
            "hash": "$(sum sshc-windows-amd64.exe)"
        },
        "arm64": {
            "url": "$rel/sshc-windows-arm64.exe#/sshc.exe",
            "hash": "$(sum sshc-windows-arm64.exe)"
        }
    },
    "bin": "sshc.exe",
    "checkver": "github",
    "autoupdate": {
        "architecture": {
            "64bit": {
                "url": "https://github.com/$owner/sshc/releases/download/v\$version/sshc-windows-amd64.exe#/sshc.exe"
            },
            "arm64": {
                "url": "https://github.com/$owner/sshc/releases/download/v\$version/sshc-windows-arm64.exe#/sshc.exe"
            }
        },
        "hash": {
            "url": "\$baseurl/SHA256SUMS"
        }
    }
}
JSON

cat >"$tmp/sshc.rb" <<RUBY
class Sshc < Formula
  desc "Answer ssh, scp, sftp and rsync password prompts from a stored secret"
  homepage "https://github.com/$owner/sshc"
  version "$version"
  license "MIT"

  on_macos do
    on_arm do
      url "$rel/sshc-darwin-arm64"
      sha256 "$(sum sshc-darwin-arm64)"
    end
    on_intel do
      url "$rel/sshc-darwin-amd64"
      sha256 "$(sum sshc-darwin-amd64)"
    end
  end

  on_linux do
    on_arm do
      url "$rel/sshc-linux-arm64"
      sha256 "$(sum sshc-linux-arm64)"
    end
    on_intel do
      url "$rel/sshc-linux-amd64"
      sha256 "$(sum sshc-linux-amd64)"
    end
  end

  def install
    bin.install Dir["sshc-*"].first => "sshc"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshc --version")
  end
end
RUBY

put() { # repo path file message
	sha=$(gh api "repos/$owner/$1/contents/$2" --jq .sha)
	gh api -X PUT "repos/$owner/$1/contents/$2" -f message="$4" -f sha="$sha" \
		-f content="$(base64 <"$3" | tr -d '\n')" \
		-f "author[name]=$who" -f "author[email]=$mail" \
		-f "committer[name]=$who" -f "committer[email]=$mail" \
		--jq '.content.path + " updated in " + .commit.sha[0:7]'
}
put scoop-bucket bucket/sshc.json "$tmp/sshc.json" "sshc: update to version $version"
put homebrew-tap Formula/sshc.rb "$tmp/sshc.rb" "sshc $version"
