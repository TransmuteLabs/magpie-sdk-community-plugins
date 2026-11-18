package outbound

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

func TestNativeClientRequiresAcceptanceAndReplacesIncomingAuthority(t *testing.T) {
	var requests, accepted atomic.Int32
	payload := append(fieldText(1, string(fieldText(3, "local-secret"))), fieldText(21, "old-model")...)
	client, err := New("http://127.0.0.1:1", &http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		body, _ := io.ReadAll(request.Body)
		_, native, err := codec.ReadConnectFrame(bytes.NewReader(body), false)
		if err != nil || bytes.Contains(native, []byte("local-secret")) || bytes.Contains(native, []byte("old-model")) ||
			!bytes.Contains(native, []byte("selected-key")) || request.Header.Get("Authorization") != "Basic selected-key" {
			t.Error("native authority replacement failed")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/connect+proto"}}, Body: io.NopCloser(bytes.NewReader(goodStream()))}, nil
	})}, nil)
	if err != nil {
		t.Fatal(err)
	}
	accept := func(context.Context) error { accepted.Add(1); return nil }
	emit := func(codec.NativeFrame) error { return nil }
	for _, test := range []struct {
		payload []byte
		accept  func(context.Context) error
	}{
		{[]byte{0}, accept}, {payload, nil},
	} {
		_, err := client.Native(t.Context(), Credentials{Key: "selected-key"}, test.payload, "new-model", test.accept, emit)
		var failure *Error
		if !errors.As(err, &failure) || failure.Outcome != "not-started" || requests.Load() != 0 || accepted.Load() != 0 {
			t.Fatal("native invalid input crossed acceptance boundary", err)
		}
	}
	_, err = client.Native(t.Context(), Credentials{Key: "selected-key"}, payload, "new-model",
		func(context.Context) error { accepted.Add(1); return errors.New("private claim") }, emit)
	var failure *Error
	if !errors.As(err, &failure) || failure.Outcome != "uncertain" || requests.Load() != 0 || accepted.Load() != 1 {
		t.Fatal("failed native claim dispatched or became replayable", err)
	}
	summary, err := client.Native(t.Context(), Credentials{Key: "selected-key"}, payload, "new-model", accept, emit)
	if err != nil || requests.Load() != 1 || accepted.Load() != 2 || summary.Usage == nil || summary.Usage.Output != 0 {
		t.Fatal("native execution failed", err)
	}
}

func TestNativeClientPreservesTerminalRefusalButNeverRetriesIt(t *testing.T) {
	var requests atomic.Int32
	end := frame(2, []byte(`{"error":{"code":"resource_exhausted","message":"private native details"}}`))
	client, _ := New("http://127.0.0.1:1", &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/connect+proto"}}, Body: io.NopCloser(bytes.NewReader(end))}, nil
	})}, nil)
	var terminal []byte
	_, err := client.Native(t.Context(), Credentials{Key: "selected"}, fieldText(21, "model"), "model",
		func(context.Context) error { return nil }, func(value codec.NativeFrame) error { terminal = value.Payload; return nil })
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != "resource_exhausted" || failure.Outcome != "uncertain" || requests.Load() != 1 ||
		!bytes.Equal(terminal, end[5:]) {
		t.Fatal("terminal native refusal was lost or replayed", err)
	}
}
