#!/usr/bin/env bash
# End-to-end test: runs sshc against a throwaway sshd in Docker.
# Usage: test/e2e.sh path/to/sshc
set -u

[ $# -eq 1 ] || { echo "usage: $0 path/to/sshc" >&2; exit 2; }
work=$(mktemp -d)
name=sshc-e2e-$$
trap 'docker rm -f "$name" >/dev/null 2>&1; rm -rf "$work"' EXIT

# The config file is looked up next to the binary, so test a private copy.
mkdir "$work/bin"
cp "$1" "$work/bin/sshc" || exit 2
PATH=$work/bin:$PATH
unset SSHC_PASSWORD SSHC_PROFILE SSHC_CONFIG
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
check "explicit ssh subcommand"      0 "hi"       sshc ssh -F cfg w.go-2 echo hi
check "remote exit status is kept"   7 ""         sshc -F cfg w.go-2 'exit 7'
check "keyboard-interactive"         0 "hi"       sshc -F cfg kbd echo hi

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

check "nothing stored: plain ssh"    0 "OpenSSH"  sshc -V
check "--init next to the binary"    0 "$work/bin/sshc.conf" sshc --init
check "--init does not overwrite"    1 "already exists" sshc --init
printf 'profile = work\n[profile work]\npassword = %s\n[profile bad]\npassword = nope\n[host jump@127.0.0.1]\npassword = jump-pw\n' "$PW" >bin/sshc.conf
check "config file profile"          0 "hi"       sshc -F cfg w.go-2 echo hi
check "config file host section"     0 "hi"       sshc -F cfg inner echo hi
check "SSHC_PROFILE selects a profile" 255 "Permission denied" env SSHC_PROFILE=bad sshc -F cfg w.go-2 true
check "SSHC_PASSWORD beats the profile" 0 "hi"    env SSHC_PROFILE=bad "SSHC_PASSWORD=$PW" sshc -F cfg w.go-2 echo hi
chmod 644 bin/sshc.conf
check "world-readable config is refused" 1 "chmod 600" sshc -F cfg w.go-2 true

leftover=$(find "${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}" -maxdepth 1 -name 'sshc-*' -newer cfg 2>/dev/null | grep -vc "^$work$")
check "scratch directories are removed" 0 "" test "$leftover" = 0

[ "$fails" = 0 ] && echo "all passed" || echo "$fails failed"
[ "$fails" = 0 ]
