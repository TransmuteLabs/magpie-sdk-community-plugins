package ompsessions

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func importJournal(t *testing.T, journal []byte) (sdk.SessionResult, error) {
	t.Helper()
	return Convert(t.Context(), sdk.SessionRequest{
		ID: "omp-to-document", From: "omp", To: DocumentFormat,
		Files:   []sdk.SessionDocument{{Path: "sessions/source.jsonl", Format: "omp", Body: journal}},
		Options: json.RawMessage(`{"output":"conversation.json"}`),
	})
}

func TestLargeJournalInterchangeCanBeReimported(t *testing.T) {
	document := Document{Schema: 1, ID: sessionID, Cwd: "/work/fixture", Timestamp: fixtureTime,
		Items: []Item{{Kind: "user", Content: []Content{{Type: "text", Text: strings.Repeat("x", 4<<20)}}}}}
	encoded, err := Encode(t.Context(), document, fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := importJournal(t, encoded.Journal)
	if err != nil {
		t.Fatal("valid journal import:", err)
	}
	if len(imported.Files) != 1 || len(imported.Files[0].Body) <= 8<<20 {
		t.Fatal("fixture did not exercise archive/projection growth beyond 8 MiB")
	}
	restored, err := Convert(t.Context(), sdk.SessionRequest{
		ID: "document-to-omp", From: DocumentFormat, To: "omp", Files: imported.Files,
		Options: json.RawMessage(`{"output":"sessions/bucket"}`),
	})
	if err != nil {
		t.Fatalf("accepted %d-byte journal emitted an unreimportable %d-byte document: %v",
			len(encoded.Journal), len(imported.Files[0].Body), err)
	}
	if len(restored.Files) != 1 || !bytes.Equal(restored.Files[0].Body, encoded.Journal) {
		t.Fatal("large journal did not round-trip byte-for-byte")
	}
}

func TestLiteralBlobTextRoundTrip(t *testing.T) {
	document := Document{Schema: 1, ID: sessionID, Cwd: "/work/fixture", Timestamp: fixtureTime,
		Items: []Item{{Kind: "user", Content: []Content{{Type: "text", Text: "blob:not-an-attachment"}}}}}
	encoded, err := Encode(t.Context(), document, fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := importJournal(t, encoded.Journal)
	if err != nil {
		t.Fatal("plain user text was interpreted as an attachment:", err)
	}
	restored, err := Convert(t.Context(), sdk.SessionRequest{
		ID: "document-to-omp", From: DocumentFormat, To: "omp", Files: imported.Files,
		Options: json.RawMessage(`{"output":"sessions/bucket"}`),
	})
	if err != nil || len(restored.Files) != 1 || !bytes.Equal(restored.Files[0].Body, encoded.Journal) {
		t.Fatal("literal blob text was not preserved:", err)
	}
}

func TestBlobRecognitionIsBoundToOMPAttachmentFields(t *testing.T) {
	literal := "blob:not-an-attachment"
	journal := fixtureJournal(t,
		textMessage("u", "", literal),
		message("a", "u", "assistant", map[string]any{"content": []any{
			map[string]any{"type": "text", "text": literal},
			map[string]any{"type": "thinking", "thinking": literal},
			map[string]any{"type": "toolCall", "id": "call", "name": "inspect",
				"arguments": map[string]any{"data": literal, "image": map[string]any{"type": "image", "data": literal}}},
		}}),
		message("result", "a", "toolResult", map[string]any{"toolCallId": "call", "content": literal}),
		node("audit", "result", "custom", map[string]any{
			"RootMeta": map[string]any{"Audit": map[string]any{
				"ScopeBound": literal, "image": map[string]any{"type": "image", "data": literal},
			}},
		}),
		node("title", "audit", "title_change", map[string]any{"title": literal}),
	)
	document, err := Decode(t.Context(), journal, nil)
	if err != nil {
		t.Fatal("non-attachment fields triggered blob resolution:", err)
	}
	restored, err := Encode(t.Context(), document, fixtureTime)
	if err != nil || !bytes.Equal(restored.Journal, journal) {
		t.Fatal("opaque text, arguments or metadata changed:", err)
	}

	asset := []byte("explicit synthetic image")
	hash := hashBytes(asset)
	image := map[string]any{"type": "image", "data": "blob:sha256:" + hash, "mimeType": "image/png"}
	for _, test := range []struct {
		name, role string
		fields     map[string]any
	}{
		{"user image", "user", map[string]any{"content": []any{image}}},
		{"assistant image", "assistant", map[string]any{"content": []any{image}}},
		{"tool-result image", "toolResult", map[string]any{"toolCallId": "call", "content": []any{image}}},
		{"file-mention image", "fileMention", map[string]any{"files": []any{map[string]any{"image": image}}}},
		{"file-mention image without type", "fileMention", map[string]any{"files": []any{map[string]any{
			"image": map[string]any{"data": "blob:sha256:" + hash, "mimeType": "image/png"},
		}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// The referenced image lies outside the selected branch, but remains
			// in the archived journal and still needs its explicit asset.
			journal := fixtureJournal(t, textMessage("root", "", "root"),
				message("abandoned", "root", test.role, test.fields),
				textMessage("selected", "root", "selected"))
			if _, err := Decode(t.Context(), journal, nil); err == nil {
				t.Fatal("off-branch attachment was left unresolved")
			}
			document, err := Decode(t.Context(), journal, []Asset{{SHA256: hash, Body: asset}})
			if err != nil {
				t.Fatal("supplied off-branch attachment:", err)
			}
			restored, err := Encode(t.Context(), document, fixtureTime)
			if err != nil || !bytes.Equal(restored.Journal, journal) || len(restored.Assets) != 1 {
				t.Fatal("off-branch attachment was lost:", err)
			}
		})
	}
}

func TestConversionRejectsOutputsBeyondReverseAdmission(t *testing.T) {
	t.Run("archive exceeds plan", func(t *testing.T) {
		journal := fixtureJournal(t, textMessage("u", "", strings.Repeat("x", 7<<20)))
		if len(journal) >= 8<<20 {
			t.Fatal("fixture exceeds the journal input budget")
		}
		result, err := importJournal(t, journal)
		if err == nil || len(result.Files) != 0 {
			t.Fatal("emitted an archived document beyond checked-plan bounds")
		}
	})
	t.Run("fresh assets exceed journal admission", func(t *testing.T) {
		image := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x71}, 8<<20))
		document := Document{Schema: 1, ID: sessionID, Cwd: "/work/fixture", Timestamp: fixtureTime,
			Items: []Item{{Kind: "user", Content: []Content{{Type: "image", Data: image, MimeType: "image/png"}}}}}
		if _, err := Encode(t.Context(), document, fixtureTime); err == nil {
			t.Fatal("pure encoder emitted a journal/asset set its decoder cannot admit")
		}
		body, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		result, err := Convert(t.Context(), sdk.SessionRequest{
			ID: "document-to-omp", From: DocumentFormat, To: "omp",
			Files:   []sdk.SessionDocument{{Path: "conversation.json", Format: DocumentFormat, Body: body}},
			Options: json.RawMessage(`{"output":"sessions/bucket"}`),
		})
		if err == nil || len(result.Files) != 0 {
			t.Fatal("emitted a journal/asset set beyond reverse import admission")
		}
	})
}
