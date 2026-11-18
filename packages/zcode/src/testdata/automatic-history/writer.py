import hashlib
import json
import os
from pathlib import Path
import sqlite3
import sys


def main():
    root = Path(sys.argv[1])
    root.mkdir(parents=True, exist_ok=True)
    path = root / "db.sqlite"
    database = sqlite3.connect(path, timeout=0)
    database.execute("PRAGMA journal_mode=WAL")
    database.execute("PRAGMA wal_autocheckpoint=0")
    database.executescript("""
CREATE TABLE session(id TEXT PRIMARY KEY,parent_id TEXT,directory TEXT NOT NULL,title TEXT NOT NULL,
 title_source TEXT,time_created INTEGER NOT NULL,time_updated INTEGER NOT NULL,time_deleted INTEGER);
CREATE TABLE message(id TEXT PRIMARY KEY,session_id TEXT NOT NULL,time_created INTEGER NOT NULL,
 time_updated INTEGER NOT NULL,data TEXT NOT NULL,sequence INTEGER);
CREATE TABLE part(id TEXT PRIMARY KEY,message_id TEXT NOT NULL,session_id TEXT NOT NULL,
 time_created INTEGER NOT NULL,time_updated INTEGER NOT NULL,data TEXT NOT NULL,sequence INTEGER);
CREATE INDEX message_session_time_created_id_idx ON message(session_id,time_created,id);
CREATE INDEX part_session_idx ON part(session_id);
CREATE TRIGGER message_sequence_autofill AFTER INSERT ON message WHEN NEW.sequence IS NULL
 BEGIN UPDATE message SET sequence=0 WHERE id=NEW.id; END;
CREATE TABLE zcode_database_identity(scope TEXT PRIMARY KEY,identity TEXT NOT NULL);
INSERT INTO zcode_database_identity VALUES('session','synthetic-database-generation-one');
""")
    database.commit()
    database.execute("PRAGMA wal_checkpoint(TRUNCATE)")
    baseline = path.read_bytes()
    workspace = sys.argv[2] if len(sys.argv) == 3 else "/synthetic/workspace"
    clock = 1800000000000
    for session, parent, title, source, offset in [
        ("ses_one", None, "Committed WAL session", "custom", 5),
        ("ses_child", "ses_one", "Default", "default", 4),
        ("ses_orphan", None, "Orphan", "custom", 3),
        ("ses_unknown", None, "Unknown usage", "custom", 2),
        ("ses_zero", None, "Zero usage", "custom", 1),
    ]:
        database.execute("INSERT INTO session VALUES(?,?,?,?,?,?,?,NULL)",
                         (session, parent, workspace, title, source, clock, clock + offset))
    database.execute("INSERT INTO session VALUES(?,?,?,?,?,?,?,?)",
                     ("ses_deleted", None, "/synthetic", "Deleted", "custom", clock, clock, clock))

    def message(identity, session, role, sequence, **extra):
        body = dict(role=role, id="embedded-identity-is-not-authority", **extra)
        database.execute("INSERT INTO message VALUES(?,?,?,?,?,?)",
                         (identity, session, clock + sequence, clock + sequence, json.dumps(body), sequence))

    def part(identity, message_id, session, sequence, **body):
        body.update(id="foreign-part", sessionID="foreign-session", messageID="foreign-message")
        database.execute("INSERT INTO part VALUES(?,?,?,?,?,?,?)",
                         (identity, message_id, session, clock, clock, json.dumps(body), sequence))

    tokens = {"input": 9007199254740993, "output": 7, "reasoning": 3, "cache": {"read": 11, "write": 2}}
    message("msg_user", "ses_one", "user", 0, semantics={"origin": "real_user", "kind": "user_prompt"})
    message("msg_original", "ses_one", "assistant", 1, modelId="GLM-5.3", tokens=tokens)
    part("part_user", "msg_user", "ses_one", 0, type="text", text="Original user text")
    part("part_last", "msg_original", "ses_one", 0, type="text", text="last block", blockIndex=2)
    part("part_reasoning", "msg_original", "ses_one", 1, type="reasoning", text="first reasoning",
         blockIndex=0, signature="opaque-synthetic-signature")
    part("part_tool", "msg_original", "ses_one", 2, type="tool", tool="Read", callID="call-one", blockIndex=1,
         state={"status": "completed", "input": {"path": "synthetic"}, "output": "tool fixture"})
    part("part_media", "msg_original", "ses_one", 3, type="file", mime="image/png", filename="external.png",
         url="file:///must-not-open")
    part("part_body", "msg_original", "ses_one", 4, type="reasoning", bodyRef="external-body", blockIndex=3)
    message("msg_copy", "ses_child", "assistant", 0, modelId="GLM-5.3", tokens=tokens,
            metadata={"forkOrigin": {"sessionId": "ses_one", "messageId": "msg_original"}})
    message("msg_runtime", "ses_child", "user", 1, synthetic=True, semantics={"origin": "agent_runtime"})
    part("part_runtime", "msg_runtime", "ses_child", 0, type="text", text="Do not use as title")
    message("msg_child_user", "ses_child", "user", 2, semantics={"origin": "real_user", "kind": "user_prompt"})
    part("part_child_user", "msg_child_user", "ses_child", 0, type="text", text="Actual child title")
    message("msg_child_new", "ses_child", "assistant", 3, modelId="GLM-5.3",
            tokens={"input": 5, "output": 4, "reasoning": 1, "cache": {"read": 2, "write": 0}})
    message("msg_orphan", "ses_orphan", "assistant", 0, modelId="GLM-5.3",
            tokens={"input": 7, "output": 3, "reasoning": 1, "cache": {"read": 1, "write": 0}},
            metadata={"forkOrigin": {"sessionId": "removed", "messageId": "msg_missing"}})
    message("msg_unknown", "ses_unknown", "assistant", 0, modelId="GLM-5.3")
    message("msg_zero", "ses_zero", "assistant", 0, modelId="GLM-5.3",
            tokens={"input": 0, "output": 0, "reasoning": 0, "cache": {"read": 0, "write": 0}})
    database.commit()
    assert path.read_bytes() == baseline, "fixture checkpointed its WAL-only commits"
    main_only = root / "main-only.sqlite"
    main_only.write_bytes(baseline)
    comparison = sqlite3.connect(main_only.as_uri() + "?mode=ro&immutable=1", uri=True)
    main_count = comparison.execute("SELECT count(*) FROM session").fetchone()[0]
    comparison.close()
    assert main_count == 0, "fixture did not establish WAL-only history"

    def state():
        return {name: hashlib.sha256(Path(str(path) + suffix).read_bytes()).hexdigest()
                if Path(str(path) + suffix).exists() else None
                for name, suffix in [("db", ""), ("wal", "-wal"), ("shm", "-shm")]}

    print(json.dumps(dict(pid=os.getpid(), mainOnly=main_count, **state())), flush=True)
    for line in sys.stdin:
        command = json.loads(line)
        if command == "append":
            database.execute("UPDATE session SET title='Updated WAL session',time_updated=? WHERE id='ses_one'", (clock + 99,))
            message("msg_later", "ses_one", "user", 3)
            part("part_later", "msg_later", "ses_one", 0, type="text", text="later committed message")
            database.commit()
        elif command == "uncommitted":
            message("msg_uncommitted", "ses_one", "user", 4)
            part("part_uncommitted", "msg_uncommitted", "ses_one", 0, type="text", text="uncommitted secret")
        elif command == "rollback":
            database.rollback()
        elif command == "checkpoint":
            busy, log, checkpointed = database.execute("PRAGMA wal_checkpoint(TRUNCATE)").fetchone()
            print(json.dumps(dict(busy=busy, log=log, checkpointed=checkpointed, **state())), flush=True)
            continue
        elif command == "malformed":
            database.execute("UPDATE message SET data='{' WHERE id='msg_original'")
            database.commit()
        elif command == "identity":
            database.execute("UPDATE zcode_database_identity SET identity='synthetic-database-generation-two' WHERE scope='session'")
            database.commit()
        elif command == "legacy":
            database.execute("DROP TABLE zcode_database_identity")
            database.commit()
        elif command == "replace":
            assert database.execute("PRAGMA wal_checkpoint(TRUNCATE)").fetchone()[0] == 0
            replacement = path.read_bytes()
            database.close()
            path.rename(root / "previous.sqlite")
            path.write_bytes(replacement)
            database = sqlite3.connect(path, timeout=0)
            database.execute("PRAGMA wal_autocheckpoint=0")
            database.execute("UPDATE session SET title=title WHERE id='ses_one'")
            database.commit()
        elif command == "state":
            pass
        else:
            raise ValueError(command)
        print(json.dumps(state()), flush=True)
    database.rollback()
    database.close()


if __name__ == "__main__":
    main()
