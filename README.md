# sshc

`ssh`, `scp`, `sftp`, `rsync` and `ssh-copy-id` that type your stored
password, or the passphrase of your SSH key, for you.

```console
$ sshc set -s                 # type it once, hidden; kept for this terminal only
$ sshc w.go-2                 # no prompt
$ sshc scp -r ./site w.go-2:/var/www
$ sshc each web1 web2 -- uptime
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

Then open a new terminal and run `sshc -v`. A `winget` package has been
submitted and is awaiting Microsoft's review; until it is accepted,
`winget install` will not find sshc. Manual downloads, building from source
and what each method does are covered in [Getting started](#2-install-sshc).

## About

sshc is a thin wrapper around the OpenSSH client you already have. It is a
single executable with no runtime dependencies - no `sshpass`, no `expect` -
and runs on Linux, macOS and Windows.

What it does:

- **Answers the prompt for you** in `ssh`, `scp`, `sftp`, `rsync` and
  `ssh-copy-id`, for a login password or an SSH
  [key passphrase](#password-or-passphrase), with every option of those tools
  still working.
- **Keeps the secret where you choose**: for
  [this terminal only](#environment-variables), encrypted in your system's
  [credential store](#the-credential-store), or not at all - fetched from
  [1Password, Bitwarden, `pass` or any other password manager](#password-managers)
  when it is needed.
- **Works with programs that call ssh themselves**: `sshc run git push`,
  `sshc run ansible-playbook ...` - see
  [`sshc run`](#git-ansible-and-other-programs-sshc-run).
- **Unlocks your key once for everything** with
  [`sshc ssh-add`](#unlocking-a-key-for-everything-sshc-ssh-add), so plain
  `ssh` and `git` stop asking too.
- **Runs a command on [several hosts at once](#several-hosts-at-once)**:
  `sshc each web1 web2 -- uptime`.
- **Finds your hosts**: a [host picker](#picking-a-host) when you run `sshc`
  on its own, and Tab completion of host names in bash, zsh, fish and
  PowerShell.
- **Handles one-time codes** too, for hosts that ask for a
  [verification code](#one-time-codes) after the password.
- **Installs in CI** as a [GitHub Action](#github-actions), for deploying to
  hosts that only take a password.
- **Can be switched off**: [`sshc lock`](#locking-sshc) stops it supplying
  anything until your device has verified you again.
- **Looks after itself**: [`sshc doctor`](#when-something-is-off-sshc-doctor)
  diagnoses the setup and `sshc update` installs the latest release.

> Where you can, prefer an SSH key with
> [`ssh-agent`](#a-safer-alternative-ssh-agent): it gives you the same
> no-prompt logins with no stored secret at all. sshc is for everything else.

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
  - [sshc's own commands](#sshcs-own-commands)
  - [Picking a host](#picking-a-host)
  - [Several hosts at once](#several-hosts-at-once)
  - [git, ansible and other programs: sshc run](#git-ansible-and-other-programs-sshc-run)
  - [Unlocking a key for everything: sshc ssh-add](#unlocking-a-key-for-everything-sshc-ssh-add)
  - [When something is off: sshc doctor](#when-something-is-off-sshc-doctor)
  - [Moving a host to a key](#moving-a-host-to-a-key)
- [Storing passwords and passphrases](#storing-passwords-and-passphrases)
  - [Environment variables](#environment-variables)
    - [The shell hook](#the-shell-hook)
  - [The credential store](#the-credential-store)
  - [Password managers](#password-managers)
  - [One-time codes](#one-time-codes)
  - [Profiles](#profiles)
  - [Config file](#config-file)
  - [Which one is used](#which-one-is-used)
  - [Jump hosts](#jump-hosts)
- [Safety](#safety)
  - [Locking sshc](#locking-sshc)
  - [A safer alternative: ssh-agent](#a-safer-alternative-ssh-agent)
- [GitHub Actions](#github-actions)
- [Notes](#notes)
- [Troubleshooting](#troubleshooting)
- [Development](#development)
- [License](#license)

## Password or passphrase?

ssh can ask you for two different secrets, and they are easy to mix up because
you type both at a prompt when you connect. sshc can answer either, but you
have to store the right kind. **The wording of the prompt tells you which one
you have:**

| ssh asks | What it is | Store it with |
| -------- | ---------- | ------------- |
| `luke@203.0.113.7's password:` | a **login password**: your account's password on the server | `sshc set` |
| `Enter passphrase for key '/home/luke/.ssh/id_ed25519':` | a **key passphrase**: it unlocks a private key file on *your* machine, and never leaves it | `sshc set -p` |

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
- the secret is never on a command line, where `ps` would show it.

## Requirements

To run sshc:

