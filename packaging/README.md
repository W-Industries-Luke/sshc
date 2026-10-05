# Packaging

Where sshc is published, and what to update for a new release `X.Y.Z`.

| Channel | Lives in | Update for a release |
| ------- | -------- | -------------------- |
| GitHub release | this repo | automatic: `.github/workflows/release.yml` builds, checksums, attests and publishes when a `vX.Y.Z` tag is pushed |
| `.deb`, `.rpm` | built by `build-packages.sh` from `nfpm.yaml` | automatic, attached to the release |
| `install.sh`, `install.ps1`, `sshc update` | this repo | nothing: they fetch the latest release |
| GitHub Action | `action.yml` in this repo | nothing: it fetches the latest release; the version in the README example is the tag people pin |
| Scoop | [scoop-bucket](https://github.com/W-Industries-Luke/scoop-bucket) `bucket/sshc.json` | `update-manifests.sh X.Y.Z`; the release workflow runs it when the `PACKAGING_TOKEN` secret is set |
| Homebrew | [homebrew-tap](https://github.com/W-Industries-Luke/homebrew-tap) `Formula/sshc.rb` | the same script |
| winget | `winget/` here, submitted to [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs) | see below |
| AUR | `aur/PKGBUILD` here | not published yet; needs an AUR account - see the comment in the file |

## Cutting a release

1. Set `version` in `internal/sshc/cli.go`, the date and version in
   `docs/sshc.1`, `pkgver` in `aur/PKGBUILD`, and add a section to
   `CHANGELOG.md`. Commit and push; wait for CI.
2. Push the tag: `git tag vX.Y.Z && git push origin vX.Y.Z`. The release
   workflow refuses a tag that does not match the version in the source or
   has no changelog section.
3. If no `PACKAGING_TOKEN` secret is set, run
   `packaging/update-manifests.sh X.Y.Z`.

`PACKAGING_TOKEN` is a fine-grained personal access token limited to the
`scoop-bucket` and `homebrew-tap` repositories with "Contents: read and
write". It is optional; without it step 3 is done by hand.

## winget

`winget/` holds the manifest for the current version. It is not in Microsoft's
package list until a pull request adding it has been accepted there.

On Windows, validate and try it, then submit it:

```powershell
winget validate --manifest packaging\winget
winget install --manifest packaging\winget        # needs: winget settings --enable LocalManifestFiles
winget install wingetcreate
wingetcreate submit packaging\winget
```

`wingetcreate submit` forks microsoft/winget-pkgs under your account and opens
the pull request. Microsoft's bot then asks you to accept their Contributor
License Agreement in a comment, and the package is published after their
automated validation and a moderator's review.

For later versions, `wingetcreate update LukeWeaver.sshc --version X.Y.Z --urls <amd64 url> <arm64 url> --submit`
does all of it in one step.
