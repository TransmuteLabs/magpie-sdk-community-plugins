"""Select original OBS bytes without writing the shared host directory."""

import hashlib
import json
from pathlib import Path
import tarfile


def frozen_sdk(packet):
    shared = Path("/tmp/magpie-obs-policy-fix-20261115")
    manifest = shared / "candidate-01.json"
    archive = shared / "candidate-01.tar.gz"
    manifest_digest = hashlib.sha256(manifest.read_bytes()).hexdigest()
    archive_digest = hashlib.sha256(archive.read_bytes()).hexdigest()
    assert manifest_digest == "8724cf734d51d881f6d88561eceefeaecf9f722e1bc4e1a078fcb8f010cccdd4"
    assert archive_digest == "9015f443e9ea051f26c7399d6fa0292e4229d1d8f15e332d2b5fc9f4f85f4208"
    expected = json.loads(manifest.read_text())
    destination = packet / "sdk-obs-original"
    if not destination.exists():
        destination.mkdir()
        with tarfile.open(archive) as source:
            for member in source:
                if not member.isfile():
                    continue
                assert member.name in expected
                data = source.extractfile(member).read()
                assert hashlib.sha256(data).hexdigest() == expected[member.name]["sha256"]
                target = destination / member.name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(data)
                target.chmod(member.mode & 0o755)
    for name, record in expected.items():
        assert hashlib.sha256((destination / name).read_bytes()).hexdigest() == record["sha256"], name
    drift = {}
    for name, record in expected.items():
        actual = shared / "candidate-01" / name
        digest = hashlib.sha256(actual.read_bytes()).hexdigest() if actual.is_file() else None
        if digest != record["sha256"]:
            drift[name] = {"expected": record["sha256"], "observed": digest}
    receipt = dict(source=str(destination), files=len(expected), manifestSHA256=manifest_digest,
                   archiveSHA256=archive_digest, sharedDirectoryDrift=drift)
    (packet / "sdk-freeze.json").write_text(json.dumps(receipt, indent=2) + "\n")
    return destination, receipt
