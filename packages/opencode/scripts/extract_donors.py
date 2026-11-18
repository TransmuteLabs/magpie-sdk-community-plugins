#!/usr/bin/env python3
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

root = Path(__file__).resolve().parents[1] / "src"
native = Path(sys.argv[1]).resolve()
community = Path(sys.argv[2]).resolve()
archive_repository = Path(sys.argv[3]).resolve() if len(sys.argv) > 3 else root.parents[3] / "magpie"
destination = root / "testdata/donors"
destination.mkdir(exist_ok=True)
records = []


def source(repository, commit, path):
    return subprocess.check_output(["git", "-C", str(repository), "show", commit + ":" + path])


def save(name, data, commit, path, selection):
    target = destination / name
    target.write_bytes(data)
    records.append({"fixture": str(target.relative_to(root)), "commit": commit,
                    "path": path, "selection": selection,
                    "sha256": hashlib.sha256(data).hexdigest()})


vendor = "38bd73789a43bf043599d6196af96583228035d7"
donor = "57d1cc6a9f49389d495ea9be8d986c70de7767bf"
path = "packages/core/src/models-dev/snapshot.txt"
snapshot = source(native, vendor, path).decode()
decoder = json.JSONDecoder()
position = 1
while True:
    name, end = decoder.raw_decode(snapshot, position)
    start = end + 1
    value, end = decoder.raw_decode(snapshot, start)
    if name == "opencode":
        save("zen-catalog.json", snapshot[start:end].encode(), vendor, path,
             "Exact JSON value of top-level opencode; no reserialization")
        break
    position = end + 1

for name, path in (
    ("zen-list.json", "packages/zen-free/fixtures/zen-models-2026-10-15.json"),
    ("tool-fixtures.mjs", "packages/zen-free/tool-fixtures.mjs"),
    ("zen-tool-response.mjs", "packages/zen-free/tool-response.mjs"),
    ("zen-collapse.mjs", "packages/zen-free/stream.mjs"),
):
    save(name, source(community, donor, path), donor, path, "Entire reviewed public fixture")

path = "packages/zen-free/provider.mjs"
text = source(community, donor, path).decode()
constants = text[text.index("const CORE_TOOLS ="):text.index("\n};", text.index("const CORE_PARAMETER =")) + 3]
helpers = text[text.index("function normalizeTools("):text.index("\n// Zen's free tier", text.index("function renameRefused("))]
save("zen-tool-request.js.txt", (constants + "\n\n" + helpers).encode(), donor, path,
     "Literal CORE_TOOLS/CORE_PARAMETER and normalizeTools through renameRefused; no environment, transport or provider initialization")

start = text.index("const HAS_ENVIRONMENT =")
end = text.index("\nconst ALPHABET =", start)
save("zen-environment.js.txt", text[start:end].encode(), donor, path,
     "Literal environment layout and insertion helpers; oracle supplies synthetic filesystem/platform facts, no local inspection")

for name, path in (
    ("skill-file.ts.txt", "packages/core/src/config/plugin/skill-file.ts"),
    ("markdown.ts.txt", "packages/core/src/config/markdown.ts"),
):
    save(name, source(native, vendor, path), vendor, path,
         "Entire literal native skill frontmatter parser/schema authority; not executed")

path = "packages/core/src/plugin/provider/opencode-watches.ts"
text = source(native, vendor, path).decode()
start = text.index("const EventType =")
end = text.index("/** The longest watch", start)
schema = text[start:end]
start = text.index('      editor.add({\n        name: "events"')
end = text.index("\n    })", start)
save("watch-types.ts.txt", (schema + "\n" + text[start:end]).encode(), vendor, path,
     "Literal EventType schema, description limit and events action normalization; session-mutating watch actions excluded")

path = "packages/core/test/plugin/provider-opencode.test.ts"
text = source(native, vendor, path).decode()
start = text.index("const skillBundle = {")
end = text.index("\nconst consolePolicies", start)
save("skill-bundle.ts.txt", text[start:end].encode(), vendor, path,
     "Literal skillBundle declaration")

start = text.index('const state = { instruction: "Use tabs.", lists: 200 }')
end = text.index("\n      })", start)
save("console-lists.ts.txt", text[start:end].encode(), vendor, path,
     "Literal Console list server in header/cache regression")

path = "packages/core/test/plugin/provider-opencode-watches.test.ts"
text = source(native, vendor, path).decode()
start = text.index("const watchEvent = (")
end = text.index("\n/** Console's page size", start)
save("watch-event.ts.txt", text[start:end].encode(), vendor, path,
     "Literal watchEvent fixture constructor; not executed locally")

