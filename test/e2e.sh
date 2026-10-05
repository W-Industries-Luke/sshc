#!/usr/bin/env bash
# End-to-end test: runs sshc against a throwaway sshd in Docker.
# Usage: test/e2e.sh path/to/sshc
set -u

[ $# -eq 1 ] || { echo "usage: $0 path/to/sshc" >&2; exit 2; }
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
name=sshc-e2e-$$
trap 'docker rm -f "$name" >/dev/null 2>&1; rm -rf "$work"' EXIT

# The config file is looked up next to the binary, so test a private copy.
mkdir "$work/bin"
cp "$1" "$work/bin/sshc" || exit 2
PATH=$work/bin:$PATH
unset SSHC_PASSWORD SSHC_PASSPHRASE SSHC_PROFILE SSHC_CONFIG
# Keep the tests out of the developer's real keyring; the credential store
# gets its own section below.
export SSHC_CREDENTIAL_STORE=off
# ... and out of the developer's real lock state.
export SSHC_STATE_DIR=$work/state
cd "$work" || exit 2

PW='s3cret pass#1'
docker build -q -t sshc-e2e - >/dev/null <<DOCKER || exit 2
FROM alpine:3.20
RUN apk add --no-cache openssh-server-pam rsync && ssh-keygen -A \\
 && adduser -D luke && echo 'luke:$PW' | chpasswd \\
 && adduser -D jump && echo 'jump:jump-pw' | chpasswd
# Port 22 takes password authentication, port 23 keyboard-interactive only.
RUN printf 'Port 22\nPasswordAuthentication yes\nKbdInteractiveAuthentication no\nSubsystem sftp internal-sftp\n' >/etc/ssh/pw.conf \\
 && printf 'Port 23\nUsePAM yes\nPasswordAuthentication no\nKbdInteractiveAuthentication yes\n' >/etc/ssh/kbd.conf
CMD ["/bin/sh", "-c", "/usr/sbin/sshd.pam -f /etc/ssh/kbd.conf && exec /usr/sbin/sshd.pam -D -e -f /etc/ssh/pw.conf"]
DOCKER
docker run -d --name "$name" -p 127.0.0.1::22 -p 127.0.0.1::23 sshc-e2e >/dev/null || exit 2
port() { docker port "$name" "$1" | head -n1 | sed 's/.*://'; }

cat >cfg <<CFG
Host *
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  LogLevel ERROR
  PubkeyAuthentication no
  IdentityAgent none
Host w.go-2
  HostName 127.0.0.1
  Port $(port 22)
  User luke
Host kbd
  HostName 127.0.0.1
  Port $(port 23)
  User luke
Host inner
  HostName localhost
  User luke
  ProxyJump jump@127.0.0.1:$(port 22)
CFG

fails=0
# check DESCRIPTION EXPECTED-STATUS EXPECTED-OUTPUT-SUBSTRING command...
check() {
	local desc=$1 want_rc=$2 want_out=$3 out rc
	shift 3
	out=$("$@" 2>&1 </dev/null)
	rc=$?
	if [ "$rc" = "$want_rc" ] && [[ $out == *"$want_out"* ]]; then
		echo "ok   $desc"
	else
		echo "FAIL $desc (status $rc, want $want_rc)"
		echo "$out" | sed 's/^/       /'
		fails=$((fails + 1))
	fi
}

for _ in $(seq 20); do
	SSHC_PASSWORD=$PW sshc -F cfg w.go-2 true >/dev/null 2>&1 && break
	sleep 0.5
done

export SSHC_PASSWORD=$PW
check "ssh by alias"                 0 "hi luke"  sshc -F cfg w.go-2 'echo hi $(whoami)'
check "ssh by full connection string" 0 "hi luke" sshc -F cfg -p "$(port 22)" luke@127.0.0.1 'echo hi $(whoami)'
check "-v with a host stays ssh -v"  0 "debug1: Authenticating to" sshc -v -F cfg -o LogLevel=DEBUG w.go-2 echo hi
check "explicit ssh subcommand"      0 "hi"       sshc ssh -F cfg w.go-2 echo hi
check "remote exit status is kept"   7 ""         sshc -F cfg w.go-2 'exit 7'
check "keyboard-interactive"         0 "hi"       sshc -F cfg kbd echo hi

# A start directory and an entry command, for interactive logins only.
check "set a start directory and entry command" 0 "Updated!" sshc set -H w.go-2 -d /tmp -e 'export ENTRY_RAN=yes'
if command -v python3 >/dev/null; then
	login() { python3 "$here/ptydrive.py" '$ =echo "at=$PWD ran=${ENTRY_RAN-no}"; exit' -- "$@"; }
	check "an interactive login starts there" 0 "at=/tmp ran=yes" login sshc -F cfg w.go-2
	check "--no-entry logs in plainly"   0 "at=/home/luke ran=no" login sshc --no-entry -F cfg w.go-2
	check "the picker's kind of login too" 0 "at=/tmp ran=yes" login sshc ssh -F cfg w.go-2
else
	echo "skip interactive login tests (no python3)"
fi
check "a command of your own is left alone" 0 "/home/luke" sshc -F cfg w.go-2 pwd
check "check shows what a login runs" 0 "an interactive login runs: cd '/tmp' && export ENTRY_RAN=yes" sshc check -F cfg w.go-2
check "list shows the login settings" 0 "On login:" sshc list
check "unset the start directory"    0 "directory of [host w.go-2]" sshc unset -H w.go-2 -d
check "unset the entry command"      0 "entry of [host w.go-2]" sshc unset -H w.go-2 --entry
check "nothing left to unset"        1 "has no entry set" sshc unset -H w.go-2 -e
check "dir and entry need a host"    1 "belong to one host" sshc set -d /tmp
rm -f bin/sshc.conf

mkdir -p up/d && echo payload >up/d/f
check "scp -r upload"                0 ""         sshc scp -F cfg -r ./up w.go-2:copy
check "scp -r download"              0 ""         sshc scp -F cfg -r w.go-2:copy ./down
check "scp round trip content"       0 "payload"  cat down/d/f
echo 'ls copy/d' >batch
check "sftp batch"                   0 "copy/d/f" sshc sftp -F cfg -o BatchMode=no -b batch w.go-2

if command -v rsync >/dev/null; then
	check "rsync upload"                 0 ""         sshc rsync -a -e 'ssh -F cfg' ./up/ w.go-2:synced/
	check "rsync download"               0 ""         sshc rsync -a --rsh='ssh -F cfg' w.go-2:synced/ ./synced
	check "rsync round trip content"     0 "payload"  cat synced/d/f
else
	echo "skip rsync (not installed)"
fi
if command -v ssh-copy-id >/dev/null; then
	# ssh-copy-id keeps scratch files in ~/.ssh; give it one of its own.
	ssh-keygen -q -t ed25519 -N '' -f key
	mkdir -m 700 -p home/.ssh
	check "ssh-copy-id installs the key" 0 "key(s) added: 1" \
		env "HOME=$work/home" sshc ssh-copy-id -i key.pub -F cfg w.go-2
	check "the installed key logs in"    0 "hi" \
		env -u SSHC_PASSWORD ssh -F cfg -i key -o PubkeyAuthentication=yes -o BatchMode=yes w.go-2 echo hi
else
	echo "skip ssh-copy-id (not installed)"
fi

# A passphrase-protected key, installed for luke over the password login.
ssh-keygen -q -t ed25519 -N 'key phrase#2' -f enckey
sshc -F cfg w.go-2 'mkdir -p -m 700 .ssh && cat >>.ssh/authorized_keys && chmod 600 .ssh/authorized_keys' <enckey.pub
keyssh() { sshc -F cfg -i enckey -o PubkeyAuthentication=yes -o PasswordAuthentication=no "$@"; }
check "key passphrase from SSHC_PASSPHRASE" 0 "hi" \
	env -u SSHC_PASSWORD 'SSHC_PASSPHRASE=key phrase#2' bash -c "$(declare -f keyssh); keyssh w.go-2 echo hi"
check "key passphrase by key file name" 0 "hi" \
	env -u SSHC_PASSWORD SSHC_PASSPHRASE=wrong 'SSHC_PASSPHRASE_ENCKEY=key phrase#2' bash -c "$(declare -f keyssh); keyssh w.go-2 echo hi"
check "scp with a key passphrase"    0 ""   \
	env -u SSHC_PASSWORD 'SSHC_PASSPHRASE=key phrase#2' sshc scp -F cfg -i enckey -o PubkeyAuthentication=yes -o PasswordAuthentication=no ./up/d/f w.go-2:via-key
check "rejected passphrase is not resent" 255 "passphrase for key enckey (from environment variable SSHC_PASSPHRASE) was not accepted" \
	env -u SSHC_PASSWORD SSHC_PASSPHRASE=wrong bash -c "$(declare -f keyssh); keyssh w.go-2 true"
check "the login password is not used as a passphrase" 255 "Permission denied" \
	bash -c "$(declare -f keyssh); keyssh w.go-2 true"
check "--check reports the key"      0 "enckey: passphrase from environment variable SSHC_PASSPHRASE" \
	env SSHC_PASSPHRASE=x sshc --check -F cfg -i enckey w.go-2

# lock: while locked nothing stored is supplied.
if [ "$(id -u)" != 0 ]; then
	check "lock switches sshc off"       0 "Locked!"  sshc lock
	check "a locked sshc supplies nothing" 255 "sshc is locked" sshc -F cfg -o BatchMode=yes w.go-2 echo hi
	check "list shows the lock"          0 "LOCKED"   sshc list
	check "set says it is locked"        0 "sshc is locked. This is stored" sshc set -P locktest x
	check "unset says it is locked"      0 "sshc is locked" sshc unset -P locktest
	check "unlock needs to verify you"   1 "still locked" sshc unlock
	rm -f "$SSHC_STATE_DIR/locked" bin/sshc.conf
	check "unlocked again"               0 "hi"       sshc -F cfg w.go-2 echo hi
fi

# ssh-add: unlock the key into an agent with the stored passphrase.
eval "$(ssh-agent -s)" >/dev/null
check "ssh-add with a stored passphrase" 0 "Identity added" \
	env -u SSHC_PASSWORD 'SSHC_PASSPHRASE=key phrase#2' sshc ssh-add enckey
check "the agent now holds the key"  0 "ED25519" ssh-add -l
ssh-add -D >/dev/null 2>&1
check "ssh-add reports a rejected passphrase" 1 "was not accepted" \
	env -u SSHC_PASSWORD SSHC_PASSPHRASE=wrong sshc ssh-add enckey
ssh-agent -k >/dev/null 2>&1

# run: any program that uses ssh underneath.
check "run answers a key passphrase" 0 "via-run" \
	env -u SSHC_PASSWORD 'SSHC_PASSPHRASE=key phrase#2' sshc run ssh -F cfg -i enckey -o PubkeyAuthentication=yes -o PasswordAuthentication=no w.go-2 echo via-run
check "run -d offers the active password" 0 "via-run" sshc run -d 127.0.0.1 -- ssh -F cfg w.go-2 echo via-run
check "run without -d does not"      255 "Permission denied" sshc run ssh -F cfg w.go-2 echo via-run

# each: one command on several hosts.
check "each runs on every host"      0 "kbd    | hello" sshc each -F cfg w.go-2 kbd -- echo hello
check "each labels each host"        0 "w.go-2 | hello" sshc each -j 1 -F cfg w.go-2 kbd -- echo hello
check "each reports the hosts that failed" 1 "1 of 2 hosts failed" \
	sshc each -F cfg -o ConnectTimeout=3 w.go-2 nosuch.invalid -- true
check "each never asks on the terminal" 1 "kbd (exit status 255)" \
	env SSHC_PASSWORD=wrong sshc each -F cfg kbd -- true

check "active password is not offered to a jump host" 255 "jump@127.0.0.1: Permission denied" \
	sshc -F cfg inner echo hi
check "jump host with its own variable" 0 "hi" \
	env SSHC_PASSWORD_127_0_0_1=jump-pw sshc -F cfg inner echo hi
check "host variable by alias wins"  0 "hi" \
	env SSHC_PASSWORD=wrong "SSHC_PASSWORD_W_GO_2=$PW" sshc -F cfg w.go-2 echo hi

check "rejected password is not resent" 255 "was not accepted" env SSHC_PASSWORD=wrong sshc -F cfg w.go-2 true
check "--check names the source, not the password" 0 "environment variable SSHC_PASSWORD" sshc --check -F cfg w.go-2
if sshc --check -F cfg w.go-2 2>&1 | grep -qF "$PW"; then
	echo "FAIL --check printed the password"
	fails=$((fails + 1))
fi
unset SSHC_PASSWORD

mkdir -p sshhome/.ssh && printf 'Host alpha beta\n HostName 127.0.0.1\nHost *.wild\n User x\n' >sshhome/.ssh/config
check "hosts lists the ssh config"   0 "alpha" env "HOME=$work/sshhome" sshc hosts
check "completion offers hosts"      0 "beta"  env "HOME=$work/sshhome" sshc --complete 1 sshc b
check "completion offers commands"   0 "unset" env "HOME=$work/sshhome" sshc --complete 1 sshc un
check "completion offers options"    0 "--passphrase" env "HOME=$work/sshhome" sshc --complete 2 sshc set --pa
check "no arguments without a terminal prints the help" 0 "Usage: sshc" sshc
check "-v alone is the sshc version"  0 "sshc 0."   sshc -v
check "word forms of the commands"   0 "sshc 0."   sshc version
check "check as a word"              0 "w.go-2 -> 127.0.0.1" env SSHC_PASSWORD=x sshc check -F cfg w.go-2
check "-h alone is the sshc help"     0 "Usage: sshc" sshc -h
check "nothing stored: plain ssh"    0 "OpenSSH"  sshc -V
check "--init next to the binary"    0 "$work/bin/sshc.conf" sshc --init
check "--init does not overwrite"    1 "already exists" sshc --init
printf 'profile = work\n[profile work]\npassword = %s\n[profile bad]\npassword = nope\n[host jump@127.0.0.1]\npassword = jump-pw\n' "$PW" >bin/sshc.conf
check "config file profile"          0 "hi"       sshc -F cfg w.go-2 echo hi
check "config file host section"     0 "hi"       sshc -F cfg inner echo hi
check "SSHC_PROFILE selects a profile" 255 "Permission denied" env SSHC_PROFILE=bad sshc -F cfg w.go-2 true
check "SSHC_PASSWORD beats the profile" 0 "hi"    env SSHC_PROFILE=bad "SSHC_PASSWORD=$PW" sshc -F cfg w.go-2 echo hi
check "set saves the active profile" 0 "Updated!" sshc set 'new pw'
check "set kept the other entries"   0 "password = jump-pw" cat bin/sshc.conf
check "set replaced the password"    0 "password of [profile work], in plain text in" sshc set -- "$PW"
check "set from a pipe"              0 "Updated!" sh -c "printf '%s\\n' '$PW' | sshc set --profile piped"
check "the piped profile logs in"    0 "hi"       env SSHC_PROFILE=piped sshc -F cfg w.go-2 echo hi
check "set --host"                   0 "password of [host kbd], in plain text" sshc set --host kbd "$PW"
# A command in place of a stored value, as for a password manager.
printf '#!/bin/sh\necho "$1" >>"%s/pm.log"\necho "%s"\n' "$work" "$PW" >bin/fakepm && chmod +x bin/fakepm
check "set --command"                0 "fetched by running" sshc set -H w.go-2 -c "fakepm 'asked for %u at %h'"
check "check describes the command without running it" 0 "by running fakepm" sshc check -F cfg w.go-2
check "check did not run it"         1 ""         test -e pm.log
check "connect through the command"  0 "hi"       env SSHC_PROFILE=bad sshc -F cfg w.go-2 echo hi
check "the command got the host and user" 0 "asked for luke at 127.0.0.1" cat pm.log
check "list shows the command, it is not a secret" 0 "from the command: fakepm" sshc list
check "unset removes the command"    0 "password of [host w.go-2]" sshc unset -H w.go-2
check "set -H (short form)"          0 "password of [host short], in plain text" sshc set -H short x
check "list names entries, not values" 0 "[host kbd]" sshc list
if sshc list | grep -qF "$PW"; then
	echo "FAIL list printed a password"
	fails=$((fails + 1))
fi
check "unset removes an entry"       0 "password of [host short]" sshc unset -H short
check "unset of a missing entry"     1 "no stored password for [host short]" sshc unset -H short
check "unset left the others alone"  0 "hi"       sshc -F cfg kbd echo hi
check "set --passphrase"              0 "passphrase of [profile work], in plain text" sshc set -p 'key phrase#2'
check "set kept the profile password" 0 "hi"      sshc -F cfg w.go-2 echo hi
check "passphrase from the profile"  0 "hi"       bash -c "$(declare -f keyssh); keyssh w.go-2 echo hi"
check "set --key"                    0 "passphrase of [key enckey], in plain text" sshc set -k enckey 'key phrase#2'
check "key section beats the profile" 0 "hi"      sh -c "sshc set --passphrase wrong >/dev/null && $(declare -f keyssh); keyssh w.go-2 echo hi"
check "set refuses mixed targets"    1 "use one"  sshc set --host kbd --key enckey x
check "set rejects stray options"    1 "Usage: sshc set" sshc set --bogus x
chmod 644 bin/sshc.conf
check "world-readable config is refused" 1 "chmod 600" sshc -F cfg w.go-2 true

# --install into a scratch home directory.
mkdir fakehome && chmod 600 bin/sshc.conf
inst() { env "HOME=$work/fakehome" SHELL=/bin/bash "$@"; }
# The real credential store, where this machine can run one.
if command -v secret-tool >/dev/null && command -v gnome-keyring-daemon >/dev/null && command -v dbus-run-session >/dev/null; then
	cat >keyring.sh <<KEYRING
eval "\$(printf '\n' | gnome-keyring-daemon --unlock --components=secrets 2>/dev/null)"
sleep 1
unset SSHC_CREDENTIAL_STORE
printf '%s\n' '$PW' | sshc set -H kbd || exit 1
grep -A1 '^\[host kbd\]' bin/sshc.conf
sshc -F cfg kbd echo logged-in
sshc unset -H kbd && ! secret-tool lookup service sshc account 'host kbd/password'
KEYRING
	check "credential store: set, connect, unset" 0 "logged-in" dbus-run-session -- bash keyring.sh
	check "credential store keeps it out of the file" 0 "password = @credential-store" \
		dbus-run-session -- bash keyring.sh
	cat >migrate.sh <<MIGRATE
eval "\$(printf '\n' | gnome-keyring-daemon --unlock --components=secrets 2>/dev/null)"
sleep 1
SSHC_CREDENTIAL_STORE=off sshc set -H plainhost plain-secret >/dev/null
unset SSHC_CREDENTIAL_STORE
sshc migrate
grep -c plain-secret bin/sshc.conf
secret-tool lookup service sshc account 'host plainhost/password'
sshc unset -H plainhost >/dev/null
MIGRATE
	check "migrate moves plain text into the store" 0 "plain-secret" dbus-run-session -- bash migrate.sh
else
	echo "skip credential store (needs gnome-keyring, libsecret-tools and dbus)"
fi

check "--install copies the binary"  0 "Installed sshc" inst sshc --install
check "installed binary runs"        0 "sshc "    fakehome/.local/bin/sshc --version
check "--install moved the config"   0 "profile = work" cat fakehome/.local/bin/sshc.conf
check "--install added a PATH line"  0 'export PATH="$HOME/.local/bin:$PATH"' cat fakehome/.bashrc
check "--install added the shell hook" 0 'eval "$(sshc --shell-init posix)"' cat fakehome/.bashrc
check "a stale hook replaces itself" 0 "[$(sshc version | cut -d' ' -f2)]" \
	bash -c 'eval "$(sshc --shell-init bash)"; export SSHC_HOOK_VERSION=0.1.0; sshc set -s x 2>/dev/null; echo "[$SSHC_HOOK_VERSION]"'
mkdir -p hookhome
check "install under the hook reloads it" 0 "already using it" \
	env "HOME=$work/hookhome" SHELL=/bin/bash bash -c 'eval "$(sshc --shell-init bash)"; sshc install "$HOME/hooked" 2>&1'
check "use switches the profile of one terminal" 0 "[work]" \
	bash -c 'eval "$(sshc --shell-init bash)"; sshc use work 2>/dev/null; echo "[$SSHC_PROFILE]"'
check "use without the hook explains" 1 "needs the sshc shell hook" sshc use work
check "doctor reports on the setup"  1 "the shell hook is not loaded" sshc doctor
check "doctor is content with the hook loaded" 0 "no problems found" \
	bash -c 'eval "$(sshc --shell-init bash)"; sshc doctor'
check "set -s through the hook"      0 "hi" \
	env -u SSHC_PASSWORD "PW=$PW" bash -c 'eval "$(sshc --shell-init bash)"; sshc set -s "$PW" 2>/dev/null; sshc -F cfg -o PubkeyAuthentication=no kbd echo hi'
check "unset -s through the hook"    0 "[]" \
	bash -c 'eval "$(sshc --shell-init bash)"; sshc set -sp x 2>/dev/null; sshc unset -sp 2>/dev/null; echo "[${SSHC_PASSPHRASE-}]"'
check "set --session writes no file"  1 "" \
	env "SSHC_CONFIG=$work/none.conf" bash -c 'eval "$(sshc --shell-init bash)"; sshc set --session x 2>/dev/null; test -e "$SSHC_CONFIG"'
check "set --session without the hook" 1 "needs the sshc shell hook" sshc set --session x
mkdir -p shadow && printf '#!/bin/sh\necho old\n' >shadow/sshc && chmod +x shadow/sshc
check "--install warns about an older copy on PATH" 0 "another copy of sshc comes first" \
	env "HOME=$work/fakehome" SHELL=/bin/bash "PATH=$work/shadow:$work/fakehome/.local/bin:$PATH" fakehome/.local/bin/sshc --install
check "--install is quiet when it is first on PATH" 0 "already on your PATH" \
	env "HOME=$work/fakehome" SHELL=/bin/bash "PATH=$work/fakehome/.local/bin:$work/shadow:$PATH" fakehome/.local/bin/sshc --install
check "--install twice is harmless"  0 "already installed" inst fakehome/.local/bin/sshc --install
check "PATH line is not duplicated"  0 "1"        grep -c 'export PATH' fakehome/.bashrc
check "--install sees PATH already set" 0 "already on your PATH" \
	env "HOME=$work/fakehome" "PATH=$work/fakehome/.local/bin:$PATH" fakehome/.local/bin/sshc --install

leftover=$(find "${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}" -maxdepth 1 -name 'sshc-*' -newer cfg 2>/dev/null | grep -vc "^$work$")
check "scratch directories are removed" 0 "" test "$leftover" = 0

[ "$fails" = 0 ] && echo "all passed" || echo "$fails failed"
[ "$fails" = 0 ]
