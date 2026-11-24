package opencode

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestPackagedModelsPreserveDonorProtocolsAndModalities(t *testing.T) {
	manifest := Manifest()
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if manifest.Contract.Version.Minor != 12 || manifest.Compatibility.HostAPI != "^1.12" || len(Models()) != 4 {
		t.Fatal("full model metadata is not guarded by the negotiated host contract")
	}
	for _, test := range []struct {
		id          string
		protocol    string
		modalities  []string
		window      int64
		output      int64
		temperature bool
	}{
		{"deepseek-v4.1-flash", "chat", []string{"text", "image"}, 1000000, 384000, true},
		{"glm-5.3", "chat", []string{"text"}, 1000000, 131072, true},
		{"kimi-k3", "chat", []string{"text", "image", "video"}, 1048576, 131072, false},
		{"gemini-4-argon", "gemini", []string{"text", "image", "audio", "video", "pdf"}, 1048576, 65536, false},
	} {
		var found bool
		for _, model := range Models() {
			if model.ID != test.id {
				continue
			}
			found = true
			if !modelSupports(test.id, test.protocol) || model.ContextWindow != test.window || model.MaxOutput != test.output ||
				model.Kind != "chat" || !reflect.DeepEqual(model.InputModalities, test.modalities) ||
				!reflect.DeepEqual(model.OutputModalities, []string{"text"}) ||
				model.Temperature == nil || *model.Temperature != test.temperature || model.Released == "" {
				t.Errorf("packaged model %s changed donor metadata", test.id)
			}
		}
		if !found {
			t.Errorf("missing donor model %s", test.id)
		}
	}
	if modelSupports("gemini-4-argon", "chat") || modelSupports("kimi-k3", "gemini") ||
		modelSupports("glm-5.2", "chat") || modelSupports("unknown", "chat") {
		t.Fatal("packaged models advertise an unestablished protocol")
	}
}

func TestNativeGeminiUsesSelectedCredentialAndExactEndpoint(t *testing.T) {
	const payload = `{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"generationConfig":{"thinkingConfig":{"thinkingLevel":"HIGH"}}}`
	var calls atomic.Int32
	client, _ := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		count := calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		want := "/inference/go/google/v1beta/models/gemini-4-argon:generateContent"
		if count == 2 {
			want = "/inference/go/google/v1beta/models/gemini-4-argon:streamGenerateContent?alt=sse"
		}
		if r.Method != "POST" || r.URL.RequestURI() != want || string(body) != payload ||
			r.Header.Get("X-Goog-Api-Key") != "access-a" || r.Header.Get("Authorization") != "" ||
			r.Header.Get("X-Opencode-Org-Id") != "org-a" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Api-Key") != "" {
			t.Error("native Gemini request changed protocol bytes or selected authority")
		}
		io.WriteString(w, `{"candidates":[]}`)
	})
	headers := http.Header{"Authorization": {"Bearer other"}, "x-goog-api-key": {"foreign"}, "Cookie": {"other=private"}, "X-Api-Key": {"other"}}
	account := credential(time.Now(), "a", "org-a")
	for _, streaming := range []bool{false, true} {
		response, err := client.GenerateContent(t.Context(), account, headers, []byte(payload), "gemini-4-argon", streaming)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	if _, err := client.GenerateContent(t.Context(), account, headers, []byte(payload), "kimi-k3", false); err == nil || calls.Load() != 2 {
		t.Fatal("chat model dispatched on a Gemini endpoint")
	}
	account.Expires = time.Now()
	if _, err := client.GenerateContent(t.Context(), account, headers, []byte(payload), "gemini-4-argon", false); err == nil || calls.Load() != 2 {
		t.Fatal("expired account dispatched a Gemini request")
	}
	if headers.Get("Authorization") != "Bearer other" || headers["x-goog-api-key"][0] != "foreign" {
		t.Fatal("Gemini request mutated caller headers")
	}
}

