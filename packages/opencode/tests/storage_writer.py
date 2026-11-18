#!/usr/bin/env python3
import base64
import hashlib
import json
import os
from pathlib import Path
import shutil
import sqlite3
import sys

directory = Path(sys.argv[1]).resolve()
directory.mkdir(parents=True, exist_ok=True)
database = directory / "opencode.db"
writer = sqlite3.connect(database, timeout=0.05)
writer.execute("PRAGMA journal_mode=WAL")
writer.execute("PRAGMA wal_autocheckpoint=0")
writer.execute("PRAGMA foreign_keys=ON")
writer.executescript((Path(__file__).resolve().parents[1] / "src/testdata/donors/history-schema.sql").read_text())
writer.executescript("""
CREATE TABLE kv(key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE session AS SELECT * FROM session_v2;
CREATE TABLE message(id TEXT,session_id TEXT,time_created INTEGER,data TEXT);
CREATE TABLE part(id TEXT,session_id TEXT,message_id TEXT,data TEXT);
""")
writer.commit()
writer.execute("PRAGMA wal_checkpoint(TRUNCATE)")
clock = 1794600000000
writer.execute("INSERT INTO project(id,worktree,time_created,time_updated,sandboxes) VALUES('global','/fixture',?,?,'[]')", (clock, clock))
writer.execute("INSERT INTO session_v2(id,project_id,slug,directory,title,version,time_created,time_updated) VALUES('ses_wal','global','wal','/fixture','Committed WAL session','2.3.0',?,?)", (clock, clock))
writer.execute("INSERT INTO session SELECT * FROM session_v2")
writer.execute("INSERT INTO session(id,project_id,slug,directory,title,version,time_created,time_updated) VALUES('ses_v1','global','old','/fixture','Unmigrated V1','1',?,?)", (clock, clock))
writer.execute("INSERT INTO kv VALUES('migration.v1-v2','{\"phase\":\"sessions\"}')")
media = base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aWQAAAABJRU5ErkJggg==")
digest = hashlib.sha256(media).hexdigest()
for message, kind, seq, body in (
    ("msg_old", "system", 1, {"text": "retired instruction", "metadata": {"notice": "instructions"}}),
    ("msg_user", "user", 2, {"text": "ordinary SQLite question", "files": [
        {"mime": "image/png", "name": "fixture.png", "uri": "media:" + digest},
        {"mime": "image/png", "name": "same-blob.png", "uri": "media:" + digest},
        {"mime": "text/plain", "name": "external", "uri": "file:///etc/passwd"},
    ]}),
    ("msg_new", "system", 3, {"text": "current instruction", "metadata": {"notice": "instructions"}}),
    ("msg_assistant", "assistant", 4, {"model": {"id": "fixture-model", "providerID": "fixture"},
        "content": [{"type": "text", "text": "inline tail"}],
        "tokens": {"input": 7, "output": 3, "reasoning": 5, "cache": {"read": 2, "write": 1}}}),
):
    writer.execute("INSERT INTO session_message(id,session_id,type,seq,time_created,time_updated,data) VALUES(?,'ses_wal',?,?,?,?,?)",
                   (message, kind, seq, clock, clock, json.dumps(body)))
for ordinal, text in ((2, "second split part"), (1, "first split part")):
    writer.execute("INSERT INTO session_part(message_id,session_id,ordinal,type,time_created,time_updated,data) VALUES('msg_assistant','ses_wal',?,'text',?,?,?)",
                   (ordinal, clock, clock, json.dumps({"type": "text", "text": text})))
writer.execute("INSERT INTO instruction_state VALUES('ses_wal',3,4,'[]','[]')")
writer.execute("INSERT INTO blob(hash,mime,bytes,data,time_created) VALUES(?,'image/png',?,?,?)", (digest, len(media), media, clock))
writer.execute("INSERT INTO blob_ref(session_id,message_id,hash,seq) VALUES('ses_wal','msg_user',?,2)", (digest,))
writer.commit()
legacy = directory / "storage/session/global/ses_leftover.json"
legacy.parent.mkdir(parents=True)
legacy.write_text(json.dumps({"id": "ses_leftover", "directory": "/fixture", "title": "Left behind JSON",
                             "time": {"created": clock, "updated": clock}}))
main_copy = directory / "main-only.db"
shutil.copyfile(database, main_copy)
with sqlite3.connect(main_copy.as_uri() + "?mode=ro&immutable=1", uri=True) as main_reader:
    main_only = main_reader.execute("SELECT count(*) FROM session_v2").fetchone()[0]

def state():
    value = {"pid": os.getpid(), "mainOnly": main_only, "mediaId": "media:" + digest}
    for key, suffix in (("db", ""), ("wal", "-wal"), ("shm", "-shm")):
        value[key] = hashlib.sha256(Path(str(database) + suffix).read_bytes()).hexdigest()
    return value

print(json.dumps(state()), flush=True)
try:
    for line in sys.stdin:
        command = json.loads(line)
        if command == "stop":
            break
        if command == "append":
            writer.execute("INSERT INTO session_message VALUES('msg_later','ses_wal','user',5,?,?,?)",
                           (clock + 1, clock + 1, '{"text":"later committed message"}'))
            writer.execute("UPDATE session_v2 SET title='Updated WAL session',time_updated=? WHERE id='ses_wal'", (clock + 1,))
            writer.commit()
        elif command == "migrate":
            writer.execute("UPDATE kv SET value='{\"phase\":\"completed\"}' WHERE key='migration.v1-v2'")
            writer.commit()
        elif command == "uncommitted":
            writer.execute("INSERT INTO session_message VALUES('msg_secret','ses_wal','user',6,?,?,?)",
                           (clock + 2, clock + 2, '{"text":"uncommitted secret"}'))
        elif command == "rollback":
            writer.rollback()
        elif command == "checkpoint":
            busy, _, _ = writer.execute("PRAGMA wal_checkpoint(TRUNCATE)").fetchone()
            print(json.dumps({"busy": busy}), flush=True)
            continue
        elif command != "state":
            raise ValueError("unknown fixture command")
        print(json.dumps(state()), flush=True)
finally:
    writer.close()
