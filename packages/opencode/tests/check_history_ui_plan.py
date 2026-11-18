import argparse
import hashlib
import json
from pathlib import Path


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("host", type=Path)
    parser.add_argument("capture", type=Path)
    parser.add_argument("archive", type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1] / "src"
    plan = json.loads((root / "testdata/history-ui-plan.json").read_text())
    captured = json.loads(args.capture.read_text())
    assert captured["tree"] == plan["host"]["tree"]
    assert digest(args.archive) == plan["host"]["archiveSHA256"]
    results = []
    for seam in plan["seams"]:
        path = args.host / seam["path"]
        expected = captured["files"][seam["path"]]["sha256"]
        before = digest(path)
        assert before == expected, seam["path"]
        lines = path.read_text().splitlines()
        matches = [index + 1 for index, line in enumerate(lines) if line == seam["anchor"]]
        assert len(matches) == 1, seam
        assert digest(path) == before, seam["path"]
        results.append({**seam, "line": matches[0], "sha256": before})
    assert len({case["id"] for case in plan["cases"]}) == len(plan["cases"])
    assert all(case["state"] == "planned" for case in plan["cases"])
    assert plan["runtimeBehaviorVerified"] is False
    print(json.dumps({
        "kind": "read-only source-seam audit, not browser/runtime acceptance",
        "hostTree": captured["tree"],
        "hostArchiveSHA256": plan["host"]["archiveSHA256"],
        "acceptedModuleCommit": plan["acceptedModuleCommit"],
        "verifiedAnchors": len(results),
        "verifiedFiles": len({entry["path"] for entry in results}),
        "plannedAcceptanceCases": len(plan["cases"]),
        "runtimeBehaviorVerified": False,
        "sourceUnchanged": True,
        "seams": results,
    }, indent=2))


if __name__ == "__main__":
    main()
