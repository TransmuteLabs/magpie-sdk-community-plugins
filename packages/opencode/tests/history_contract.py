#!/usr/bin/env python3
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import sqlite3

parser = argparse.ArgumentParser()
parser.add_argument("output", type=Path)
parser.add_argument("--negative-main-only", action="store_true")
args = parser.parse_args()
root = Path(__file__).resolve().parents[1] / "src"
output = args.output.resolve()
output.mkdir(parents=True, exist_ok=False)
database = output / "opencode.db"
writer = sqlite3.connect(database)
writer.execute("PRAGMA journal_mode=WAL")
writer.execute("PRAGMA wal_autocheckpoint=0")
writer.execute("PRAGMA foreign_keys=ON")
writer.executescript((root / "testdata/donors/history-schema.sql").read_text())
writer.commit()
writer.execute("PRAGMA wal_checkpoint(TRUNCATE)")
baseline = database.read_bytes()
clock = 1794600000000
writer.execute("INSERT INTO project(id, worktree, time_created, time_updated, sandboxes) VALUES(?,?,?,?,?)",
               ("project_fixture", "/fixture/project", clock, clock, "[]"))
writer.execute("INSERT INTO session_v2(id,project_id,slug,directory,title,version,time_created,time_updated) VALUES(?,?,?,?,?,?,?,?)",
               ("ses_fixture", "project_fixture", "fixture", "/fixture/project", "Committed WAL session", "2.3.0", clock, clock))
for message, kind, seq, body in (
    ("msg_notice_old", "system", 1, {"metadata": {"notice": "instructions"}, "content": [{"type": "text", "text": "old notice"}]}),
    ("msg_user", "user", 2, {"text": "Review fixture", "files": [{"mime": "image/png", "name": "fixture.png", "uri": "media:fixture_hash"}]}),
    ("msg_assistant", "assistant", 3, {"content": [{"type": "text", "text": "legacy inline tail"}]}),
    ("msg_notice_new", "system", 4, {"metadata": {"notice": "instructions"}, "content": [{"type": "text", "text": "new notice"}]}),
):
    writer.execute("INSERT INTO session_message(id,session_id,type,seq,time_created,time_updated,data) VALUES(?,?,?,?,?,?,?)",
                   (message, "ses_fixture", kind, seq, clock, clock, json.dumps(body)))
for ordinal, text in ((2, "second split part"), (1, "first split part")):
    writer.execute("INSERT INTO session_part(message_id,session_id,ordinal,type,time_created,time_updated,data) VALUES(?,?,?,?,?,?,?)",
                   ("msg_assistant", "ses_fixture", ordinal, "text", clock, clock, json.dumps({"type": "text", "text": text})))
writer.execute("INSERT INTO instruction_state(session_id,epoch_start,through_seq,initial_values,current_values) VALUES(?,?,?,?,?)",
               ("ses_fixture", 2, 4, "{}", "{}"))
payload = b"synthetic image bytes"
writer.execute("INSERT INTO blob(hash,mime,bytes,data,time_created) VALUES(?,?,?,?,?)",
               ("fixture_hash", "image/png", len(payload), payload, clock))
writer.execute("INSERT INTO blob_ref(session_id,message_id,hash,seq) VALUES(?,?,?,?)",
               ("ses_fixture", "msg_user", "fixture_hash", 2))
writer.commit()
assert database.read_bytes() == baseline, "test setup checkpointed away its committed WAL evidence"
assert Path(str(database) + "-wal").stat().st_size > 32
main_only = output / "main-only.db"
shutil.copyfile(database, main_only)
reader = sqlite3.connect(database.as_uri() + "?mode=ro", uri=True)
reader.execute("PRAGMA query_only=ON")
reader.execute("BEGIN")
assert reader.execute("SELECT count(*) FROM session_v2").fetchone()[0] == 1
copy_reader = sqlite3.connect(main_only.as_uri() + "?mode=ro&immutable=1", uri=True)
copy_count = copy_reader.execute("SELECT count(*) FROM session_v2").fetchone()[0]
assert copy_count == 0, "main-only copy unexpectedly supplied committed WAL sessions"
selected = copy_reader if args.negative_main_only else reader
assert selected.execute("SELECT title FROM session_v2 WHERE id=?", ("ses_fixture",)).fetchone() == ("Committed WAL session",), \
    "history read lost committed WAL state"
writer.execute("INSERT INTO session_message(id,session_id,type,seq,time_created,time_updated,data) VALUES(?,?,?,?,?,?,?)",
               ("msg_later", "ses_fixture", "user", 5, clock, clock, '{"text":"committed after reader began"}'))
writer.commit()
assert reader.execute("SELECT count(*) FROM session_message").fetchone()[0] == 4, "selected read changed its transaction snapshot"
reader.commit()
assert reader.execute("SELECT count(*) FROM session_message").fetchone()[0] == 5
writer.execute("INSERT INTO session_message(id,session_id,type,seq,time_created,time_updated,data) VALUES(?,?,?,?,?,?,?)",
               ("msg_uncommitted", "ses_fixture", "user", 6, clock, clock, '{"text":"not committed"}'))
assert reader.execute("SELECT count(*) FROM session_message").fetchone()[0] == 5, "history read admitted an uncommitted message"
writer.rollback()
parts = [json.loads(value[0]) for value in reader.execute(
    "SELECT data FROM session_part WHERE message_id=? ORDER BY ordinal", ("msg_assistant",))]
inline = json.loads(reader.execute("SELECT data FROM session_message WHERE id=?", ("msg_assistant",)).fetchone()[0])["content"]
assert [part["text"] for part in parts + inline] == ["first split part", "second split part", "legacy inline tail"]
epoch = reader.execute("SELECT epoch_start FROM instruction_state WHERE session_id=?", ("ses_fixture",)).fetchone()[0]
notices = [(seq, json.loads(data)) for seq, data in reader.execute(
    "SELECT seq,data FROM session_message WHERE session_id=? AND type='system' ORDER BY seq", ("ses_fixture",))]
replayed = [body["content"][0]["text"] for seq, body in notices if body.get("metadata", {}).get("notice") != "instructions" or seq >= epoch]
assert replayed == ["new notice"]
assert reader.execute("SELECT data FROM blob WHERE hash=?", ("fixture_hash",)).fetchone()[0] == payload
try:
    reader.execute("DELETE FROM session_v2")
except sqlite3.OperationalError as failure:
    assert "readonly" in str(failure)
else:
    raise AssertionError("own-storage test reader was writable")
report = {"syntheticSQLite": True, "actualPluginHistory": False, "actualOpenCodeBinary": False,
          "schema": "Reviewed literal DDL selection, not a complete migration",
          "committedWALVisible": True, "mainOnlySessionCount": copy_count, "transactionSnapshotStable": True,
          "uncommittedExcluded": True, "splitPartsThenInline": True, "instructionEpochApplied": True,
          "blobHydrated": True, "queryOnly": True,
          "schemaSHA256": hashlib.sha256((root / "testdata/donors/history-schema.sql").read_bytes()).hexdigest(),
          "proposedGrant": "Host-selected root read lease, module-owned logical list/read, coherent SQLite/WAL view and publication revocation"}
(output / "result.json").write_text(json.dumps(report, indent=2) + "\n")
reader.close()
copy_reader.close()
writer.close()
print(json.dumps(report))
