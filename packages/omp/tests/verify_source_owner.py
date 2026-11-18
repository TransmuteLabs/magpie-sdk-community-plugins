"""Run an existing host-owner Source probe against this module's distribution.

The probe is supplied by the host verification lane. It grants its own private
root and calls ordinary host session APIs; it is not plugin implementation.
"""

import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile


def verify(owner_probe, distribution, fixture_module=None, repeats=1):
    owner_probe = Path(owner_probe).resolve(strict=True)
    distribution = Path(distribution).resolve(strict=True)
    fixture_module = Path(fixture_module or Path(__file__).resolve().parents[1] / "src").resolve(strict=True)
    results = []
    for iteration in range(repeats):
        with tempfile.TemporaryDirectory(prefix="omp-source-owner-") as temporary:
            home = Path(temporary)
            bindir = home / "bin"
            bindir.mkdir()
            fixture_cli = bindir / "omp"
            fixture_cli.write_text("#!/bin/sh\nprintf 'omp-fixture\\n'\n")
            fixture_cli.chmod(0o700)
            env = {
                "HOME": str(home), "USERPROFILE": str(home), "APPDATA": str(home / "app"),
                "PATH": str(bindir) + ":/usr/bin:/bin", "TMPDIR": str(home),
                "XDG_CONFIG_HOME": str(home / "config"), "XDG_DATA_HOME": str(home / "data"),
                "XDG_CACHE_HOME": str(home / "cache"), "XDG_STATE_HOME": str(home / "state"),
                "LANG": "C.UTF-8", "HTTP_PROXY": "http://127.0.0.1:9",
                "HTTPS_PROXY": "http://127.0.0.1:9", "ALL_PROXY": "http://127.0.0.1:9",
            }
            completed = subprocess.run(
                [str(owner_probe), str(distribution), str(fixture_module)],
                cwd=home, env=env, text=True, capture_output=True, timeout=120,
            )
            if completed.returncode:
                raise RuntimeError("host-owner Source probe failed: " + completed.stderr)
            result = json.loads(completed.stdout)
            required = (
                "ordinaryFind", "ordinaryList", "ordinaryTranscript", "ordinarySearch",
                "ordinaryStats", "hostResume", "consentRevocation", "offRevocation",
                "journalUnchanged",
                "moduleReadDeniedWithFreshBinding", "moduleRecordAllowed",
            )
            if not all(result.get(field) is True for field in required):
                raise AssertionError("host-owner probe did not prove the ordinary Source path")
            results.append({"iteration": iteration, "exit": 0, "proof": result})
    return results


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("owner_probe", help="compiled probe from the matching host preview")
    parser.add_argument("distribution")
    parser.add_argument("--fixtures")
    parser.add_argument("--repeat", type=int, default=1)
    arguments = parser.parse_args()
    if arguments.repeat < 1:
        parser.error("--repeat must be positive")
    print(json.dumps(verify(arguments.owner_probe, arguments.distribution,
                            arguments.fixtures, arguments.repeat), indent=2))
