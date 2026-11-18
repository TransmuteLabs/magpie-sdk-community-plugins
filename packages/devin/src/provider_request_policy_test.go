package devin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestDonor044AmbiguousLongRefusalNeverInventsLengthOrReplay(t *testing.T) {
	for _, protocol := range []string{"chat", "anthropic"} {
		for _, size := range []int{1000, 420000} {
			t.Run(protocol+"/"+strconv.Itoa(size), func(t *testing.T) {
				var sends atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					sends.Add(1)
					writer.Header().Set("Content-Type", "application/connect+proto")
					writer.Write(nativeFrame(2, []byte(`{"error":{"code":"invalid_argument","message":"There is an issue with this request, please try a different model."}}`)))
				}))
				defer server.Close()
				peer, store := providerPeer(t, server.URL)
				selectAccount(peer, server.URL)
				callbacks := 0
				peer.host = func(frame sdk.Frame) (any, error) {
					if frame.Method == sdk.MethodRequestRecover {
						callbacks++
						var value sdk.RequestRecoveryRequest
						if json.Unmarshal(frame.Params, &value) != nil || value.Evidence != "" ||
							value.Facts.Code != "invalid_argument" || strings.Contains(value.Facts.Message, "too long") {
							t.Fatal("ambiguous refusal acquired fabricated length/effect evidence")
						}
						return sdk.RequestRecoveryResult{Step: value.Step, Retry: true, Action: "wait"}, nil
					}
					return store.host(frame)
				}
				body, _ := json.Marshal(map[string]any{"model": "fixture-model", "max_tokens": 64,
					"messages": []any{map[string]string{"role": "user", "content": strings.Repeat("x", size)}}})
				request := fixtureExecute(body, sdk.ExecuteOptions{CallerID: "caller"})
				if protocol == "anthropic" {
					request.Protocol = protocol
					request.Options, _ = json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: "/v1/messages", CallerID: "caller"})
				}
				result, events := peer.call(t, sdk.MethodProviderExecute, request)
				if result.Error == nil || result.Error.Code != "invalid_argument" || result.Error.Outcome != "uncertain" ||
					sends.Load() != 1 || callbacks != 1 {
					t.Fatal("ambiguous refusal was replayed or gained length/final refusal authority", result.Error, sends.Load(), callbacks)
				}
				accepted := 0
				for _, event := range events {
					if event.Event == "accepted" {
						accepted++
					}
					if strings.Contains(string(event.Data), "too long") {
						t.Fatal("request size invented a length verdict")
					}
				}
				if accepted != 1 {
					t.Fatal("ambiguous dispatched refusal lost its uncertainty fence", accepted)
				}
			})
		}
	}
}

func TestRequestPolicyFinalRefusalPreservesOriginalWireFacts(t *testing.T) {
	for _, protocol := range []string{"chat", "anthropic"} {
		for _, source := range []string{"connect", "http"} {
			t.Run(protocol+"/"+source, func(t *testing.T) {
				var sends atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					sends.Add(1)
					if source == "http" {
						writer.Header().Set("Content-Type", "application/json")
						writer.Header().Set("Retry-After", "60")
						writer.WriteHeader(429)
						io.WriteString(writer, `{"code":"resource_exhausted","type":"free_limit","message":"Reached free model rate limit. Please wait before retrying. Your limit will reset in 42 seconds"}`)
					} else {
						writer.Header().Set("Content-Type", "application/connect+proto")
						writer.Write(nativeFrame(2, []byte(`{"error":{"code":"resource_exhausted","message":"Reached free model rate limit. Please wait before retrying. Your limit will reset in 42 seconds"}}`)))
					}
				}))
				defer server.Close()
				peer, store := providerPeer(t, server.URL)
				selectAccount(peer, server.URL)
				callbacks := 0
				peer.host = func(frame sdk.Frame) (any, error) {
					if frame.Method == sdk.MethodRequestRecover {
						callbacks++
						var value sdk.RequestRecoveryRequest
						if json.Unmarshal(frame.Params, &value) != nil || value.Evidence != "" {
							t.Fatal("intermediate classification acquired replay authority", string(frame.Params))
						}
						return sdk.RequestRecoveryResult{Step: value.Step, Action: "next-account", Class: "quota", Scope: "model"}, nil
					}
					return store.host(frame)
				}
				request := fixtureExecute([]byte(`{"model":"fixture-model","stream":true,"max_tokens":10,"messages":[{"role":"user","content":"hello"}]}`), sdk.ExecuteOptions{CallerID: "caller"})
				if protocol == "anthropic" {
					request.Protocol = protocol
					request.Options, _ = json.Marshal(sdk.ExecuteOptions{Method: "POST", Path: "/v1/messages", CallerID: "caller"})
				}
				result, events := peer.call(t, sdk.MethodProviderExecute, request)
				var final sdk.ExecuteResult
				if result.Error != nil || json.Unmarshal(result.Result, &final) != nil || final.Refusal == nil {
					t.Fatal("complete pre-effect refusal lost final policy authority", result.Error, string(result.Result))
				}
				wantStatus := 200
				if source == "http" {
					wantStatus = 429
				}
				facts := final.Refusal.Facts
				if final.Outcome != "not-started" || final.Status != wantStatus || facts.Status != wantStatus ||
					!facts.Complete || facts.Code != "resource_exhausted" || !strings.Contains(facts.Message, "42 seconds") ||
					facts.Operation != sdk.MethodProviderExecute || facts.Phase != "response" ||
					facts.RetryAfterMS == nil || *facts.RetryAfterMS != 42000 ||
					final.Refusal.Evidence != "rejected-before-effect" || final.Refusal.Step != "devin.execute.chat" {
					t.Fatal("wire response was normalized or lost bounded timing/scope", final)
				}
				if source == "http" && (facts.Type != "free_limit" || facts.Headers["Retry-After"] != "60") ||
					source == "connect" && len(facts.Headers) != 0 {
					t.Fatal("original headers/type lost or synthetic HTTP timing invented", facts)
				}
				if len(final.Refusal.Windows) != 1 || len(final.Refusal.Windows[0].Models) != 1 ||
					final.Refusal.Windows[0].Models[0] != request.Model || len(final.Refusal.Windows[0].NotModels) != 0 {
					t.Fatal("free refusal inferred a shared bucket", final.Refusal.Windows)
				}
				for _, event := range events {
					switch event.Event {
					case "accepted", "headers", "data", "usage":
						t.Fatal("final refusal crossed the effect/publication fence", event)
					}
				}
				if sends.Load() != 1 || callbacks != 1 || store.writes != 0 {
					t.Fatal("module repeated work or published continuation state", sends.Load(), callbacks, store.writes)
				}
			})
		}
	}
}

