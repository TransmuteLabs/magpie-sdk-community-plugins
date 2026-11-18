package zcode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

const nativeFrameLimit = 16 << 20

type nativeFrame struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
	Trace  json.RawMessage `json:"trace,omitempty"`
}

type nativeFrameRead struct {
	frame nativeFrame
	err   error
}

type nativePipe struct {
	ctx    context.Context
	stream *sdk.Stream
	handle string
	pipe   string
	seq    uint64
	body   []byte
	eof    bool
}

func (pipe *nativePipe) Read(target []byte) (int, error) {
	if len(target) == 0 {
		return 0, nil
	}
	if len(pipe.body) == 0 {
		if pipe.eof {
			return 0, io.EOF
		}
		pipe.seq++
		sequence := strconv.FormatUint(pipe.seq, 10)
		var result sdk.ProcessReadResult
		if err := pipe.stream.HostCall(pipe.ctx, sdk.MethodProcessRead, sdk.ProcessReadRequest{
			Handle: pipe.handle, Pipe: pipe.pipe, Seq: sequence, MaxBytes: sdk.MaxChunkBytes,
		}, &result); err != nil {
			return 0, err
		}
		if result.Seq != sequence || len(result.Bytes) > sdk.MaxChunkBytes {
			return 0, errors.New("native process read sequence or bound changed")
		}
		pipe.body, pipe.eof = result.Bytes, result.EOF
		if len(pipe.body) == 0 {
			if pipe.eof {
				return 0, io.EOF
			}
			return 0, io.ErrNoProgress
		}
	}
	count := copy(target, pipe.body)
	pipe.body = pipe.body[count:]
	return count, nil
}

type nativeWire struct {
	stream    *sdk.Stream
	opened    sdk.ProcessOpenResult
	workspace nativeWorkspace
	request   nativeRunRequest
	events    chan nativeFrameRead
	faults    chan error
	cancel    context.CancelFunc
	readers   sync.WaitGroup
	writeSeq  uint64
	subID     string
	epoch     string
	seq       uint64
	initial   bool
	sent      bool
	execution string
	reverse   map[string]string
	seen      map[string]bool
	closed    bool
	closeErr  error
}

func newNativeWire(ctx context.Context, stream *sdk.Stream, opened sdk.ProcessOpenResult,
	workspace nativeWorkspace, request nativeRunRequest,
) *nativeWire {
	ctx, cancel := context.WithCancel(ctx)
	wire := &nativeWire{stream: stream, opened: opened, workspace: workspace, request: request,
		events: make(chan nativeFrameRead, 1), faults: make(chan error, 1), cancel: cancel,
		reverse: make(map[string]string), seen: make(map[string]bool)}
	wire.readers.Add(2)
	go func() {
		defer wire.readers.Done()
		defer close(wire.events)
		reader := &nativePipe{ctx: ctx, stream: stream, handle: opened.Handle, pipe: "stdout"}
		scanner := bufio.NewScanner(io.LimitReader(reader, 64<<20))
		scanner.Buffer(make([]byte, 4096), nativeFrameLimit)
		publish := func(value nativeFrameRead) bool {
			select {
			case wire.events <- value:
				return true
			case <-ctx.Done():
				return false
			}
		}
		for scanner.Scan() {
			body := bytes.TrimSpace(scanner.Bytes())
			if len(body) == 0 {
				continue
			}
			var frame nativeFrame
			if _, err := protocol.ReadObject(body); err != nil || decode(body, &frame) != nil ||
				!validNativeFrame(frame) {
				publish(nativeFrameRead{err: errors.New("invalid native protocol envelope")})
				return
			}
			if !publish(nativeFrameRead{frame: frame}) {
				return
			}
		}
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		publish(nativeFrameRead{err: err})
	}()
	go func() {
		defer wire.readers.Done()
		reader := &nativePipe{ctx: ctx, stream: stream, handle: opened.Handle, pipe: "stderr"}
		count, err := io.Copy(io.Discard, io.LimitReader(reader, (16<<20)+1))
		if count > 16<<20 {
			err = errors.New("native diagnostic byte bound exceeded")
		}
		if err != nil && ctx.Err() == nil {
			select {
			case wire.faults <- err:
			default:
			}
		}
	}()
	return wire
}

