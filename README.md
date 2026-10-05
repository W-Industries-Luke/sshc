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

## Install

With Go 1.26 or newer:

```console
$ go install github.com/W-Industries-Luke/sshc@latest
```

or from a clone:

```console
$ make install            # builds and copies to ~/.local/bin/sshc
```

On Windows, build with `go build .` and put `sshc.exe` in a directory on your
`PATH`. sshc uses the OpenSSH client that ships with Windows 10 and 11.

Requires OpenSSH 8.5 or newer (`ssh -V`).

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

A host that happens to share a name with one of the tools is reachable as
`sshc ssh scp`.

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

## Development

```console
$ make test     # go vet + unit tests
$ make e2e      # real ssh/scp/sftp against an sshd in Docker
$ make dist     # cross-compile release binaries
```

## License

MIT
