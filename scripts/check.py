#!/usr/bin/env python3
"""Run offline development checks with a private user configuration."""
import argparse
from pathlib import Path
import subprocess
import sys
import tempfile

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _workspace import ROOT, build_environment, packages, sdk_source


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("package", choices=["all", *packages()])
    parser.add_argument("--sdk", type=Path)
    parser.add_argument("--race", action="store_true")
    args = parser.parse_args()
    sdk = sdk_source(args.sdk)
    subprocess.run([sys.executable, "-m", "unittest", "discover", "-s", str(ROOT / "tests")], check=True)
    for name, spec in packages(args.package).items():
        package = ROOT / "packages" / name
        with build_environment(package, sdk) as (env, _), tempfile.TemporaryDirectory(prefix="magpie-plugin-check-") as home:
            # Preserve existing offline Go caches before changing HOME.
            for key in ("GOROOT", "GOPATH", "GOCACHE", "GOMODCACHE"):
                env[key] = subprocess.check_output(["go", "env", key], env=env, cwd=package / "src", text=True).strip()
            env.update(HOME=home, USERPROFILE=home, XDG_CONFIG_HOME=home + "/config",
                       XDG_CACHE_HOME=home + "/cache", XDG_DATA_HOME=home + "/data",
                       XDG_STATE_HOME=home + "/state", APPDATA=home + "/appdata",
                       LOCALAPPDATA=home + "/localappdata", PYTHONDONTWRITEBYTECODE="1")
            print(f"Checking {name}", flush=True)
            sources = [str(path) for path in sorted((package / "src").rglob("*.go"))]
            unformatted = subprocess.check_output(["gofmt", "-l", *sources], env=env, text=True)
            if unformatted:
                raise SystemExit("Run gofmt on:\n" + unformatted)
            subprocess.run(["go", "vet", "./..."], env=env, cwd=package / "src", check=True)
            subprocess.run(["go", "test", "-count=1", "-timeout=10m", *( ["-race"] if args.race else []), "./..."],
                           env=env, cwd=package / "src", check=True)
            for check in spec["checks"]:
                substitutions = {"{python}": sys.executable, "{sdk}": str(sdk)}
                command = [substitutions.get(part, part) for part in check["command"]]
                subprocess.run(command, env=env, cwd=package / check.get("cwd", "."), check=True)


if __name__ == "__main__":
    main()
