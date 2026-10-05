# Changelog

## 0.7.1

- `--dir` and `--entry` can be given inline, for one connection, before the host: `sshc --dir /var/www w.go-2`. Nothing is stored.
- Inline, they also apply when you give a command: `sshc --dir /var/www w.go-2 git status` runs it in that directory, after the entry command, and stops if either fails.
- An inline value replaces the stored one for that connection; `--no-entry` may now appear anywhere before the host, and wins over both.
- They are long options only on a connection: `-d` and `-e` there would be read as ssh's own.

## 0.7.0

- A start directory and an entry command per host: `sshc set -H HOST -d /var/www` and `sshc set -H HOST -e 'COMMAND'` (`--dir`, `--entry`). An interactive login to that host then starts in the directory, runs the command, and hands over to your normal login shell.
- They apply only to plain `sshc HOST` at a terminal. A command of your own, `scp`, `sftp`, `rsync`, `each`, `run`, port forwarding with `-N` and piped input are untouched - unlike ssh's `RemoteCommand`, which applies to every connection.
- `sshc --no-entry HOST` (or `SSHC_NO_ENTRY=1`) logs in without them once. `sshc unset -H HOST -d` / `-e` removes one; `sshc list` and `sshc check` show them.

This is the one case in which sshc adds to what it passes to ssh, and only for a host you have set it up for. It expects a Unix-style shell on the host.

## 0.6.2

No more "open a new terminal" after an update:

- The shell hook now keeps itself current. `sshc update` and `sshc install` reload it in the terminal they run in, and if sshc was updated some other way (a package manager, the install script in another window), the next `sshc set`, `unset` or `use` notices the old hook and replaces it.
- On Windows, the install script brings the PowerShell window it runs in up to date: the install folder goes on that session's `Path` and the hook is loaded, so `sshc` works straight away.
- On Linux and macOS, `sshc install` ends by naming the one command that refreshes the current terminal (`. ~/.bashrc` or the like), since a program cannot change the shell that started it.

A terminal opened before 0.6.2 still has the old hook, so it needs reopening one last time - or one `sshc set`, `unset` or `use`, which replaces it.

## 0.6.1

- While sshc is locked, `sshc set`, `sshc unset` and `sshc use` now say so, so that a change made then does not look as if it had no effect. `sshc list` shows the lock as a message of its own, set apart by a blank line like every other message.

## 0.6.0

- `sshc lock` switches sshc off: while locked it supplies no stored password, passphrase or one-time code from any source, and connections ask you instead. `sshc unlock` switches it back on after your device has verified you - Windows Hello on Windows, the account password on macOS and Linux.
- `lock_after = 8h` (or `30m`, `2d`, ...) in the config file locks sshc by itself when it has not been used for that long.
- `sshc list`, `sshc check` and `sshc doctor` show when sshc is locked. `sshc lock` refuses where an unlock could not work: without Windows Hello set up, or as root.

The lock is a switch that sshc obeys, for a person at an unattended terminal or a stray script. It does not encrypt anything and does not stop other software running under your account from reading the credential store or removing the lock; `SECURITY.md` says so in full.

Known limitation: the Windows Hello prompt cannot be exercised by automated tests, so on Windows the unlock itself has been tested only as far as detecting that Hello is unavailable.

## 0.5.0

**New commands**

- `sshc doctor` checks the setup - OpenSSH version, copies of sshc on `PATH`, the shell hook, the config file, the credential store, ssh-agent - and says how to fix each problem.
- `sshc update` installs the latest release after checking its checksum; `sshc update --check` only looks. A copy installed by Scoop, Homebrew or winget is pointed at its package manager instead.
- `sshc use NAME` switches the active profile for the current terminal; `sshc use --save NAME` changes the default; `sshc use` shows it.
- `sshc migrate` moves every plain-text secret in the config file into the credential store.
- `sshc help COMMAND` shows the help of `set`, `unset`, `use`, `run` or `each`.

**One-time codes**

- `sshc set -o -H HOST -c 'COMMAND'` supplies the verification code a host asks for after the password, by running a command such as `oathtool` or a password manager. It is opt-in per host.

**Other additions**

- `log = yes` in the config file records every prompt sshc answers: when, for what, and where the secret came from. Never the secret.
- `SSHC_NO_PROMPT=1` makes sshc fail instead of asking on the terminal, for scripts and scheduled jobs.
- On Windows, `sshc install` now adds the shell hook to your PowerShell profile itself when the script execution policy allows profiles to load.
- `.deb` and `.rpm` packages, a man page, and an AUR recipe.

**Releases**

- Releases are now built by GitHub Actions from the tagged commit and carry a build provenance attestation: `gh attestation verify <file> --repo W-Industries-Luke/sshc`.
- A `SECURITY.md` describes how to report a vulnerability and what sshc does and does not protect against.
- The end-to-end tests now also run on Windows and macOS, against an SSH server built into the test.

**Fixed**

- Tab completion of `user@h...` no longer offers sshc's own commands.

## 0.4.0

- Password managers as a source: `sshc set -c 'COMMAND'`.
- `sshc ssh-add`, `sshc run`, `sshc each`, the host picker (`sshc`, `sshc pick`, `sshc hosts`).
- Tab completion for bash, zsh, fish and PowerShell.
- A GitHub Action that installs sshc on a runner.

## 0.3.2

- Messages are spaced out and, on a terminal, coloured.

## 0.3.1

- `sshc install` warns when another copy of sshc comes first on `PATH`.
- `sshc -h` lists the short options.

## 0.3.0

- Saved secrets go to the system credential store instead of plain text.
- `sshc unset`, `sshc list`, short options, plain-word commands.

## 0.2.1

- `sshc -v` and `sshc -h` on their own.

## 0.2.0

- `sshc set --session` and the shell hook.

## 0.1.1

- Install scripts; config file kept out of package-manager directories.

## 0.1.0

- First release: ssh, scp, sftp, rsync and ssh-copy-id with a stored login password or key passphrase; `sshc set`; `sshc --install`.