func validNativeFrame(frame nativeFrame) bool {
	hasID := len(frame.ID) != 0
	if hasID {
		var identity string
		if json.Unmarshal(frame.ID, &identity) != nil {
			var number int64
			if bytes.Equal(frame.ID, []byte("null")) || json.Unmarshal(frame.ID, &number) != nil {
				return false
			}
		} else if !nativeToken(identity, 256) {
			return false
		}
	}
	if frame.Method != "" {
		return nativeToken(frame.Method, 256) && len(frame.Result) == 0 && len(frame.Error) == 0
	}
	return hasID && (len(frame.Result) != 0) != (len(frame.Error) != 0) && len(frame.Params) == 0
}

func (wire *nativeWire) write(ctx context.Context, frame nativeFrame) error {
	body, err := json.Marshal(frame)
	if err != nil || len(body) >= nativeFrameLimit {
		return errors.New("native write exceeds its bounded envelope")
	}
	body = append(body, '\n')
	for len(body) > 0 {
		count := min(len(body), sdk.MaxChunkBytes)
		wire.writeSeq++
		sequence := strconv.FormatUint(wire.writeSeq, 10)
		var result sdk.ProcessWriteResult
		if err := wire.stream.HostCall(ctx, sdk.MethodProcessWrite, sdk.ProcessWriteRequest{
			Handle: wire.opened.Handle, WriteID: "native-" + sequence, Seq: sequence, Bytes: body[:count],
		}, &result); err != nil {
			return err
		}
		if result.Delivery != "written" || result.Written != count {
			return errors.New("native write delivery is uncertain")
		}
		body = body[count:]
	}
	return nil
}

func (wire *nativeWire) next(ctx context.Context) (nativeFrame, error) {
	select {
	case <-ctx.Done():
		return nativeFrame{}, ctx.Err()
	case err := <-wire.faults:
		return nativeFrame{}, err
	case value, open := <-wire.events:
		if !open {
			return nativeFrame{}, io.EOF
		}
		return value.frame, value.err
	}
}

func (wire *nativeWire) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	identity, err := newRequestID()
	if err != nil {
		return nil, err
	}
	id, _ := json.Marshal(identity)
	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if err := wire.write(ctx, nativeFrame{ID: id, Method: method, Params: body}); err != nil {
		return nil, err
	}
	for {
		frame, err := wire.next(ctx)
		if err != nil {
			return nil, err
		}
		if frame.Method != "" {
			if err := wire.notification(ctx, frame); err != nil {
				return nil, err
			}
			continue
		}
		if !bytes.Equal(frame.ID, id) || len(frame.Error) != 0 {
			return nil, errors.New("native response is foreign or refused")
		}
		return frame.Result, nil
	}
}

func (wire *nativeWire) subscribe(ctx context.Context) error {
	raw, err := wire.call(ctx, "v4/conversation/subscribe", map[string]any{
		"topic":        "conversation/" + wire.request.SessionID,
		"connectionId": seededUUID("native-connection\x00" + wire.request.CommandID),
		"clientMode":   "desktop-continuous", "workspace": wire.workspace,
	})
	if err != nil {
		return err
	}
	var result struct {
		Ack struct {
			Subscription string `json:"subscriptionId"`
			Epoch        string `json:"logEpoch"`
			Mode         string `json:"mode"`
		} `json:"ack"`
	}
	if json.Unmarshal(raw, &result) != nil || !nativeToken(result.Ack.Subscription, 1024) ||
		!nativeToken(result.Ack.Epoch, 256) || result.Ack.Mode != "snapshot" {
		return errors.New("native subscription acknowledgment is invalid")
	}
	wire.subID, wire.epoch = result.Ack.Subscription, result.Ack.Epoch
	for !wire.initial {
		frame, err := wire.next(ctx)
		if err != nil {
			return err
		}
		if frame.Method == "" {
			return errors.New("native initial snapshot has an unowned response")
		}
		if err := wire.notification(ctx, frame); err != nil {
			return err
		}
	}
	if wire.execution != "" {
		return errors.New("native session already has another foreground execution")
	}
	return nil
}

func (wire *nativeWire) command(ctx context.Context, command, kind string, payload any) (json.RawMessage, error) {
	if kind == "sendText" {
		if wire.sent {
			return nil, errors.New("native input cannot be repeated")
		}
		wire.sent = true
	}
	return wire.call(ctx, "v4/command", map[string]any{
		"workspace": wire.workspace, "commandId": command,
		"clientId":  seededUUID("native-client\x00" + wire.request.CommandID),
		"sessionId": wire.request.SessionID, "type": kind, "payload": payload,
		"issuedAt": time.Now().UnixMilli(),
	})
}