path = "packages/schema/src/config/policy.ts"
save("policy.ts.txt", source(native, vendor, path), vendor, path,
     "Exact policy enums and legacy resource normalization")

for name, path in (
    ("database-path.ts.txt", "packages/cli/src/database-path.ts"),
    ("global-roots.ts.txt", "packages/util/src/global-roots.ts"),
):
    save(name, source(native, vendor, path), vendor, path, "Entire literal native storage-root source; not executed")

migrations = [
    ("packages/core/src/database/migration/20260127222353_familiar_lady_ursula.ts", {"project"}),
    ("packages/core/src/database/migration/20260804233008_loose_psylocke.ts",
     {"session_v2", "__new_session_message", "instruction_state"}),
    ("packages/core/src/database/migration/20261029002625_session_part.ts", {"session_part"}),
    ("packages/core/src/database/migration/20261025123711_blob.ts", {"blob_ref", "blob"}),
]
statements = []
for path, tables in migrations:
    text = source(native, vendor, path).decode()
    for payload in re.findall(r"tx\.run\(`([\s\S]*?)(?<!\\)`\)", text):
        statement = payload.replace(r"\`", "`")
        table = re.search(r"CREATE TABLE (?:IF NOT EXISTS )?`([^`]+)`", statement)
        if table and table[1] in tables:
            statements.append(statement)
        if statement == "ALTER TABLE `__new_session_message` RENAME TO `session_message`;":
            statements.append(statement)
save("history-schema.sql", "\n".join(statements).encode(), vendor, ";".join(path for path, _ in migrations),
     "Literal CREATE TABLE template payloads for project/session_v2/staging session_message/instruction_state/session_part/blob_ref/blob plus literal staging rename; only JS backtick escapes decoded, not a complete migration")

for name, path, begin, end, selection in (
    ("history-parts.ts.txt", "packages/core/src/session/history.ts",
     "export const withParts =", "\n// Every stored message row", "Literal withParts ordering and legacy-inline merge"),
    ("history-epoch.ts.txt", "packages/core/src/session/history.ts",
     "  const native = entries.findLast", "\n})", "Literal instruction baseline epoch and native checkpoint replay filter"),
    ("history-wal.ts.txt", "packages/core/src/database/database.ts",
     "      if (supportsTuningPragmas", "\n      // Durable Object SQLite", "Literal native file-database WAL/bootstrap pragmas; not executed"),
    ("history-media.ts.txt", "packages/core/src/session/blob.ts",
     "const unavailable =", "\nfunction inlineToolItem", "Literal missing-media and inline file hydration behavior"),
):
    text = source(native, vendor, path).decode()
    start = text.index(begin)
    finish = text.index(end, start)
    save(name, text[start:finish].encode(), vendor, path, selection)

archive_commit = "893f560cccdeb8fa2e98eadb60427990c10eedcc"
path = "internal/sessions/opencode.go"
text = source(archive_repository, archive_commit, path).decode()
save("history-legacy-roots.go.txt", text[text.index("// OpenCode keeps"):text.index("// ocInfo")].encode(),
     archive_commit, path, "Literal archived root/storage and V1/V2 selection commentary/helpers; not a runtime dependency")

for name, selection, path in (
    ("console.json", "Reviewed JSON adaptation of native skillBundle, instruction/list fixtures and watchEvent defaults; synthetic sessionID ses_fixture and cursor 1.1",
     "packages/core/test/plugin/provider-opencode.test.ts;packages/core/test/plugin/provider-opencode-watches.test.ts"),
    ("zen-replies.json", "toolReply(protocol, Read, false/true) from exact tool-fixtures.mjs, generated only on usbox with Node via testdata/donor_replies.mjs",
     "packages/zen-free/tool-fixtures.mjs"),
    ("zen-tools.json", "Reviewed synthetic native requests/replies evaluated by literal request helpers and guardToolResponse on usbox through testdata/zen_tools_oracle.mjs; no environment or stream-collapse policy",
     "packages/zen-free/provider.mjs;packages/zen-free/tool-response.mjs;packages/zen-free/tool-fixtures.mjs"),
    ("zen-policy.json", "Synthetic cases generated on usbox by unchanged literal collapseResponse and environment helpers; configured facts only, temporary preference omits donor's unverifiable approval claim; native strict incomplete/error rejection stays module-owned",
     "packages/zen-free/provider.mjs;packages/zen-free/stream.mjs"),
):
    fixture = destination / name
    if fixture.exists():
        save(name, fixture.read_bytes(), donor if name.startswith("zen-") else vendor, path, selection)

(destination / "provenance.json").write_text(json.dumps(records, indent=2) + "\n")
