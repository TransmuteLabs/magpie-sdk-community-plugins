#!/usr/bin/env python3
import argparse
import hashlib
import json
import os
from pathlib import Path
import stat


def git_object(kind, data):
    return hashlib.sha1(kind + b" " + str(len(data)).encode() + b"\0" + data).digest()


def main():
    parser = argparse.ArgumentParser(description="Verify a read-only physical host packet against the source lock.")
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    module = Path(__file__).resolve().parent.parent
    lock = json.loads((module / "docs/source-lock.json").read_bytes())
    pin = next(source for source in lock["sources"] if source["id"] == "sdk-tested-host")
    root = args.root.resolve()
    receipt_body = (root / "source.json").read_bytes()
    receipt = json.loads(receipt_body)
    snapshot = pin["snapshot"]
    for name, expected in (
        ("source.json", snapshot["receiptSHA256"]),
        ("source.tar.gz", snapshot["archiveSHA256"]),
        ("magpie", pin["binary"]["sha256"]),
    ):
        if hashlib.sha256((root / name).read_bytes()).hexdigest() != expected:
            raise SystemExit("Host packet differs: " + name)
    capture_head = snapshot.get("captureHead", pin["commit"])
    if receipt["tree"] != pin["tree"] or receipt["head_at_capture"] != capture_head:
        raise SystemExit("Host physical tree or baseline differs")
    if receipt["archive_sha256"] != snapshot["archiveSHA256"]:
        raise SystemExit("Host receipt archive differs")
    source = root / "source"
    actual_files = {str(path.relative_to(source)) for path in source.rglob("*")
        if path.is_file() or path.is_symlink()}
    if actual_files != set(receipt["files"]) or len(actual_files) != snapshot["paths"]:
        raise SystemExit("Host physical inventory differs")
    tree = {}
    permission_differences = 0
    for name, entry in receipt["files"].items():
        path = source / name
        body = path.read_bytes()
        if hashlib.sha256(body).hexdigest() != entry["sha256"]:
            raise SystemExit("Host source differs: " + name)
        mode_bits = stat.S_IMODE(path.stat().st_mode)
        if mode_bits & 0o111 != entry["mode"] & 0o111:
            raise SystemExit("Host executable mode differs: " + name)
        permission_differences += mode_bits != entry["mode"]
        node = tree
        parts = Path(name).parts
        for part in parts[:-1]:
            node = node.setdefault(part, {})
        mode = b"120000" if path.is_symlink() else b"100755" if path.stat().st_mode & 0o100 else b"100644"
        data = os.fsencode(path.readlink()) if path.is_symlink() else body
        node[parts[-1]] = (mode, git_object(b"blob", data))

    def encode(node):
        entries = []
        for name, child in sorted(node.items(),
            key=lambda item: os.fsencode(item[0] + "/" if isinstance(item[1], dict) else item[0])):
            mode, digest = (b"40000", encode(child)) if isinstance(child, dict) else child
            entries.append(mode + b" " + os.fsencode(name) + b"\0" + digest)
        return git_object(b"tree", b"".join(entries))

    physical_tree = encode(tree).hex()
    if physical_tree != pin["tree"]:
        raise SystemExit("Recomputed host tree differs: " + physical_tree)
    identity = hashlib.sha256(json.dumps({name: entry["sha256"] for name, entry in receipt["files"].items()},
        sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    if identity != snapshot["sourceIdentitySHA256"]:
        raise SystemExit("Host source identity differs")
    result = {
        "root": str(root), "physicalTree": physical_tree, "paths": len(actual_files),
        "captureHead": capture_head, "committedHost": pin["commit"], "committedHostTree": pin["commitTree"],
        "physicalTreeIsAuthoritative": True, "committedTreeMatchesPhysicalTree": pin["commitTree"] == physical_tree,
        "sourceIdentitySHA256": identity, "receiptSHA256": hashlib.sha256(receipt_body).hexdigest(),
        "archiveSHA256": snapshot["archiveSHA256"], "binarySHA256": pin["binary"]["sha256"],
        "gitModesMatched": True, "readWritePermissionDifferences": permission_differences,
        "fullHostAcceptancePending": snapshot["releaseAcceptancePending"],
    }
    if args.output:
        with args.output.open("x") as output:
            output.write(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result))


if __name__ == "__main__":
    main()
