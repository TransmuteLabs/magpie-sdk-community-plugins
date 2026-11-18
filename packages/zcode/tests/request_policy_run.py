import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--stage", required=True)
    parser.add_argument("--source", type=Path)
    parser.add_argument("--sdk", type=Path, default=Path("/tmp/magpie-obs-policy-fix-20261115/candidate-01"))
    parser.add_argument("--guarded", action="store_true")
    parser.add_argument("--namespace", action="store_true")
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    root = args.root.resolve()
    own = str(Path(__file__).resolve())
    arguments = ["--root", str(root), "--stage", args.stage, *args.command]
    if args.source is not None:
        arguments = ["--source", str(args.source.resolve()), *arguments]
    arguments = ["--sdk", str(args.sdk.resolve()), *arguments]
    if not args.namespace and not args.guarded:
        os.execvp("unshare", [
            "unshare", "--user", "--map-current-user", "--keep-caps", "--net",
            sys.executable, own, "--namespace", *arguments,
        ])
    if args.namespace:
        subprocess.run(["ip", "link", "set", "lo", "up"], check=True)
        subprocess.run(["ip", "link", "add", "fixture0", "type", "dummy"], check=True)
        subprocess.run(["ip", "address", "add", "192.0.2.1/32", "dev", "fixture0"], check=True)
        subprocess.run(["ip", "link", "set", "fixture0", "up"], check=True)
        os.execvp("setpriv", [
            "setpriv", "--bounding-set=-all", "--inh-caps=-all", "--ambient-caps=-all",
            sys.executable,
            "/tmp/magpie-upstream-signing-closure.ofRrrBdz/magpie-verification-network.py",
            str(root / (args.stage + "-network.json")),
            sys.executable, own, "--guarded", *arguments,
        ])
    home = root / (args.stage + "-home")
    home.mkdir(mode=0o700)
    caches = json.loads(Path(
        "/tmp/magpie-sdk-v11-usbox-niq306rt/go-env-external-toolchain.json"
    ).read_text())
    environment = dict(
        caches, PATH=caches["GOROOT"] + "/bin:/Users/maratkarimov/.local/bin:/Users/maratkarimov/.bun/bin:/usr/bin:/bin:/usr/sbin:/sbin",
        HOME=str(home), USERPROFILE=str(home), TMPDIR=str(home),
        XDG_CONFIG_HOME=str(home / "config"), XDG_CACHE_HOME=str(home / "cache"),
        XDG_DATA_HOME=str(home / "data"), XDG_STATE_HOME=str(home / "state"),
        APPDATA=str(home / "appdata"), LOCALAPPDATA=str(home / "localappdata"),
        GOENV="off", GOTELEMETRY="off", GOPROXY="off", GOSUMDB="off",
        GOTOOLCHAIN="local", GOWORK="off", CGO_ENABLED="1", GOMAXPROCS="2",
        GOOS="linux", GOARCH="amd64", PYTHONDONTWRITEBYTECODE="1",
    )
    package = (args.source or root / "source") / "packages/zcode"
    modfile = home / "verify.mod"
    modfile.write_text((package / "src/go.mod").read_text().replace(
        "../../../../magpie", str(args.sdk.resolve())
    ))
    modfile.with_suffix(".sum").write_bytes((package / "src/go.sum").read_bytes())
    environment["GOFLAGS"] = "-mod=readonly -modfile=" + str(modfile)
    command = args.command[1:] if args.command[:1] == ["--"] else args.command
    with (root / (args.stage + ".log")).open("x") as output:
        result = subprocess.run(command, cwd=package / "src", env=environment,
                                stdout=output, stderr=subprocess.STDOUT, timeout=600)
    receipt = dict(command=command, exit=result.returncode, home=str(home),
                   toolchain=caches, source=str(package), sdk=str(args.sdk.resolve()),
                   logSHA256=hashlib.sha256((root / (args.stage + ".log")).read_bytes()).hexdigest())
    (root / (args.stage + "-result.json")).write_text(json.dumps(receipt, indent=2) + "\n")
    print(json.dumps(receipt), flush=True)
    sys.exit(result.returncode)


if __name__ == "__main__":
    main()