func nativeAck(raw json.RawMessage, command string) bool {
	var result struct {
		Command string `json:"commandId"`
		Status  string `json:"status"`
	}
	return json.Unmarshal(raw, &result) == nil && result.Command == command && result.Status == "accepted"
}

func (wire *nativeWire) query(ctx context.Context) (string, error) {
	raw, err := wire.call(ctx, "v4/commands/query", map[string]any{
		"commands": []map[string]string{{"sessionId": wire.request.SessionID, "commandId": wire.request.CommandID}},
	})
	if err != nil {
		return "", err
	}
	var result struct {
		Results []struct {
			Key struct {
				Session string `json:"sessionId"`
				Command string `json:"commandId"`
			} `json:"key"`
			Result json.RawMessage `json:"result"`
			Turn   *struct {
				ID    string `json:"turnId"`
				State string `json:"state"`
			} `json:"turn"`
		} `json:"results"`
	}
	if json.Unmarshal(raw, &result) != nil || len(result.Results) != 1 {
		return "", errors.New("native command query is invalid")
	}
	item := result.Results[0]
	if item.Key.Session != wire.request.SessionID || item.Key.Command != wire.request.CommandID ||
		!nativeAck(item.Result, wire.request.CommandID) {
		return "", errors.New("native command query cannot confirm the owned input")
	}
	if item.Turn == nil {
		return "", nil
	}
	if !nativeToken(item.Turn.ID, 256) {
		return "", errors.New("native command query has no owned turn identity")
	}
	switch item.Turn.State {
	case "running":
		return "", nil
	case "completedSuccess", "completedInterrupted", "failed":
		return item.Turn.State, nil
	default:
		return "", errors.New("native command query has an unknown terminal")
	}
}

func (wire *nativeWire) notification(ctx context.Context, frame nativeFrame) error {
	if frame.Method == "v4/conversation/frame" && len(frame.ID) == 0 {
		return wire.projection(frame.Params)
	}
	if frame.Method == "interaction/providerRuntimeHeadersCancelled" && len(frame.ID) == 0 {
		var request struct {
			Request   string          `json:"requestId"`
			Session   string          `json:"sessionId"`
			Workspace nativeWorkspace `json:"workspace"`
		}
		if decode(frame.Params, &request) != nil || request.Session != wire.request.SessionID ||
			request.Workspace != wire.workspace || !nativeToken(request.Request, 1024) {
			return errors.New("native reverse cancellation is foreign")
		}
		found := false
		for identity, pending := range wire.reverse {
			if pending == request.Request {
				delete(wire.reverse, identity)
				found = true
				break
			}
		}
		if !found {
			return errors.New("native reverse cancellation is uncorrelated")
		}
		return nil
	}
	if len(frame.ID) == 0 {
		return nil
	}
	if frame.Method != "interaction/requestProviderRuntimeHeaders" {
		body := json.RawMessage(`{"code":-32601,"message":"This native interaction requires an unsupported explicit owner"}`)
		if err := wire.write(ctx, nativeFrame{ID: frame.ID, Error: body}); err != nil {
			return err
		}
		return errors.New("native interaction has no explicit supported owner")
	}
	var request struct {
		Request   string          `json:"requestId"`
		Session   string          `json:"sessionId"`
		Workspace nativeWorkspace `json:"workspace"`
		Provider  string          `json:"providerId"`
		Selection struct {
			Provider string `json:"providerId"`
			Model    string `json:"modelId"`
		} `json:"modelSelection"`
	}
	if json.Unmarshal(frame.Params, &request) != nil || request.Session != wire.request.SessionID ||
		request.Workspace != wire.workspace || request.Provider != "magpie" || request.Selection.Provider != "magpie" ||
		request.Selection.Model != wire.request.Model || !nativeToken(request.Request, 1024) ||
		wire.seen[string(frame.ID)] || wire.seen[request.Request] || len(wire.seen) >= 256 {
		return errors.New("native reverse request is foreign or duplicated")
	}
	wire.seen[string(frame.ID)], wire.seen[request.Request] = true, true
	wire.reverse[string(frame.ID)] = request.Request
	return wire.write(ctx, nativeFrame{ID: frame.ID, Result: json.RawMessage(`{"headersApplied":false}`)})
}

