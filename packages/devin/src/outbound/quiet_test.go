package outbound

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

type quietTestTimer struct {
	at      time.Duration
	run     func()
	started bool
	stopped bool
}

type quietTestClock struct {
	mu     sync.Mutex
	now    time.Duration
	timers []*quietTestTimer
}

func (clock *quietTestClock) after(wait time.Duration, run func()) func() bool {
	clock.mu.Lock()
	timer := &quietTestTimer{at: clock.now + wait, run: run}
	clock.timers = append(clock.timers, timer)
	clock.mu.Unlock()
	return func() bool {
		clock.mu.Lock()
		defer clock.mu.Unlock()
		if timer.started || timer.stopped {
			return false
		}
		timer.stopped = true
		return true
	}
}

func (clock *quietTestClock) advance(elapsed time.Duration) int {
	clock.mu.Lock()
	clock.now += elapsed
	var due []*quietTestTimer
	for _, timer := range clock.timers {
		if !timer.started && !timer.stopped && timer.at <= clock.now {
			timer.started = true
			due = append(due, timer)
		}
	}
	clock.mu.Unlock()
	for _, timer := range due {
		timer.run()
	}
	return len(due)
}

type quietTestBody struct {
	ctx     context.Context
	chunks  chan []byte
	ready   chan struct{}
	closed  chan struct{}
	once    sync.Once
	buffer  []byte
	closes  atomic.Int32
	reading atomic.Int32
}

func (body *quietTestBody) Read(output []byte) (int, error) {
	body.reading.Add(1)
	defer body.reading.Add(-1)
	if len(body.buffer) == 0 {
		select {
		case body.ready <- struct{}{}:
		case <-body.ctx.Done():
			return 0, body.ctx.Err()
		case <-body.closed:
			return 0, io.ErrClosedPipe
		}
		select {
		case chunk, ok := <-body.chunks:
			if !ok {
				return 0, io.EOF
			}
			body.buffer = chunk
		case <-body.ctx.Done():
			return 0, body.ctx.Err()
		case <-body.closed:
			return 0, io.ErrClosedPipe
		}
	}
	count := copy(output, body.buffer)
	body.buffer = body.buffer[count:]
	return count, nil
}

func (body *quietTestBody) Close() error {
	body.closes.Add(1)
	body.once.Do(func() { close(body.closed) })
	return nil
}

type quietTestResult struct {
	summary codec.NativeSummary
	err     error
}

type quietTestOperation struct {
	ctx      context.Context
	cancel   context.CancelFunc
	clock    quietTestClock
	body     *quietTestBody
	result   chan quietTestResult
	done     chan struct{}
	sends    atomic.Int32
	accepted atomic.Int32
	emitted  atomic.Int32
}

func quietUsageFrame() []byte {
	payload := append(fieldText(3, "partial answer"), fieldText(7, string(append(fieldNumber(2, 7), fieldNumber(3, 3)...)))...)
	return frame(0, payload)
}

func startQuietTest(t *testing.T, raw bool, status int, emit func()) *quietTestOperation {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	operation := &quietTestOperation{
		ctx: ctx, cancel: cancel, result: make(chan quietTestResult, 1), done: make(chan struct{}),
		body: &quietTestBody{chunks: make(chan []byte, 1), ready: make(chan struct{}, 1), closed: make(chan struct{})},
	}
	called := make(chan struct{})
	client, err := New("http://127.0.0.1:1", &http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
		operation.sends.Add(1)
		operation.body.ctx = request.Context()
		close(called)
		kind := "application/connect+proto"
		if status != http.StatusOK {
			kind = "application/json"
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {kind}}, Body: operation.body}, nil
	})}, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	client.quietTimer = operation.clock.after
	t.Cleanup(func() {
		cancel()
		select {
		case <-operation.done:
		case <-time.After(10 * time.Second):
			t.Error("ordinary client reader did not join during cleanup")
		}
	})
	go func() {
		defer close(operation.done)
		accept := func(context.Context) error { operation.accepted.Add(1); return nil }
		onEmit := func() {
			operation.emitted.Add(1)
			if emit != nil {
				emit()
			}
		}
		var result quietTestResult
		if raw {
			result.summary, result.err = client.Native(ctx, Credentials{Key: "selected-key"}, fieldText(21, "model"), "model", accept, func(codec.NativeFrame) error {
				onEmit()
				return nil
			})
		} else {
			result.summary, result.err = client.Complete(ctx, Credentials{Key: "selected-key"}, codec.EncodedChat{Payload: fieldText(21, "model")}, accept, func(codec.NativeDelta) error {
				onEmit()
				return nil
			})
		}
		operation.result <- result
	}()
	select {
	case <-called:
	case <-operation.done:
		t.Fatal("ordinary client ended before dispatch")
	case <-time.After(10 * time.Second):
		t.Fatal("ordinary client did not reach dispatch")
	}
	return operation
}

