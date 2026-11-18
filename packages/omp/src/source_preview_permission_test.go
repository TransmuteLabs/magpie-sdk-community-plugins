package ompsessions

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const allowedSourceConfiguration = `{"sessionSource":{"ownOff":false,"records":[{"sourceId":"omp-history","owned":true,"enabled":true}]}}`

type sourcePeer struct {
	connection net.Conn
	encoder    *json.Encoder
	decoder    *json.Decoder
	next       int
}

func newSourcePeer(test *testing.T, module sdk.Module, config string, instance, version string) *sourcePeer {
	test.Helper()
	client, server := net.Pipe()
	ctx, cancel := context.WithCancel(test.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = sdk.Serve(ctx, server, server, module)
	}()
	test.Cleanup(func() {
		cancel()
		client.Close()
		server.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			test.Error("Source SDK server did not stop")
		}
	})
	peer := &sourcePeer{connection: client, encoder: json.NewEncoder(client), decoder: json.NewDecoder(client)}
	frame := peer.call(test, sdk.MethodComponentConfigure, sdk.ComponentConfigureRequest{
		Component: sdk.PrimaryComponent, Instance: instance,
		Config: sdk.ConfigSnapshot{Version: version, Values: json.RawMessage(config)},
	})
	if frame.Error != nil {
		test.Fatal("module configuration failed", frame.Error)
	}
	return peer
}

func (peer *sourcePeer) call(test *testing.T, method string, params any) sdk.Frame {
	test.Helper()
	_ = peer.connection.SetDeadline(time.Now().Add(10 * time.Second))
	raw, err := json.Marshal(params)
	if err != nil {
		test.Fatal(err)
	}
	peer.next++
	frame := sdk.Frame{Kind: sdk.KindCall, ID: strconv.Itoa(peer.next), Method: method, Params: raw}
	if method == sdk.MethodSessionSourceRead || method == sdk.MethodModuleCall {
		frame.Scope = sdk.Scope{Session: sessionID, Operation: sdk.MethodSessionSourceRead}
	}
	if err := peer.encoder.Encode(frame); err != nil {
		test.Fatal("Source request failed", err)
	}
	if err := peer.decoder.Decode(&frame); err != nil {
		test.Fatal("Source response failed", err)
	}
	return frame
}

func requireSourceDenied(test *testing.T, err error) {
	test.Helper()
	var fault *sdk.Error
	if !errors.As(err, &fault) || fault == nil || fault.Code != "permission_denied" {
		test.Fatal("module record did not refuse the read", err)
	}
}

func TestSourceRecordPolicyRefusesReadsAndPlanning(test *testing.T) {
	for name, config := range map[string]string{
		"missing":      `{}`,
		"unowned":      `{"sessionSource":{"ownOff":false,"records":[{"sourceId":"omp-history","owned":false,"enabled":true}]}}`,
		"disabled":     `{"sessionSource":{"ownOff":false,"records":[{"sourceId":"omp-history","owned":true,"enabled":false}]}}`,
		"own-off":      `{"sessionSource":{"ownOff":true,"records":[{"sourceId":"omp-history","owned":true,"enabled":true}]}}`,
		"unknown":      `{"sessionSource":{"ownOff":false,"records":[{"sourceId":"other","owned":true,"enabled":true}]}}`,
		"null":         `{"sessionSource":null}`,
		"missing-flag": `{"sessionSource":{"ownOff":false,"records":[{"sourceId":"omp-history","enabled":true}]}}`,
		"ambiguous":    `{"sessionSource":{"ownOff":false,"records":[{"sourceId":"omp-history","owned":true,"enabled":true},{"sourceId":"omp-history","owned":false,"enabled":false}]}}`,
		"duplicate":    `{"sessionSource":{"ownOff":false,"records":[{"sourceId":"omp-history","owned":false,"owned":true,"enabled":true}]}}`,
		"case-alias":   `{"sessionSource":{"ownOff":false,"records":[{"sourceId":"omp-history","Owned":true,"enabled":true}]}}`,
		"root-field":   `{"sessionSource":{"ownOff":false,"records":[{"sourceId":"omp-history","owned":true,"enabled":true,"root":"/not-authority"}]}}`,
		"wrong-type":   `{"sessionSource":{"ownOff":"false","records":[{"sourceId":"omp-history","owned":true,"enabled":true}]}}`,
	} {
		test.Run(name, func(test *testing.T) {
			peer := newSourcePeer(test, SourcePreviewModule(), config, "policy-instance", "1")
			for _, mode := range []string{"summary", "transcript"} {
				request := sourceRequest(test, mode, []byte("{bad-native-data}\n"))
				frame := peer.call(test, sdk.MethodSessionSourceRead, request)
				requireSourceDenied(test, frame.Error)
			}
			request := sdk.SessionRequest{ID: SourceDescriptorID, From: OMPFormat, To: OMPFormat,
				Files: sourceRequest(test, "summary", []byte("{bad-native-data}\n")).Documents}
			raw, _ := json.Marshal(request)
			frame := peer.call(test, sdk.MethodModuleCall, sdk.ModuleCallRequest{Action: SourceInputsAction, Input: raw})
			requireSourceDenied(test, frame.Error)
		})
	}
}

