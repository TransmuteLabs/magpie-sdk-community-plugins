package history

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCLILogPreservesNativeObservabilityWithoutInventingSessionMessages(t *testing.T) {
	body := []byte("{\"timestamp\":\"2026-11-14T00:00:00.123Z\",\"level\":\"info\",\"event\":\"model.request.completed\",\"module\":\"zcode.model\",\"message\":\"done\",\"sessionId\":\"sess_observed\",\"turnId\":\"query_turn\",\"traceId\":\"trace\",\"spanId\":\"span\",\"context\":{\"rawUsage\":{\"inputTokens\":9007199254740993},\"apiKey\":\"[Redacted]\"},\"future\":{\"keep\":true}}\n" +
		"{\"timestamp\":\"2026-11-14T00:00:00.124Z\",\"level\":\"warn\",\"event\":\"ttft.log.dropped\",\"count\":2}\n")
	document, err := Read("zcode-cli-log-jsonl", body)
	if err != nil || document.Summary().Records != 2 || document.Summary().UserMessages != 0 ||
		document.Summary().AssistantMessages != 0 || len(document.Summary().SessionIDs) != 1 {
		t.Fatalf("native JSONL diagnostics were rejected or fabricated as conversation: %v %+v", err, document.Summary())
	}
	if !bytes.Contains(document.Records()[0], []byte("9007199254740993")) || !bytes.Equal(document.Source(), body) {
		t.Fatal("source logging precision, unknown context or exact bytes were lost")
	}
	bundle, err := Convert("zcode-cli-log-jsonl", Bundle, body)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Convert(Bundle, "zcode-cli-log-jsonl", bundle)
	if err != nil || !bytes.Equal(restored, body) {
		t.Fatal("native log bundle failed exact restoration:", err)
	}
	if _, err := Convert("zcode-cli-log-jsonl", NativeJSONL, body); err == nil {
		t.Fatal("diagnostic logs were silently converted to session history")
	}
}

func TestCLILogIncompleteTailIsRetainedAsUnknownWithoutLosingCompleteRecords(t *testing.T) {
	body := []byte("{\"timestamp\":\"2026-11-14T00:00:00Z\",\"level\":\"info\",\"message\":\"complete\"}\n{\"timestamp\":")
	document, err := Read("zcode-cli-log-jsonl", body)
	encoded, _ := json.Marshal(document.Summary())
	if err != nil || document.Summary().Records != 1 || !bytes.Contains(encoded, []byte(`"incompleteTail":true`)) ||
		!bytes.Equal(document.Source(), body) {
		t.Fatalf("partial log tail became an empty/complete observation: %v %s", err, encoded)
	}
	for _, invalid := range []string{
		"{\"timestamp\":\"bad\",\"level\":\"info\"}\n",
		"{\"timestamp\":\"2026-11-14T00:00:00Z\",\"level\":\"invented\"}\n",
		"{\"timestamp\":\"2026-11-14T00:00:00Z\",\"level\":\"info\",\"level\":\"warn\"}\n",
	} {
		if _, err := Read("zcode-cli-log-jsonl", []byte(invalid)); err == nil {
			t.Fatal("malformed native diagnostic record was silently accepted")
		}
	}
}