func TestGeminiTerminalRequiresEveryCandidateAndPreservesNativeEndings(t *testing.T) {
	for _, test := range []struct {
		name     string
		count    int
		events   string
		complete bool
		bad      bool
	}{
		{"stop", 1, "data: {\"candidates\":[{\"index\":0,\"finishReason\":\"STOP\"}]}\r\n\r\n", true, false},
		{"tool-call", 1, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"f\",\"args\":{}}}]},\"finishReason\":\"STOP\"}]}\n\n", true, false},
		{"blocked", 1, "data: {\"promptFeedback\":{\"blockReason\":\"SAFETY\"}}\n\n", true, false},
		{"two", 2, "data: {\"candidates\":[{\"index\":0,\"finishReason\":\"STOP\"}]}\n\ndata: {\"candidates\":[{\"index\":1,\"finishReason\":\"MAX_TOKENS\"}]}\n\n", true, false},
		{"one-of-two", 2, "data: {\"candidates\":[{\"index\":0,\"finishReason\":\"STOP\"}]}\n\n", false, false},
		{"text-only", 1, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"finishReason STOP\"}]}}]}\n\n", false, false},
		{"unspecified", 1, "data: {\"candidates\":[{\"finishReason\":\"FINISH_REASON_UNSPECIFIED\"}]}\n\n", false, false},
		{"unterminated", 1, "data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n", false, false},
		{"foreign-candidate", 1, "data: {\"candidates\":[{\"index\":2,\"finishReason\":\"STOP\"}]}\n\n", false, true},
		{"bad-json", 1, "data: {broken}\n\n", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var terminal geminiTerminal
			body := map[string]json.RawMessage{"contents": raw([]any{map[string]any{"parts": []any{map[string]string{"text": "hello"}}}}),
				"generationConfig": raw(map[string]int{"candidateCount": test.count})}
			if err := terminal.configure(body); err != nil {
				t.Fatal(err)
			}
			var parseErr error
			for _, value := range []byte(test.events) {
				if parseErr = terminal.write([]byte{value}, nil); parseErr != nil {
					break
				}
			}
			if (parseErr != nil) != test.bad || terminal.complete() != test.complete {
				t.Fatalf("native Gemini terminal: err=%v complete=%v", parseErr, terminal.complete())
			}
		})
	}
	var terminal geminiTerminal
	if err := terminal.configure(map[string]json.RawMessage{"contents": raw([]any{map[string]string{"role": "user"}})}); err != nil {
		t.Fatal(err)
	}
	if err := terminal.write([]byte("data: "+strings.Repeat("x", maxReplyBytes)), nil); err == nil ||
		len(terminal.frame) > maxReplyBytes {
		t.Fatal("native Gemini stream event is not bounded")
	}
}

func TestPublicSDKGeminiNativeStreamsAndSelectedAccounts(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		name   string
		status int
		events string
		code   string
	}{
		{"complete", 200, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hello\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"totalTokenCount\":5}}\n\n", ""},
		{"truncated", 200, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"incomplete\"}]}}]}\n\n", "truncated"},
		{"upstream-error", 429, "data: {\"error\":{\"message\":\"quota\"}}\n\n", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if serveFixtureGoCatalog(w, r) {
					return
				}
				count := calls.Add(1)
				user := "a"
				if count == 2 {
					user = "b"
				}
				if r.URL.RequestURI() != "/inference/go/google/v1beta/models/gemini-4-argon:streamGenerateContent?alt=sse" ||
					r.Header.Get("X-Goog-Api-Key") != "access-"+user || r.Header.Get("Authorization") != "" ||
					r.Header.Get("X-Opencode-Org-Id") != "org-"+user {
					t.Error("native SDK request crossed protocol or account boundaries")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(test.status)
				io.WriteString(w, test.events)
			}))
			t.Cleanup(server.Close)
			h := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
				Now: func() time.Time { return now }, HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }})
			for _, user := range []string{"a", "b"} {
				account := credential(now, user, "org-"+user)
				request := sdk.ExecuteRequest{Protocol: "gemini", Model: "gemini-4-argon",
					Body:    raw(map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]string{"text": "hello"}}}}}),
					Headers: http.Header{"Authorization": {"Bearer foreign"}, "X-Goog-Api-Key": {"foreign"}},
					Options: raw(map[string]string{"method": "POST", "path": "/v1beta/models/gemini-4-argon:streamGenerateContent", "escapedPath": "/v1beta/models/gemini-4-argon:streamGenerateContent", "query": "alt=sse"})}
				frame, body := h.invoke(sdk.MethodProviderExecute, sdk.Scope{Provider: ProviderGo, Account: account.AccountID(ProviderGo), Generation: 1}, request, raw(account))
				if test.code == "" {
					if frame.Error != nil || string(body) != test.events {
						t.Fatalf("native Gemini response changed: %v", frame.Error)
					}
				} else if frame.Error == nil || frame.Error.Code != test.code || frame.Error.Outcome != "uncertain" {
					t.Fatalf("native Gemini truncation lost its uncertain classification: %v", frame.Error)
				}
				if !h.accepted[frame.ID] || h.headers[len(h.headers)-1].Status != test.status {
					t.Fatal("native generation bypassed acceptance or response status")
				}
			}
			if calls.Load() != 2 || len(h.proxies) != 4 {
				t.Fatal("native Gemini retried or bypassed selected proxy resolution")
			}
		})
	}
}

