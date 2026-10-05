# Security

sshc handles passwords and key passphrases, so problems in it matter. This
page says how to report one and what sshc does and does not protect against.

## Reporting a vulnerability

Please report it privately: on the repository's **Security** tab, choose
**Report a vulnerability**. Do not open a public issue for something that
could put users' secrets at risk.

Include the sshc version (`sshc -v`), your operating system and OpenSSH
version (`ssh -V`), and the steps that show the problem. You should get a
first answer within a week. Fixes are released as a new version, and the
report is credited unless you would rather it was not.

Only the latest release is supported.

## What sshc protects against

- **A server asking for another host's secret.** sshc answers a password
  prompt only for the host that the local ssh client names in it. That name
  is written by the client, not the server. The active password is offered
  only to a host named on the command line, never to a jump host or to
  whatever else asks.
- **A server asking for a key passphrase or anything unexpected.** Only two
  prompt shapes written by the local client are answered: a login password
  and the passphrase of a local key file. A one-time code is supplied only to
  a host you configured one for. Host key confirmations, password changes and
  every other prompt go to the terminal.
- **Account lockout from a stale secret.** A stored secret is offered once
  per connection. If it is rejected, sshc says so and asks you instead.
- **Secrets on command lines or in output.** sshc passes secrets to ssh
  through the askpass channel, never as arguments, and no command prints a
  stored value.
- **Other users and offline access**, when a secret is saved to the
  credential store: Windows Credential Manager, the macOS Keychain and the
  Secret Service keyring keep it encrypted and tied to your login.
- **A config file others can read or write.** On Linux and macOS sshc
  refuses a config file that is not owned by you with mode 600.
- **Command injection through host names.** A configured command is run
  directly, never by a shell, and `%h`, `%u` and `%k` are substituted as
  whole arguments.
- **Tampered downloads.** Release files are built by GitHub Actions from the
  tagged commit, listed in `SHA256SUMS`, and carry a build provenance
  attestation. The install scripts, `sshc update` and the GitHub Action check
  the checksum before running anything. To check the attestation yourself:
  `gh attestation verify <file> --repo W-Industries-Luke/sshc`.

## What sshc does not protect against

- **Programs running as you.** sshc supplies secrets without asking, so
  other software under your account can obtain them the same way: by reading
  an environment variable, asking the credential store, or running sshc.
  Nothing that automates a password can prevent this.
- **An administrator or root** on your machine.
- **Plain-text storage you chose.** `sshc set --plain`, a secret typed into
  the config file, or the fallback where no credential store exists, is
  readable by anything that can read your files. A key passphrase stored that
  way protects the key about as well as no passphrase.
- **A password manager command that someone else can change.** A
  `password_command` runs with your permissions. On Windows, where sshc cannot
  check the config file's permissions, keep the file under your user profile.
- **The server itself.** A password you send to a host is known to that host.
- **Unsigned binaries.** The release files are not code-signed, so Windows
  and macOS cannot vouch for them; use the checksums or attestation above.

Where you can, prefer an SSH key held by `ssh-agent`: it removes the stored
secret altogether. `sshc ssh-add` and `sshc ssh-copy-id` exist to help you
get there.