func (wire *nativeWire) projection(raw json.RawMessage) error {
	var frame struct {
		Topic        string `json:"topic"`
		Subscription string `json:"subscriptionId"`
		From         uint64 `json:"fromSeq"`
		To           uint64 `json:"toSeq"`
		Payload      struct {
			Kind     string          `json:"kind"`
			Snapshot json.RawMessage `json:"snapshot"`
			Deltas   []struct {
				Operation string          `json:"op"`
				Patch     json.RawMessage `json:"patch"`
			} `json:"deltas"`
		} `json:"payload"`
	}
	if json.Unmarshal(raw, &frame) != nil || frame.Topic != "conversation/"+wire.request.SessionID ||
		frame.Subscription != wire.subID || frame.To < frame.From {
		return errors.New("native conversation frame is foreign or malformed")
	}
	switch frame.Payload.Kind {
	case "snapshot":
		var snapshot struct {
			Session string          `json:"sessionId"`
			Epoch   string          `json:"logEpoch"`
			Seq     uint64          `json:"seq"`
			Control json.RawMessage `json:"control"`
		}
		if wire.initial || frame.From != 0 || json.Unmarshal(frame.Payload.Snapshot, &snapshot) != nil ||
			snapshot.Session != wire.request.SessionID || snapshot.Epoch != wire.epoch || snapshot.Seq != frame.To {
			return errors.New("native snapshot changed its selected session or epoch")
		}
		if err := wire.control(snapshot.Control); err != nil {
			return err
		}
		wire.initial = true
	case "deltas":
		if !wire.initial || frame.From != wire.seq || frame.To <= frame.From {
			return errors.New("native deltas are stale or have a sequence gap")
		}
		for _, delta := range frame.Payload.Deltas {
			if delta.Operation == "state.updated" {
				var patch struct {
					Control json.RawMessage `json:"control"`
				}
				if json.Unmarshal(delta.Patch, &patch) != nil {
					return errors.New("native state patch is invalid")
				}
				if len(patch.Control) != 0 {
					if err := wire.control(patch.Control); err != nil {
						return err
					}
				}
			}
		}
	default:
		return errors.New("native conversation frame has no supported payload")
	}
	wire.seq = frame.To
	return nil
}

func (wire *nativeWire) control(raw json.RawMessage) error {
	var control struct {
		CanStop *bool `json:"canStop"`
		Works   []struct {
			Kind      string `json:"kind"`
			Execution string `json:"foregroundExecutionId"`
			Command   string `json:"sourceCommandId"`
		} `json:"activeWorks"`
	}
	if json.Unmarshal(raw, &control) != nil || control.CanStop == nil || control.Works == nil || len(control.Works) > 256 {
		return errors.New("native control state is incomplete")
	}
	execution := ""
	for _, work := range control.Works {
		if !wire.sent && work.Execution != "" {
			return errors.New("native session has an existing foreground execution")
		}
		if work.Kind == "primaryTurn" && work.Command == wire.request.CommandID {
			if !*control.CanStop || !nativeToken(work.Execution, 256) || execution != "" {
				return errors.New("native stop target is unavailable or ambiguous")
			}
			execution = work.Execution
		}
	}
	wire.execution = execution
	return nil
}

func (wire *nativeWire) close(ctx context.Context) error {
	if wire.closed {
		return wire.closeErr
	}
	wire.closed = true
	closeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var result sdk.ProcessCloseResult
	err := wire.stream.HostCall(closeCtx, sdk.MethodProcessClose,
		sdk.ProcessHandleRequest{Handle: wire.opened.Handle}, &result)
	if err != nil || !result.Joined {
		wire.cancel()
		wire.readers.Wait()
		wire.closeErr = errors.New("native process close did not confirm joined cleanup")
		return wire.closeErr
	}
	done := make(chan struct{})
	go func() {
		wire.readers.Wait()
		close(done)
	}()
	events := wire.events
	for {
		select {
		case <-done:
			wire.cancel()
			return nil
		case <-closeCtx.Done():
			wire.cancel()
			<-done
			wire.closeErr = errors.New("native process read replies did not join before completion")
			return wire.closeErr
		case _, open := <-events:
			if !open {
				events = nil
			}
		}
	}
}