func TestPublicSDKGeminiChecksStreamAfterFinishReason(t *testing.T) {
	const stop = "data: {\"candidates\":[{\"index\":0,\"finishReason\":\"STOP\"}]}\n\n"
	const usage = "data: {\"usageMetadata\":{\"totalTokenCount\":5}}\n\n"
	for _, provider := range []string{ProviderGo, ProviderZen} {
		t.Run(provider, func(t *testing.T) {
			for _, fixture := range []struct {
				name  string
				tail  string
				split int
				short bool
				valid bool
			}{
				{name: "late-usage", tail: usage, valid: true},
				{name: "partial-line", tail: `data: {"usageMetadata":`},
				{name: "partial-event", tail: strings.TrimSuffix(usage, "\n")},
				{name: "transport-error", short: true},
				{name: "late-error", tail: "data: {\"error\":{\"code\":503,\"message\":\"service unavailable\",\"status\":\"UNAVAILABLE\"}}\n\n"},
				{name: "same-read-error", tail: "data: {\"error\":{\"code\":503}}\n\n", split: -1},
				{name: "split-within-error", tail: "data: {\"error\":{\"code\":503}}\n\n", split: 7},
			} {
				t.Run(fixture.name, func(t *testing.T) {
					model := "gemini-4-argon"
					if provider == ProviderZen {
						model = "gemini-3.1-pro"
					}
					var calls atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
						if serveFixtureGoCatalog(writer, request) {
							return
						}
						if request.URL.Path == "/inference/v1/models" {
							io.WriteString(writer, `{"data":[{"id":"`+model+`"}]}`)
							return
						}
						calls.Add(1)
						writer.Header().Set("Content-Type", "text/event-stream")
						if fixture.short {
							writer.Header().Set("Content-Length", strconv.Itoa(len(stop)+1))
						}
						io.WriteString(writer, stop+fixture.tail)
					}))
					t.Cleanup(server.Close)
					firstBytes := len(stop) + fixture.split
					if fixture.split < 0 {
						firstBytes = len(stop) + len(fixture.tail)
					}
					options := ModuleOptions{
						Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
						HTTPFactory: func(string) (Doer, func(), error) {
							return geminiSplitClient{server.Client(), firstBytes}, nil, nil
						},
					}
					scope := sdk.Scope{Provider: provider, Generation: 1}
					var module sdk.Module
					var auth json.RawMessage
					if provider == ProviderGo {
						account := credential(time.Now(), "a", "org-a")
						scope.Account, auth = account.AccountID(provider), raw(account)
						module = NewModule(options)
					} else {
						account := ZenCredentials{Kind: "key", Key: "fixture-zen-key"}
						scope, auth = zenScope(provider, account), raw(account)
						module = NewWindowModule(options)
					}
					harness := serveHarness(t, module, raw(moduleConfig{ProjectID: "global"}))
					frame, actual := harness.invoke(sdk.MethodProviderExecute, scope, sdk.ExecuteRequest{
						Protocol: "gemini", Model: model, Body: json.RawMessage(`{"contents":[{"parts":[{"text":"hello"}]}]}`),
						Options: raw(sdk.ExecuteOptions{Path: "/v1beta/models/" + model + ":streamGenerateContent", Query: "alt=sse"}),
					}, auth)
					if calls.Load() != 1 || !harness.accepted[frame.ID] {
						t.Fatalf("Gemini dispatch count=%d, error=%v", calls.Load(), frame.Error)
					}
					if fixture.valid {
						if frame.Error != nil || string(actual) != stop+fixture.tail {
							t.Fatalf("Gemini lost final usage: %q, %v", actual, frame.Error)
						}
					} else if frame.Error == nil || frame.Error.Outcome != "uncertain" {
						t.Fatalf("Gemini accepted a failed stream: result=%s, error=%v", frame.Result, frame.Error)
					}
					if !fixture.valid && string(actual) != stop {
						t.Fatalf("Gemini failed event changed the valid prefix: %q", actual)
					}
				})
			}
		})
	}
}