| Dependency | Version | Needed for |
| ---------- | ------- | ---------- |
| OpenSSH client (`ssh`, `scp`, `sftp`) | 8.5 or newer | everything; preinstalled on Linux, macOS and Windows 10/11 |
| `rsync` | any | `sshc rsync` only |
| `ssh-copy-id` | any | `sshc ssh-copy-id` only; ships with OpenSSH on Linux and macOS |
| `secret-tool` (`libsecret-tools`) and a running keyring | any | Linux only, and optional: lets `sshc set` use the encrypted keyring instead of a plain-text file |
| `curl` | any | `sshc update` and the install scripts only; ships with Windows 10/11 and macOS |

sshc itself is one self-contained executable. It does **not** need `sshpass`,
`expect`, Python or any other runtime. Supported platforms are Linux, macOS
and Windows, on x86-64 and ARM64.

To build sshc from source (not needed if you download a release):

| Dependency | Version | Needed for |
| ---------- | ------- | ---------- |
| [Go](https://go.dev/dl/) | 1.26 or newer | building; it downloads the Go modules sshc uses, `golang.org/x/term` and `golang.org/x/sys`, plus `golang.org/x/crypto` for the tests only |
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

Pick **one** method and stay with it. All of them leave you with an `sshc`
command on your `PATH` and need no administrator rights. Installing with two
methods (say Scoop and the installer script) leaves two copies, and whichever
folder comes first on `PATH` wins - see
[Troubleshooting](#troubleshooting) if `sshc -v` shows an old version.

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

**Linux package** - each release also has `.deb` and `.rpm` files, for
`amd64` and `arm64`, which install `sshc` to `/usr/bin` with its man page:

```console
$ sudo apt install ./sshc_0.6.1_amd64.deb        # Debian, Ubuntu
$ sudo dnf install ./sshc-0.6.1-1.x86_64.rpm     # Fedora, RHEL
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
that `sshc set -s` needs; on Windows it does so when PowerShell's script
policy allows profiles to load, and otherwise prints the two lines to run.

**Verifying a download.** Every release lists its files in `SHA256SUMS`, and
the install scripts, `sshc update` and the GitHub Action check that before
running anything. The files are built by GitHub Actions from the tagged
commit and carry a build attestation, which the GitHub CLI can check:

```console
$ gh attestation verify sshc-linux-amd64 --repo W-Industries-Luke/sshc
```

**Upgrading.** `sshc update` downloads the latest release, checks it against
its checksum and replaces the copy you are running (`sshc update --check`
only tells you whether there is one). If sshc came from a package manager it
says so instead: use `scoop update sshc`, `brew upgrade sshc`, or your
system's package tool. Afterwards open a new terminal, so that the
[shell hook](#the-shell-hook) of the new version is loaded.

**From source instead** (needs [Go](https://go.dev/dl/) 1.26 or newer):

```console
$ go install github.com/W-Industries-Luke/sshc@latest
```

This puts `sshc` in Go's `bin` folder (`~/go/bin`), which the Go installer
adds to `PATH` on Windows but usually not on Linux or macOS. From a clone,
`make install` builds and copies to `~/.local/bin`.

> sshc is tested on Linux, macOS and Windows on every change, but it is a
> young project and Windows has seen the least everyday use - please open an
> issue if something misbehaves.

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
Then choose how long it should be kept. Either way you type it hidden, twice.

**For this terminal only** - add `-s` (`--session`). Nothing is written
anywhere; the secret is held in an environment variable of the terminal you
are in.

```console
$ sshc set -s                 # a login password
$ sshc set -sp                # or: the passphrase of your SSH key
New passphrase:
Again:

Updated!

SSHC_PASSPHRASE is set for this terminal session only.

```

It **stays in effect until you set it again or close the terminal**: every
`sshc` command in that terminal uses it, and no other terminal can see it.
`-s` relies on a small [shell hook](#the-shell-hook), which the installer sets
up for you (`sshc doctor` tells you whether it is loaded).

**For every terminal, until you remove it** - leave `-s` off. The secret goes
into your system's [credential store](#the-credential-store) (Windows
Credential Manager, macOS Keychain, the Linux keyring), encrypted:

```console
$ sshc set                    # a login password
$ sshc set -p                 # or: the passphrase of your SSH key
New passphrase:
Again:

Updated!

passphrase of [profile default], kept in Windows Credential Manager

```

`sshc list` shows what is stored and where (never the values), and
`sshc unset` removes an entry.

### 5. Check, then connect

```console
$ sshc check w.go-2

config file: none

w.go-2 -> 203.0.113.7 (user luke)
  no stored password; you would be prompted
  key ~/.ssh/id_ed25519: passphrase from environment variable SSHC_PASSPHRASE

$ sshc w.go-2
```

`check` does not connect and never prints a secret. It lists the login
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
sshc ssh-add [ssh-add options] [key ...]
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

If nothing is stored anywhere, sshc simply runs the tool.

`rsync` and `ssh-copy-id` are not part of OpenSSH and have to be installed
separately; Windows ships neither, so there these two subcommands are for WSL.

### sshc's own commands

Besides running the tools above, sshc has a few commands of its own:

| Command | Does |
| ------- | ---- |
| `sshc set` | store a login password; `-p` for a key passphrase |
| `sshc unset` | remove a stored one |
| `sshc list` | show what is stored and where, never the values |
| `sshc check <destination>` | show which stored secret a connection would use |
| `sshc` (nothing else), `sshc pick` | [pick a host](#picking-a-host) from your ssh config and connect |
| `sshc hosts` | list the hosts in your ssh config |
| `sshc each <hosts> -- <command>` | [run a command on several hosts](#several-hosts-at-once) |
| `sshc run <command>` | [run any program that uses ssh](#git-ansible-and-other-programs-sshc-run), such as git |
| `sshc use [NAME]` | show or switch the [active profile](#profiles) |
| `sshc migrate` | move plain-text entries into the credential store |
| `sshc lock`, `sshc unlock` | [switch sshc off](#locking-sshc), and back on after your device has verified you |
| `sshc doctor` | [check the setup](#when-something-is-off-sshc-doctor) and say how to fix it |
| `sshc update` | install the latest release |
| `sshc install` | copy sshc to a per-user folder and put it on `PATH` |
| `sshc init` | create a config file template |
| `sshc shell-init [shell]` | print the [shell hook](#the-shell-hook) |
| `sshc help [COMMAND]`, `sshc version` | also `-h` and `-v`, when given on their own |

`check`, `install`, `init`, `shell-init`, `help` and `version` can also be
written with a leading `--` (`sshc --check ...`).

Every option of `set` and `unset` has a one-letter form, and the letters
combine:

| Short | Long | Meaning |
| ----- | ---- | ------- |
| `-s` | `--session` | this terminal only, as an environment variable |
| `-p` | `--passphrase` | a key passphrase rather than a login password |
| `-H NAME` | `--host NAME` | the login password of one host |
| `-k NAME` | `--key NAME` | the passphrase of one key |
| `-P NAME` | `--profile NAME` | a profile other than the active one |
| `-c CMD` | `--command CMD` | do not store it; [run CMD](#password-managers) each time to fetch it |
| `-o` | `--otp` | a [one-time code](#one-time-codes); needs `-H` and `-c` |
| `-f` | `--plain` | keep it in the config file, in plain text |
| `-n` | `--no-clear` | do not clear the screen afterwards |
| `-h` | `--help` | show the options |

So `sshc set -sp` is `sshc set --session --passphrase`, and
`sshc set -H w.go-2` stores a password for that host only.

sshc does not use single letters for `check`, `install` and the rest, because
`-c`, `-i` and friends are already ssh options and are passed through to ssh.
For the same reason `sshc -v` and `sshc -h` mean `version` and `help` only
when given on their own: `sshc -v w.go-2` is still ssh's verbose mode.

A host that happens to share a name with one of these commands is reachable
through the explicit form, e.g. `sshc ssh list` or `sshc ssh set`.

sshc's own messages are set apart by blank lines, and on a terminal their key
words are coloured. Output that is piped or redirected is always plain, and
`NO_COLOR=1` turns colour off on a terminal too.

### Picking a host

Run `sshc` with nothing after it, or `sshc pick`, to choose from the hosts in
your `~/.ssh/config`:

```console
$ sshc

    1  db-prod
    2  web1
    3  web2

Connect to (number or part of a name, Enter to cancel): web
```

Type a number, a name, or any part of a name. If more than one host matches,
the list narrows and asks again. `sshc hosts` just prints the list. (When
there is no terminal, or no hosts are defined, `sshc` alone prints the help
as before.)

With the [shell hook](#the-shell-hook) loaded, pressing Tab completes host
names too: `sshc w<Tab>`, `sshc scp ./file w<Tab>`, `sshc set -H <Tab>`.

### Several hosts at once

```console
$ sshc each web1 web2 db-prod -- uptime
web1    |  14:02:11 up 12 days,  3:41,  0 users,  load average: 0.08, 0.03, 0.01
db-prod |  14:02:11 up 40 days,  1:02,  1 user,   load average: 0.61, 0.44, 0.40
web2    |  14:02:12 up 12 days,  3:40,  0 users,  load average: 0.00, 0.01, 0.00

Done: all 3 hosts succeeded.

```

Everything before `--` is hosts, everything after is the command. The hosts
run at the same time (`-j N`, or `--jobs N`, limits how many), each line is labelled with the
host it came from, and the exit status is non-zero if any host failed. ssh
options placed before the hosts (`sshc each -o ConnectTimeout=5 web1 web2 --
...`) apply to all of them.

Because the hosts share your terminal, nothing is asked while this runs: a
host that needs a secret sshc has not stored, or whose host key you have not
accepted yet, fails and is listed at the end. Connect to a new host once on
its own first.

### git, ansible and other programs: `sshc run`

Many programs start ssh themselves. `sshc run` runs any of them with sshc
answering the prompts underneath:

```console
$ sshc run git push
$ sshc run ansible-playbook site.yml
$ sshc run -d deploy.example.com -- ./deploy.sh
```

Stored key passphrases and host-specific passwords are used automatically.
The active login password is the exception: sshc cannot see which host the
program will connect to, so it is only offered to hosts you name with `-d`
(`--dest`, by the name the server is reached under), never to whatever
happens to ask.

### Unlocking a key for everything: `sshc ssh-add`

If your secret is a key passphrase, the neatest result is not to need sshc
for each command at all. `sshc ssh-add` hands the key to
[`ssh-agent`](#a-safer-alternative-ssh-agent) using the stored passphrase:

```console
$ sshc ssh-add                # or: sshc ssh-add ~/.ssh/id_ed25519
Identity added: /home/luke/.ssh/id_ed25519 (luke@laptop)
$ ssh w.go-2                  # plain ssh, git, scp, your editor: no prompt
```

It needs an agent to be running; see the ssh-agent section for starting one
(on Windows it is a service you enable once). Put `sshc ssh-add` in your shell
startup file and your key is unlocked in every session without typing.

### When something is off: `sshc doctor`

```console
$ sshc doctor

  ok       OpenSSH client: OpenSSH_9.6p1
  ok       sshc 0.6.1 at /home/luke/.local/bin/sshc
  PROBLEM  the shell hook is not loaded in this terminal
           -> add this line to your shell's startup file and open a new terminal:
              command -v sshc >/dev/null 2>&1 && eval "$(sshc --shell-init posix)"
  ok       config file: /home/luke/.local/bin/sshc.conf
  ok       credential store: the system keyring (Secret Service)
  note     ssh-agent is not reachable; only needed for "sshc ssh-add"

Warning: 1 problem(s) found; each has a suggested fix above.

```

It checks the OpenSSH version, whether more than one copy of sshc is on your
`PATH`, the shell hook, the config file and its permissions, the credential
store and whether saved entries are really in it, and ssh-agent. Run it
first when sshc does not behave as this page says.

### Moving a host to a key

`ssh-copy-id` installs your public key on the server, which normally costs one
last password prompt. With sshc it costs none, and afterwards the host no
longer needs a stored password at all:

```console
$ sshc ssh-copy-id w.go-2
$ ssh w.go-2                  # logs in with the key
```

## Storing passwords and passphrases

A secret can come from one of four places. sshc looks in all of them, and
`sshc list` shows what is where.

| Place | Set with | Lasts | Protection |
| ----- | -------- | ----- | ---------- |
| An environment variable | `sshc set -s` | until the terminal closes | in memory only |
| Your system's credential store | `sshc set` | until you `sshc unset` it | encrypted, tied to your login |
| Your password manager | `sshc set -c '<command>'` | sshc keeps nothing | whatever the manager provides |
| The config file, in plain text | `sshc set --plain`, or by hand | until you remove it | file permissions only |

### Environment variables

`SSHC_PASSWORD` is the active login password and `SSHC_PASSPHRASE` the active
key passphrase. They live in the terminal that set them and the programs it
starts, so different terminals can hold different values at the same time, and
nothing is written to disk.

`sshc set -s` sets them for you, asking for the value hidden:

| Command | Sets |
| ------- | ---- |
| `sshc set -s` | `SSHC_PASSWORD` |
| `sshc set -sp` | `SSHC_PASSPHRASE` |
| `sshc set -s -H w.go-2` | `SSHC_PASSWORD_W_GO_2`, for that one host |
| `sshc set -s -k id_ed25519` | `SSHC_PASSPHRASE_ID_ED25519`, for that one key |

A value set this way **stays in effect for the rest of that terminal session**.
Every `sshc` command you run there uses it until you set it again, remove it
with `sshc unset -s` (`-sp`, ...), or close the terminal - it is not asked for
again and does not expire. Other terminals, including ones you open later, are
unaffected and start without it. A variable also takes precedence over
anything saved, so plain `sshc set` does not change what a terminal with the
variable set will use.

#### The shell hook

A program cannot change the environment of the terminal that started it, so
`-s` works through a few lines of shell code that wrap the `sshc` command.
The same code sets up **tab completion** for host names, sshc's commands and
their options. It has to be loaded when your shell starts:

| Shell | Add this line to | Line |
| ----- | ---------------- | ---- |
| bash | `~/.bashrc` | `eval "$(sshc --shell-init bash)"` |
| zsh | `~/.zshrc` | `eval "$(sshc --shell-init zsh)"` |
| fish | `~/.config/fish/config.fish` | `sshc --shell-init fish \| source` |
| PowerShell | the file `$PROFILE` names | `sshc --shell-init powershell \| Out-String \| Invoke-Expression` |

The installer script and `sshc install` add the line for you. On Windows they
can only do that when PowerShell's script policy lets a profile load; if it
does not, they say so, and you run these two lines once:

```powershell
if (!(Test-Path $PROFILE)) { New-Item -Force -ItemType File $PROFILE | Out-Null }
Add-Content $PROFILE 'sshc --shell-init powershell | Out-String | Invoke-Expression'
```

If new PowerShell windows then say that running scripts is disabled, allow
your own profile with `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned`.
Open a new terminal afterwards. `sshc --shell-init` prints the hook if you
want to read it first; apart from completion, all it does is hand `sshc set`, `sshc unset` and
`sshc use` to the real program and apply the one variable change it asks
for.

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

### The credential store

Without `-s`, `sshc set` saves the secret so that it is there in every
terminal, in the place your operating system provides for exactly this:

| System | Store | Needs |
| ------ | ----- | ----- |
| Windows | Credential Manager | nothing; entries appear as `sshc:...` under *Windows Credentials* |
| macOS | the login Keychain | nothing; entries have the service name `sshc` |
| Linux | the Secret Service keyring (GNOME Keyring, KWallet, KeePassXC) | `secret-tool` from `libsecret-tools`, and a keyring running in your session |

There the secret is encrypted and tied to your login, and it is never written
to a file sshc owns. All three are exercised by the automated tests on every change, with a
real login through each on Linux, Windows and macOS.

Which option you give decides what is saved:

| Command | Saves |
| ------- | ----- |
| `sshc set` | the login password of the active profile |
| `sshc set -p` | the key passphrase of the active profile, tried for any key |
| `sshc set -H w.go-2` | the login password for that one host |
| `sshc set -k id_ed25519` | the passphrase for that one key (file name or full path) |

Add `-P NAME` to the first two to save into a profile other than the active
one. Each command asks for the value hidden; you can also put it at the end of
the command (`sshc set 'correct horse'`), with the caveats below.
`sshc unset` takes the same options and removes the entry, and `sshc list`
shows everything that is stored:

```console
$ sshc list

config file: /home/luke/.local/bin/sshc.conf
credential store: the system keyring (Secret Service)
active profile: default

Stored:
  [host w.go-2]                password    credential store
  [profile default]            passphrase  credential store

```

When the value is given on the command line, sshc clears the screen and
scrollback afterwards so it is not left on display. `--no-clear`, or
`clear_on_set = no` in the config file, turns that off. Clearing the screen
does not remove the command from your **shell history**, and the argument is
briefly visible to other users in the process list - leave the value off to
avoid both.

**Where there is no credential store** - a server, a container, a Linux
session without a keyring - `sshc set` says so and keeps the secret in the
config file in plain text instead. `sshc set --plain` asks for that
explicitly, and `SSHC_CREDENTIAL_STORE=off` turns the store off altogether.
An entry saved to the store on your desktop cannot be read from an SSH
session into the same machine if the keyring is not unlocked there; `sshc
check` tells you when that is the case.

### Password managers

Instead of storing a secret, sshc can ask the tool that already has it.
`sshc set -c` records a command; sshc runs it whenever the secret is needed
and uses the first line it prints.

```console
$ sshc set -c 'op read "op://Work/web server/password"'      # 1Password
$ sshc set -p -c 'pass show ssh/id_ed25519'                  # pass, for a key passphrase
$ sshc set -H w.go-2 -c 'bw get password w.go-2'             # Bitwarden, one host
```

The same options as for any `sshc set` decide which entry the command belongs
to. Any program works, as long as it prints the secret and nothing before it:
a cloud secrets service (`aws secretsmanager get-secret-value ...`), the
macOS `security` tool, or a script of your own. The examples above use each
tool's own syntax, which is theirs to document.

In the command, `%h` stands for the host being logged in to, `%u` for the
user and `%k` for the key file, so one entry can serve many hosts:

```console
$ sshc set -c 'pass show ssh/%h'
```

Things to know:

- The command is run directly, **not by a shell**: quotes group words, but
  there are no pipes, variables or `~`. Put anything more elaborate in a
  script and name the script.
- If your password manager needs unlocking, it asks you in its own way (a
  fingerprint, a window, a prompt) and sshc waits, for up to two minutes.
- `sshc check` and `sshc list` show the command but never run it.
- A command is not a secret, so it is written to the config file as it is.

### One-time codes

Some hosts ask for a verification code after the password. sshc can supply
it from a command, in the same way as from a password manager:

```console
$ sshc set -o -H w.go-2 -c 'oathtool --totp -b JBSWY3DPEHPK3PXP'
$ sshc set -o -H w.go-2 -c 'op item get "w.go-2" --otp'
```

A code is always fetched by a command (it changes every time), and always
for one named host: there is no "active" code. The wording of that second
question is chosen by the server, so sshc answers it only for a host you set
a code up for, and only when the question reads like a request for one -
"Verification code", "One-time password", "Passcode" and the like. Anything
else still goes to your terminal.

Keep in mind what a second factor is for: a command that produces the code
on the same machine as the stored password turns two factors back into one.

### Profiles

A profile is a named pair of login password and key passphrase - `work`,
`home`, one per customer. Exactly one is active, and it is the one sshc
offers and `sshc set` stores into.

```console
$ sshc set -P work            # store a password in the profile "work"
$ sshc use work               # this terminal now uses it
$ sshc use --save work        # make it the default for every terminal
$ sshc use                    # show the active profile and the others
$ sshc use --default          # this terminal: back to the default
```

`sshc use NAME` changes one terminal (through the shell hook, by setting
`SSHC_PROFILE`), so two terminals can work with two profiles side by side. For
a single command, set the variable just for it: `SSHC_PROFILE=home sshc
w.go-2`.

### Config file

The config file, `sshc.conf`, holds sshc's settings and the list of what is
saved. For a secret kept in the credential store it only records that the
entry exists:

```ini
# The active profile. $SSHC_PROFILE overrides this per shell.
profile = work

# A profile holds a login password, a key passphrase, or both.
[profile work]
password = @credential-store
passphrase = @credential-store

# One host's login password. Always wins over the active profile.
[host w.go-2]
password = @credential-store

# One key's passphrase. Always wins over the active profile.
[key id_ed25519]
passphrase = @credential-store

# Fetched from a password manager each time, by running this command.
[host db-prod]
password_command = op read "op://Work/db-prod/password"
```

`sshc set` and `sshc unset` maintain this file for you, so there is normally
no reason to open it. You can still edit it by hand: change the active
profile, set `clear_on_set = no`, or write a secret straight into it in plain
text (`password = hunter2`), which is what `sshc set --plain` does. Because it
may hold plain-text secrets, on Linux and macOS sshc refuses to read the file
unless it is owned by you with mode 600.

The file can also switch on a **log** of what sshc supplies:

```ini
log = yes
```

Every prompt sshc answers is then recorded in `sshc.log` next to the config
file (or at a path you give instead of `yes`): the time, what was asked for,
which entry answered it and through which tool - never the secret. It also
records a stored secret being rejected.

If you saved secrets before sshc used the credential store, or with
`--plain`, `sshc migrate` moves every plain-text entry into the store in one
go.

`sshc set` creates the file when it first needs it, next to the sshc
executable (`sshc init` creates just the template). If that location
is not writable, or sshc was installed by a package manager (Scoop, Homebrew,
winget - they replace that folder on every upgrade), the file goes in your
user config directory instead: `~/.config/sshc/` on Linux,
`~/Library/Application Support/sshc/` on macOS, `%AppData%\sshc\` on Windows.
`$SSHC_CONFIG` points somewhere else, and `sshc list` always shows which file
is in use.

Values run to the end of the line and are taken literally. The file is parsed
as data; the only thing sshc ever runs from it is a `password_command` or
`passphrase_command` that you put there.

### Which one is used

First match wins. The two columns never cross over:

|   | For a login password prompt | For a key passphrase prompt |
| - | --------------------------- | --------------------------- |
| 1 | `SSHC_PASSWORD_<HOST>` | `SSHC_PASSPHRASE_<KEYFILE>` |
| 2 | the `[host <name>]` entry (`sshc set -H`) | the `[key <name>]` entry (`sshc set -k`) |
| 3 | `SSHC_PASSWORD` | `SSHC_PASSPHRASE` |
| 4 | the active profile's password (`sshc set`) | the active profile's passphrase (`sshc set -p`) |

An entry in rows 2 and 4 is whatever you set it to: a value in the credential
store, a password manager command, or plain text.

A host entry can be written with the alias you type or with the real host
name, with or without `user@`. A key entry can be the key's file name or its
full path.

`sshc check <destination>` shows which sources would be used, without
connecting and without printing any secret:

```console
$ sshc check w.go-2

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
- **Offers a stored secret once.** If it is rejected, sshc says so and lets
  you type instead of repeating it and locking the account.
- **Never accepts a host key for you.** Your `StrictHostKeyChecking` setting is
  untouched.
- **Keeps saved secrets in your system's credential store**, encrypted and
  tied to your login, rather than in a file of its own.
- **Refuses a config file other users can read** (Linux and macOS: it must be
  owned by you with mode 600), since that file may hold plain-text secrets.
- **Keeps secrets off command lines and out of its own output.** No command
  prints a stored value back.
- **Supplies a one-time code only where you set one up**, for that host
  alone.
- **Runs a configured command directly, never through a shell**, and inserts
  the host, user and key names as whole words, so a hostile host name cannot
  add to the command.

What it cannot do:

- **Protect a secret from programs running as you.** sshc hands the secret to
  ssh without asking, so other software under your account can obtain it the
  same way - whichever of the three places it is kept in. The credential
  store protects against someone reading your disk, a backup or another
  account on the machine, not against malware in your own session.
- A `password_command` is run with your permissions by anything that can
  edit your config file. On Linux and macOS sshc insists that only you can;
  on Windows that rests on the folder's permissions.
- `sshc lock` does not change any of this: it stops sshc from supplying
  secrets, not other programs from reaching them. See
  [Locking sshc](#locking-sshc).
- A secret in an environment variable is readable by other processes running
  as you (and by root), like any environment variable. Do not add `SSHC_*` to
  `SendEnv` in your ssh config.
- A secret written into `sshc.conf` (`--plain`, by hand, or where there is no
  credential store) is plain text on disk. Never commit that file. On Windows
  it is protected by the folder's ACL rather than a mode check, so keep it
  somewhere under your user profile.
- A stored **key passphrase** deserves extra thought. The passphrase exists so
  that someone who copies your private key file still cannot use it. Stored in
  plain text on the same machine, it no longer protects against anyone who can
  read your files - it is then roughly as safe as a key with no passphrase.

### Locking sshc

`sshc lock` switches sshc off. While it is locked it supplies nothing - no
session variable, nothing from the credential store, no password manager
command, no one-time code - and every connection asks you, as plain ssh
would. `sshc unlock` switches it back on, after your device has verified that
it is you:

| System | `sshc unlock` asks for |
| ------ | ---------------------- |
| Windows | Windows Hello: your PIN, fingerprint or face |
| macOS | your account password |
| Linux | your account password |

```console
$ sshc lock

Locked!

sshc will not supply any stored password, passphrase or code until you run "sshc unlock",
which asks for your Windows Hello PIN, fingerprint or face. Connections will ask you instead.

$ sshc w.go-2

Note: sshc is locked, so nothing stored is being used. Run "sshc unlock" to switch it back on.

Enter passphrase for key 'C:\Users\Luke.Weaver/.ssh/id_ed25519':
```

To have it lock by itself when it has not been used for a while, set a time
in the config file - `30m`, `8h`, `2d`:

```ini
lock_after = 8h
```

`sshc list`, `sshc check` and `sshc doctor` show when sshc is locked.
`sshc lock` refuses to lock where the unlock could not work: on Windows
without Windows Hello set up, or when you are root.

**What the lock is, and is not.** It is a switch that sshc itself obeys. It
protects against:

- someone sitting down at your unlocked computer and running `sshc`;
- a script or scheduled job using your stored secrets when you did not mean
  it to;
- your own slip of the hand.

It does **not** protect against software running under your account. Such a
program has no need to go through sshc: it can read the credential store
directly, read a session variable, or delete the lock file. The lock does
not encrypt anything, and a secret that is stored stays exactly where it was.
Treat it like locking your screen, not like a safe.

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

## GitHub Actions

For deploying from a workflow to a host that only takes a password, this
repository is also an action that installs sshc on the runner:

```yaml
steps:
  - uses: actions/checkout@v4
  - uses: W-Industries-Luke/sshc@v0.6.1
  - run: |
      mkdir -p ~/.ssh && echo "$KNOWN_HOSTS" >> ~/.ssh/known_hosts
      sshc scp -r ./site deploy@example.com:/var/www
    env:
      SSHC_PASSWORD: ${{ secrets.DEPLOY_PASSWORD }}
      KNOWN_HOSTS: ${{ secrets.DEPLOY_KNOWN_HOSTS }}
```

It works on Linux, macOS and Windows runners, verifies the download against
the release's checksums, and takes an optional `version` input (default: the
latest release). Keep the password in a repository secret, and give the job
the server's host key as shown rather than turning host key checking off -
there is nobody to answer the "are you sure" question in a workflow.

## Notes

- `sftp -b batchfile` turns on ssh's batch mode, which disables password and
  passphrase prompts altogether. Add `-o BatchMode=no` before `-b` to use a
  stored secret in a batch.
- sshc adds one `ssh -V` and one `ssh -G` call before connecting (about 10 ms)
  to check the client version and resolve the destination.

## Troubleshooting

Start with `sshc doctor`: it checks for most of what follows and prints the
fix.

**It still asks me for the password.** Run `sshc check <destination>` with
the same arguments. If it says "no stored password", nothing matching is
stored: a value from `sshc set -s` does not carry over to new terminals, and
`sshc list` shows everything that is saved and which config file is in use.

Then read the prompt itself. `Enter passphrase for key ...` needs a stored
*passphrase* (`sshc set -p`), and `...'s password:` needs a stored
*password* (`sshc set`); having only the other kind is the most common reason
for still being asked. See [Password or passphrase?](#password-or-passphrase).
Otherwise the prompt may come from a [jump host](#jump-hosts), or be a
one-time code, which sshc leaves to you.

Setting `SSHC_DEBUG=1` makes sshc print what it decides at each step (never
the secret itself).

**`sshc -v` still shows the old version after an upgrade**, or a new option
just prints the usage text. First open a new terminal. If that does not help,
you have two copies installed and the older one comes first on your `PATH`.
List them:

```powershell
where.exe sshc        # Windows
```

```bash
which -a sshc         # Linux, macOS
```

Keep one. If the first one listed belongs to a package manager (a path
containing `scoop\shims` or `Cellar`), either upgrade it there
(`scoop update sshc`, `brew upgrade sshc`) and delete the other copy, or
uninstall it there (`scoop uninstall sshc`, `brew uninstall sshc`). The
installer script's copy lives in `%LOCALAPPDATA%\Programs\sshc` on Windows and
`~/.local/bin` elsewhere.

**"the stored password for ... was not accepted"** (or passphrase). It was
rejected, so sshc stopped offering it and let you type. Update the stored
value.

**"... is kept in the credential store, which cannot be used here".** The
entry was saved to the credential store, and this session cannot reach it -
typically an SSH login or a scheduled job on Linux, where no keyring is
unlocked. Set the variable for that session (`sshc set -s`), or store that
entry with `sshc set --plain`.

**"the password command of [...] failed".** sshc ran the command you set
with `sshc set -c` and it did not print a secret. Run the same command
yourself to see why: the password manager may be locked, signed out, or not
on your `PATH` in that session. Remember that it is not run by a shell.

**Tab does not complete host names.** Completion comes with the
[shell hook](#the-shell-hook), so the hook has to be loaded and the terminal
opened after it was added. Hosts are read from `~/.ssh/config`; patterns such
as `*.example.com` are not offered. zsh also needs its completion system on
(`autoload -Uz compinit && compinit` in `~/.zshrc`, before the sshc line).

**`sshc each` says a host failed with exit status 255.** ssh could not log
in. Most often the host has no stored secret or its host key is not known
yet, and `each` does not ask; run `sshc <host>` once on its own.

**"sshc is locked".** sshc was switched off with `sshc lock`, or locked
itself after the time set as `lock_after`. Run `sshc unlock`. If your device
cannot verify you at all - no Windows Hello any more, an account without a
password - the message from `sshc unlock` names the lock file; deleting it
unlocks sshc.

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
password is a failed login rather than a hang. A script started from a
terminal still has one; set `SSHC_NO_PROMPT=1` to get the same behaviour
there.

## Development

The repository is laid out as follows:

```
main.go             entry point; everything else is in internal/sshc
internal/sshc/      the program, one file per concern (see the comment at the
                    top of cli.go for a guide), with its unit tests alongside
install.sh          one-line installers for Linux/macOS and Windows; they are
install.ps1         fetched by URL, so they stay at the top level
internal/e2e/       end-to-end tests that run on every platform: the real ssh
                    client against an SSH server built into the test
test/e2e.sh         further end-to-end tests against a real sshd in Docker
                    (scp, sftp, rsync, ssh-copy-id, the Linux keyring)
docs/sshc.1         the man page
packaging/          where sshc is published and how to update each channel
action.yml          the GitHub Action that installs sshc on a runner
.github/workflows/  CI: unit tests on Linux, Windows and macOS, plus e2e
```

```console
$ make test     # go vet, unit tests and the built-in end-to-end tests
$ make e2e      # real ssh/scp/sftp against an sshd in Docker
$ make dist     # cross-compile release binaries
```

Neither test suite contacts a real server or uses real credentials: both
start a throwaway SSH server with a made-up user for the length of the test.

Releases are built and published by `.github/workflows/release.yml` when a
version tag is pushed; see [packaging/README.md](packaging/README.md).
[SECURITY.md](SECURITY.md) covers reporting a vulnerability and the threat
model, and [CHANGELOG.md](CHANGELOG.md) what changed in each version.

## License

MIT
