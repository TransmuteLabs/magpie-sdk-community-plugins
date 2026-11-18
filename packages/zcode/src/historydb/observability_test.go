package historydb

import (
	"bytes"
	"testing"
)

func TestSQLiteObservabilityRetainsAttemptsRawUsageAndTurnToolRows(t *testing.T) {
	body := fixture(t,
		`CREATE TABLE session (id TEXT PRIMARY KEY, directory TEXT, title TEXT, time_created INTEGER, time_updated INTEGER)`,
		`CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT, data TEXT)`,
		`CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT, session_id TEXT, data TEXT)`,
		`CREATE TABLE model_usage (id TEXT PRIMARY KEY, logical_request_id TEXT, attempt_index INTEGER, session_id TEXT, turn_id TEXT, trace_id TEXT, query_source TEXT, provider_id TEXT, model_id TEXT, status TEXT, started_at INTEGER, input_tokens INTEGER, output_tokens INTEGER, reasoning_tokens INTEGER, cache_creation_input_tokens INTEGER, cache_read_input_tokens INTEGER, raw_usage_json TEXT, provider_metadata_json TEXT)`,
		`CREATE TABLE turn_usage (session_id TEXT, turn_id TEXT, status TEXT, started_at INTEGER, input_tokens INTEGER, output_tokens INTEGER, model_request_count INTEGER)`,
		`CREATE TABLE tool_usage (id TEXT PRIMARY KEY, session_id TEXT, tool_call_id TEXT, tool_name TEXT, status TEXT, started_at INTEGER, duration_ms INTEGER)`,
		`INSERT INTO session VALUES ('sess_source','/fixture','Source',1,2)`,
		`INSERT INTO model_usage VALUES ('attempt1','logical',0,'sess_source','query_turn','trace','main_turn','zai','GLM-5.3','error',1,7,0,0,3,11,'{"cache_creation":{"ephemeral_1h_input_tokens":3},"future":9007199254740993}','{"opaque":"keep"}')`,
		`INSERT INTO model_usage VALUES ('attempt2','logical',1,'sess_source','query_turn','trace','main_turn','zai','GLM-5.3','completed',2,8,5,2,4,12,'{"output_tokens":5}',NULL)`,
		`INSERT INTO turn_usage VALUES ('sess_source','query_turn','completed',1,15,5,2)`,
		`INSERT INTO tool_usage VALUES ('tool_row','sess_source','call1','Read','completed',1,8)`)
	document, err := Read(t.Context(), body)
	if err != nil {
		t.Fatal(err)
	}
	tables := document.Tables()
	if len(tables["model_usage"]) != 2 || len(tables["turn_usage"]) != 1 || len(tables["tool_usage"]) != 1 ||
		document.Summary().Rows["model_usage"] != 2 || document.Summary().Calls != 0 {
		t.Fatalf("native observability rows were omitted or counted again as messages: %+v", document.Summary())
	}
	if !bytes.Contains(tables["model_usage"][0]["raw_usage_json"], []byte("9007199254740993")) ||
		string(tables["model_usage"][1]["attempt_index"]) != "1" ||
		string(tables["model_usage"][1]["provider_metadata_json"]) != "null" {
		t.Fatal("attempt identity, nullable provider metadata or exact raw usage was normalized away")
	}
	bundle, _, err := Convert(t.Context(), Format, Bundle, body)
	if err != nil {
		t.Fatal(err)
	}
	restored, _, err := Convert(t.Context(), Bundle, Format, bundle)
	if err != nil || !bytes.Equal(body, restored) {
		t.Fatal("usage-bearing snapshot failed exact binary restoration")
	}
}
