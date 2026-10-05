# sshc

`ssh`, `scp`, `sftp`, `rsync` and `ssh-copy-id` that type your stored password for you.

```console
$ export SSHC_PASSWORD        # set once per shell, see below
$ sshc w.go-2                 # no prompt
$ sshc scp -r ./site w.go-2:/var/www
```

sshc is a thin wrapper around the OpenSSH client you already have. It is a
single executable with no runtime dependencies - no `sshpass`, no `expect` -
and runs on Linux, macOS and Windows.

> SSH keys are still the better answer wherever you are allowed to use them.
> sshc is for the hosts where you are not.

## How it works

OpenSSH has a hook, `SSH_ASKPASS`, for a program that answers its prompts.
sshc starts the real `ssh` with itself registered as that program, and answers
the password prompt when ssh calls back. Because ssh does the asking:

- every option of the wrapped tool works, because sshc never rewrites your arguments;
- `~/.ssh/config` aliases, `ProxyJump`, port forwards, etc. behave as usual;
- the password is never on a command line, where `ps` would show it.

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

There are no prebuilt downloads yet, so you need [Go](https://go.dev/dl/) 1.26
or newer to build it.

**Linux and macOS**

```console
$ go install github.com/W-Industries-Luke/sshc@latest
```

This puts `sshc` in `~/go/bin`. If `sshc --version` is then "command not
found", add that directory to your `PATH`:

```console
$ echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.bashrc    # or ~/.zshrc
```

Or, from a clone of this repository, `make install` builds it and copies it to
`~/.local/bin`.

**Windows** (PowerShell)

```powershell
go install github.com/W-Industries-Luke/sshc@latest
sshc --version
```

The Go installer already puts `%USERPROFILE%\go\bin` on your `PATH`; open a
new terminal if `sshc` is not found straight away.

> Windows support is new. The Windows build passes its tests in CI, but has
> seen far less real use than the Linux one - please open an issue if
> something misbehaves.

### 3. Give your host a short name (optional)

sshc uses your `~/.ssh/config` (`%USERPROFILE%\.ssh\config` on Windows) like
ssh does, so an alias saves typing the full connection string:

```
Host w.go-2
    HostName 203.0.113.7
    User luke
    Port 22
```

### 4. Store the password

For the current terminal only - nothing is written to disk:

```bash
# bash / zsh
read -rs SSHC_PASSWORD && export SSHC_PASSWORD
```

```powershell
# PowerShell 7
$env:SSHC_PASSWORD = Read-Host -MaskInput 'Password'
```

Type the password and press Enter; nothing is shown.

To have it available in every terminal instead, save it in the
[config file](#config-file):

```console
$ sshc set
New password:
Again:
Updated!
```

### 5. Check, then connect

```console
$ sshc --check w.go-2
config file: none
w.go-2 -> 203.0.113.7 (user luke)
  password from environment variable SSHC_PASSWORD
$ sshc w.go-2
```

`--check` does not connect and never prints the password. The first time you
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

If no password is stored anywhere, sshc simply runs the tool.

## Storing passwords

### Environment variables (preferred)

`SSHC_PASSWORD` is the active password. It lives in the shell that set it and
the programs that shell starts, so different terminals can hold different
passwords at the same time.

Set it without it landing in your shell history:

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

### Config file

`sshc set` saves a password in the config file, creating the file if needed:

```console
$ sshc set                        # asks for it, hidden - the safest form
$ sshc set 'correct horse'        # or give it directly
$ sshc set --profile home         # a named profile
$ sshc set --host w.go-2          # one host only
```

A program cannot change the environment of the shell that started it, so
`sshc set` always writes to the file; it is the persistent counterpart of
`SSHC_PASSWORD`, which still wins in a shell where it is set.

When the password is given on the command line, sshc clears the screen and
scrollback afterwards so it is not left on display. `--no-clear`, or
`clear_on_set = no` in the config file, turns that off. Clearing the screen
does not remove the command from your **shell history**, and the argument is
briefly visible to other users in the process list - use plain `sshc set` to
avoid both.

You can also edit the file by hand.
`sshc --init` creates `sshc.conf` next to the sshc executable (or in your user
config directory if that location is not writable). `$SSHC_CONFIG` points
somewhere else.

```ini
# The active profile. $SSHC_PROFILE overrides this per shell.
profile = work

[profile work]
password = correct horse battery staple

[profile home]
password = hunter2

# One host. Always wins over the active profile.
[host w.go-2]
password = something else
```

Switch profile for one shell with `export SSHC_PROFILE=home`, or for one
command with `SSHC_PROFILE=home sshc w.go-2`.

Values run to the end of the line and are taken literally. The file is parsed
as data and nothing in it is ever executed.

### Which password is used

First match wins:

1. `SSHC_PASSWORD_<HOST>`
2. `[host <name>]` in the config file
3. `SSHC_PASSWORD`
4. the active `[profile]` in the config file

A host entry can be written with the alias you type or with the real host
name, with or without `user@`.

`sshc --check <destination>` shows which source would be used, without
connecting and without printing the password:

```console
$ sshc --check w.go-2
config file: none
w.go-2 -> 203.0.113.7 (user luke)
  password from environment variable SSHC_PASSWORD
```

### Jump hosts

The active password (3 and 4 above) is only ever offered to the destination
you named. A jump host gets a password only if it has its own host entry, so
your password for one machine is never handed to another one on the way:

```console
$ export SSHC_PASSWORD_BASTION_EXAMPLE_COM=...
$ sshc -J bastion.example.com w.go-2
```

Use the jump host's real host name here; only command-line destinations are
matched by alias.

## Safety

What sshc does:

- **Answers only login password prompts**, and only for the host that the
  local ssh client says is asking. Host key confirmations, key passphrases,
  one-time codes and password-change prompts go to your terminal as usual.
  A server cannot word a prompt to obtain a different host's password.
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
- On Windows the config file is protected by the folder's ACL rather than a
  mode check, so keep it somewhere under your user profile.

## Notes

- `sftp -b batchfile` turns on ssh's batch mode, which disables password
  authentication altogether. Add `-o BatchMode=no` before `-b` to use a stored
  password in a batch.
- sshc adds one `ssh -V` and one `ssh -G` call before connecting (about 10 ms)
  to check the client version and resolve the destination.

## Troubleshooting

**It still asks me for the password.** Run `sshc --check <destination>` with
the same arguments. If it says "no stored password", the variable is not set
in this terminal (it does not carry over to new ones) or the config file is
not where sshc looks - the first line of the output shows which file is in
use. If a source is listed but you are still prompted, the prompt is probably
not for that host's login password: it may come from a
[jump host](#jump-hosts), or be a key passphrase or a one-time code, which sshc
leaves to you.

**"the stored password for ... was not accepted".** The server rejected it, so
sshc stopped offering it and let you type. Update the stored password.

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

```console
$ make test     # go vet + unit tests
$ make e2e      # real ssh/scp/sftp against an sshd in Docker
$ make dist     # cross-compile release binaries
```

## License

MIT
