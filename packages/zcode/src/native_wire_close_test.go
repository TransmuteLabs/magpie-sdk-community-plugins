package zcode

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestNativeCloseJoinsReadRepliesBeforePublishingSuccess(t *testing.T) {
	reading := make(chan struct{})
	returned := make(chan struct{})
	factory := func(options ModuleOptions) sdk.Module {
		module := NewControlModule(options)
		module.Actions["native.run"] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
			wire := newNativeWire(ctx, stream, sdk.ProcessOpenResult{Handle: "owned-handle"},
				nativeWorkspace{Path: "/workspace", Key: "workspace"}, nativeRunRequest{})
			<-reading
			err := wire.close(ctx)
			close(returned)
			return map[string]string{"outcome": "completed"}, err
		}
		return module
	}
	harness := newHarnessModule(t, ModuleOptions{}, struct{}{}, factory)
	callID := "native-close"
	if err := harness.send(sdk.Frame{Kind: sdk.KindCall, ID: callID, Method: sdk.MethodModuleCall,
		Scope:  sdk.Scope{Plugin: ProviderID, Operation: callID, Attempt: 1},
		Params: raw(sdk.ModuleCallRequest{Action: "native.run", Input: json.RawMessage(`{}`)}),
	}); err != nil {
		t.Fatal(err)
	}
	reads := []sdk.Frame{}
	for len(reads) < 2 {
		var frame sdk.Frame
		if err := harness.read.Decode(&frame); err != nil || frame.Method != sdk.MethodProcessRead {
			t.Fatal("native reader did not enter its public callback lane", frame, err)
		}
		reads = append(reads, frame)
	}
	close(reading)
	var closing sdk.Frame
	if err := harness.read.Decode(&closing); err != nil || closing.Method != sdk.MethodProcessClose {
		t.Fatal("native close did not use the public host owner", closing, err)
	}
	if err := harness.send(sdk.Frame{Kind: sdk.KindHostResult, ID: closing.ID, Parent: callID,
		Result: raw(sdk.ProcessCloseResult{Joined: true}),
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-returned:
		t.Error("native success canceled read waits before their host replies joined")
	case <-time.After(50 * time.Millisecond):
	}
	for _, frame := range reads {
		if err := harness.send(sdk.Frame{Kind: sdk.KindHostResult, ID: frame.ID, Parent: callID,
			Error: &sdk.Error{Code: "host_error", Message: "Owned process closed"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	var result sdk.Frame
	if err := harness.read.Decode(&result); err != nil || result.Kind != sdk.KindResult || result.ID != callID ||
		result.Error != nil {
		t.Fatal("joined native close lost its terminal success", result, err)
	}
}
