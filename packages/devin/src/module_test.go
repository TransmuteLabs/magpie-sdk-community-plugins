package devin

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

type sdkPeer struct {
	input  *io.PipeWriter
	output *io.PipeReader
	cancel context.CancelFunc
	frames chan sdk.Frame
	done   chan error
	read   chan struct{}
	next   int
	mu     sync.Mutex
	id     string
	scope  sdk.Scope
	auth   json.RawMessage
	host   func(sdk.Frame) (any, error)
}

func newSDKPeer(t *testing.T) *sdkPeer {
	t.Helper()
	module, err := NewInbound()
	if err != nil {
		t.Fatal(err)
	}
	return startSDKPeer(t, module)
}

func startSDKPeer(t *testing.T, module sdk.Module, configuration ...json.RawMessage) *sdkPeer {
	t.Helper()
	input, send := io.Pipe()
	receive, output := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	p := &sdkPeer{input: send, output: receive, cancel: cancel, frames: make(chan sdk.Frame, 32),
		done: make(chan error, 1), read: make(chan struct{}), id: module.Manifest.ID}
	go func() { p.done <- sdk.Serve(ctx, input, output, module) }()
	go func() {
		defer close(p.read)
		decoder := json.NewDecoder(receive)
		for {
			var frame sdk.Frame
			if decoder.Decode(&frame) != nil {
				return
			}
			select {
			case p.frames <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		send.Close()
		receive.Close()
		select {
		case <-p.done:
		case <-time.After(10 * time.Second):
			t.Error("SDK fixture left the server running")
		}
		select {
		case <-p.read:
		case <-time.After(10 * time.Second):
			t.Error("SDK fixture left the reader running")
		}
	})
	values := json.RawMessage(`{}`)
	if len(configuration) > 0 {
		values = configuration[0]
	}
	result, events := p.call(t, sdk.MethodComponentConfigure, sdk.ComponentConfigureRequest{
		Component: sdk.PrimaryComponent, Instance: "devin-fixture",
		Config: sdk.ConfigSnapshot{Version: "0", Values: values},
	})
	if result.Error != nil || len(events) != 0 {
		t.Fatal("module configuration failed:", result)
	}
	return p
}

func (p *sdkPeer) send(t *testing.T, frame sdk.Frame) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := json.NewEncoder(p.input).Encode(frame); err != nil {
		t.Fatal(err)
	}
}

func (p *sdkPeer) call(t *testing.T, method string, params any) (sdk.Frame, []sdk.Frame) {
	t.Helper()
	p.next++
	id := strconv.Itoa(p.next)
	body, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	p.send(t, sdk.Frame{Kind: sdk.KindCall, ID: id, Method: method, Params: body,
		Scope: p.callScope(), Auth: p.auth})
	var events []sdk.Frame
	for {
		select {
		case frame := <-p.frames:
			if frame.Kind == sdk.KindHostCall && p.host != nil {
				value, err := p.host(frame)
				reply := sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: frame.Parent}
				if err != nil {
					reply.Error = &sdk.Error{Code: "permission_denied", Message: "Synthetic host rejected callback"}
				} else {
					reply.Result, _ = json.Marshal(value)
				}
				p.send(t, reply)
				continue
			}
			if frame.ID != id {
				t.Fatal("SDK response crossed call IDs:", frame)
			}
			if frame.Kind == sdk.KindEvent {
				events = append(events, frame)
				p.send(t, sdk.Frame{Kind: sdk.KindAck, ID: id, Seq: frame.Seq})
				continue
			}
			if frame.Kind != sdk.KindResult {
				t.Fatal("unexpected SDK frame:", frame)
			}
			return frame, events
		case <-time.After(10 * time.Second):
			t.Fatal("SDK fixture call did not complete:", method)
		}
	}
}

func (peer *sdkPeer) callScope() sdk.Scope {
	scope := peer.scope
	scope.Plugin = peer.id
	if scope.Session == "" {
		scope.Session = "synthetic"
	}
	return scope
}

type nativeResult struct {
	status  int
	headers http.Header
	body    []byte
}

