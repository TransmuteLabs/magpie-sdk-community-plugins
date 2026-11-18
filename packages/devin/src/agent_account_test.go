package devin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestAgentAccountActionsKeepSelectedNativeAuthorityAndOrigins(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		payload, _ := io.ReadAll(request.Body)
		if request.Header.Get("Content-Type") != "application/proto" ||
			request.Header.Get("Authorization") != "Basic selected-fixture-secret-selected-fixture-secret" ||
			!bytes.Contains(payload, []byte("selected-fixture-secret")) || bytes.Contains(payload, []byte("local-key")) {
			t.Error("native account action lost its selected authority")
		}
		var body []byte
		switch request.URL.Path {
		case codec.UserStatus.Path():
			body = nativeText(1, string(nativeText(7, "fixture@example.invalid")))
		case codec.TeamSettings.Path():
			body = nativeNumber(1, 1)
		case codec.ManagedPlugins.Path():
			body = nativeText(1, `{"plugins":[]}`)
		case codec.ModelAssignment.Path():
			body = nativeText(1, string(nativeFields(nativeText(1, "assignment-fixture"), nativeText(2, "model"))))
		default:
			t.Error("unknown native action path")
		}
		writer.Header().Set("Content-Type", "application/proto")
		_, _ = writer.Write(body)
	}))
	defer server.Close()
	module, err := NewAgent(ModuleOptions{Server: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	peer := startSDKPeer(t, module)
	store := &stateFixture{rows: map[string]sdk.StateValue{}}
	peer.host = store.host
	selectAccount(peer, server.URL)
	for _, kind := range []codec.AccountKind{codec.UserStatus, codec.TeamSettings, codec.ManagedPlugins, codec.ModelAssignment} {
		request := sdk.AccountOperationRequest{Model: "model", CallerID: "fixture-owner"}
		if kind == codec.ModelAssignment {
			request.Body = nativeFields(nativeText(2, "model"), nativeText(3, "session"))
		}
		input, _ := json.Marshal(request)
		result, events := peer.call(t, sdk.MethodModuleCall, sdk.ModuleCallRequest{Action: "devin.account." + string(kind), Input: input})
		if result.Error != nil {
			t.Fatal(kind, result.Error)
		}
		var reply sdk.AccountOperationResult
		if json.Unmarshal(result.Result, &reply) != nil || reply.Status != 200 ||
			reply.Headers.Get("Content-Type") != "application/proto" || codec.ValidateAccountResponse(kind, reply.Body) != nil {
			t.Fatal("account action lost native response authority")
		}
		if kind == codec.ModelAssignment && (len(events) != 1 || events[0].Event != "accepted") ||
			kind != codec.ModelAssignment && len(events) != 0 {
			t.Fatal("native account action changed its declared effect")
		}
	}
	if requests.Load() != 4 || store.writes != 1 {
		t.Fatal("native bootstrap repeated a request or failed to retain its assignment origin")
	}
	var receipt originBucket
	for _, value := range store.rows {
		if json.Unmarshal(value.Value, &receipt) != nil {
			t.Fatal("invalid assignment origin")
		}
	}
	if len(receipt.Receipts) != 1 || receipt.Receipts[0].Kind != "assignment" ||
		receipt.Receipts[0].Generation != 3 || receipt.Receipts[0].Caller != "fixture-owner" ||
		receipt.Receipts[0].Model != "model" || receipt.Receipts[0].Format != "session" {
		t.Fatal("native assignment origin lost its account, caller or model scope")
	}
}
