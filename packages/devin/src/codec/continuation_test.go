package devin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/protocolext"
)

func TestRecoveredCascadeAndSiteCatalogs(t *testing.T) {
	tests := []struct {
		kind CatalogKind
		path string
		body []byte
		team int
	}{
		{"cascade-models", "/exa.api_server_pb.ApiServerService/GetCascadeModelConfigs", fixtureText(1, string(fixtureText(3, "credential-sentinel"))), 0},
		{"cascade-models-site", "/exa.api_server_pb.ApiServerService/GetCascadeModelConfigsForSite", join(fixtureText(1, "credential-sentinel"), fixtureText(2, "synthetic-ide"), fixtureText(3, "1"), fixtureText(4, "1"), fixtureText(7, "")), 7},
		{"command-models-site", "/exa.api_server_pb.ApiServerService/GetCommandModelConfigsForSite", fixtureText(2, ""), 2},
		{"cli-models-site", "/exa.api_server_pb.ApiServerService/GetCliModelConfigsForSite", fixtureText(3, ""), 3},
	}
	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			adapter := CatalogAdapter{Kind: test.kind}
			if adapter.Path() != test.path || adapter.ID() != "devin."+string(test.kind) {
				t.Fatalf("missing recovered discovery method: id=%q path=%q", adapter.ID(), adapter.Path())
			}
			request, err := adapter.Decode(context.Background(), catalogHeaders(), test.body)
			if err != nil || request.Protocol != sdk.Models || len(request.Body) != 0 || len(request.Metadata) != 0 {
				t.Fatalf("native discovery did not use the guarded Models operation: %+v %v", request, err)
			}
			recorder := httptest.NewRecorder()
			reply := adapter.Reply(recorder)
			reply.Header().Set("Content-Type", "application/json")
			if _, err := reply.Write([]byte(catalogFixture)); err != nil {
				t.Fatal(err)
			}
			if err := reply.Finish(nil); err != nil {
				t.Fatal(err)
			}
			original, err := encodeCatalog([]byte(catalogFixture), CliModels)
			if err != nil || !bytes.Equal(recorder.Body.Bytes(), original) || recorder.Code != http.StatusOK {
				t.Fatalf("recovered catalog projection differs: %x %v", recorder.Body.Bytes(), err)
			}
			if test.team != 0 {
				_, err := adapter.Decode(context.Background(), catalogHeaders(), fixtureText(test.team, "private-team-sentinel"))
				var public *sdk.Error
				if !errors.As(err, &public) || public.Status != http.StatusBadRequest || strings.Contains(public.Message, "sentinel") {
					t.Fatalf("unresolvable native team selection was accepted or leaked: %v", err)
				}
			}
			_, err = adapter.Decode(context.Background(), catalogHeaders(), fixtureText(99, "credential-sentinel"))
			if err == nil || strings.Contains(err.Error(), "sentinel") {
				t.Fatalf("unknown discovery semantic field was accepted or leaked: %v", err)
			}
		})
	}
}

