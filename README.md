# sshc

`ssh`, `scp`, `sftp`, `rsync` and `ssh-copy-id` that type your stored
password, or the passphrase of your SSH key, for you.

```console
$ sshc set --session          # type it once, hidden; kept for this terminal only
$ sshc w.go-2                 # no prompt
$ sshc scp -r ./site w.go-2:/var/www
```

## Install

**Windows** (PowerShell) - any one of:

```powershell
# installer script: downloads, verifies and installs the latest release
irm https://raw.githubusercontent.com/W-Industries-Luke/sshc/main/install.ps1 | iex

# Scoop
scoop bucket add sshc https://github.com/W-Industries-Luke/scoop-bucket
scoop install sshc
```

**macOS and Linux** - any one of:

```bash
# installer script: downloads, verifies and installs the latest release
curl -fsSL https://raw.githubusercontent.com/W-Industries-Luke/sshc/main/install.sh | sh

# Homebrew
brew install W-Industries-Luke/tap/sshc
```

Then open a new terminal and run `sshc --version` (or `sshc -v`). A `winget` package is
prepared but not yet in Microsoft's catalog. Manual downloads, building from
source and what each method does are covered in
[Getting started](#2-install-sshc).

## About

sshc is a thin wrapper around the OpenSSH client you already have. It is a
single executable with no runtime dependencies - no `sshpass`, no `expect` -
and runs on Linux, macOS and Windows.

> sshc stores secrets in plain text for convenience. Where you can, prefer an
> SSH key with [`ssh-agent`](#a-safer-alternative-ssh-agent), which gives you
> the same no-prompt logins without a readable secret on disk.

## Contents

- [Install](#install)
- [About](#about)
- [Password or passphrase?](#password-or-passphrase)
- [How it works](#how-it-works)
- [Requirements](#requirements)
- [Getting started](#getting-started)
  - [1. Check your OpenSSH](#1-check-your-openssh)
  - [2. Install sshc](#2-install-sshc)
  - [3. Give your host a short name (optional)](#3-give-your-host-a-short-name-optional)
  - [4. Store the password or passphrase](#4-store-the-password-or-passphrase)
  - [5. Check, then connect](#5-check-then-connect)
- [Usage](#usage)
  - [Moving a host to a key](#moving-a-host-to-a-key)
- [Storing passwords and passphrases](#storing-passwords-and-passphrases)
  - [Environment variables (preferred)](#environment-variables-preferred)
    - [The shell hook](#the-shell-hook)
  - [Config file](#config-file)
  - [Which one is used](#which-one-is-used)
  - [Jump hosts](#jump-hosts)
- [Safety](#safety)
  - [A safer alternative: ssh-agent](#a-safer-alternative-ssh-agent)
- [Notes](#notes)
- [Troubleshooting](#troubleshooting)
- [Development](#development)
- [License](#license)

## Password or passphrase?

ssh can ask you for two different secrets, and they are easy to mix up because
you type both at a prompt when you connect. sshc can answer either, but you
have to store the right kind. **The wording of the prompt tells you which one
you have:**

| ssh asks | What it is | Stored as | Store it with |
| -------- | ---------- | --------- | ------------- |
| `luke@203.0.113.7's password:` | a **login password**: your account's password on the server | `SSHC_PASSWORD` | `sshc set --session` |
| `Enter passphrase for key '/home/luke/.ssh/id_ed25519':` | a **key passphrase**: it unlocks a private key file on *your* machine, and never leaves it | `SSHC_PASSPHRASE` | `sshc set --session --passphrase` |

(`--session` keeps it in the current terminal's environment only. Leave it off
to save to the config file instead - see
[Storing passwords and passphrases](#storing-passwords-and-passphrases).)

Not sure? Run plain `ssh yourhost` once and read the prompt.

The two are kept apart on purpose. A value stored as a password is never tried
as a passphrase, or the other way round, so if sshc "still asks", the first
thing to check is that you stored the kind of secret the prompt is asking for.
You can store both - for example a key passphrase for one host and a login
password for another.

## How it works

OpenSSH has a hook, `SSH_ASKPASS`, for a program that answers its prompts.
sshc starts the real `ssh` with itself registered as that program, and answers
the password or passphrase prompt when ssh calls back. Because ssh does the asking:

- every option of the wrapped tool works, because sshc never rewrites your arguments;
- `~/.ssh/config` aliases, `ProxyJump`, port forwards, etc. behave as usual;
- the password is never on a command line, where `ps` would show it.

## Requirements

To run sshc:

| Dependency | Version | Needed for |
| ---------- | ------- | ---------- |
| OpenSSH client (`ssh`, `scp`, `sftp`) | 8.5 or newer | everything; preinstalled on Linux, macOS and Windows 10/11 |
| `rsync` | any | `sshc rsync` only |
| `ssh-copy-id` | any | `sshc ssh-copy-id` only; ships with OpenSSH on Linux and macOS |

sshc itself is one self-contained executable. It does **not** need `sshpass`,
`expect`, Python or any other runtime. Supported platforms are Linux, macOS
and Windows, on x86-64 and ARM64.

To build sshc from source (not needed if you download a release):

| Dependency | Version | Needed for |
| ---------- | ------- | ---------- |
| [Go](https://go.dev/dl/) | 1.26 or newer | building; it downloads the two Go modules sshc uses, `golang.org/x/term` and `golang.org/x/sys` |
| `make` | any | optional, for the shortcuts in the `Makefile` |
| Docker | any | optional, only for the end-to-end tests (`make e2e`) |

## Getting started

### 1. Check your OpenSSH

sshc drives the OpenSSH client that is already on your machine and needs
version 8.5 or newer:

```console
$ ssh -V
OpenSSH_9.6p1 ...
```

Linux and macOS have shipped a new enough version for years, and so do current
Windows 10 and 11 (as "OpenSSH Client", on by default). If `ssh` is not found
on Windows, enable it under *Settings > System > Optional features*.

### 2. Install sshc

Pick one method. All of them leave you with an `sshc` command on your `PATH`
and need no administrator rights.

**Installer script** - nothing else required. It downloads the right file for
your machine from the latest release, checks it against the release's
`SHA256SUMS`, and runs `sshc --install` (described below).

```powershell
irm https://raw.githubusercontent.com/W-Industries-Luke/sshc/main/install.ps1 | iex      # Windows
```

```bash
curl -fsSL https://raw.githubusercontent.com/W-Industries-Luke/sshc/main/install.sh | sh  # macOS, Linux
```

**Package manager** - if you already use one. Upgrades then come through it
(`scoop update sshc`, `brew upgrade sshc`).

```powershell
scoop bucket add sshc https://github.com/W-Industries-Luke/scoop-bucket   # Windows, Scoop
scoop install sshc
```

```bash
brew install W-Industries-Luke/tap/sshc                                   # macOS or Linux, Homebrew
```

**Manual download** - download the one file for your system, then run it once
with `--install`. That copies it to a per-user folder and puts that folder on
your `PATH`.

Download from the
[latest release](https://github.com/W-Industries-Luke/sshc/releases/latest):

| System | File |
| ------ | ---- |
| Windows (most PCs) | `sshc-windows-amd64.exe` |
| Windows on ARM | `sshc-windows-arm64.exe` |
| Linux (most PCs and servers) | `sshc-linux-amd64` |
| Linux on ARM (Raspberry Pi, Graviton, ...) | `sshc-linux-arm64` |
| macOS, Apple silicon | `sshc-darwin-arm64` |
| macOS, Intel | `sshc-darwin-amd64` |

**Windows** (PowerShell, in the folder you downloaded to)

```powershell
.\sshc-windows-amd64.exe --install
```

It installs to `%LOCALAPPDATA%\Programs\sshc` and adds that to your user
`Path`. Windows may warn that the file is from an unknown publisher, because
the binaries are not code-signed; choose *More info > Run anyway*, or build
from source instead.

**Linux and macOS**

```console
$ chmod +x sshc-linux-amd64
$ ./sshc-linux-amd64 --install
```

It installs to `~/.local/bin`. If that is not on your `PATH` yet, it appends
one line to your shell's startup file (`~/.bashrc`, `~/.zshrc`, ...) and tells
you which. On macOS, a file downloaded with a browser has to be released from
quarantine first: `xattr -d com.apple.quarantine sshc-darwin-arm64`.

**Then open a new terminal** - the one you installed from still has the old
`PATH` - and check:

```console
$ sshc --version
```

After that you can delete the downloaded file. `sshc --install <directory>`
installs somewhere else; running `--install` from a newer download upgrades.
On Linux and macOS, `--install` also adds the [shell hook](#the-shell-hook)
that `sshc set --session` needs.
`SHA256SUMS` on the release page lets you verify the download.

**From source instead** (needs [Go](https://go.dev/dl/) 1.26 or newer):

```console
$ go install github.com/W-Industries-Luke/sshc@latest
```

This puts `sshc` in Go's `bin` folder (`~/go/bin`), which the Go installer
adds to `PATH` on Windows but usually not on Linux or macOS. From a clone,
`make install` builds and copies to `~/.local/bin`.

> Windows support is new and has seen far less real use than the Linux
> build - please open an issue if something misbehaves.

### 3. Give your host a short name (optional)

sshc uses your `~/.ssh/config` (`%USERPROFILE%\.ssh\config` on Windows) like
ssh does, so an alias saves typing the full connection string:

```
Host w.go-2
    HostName 203.0.113.7
    User luke
    Port 22
```

### 4. Store the password or passphrase

First work out [which of the two](#password-or-passphrase) your host asks for.
Then choose where it should live.

**In this terminal only (preferred).** Nothing is written to disk; the secret
is held in an environment variable of the terminal you are in.

```console
$ sshc set --session                # a login password
$ sshc set --session --passphrase   # or: the passphrase of your SSH key
New passphrase:
Again:
Updated!
  SSHC_PASSPHRASE is set for this terminal session only.
```

You type it hidden, twice. It then **stays in effect until you set it again or
close the terminal** - every `sshc` command in that terminal uses it, and no
other terminal can see it.

`--session` relies on a small [shell hook](#the-shell-hook). The installer
script and `sshc --install` set it up on Linux and macOS; on Windows, and after
installing with Scoop or Homebrew, it is one line to add yourself. Without the
hook you can set the variable by hand - see
[Environment variables](#environment-variables-preferred).

**In every terminal.** Leave off `--session` and sshc saves to its
[config file](#config-file) instead, where it stays until you change it:

```console
$ sshc set                    # a login password
$ sshc set --passphrase       # or: the passphrase of your SSH key
```

### 5. Check, then connect

```console
$ sshc --check w.go-2
config file: none
w.go-2 -> 203.0.113.7 (user luke)
  no stored password; you would be prompted
  key ~/.ssh/id_ed25519: passphrase from environment variable SSHC_PASSPHRASE
$ sshc w.go-2
```

`--check` does not connect and never prints a secret. It lists the login
password and each key file ssh would try for that host, so a line saying "no
stored password" is fine when you log in with a key, as above. The first time you
reach a new host, ssh still asks you to confirm its host key, as always.

## Usage

```
sshc [ssh] [ssh options] destination [command ...]
sshc scp  [scp options] source ... target
sshc sftp [sftp options] destination
sshc rsync [rsync options] source ... target
sshc ssh-copy-id [ssh-copy-id options] destination
```

Whatever follows is passed to the tool as is:

```console
$ sshc w.go-2                              # alias from ~/.ssh/config
$ sshc -p 2222 luke@203.0.113.7            # full connection string
$ sshc -L 8080:localhost:80 w.go-2 -N
$ sshc scp -r ./copy-path w.go-2:paste-path
$ sshc scp -P 2222 luke@203.0.113.7:log.txt .
$ sshc sftp w.go-2
$ sshc rsync -av --delete ./site/ w.go-2:/var/www/
$ sshc ssh-copy-id w.go-2
```

`sshc -v` and `sshc -h` on their own are short for `--version` and `--help`.
Together with a destination they keep their ssh meaning, so `sshc -v w.go-2`
is still ssh's verbose mode.

A host that happens to share a name with a subcommand is reachable through
the explicit form, e.g. `sshc ssh scp` or `sshc ssh set`.

`rsync` and `ssh-copy-id` are not part of OpenSSH and have to be installed
separately; Windows ships neither, so there these two subcommands are for WSL.

### Moving a host to a key

`ssh-copy-id` installs your public key on the server, which normally costs one
last password prompt. With sshc it costs none, and afterwards the host no
longer needs a stored password at all:

```console
$ sshc ssh-copy-id w.go-2
$ ssh w.go-2                  # logs in with the key
```

If nothing is stored anywhere, sshc simply runs the tool.

## Storing passwords and passphrases

### Environment variables (preferred)

`SSHC_PASSWORD` is the active login password and `SSHC_PASSPHRASE` the active
key passphrase. They live in the terminal that set them and the programs it
starts, so different terminals can hold different values at the same time, and
nothing is written to disk.

`sshc set --session` sets them for you, asking for the value hidden:

| Command | Sets |
| ------- | ---- |
| `sshc set --session` | `SSHC_PASSWORD` |
| `sshc set --session --passphrase` | `SSHC_PASSPHRASE` |
| `sshc set --session --host w.go-2` | `SSHC_PASSWORD_W_GO_2`, for that one host |
| `sshc set --session --key id_ed25519` | `SSHC_PASSPHRASE_ID_ED25519`, for that one key |

A value set this way **stays in effect for the rest of that terminal session**.
Every `sshc` command you run there uses it until you set it again, unset it,
or close the terminal - it is not asked for again and does not expire. Other
terminals, including ones you open later, are unaffected and start without it.
A variable also takes precedence over the config file, so plain `sshc set`
does not change what a terminal with the variable set will use.

```bash
unset SSHC_PASSWORD                  # bash / zsh: stop using it in this terminal
```

```powershell
Remove-Item Env:SSHC_PASSWORD        # PowerShell
```

#### The shell hook

A program cannot change the environment of the terminal that started it, so
`--session` works through a few lines of shell code that wrap the `sshc`
command. They have to be loaded when your shell starts:

| Shell | Add this line to | Line |
| ----- | ---------------- | ---- |
| bash | `~/.bashrc` | `eval "$(sshc --shell-init bash)"` |
| zsh | `~/.zshrc` | `eval "$(sshc --shell-init zsh)"` |
| fish | `~/.config/fish/config.fish` | `sshc --shell-init fish \| source` |
| PowerShell | the file `$PROFILE` names | `sshc --shell-init powershell \| Out-String \| Invoke-Expression` |

On Linux and macOS, the installer script and `sshc --install` add the line for
you. On Windows, run these two lines once:

```powershell
if (!(Test-Path $PROFILE)) { New-Item -Force -ItemType File $PROFILE | Out-Null }
Add-Content $PROFILE 'sshc --shell-init powershell | Out-String | Invoke-Expression'
```

If new PowerShell windows then say that running scripts is disabled, allow
your own profile with `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned`.
Open a new terminal afterwards. `sshc --shell-init` prints the hook if you
want to read it first; all it does is intercept `sshc set --session`.

#### Without the hook

The variables are ordinary environment variables, so you can also set them
yourself. These forms keep the value out of your shell history:

```bash
# bash / zsh
read -rs SSHC_PASSWORD && export SSHC_PASSWORD
```

```powershell
# PowerShell 7
$env:SSHC_PASSWORD = Read-Host -MaskInput 'Password'
```

For a password that belongs to one host, add the host to the variable name -
upper-cased, with everything that is not a letter or digit turned into `_`:

| You connect to   | Variable                    |
| ---------------- | --------------------------- |
| `w.go-2`         | `SSHC_PASSWORD_W_GO_2`      |
| `203.0.113.7`    | `SSHC_PASSWORD_203_0_113_7` |
| `root@w.go-2`    | `SSHC_PASSWORD_ROOT_W_GO_2` |

A passphrase that belongs to one key works the same way, with the key's file
name: `SSHC_PASSPHRASE_ID_ED25519` for `~/.ssh/id_ed25519`.

### Config file

Without `--session`, `sshc set` saves to the config file, creating it if
needed. Use it for a secret you want available in every terminal without
setting it each time. Which option you give decides what is saved:

| Command | Saves |
| ------- | ----- |
| `sshc set` | the login password of the active profile |
| `sshc set --passphrase` | the key passphrase of the active profile, tried for any key |
| `sshc set --host w.go-2` | the login password for that one host |
| `sshc set --key id_ed25519` | the passphrase for that one key (file name or full path) |

Add `--profile NAME` to the first two to save into a profile other than the
active one. Each command asks for the value hidden; you can also put it at the
end of the command (`sshc set 'correct horse'`), with the caveats below.

The file is the persistent counterpart of the environment variables, which
still win in a terminal where they are set.

When the value is given on the command line, sshc clears the screen and
scrollback afterwards so it is not left on display. `--no-clear`, or
`clear_on_set = no` in the config file, turns that off. Clearing the screen
does not remove the command from your **shell history**, and the argument is
briefly visible to other users in the process list - use plain `sshc set` to
avoid both.

You can also edit the file by hand. `sshc --init` creates `sshc.conf` next to
the sshc executable. If that location is not writable, or sshc was installed
by a package manager (Scoop, Homebrew, winget - they replace that folder on
every upgrade), the file goes in your user config directory instead:
`~/.config/sshc/` on Linux, `~/Library/Application Support/sshc/` on macOS,
`%AppData%\sshc\` on Windows. `$SSHC_CONFIG` points somewhere else, and
`sshc --check` always shows which file is in use.

```ini
# The active profile. $SSHC_PROFILE overrides this per shell.
profile = work

# A profile holds a login password, a key passphrase, or both.
[profile work]
password = correct horse battery staple
passphrase = unlocks my ssh key

[profile home]
password = hunter2

# One host's login password. Always wins over the active profile.
[host w.go-2]
password = something else

# One key's passphrase. Always wins over the active profile.
[key id_ed25519]
passphrase = something else again
```

Switch profile for one shell with `export SSHC_PROFILE=home`, or for one
command with `SSHC_PROFILE=home sshc w.go-2`.

Values run to the end of the line and are taken literally. The file is parsed
as data and nothing in it is ever executed.

### Which one is used

First match wins. The two columns never cross over:

|   | For a login password prompt | For a key passphrase prompt |
| - | --------------------------- | --------------------------- |
| 1 | `SSHC_PASSWORD_<HOST>` | `SSHC_PASSPHRASE_<KEYFILE>` |
| 2 | `[host <name>]` in the config file | `[key <name>]` in the config file |
| 3 | `SSHC_PASSWORD` | `SSHC_PASSPHRASE` |
| 4 | `password =` in the active profile | `passphrase =` in the active profile |

A host entry can be written with the alias you type or with the real host
name, with or without `user@`. A key entry can be the key's file name or its
full path.

`sshc --check <destination>` shows which sources would be used, without
connecting and without printing any secret:

```console
$ sshc --check w.go-2
config file: none
w.go-2 -> 203.0.113.7 (user luke)
  password from environment variable SSHC_PASSWORD
  key ~/.ssh/id_ed25519: no stored passphrase; you would be prompted if it has one
```

### Jump hosts

The active login password (rows 3 and 4 above) is only ever offered to the
destination you named. A jump host gets a password only if it has its own host entry, so
your password for one machine is never handed to another one on the way:

```console
$ export SSHC_PASSWORD_BASTION_EXAMPLE_COM=...
$ sshc -J bastion.example.com w.go-2
```

Use the jump host's real host name here; only command-line destinations are
matched by alias.

Key passphrases have no such restriction: a passphrase only unlocks a file on
your own machine and is never sent to any host, so the active passphrase is
tried for whichever key ssh asks about.

## Safety

What sshc does:

- **Answers only two kinds of prompt**: a login password, and only for the
  host that the local ssh client says is asking; and the passphrase of a local
  key file. Host key confirmations, one-time codes, security-key PINs and
  password-change prompts go to your terminal as usual. A server cannot word a
  prompt to obtain a different host's password, or a key passphrase.
- **Offers a stored password once.** If the server rejects it, sshc says so and
  lets you type instead of repeating it and locking the account.
- **Never accepts a host key for you.** Your `StrictHostKeyChecking` setting is
  untouched.
- **Refuses a config file other users can read** (Linux and macOS: it must be
  owned by you with mode 600).
- **Keeps passwords off command lines and out of its own output.**

What it cannot do:

- A password in an environment variable is readable by other processes running
  as you (and by root), like any environment variable. Do not add `SSHC_*` to
  `SendEnv` in your ssh config.
- A password in `sshc.conf` is plain text on disk. Never commit that file.
- A stored **key passphrase** deserves extra thought. The passphrase exists so
  that someone who copies your private key file still cannot use it. Stored in
  plain text on the same machine, it no longer protects against anyone who can
  read your files - it is then roughly as safe as a key with no passphrase.
- On Windows the config file is protected by the folder's ACL rather than a
  mode check, so keep it somewhere under your user profile.

### A safer alternative: ssh-agent

If the prompt you want rid of is a key passphrase, OpenSSH has a built-in
answer that needs no stored secret: `ssh-agent` holds the unlocked key in
memory, and `ssh`, `scp`, `git` and the rest use it without asking.

```console
$ eval "$(ssh-agent)"         # Linux / macOS, once per login session
$ ssh-add ~/.ssh/id_ed25519   # type the passphrase one last time
```

On Windows the agent is a service, and it remembers keys across reboots. In an
administrator PowerShell, once:

```powershell
Get-Service ssh-agent | Set-Service -StartupType Automatic
Start-Service ssh-agent
```

then, in a normal one: `ssh-add $HOME\.ssh\id_ed25519`.

Use sshc's passphrase support where an agent is not practical - in scripts and
scheduled jobs, or on machines where you cannot run one.

## Notes

- `sftp -b batchfile` turns on ssh's batch mode, which disables password and
  passphrase prompts altogether. Add `-o BatchMode=no` before `-b` to use a
  stored secret in a batch.
- sshc adds one `ssh -V` and one `ssh -G` call before connecting (about 10 ms)
  to check the client version and resolve the destination.

## Troubleshooting

**It still asks me for the password.** Run `sshc --check <destination>` with
the same arguments. If it says "no stored password", the variable is not set
in this terminal (a value from `sshc set --session` does not carry over to new
ones) or the config file is
not where sshc looks - the first line of the output shows which file is in
use.

Then read the prompt itself. `Enter passphrase for key ...` needs a stored
*passphrase* (`sshc set --passphrase`), and `...'s password:` needs a stored
*password* (`sshc set`); having only the other kind is the most common reason
for still being asked. See [Password or passphrase?](#password-or-passphrase).
Otherwise the prompt may come from a [jump host](#jump-hosts), or be a
one-time code, which sshc leaves to you.

Setting `SSHC_DEBUG=1` makes sshc print what it decides at each step (never
the secret itself).

**"the stored password for ... was not accepted"** (or passphrase). It was
rejected, so sshc stopped offering it and let you type. Update the stored
value.

**"--session needs the sshc shell hook".** The [shell hook](#the-shell-hook)
is not loaded in this terminal. Add the line the message shows to your shell's
startup file and open a new terminal.

**"OpenSSH 8.5 or newer is required".** Upgrade the OpenSSH client. On Windows
a newer one is available from the
[Win32-OpenSSH](https://github.com/PowerShell/Win32-OpenSSH) project.

**"config file ... is accessible by other users".** Run the `chmod 600`
command from the message. sshc will not read passwords from a file that other
accounts on the machine can open.

**"rsync is not installed, or not on your PATH"** (or `ssh-copy-id`). sshc
runs the real tool, so it has to be installed.

**`sftp -b` fails with "Permission denied".** See [Notes](#notes): add
`-o BatchMode=no` before `-b`.

**In a script or cron job it fails instead of prompting.** That is intended:
with no terminal there is nobody to ask, so a host without a matching stored
password is a failed login rather than a hang.

## Development

The repository is laid out as follows:

```
main.go             entry point; everything else is in internal/sshc
internal/sshc/      the program, one file per concern (see the comment at the
                    top of cli.go for a guide), with its unit tests alongside
install.sh          one-line installers for Linux/macOS and Windows; they are
install.ps1         fetched by URL, so they stay at the top level
test/e2e.sh         end-to-end tests against a real sshd in Docker
packaging/          where sshc is published and how to update each channel
.github/workflows/  CI: unit tests on Linux, Windows and macOS, plus e2e
```

```console
$ make test     # go vet + unit tests
$ make e2e      # real ssh/scp/sftp against an sshd in Docker
$ make dist     # cross-compile release binaries
```

## License

MIT
