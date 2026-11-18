package devin

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type cachePeerHost struct {
	store     stateFixture
	scheduled []sdk.AccountTaskScheduleRequest
	canceled  []sdk.AccountTaskCancelRequest
	proxy     string
	order     []string
}

func (host *cachePeerHost) call(frame sdk.Frame) (any, error) {
	if len(frame.Auth) != 0 {
		return nil, fmt.Errorf("task callback exposed credentials")
	}
	host.order = append(host.order, frame.Method)
	switch frame.Method {
	case sdk.MethodAccountTaskCancel:
		var request sdk.AccountTaskCancelRequest
		if json.Unmarshal(frame.Params, &request) != nil {
			return nil, fmt.Errorf("invalid task cancellation")
		}
		host.canceled = append(host.canceled, request)
		return sdk.AccountTaskCancelResult{Canceled: true}, nil
	case sdk.MethodAccountTaskSchedule:
		request, err := sdk.DecodeAccountTaskSchedule(frame.Params)
		if err != nil {
			return nil, err
		}
		host.scheduled = append(host.scheduled, request)
		return sdk.AccountTaskScheduleResult{OperationID: "fixture-task"}, nil
	case sdk.MethodNetworkProxy:
		return sdk.NetworkProxyResult{URL: host.proxy}, nil
	default:
		return host.store.host(frame)
	}
}

func cachePeer(t *testing.T, server string, configuration string, options ModuleOptions) (*sdkPeer, *cachePeerHost) {
	t.Helper()
	options.Server = server
	module, err := NewAgent(options)
	if err != nil {
		t.Fatal(err)
	}
	peer := startSDKPeer(t, module, json.RawMessage(configuration))
	host := &cachePeerHost{store: stateFixture{rows: map[string]sdk.StateValue{}}}
	peer.host = host.call
	selectAccount(peer, server)
	return peer, host
}

func cacheExecute(session string) sdk.ExecuteRequest {
	request := fixtureExecute([]byte(`{"model":"fixture-model","max_tokens":64,"messages":[{"role":"user","content":"cache-prefix"}]}`),
		sdk.ExecuteOptions{CallerID: "fixture-caller"})
	request.Headers = http.Header{"X-Magpie-Session": {session}}
	return request
}

func cacheTaskInput(request sdk.AccountTaskScheduleRequest) sdk.ModuleCallRequest {
	input, _ := json.Marshal(sdk.AccountTaskRequest{
		Task: request.Task, Key: request.Key, Job: request.Job, DueAt: request.DueAt, Model: "fixture-model",
	})
	return sdk.ModuleCallRequest{Action: cacheTaskAction, Input: input}
}

func cacheResponse(writer http.ResponseWriter, hit bool) {
	read := uint64(17)
	if !hit {
		read = 0
	}
	writer.Header().Set("Content-Type", "application/connect+proto")
	usage := nativeFields(nativeNumber(2, 7), nativeNumber(3, 1), nativeNumber(4, 5), nativeNumber(5, read))
	_, _ = writer.Write(nativeFrame(0, nativeFields(nativeText(3, "answer"), nativeText(7, string(usage)), nativeNumber(5, 2))))
	_, _ = writer.Write(nativeFrame(2, []byte(`{}`)))
}

func cacheEvents(t *testing.T, events []sdk.Frame) (int, []sdk.OwnCall) {
	t.Helper()
	accepted := 0
	var measured []sdk.OwnCall
	for _, frame := range events {
		switch frame.Event {
		case "accepted":
			accepted++
		case sdk.EventAccountTaskUsage:
			var call sdk.OwnCall
			if json.Unmarshal(frame.Data, &call) != nil || call.Kind != "cache_keepalive" ||
				call.Model != "fixture-model" || call.Usage.Facts == nil {
				t.Fatal("task lost measured selected-account usage")
			}
			measured = append(measured, call)
		}
	}
	return accepted, measured
}

func TestCacheTasksDefaultOffAndTimingAloneCannotSchedule(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		cacheResponse(writer, true)
	}))
	defer server.Close()
	for _, configuration := range []string{`{}`, `{"keepAlive":false}`,
		`{"cacheKeepaliveInterval":30,"cacheKeepaliveMaxPings":1}`} {
		t.Run(configuration, func(t *testing.T) {
			peer, host := cachePeer(t, server.URL, configuration, ModuleOptions{})
			result, _ := peer.call(t, sdk.MethodProviderExecute, cacheExecute("default-off"))
			if result.Error != nil || len(host.canceled) != 0 || len(host.scheduled) != 0 {
				t.Fatal("default-off changed ordinary foreground execution or created paid work", result)
			}
		})
	}
	if requests.Load() != 3 {
		t.Fatal("default-off duplicated foreground requests", requests.Load())
	}
}

