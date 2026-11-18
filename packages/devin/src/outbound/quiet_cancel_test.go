package outbound

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

type quietCancelBody struct {
	*quietTestBody
	closeStarted chan struct{}
	closeRelease chan struct{}
	startOnce    sync.Once
	releaseOnce  sync.Once
}

func (body *quietCancelBody) release() {
	if body.closeRelease != nil {
		body.releaseOnce.Do(func() { close(body.closeRelease) })
	}
}

func (body *quietCancelBody) Close() error {
	if body.closeStarted != nil {
		body.startOnce.Do(func() { close(body.closeStarted) })
		<-body.closeRelease
	}
	return body.quietTestBody.Close()
}

func startQuietCancelTest(t *testing.T, raw bool, parent context.Context, holdClose bool) (*quietTestOperation, *quietCancelBody) {
	t.Helper()
	ctx, cancel := context.WithCancel(parent)
	operation := &quietTestOperation{
		ctx: ctx, cancel: cancel, result: make(chan quietTestResult, 1), done: make(chan struct{}),
		body: &quietTestBody{chunks: make(chan []byte, 1), ready: make(chan struct{}, 1), closed: make(chan struct{})},
	}
	body := &quietCancelBody{quietTestBody: operation.body}
	if holdClose {
		body.closeStarted = make(chan struct{})
		body.closeRelease = make(chan struct{})
	}
	called := make(chan struct{})
	client, err := New("http://127.0.0.1:1", &http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
		operation.sends.Add(1)
		operation.body.ctx = request.Context()
		close(called)
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/connect+proto"}}, Body: body,
		}, nil
	})}, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	client.quietTimer = operation.clock.after
	t.Cleanup(func() {
		cancel()
		body.release()
		select {
		case <-operation.done:
		case <-time.After(10 * time.Second):
			t.Error("ordinary cancellation-boundary operation did not join during cleanup")
		}
	})
	go func() {
		defer close(operation.done)
		accept := func(context.Context) error { operation.accepted.Add(1); return nil }
		var result quietTestResult
		if raw {
			result.summary, result.err = client.Native(ctx, Credentials{Key: "selected-key"}, fieldText(21, "model"), "model", accept, func(codec.NativeFrame) error {
				operation.emitted.Add(1)
				return nil
			})
		} else {
			result.summary, result.err = client.Complete(ctx, Credentials{Key: "selected-key"}, codec.EncodedChat{Payload: fieldText(21, "model")}, accept, func(codec.NativeDelta) error {
				operation.emitted.Add(1)
				return nil
			})
		}
		operation.result <- result
	}()
	select {
	case <-called:
	case <-operation.done:
		t.Fatal("ordinary cancellation-boundary call ended before dispatch")
	case <-time.After(10 * time.Second):
		t.Fatal("ordinary cancellation-boundary call did not reach dispatch")
	}
	return operation, body
}

func assertQuietCancelResult(t *testing.T, operation *quietTestOperation, result quietTestResult, status int, code string, kind, cause error) {
	t.Helper()
	var failure *Error
	typed := errors.As(result.err, &failure)
	if !typed || failure.Status != status || failure.Code != code || failure.Outcome != "uncertain" ||
		failure.Facts != nil || failure.NoEffect || !errors.Is(result.err, kind) || !errors.Is(result.err, cause) {
		if typed {
			t.Logf("structured boundary failure status=%d code=%s outcome=%s noEffect=%v kindMatches=%v causeMatches=%v",
				failure.Status, failure.Code, failure.Outcome, failure.NoEffect,
				errors.Is(result.err, kind), errors.Is(result.err, cause))
		}
		t.Fatalf("original parent termination kind and cause must own publication: want status=%d code=%s", status, code)
	}
	if operation.emitted.Load() != 1 || result.summary.Usage == nil ||
		result.summary.Usage.Input != 7 || result.summary.Usage.Output != 3 {
		t.Fatalf("parent termination discarded or repeated partial output/usage: emitted=%d usage=%+v",
			operation.emitted.Load(), result.summary.Usage)
	}
	if expired := operation.clock.advance(10 * time.Minute); expired != 0 {
		t.Fatal("parent termination left an unjoined watchdog callback")
	}
}

func TestQuietReadParentCancellationAfterWatchdog(t *testing.T) {
	for _, raw := range []bool{false, true} {
		t.Run(map[bool]string{false: "decoded", true: "raw"}[raw], func(t *testing.T) {
			parent, cancelParent := context.WithCancel(t.Context())
			defer cancelParent()
			operation, body := startQuietCancelTest(t, raw, parent, true)
			operation.pendingRead(t)
			operation.body.chunks <- quietUsageFrame()
			operation.pendingRead(t)
			expired := make(chan int, 1)
			callbackJoined := make(chan struct{})
			go func() {
				defer close(callbackJoined)
				expired <- operation.clock.advance(5 * time.Minute)
			}()
			t.Cleanup(func() {
				body.release()
				select {
				case <-callbackJoined:
				case <-time.After(10 * time.Second):
					t.Error("controlled watchdog callback did not join")
				}
			})
			select {
			case <-body.closeStarted:
			case <-time.After(10 * time.Second):
				t.Fatal("watchdog did not reach the actual held body.Close")
			}
			if parent.Err() != nil || operation.ctx.Err() != nil ||
				context.Cause(operation.body.ctx) != context.DeadlineExceeded {
				t.Fatal("timer-first boundary was not established while original caller stayed live")
			}
			select {
			case <-operation.done:
				t.Fatal("ordinary call returned before owned Close/callback joined")
			default:
			}
			cancelParent()
			if operation.ctx.Err() != context.Canceled {
				t.Fatal("original caller cancellation was not established before publication")
			}
			body.release()
			select {
			case <-callbackJoined:
			case <-time.After(10 * time.Second):
				t.Fatal("released watchdog callback did not join")
			}
			if count := <-expired; count != 1 {
				t.Fatalf("expected exactly one fired watchdog, got%d", count)
			}
			result := operation.joined(t)
			assertQuietCancelResult(t, operation, result, 499, "canceled", context.Canceled, context.Canceled)
		})
	}
}

func TestQuietReadCustomParentDeadlineCause(t *testing.T) {
	for _, raw := range []bool{false, true} {
		t.Run(map[bool]string{false: "decoded", true: "raw"}[raw], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cause := errors.New("fixture parent deadline cause")
				parent, cancelParent := context.WithTimeoutCause(t.Context(), time.Hour, cause)
				defer cancelParent()
				operation, _ := startQuietCancelTest(t, raw, parent, false)
				operation.pendingRead(t)
				operation.body.chunks <- quietUsageFrame()
				operation.pendingRead(t)
				time.Sleep(time.Hour)
				synctest.Wait()
				if parent.Err() != context.DeadlineExceeded || context.Cause(parent) != cause ||
					operation.ctx.Err() != context.DeadlineExceeded {
					t.Fatal("actual custom-cause parent deadline was not established after dispatch")
				}
				result := operation.joined(t)
				assertQuietCancelResult(t, operation, result, http.StatusGatewayTimeout, "deadline_exceeded", context.DeadlineExceeded, cause)
			})
		})
	}
}
