package ompsessions

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

const sessionID = "019a0000-0000-7000-8000-00000000000a"

var fixtureTime = time.Date(2026, 10, 25, 3, 50, 25, 123000000, time.UTC)

func fixtureJournal(t *testing.T, records ...map[string]any) []byte {
	t.Helper()
	header, err := json.Marshal(map[string]any{
		"type": "session", "version": 3, "id": sessionID,
		"cwd": "/work/omp", "timestamp": fixtureTime.Format(time.RFC3339Nano), "title": "Fixture title",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := append(header, '\n')
	for _, record := range records {
		raw, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		body = append(append(body, raw...), '\n')
	}
	return body
}

func node(id, parent, kind string, fields map[string]any) map[string]any {
	var previous any
	if parent != "" {
		previous = parent
	}
	result := map[string]any{
		"type": kind, "id": id, "parentId": previous,
		"timestamp": fixtureTime.Format(time.RFC3339Nano),
	}
	for key, value := range fields {
		result[key] = value
	}
	return result
}

func message(id, parent, role string, fields map[string]any) map[string]any {
	payload := map[string]any{"role": role}
	for key, value := range fields {
		payload[key] = value
	}
	return node(id, parent, "message", map[string]any{"message": payload})
}

func textMessage(id, parent, text string) map[string]any {
	return message(id, parent, "user", map[string]any{"content": text})
}

func itemWords(items []Item) string {
	body, _ := json.Marshal(items)
	return string(body)
}

func TestLosslessJournalRoundTrip(t *testing.T) {
	body, err := os.ReadFile("testdata/omp-v3.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	assetBody, err := os.ReadFile("testdata/image.bin")
	if err != nil {
		t.Fatal(err)
	}
	assets := []Asset{{SHA256: hashBytes(assetBody), Body: assetBody}}
	before := bytes.Clone(body)
	document, err := Decode(t.Context(), body, assets)
	if err != nil {
		t.Fatal(err)
	}
	if document.Title != "Renamed OMP fixture" || document.Cwd != "/work/omp" || document.ID != sessionID {
		t.Fatalf("metadata: %+v", document)
	}
	words := itemWords(document.Items)
	for _, want := range []string{"human request", "visible narration", "printf fixture", "tool failed", "standalone output"} {
		if !strings.Contains(words, want) {
			t.Fatalf("portable projection missing %q: %s", want, words)
		}
	}
	for _, hidden := range []string{"system-only", "private reasoning", "abandoned sibling"} {
		if strings.Contains(words, hidden) {
			t.Fatalf("nonportable/off-branch content entered projection: %s", words)
		}
	}
	rawDocument, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var copied Document
	if err := strictJSON(rawDocument, &copied); err != nil {
		t.Fatal(err)
	}
	result, err := Encode(t.Context(), copied, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Journal, before) || !bytes.Equal(body, before) || len(result.Assets) != 1 ||
		!bytes.Equal(result.Assets[0].Body, assetBody) {
		t.Fatal("journal fields, title slot, tool history or asset bytes changed on round trip")
	}
	copied.Items[0].Text = "changed"
	if _, err := Encode(t.Context(), copied, fixtureTime); err == nil || !strings.Contains(err.Error(), "projection changed") {
		t.Fatalf("ignored an edited portable projection: %v", err)
	}
	copied = document
	copied.Archive.Journal = append(bytes.Clone(body), []byte(" ")...)
	if _, err := Encode(t.Context(), copied, fixtureTime); err == nil || !strings.Contains(err.Error(), "digest changed") {
		t.Fatalf("ignored changed archived journal bytes: %v", err)
	}
}

func TestActiveBranchResetAndCompaction(t *testing.T) {
	for _, test := range []struct {
		name   string
		fields map[string]any
		want   []string
		absent []string
	}{
		{"portable", map[string]any{"summary": "safe summary", "firstKeptEntryId": "tail"}, []string{"safe summary", "retained tail", "after compaction"}, []string{"old history"}},
		{"empty cutoff", map[string]any{"summary": "", "firstKeptEntryId": "tail"}, []string{"retained tail", "after compaction"}, []string{"old history"}},
		{"provider opaque", map[string]any{"summary": "opaque", "preserveData": map[string]any{"openaiRemoteCompaction": "secret"}}, []string{"old history", "retained tail", "after compaction"}, []string{"opaque"}},
		{"snapcompact", map[string]any{"summary": "opaque", "method": "snapcompact"}, []string{"old history", "retained tail", "after compaction"}, []string{"opaque"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			journal := fixtureJournal(t,
				textMessage("old", "", "old history"),
				textMessage("tail", "old", "retained tail"),
				node("compact", "tail", "compaction", test.fields),
				textMessage("after", "compact", "after compaction"),
				textMessage("abandoned", "old", "abandoned sibling"),
				node("metadata", "abandoned", "model_usage", map[string]any{"activeLeafId": "after"}),
			)
			document, err := Decode(t.Context(), journal, nil)
			if err != nil {
				t.Fatal(err)
			}
			words := itemWords(document.Items)
			for _, word := range test.want {
				if !strings.Contains(words, word) {
					t.Fatalf("missing %q: %s", word, words)
				}
			}
			for _, word := range append(test.absent, "abandoned sibling") {
				if strings.Contains(words, word) {
					t.Fatalf("unexpected %q: %s", word, words)
				}
			}
			restored, err := Encode(t.Context(), document, fixtureTime)
			if err != nil || !bytes.Equal(restored.Journal, journal) {
				t.Fatal("compaction's provider fields or off-branch history were lost", err)
			}
		})
	}
	journal := fixtureJournal(t, textMessage("old", "", "old history"),
		node("reset", "old", "reset_boundary", nil), textMessage("new", "reset", "after reset"))
	document, err := Decode(t.Context(), journal, nil)
	if err != nil || strings.Contains(itemWords(document.Items), "old history") || !strings.Contains(itemWords(document.Items), "after reset") {
		t.Fatalf("reset boundary: %+v %v", document.Items, err)
	}
	journal = fixtureJournal(t, textMessage("old", "", "old history"),
		node("clear", "old", "custom", map[string]any{"activeLeafId": nil}))
	document, err = Decode(t.Context(), journal, nil)
	if err != nil || len(document.Items) != 0 {
		t.Fatalf("explicit null did not clear active branch: %+v %v", document.Items, err)
	}
}

func TestFreshV3JournalAndBlobPlan(t *testing.T) {
	large := bytes.Repeat([]byte{0x71}, 768)
	small := []byte("tiny image")
	document := Document{Schema: 1, Cwd: "/work/omp", Title: "fresh title", Timestamp: fixtureTime,
		Items: []Item{
			{Kind: "user", Content: []Content{{Type: "text", Text: "before"}, {Type: "image", MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(large)}, {Type: "text", Text: "between"}, {Type: "image", MimeType: "image/jpeg", Data: base64.RawURLEncoding.EncodeToString(small)}}},
			{Kind: "assistant", Text: "Checking."},
			{Kind: "call", ID: "bash", Name: "bash", Shell: "false", Workdir: "/work/other"},
			{Kind: "result", ID: "bash", Text: strings.Repeat("output", 12000), IsError: true},
			{Kind: "call", ID: "read", Name: "read", Input: `{"path":"file.go"}`},
			{Kind: "result", ID: "read", Text: "full read output"},
		}}
	result, err := Encode(t.Context(), document, fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	if !validUUID(result.ID) || result.ID[14] != '7' || bytes.IndexByte(result.Journal, '\n') != 255 {
		t.Fatalf("UUID/title slot: %s / %d", result.ID, bytes.IndexByte(result.Journal, '\n'))
	}
	if !strings.HasSuffix(result.Filename, "_"+result.ID+".jsonl") || !strings.HasPrefix(result.Filename, "2026-10-25T03-50-25-123Z_") {
		t.Fatalf("native filename: %s", result.Filename)
	}
	if len(result.Assets) != 1 || !bytes.Equal(result.Assets[0].Body, large) || result.Assets[0].SHA256 != hashBytes(large) {
		t.Fatalf("content-addressed blob plan: %+v", result.Assets)
	}
	var parent any
	var calls = map[string]string{}
	for i, line := range bytes.Split(bytes.TrimSpace(result.Journal), []byte{'\n'}) {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			continue
		}
		if record["parentId"] != parent {
			t.Fatalf("parent chain: %v -> %v", parent, record)
		}
		parent = record["id"]
		message := record["message"].(map[string]any)
		at, _ := time.Parse(time.RFC3339Nano, record["timestamp"].(string))
		if message["timestamp"] != float64(at.UnixMilli()) {
			t.Fatal("envelope/message timestamp mismatch")
		}
		switch message["role"] {
		case "assistant":
			if message["model"] != "<synthetic>" || message["api"] != "anthropic-messages" || message["usage"].(map[string]any)["input"] != float64(0) {
				t.Fatal("synthetic assistant acquired real usage")
			}
			for _, block := range message["content"].([]any) {
				typed := block.(map[string]any)
				if typed["type"] == "toolCall" {
					calls[typed["id"].(string)] = typed["name"].(string)
				}
			}
		case "toolResult":
			if calls[message["toolCallId"].(string)] != message["toolName"] {
				t.Fatal("tool result lost its native name or call pairing")
			}
		}
	}
	readback, err := Decode(t.Context(), result.Journal, result.Assets)
	if err != nil {
		t.Fatal(err)
	}
	if len(readback.Items) != len(document.Items) || readback.Items[3].Text != document.Items[3].Text ||
		readback.Items[0].Content[2].Text != "between" || readback.Title != document.Title {
		t.Fatalf("native readback truncated or reordered content: %+v", readback.Items)
	}
}

func TestRejectMissingCorruptAndAmbiguousInputs(t *testing.T) {
	hash := strings.Repeat("a", 64)
	withBlob := fixtureJournal(t, message("one", "", "user", map[string]any{"content": []any{
		map[string]any{"type": "image", "data": "blob:sha256:" + hash, "mimeType": "image/png"},
	}}))
	for _, test := range []struct {
		name    string
		journal []byte
		assets  []Asset
	}{
		{"missing asset", withBlob, nil},
		{"wrong digest", withBlob, []Asset{{SHA256: hash, Body: []byte("wrong")}}},
		{"malformed json", []byte("{oops\n"), nil},
		{"missing header", []byte("{\"type\":\"custom\",\"id\":\"one\"}\n"), nil},
		{"duplicate id", fixtureJournal(t, textMessage("same", "", "first"), textMessage("same", "", "second")), nil},
		{"duplicate metadata id", fixtureJournal(t, textMessage("same", "", "first"), node("same", "", "title_change", map[string]any{"title": "second"})), nil},
		{"missing parent", fixtureJournal(t, textMessage("one", "missing", "first")), nil},
		{"cycle", fixtureJournal(t, textMessage("one", "two", "first"), textMessage("two", "one", "second")), nil},
		{"bad blob", bytes.ReplaceAll(withBlob, []byte("blob:sha256:"+hash), []byte("blob:sha256:../../private")), nil},
		{"oversized", bytes.Repeat([]byte("x"), MaxBytes+1), nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode(t.Context(), test.journal, test.assets); err == nil {
				t.Fatal("invalid input was accepted")
			}
		})
	}
	for _, item := range []Item{
		{Kind: "developer", Text: "must not disappear"},
		{Kind: "user", Content: []Content{{Type: "audio", Data: "must not disappear"}}},
		{Kind: "call", ID: "pending", Name: "read", Input: `{}`},
		{Kind: "result", ID: "orphan", Text: "must not disappear"},
		{Kind: "user", Content: []Content{{Type: "image", MimeType: "image/png", Data: "invalid!!!"}}},
	} {
		document := Document{Schema: 1, Cwd: "/work/omp", Timestamp: fixtureTime, Items: []Item{item}}
		if _, err := Encode(t.Context(), document, fixtureTime); err == nil {
			t.Fatalf("unsupported or incomplete item was dropped: %+v", item)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Decode(ctx, withBlob, nil); err == nil {
		t.Fatal("cancelled decode succeeded")
	}
	if _, err := Encode(ctx, Document{Schema: 1}, fixtureTime); err != context.Canceled {
		t.Fatalf("cancelled encode: %v", err)
	}
}

func TestUUIDVersionsVariantsAndTimestamp(t *testing.T) {
	seen := map[string]bool{}
	for _, version := range []byte{4, 7} {
		for i := 0; i < 500; i++ {
			id, err := newUUID(fixtureTime, version)
			if err != nil || !validUUID(id) || seen[id] {
				t.Fatalf("UUID collision/format: %q %v", id, err)
			}
			seen[id] = true
			raw, _ := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
			if raw[6]>>4 != version || raw[8]&0xc0 != 0x80 {
				t.Fatalf("UUID version/variant: %s", id)
			}
			if version == 7 {
				var milliseconds int64
				for _, value := range raw[:6] {
					milliseconds = milliseconds<<8 | int64(value)
				}
				if milliseconds != fixtureTime.UnixMilli() {
					t.Fatalf("UUIDv7 timestamp: %d", milliseconds)
				}
			}
		}
	}
	if _, err := newUUID(time.UnixMilli(-1), 7); err == nil {
		t.Fatal("UUIDv7 accepted a negative timestamp")
	}
	if _, err := newUUID(time.UnixMilli(1<<48), 7); err == nil {
		t.Fatal("UUIDv7 accepted an overflowing timestamp")
	}
}