func TestCacheTaskKeepsBytesUsesFreshProxyAndFencesReplay(t *testing.T) {
	var payloads [][]byte
	var payloadMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, payload, err := codec.ReadConnectFrame(request.Body, true)
		if err != nil || request.Header.Get("Authorization") != "Basic selected-fixture-secret" {
			t.Error("task changed framing or selected credentials", err)
		}
		payloadMu.Lock()
		payloads = append(payloads, bytes.Clone(payload))
		payloadMu.Unlock()
		cacheResponse(writer, true)
	}))
	defer server.Close()
	sent := time.Unix(1800000000, 0)
	var proxies []string
	peer, host := cachePeer(t, server.URL, `{"keepAlive":true,"cacheKeepaliveInterval":30,"cacheKeepaliveMaxPings":2}`,
		ModuleOptions{Now: func() time.Time { return sent }, HTTPFactory: func(proxy string) (http.RoundTripper, func(), error) {
			proxies = append(proxies, proxy)
			return http.DefaultTransport, func() {}, nil
		}})
	host.proxy = "http://foreground-proxy.invalid:1"
	result, events := peer.call(t, sdk.MethodProviderExecute, cacheExecute("byte-proof"))
	accepted, _ := cacheEvents(t, events)
	if result.Error != nil || accepted != 1 || len(host.canceled) != 1 || len(host.scheduled) != 1 ||
		host.order[0] != sdk.MethodAccountTaskCancel {
		t.Fatal("foreground failed to bind before dispatch or arm exactly once", result, host.order)
	}
	planned := host.scheduled[0]
	if planned.DueAt != sent.Add(30*time.Second).UnixMilli() || planned.Task != cacheTaskID ||
		planned.Key != host.canceled[0].Key {
		t.Fatal("schedule changed conversation or send-time cadence", planned)
	}
	peer.scope.Operation = "paid-fixture-op"
	host.proxy = "http://fresh-task-proxy.invalid:2"
	result, events = peer.call(t, sdk.MethodModuleCall, cacheTaskInput(planned))
	accepted, measured := cacheEvents(t, events)
	var completed sdk.AccountTaskResult
	if result.Error != nil || json.Unmarshal(result.Result, &completed) != nil ||
		completed.Outcome != "completed" || completed.Call == nil || accepted != 1 ||
		len(measured) == 0 || len(host.scheduled) != 2 {
		t.Fatal("paid tick lost acceptance, measured usage or successor", result, events)
	}
	if !reflect.DeepEqual(proxies, []string{"http://foreground-proxy.invalid:1", "http://fresh-task-proxy.invalid:2"}) {
		t.Fatal("task retained the foreground proxy", proxies)
	}
	payloadMu.Lock()
	if len(payloads) != 2 {
		t.Fatal("unexpected paid dispatch count", len(payloads))
	}
	template, err := codec.NewCacheKeepaliveTemplate(payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	expected, err := template.Payload("selected-fixture-secret")
	if err != nil {
		t.Fatal(err)
	}
	actual := bytes.Clone(payloads[1])
	payloadMu.Unlock()
	fields, err := codec.RawFields(expected)
	received, receivedErr := codec.RawFields(actual)
	if err != nil || receivedErr != nil || len(fields) != len(received) {
		t.Fatal("task changed native field set")
	}
	for number, chunks := range fields {
		if number == 3 {
			original, _ := codec.RawFields(payloads[0])
			if !bytes.HasPrefix(received[number], original[number]) {
				t.Fatal("task changed original prompt IDs, text or cache marks")
			}
			extra := received[number][len(original[number]):]
			tag, used := binary.Uvarint(extra)
			size, count := binary.Uvarint(extra[used:])
			if tag != 3<<3|2 || size != uint64(len(extra)-used-count) {
				t.Fatal("task did not append exactly one prompt")
			}
			prompt, err := codec.RawFields(extra[used+count:])
			if err != nil || len(prompt) != 3 || len(prompt[1]) == 0 ||
				!bytes.Equal(prompt[2], nativeNumber(2, 1)) || !bytes.Equal(prompt[3], nativeText(3, "continue")) {
				t.Fatal("task changed continue role, identity, text or cache marks")
			}
		} else if !reflect.DeepEqual(chunks, received[number]) {
			t.Fatal("task changed prepared non-prompt bytes", number)
		}
	}
	last := measured[len(measured)-1].Usage.Facts
	if last.CachedTokens == nil || *last.CachedTokens != 17 || last.CacheWriteTokens == nil || *last.CacheWriteTokens != 5 {
		t.Fatal("paid cache usage disappeared", last)
	}
	result, events = peer.call(t, sdk.MethodModuleCall, cacheTaskInput(planned))
	accepted, _ = cacheEvents(t, events)
	if result.Error == nil || accepted != 0 {
		t.Fatal("replayed paid tick was accepted", result)
	}
	result, _ = peer.call(t, sdk.MethodModuleCall, cacheTaskInput(host.scheduled[1]))
	if result.Error != nil || len(host.scheduled) != 2 {
		t.Fatal("configured maximum was not terminal", result, host.scheduled)
	}
	payloadMu.Lock()
	defer payloadMu.Unlock()
	if len(payloads) != 3 || !bytes.Equal(payloads[1], payloads[2]) {
		t.Fatal("repeat ping appended history or replay reached the network")
	}
}

