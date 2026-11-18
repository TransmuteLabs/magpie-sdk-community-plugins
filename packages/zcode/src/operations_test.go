package zcode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestLostTicketCanReconcileOnlyVendorAuthenticatedTaskThenSettleOnce(t *testing.T) {
	var tickets, inferences, settlements atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/off-peak/ticket":
			tickets.Add(1)
			writer.WriteHeader(http.StatusServiceUnavailable)
		case "/api/v1/off-peak/ticket/status":
			io.WriteString(writer, `{"tickets":[{"ticket_id":"ticket-one","task_id":"fixture-session","state":"ready"}]}`)
		case "/api/v1/off-peak/anthropic/v1/messages":
			inferences.Add(1)
			io.WriteString(writer, `{"type":"message","content":[]}`)
		case "/api/v1/off-peak/ticket/ticket-one/settle":
			settlements.Add(1)
			io.WriteString(writer, `{"ticket_id":"ticket-one","task_id":"fixture-session","state":"settled"}`)
		default:
			t.Error("unexpected reconciliation target")
		}
	}))
	defer server.Close()
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
	account := nativeCredentials()
	request := sdk.ExecuteRequest{Protocol: "anthropic", Model: "GLM-5.3-OffPeak",
		Body: []byte(`{"model":"GLM-5.3-OffPeak","messages":[]}`)}
	frame, _ := h.call(sdk.MethodProviderExecute, scope(account), request, raw(account))
	if frame.Error == nil || frame.Error.Outcome != "uncertain" {
		t.Fatal("fixture admission must remain uncertain")
	}
	frame, _ = h.call(sdk.MethodModuleCall, scope(account), actionInput("execution.list", struct{}{}), raw(account))
	var stored []executionView
	if frame.Error != nil || json.Unmarshal(frame.Result, &stored) != nil || len(stored) != 1 {
		t.Fatal("lost admission did not expose its durable opaque reference")
	}
	reference := map[string]string{"reference": stored[0].Reference}
	frame, _ = h.call(sdk.MethodModuleCall, scope(account), actionInput("execution.reconcile", map[string]any{
		"reference": reference, "ticketId": "ticket-one",
	}), raw(account))
	if frame.Error != nil {
		t.Fatalf("ticket reconciliation: %+v", frame.Error)
	}
	frame, _ = h.call(sdk.MethodProviderExecute, scope(account), request, raw(account))
	if frame.Error != nil {
		t.Fatalf("reconciled ticket execution: %+v", frame.Error)
	}
	for range 2 {
		frame, _ = h.call(sdk.MethodModuleCall, scope(account), actionInput("execution.settle", map[string]any{
			"reference": reference,
		}), raw(account))
		if frame.Error != nil {
			t.Fatalf("settlement: %+v", frame.Error)
		}
	}
	if tickets.Load() != 1 || inferences.Load() != 1 || settlements.Load() != 1 {
		t.Fatal("reconciliation retook a ticket, replayed inference or repeated settlement")
	}
}

func TestUntrustedTicketIDCannotRebindAnotherTask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/off-peak/ticket":
			writer.WriteHeader(http.StatusServiceUnavailable)
		case "/api/v1/off-peak/ticket/status":
			io.WriteString(writer, `{"tickets":[{"ticket_id":"foreign-ticket","task_id":"another-task","state":"ready"}]}`)
		default:
			t.Errorf("an untrusted task reached fixture %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()
	h := newHarnessConfigured(t, endpointOptions(server), moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")})
	account := nativeCredentials()
	h.call(sdk.MethodProviderExecute, scope(account), sdk.ExecuteRequest{Protocol: "anthropic", Model: "GLM-5.3-OffPeak",
		Body: []byte(`{"model":"GLM-5.3-OffPeak","messages":[]}`)}, raw(account))
	frame, _ := h.call(sdk.MethodModuleCall, scope(account), actionInput("execution.list", struct{}{}), raw(account))
	var stored []executionView
	json.Unmarshal(frame.Result, &stored)
	if len(stored) != 1 {
		t.Fatal("missing fixture reference")
	}
	frame, _ = h.call(sdk.MethodModuleCall, scope(account), actionInput("execution.reconcile", map[string]any{
		"reference": map[string]string{"reference": stored[0].Reference}, "ticketId": "foreign-ticket",
	}), raw(account))
	if frame.Error == nil || frame.Error.Outcome != "uncertain" {
		t.Fatal("caller ticket ID was treated as authoritative admission")
	}
}
