package devin

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

type queuedNative struct {
	wire    []byte
	release func()
}
type nativeBridge struct {
	id       string
	envelope codec.NativeEnvelope
	frames   chan queuedNative
	closed   chan struct{}
	once     sync.Once
	mu       sync.Mutex
	claimed  bool
	store    *bridgeStore
}

type bridgeStore struct {
	mu            sync.Mutex
	rows          map[string]*nativeBridge
	bytes         int
	responseBytes int
	closed        bool
}

func newBridgeStore() *bridgeStore { return &bridgeStore{rows: map[string]*nativeBridge{}} }
func (store *bridgeStore) open(envelope codec.NativeEnvelope) (*nativeBridge, error) {
	id, err := randomIdentity()
	if err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed || len(store.rows) >= 16 || len(envelope.Payload)+len(envelope.Projection) > 32<<20-store.bytes {
		return nil, &sdk.Error{Code: "resource_exhausted", Message: "Devin native request retention limit reached"}
	}
	bridge := &nativeBridge{id: id, envelope: envelope, frames: make(chan queuedNative, 1), closed: make(chan struct{}), store: store}
	store.rows[id] = bridge
	store.bytes += len(envelope.Payload) + len(envelope.Projection)
	return bridge, nil
}

func (store *bridgeStore) get(id string) (*nativeBridge, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	bridge := store.rows[id]
	if store.closed || bridge == nil {
		return nil, &sdk.Error{Code: "permission_denied", Message: "Native Devin request handle is missing or belongs to another generation", Outcome: "not-started"}
	}
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if bridge.claimed {
		return nil, &sdk.Error{Code: "conflict", Message: "Native Devin request was already acquired", Outcome: "not-started"}
	}
	bridge.claimed = true
	return bridge, nil
}

func (store *bridgeStore) remove(bridge *nativeBridge) {
	store.mu.Lock()
	if store.rows[bridge.id] == bridge {
		delete(store.rows, bridge.id)
		store.bytes -= len(bridge.envelope.Payload) + len(bridge.envelope.Projection)
	}
	store.mu.Unlock()
	bridge.once.Do(func() { close(bridge.closed) })
	for {
		select {
		case frame := <-bridge.frames:
			frame.release()
		default:
			return
		}
	}
}

func (store *bridgeStore) close() {
	store.mu.Lock()
	store.closed = true
	rows := make([]*nativeBridge, 0, len(store.rows))
	for _, row := range store.rows {
		rows = append(rows, row)
	}
	store.mu.Unlock()
	for _, row := range rows {
		store.remove(row)
	}
}

func (bridge *nativeBridge) push(ctx context.Context, frame codec.NativeFrame) error {
	if len(frame.Payload) > 16<<20 {
		return errors.New("native frame exceeds its bound")
	}
	size := 5 + len(frame.Payload)
	bridge.store.mu.Lock()
	if bridge.store.closed || size > 32<<20-bridge.store.responseBytes {
		bridge.store.mu.Unlock()
		return errors.New("native response retention limit reached")
	}
	bridge.store.responseBytes += size
	bridge.store.mu.Unlock()
	var released sync.Once
	release := func() {
		released.Do(func() { bridge.store.mu.Lock(); bridge.store.responseBytes -= size; bridge.store.mu.Unlock() })
	}
	wire := make([]byte, 5+len(frame.Payload))
	wire[0] = frame.Flags
	binary.BigEndian.PutUint32(wire[1:5], uint32(len(frame.Payload)))
	copy(wire[5:], frame.Payload)
	value := queuedNative{wire: wire, release: release}
	select {
	case bridge.frames <- value:
		select {
		case <-bridge.closed:
			for {
				select {
				case pending := <-bridge.frames:
					pending.release()
				default:
					return context.Canceled
				}
			}
		default:
			return nil
		}
	case <-bridge.closed:
		release()
		return context.Canceled
	case <-ctx.Done():
		release()
		return ctx.Err()
	}
}

type pairedAdapter struct{ owner *providerOwner }
type pairedCall struct {
	owner  *providerOwner
	ctx    context.Context
	state  *providerState
	bridge *nativeBridge
}