func (result *nativeResult) events(t *testing.T, events []sdk.Frame) {
	t.Helper()
	for _, event := range events {
		switch event.Event {
		case "headers":
			var header sdk.HeadersEvent
			if err := json.Unmarshal(event.Data, &header); err != nil {
				t.Fatal(err)
			}
			result.status, result.headers = header.Status, header.Headers
		case "data":
			var body []byte
			if err := json.Unmarshal(event.Data, &body); err != nil {
				t.Fatal(err)
			}
			result.body = append(result.body, body...)
		case "flush":
		default:
			t.Fatal("unexpected native writer event:", event)
		}
	}
}

func fixtureChat(t *testing.T) []byte {
	t.Helper()
	body, err := hex.DecodeString("00000000141a0910011a0568656c6c6f720767726f75702f6d")
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func chatHeaders() http.Header {
	return http.Header{"Content-Type": {"application/connect+proto"}, "Connect-Protocol-Version": {"1"}}
}

func TestModuleExposesOnlyTheDeclaredInboundRoutes(t *testing.T) {
	module, err := NewInbound()
	if err != nil {
		t.Fatal(err)
	}
	if module.Manifest.Contract.Version != (sdk.APIVersion{Major: 1, Minor: 1}) ||
		!reflect.DeepEqual(module.Manifest.Contract.Capabilities, []sdk.Capability{sdk.InboundCodec, sdk.ModelDiscovery}) ||
		module.Manifest.Compatibility.NativeABI != 1 || module.Manifest.Compatibility.HostAPI != "^1.1" ||
		len(module.Manifest.Inbounds) != 8 || len(module.Inbounds) != 8 || len(module.Manifest.Providers) != 0 || len(module.Handlers) != 0 {
		t.Fatal("module exceeded its codec-only contract:", module.Manifest)
	}
	registry, err := protocolext.New(module.Inbounds...)
	if err != nil {
		t.Fatal(err)
	}
	for _, descriptor := range module.Manifest.Inbounds {
		adapter, found := registry.Lookup(descriptor.Path)
		if !found || adapter.ID() != descriptor.ID {
			t.Fatal("manifest and codec routes diverged:", descriptor)
		}
	}
}

func TestSDKConnectChatFixtureMatchesDirectCodec(t *testing.T) {
	p := newSDKPeer(t)
	headers, body := chatHeaders(), fixtureChat(t)
	headers.Set("Connect-Timeout-Ms", "1500")
	direct, err := (codec.Adapter{}).Decode(context.Background(), headers, body)
	if err != nil {
		t.Fatal(err)
	}
	frame, events := p.call(t, sdk.MethodInboundDecode, sdk.NativeRequest{
		ID: "devin", ResponseID: "chat", Headers: headers, Body: body,
	})
	var decoded sdk.DecodedRequest
	if frame.Error != nil || json.Unmarshal(frame.Result, &decoded) != nil || len(events) != 0 ||
		decoded.Protocol != string(direct.Protocol) || !bytes.Equal(decoded.Body, direct.Body) ||
		decoded.Session != direct.Session || decoded.TimeoutMs != direct.Timeout.Milliseconds() {
		t.Fatal("SDK changed the native decode:", frame, decoded)
	}
	canonical := []byte("data: {\"id\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n")
	directWriter := httptest.NewRecorder()
	reply := (codec.Adapter{}).Reply(directWriter)
	reply.Header().Set("Content-Type", "text/event-stream")
	reply.WriteHeader(200)
	result := nativeResult{}
	frame, events = p.call(t, sdk.MethodInboundResponse, sdk.ResponseEvent{
		ID: "devin", ResponseID: "chat", Action: sdk.ResponseStart,
		Status: 200, Headers: http.Header{"Content-Type": {"text/event-stream"}},
	})
	if frame.Error != nil {
		t.Fatal(frame)
	}
	result.events(t, events)
	for len(canonical) > 0 {
		size := min(23, len(canonical))
		part := canonical[:size]
		canonical = canonical[size:]
		if _, err := reply.Write(part); err != nil {
			t.Fatal(err)
		}
		frame, events = p.call(t, sdk.MethodInboundResponse, sdk.ResponseEvent{
			ID: "devin", ResponseID: "chat", Action: sdk.ResponseWrite, Body: part,
		})
		if frame.Error != nil {
			t.Fatal(frame)
		}
		result.events(t, events)
	}
	if err := reply.Finish(nil); err != nil {
		t.Fatal(err)
	}
	frame, events = p.call(t, sdk.MethodInboundResponse, sdk.ResponseEvent{ID: "devin", ResponseID: "chat", Action: sdk.ResponseFinish})
	if frame.Error != nil {
		t.Fatal(frame)
	}
	result.events(t, events)
	if result.status != directWriter.Code || result.headers.Get("Content-Type") != "application/connect+proto" ||
		!bytes.Equal(result.body, directWriter.Body.Bytes()) {
		t.Fatalf("SDK changed Connect framing: status=%d direct=%d body=%x direct=%x",
			result.status, directWriter.Code, result.body, directWriter.Body.Bytes())
	}
}

func TestSDKUnaryDiscoveryFixturesMatchDirectCodecs(t *testing.T) {
	for _, kind := range []codec.CatalogKind{codec.CliModels, codec.CommandModels, codec.CascadeModels,
		codec.CliModelsSite, codec.CommandModelsSite, codec.CascadeModelsSite, codec.ModelStatuses} {
		t.Run(string(kind), func(t *testing.T) {
			p := newSDKPeer(t)
			adapter := codec.CatalogAdapter{Kind: kind}
			frame, _ := p.call(t, sdk.MethodInboundDecode, sdk.NativeRequest{
				ID: adapter.ID(), ResponseID: "catalog",
				Headers: http.Header{"Content-Type": {"application/proto"}, "Connect-Protocol-Version": {"1"}},
			})
			var decoded sdk.DecodedRequest
			if frame.Error != nil || json.Unmarshal(frame.Result, &decoded) != nil || decoded.Protocol != "models" || len(decoded.Body) != 0 {
				t.Fatal("discovery left the guarded Models operation:", frame)
			}
			models := []byte(`{"object":"list","data":[{"id":"group/visible","object":"model","display_name":"Visible","context_window":32768,"max_output_tokens":4096,"modalities":{"input":["text","image"]}},{"id":"provider/visible","object":"model"}]}`)
			direct := httptest.NewRecorder()
			reply := adapter.Reply(direct)
			reply.Header().Set("Content-Type", "application/json")
			reply.WriteHeader(200)
			if _, err := reply.Write(models); err != nil {
				t.Fatal(err)
			}
			if err := reply.Finish(nil); err != nil {
				t.Fatal(err)
			}
			result := nativeResult{}
			for _, event := range []sdk.ResponseEvent{
				{ID: adapter.ID(), ResponseID: "catalog", Action: sdk.ResponseStart, Status: 200, Headers: http.Header{"Content-Type": {"application/json"}}},
				{ID: adapter.ID(), ResponseID: "catalog", Action: sdk.ResponseWrite, Body: models},
				{ID: adapter.ID(), ResponseID: "catalog", Action: sdk.ResponseFinish},
			} {
				frame, events := p.call(t, sdk.MethodInboundResponse, event)
				if frame.Error != nil {
					t.Fatal(frame)
				}
				result.events(t, events)
			}
			if result.status != 200 || result.headers.Get("Content-Type") != "application/proto" ||
				!bytes.Equal(result.body, direct.Body.Bytes()) {
				t.Fatal("SDK changed unary catalog framing:", result, direct.Body.Bytes())
			}
			if kind == codec.ModelStatuses && len(result.body) != 0 {
				t.Fatal("advisory statuses fabricated availability")
			}
		})
	}
}

func TestSDKPreservesNativeDeadlineHTTPStatus(t *testing.T) {
	headers := chatHeaders()
	headers.Set("Connect-Timeout-Ms", "0")
	_, err := (codec.Adapter{}).Decode(context.Background(), headers, fixtureChat(t))
	var direct *protocolext.Error
	if !errors.As(err, &direct) || direct.Status != 504 {
		t.Fatal("codec silently accepted an expired native deadline:", err)
	}
	p := newSDKPeer(t)
	frame, _ := p.call(t, sdk.MethodInboundDecode, sdk.NativeRequest{
		ID: "devin", ResponseID: "deadline", Headers: headers, Body: fixtureChat(t),
	})
	if frame.Error == nil || frame.Error.Code != "http.504" {
		t.Fatalf("SDK lost the codec's native HTTP %d rejection: error=%+v", direct.Status, frame.Error)
	}
}
