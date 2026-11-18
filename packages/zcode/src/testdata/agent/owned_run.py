import argparse
import json
import os
from pathlib import Path
import signal
import subprocess
import time


def identity(pid):
    fields = Path("/proc", str(pid), "stat").read_text().rsplit(")", 1)[1].split()
    return {"pid": pid, "startTicks": int(fields[19])}


def stop_group(process, expected, grace):
    if identity(process.pid) != expected:
        raise RuntimeError("fixture leader identity changed")
    if os.getpgid(process.pid) != process.pid or os.getsid(process.pid) != process.pid:
        raise RuntimeError("fixture does not own a separate process session")
    os.killpg(process.pid, signal.SIGTERM)
    deadline = time.monotonic() + grace
    while time.monotonic() < deadline:
        time.sleep(min(0.01, max(0, deadline - time.monotonic())))
    os.killpg(process.pid, signal.SIGKILL)
    process.wait(timeout=3)


def run_owned(command, *, timeout, grace=0.2, **options):
    if not 0 < timeout <= 180 or not 0 <= grace <= 3:
        raise ValueError("fixture timeout and cleanup grace must be bounded and positive")
    started = time.monotonic()
    process = subprocess.Popen(command, start_new_session=True, **options)
    owner = identity(process.pid)
    timed_out = False
    try:
        while os.waitid(os.P_PID, process.pid, os.WEXITED | os.WNOHANG | os.WNOWAIT) is None:
            if time.monotonic() - started >= timeout:
                timed_out = True
                break
            time.sleep(0.01)
    finally:
        stop_group(process, owner, grace)
    return {
        "exit": 124 if timed_out else process.returncode, "childExit": process.returncode,
        "timedOut": timed_out, "ownedGroupStopped": True, "owner": owner,
        "seconds": round(time.monotonic() - started, 3),
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--result", type=Path, required=True)
    parser.add_argument("--timeout", type=float, default=180)
    parser.add_argument("command", nargs=argparse.REMAINDER)
    arguments = parser.parse_args()
    command = arguments.command
    if command[:1] == ["--"]:
        command = command[1:]
    if not command:
        parser.error("an explicit fixture command is required")
    with arguments.result.open("x") as output:
        arguments.result.chmod(0o600)
        result = run_owned(command, timeout=arguments.timeout)
        output.write(json.dumps(result, sort_keys=True, indent=2) + "\n")
        output.flush()
    raise SystemExit(result["exit"])


if __name__ == "__main__":
    main()