func TestSourceRecordAllowsGrantedBytesWithoutRoots(test *testing.T) {
	peer := newSourcePeer(test, SourcePreviewModule(), allowedSourceConfiguration, "allowed-instance", "2")
	for _, mode := range []string{"summary", "transcript"} {
		request := sourceRequest(test, mode, sourceFixture(test, "omp-v3.jsonl"))
		frame := peer.call(test, sdk.MethodSessionSourceRead, request)
		if frame.Error != nil {
			test.Fatal("owned enabled record refused", frame.Error)
		}
		if _, err := sdk.DecodeSessionSourceResult(request, frame.Result); err != nil {
			test.Fatal(err)
		}
	}
}

func TestSourceRecordOwnOffDoesNotChangeMigrationPlanner(test *testing.T) {
	peer := newSourcePeer(test, SourcePreviewModule(),
		`{"sessionSource":{"ownOff":true,"records":[{"sourceId":"omp-history","owned":true,"enabled":true}]}}`,
		"migration-instance", "4")
	request := sdk.SessionRequest{ID: "omp-to-document", From: "omp", To: DocumentFormat,
		Files: sourceRequest(test, "summary", sourceFixture(test, "omp-v3.jsonl")).Documents}
	request.Files[0].Format = "omp"
	expected, err := PlanSourceInputs(test.Context(), request)
	if err != nil {
		test.Fatal(err)
	}
	raw, _ := json.Marshal(request)
	frame := peer.call(test, sdk.MethodModuleCall, sdk.ModuleCallRequest{Action: SourceInputsAction, Input: raw})
	var result SourceInputs
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil ||
		!reflect.DeepEqual(result, expected) || len(result.Files) != 1 {
		test.Fatal("Source own-off changed ordinary migration planning", frame.Error)
	}
}

func TestSourceRecordIsBoundToInstanceAndGeneration(test *testing.T) {
	for _, mismatch := range []string{"instance", "version"} {
		test.Run(mismatch, func(test *testing.T) {
			module := SourcePreviewModule()
			module.Start = func(ctx context.Context, value sdk.ModuleContext) (sdk.ModuleReady, func(), error) {
				if mismatch == "instance" {
					value.Instance = "another-instance"
				} else {
					value.Config.Version = "0"
				}
				return sourcePreviewStart(ctx, value)
			}
			peer := newSourcePeer(test, module, allowedSourceConfiguration, "actual-instance", "1")
			frame := peer.call(test, sdk.MethodSessionSourceRead, sourceRequest(test, "summary", sourceFixture(test, "omp-v3.jsonl")))
			requireSourceDenied(test, frame.Error)
		})
	}
}