func TestRecoveredSystemPromptAndAdvisoryFields(t *testing.T) {
	system := fixturePrompt(5, "native system", fixtureNumber(4, 27), fixtureNumber(5, 1), fixtureText(8, string(fixtureNumber(1, 1))))
	user := fixturePrompt(1, "question", fixtureNumber(4, 7), fixtureNumber(5, 0), fixtureNumber(9, 0),
		fixtureNumber(13, 0), fixtureText(15, ""), fixtureText(16, ""), fixtureText(17, ""),
		fixtureText(19, ""), fixtureText(22, ""))
	body := join(fixtureText(14, "group/test"), fixtureText(2, "outer system"),
		fixtureText(3, string(system)), fixtureText(3, string(user)))
	request, err := (Adapter{}).Decode(context.Background(), fixtureHeaders(), fixtureFrame(0, body))
	if err != nil {
		t.Fatalf("generated system/advisory prompt fields were refused: %v", err)
	}
	var canonical struct {
		Messages []struct {
			Role    string            `json:"role"`
			Content string            `json:"content"`
			Cache   map[string]string `json:"cache_control"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(request.Body, &canonical); err != nil {
		t.Fatal(err)
	}
	if len(canonical.Messages) != 3 || canonical.Messages[0].Role != "system" ||
		canonical.Messages[1].Role != "system" || canonical.Messages[1].Content != "native system" ||
		canonical.Messages[1].Cache["type"] != "ephemeral" || canonical.Messages[2].Content != "question" {
		t.Fatalf("native system ordering/cache or user content changed: %s", request.Body)
	}
	for _, forbidden := range []string{"num_tokens", "safe_for_code_telemetry"} {
		if strings.Contains(string(request.Body), forbidden) {
			t.Fatal("advisory native fields became canonical authority:", string(request.Body))
		}
	}
}

func TestRecoveredPromptStateRefusals(t *testing.T) {
	for name, field := range map[string][]byte{
		"token overflow":         fixtureNumber(4, 1<<32),
		"invalid telemetry bool": fixtureNumber(5, 2),
		"tool result error":      fixtureNumber(9, 1),
		"redacted thinking":      fixtureNumber(13, 1),
		"output provenance":      fixtureText(15, "opaque-sentinel"),
		"thinking provenance":    fixtureText(16, "opaque-sentinel"),
		"gemini thought state":   fixtureText(17, "opaque-sentinel"),
		"assistant phase":        fixtureText(19, "opaque-sentinel"),
		"generation provenance":  fixtureText(22, "opaque-sentinel"),
		"prompt annotations":     fixtureText(14, string(fixtureNumber(1, 0))),
		"video content":          fixtureText(20, ""),
		"document content":       fixtureText(21, ""),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (Adapter{}).Decode(context.Background(), fixtureHeaders(), fixtureFrame(0,
				fixtureRequest(fixtureText(3, string(fixturePrompt(2, "answer", field))))))
			if err == nil || strings.Contains(err.Error(), "opaque-sentinel") {
				t.Fatalf("unrepresentable native prompt state accepted or leaked: %v", err)
			}
		})
	}
	for name, body := range map[string][]byte{
		"after user": fixtureRequest(fixtureText(3, string(fixturePrompt(5, "late system")))),
		"with tools": join(fixtureText(14, "group/test"), fixtureText(3, string(fixturePrompt(5, "system",
			fixtureText(6, string(join(fixtureText(1, "call"), fixtureText(2, "read"), fixtureText(3, "{}")))))))),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := (Adapter{}).Decode(context.Background(), fixtureHeaders(), fixtureFrame(0, body)); err == nil {
				t.Fatal("invalid native system prompt accepted")
			}
		})
	}
}

func TestChatRejectsAmbiguousConnectHeaders(t *testing.T) {
	for _, name := range []string{"Content-Type", "Connect-Protocol-Version", "Connect-Content-Encoding", "Content-Encoding"} {
		t.Run(name, func(t *testing.T) {
			header := fixtureHeaders()
			switch name {
			case "Content-Type":
				header[name] = []string{"application/connect+proto", "application/json"}
			case "Connect-Protocol-Version":
				header[name] = []string{"1", "2"}
			default:
				header[name] = []string{"identity", "gzip"}
			}
			if _, err := (Adapter{}).Decode(context.Background(), header, fixtureFrame(0, fixtureRequest())); err == nil {
				t.Fatal("ambiguous singular Connect header accepted:", name)
			}
		})
	}
}

func TestRecoveredToolResultImages(t *testing.T) {
	call := join(fixtureText(1, "screenshot-1"), fixtureText(2, "screenshot"), fixtureText(3, "{}"))
	image := join(fixtureText(1, "aW1hZ2U="), fixtureText(2, "image/png"), fixtureText(3, ""))
	body := join(fixtureText(14, "group/test"),
		fixtureText(3, string(fixturePrompt(1, "inspect"))),
		fixtureText(3, string(fixturePrompt(2, "", fixtureText(6, string(call))))),
		fixtureText(3, string(fixturePrompt(4, "screen", fixtureText(7, "screenshot-1"), fixtureText(10, string(image))))))
	request, err := (Adapter{}).Decode(context.Background(), fixtureHeaders(), fixtureFrame(0, body))
	if err != nil {
		t.Fatalf("native tool-result image was refused: %v", err)
	}
	var canonical struct {
		Messages []struct {
			Role    string          `json:"role"`
			CallID  string          `json:"tool_call_id"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(request.Body, &canonical); err != nil {
		t.Fatal(err)
	}
	if len(canonical.Messages) != 3 {
		t.Fatalf("native conversation changed: %s", request.Body)
	}
	message := canonical.Messages[2]
	if message.Role != "tool" || message.CallID != "screenshot-1" {
		t.Fatalf("image result detached from its tool call: %s", request.Body)
	}
	var content []struct {
		Type  string `json:"type"`
		Text  string `json:"text"`
		Image struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(message.Content, &content); err != nil || len(content) != 2 ||
		content[0].Type != "text" || content[0].Text != "screen" || content[1].Type != "image_url" ||
		content[1].Image.URL != "data:image/png;base64,aW1hZ2U=" {
		t.Fatalf("tool text/image data was changed: %s %v", message.Content, err)
	}
	for name, bad := range map[string][]byte{
		"invalid base64": join(fixtureText(1, "not-base64"), fixtureText(2, "image/png")),
		"caption":        join(fixtureText(1, "aW1hZ2U="), fixtureText(2, "image/png"), fixtureText(3, "caption-sentinel")),
		"invalid mime":   join(fixtureText(1, "aW1hZ2U="), fixtureText(2, "application/octet-stream")),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodePrompt(fixturePrompt(4, "", fixtureText(7, "screenshot-1"), fixtureText(10, string(bad)))); err == nil ||
				strings.Contains(err.Error(), "sentinel") {
				t.Fatal("unrepresentable native image accepted or leaked:", err)
			}
		})
	}
	if _, err := decodePrompt(fixturePrompt(2, "", fixtureText(10, string(image)))); err == nil {
		t.Fatal("assistant image accepted without a canonical mapping")
	}
}
