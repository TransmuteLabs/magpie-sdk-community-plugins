package devin

import (
	"bytes"
	"testing"
)

func TestDonor044DeferredImageResidueKeepsUserOwnership(test *testing.T) {
	for _, fixture := range []struct {
		name      string
		residue   string
		cache     string
		wantText  string
		wantImage bool
		wantError bool
	}{
		{name: "whitespace", residue: `{"type":"text","text":" \n\t"}`},
		{name: "empty", residue: `{"type":"text","text":""}`},
		{name: "words", residue: `{"type":"text","text":" user words "}`, wantText: " user words "},
		{name: "user-image", residue: `{"type":"text","text":" "},{"type":"image_url","image_url":{"url":"data:image/png;base64,eQ=="}}`, wantText: " ", wantImage: true},
		{name: "whitespace-cache", residue: `{"type":"text","text":" \n\t"}`, cache: `,"cache_control":{"type":"ephemeral"}`, wantError: true},
		{name: "empty-cache", residue: `{"type":"text","text":""}`, cache: `,"cache_control":{"type":"ephemeral"}`, wantError: true},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			body := []byte(`{"model":"swe","messages":[{"role":"user","content":"look"},
				{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"shot","arguments":"{}"}}]},
				{"role":"tool","tool_call_id":"c1","content":"done\n\n[The tool returned an image; it follows in the next message.]"},
				{"role":"user","content":[{"type":"text","text":"[From the result of shot (tool call c1):]"},
				{"type":"image_url","image_url":{"url":"data:image/png;base64,eA=="}},` + fixture.residue + `]` + fixture.cache + `}]}`)
			original := bytes.Clone(body)
			encoded, err := EncodeChatRequest(body, "selected-key")
			if fixture.wantError {
				if err == nil {
					test.Fatal("residual whitespace cache boundary was silently lost or sent")
				}
				return
			}
			if err != nil {
				test.Fatal(err)
			}
			fields, _ := readMessage(encoded.Payload, nil)
			expected := 3
			if fixture.wantText != "" || fixture.wantImage {
				expected = 4
			}
			if len(fields[3]) != expected {
				test.Fatalf("residual whitespace created a user prompt: got %d want %d", len(fields[3]), expected)
			}
			tool, _ := readMessage(fields[3][2].b, nil)
			if tool.text(7) != "c1" || tool.text(3) != "done" || len(tool[10]) != 1 {
				test.Fatal("real tool result lost its identity, text or image")
			}
			image, _ := readMessage(tool[10][0].b, nil)
			if image.text(1) != "eA==" {
				test.Fatal("real tool image crossed owners")
			}
			if expected == 4 {
				user, _ := readMessage(fields[3][3].b, nil)
				imageCount := 0
				if fixture.wantImage {
					imageCount = 1
				}
				if user.text(3) != fixture.wantText || len(user[10]) != imageCount {
					test.Fatal("residual meaningful user content changed")
				}
				if fixture.wantImage {
					image, _ := readMessage(user[10][0].b, nil)
					if image.text(1) != "eQ==" {
						test.Fatal("remaining user image changed ownership")
					}
				}
			}
			if !bytes.Equal(original, body) {
				test.Fatal("caller body was mutated")
			}
		})
	}
}