func TestSourceRecordRevocationSurvivesDetachedCallerContext(test *testing.T) {
	module := SourcePreviewModule()
	var captured context.Context
	original := module.Handlers[sdk.MethodSessionSourceRead]
	module.Handlers[sdk.MethodSessionSourceRead] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
		captured = context.WithoutCancel(ctx)
		return original(ctx, call, stream)
	}
	peer := newSourcePeer(test, module, allowedSourceConfiguration, "revoked-instance", "1")
	request := sourceRequest(test, "summary", sourceFixture(test, "omp-v3.jsonl"))
	if frame := peer.call(test, sdk.MethodSessionSourceRead, request); frame.Error != nil {
		test.Fatal(frame.Error)
	}
	raw, _ := json.Marshal(request)
	if _, err := SourceReadHandler(captured, sdk.Call{Params: raw}, nil); err != nil {
		test.Fatal("live instance unexpectedly refused detached caller", err)
	}
	if frame := peer.call(test, sdk.MethodComponentStop, struct{}{}); frame.Error != nil {
		test.Fatal(frame.Error)
	}
	_, err := SourceReadHandler(captured, sdk.Call{Params: raw}, nil)
	requireSourceDenied(test, err)
	fresh := newSourcePeer(test, SourcePreviewModule(), allowedSourceConfiguration, "fresh-instance", "2")
	if frame := fresh.call(test, sdk.MethodSessionSourceRead, request); frame.Error != nil {
		test.Fatal("revoked instance contaminated a fresh record", frame.Error)
	}
	_, err = SourceReadHandler(captured, sdk.Call{Params: raw}, nil)
	requireSourceDenied(test, err)
}

type sourceDecodeRevocationContext struct {
	context.Context
	checks atomic.Int32
	revoke func()
}

func (ctx *sourceDecodeRevocationContext) Err() error {
	if ctx.checks.Add(1) == 2 {
		ctx.revoke()
	}
	return ctx.Context.Err()
}

func TestSourceRecordRechecksAfterDecode(test *testing.T) {
	module := SourcePreviewModule()
	var captured context.Context
	original := module.Handlers[sdk.MethodSessionSourceRead]
	module.Handlers[sdk.MethodSessionSourceRead] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
		captured = context.WithoutCancel(ctx)
		return original(ctx, call, stream)
	}
	peer := newSourcePeer(test, module, allowedSourceConfiguration, "decode-instance", "1")
	request := sourceRequest(test, "summary", sourceFixture(test, "omp-v3.jsonl"))
	if frame := peer.call(test, sdk.MethodSessionSourceRead, request); frame.Error != nil {
		test.Fatal(frame.Error)
	}
	value, _ := sdk.ModuleContextFrom(captured)
	permission := value.State.(*sourceReadPermission)
	ctx := &sourceDecodeRevocationContext{Context: captured, revoke: func() { permission.closed.Store(true) }}
	raw, _ := json.Marshal(request)
	_, err := SourceReadHandler(ctx, sdk.Call{Params: raw}, nil)
	requireSourceDenied(test, err)
	if ctx.checks.Load() < 2 {
		test.Fatal("revocation did not occur inside the byte decoder")
	}
}

func TestSourceRecordSnapshotAndCloseAreOwned(test *testing.T) {
	config := []byte(allowedSourceConfiguration)
	ready, dispose, err := sourcePreviewStart(test.Context(), sdk.ModuleContext{
		Instance: "immutable", Config: sdk.ConfigSnapshot{Version: "3", Values: config},
	})
	if err != nil {
		test.Fatal(err)
	}
	permission := ready.State.(*sourceReadPermission)
	for index := range config {
		config[index] = '!'
	}
	if err := permission.check(test.Context()); err != nil {
		test.Fatal("record retained mutable config bytes", err)
	}
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			for range 100 {
				_ = permission.check(test.Context())
			}
		})
	}
	dispose()
	workers.Wait()
	requireSourceDenied(test, permission.check(test.Context()))
}
