package zcode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func nativeCredentials() Credentials {
	account := renewableCredentials("individual")
	account.SessionToken = "fixture-zcode-session"
	account.SessionExpiresAt = fixtureNow.Add(time.Hour).UnixMilli()
	return account
}

func profile(writer http.ResponseWriter, request *http.Request, identity string) {
	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{"user": map[string]string{"user_id": identity}}})
}

func TestCandidateReadinessDoesNotTouchSharedVendorState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "uncreated", "vendor-state")
	newHarnessConfigured(t, ModuleOptions{}, moduleConfig{DataDir: path})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("Module.Start created or migrated shared vendor state during candidate validation")
	}
}

func TestCompletionNativeInferenceRetainsTurnAndRequestAttribution(t *testing.T) {
	var observed []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/off-peak/ticket":
			io.WriteString(writer, `{"ticket_id":"ticket","task_id":"fixture-session","state":"ready"}`)
		case "/api/v1/off-peak/anthropic/v1/messages":
			observed = append(observed, request.Clone(request.Context()))
			writer.Header().Set("Content-Type", "application/json")
			io.WriteString(writer, `{"type":"message","content":[]}`)
		default:
			t.Errorf("unexpected native attribution path %s", request.URL.Path)
			writer.WriteHeader(404)
		}
	}))
	defer server.Close()
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
	account := nativeCredentials()
	request := giftRequest("GLM-5.3-OffPeak")
	request.Body = []byte(`{"model":"GLM-5.3-OffPeak","messages":[{"role":"user","content":"first"}]}`)
	for range 2 {
		frame, _ := h.call(sdk.MethodProviderExecute, scope(account), request, raw(account))
		if frame.Error != nil {
			t.Fatalf("native attribution dispatch failed: %+v", frame.Error)
		}
	}
	for _, sent := range observed {
		for _, header := range []string{"X-Query-Id", "X-ZCode-Trace-Id", "X-Request-Id"} {
			if len(sent.Header.Get(header)) != 36 {
				t.Fatalf("native usage/log attribution omitted %s", header)
			}
		}
	}
	if len(observed) != 2 || observed[0].Header.Get("X-Query-Id") != observed[1].Header.Get("X-Query-Id") ||
		observed[0].Header.Get("X-ZCode-Trace-Id") != observed[1].Header.Get("X-ZCode-Trace-Id") ||
		observed[0].Header.Get("X-Request-Id") == observed[1].Header.Get("X-Request-Id") {
		t.Fatal("native continuation lost stable turn/trace or unique request attribution")
	}
}

func TestNativeAliasOwnsAdmissionAndReusesTicketAcrossPATGeneration(t *testing.T) {
	var tickets, inferences atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/off-peak/ticket":
			tickets.Add(1)
			io.WriteString(writer, `{"ticket_id":"ticket-one","task_id":"fixture-session","state":"ready"}`)
		case "/api/v1/off-peak/anthropic/v1/messages":
			inferences.Add(1)
			if request.Header.Get("Authorization") != "Bearer fixture-zcode-session" ||
				request.Header.Get("X-Api-Key") != "fixture-zcode-session" ||
				request.Header.Get("X-Off-Peak-Ticket-ID") != "ticket-one" ||
				request.Header.Get("X-Coding-Plan-Api-Key") == "" || request.Header.Get("X-Bigmodel-Authorization") != "" {
				t.Error("native ticket role or scope changed")
			}
			body, _ := io.ReadAll(request.Body)
			var sent map[string]any
			json.Unmarshal(body, &sent)
			if sent["model"] != "GLM-5.3" {
				t.Error("declared alias was not translated to its raw native model")
			}
			writer.Header().Set("Content-Type", "application/json")
			io.WriteString(writer, `{"type":"message","content":[{"type":"text","text":"native-ok"}]}`)
		default:
			t.Errorf("unexpected native fixture target %s %s", request.Method, request.URL.Path)
			writer.WriteHeader(404)
		}
	}))
	defer server.Close()
	directory := filepath.Join(t.TempDir(), "controller")
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: directory})
	account := nativeCredentials()
	request := sdk.ExecuteRequest{Protocol: "anthropic", Model: "GLM-5.3-OffPeak",
		Body: []byte(`{"model":"GLM-5.3-OffPeak","messages":[]}`)}
	frame, body := h.call(sdk.MethodProviderExecute, scope(account), request, raw(account))
	if frame.Error != nil || !strings.Contains(string(body), "native-ok") {
		t.Fatalf("native inference: %+v", frame.Error)
	}
	account.ProjectToken = "new-generation-pat"
	selected := scope(account)
	selected.Generation++
	frame, _ = h.call(sdk.MethodProviderExecute, selected, request, raw(account))
	if frame.Error != nil || tickets.Load() != 1 || inferences.Load() != 2 {
		t.Fatalf("PAT generation caused admission replay: %+v", frame.Error)
	}
}

