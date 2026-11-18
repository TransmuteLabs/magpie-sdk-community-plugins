package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestZenPublicToolCompatibilityUsesSelectedNativeExecution(t *testing.T) {
	requests := make(chan []byte, 1)
	server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/inference/v1/models" {
			io.WriteString(writer, `{"data":[{"id":"future-free"}]}`)
			return
		}
		if request.Header.Get("Authorization") != "Bearer public" {
			t.Error("tool compatibility borrowed personal authority")
		}
		sent, _ := io.ReadAll(request.Body)
		requests <- sent
		writer.Header().Set("Content-Type", "application/json")
		io.WriteString(writer, `{"choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_fixture","type":"function","function":{"name":"exec_program","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
	}))
	defer server.Close()
	harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{ProjectID: "global"}))
	identity := ZenCredentials{Kind: "public", Key: "public"}
	request := sdk.ExecuteRequest{Protocol: "chat", Model: "future-free", Body: []byte(`{"model":"future-free","messages":[{"role":"user","content":"fixture"},{"role":"assistant","function_call":{"name":"run_code","arguments":"{}"}}],"tools":[{"type":"function","function":{"name":"run_code","parameters":{"type":"object"}}}],"tool_choice":{"type":"function","function":{"name":"run_code"}},"seed":9007199254740993}`)}
	frame, response := harness.invoke(sdk.MethodProviderExecute, zenScope(ProviderZenPublic, identity), request, raw(identity))
	if frame.Error != nil {
		t.Fatal("public native tool execution failed", frame.Error)
	}
	sent := <-requests
	if !bytes.Contains(sent, []byte(`"name":"exec_program"`)) || bytes.Contains(sent, []byte(`"name":"run_code"`)) ||
		!bytes.Contains(sent, []byte(`"required":["command"]`)) || !bytes.Contains(sent, []byte(`9007199254740993`)) {
		t.Fatal("public tool request did not preserve donor names/schemas/history/choice/large integers")
	}
	if !bytes.Contains(response, []byte(`"name":"run_code"`)) || bytes.Contains(response, []byte(`"name":"exec_program"`)) {
		t.Fatal("public tool reply did not restore the caller's declared name", string(response))
	}
	if strings.Contains(string(sent), "Working directory:") {
		t.Fatal("tool compatibility fabricated environment facts")
	}
}

func sameToolJSON(first, second []byte) bool {
	decode := func(body []byte) any {
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		var value any
		if decoder.Decode(&value) != nil {
			return nil
		}
		return value
	}
	left, _ := json.Marshal(decode(first))
	right, _ := json.Marshal(decode(second))
	return bytes.Equal(left, right)
}

type zenToolFixture struct {
	Name, Protocol, Request, ExpectedRequest, Reply, ExpectedReply string
	Streaming                                                      bool
}

func zenToolEvents(body []byte) ([][]byte, error) {
	reader, first := bufio.NewReader(bytes.NewReader(body)), true
	var events [][]byte
	for {
		frame, err := readZenToolFrame(reader, &first)
		if err == io.EOF {
			return events, nil
		}
		if err != nil {
			return nil, err
		}
		if len(frame.data) != 0 {
			events = append(events, frame.data)
		}
	}
}

func TestZenToolDonorParity(t *testing.T) {
	body, err := os.ReadFile("testdata/donors/zen-tools.json")
	var fixtures []zenToolFixture
	if err != nil || json.Unmarshal(body, &fixtures) != nil || len(fixtures) < 80 {
		t.Fatal("missing reviewed donor tool fixtures", err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			shaped, policy, err := prepareZenTools(fixture.Protocol, []byte(fixture.Request))
			if err != nil || !sameToolJSON(shaped, []byte(fixture.ExpectedRequest)) {
				t.Fatal("request diverged from pinned donor normalization", err, string(shaped))
			}
			var reply bytes.Buffer
			err = relayZenTools(t.Context(), strings.NewReader(fixture.Reply), policy, fixture.Protocol, fixture.Streaming,
				func(body []byte) error { _, err := reply.Write(body); return err })
			if err != nil {
				t.Fatal("reply diverged from pinned donor guard", err)
			}
			if !fixture.Streaming {
				if !sameToolJSON(reply.Bytes(), []byte(fixture.ExpectedReply)) {
					t.Fatal("JSON reply diverged from pinned donor guard", reply.String())
				}
				return
			}
			actual, err := zenToolEvents(reply.Bytes())
			expected, expectedErr := zenToolEvents([]byte(fixture.ExpectedReply))
			if err != nil || expectedErr != nil || len(actual) != len(expected) {
				t.Fatal("SSE event sequence diverged from pinned donor guard", err, expectedErr, reply.String())
			}
			for index := range actual {
				if !bytes.Equal(actual[index], expected[index]) && !sameToolJSON(actual[index], expected[index]) {
					t.Fatalf("SSE tool event %d diverged from pinned donor guard: %s; want %s", index, actual[index], expected[index])
				}
			}
		})
	}
}

func TestZenToolNativePublicStreamRestoresCallerNames(t *testing.T) {
	var dispatched atomic.Int32
	server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/inference/v1/models" {
			io.WriteString(writer, `{"data":[{"id":"fixture-free"}]}`)
			return
		}
		dispatched.Add(1)
		sent, _ := io.ReadAll(request.Body)
		if request.Header.Get("Authorization") != "Bearer public" || bytes.Contains(sent, []byte(`"name":"run_code"`)) {
			t.Error("stream did not use selected public tool policy")
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_fixture","type":"function","function":{"name":"exec_","arguments":"{"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"program","arguments":"}"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		} {
			io.WriteString(writer, "data: "+event+"\n\n")
			writer.(http.Flusher).Flush()
		}
		io.WriteString(writer, "data: [DONE]\n\n")
	}))
	defer server.Close()
	harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{ProjectID: "global"}))
	identity := ZenCredentials{Kind: "public", Key: "public"}
	request := sdk.ExecuteRequest{Protocol: "chat", Model: "fixture-free", Body: []byte(`{"model":"fixture-free","stream":true,"messages":[],"tools":[{"type":"function","function":{"name":"run_code","parameters":{"type":"object"}}}]}`)}
	frame, response := harness.invoke(sdk.MethodProviderExecute, zenScope(ProviderZenPublic, identity), request, raw(identity))
	if frame.Error != nil || dispatched.Load() != 1 || !bytes.Contains(response, []byte(`"name":"run_code"`)) ||
		bytes.Contains(response, []byte(`exec_`)) || !bytes.Contains(response, []byte("[DONE]")) {
		t.Fatal("fragmented native public tool reply lost caller identity or completion", frame.Error, string(response))
	}
	if len(harness.headers) != 1 || harness.headers[0].Headers.Get("Content-Length") != "" {
		t.Fatal("rewritten stream kept a stale byte length")
	}
}

func TestZenToolRefusalAndInvalidInputDoNotRetry(t *testing.T) {
	var dispatches atomic.Int32
	refusal := `{"error":{"type":"FreeUsageLimitError","message":"fixture refusal"}}`
	server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/inference/v1/models" {
			io.WriteString(writer, `{"data":[{"id":"fixture-free"}]}`)
			return
		}
		dispatches.Add(1)
		if request.Header.Get("Authorization") != "Bearer public" || request.Header.Get("x-api-key") != "" {
			t.Error("public refusal borrowed another identity")
		}
		writer.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(writer, refusal)
	}))
	defer server.Close()
	harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
		HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{ProjectID: "global"}))
	identity := ZenCredentials{Kind: "public", Key: "public"}
	request := sdk.ExecuteRequest{Protocol: "chat", Model: "fixture-free", Body: []byte(`{"model":"fixture-free","messages":[]}`)}
	frame, response := harness.invoke(sdk.MethodProviderExecute, zenScope(ProviderZenPublic, identity), request, raw(identity))
	var final sdk.ExecuteResult
	if frame.Error != nil || json.Unmarshal(frame.Result, &final) != nil || final.Refusal == nil ||
		final.Refusal.Facts.Type != "FreeUsageLimitError" || final.Refusal.Facts.Message != "fixture refusal" ||
		len(response) != 0 || dispatches.Load() != 1 || harness.accepted[frame.ID] {
		t.Fatal("public refusal was rewritten, retried or hidden", frame.Error, string(response))
	}
	request.Body = []byte(`{"model":"fixture-free","tools":null}`)
	frame, _ = harness.invoke(sdk.MethodProviderExecute, zenScope(ProviderZenPublic, identity), request, raw(identity))
	if frame.Error == nil || frame.Error.Outcome != "not-started" || dispatches.Load() != 1 {
		t.Fatal("invalid public tools reached inference", frame.Error)
	}
}

func TestZenToolPublicNativeErrorsRemainStrict(t *testing.T) {
	for _, test := range []struct{ name, reply, code string }{
		{"missing-terminal", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n", "truncated"},
		{"partial-event", "data: {\"choices\":[]}", "truncated"},
		{"upstream-error", "event: error\ndata: {\"error\":{\"message\":\"private fixture\"}}\n\n", "upstream"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var dispatches atomic.Int32
			server := newZenFixture(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/inference/v1/models" {
					io.WriteString(writer, `{"data":[{"id":"fixture-free"}]}`)
					return
				}
				dispatches.Add(1)
				writer.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(writer, test.reply)
			}))
			defer server.Close()
			harness := serveHarness(t, NewWindowModule(ModuleOptions{Endpoints: Endpoints{Console: server.URL, Inference: server.URL},
				HTTPFactory: func(string) (Doer, func(), error) { return server.Client(), nil, nil }}), raw(moduleConfig{ProjectID: "global"}))
			identity := ZenCredentials{Kind: "public", Key: "public"}
			frame, response := harness.invoke(sdk.MethodProviderExecute, zenScope(ProviderZenPublic, identity),
				sdk.ExecuteRequest{Protocol: "chat", Model: "fixture-free", Body: []byte(`{"model":"fixture-free","stream":true,"messages":[]}`)}, raw(identity))
			if frame.Error == nil || frame.Error.Code != test.code || frame.Error.Outcome != "uncertain" || dispatches.Load() != 1 ||
				bytes.Contains(response, []byte("[DONE]")) || bytes.Contains(response, []byte("private fixture")) {
				t.Fatal("public error became completion, leaked private detail or retried", frame.Error, string(response))
			}
		})
	}
}

type zenByteReader struct{ io.Reader }

func (reader zenByteReader) Read(body []byte) (int, error) {
	return reader.Reader.Read(body[:min(1, len(body))])
}

func TestZenToolBoundariesDoNotPublishInvalidCalls(t *testing.T) {
	_, policy, err := prepareZenTools("chat", []byte(`{"tools":[{"type":"function","function":{"name":"Read"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":-1,\"function\":{\"name\":\"Read\"}}]}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"type\":\"custom\",\"function\":{\"name\":\"Read\"}}]}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"function_call\":{\"name\":42}}}]}\n\n",
		"data: " + strings.Repeat("x", maxReplyBytes) + "\n\n",
		"data: {\"choices\":[]}\n",
		"data: {\"choices\":[]}\n\n",
	} {
		var published bytes.Buffer
		err := relayZenTools(t.Context(), strings.NewReader(source), policy, "chat", true,
			func(body []byte) error { _, err := published.Write(body); return err })
		if err == nil || bytes.Contains(published.Bytes(), []byte(`"function"`)) {
			t.Fatal("invalid or incomplete public tool stream was published", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := relayZenTools(ctx, strings.NewReader("{}"), policy, "chat", false, func([]byte) error {
		t.Fatal("canceled tool result was published")
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled public tool relay ignored cancellation", err)
	}
	for _, delimiter := range []string{"\n", "\r", "\r\n"} {
		body := "\xef\xbb\xbf: fixture" + delimiter + "id: preserved" + delimiter +
			"data: {\"choices\":[{\"delta\":{\"content\":\"Привет 🌍\"}}]}" + delimiter + delimiter +
			"data: [DONE]" + delimiter + delimiter
		var published bytes.Buffer
		err := relayZenTools(t.Context(), zenByteReader{strings.NewReader(body)}, policy, "chat", true,
			func(body []byte) error { _, err := published.Write(body); return err })
		if err != nil || published.String() != body {
			t.Fatal("fragmented UTF-8, SSE metadata or line endings changed", err, published.String())
		}
	}
	cause := errors.New("fixture consumer closed")
	calls := 0
	err = relayZenTools(t.Context(), strings.NewReader("data: {\"choices\":[]}\n\ndata: [DONE]\n\n"), policy, "chat", true, func([]byte) error {
		calls++
		return cause
	})
	if !errors.Is(err, cause) || calls != 1 {
		t.Fatal("tool relay ignored consumer backpressure failure", err, calls)
	}
}
