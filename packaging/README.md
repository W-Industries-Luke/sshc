# Packaging

Where sshc is published, and what to update for a new release `X.Y.Z`.

| Channel | Lives in | Update for a release |
| ------- | -------- | -------------------- |
| GitHub release | this repo | tag `vX.Y.Z`, upload the `make dist` binaries and `SHA256SUMS` |
| `install.sh`, `install.ps1` | this repo | nothing: they fetch the latest release |
| Scoop | [scoop-bucket](https://github.com/W-Industries-Luke/scoop-bucket) `bucket/sshc.json` | `version`, the two URLs and hashes |
| Homebrew | [homebrew-tap](https://github.com/W-Industries-Luke/homebrew-tap) `Formula/sshc.rb` | `version`, the four URLs and hashes |
| winget | `winget/` here, submitted to [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs) | see below |

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
