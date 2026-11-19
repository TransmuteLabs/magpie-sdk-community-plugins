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

func TestAgentManagedBundleReadPreservesVendorResponse(t *testing.T) {
	const rpc = "/exa.api_server_pb.ApiServerService/GetAccountManagedPluginBundle"
	kind := codec.AccountKind("managed-plugin-bundle")
	action := "devin.account." + string(kind)
	var requests atomic.Int32
	found := nativeFields(nativeNumber(1, 1), nativeText(2, "https://download.invalid/fixture"),
		nativeText(3, "owned-fixture-revision"), nativeText(4, string(nativeFields(
			nativeText(1, "X-Fixture-Signature"), nativeText(2, "owned-fixture-signature")))))
	missing := nativeNumber(1, 0)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		scope := uint64(requests.Add(1))
		payload, _ := io.ReadAll(request.Body)
		metadata, err := codec.NativeMetadata("selected-fixture-secret")
		if err != nil || request.Method != "POST" || request.URL.Path != rpc ||
			request.Header.Get("Authorization") != "Basic selected-fixture-secret-selected-fixture-secret" ||
			!bytes.Equal(payload, nativeFields(nativeText(1, string(metadata)),
				nativeText(2, "owned-fixture-bundle"), nativeNumber(3, scope))) {
			t.Error("native bundle read lost its selected-account authority or request fields")
		}
		writer.Header().Set("Content-Type", "application/proto")
		switch scope {
		case 2:
			_, _ = writer.Write(missing)
		case 3:
			_, _ = writer.Write(nativeText(2, "https://download.invalid/selected-fixture-secret"))
		default:
			_, _ = writer.Write(found)
		}
	}))
	defer server.Close()
	module, err := NewAgent(ModuleOptions{Server: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	operation, exists := module.Manifest.AccountOperation(string(kind))
	if !exists || operation.Provider != ProviderID || operation.Agent != AgentID || operation.Action != action {
		t.Fatal("bundle lookup is missing from the installed agent's account operations")
	}
	declared, exists := module.Manifest.ResolveAction(action)
	if !exists || declared.Effect != "read" {
		t.Fatal("bundle lookup is not a read action")
	}
	peer := startSDKPeer(t, module)
	store := &stateFixture{rows: map[string]sdk.StateValue{}}
	peer.host = store.host
	selectAccount(peer, server.URL)
	for index, want := range [][]byte{found, missing, nil} {
		input, _ := json.Marshal(sdk.AccountOperationRequest{Model: "model", CallerID: "fixture-owner",
			Body: nativeFields(nativeText(2, "owned-fixture-bundle"), nativeNumber(3, uint64(index+1)))})
		result, events := peer.call(t, sdk.MethodModuleCall, sdk.ModuleCallRequest{Action: action, Input: input})
		if len(events) != 0 {
			t.Fatalf("bundle lookup emitted a write or response event for scope %d", index+1)
		}
		if want == nil {
			if result.Error == nil || result.Error.Code != "data_loss" ||
				bytes.Contains(result.Result, []byte("selected-fixture-secret")) {
				t.Fatal("bundle lookup returned a response containing selected credentials")
			}
			continue
		}
		var reply sdk.AccountOperationResult
		if result.Error != nil || json.Unmarshal(result.Result, &reply) != nil || reply.Status != 200 ||
			reply.Headers.Get("Content-Type") != "application/proto" || !bytes.Equal(reply.Body, want) {
			t.Fatalf("bundle lookup changed vendor metadata for scope %d: %v", index+1, result.Error)
		}
	}
	if requests.Load() != 3 || store.writes != 0 {
		t.Fatal("bundle lookup dispatched a download, repeated a request or wrote state")
	}
}