func TestCacheTaskRefusesChangedAuthorityAndUnplannedInputsBeforeAcceptance(t *testing.T) {
	for _, mutation := range []string{"account", "generation", "model", "key", "due", "job", "expired"} {
		t.Run(mutation, func(t *testing.T) {
			var dispatches atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				dispatches.Add(1)
				cacheResponse(writer, true)
			}))
			defer server.Close()
			peer, host := cachePeer(t, server.URL, `{"keepAlive":true}`, ModuleOptions{})
			result, _ := peer.call(t, sdk.MethodProviderExecute, cacheExecute("authority"))
			if result.Error != nil || len(host.scheduled) != 1 {
				t.Fatal("foreground did not arm", result)
			}
			input := cacheTaskInput(host.scheduled[0])
			var request sdk.AccountTaskRequest
			_ = json.Unmarshal(input.Input, &request)
			switch mutation {
			case "account":
				peer.scope.Account = "different-account"
			case "generation":
				peer.scope.Generation++
			case "model":
				request.Model = "other-model"
			case "key":
				request.Key = "other-conversation"
			case "due":
				request.DueAt++
			case "job":
				request.Job += ".replay"
			case "expired":
				peer.auth, _ = json.Marshal(outbound.Credentials{
					Key: "selected-fixture-secret", ExpiresAt: time.Now().Add(-time.Hour),
				})
			}
			input.Input, _ = json.Marshal(request)
			result, events := peer.call(t, sdk.MethodModuleCall, input)
			accepted, _ := cacheEvents(t, events)
			if result.Error == nil || accepted != 0 || dispatches.Load() != 1 || len(host.scheduled) != 1 {
				t.Fatal("changed task authority reached paid dispatch", result, dispatches.Load())
			}
		})
	}
}

func TestCacheTaskStopsOnTwoCompletedMissesOrFirstUncertainFailure(t *testing.T) {
	for _, behavior := range []string{"two-misses", "refusal", "malformed"} {
		t.Run(behavior, func(t *testing.T) {
			var dispatches atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if dispatches.Add(1) == 1 {
					cacheResponse(writer, true)
					return
				}
				switch behavior {
				case "two-misses":
					cacheResponse(writer, false)
				case "refusal":
					writer.WriteHeader(http.StatusTooManyRequests)
					_, _ = writer.Write([]byte(`{"code":"resource_exhausted"}`))
				case "malformed":
					writer.Header().Set("Content-Type", "application/connect+proto")
					_, _ = writer.Write(nativeFrame(0, nativeNumber(31, 1)))
					_, _ = writer.Write(nativeFrame(2, []byte(`{}`)))
				}
			}))
			defer server.Close()
			peer, host := cachePeer(t, server.URL, `{"keepAlive":true}`, ModuleOptions{})
			result, _ := peer.call(t, sdk.MethodProviderExecute, cacheExecute("stop-policy"))
			if result.Error != nil || len(host.scheduled) != 1 {
				t.Fatal("foreground failed", result)
			}
			first := host.scheduled[0]
			result, _ = peer.call(t, sdk.MethodModuleCall, cacheTaskInput(first))
			if behavior == "two-misses" {
				if result.Error != nil || len(host.scheduled) != 2 {
					t.Fatal("first completed miss lost its permitted successor", result)
				}
				result, _ = peer.call(t, sdk.MethodModuleCall, cacheTaskInput(host.scheduled[1]))
				if result.Error != nil || len(host.scheduled) != 2 || dispatches.Load() != 3 {
					t.Fatal("second miss did not stop warming", result)
				}
			} else if result.Error == nil || len(host.scheduled) != 1 || dispatches.Load() != 2 {
				t.Fatal("uncertain paid failure retried or rearmed", result)
			}
			before := dispatches.Load()
			result, _ = peer.call(t, sdk.MethodModuleCall, cacheTaskInput(first))
			if result.Error == nil || dispatches.Load() != before {
				t.Fatal("terminal paid task replay reached the vendor")
			}
		})
	}
}

