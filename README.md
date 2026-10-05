# sshc

`ssh`, `scp`, `sftp`, `rsync` and `ssh-copy-id` that type your stored
password, or the passphrase of your SSH key, for you.

```console
$ sshc set -s                 # type it once, hidden; kept for this terminal only
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

Then open a new terminal and run `sshc -v`. A `winget` package has been
submitted and is awaiting Microsoft's review; until it is accepted,
`winget install` will not find sshc. Manual downloads, building from source
and what each method does are covered in [Getting started](#2-install-sshc).

## About

sshc is a thin wrapper around the OpenSSH client you already have. It is a
single executable with no runtime dependencies - no `sshpass`, no `expect` -
and runs on Linux, macOS and Windows.

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
  - [Moving a host to a key](#moving-a-host-to-a-key)
- [Storing passwords and passphrases](#storing-passwords-and-passphrases)
  - [Environment variables](#environment-variables)
    - [The shell hook](#the-shell-hook)
  - [The credential store](#the-credential-store)
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
that `sshc set -s` needs. `SHA256SUMS` on the release page lets you verify the
download.

**Upgrading.** Use the method you installed with: run the installer script
again, `scoop update sshc`, or `brew upgrade sshc`. Then open a new terminal,
so that the [shell hook](#the-shell-hook) of the new version is loaded.
`sshc --install` warns if it finds another copy ahead of it on your `PATH`.

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
up on Linux and macOS and is two lines to add on Windows.

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
| `sshc install` | copy sshc to a per-user folder and put it on `PATH` |
| `sshc init` | create a config file template |
| `sshc shell-init [shell]` | print the [shell hook](#the-shell-hook) |
| `sshc help`, `sshc version` | also `-h` and `-v`, when given on their own |

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

### Moving a host to a key

`ssh-copy-id` installs your public key on the server, which normally costs one
last password prompt. With sshc it costs none, and afterwards the host no
longer needs a stored password at all:

```console
$ sshc ssh-copy-id w.go-2
$ ssh w.go-2                  # logs in with the key
```

## Storing passwords and passphrases

A secret can live in one of three places. sshc looks in all of them, and
`sshc list` shows what is where.

| Place | Set with | Lasts | Protection |
| ----- | -------- | ----- | ---------- |
| An environment variable | `sshc set -s` | until the terminal closes | in memory only |
| Your system's credential store | `sshc set` | until you `sshc unset` it | encrypted, tied to your login |
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
They have to be loaded when your shell starts:

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
want to read it first; all it does is hand `sshc set` and `sshc unset` to the
real program and apply the one variable change it asks for.

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
to a file sshc owns. The Linux store has been used end to end; the Windows
and macOS ones are newer and so far covered mainly by automated tests, so
please open an issue if one misbehaves.

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
```

`sshc set` and `sshc unset` maintain this file for you, so there is normally
no reason to open it. You can still edit it by hand: change the active
profile, set `clear_on_set = no`, or write a secret straight into it in plain
text (`password = hunter2`), which is what `sshc set --plain` does. Because it
may hold plain-text secrets, on Linux and macOS sshc refuses to read the file
unless it is owned by you with mode 600.

Switch profile for one shell with `export SSHC_PROFILE=home`, or for one
command with `SSHC_PROFILE=home sshc w.go-2`.

`sshc set` creates the file when it first needs it, next to the sshc
executable (`sshc init` creates just the template). If that location
is not writable, or sshc was installed by a package manager (Scoop, Homebrew,
winget - they replace that folder on every upgrade), the file goes in your
user config directory instead: `~/.config/sshc/` on Linux,
`~/Library/Application Support/sshc/` on macOS, `%AppData%\sshc\` on Windows.
`$SSHC_CONFIG` points somewhere else, and `sshc list` always shows which file
is in use.

Values run to the end of the line and are taken literally. The file is parsed
as data and nothing in it is ever executed.

### Which one is used

First match wins. The two columns never cross over:

|   | For a login password prompt | For a key passphrase prompt |
| - | --------------------------- | --------------------------- |
| 1 | `SSHC_PASSWORD_<HOST>` | `SSHC_PASSPHRASE_<KEYFILE>` |
| 2 | a saved `[host <name>]` entry (`sshc set -H`) | a saved `[key <name>]` entry (`sshc set -k`) |
| 3 | `SSHC_PASSWORD` | `SSHC_PASSPHRASE` |
| 4 | the active profile's saved password (`sshc set`) | the active profile's saved passphrase (`sshc set -p`) |

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

What it cannot do:

- **Protect a secret from programs running as you.** sshc hands the secret to
  ssh without asking, so other software under your account can obtain it the
  same way - whichever of the three places it is kept in. The credential
  store protects against someone reading your disk, a backup or another
  account on the machine, not against malware in your own session.
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