func (operation *quietTestOperation) pendingRead(t *testing.T) {
	t.Helper()
	select {
	case <-operation.body.ready:
	case <-operation.done:
		t.Fatal("ordinary client ended before the held body read")
	case <-time.After(10 * time.Second):
		t.Fatal("fixture did not reach the held body read")
	}
}

func (operation *quietTestOperation) joined(t *testing.T) quietTestResult {
	t.Helper()
	select {
	case <-operation.done:
		result := <-operation.result
		if operation.body.reading.Load() != 0 || operation.body.closes.Load() != 1 ||
			operation.sends.Load() != 1 || operation.accepted.Load() != 1 {
			t.Fatalf("stream reader/close/send/accept ownership changed: reading=%d closes=%d sends=%d accepted=%d",
				operation.body.reading.Load(), operation.body.closes.Load(), operation.sends.Load(), operation.accepted.Load())
		}
		return result
	case <-time.After(10 * time.Second):
		operation.cancel()
		t.Fatal("ordinary client did not join after its controlled terminal event")
		return quietTestResult{}
	}
}

func TestQuietReadTerminatesUncertainWithoutReplay(t *testing.T) {
	for _, raw := range []bool{false, true} {
		for _, mode := range []string{"silent", "partial frame", "heartbeat then quiet", "output then quiet", "partial error body"} {
			t.Run(map[bool]string{false: "decoded/", true: "raw/"}[raw]+mode, func(t *testing.T) {
				status := http.StatusOK
				if mode == "partial error body" {
					status = http.StatusTooManyRequests
				}
				operation := startQuietTest(t, raw, status, nil)
				operation.pendingRead(t)
				var prefix []byte
				switch mode {
				case "partial frame":
					prefix = goodStream()[:2]
				case "heartbeat then quiet":
					prefix = frame(0, fieldText(1, "a"))
				case "output then quiet":
					prefix = quietUsageFrame()
				case "partial error body":
					prefix = []byte(`{"error":{"code":"resource_exhausted"`)
				}
				if prefix != nil {
					operation.body.chunks <- prefix
					operation.pendingRead(t)
				}
				expired := operation.clock.advance(5 * time.Minute)
				callerLive := operation.ctx.Err() == nil
				if expired == 0 {
					operation.cancel()
				}
				result := operation.joined(t)
				var failure *Error
				typed := errors.As(result.err, &failure)
				if expired != 1 || !callerLive || !typed ||
					failure.Status != http.StatusGatewayTimeout || failure.Code != "deadline_exceeded" ||
					failure.Outcome != "uncertain" || failure.NoEffect || failure.Facts != nil ||
					!errors.Is(result.err, context.DeadlineExceeded) {
					if typed {
						t.Logf("structured failure status=%d code=%s outcome=%s noEffect=%v",
							failure.Status, failure.Code, failure.Outcome, failure.NoEffect)
					}
					t.Fatalf("quiet body read must terminate with a504 while caller remains live and without refusal authority: expired=%d callerLive=%v err=%+v",
						expired, callerLive, result.err)
				}
				if mode == "output then quiet" && operation.emitted.Load() != 1 {
					t.Fatal("already published output disappeared or repeated")
				}
				if mode == "output then quiet" && (result.summary.Usage == nil ||
					result.summary.Usage.Input != 7 || result.summary.Usage.Output != 3) {
					t.Fatalf("quiet timeout discarded already observed usage: %+v", result.summary.Usage)
				}
				if operation.ctx.Err() != nil && expired != 0 {
					t.Fatal("quiet watchdog canceled the still-live host caller")
				}
			})
		}
	}
}

