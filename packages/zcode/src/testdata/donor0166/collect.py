import hashlib
import json
import os
from pathlib import Path
import re
import sys
import zipfile


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    root, before = (Path(argument).resolve() for argument in sys.argv[1:3])
    source = root / "module"
    inputs = json.loads((root / "module-final2-input.json").read_bytes())
    assert digest(root / "module-final2-input.tar.gz") == inputs["archiveSHA256"]
    for name, expected in inputs["files"].items():
        assert digest(source / name) == expected, name
    sdk = json.loads((root / "sdk-source.json").read_bytes())
    for name, expected in sdk["files"].items():
        path = root / "magpie" / name
        if "symlink" in expected:
            assert path.is_symlink() and os.readlink(path) == expected["symlink"], name
        else:
            assert digest(path) == expected["sha256"], name
        if "mode" in expected:
            assert path.lstat().st_mode & 0o777 == expected["mode"], name
    report = {"moduleInputFiles": len(inputs["files"]),
              "moduleInputInventorySHA256": digest(root / "module-final2-input.json"),
              "moduleInputArchiveSHA256": inputs["archiveSHA256"],
              "sdkSnapshotSHA256": sdk["snapshotSHA256"], "sdkFilesUnchanged": len(sdk["files"]),
              "receipts": [], "logs": {}}
    receipts = [(before, name) for name in [
        "before-port", "discovery-trial-before-port", "donor-oracle", "donor-causal"]]
    receipts += [(root, "accepted-" + name) for name in [
        "static", "module", "donor-race10", "causal", "provenance", "package", "installed", "owners"]]
    for directory, label in receipts:
        receipt = json.loads((directory / (label + "-result.json")).read_bytes())
        isolation = receipt["isolation"]
        assert isolation["uid"] == isolation["euid"] == 1000
        assert len(isolation["capabilities"]) == 5 and not any(isolation["capabilities"].values())
        assert isolation["interfaces"] == ["lo"]
        assert receipt["sdkFilesBefore"] == receipt["sdkFilesAfter"] == len(sdk["files"])
        for result in receipt["results"]:
            log = directory / (result["label"] + ".log")
            assert digest(log) == result["logSHA256"], log
            assert result["exit"] == result["expectedExit"], result
            text = log.read_text()
            assert "WARNING: DATA RACE" not in text and "[build failed]" not in text
            if "-json" in result["command"]:
                counts = dict.fromkeys(["pass", "fail", "skip"], 0)
                for line in text.splitlines():
                    event = json.loads(line)
                    if event.get("Test") and event.get("Action") in counts:
                        counts[event["Action"]] += 1
                assert counts == result["counts"], result
            if result["expectedExit"]:
                assert result["counts"]["fail"] > 0 and "SyntaxError" not in text, result
            report["logs"][str(log)] = digest(log)
        report["receipts"].append(receipt)
    for runtime in ["node", "bun"]:
        text = (root / ("accepted-package-" + runtime + "-ui.log")).read_text()
        if runtime == "node":
            assert re.search(r"^# pass 31$", text, re.MULTILINE)
            assert re.search(r"^# fail 0$", text, re.MULTILINE)
        else:
            assert re.search(r"^\s*31 pass$", text, re.MULTILINE)
            assert re.search(r"^\s*0 fail$", text, re.MULTILINE)
    report["ui"] = {"nodePass": 31, "bunPass": 31}
    artifact = source / "dist/magpie-plugin_linux_amd64.zip"
    with zipfile.ZipFile(artifact) as archive:
        manifest = json.loads(archive.read("magpie-plugin.json"))
        assert manifest["version"] == "0.13.1"
        assert manifest["contract"]["version"] == {"major": 1, "minor": 9}
        assert manifest["window"] and len(manifest["agents"]) == 1
        assert [provider["id"] for provider in manifest["providers"]] == ["zcode"]
        assert manifest["providers"][0]["maxConcurrency"] == 3
        assert manifest["providers"][0]["maxAccounts"] == 2
        names = archive.namelist()
        assert len(names) == 19 and len([name for name in names if name.startswith("lib/")]) == 1
        assert not any(name.startswith(("node_modules/", "packages/", "runtime/")) for name in names)
        library = hashlib.sha256(archive.read(manifest["entry"])).hexdigest()
        assert library == digest(root / "full-artifact" / manifest["entry"])
    installed = json.loads((root / "installed-proof.json").read_bytes())
    assert installed["passed"] and installed["ordinaryExecutions"] == 11
    assert len(installed["executionDiagnostics"]) == 11
    assert installed["artifactSHA256"] == digest(artifact)
    assert installed["productionLibrarySHA256"] == library
    assert installed["hostSHA256"] == digest(root / "magpie-host")
    assert installed["vendorIsLoopbackFixture"] and not installed["actualClientPresent"]
    assert installed["unmodifiedCanonicalPackageInstallListInspectDescribeOff"]
    assert installed["unsupportedMonetaryFieldsRemainUnknown"]
    assert installed["failedTerminalRetainsUsage"] and installed["finalMeasuredZeroReplacesEarlierCounters"]
    report["installed"] = installed
    compatibility = source / "dist/compat-sdk14/magpie-plugin_linux_amd64.zip"
    with zipfile.ZipFile(compatibility) as archive:
        assert json.loads(archive.read("magpie-plugin.json"))["version"] == "0.7.2"
    report.update(artifactSHA256=digest(artifact), nativeLibrarySHA256=library,
                  artifactBytes=artifact.stat().st_size, compatibilitySHA256=digest(compatibility),
                  hostSHA256=digest(root / "magpie-host"),
                  sourceLockSHA256=digest(source / "source-lock.json"),
                  historicalAuthSHA256=digest(source / "AUTH-WEB-EVIDENCE.md"),
                  actualClientExecution=False, finalCommittedHostAcceptance=False)
    target = root / "donor0166-acceptance.json"
    target.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({"acceptanceSHA256": digest(target), "sourceFiles": len(inputs["files"]),
                      "sdkFiles": len(sdk["files"]), "artifactSHA256": digest(artifact),
                      "compatibilitySHA256": digest(compatibility), "logCount": len(report["logs"])}))


if __name__ == "__main__":
    main()