func TestRequestPolicyPriorFrameOrAmbiguityNeverBecomesFinalRefusal(t *testing.T) {
	refusal := nativeFrame(2, []byte(`{"error":{"code":"resource_exhausted","message":"Reached free model rate limit. Your limit will reset in 42 seconds"}}`))
	for name, reply := range map[string][]byte{
		"empty frame":           append(nativeFrame(0, nil), refusal...),
		"measured zero":         append(nativeFrame(0, nativeText(7, string(nativeNumber(2, 0)))), refusal...),
		"output":                append(nativeFrame(0, nativeText(3, "paid answer")), refusal...),
		"trailing bytes":        append(bytes.Clone(refusal), 0),
		"truncated":             refusal[:len(refusal)-1],
		"unknown reason":        nativeFrame(2, []byte(`{"error":{"code":"resource_exhausted","message":"private server failure"}}`)),
		"conflicting output":    nativeFrame(2, []byte(`{"error":{"code":"resource_exhausted","message":"Reached free model rate limit"},"output":"already billed"}`)),
		"nested duplicate code": nativeFrame(2, []byte(`{"error":{"code":"unavailable","code":"resource_exhausted","message":"Reached free model rate limit"}}`)),
		"nested null type":      nativeFrame(2, []byte(`{"error":{"code":"resource_exhausted","type":null,"message":"Reached free model rate limit"}}`)),
		"nested null message":   nativeFrame(2, []byte(`{"error":{"code":"unauthenticated","message":null}}`)),
		"duplicate metadata":    nativeFrame(2, []byte(`{"error":{"code":"resource_exhausted","message":"Reached free model rate limit"},"metadata":{"trace":["first"],"trace":["second"]}}`)),
	} {
		t.Run(name, func(t *testing.T) {
			var sends atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				sends.Add(1)
				writer.Header().Set("Content-Type", "application/connect+proto")
				writer.Write(reply)
			}))
			defer server.Close()
			peer, _ := providerPeer(t, server.URL)
			selectAccount(peer, server.URL)
			result, events := peer.call(t, sdk.MethodProviderExecute, fixtureExecute(
				[]byte(`{"model":"fixture-model","stream":true,"messages":[{"role":"user","content":"hello"}]}`), sdk.ExecuteOptions{}))
			if result.Error == nil || result.Error.Outcome != "uncertain" || sends.Load() != 1 {
				t.Fatal("ambiguous or already started work became replayable", result.Error, string(result.Result), sends.Load())
			}
			accepted := 0
			for _, event := range events {
				if event.Event == "accepted" {
					accepted++
				}
			}
			if accepted != 1 {
				t.Fatal("uncertain execution lost its accepted fence", accepted)
			}
		})
	}
}
