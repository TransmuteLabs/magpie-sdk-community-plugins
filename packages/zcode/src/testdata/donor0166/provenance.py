import hashlib
import json
from pathlib import Path
import subprocess
import sys


def main():
    root = Path(sys.argv[1]).resolve()
    source = Path(__file__).resolve().parents[2]
    subprocess.run(["python3", "-I", "testdata/parity/provenance.py", str(root)],
                   cwd=source, check=True, timeout=60)
    lock = json.loads((source.parent / "docs/source-lock.json").read_bytes())
    entry = next(item for item in lock["sources"] if item["id"] == "community-zcode-0166")
    checked = {}
    for name, expected in entry["checkedFiles"].items():
        actual = hashlib.sha256((root / "donor" / name).read_bytes()).hexdigest()
        assert actual == expected, (name, actual, expected)
        checked[name] = actual
    version = json.loads((root / "donor/packages/zcode/package.json").read_bytes())["version"]
    assert version == entry["version"] == "0.1.66"
    identity = json.loads((root / "sdk-source.json").read_bytes())
    sdk = next(item for item in lock["sources"] if item["id"] == "sdk-concurrency-quota9")
    assert identity["snapshotSHA256"] == sdk["snapshotSHA256"]
    assert identity["archiveSHA256"] == sdk["archiveSHA256"]
    for name, expected in sdk["checkedFiles"].items():
        assert hashlib.sha256((root / "magpie" / name).read_bytes()).hexdigest() == expected, name
    report = {"donorCommit": entry["commit"], "donorVersion": version,
              "checkoutCommit": entry["checkoutCommit"], "observedOriginHEAD": entry["observedOriginHEAD"],
              "updates": entry["updates"], "checkedDonorFiles": checked,
              "sdkSnapshotSHA256": sdk["snapshotSHA256"], "actualClientExecution": False}
    print(json.dumps(report, indent=2), flush=True)


if __name__ == "__main__":
    main()
