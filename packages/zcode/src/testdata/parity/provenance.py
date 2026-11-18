import hashlib
import json
from pathlib import Path
import sys


def main():
    source = Path(__file__).resolve().parents[2]
    root = Path(sys.argv[1]).resolve()
    lock = json.loads((source.parent / "docs/source-lock.json").read_bytes())
    result = {}
    for identity, directory in [("vendor-current-audit", "vendor-pinned"),
                                ("community-zcode", "community-pinned")]:
        entry = next(item for item in lock["sources"] if item["id"] == identity)
        checked = {}
        for name, expected in entry["checkedFiles"].items():
            location = root / directory / name
            actual = hashlib.sha256(location.read_bytes()).hexdigest()
            assert actual == expected, (identity, name, actual, expected)
            checked[name] = actual
        result[identity] = {"commit": entry["commit"], "version": entry["version"],
                            "checkedFiles": checked}
    vendor = root / "vendor-pinned"
    desktop = json.loads((vendor / "package.json").read_bytes())["version"]
    cli = json.loads((vendor / "apps/zcode-cli/package.json").read_bytes())["version"]
    assert desktop == "3.18.2" and cli == "0.17.1"
    community = json.loads((root / "community-pinned/packages/zcode/package.json").read_bytes())["version"]
    assert community == "0.1.61"
    result.update(desktop=desktop, cli=cli, community=community, actualClientExecution=False)
    print(json.dumps(result, indent=2), flush=True)


if __name__ == "__main__":
    main()
