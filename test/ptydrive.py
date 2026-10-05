#!/usr/bin/env python3
"""Run a command on a pseudo-terminal and answer it, for tests that need a
real terminal (an interactive login, a hidden prompt).

    ptydrive.py 'WAIT-FOR=SEND' ['WAIT-FOR=SEND' ...] -- command [args...]

Each time WAIT-FOR appears in the output, SEND and a newline are typed.
Everything the command prints is copied to stdout. The command is killed if
it has not finished after 40 seconds.
"""
import os
import pty
import select
import signal
import sys
import time

split = sys.argv.index("--")
steps = [s.split("=", 1) for s in sys.argv[1:split]]
command = sys.argv[split + 1:]

pid, fd = pty.fork()
if pid == 0:
    os.execvp(command[0], command)
seen = b""
deadline = time.time() + 40
timed_out = True
while time.time() < deadline:
    ready, _, _ = select.select([fd], [], [], 0.3)
    if not ready:
        continue
    try:
        data = os.read(fd, 4096)
    except OSError:
        timed_out = False
        break
    if not data:
        timed_out = False
        break
    seen += data
    sys.stdout.write(data.decode(errors="replace"))
    sys.stdout.flush()
    if steps and steps[0][0].encode() in seen:
        seen = b""
        os.write(fd, (steps[0][1] + "\n").encode())
        steps.pop(0)
if timed_out:
    os.kill(pid, signal.SIGKILL)
_, status = os.waitpid(pid, 0)
sys.exit(124 if timed_out else os.waitstatus_to_exitcode(status))