func TestQuietReadProgressRenewsOnlyPendingReads(t *testing.T) {
	for _, raw := range []bool{false, true} {
		for _, partial := range []bool{false, true} {
			t.Run(map[bool]string{false: "decoded/", true: "raw/"}[raw]+map[bool]string{false: "heartbeats", true: "partial frame"}[partial], func(t *testing.T) {
				operation := startQuietTest(t, raw, http.StatusOK, nil)
				operation.pendingRead(t)
				heartbeat := frame(0, fieldText(1, "a"))
				parts := [][]byte{heartbeat, heartbeat, heartbeat, heartbeat}
				if partial {
					parts = [][]byte{heartbeat[:1], heartbeat[1:2], heartbeat[2:3], heartbeat[3:]}
				}
				for _, part := range parts {
					if expired := operation.clock.advance(2 * time.Minute); expired != 0 {
						t.Fatalf("progress inside each five-minute read window was treated as a total deadline: expired=%d", expired)
					}
					operation.body.chunks <- part
					operation.pendingRead(t)
				}
				operation.body.chunks <- goodStream()
				close(operation.body.chunks)
				result := operation.joined(t)
				if result.err != nil || result.summary.Stop != 2 || result.summary.Usage == nil {
					t.Fatalf("eight-minute progressing stream did not preserve its completion: %+v %v", result.summary, result.err)
				}
				if expired := operation.clock.advance(10 * time.Minute); expired != 0 {
					t.Fatal("a completed read left a quiet timer running")
				}
			})
		}
	}
}

func TestQuietReadDoesNotTimeTheConsumer(t *testing.T) {
	for _, raw := range []bool{false, true} {
		t.Run(map[bool]string{false: "decoded", true: "raw"}[raw], func(t *testing.T) {
			paused, resume := make(chan struct{}), make(chan struct{})
			var once sync.Once
			operation := startQuietTest(t, raw, http.StatusOK, func() {
				once.Do(func() {
					close(paused)
					<-resume
				})
			})
			var release sync.Once
			t.Cleanup(func() { release.Do(func() { close(resume) }) })
			operation.pendingRead(t)
			operation.body.chunks <- goodStream()
			close(operation.body.chunks)
			select {
			case <-paused:
			case <-time.After(10 * time.Second):
				t.Fatal("fixture did not reach the actual output consumer")
			}
			expired := operation.clock.advance(10 * time.Minute)
			live := operation.body.ctx.Err() == nil
			release.Do(func() { close(resume) })
			result := operation.joined(t)
			if expired != 0 || !live || result.err != nil || result.summary.Stop != 2 {
				t.Fatalf("consumer delay consumed an upstream-read timeout: expired=%d live=%v err=%v", expired, live, result.err)
			}
		})
	}
}

func TestQuietReadPreservesCallerCancellation(t *testing.T) {
	for _, raw := range []bool{false, true} {
		for _, collide := range []bool{false, true} {
			t.Run(map[bool]string{false: "decoded/", true: "raw/"}[raw]+map[bool]string{false: "cancel", true: "cancel before quiet callback"}[collide], func(t *testing.T) {
				operation := startQuietTest(t, raw, http.StatusOK, nil)
				operation.pendingRead(t)
				operation.body.chunks <- quietUsageFrame()
				operation.pendingRead(t)
				operation.cancel()
				if collide {
					operation.clock.advance(5 * time.Minute)
				}
				result := operation.joined(t)
				var failure *Error
				if !errors.As(result.err, &failure) || failure.Code != "canceled" || failure.Status != 499 ||
					failure.Outcome != "uncertain" || failure.NoEffect || failure.Facts != nil || !errors.Is(result.err, context.Canceled) {
					t.Fatalf("caller cancellation became a quiet timeout or replayable failure: %+v", result.err)
				}
				if operation.emitted.Load() != 1 || result.summary.Usage == nil ||
					result.summary.Usage.Input != 7 || result.summary.Usage.Output != 3 {
					t.Fatalf("caller cancellation discarded partial output or observed usage: emitted=%d usage=%+v",
						operation.emitted.Load(), result.summary.Usage)
				}
				if expired := operation.clock.advance(10 * time.Minute); expired != 0 {
					t.Fatal("cancellation left a read watchdog callback unjoined")
				}
			})
		}
	}
}
