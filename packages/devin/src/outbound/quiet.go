package outbound

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

const quietReadTimeout = 5 * time.Minute

type quietBody struct {
	io.ReadCloser
	cancel   context.CancelCauseFunc
	timer    func(time.Duration, func()) func() bool
	close    sync.Once
	closeErr error
}

func (body *quietBody) Read(output []byte) (int, error) {
	done := make(chan struct{})
	stop := body.timer(quietReadTimeout, func() {
		defer close(done)
		body.cancel(context.DeadlineExceeded)
		body.Close()
	})
	count, err := body.ReadCloser.Read(output)
	if !stop() {
		<-done
	}
	return count, err
}

func (body *quietBody) Close() error {
	body.close.Do(func() { body.closeErr = body.ReadCloser.Close() })
	return body.closeErr
}

func readFailure(parent, readCtx context.Context, operation string) *Error {
	kind := context.Cause(readCtx)
	cause := kind
	if parentKind := parent.Err(); parentKind != nil {
		kind = parentKind
		cause = context.Cause(parent)
		if !errors.Is(cause, kind) {
			cause = errors.Join(kind, cause)
		}
	}
	if kind == nil {
		return nil
	}
	code := "canceled"
	if kind == context.DeadlineExceeded {
		code = "deadline_exceeded"
	}
	return &Error{Operation: operation, Status: nativeStatus(code), Code: code, Outcome: "uncertain", cause: cause}
}