func TestUnconfirmedAdmissionSurvivesNewOperationWithoutRetake(t *testing.T) {
	var tickets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/off-peak/ticket":
			tickets.Add(1)
			writer.WriteHeader(http.StatusServiceUnavailable)
		default:
			t.Error("unconfirmed admission reached inference or ordinary fallback")
		}
	}))
	defer server.Close()
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
	account := nativeCredentials()
	request := sdk.ExecuteRequest{Protocol: "anthropic", Model: "GLM-5.3-OffPeak",
		Body: []byte(`{"model":"GLM-5.3-OffPeak","messages":[]}`)}
	for range 2 {
		frame, _ := h.call(sdk.MethodProviderExecute, scope(account), request, raw(account))
		if frame.Error == nil || frame.Error.Outcome != "uncertain" {
			t.Fatalf("unconfirmed admission lost its fence: %+v", frame.Error)
		}
	}
	if tickets.Load() != 1 {
		t.Fatal("another operation ID caused a second ticket request")
	}
}

func TestWrongVendorPrincipalCannotInitializeController(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/oauth/profile" {
			t.Error("wrong principal reached vendor mutation")
		}
		profile(writer, request, "another-user")
	}))
	defer server.Close()
	directory := filepath.Join(t.TempDir(), "not-created")
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: directory})
	account := nativeCredentials()
	frame, _ := h.call(sdk.MethodProviderExecute, scope(account), sdk.ExecuteRequest{Protocol: "anthropic", Model: "GLM-5.3-OffPeak",
		Body: []byte(`{"model":"GLM-5.3-OffPeak","messages":[]}`)}, raw(account))
	if frame.Error == nil || frame.Error.Code != "permission_denied" {
		t.Fatal("wrong profile was accepted as an admission principal")
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("unverified profile initialized vendor state")
	}
}

func TestHighspeedDrawBindsProviderModelTaskAndBusinessRoles(t *testing.T) {
	var draws, inferences atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/highspeed/draw":
			draws.Add(1)
			var body map[string]any
			json.NewDecoder(request.Body).Decode(&body)
			if body["model"] != "GLM-5.3-Flash" || body["provider"] != "builtin:zai-coding-plan" ||
				body["task_id"] != "fixture-session" || request.Header.Get("X-Bigmodel-Authorization") != "Bearer fixture-business-token" {
				t.Error("Highspeed admission was not derived from the declared model and selected account")
			}
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "msg": "", "data": map[string]any{
				"next_draw_at": fixtureNow.Add(time.Minute).UnixMilli(),
				"card": map[string]any{"card_id": "card", "task_id": "fixture-session", "provider": "builtin:zai-coding-plan",
					"model": "GLM-5.3-Flash", "issued_at": fixtureNow.UnixMilli(), "expires_at": fixtureNow.Add(time.Hour).UnixMilli()},
			}})
		case "/api/v1/highspeed/anthropic/v1/messages":
			inferences.Add(1)
			if request.Header.Get("X-Highspeed-Card-ID") != "card" || request.Header.Get("X-Coding-Plan-Api-Key") != "" {
				t.Error("Highspeed inference mixed PAT and card roles")
			}
			writer.Header().Set("Content-Type", "application/json")
			io.WriteString(writer, `{"type":"message","content":[]}`)
		default:
			t.Error("unexpected Highspeed target")
		}
	}))
	defer server.Close()
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
	account := nativeCredentials()
	frame, _ := h.call(sdk.MethodProviderExecute, scope(account), sdk.ExecuteRequest{Protocol: "anthropic", Model: "GLM-5.3-Flash-Highspeed",
		Body: []byte(`{"model":"GLM-5.3-Flash-Highspeed","messages":[]}`)}, raw(account))
	if frame.Error != nil || draws.Load() != 1 || inferences.Load() != 1 {
		t.Fatalf("Highspeed: %+v", frame.Error)
	}
}