func TestCacheTaskNextTurnCancelsAndJoinsHeldUsageWithoutCrossingConversations(t *testing.T) {
	var dispatches atomic.Int32
	heldCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count := dispatches.Add(1)
		if count != 3 {
			cacheResponse(writer, true)
			return
		}
		writer.Header().Set("Content-Type", "application/connect+proto")
		_, _ = writer.Write(executionFactsFixture())
		writer.(http.Flusher).Flush()
		<-request.Context().Done()
		close(heldCanceled)
	}))
	t.Cleanup(server.Close)
	peer, host := cachePeer(t, server.URL, `{"keepAlive":true}`, ModuleOptions{})
	for _, session := range []string{"first", "other"} {
		result, _ := peer.call(t, sdk.MethodProviderExecute, cacheExecute(session))
		if result.Error != nil {
			t.Fatal(result)
		}
	}
	if len(host.scheduled) != 2 || host.scheduled[0].Key == host.scheduled[1].Key {
		t.Fatal("independent conversations share a lane")
	}
	planned := host.scheduled[0]
	peer.next++
	paidID := strconv.Itoa(peer.next)
	raw, _ := json.Marshal(cacheTaskInput(planned))
	scope := peer.callScope()
	scope.Operation = "held-cache-task"
	peer.send(t, sdk.Frame{Kind: sdk.KindCall, ID: paidID, Method: sdk.MethodModuleCall, Params: raw, Scope: scope, Auth: peer.auth})
	var paidEvents []sdk.Frame
	foregroundID := ""
	results := map[string]sdk.Frame{}
	deadline := time.After(10 * time.Second)
	for len(results) < 2 {
		select {
		case frame := <-peer.frames:
			switch frame.Kind {
			case sdk.KindHostCall:
				value, err := host.call(frame)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := json.Marshal(value)
				peer.send(t, sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: frame.Parent, Result: body})
			case sdk.KindEvent:
				peer.send(t, sdk.Frame{Kind: sdk.KindAck, ID: frame.ID, Seq: frame.Seq})
				if frame.ID == paidID {
					paidEvents = append(paidEvents, frame)
					if frame.Event == sdk.EventAccountTaskUsage && foregroundID == "" {
						peer.next++
						foregroundID = strconv.Itoa(peer.next)
						body, _ := json.Marshal(cacheExecute("first"))
						peer.send(t, sdk.Frame{Kind: sdk.KindCall, ID: foregroundID, Method: sdk.MethodProviderExecute,
							Params: body, Scope: peer.callScope(), Auth: peer.auth})
					}
				}
			case sdk.KindResult:
				results[frame.ID] = frame
			default:
				t.Fatal("unexpected held task frame", frame.Kind)
			}
		case <-deadline:
			t.Fatal("new foreground failed to join its held paid task")
		}
	}
	accepted, measured := cacheEvents(t, paidEvents)
	if results[paidID].Error == nil || results[foregroundID].Error != nil || accepted != 1 ||
		len(measured) == 0 || measured[len(measured)-1].Usage.Facts.Cost == nil ||
		measured[len(measured)-1].Usage.Facts.Cost.Currency != "ACU" || dispatches.Load() != 4 {
		t.Fatal("held cancellation lost partial measured paid usage or duplicated dispatch", results, measured)
	}
	select {
	case <-heldCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled paid HTTP request is still running")
	}
	if len(host.canceled) != 3 || host.canceled[2].Key != planned.Key {
		t.Fatal("next turn canceled another conversation", host.canceled)
	}
	result, _ := peer.call(t, sdk.MethodModuleCall, cacheTaskInput(planned))
	if result.Error == nil || dispatches.Load() != 4 {
		t.Fatal("held callback rearmed after a newer foreground")
	}
	result, _ = peer.call(t, sdk.MethodModuleCall, cacheTaskInput(host.scheduled[1]))
	if result.Error != nil || dispatches.Load() != 5 {
		t.Fatal("another conversation was canceled", result, dispatches.Load())
	}
}
