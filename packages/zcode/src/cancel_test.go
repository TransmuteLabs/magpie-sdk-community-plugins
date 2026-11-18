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

func TestBusinessCancelKeepsTargetAndSettlesOnceAcrossIntents(t *testing.T) {
	var settlements, inferences atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/oauth/profile":
			profile(writer, request, "fixture-user")
		case "/api/v1/off-peak/ticket":
			io.WriteString(writer, `{"ticket_id":"ticket","task_id":"fixture-session","state":"ready"}`)
		case "/api/v1/off-peak/anthropic/v1/messages":
			inferences.Add(1)
			io.WriteString(writer, `{"type":"message","content":[]}`)
		case "/api/v1/off-peak/ticket/ticket/settle":
			settlements.Add(1)
			io.WriteString(writer, `{"ticket_id":"ticket","task_id":"fixture-session","state":"settled"}`)
		default:
			t.Errorf("unexpected cancel fixture %s", request.URL.Path)
		}
	}))
	defer server.Close()
	config := moduleConfig{DataDir: filepath.Join(t.TempDir(), "controller")}
	account := nativeCredentials()
	h := newHarnessConfigured(t, endpointOptions(server), config)
	frame, _ := h.call(sdk.MethodProviderExecute, scope(account), sdk.ExecuteRequest{
		Protocol: "anthropic", Model: "GLM-5.3-OffPeak", Body: []byte(`{"model":"GLM-5.3-OffPeak","messages":[]}`),
	}, raw(account))
	if frame.Error != nil {
		t.Fatalf("cancel target execution: %+v", frame.Error)
	}
	target := frame.ID
	var reference nativeReference
	for range 2 {
		frame, _ = h.call(sdk.MethodOperationCancel, scope(account), sdk.OperationRequest{OperationID: target}, raw(account))
		var result sdk.OperationResult
		if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil ||
			result.OperationID != target || result.Outcome != "completed" || frame.ID == target {
			t.Fatalf("cancellation confused original execution with write intent: %+v", frame.Error)
		}
		var view executionView
		json.Unmarshal(result.Result, &view)
		reference.Reference = view.Reference
	}
	second := newHarnessConfigured(t, endpointOptions(server), config)
	second.next = 100
	frame, _ = second.call(sdk.MethodOperationCancel, scope(account), sdk.OperationRequest{Reference: raw(reference)}, raw(account))
	if frame.Error != nil || settlements.Load() != 1 || inferences.Load() != 1 {
		t.Fatalf("another controller replayed settlement or inference: %+v", frame.Error)
	}
}
