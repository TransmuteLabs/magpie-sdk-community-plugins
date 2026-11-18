package history

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

var nativeFixture = []byte("\r\n" +
	`{"type":"progress","sessionId":"native-fixture","opaque":{"large":9007199254740993}}` + "\r\n" +
	`{"type":"user","sessionId":"native-fixture","uuid":"u","message":{"role":"user","content":[{"type":"text","text":"synthetic question"}]}}` + "\n" +
	`{"type":"assistant","sessionId":"native-fixture","uuid":"a","parentUuid":"u","message":{"role":"assistant","content":[{"type":"thinking","thinking":"opaque","signature":"synthetic-signature"},{"type":"tool_use","id":"t","name":"fixture","input":{}}]}}` + "\n")

var legacyFixture = []byte(`{
 "meta":{"taskId":"legacy-fixture","traceId":"trace","workspacePath":"/synthetic/project","createdAt":1,"updatedAt":2,"runtimeEpoch":9007199254740993,"offPeakTaskId":"opaque-not-admission"},
 "messages":[{"role":"user","content":"synthetic question","timestamp":1},
 {"role":"assistant","content":"preview","timestamp":2,"durationMs":42,"thought":"opaque","tools":[{"id":"t","opaque":true}],"parts":[{"type":"thinking","signature":"synthetic-signature"}],"bodyRefs":[{"field":"content","refId":"external-body","hash":"opaque","fullBytes":100,"previewBytes":7}]}],
 "fileChanges":[{"turnIndex":0,"snapshots":[{"path":"synthetic.txt","beforeContent":null,"afterContent":"preview","writeCount":1,"contentRefs":[{"refId":"external-file"}]}]}],
 "turnCheckpoints":[{"turnIndex":0,"baseFileCheckpointId":"base"}],
 "future":{"large":9007199254740993}
}`)

func TestReadersPreserveOpaqueNativeHistoryAndExternalReferences(t *testing.T) {
	for _, fixture := range []struct {
		format  string
		body    []byte
		records int
	}{
		{NativeJSONL, nativeFixture, 3}, {LegacySession, legacyFixture, 2},
	} {
		t.Run(fixture.format, func(t *testing.T) {
			document, err := Read(fixture.format, fixture.body)
			if err != nil {
				t.Fatal(err)
			}
			summary := document.Summary()
			if summary.Records != fixture.records || summary.UserMessages != 1 || summary.AssistantMessages != 1 ||
				!summary.HasThinking || !bytes.Equal(document.Source(), fixture.body) {
				t.Fatalf("history projection lost source or native blocks: %+v", summary)
			}
			if fixture.format == NativeJSONL && (summary.OpaqueRecords != 1 || summary.ToolBlocks != 1) {
				t.Fatal("native opaque records/tool blocks were flattened")
			}
			if fixture.format == LegacySession && (!summary.HasExternalReferences || summary.LegacyToolEntries != 1) {
				t.Fatal("legacy references were presented as complete inline history")
			}
			if !bytes.Contains(document.Source(), []byte("9007199254740993")) ||
				!bytes.Contains(document.Source(), []byte("synthetic-signature")) {
				t.Fatal("large integers or signed opaque fields were changed")
			}
			source := document.Source()
			source[0] = 'X'
			records := document.Records()
			records[0][0] = 'X'
			summary.SessionIDs[0] = "changed"
			if !bytes.Equal(document.Source(), fixture.body) || document.Records()[0][0] == 'X' ||
				document.Summary().SessionIDs[0] == "changed" {
				t.Fatal("reader exposed mutable internal source/index state")
			}
		})
	}
}

func TestSourcePreservingBundleRoundTripsExactWhitespaceAndSignatures(t *testing.T) {
	for _, fixture := range []struct {
		format string
		body   []byte
	}{
		{NativeJSONL, nativeFixture}, {LegacySession, legacyFixture},
	} {
		t.Run(fixture.format, func(t *testing.T) {
			packed, err := Convert(fixture.format, Bundle, fixture.body)
			if err != nil {
				t.Fatal(err)
			}
			document, err := Read(Bundle, packed)
			if err != nil || !bytes.Equal(document.Source(), fixture.body) {
				t.Fatalf("bundle changed original history: %v", err)
			}
			restored, err := Convert(Bundle, fixture.format, packed)
			if err != nil || !bytes.Equal(restored, fixture.body) {
				t.Fatalf("bundle restore lost source bytes: %v", err)
			}
			same, err := Convert(Bundle, Bundle, packed)
			if err != nil || !bytes.Equal(same, packed) {
				t.Fatal("same-format conversion changed a validated bundle")
			}
			if _, err := Convert(fixture.format, "anthropic-messages", fixture.body); !errors.Is(err, ErrConversion) {
				t.Fatal("converter silently dropped native metadata for a cross-agent target")
			}
		})
	}
}

func TestBundlesRejectTamperingRecursionAndUnknownSchema(t *testing.T) {
	packed, err := Convert(NativeJSONL, Bundle, nativeFixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(map[string]any){
		func(value map[string]any) { value["sha256"] = strings.Repeat("0", 64) },
		func(value map[string]any) { value["version"] = 2 },
		func(value map[string]any) { value["sourceFormat"] = Bundle },
		func(value map[string]any) { value["unexpected"] = "would-be-dropped" },
		func(value map[string]any) { value["source"] = "not base64" },
	} {
		var value map[string]any
		json.Unmarshal(packed, &value)
		change(value)
		body, _ := json.Marshal(value)
		if _, err := Read(Bundle, body); err == nil {
			t.Fatal("accepted a corrupt or unsupported bundle")
		}
	}
}

func TestHistoryBoundsAndMalformedSourcesFailClosed(t *testing.T) {
	for _, fixture := range []struct {
		format string
		body   []byte
	}{
		{NativeJSONL, []byte(`{"type":"user","type":"assistant"}`)},
		{NativeJSONL, []byte(`{"message":{}}`)},
		{NativeJSONL, []byte(" \n\r\n")},
		{LegacySession, []byte(`{"meta":{},"messages":[]}`)},
		{LegacySession, []byte(`{"meta":{"taskId":"t","traceId":"x","workspacePath":"/fixture","createdAt":0,"updatedAt":1},"messages":[{"role":"assistant","content":"x","timestamp":-1}]}`)},
		{LegacySession, []byte("SQLite format 3\x00synthetic")},
		{NativeJSONL, []byte{0xff}},
		{"unknown-format", nativeFixture},
		{NativeJSONL, bytes.Repeat([]byte("x"), MaxBytes+1)},
		{NativeJSONL, bytes.Repeat([]byte("{\"type\":\"progress\"}\n"), MaxRecords+1)},
	} {
		if _, err := Read(fixture.format, fixture.body); err == nil {
			t.Fatal("accepted malformed or unbounded offline history")
		}
	}
}