type geminiSplitClient struct {
	*http.Client
	firstBytes int
}

func (client geminiSplitClient) Do(request *http.Request) (*http.Response, error) {
	response, err := client.Client.Do(request)
	if err == nil && strings.HasSuffix(request.URL.Path, ":streamGenerateContent") {
		first := make([]byte, client.firstBytes)
		if _, err := io.ReadFull(response.Body, first); err != nil {
			response.Body.Close()
			return nil, err
		}
		response.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(first), response.Body), response.Body}
	}
	return response, err
}

func TestPublicSDKRejectsUnsupportedGeminiSemanticsBeforeDispatch(t *testing.T) {
	var catalogs, dispatches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/models" {
			catalogs.Add(1)
			io.WriteString(w, `{"opencode-go":{"id":"opencode-go","models":{
				"gemini-4-argon":{"id":"gemini-4-argon"},"kimi-k3":{"id":"kimi-k3"}
			}}}`)
			return
		}
		dispatches.Add(1)
		t.Errorf("unexpected OpenCode request: %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(server.Close)
	h := newHarness(t, ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }})
	account := credential(time.Now(), "a", "org-a")
	for _, test := range []struct {
		protocol string
		model    string
		path     string
		query    string
		body     string
	}{
		{"chat", "gemini-4-argon", "/v1/chat/completions", "", `{"model":"gemini-4-argon","messages":[]}`},
		{"gemini", "kimi-k3", "/v1beta/models/kimi-k3:generateContent", "", `{"contents":[{}]}`},
		{"gemini", "gemini-4-argon", "/v1beta/models/other:generateContent", "", `{"contents":[{}]}`},
		{"gemini", "gemini-4-argon", "/v1beta/models/gemini-4-argon:countTokens", "", `{"contents":[{}]}`},
		{"gemini", "gemini-4-argon", "/v1beta/models/gemini-4-argon:streamGenerateContent", "key=foreign", `{"contents":[{}]}`},
		{"gemini", "gemini-4-argon", "/v1beta/models/gemini-4-argon:streamGenerateContent", "", `{"contents":[{}]}`},
		{"gemini", "gemini-4-argon", "/v1beta/models/gemini-4-argon:generateContent", "", `{"model":"other","contents":[{}]}`},
		{"gemini", "gemini-4-argon", "/v1beta/models/gemini-4-argon:generateContent", "", `{"contents":[]}`},
		{"gemini", "gemini-4-argon", "/v1beta/models/gemini-4-argon:generateContent", "", `{"contents":[{}],"generationConfig":{"candidateCount":0}}`},
	} {
		frame, _ := h.invoke(sdk.MethodProviderExecute, sdk.Scope{Provider: ProviderGo, Account: account.AccountID(ProviderGo), Generation: 1},
			sdk.ExecuteRequest{Protocol: test.protocol, Model: test.model, Body: []byte(test.body),
				Options: raw(map[string]string{"path": test.path, "query": test.query})}, raw(account))
		if frame.Error == nil || frame.Error.Outcome != "not-started" || dispatches.Load() != 0 || h.accepted[frame.ID] {
			t.Fatalf("unsupported Gemini semantics reached upstream: %+v", frame.Error)
		}
	}
	if catalogs.Load() != 1 {
		t.Fatalf("OpenCode did not reuse the selected catalog, requests: %d", catalogs.Load())
	}
}