func (*pairedAdapter) ID() string                     { return "devin" }
func (*pairedAdapter) Path() string                   { return codec.Path }
func (*pairedAdapter) Contract() protocolext.Contract { return protocolext.ExecutorContract() }
func (adapter *pairedAdapter) Bind(ctx context.Context) protocolext.Inbound {
	return &pairedCall{owner: adapter.owner, ctx: ctx}
}
func (adapter *pairedAdapter) Decode(context.Context, http.Header, []byte) (protocolext.Request, error) {
	return protocolext.Request{}, errors.New("native Devin adapter must be bound")
}
func (*pairedAdapter) Reply(http.ResponseWriter) protocolext.Response { return nil }
func (*pairedCall) ID() string                                        { return "devin" }
func (*pairedCall) Path() string                                      { return codec.Path }
func (*pairedCall) Contract() protocolext.Contract                    { return protocolext.ExecutorContract() }
func (call *pairedCall) Decode(ctx context.Context, header http.Header, body []byte) (protocolext.Request, error) {
	state, err := currentProviderState(ctx)
	if err != nil {
		return protocolext.Request{}, err
	}
	envelope, err := codec.ParseNativeEnvelope(ctx, header, body)
	if state.agentAssignments {
		envelope, err = codec.ParseAgentEnvelope(ctx, header, body)
	}
	if err != nil {
		return protocolext.Request{}, err
	}
	bridge, err := state.bridges.open(envelope)
	if err != nil {
		return protocolext.Request{}, &protocolext.Error{Status: 429, Message: "Devin native request retention limit reached"}
	}
	call.state, call.bridge = state, bridge
	context.AfterFunc(call.ctx, func() { state.bridges.remove(bridge) })
	metadata, _ := json.Marshal(struct {
		Handle string `json:"handle"`
	}{bridge.id})
	return protocolext.Request{Protocol: protocolext.Chat, Body: envelope.Projection, Session: envelope.Session, RequestID: bridge.id, Metadata: metadata,
		Timeout: envelope.Timeout, Executor: protocolext.SameModuleExecutor}, nil
}
func (call *pairedCall) Reply(writer http.ResponseWriter) protocolext.Response {
	return &pairedReply{call: call, writer: writer, fallback: (codec.Adapter{}).Reply(writer)}
}

type pairedReply struct {
	call     *pairedCall
	writer   http.ResponseWriter
	fallback protocolext.Response
	raw      bool
	terminal bool
	closed   bool
}

func (reply *pairedReply) Header() http.Header { return reply.fallback.Header() }
func (reply *pairedReply) WriteHeader(status int) {
	if !reply.raw {
		reply.fallback.WriteHeader(status)
	}
}
func (reply *pairedReply) Unwrap() http.ResponseWriter { return reply.writer }
func (reply *pairedReply) Flush() {
	if reply.raw {
		if flusher, ok := reply.writer.(http.Flusher); ok {
			flusher.Flush()
		}
	}
}
func (reply *pairedReply) drain() error {
	if reply.call.bridge == nil {
		return nil
	}
	for {
		select {
		case frame := <-reply.call.bridge.frames:
			if !reply.raw {
				reply.raw = true
				reply.writer.Header().Set("Content-Type", "application/connect+proto")
				reply.writer.Header().Set("Connect-Protocol-Version", "1")
				reply.writer.Header().Set("Cache-Control", "no-store")
				reply.writer.Header().Del("Content-Length")
				reply.writer.WriteHeader(200)
			}
			if reply.terminal {
				frame.release()
				return errors.New("native data after terminal")
			}
			count, err := reply.writer.Write(frame.wire)
			if err == nil && count != len(frame.wire) {
				err = io.ErrShortWrite
			}
			if frame.wire[0]&2 != 0 {
				reply.terminal = true
			}
			frame.release()
			if err != nil {
				return err
			}
			reply.Flush()
		default:
			return nil
		}
	}
}
func (reply *pairedReply) Write(body []byte) (int, error) {
	if reply.closed {
		return 0, io.ErrClosedPipe
	}
	if err := reply.drain(); err != nil {
		return 0, err
	}
	if reply.raw {
		return len(body), nil
	}
	return reply.fallback.Write(body)
}
func (reply *pairedReply) Finish(cause error) error {
	if reply.closed {
		return nil
	}
	reply.closed = true
	if reply.call.bridge != nil {
		defer reply.call.state.bridges.remove(reply.call.bridge)
	}
	if err := reply.drain(); err != nil {
		return err
	}
	if !reply.raw {
		return reply.fallback.Finish(cause)
	}
	if reply.terminal {
		return nil
	}
	code := "internal"
	if errors.Is(cause, context.Canceled) {
		code = "canceled"
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		code = "deadline_exceeded"
	}
	value, _ := json.Marshal(map[string]any{"error": map[string]string{"code": code, "message": "Devin native request ended before completion"}})
	wire := make([]byte, 5+len(value))
	wire[0] = 2
	binary.BigEndian.PutUint32(wire[1:], uint32(len(value)))
	copy(wire[5:], value)
	count, err := reply.writer.Write(wire)
	if err == nil && count != len(wire) {
		err = io.ErrShortWrite
	}
	reply.Flush()
	return err
}
