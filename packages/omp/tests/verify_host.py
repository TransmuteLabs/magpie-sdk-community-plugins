"""Exercise this distribution through Magpie's real CLI checked-plan path.

Run only on the verification host, with a fixture binary and no live HOME.
No private Magpie packages or vendor commands are used.
"""

import base64
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import time


def verify(binary, distribution):
    source = Path(__file__).resolve().parents[1] / "src"
    binary, distribution = Path(binary).resolve(), Path(distribution).resolve()
    journal = (source / "testdata/omp-v3.jsonl").read_bytes()
    image = (source / "testdata/image.bin").read_bytes()
    digest = hashlib.sha256(image).hexdigest()
    with tempfile.TemporaryDirectory(prefix="omp-host-proof-") as temporary:
        root = Path(temporary)
        home = root / "home"
        output = root / "output"
        home.mkdir()
        output.mkdir()
        env = os.environ.copy()
        env.update(
            HOME=str(home), USERPROFILE=str(home),
            XDG_CONFIG_HOME=str(home / "config"),
            XDG_DATA_HOME=str(home / "data"),
            XDG_CACHE_HOME=str(home / "cache"),
            XDG_STATE_HOME=str(home / "state"),
            MAGPIE_TEST_SANDBOX=str(home),
            MAGPIE_BUN=str(root / "bun-does-not-exist"),
            HTTP_PROXY="http://127.0.0.1:9",
            HTTPS_PROXY="http://127.0.0.1:9",
            ALL_PROXY="http://127.0.0.1:9",
        )

        def run(*arguments, success=True, json_result=True):
            result = subprocess.run(
                [str(binary), "plugin", "sdk", *arguments],
                env=env, cwd=root, text=True, capture_output=True, timeout=30,
            )
            if success and result.returncode != 0:
                raise AssertionError(f"{arguments[0]} failed: {result.stderr}")
            if not success:
                if result.returncode == 0:
                    raise AssertionError("unauthorized output path was accepted")
                return result.stderr
            return json.loads(result.stdout) if json_result else result.stdout

        run("add", str(distribution), json_result=False)
        installed = run("inspect", "omp-sessions")
        if installed["manifest"]["id"] != "omp-sessions":
            raise AssertionError("wrong installed module")
        manifest = installed["manifest"]
        contract = manifest["contract"]["version"]
        request = {
            "id": "omp-to-document", "from": "omp", "to": "omp-conversation-v1",
            "files": [
                {"path": "sessions/fixture.jsonl", "format": "omp",
                 "body": base64.b64encode(journal).decode()},
                {"path": "blobs/" + digest, "format": "omp",
                 "body": base64.b64encode(image).decode()},
            ],
            "options": {"output": "conversation.json"},
        }
        encoded = json.dumps(request, separators=(",", ":"))
        preview = run(
            "session", "omp-sessions", encoded, "--root", str(output),
            "--path", "conversation.json",
        )
        if (output / "conversation.json").exists() or len(preview["files"]) != 1:
            raise AssertionError("preview wrote output or omitted its document")
        applied = run(
            "session", "omp-sessions", encoded, "--root", str(output),
            "--path", "conversation.json", "--apply",
            "--backup-dir", str(root / "backups"),
        )
        if not applied["applied"]:
            raise AssertionError("document plan did not apply")
        conversation = (output / "conversation.json").read_bytes()
        value = json.loads(conversation)
        if base64.b64decode(value["ompArchive"]["journal"]) != journal:
            raise AssertionError("native conversion lost original journal bytes")
        target = (
            "sessions/--work-omp--/"
            "2026-10-25T03-50-25-123Z_019a0000-0000-7000-8000-00000000000a.jsonl"
        )
        reverse = json.dumps({
            "id": "document-to-omp", "from": "omp-conversation-v1", "to": "omp",
            "files": [{"path": "conversation.json", "format": "omp-conversation-v1",
                       "body": base64.b64encode(conversation).decode()}],
            "options": {"output": "sessions/--work-omp--"},
        }, separators=(",", ":"))
        run(
            "session", "omp-sessions", reverse, "--root", str(output),
            "--path", target, success=False,
        )
        if (output / target).exists():
            raise AssertionError("rejected blob policy published a journal")
        # Reusing an existing content-addressed blob must refresh its grace
        # period before the new journal refers to it.
        blob = output / "blobs" / digest
        blob.parent.mkdir(exist_ok=True)
        blob.write_bytes(image)
        blob.chmod(0o600)
        old = time.time() - 7200
        os.utime(blob, (old, old))
        preview = run(
            "session", "omp-sessions", reverse, "--root", str(output),
            "--path", target, "--path", "blobs/" + digest,
        )
        if len(preview["files"]) != 2 or (output / target).exists():
            raise AssertionError("journal/blob preview was incomplete or wrote files")
        applied = run(
            "session", "omp-sessions", reverse, "--root", str(output),
            "--path", target, "--path", "blobs/" + digest, "--apply",
            "--backup-dir", str(root / "backups"),
        )
        if not applied["applied"] or (output / target).read_bytes() != journal:
            raise AssertionError("native reverse conversion changed journal bytes")
        if blob.read_bytes() != image or blob.stat().st_mtime <= old:
            raise AssertionError("blob bytes or reuse grace were lost")
        for file in (output / "conversation.json", output / target, blob):
            if stat.S_IMODE(file.stat().st_mode) != 0o600:
                raise AssertionError("checked output is not private")
        return {
            "module": "omp-sessions", "runtime": "native",
            "sdkMinimum": f"{contract['major']}.{contract['minor']}",
            "nativeABI": manifest["compatibility"]["nativeABI"],
            "journalSHA256": hashlib.sha256(journal).hexdigest(),
            "blobSHA256": digest, "previewWrites": False,
            "unapprovedBlobPathRejected": True, "losslessRoundTrip": True,
            "existingBlobGraceRefreshed": True, "privateOutputs": True,
        }


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit("usage: python3 verify_host.py MAGPIE_BINARY DISTRIBUTION")
    print(json.dumps(verify(sys.argv[1], sys.argv[2]), indent=2))
