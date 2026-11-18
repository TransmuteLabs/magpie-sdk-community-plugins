package outbound

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

func TestRequestPolicyOutboundFactsPrecedeNormalization(t *testing.T) {
	for _, raw := range []bool{false, true} {
		for _, test := range []struct {
			code, message string
			rejected      bool
		}{
			{"failed_precondition", "User monthly ACU quota exhausted (trace ID: 12345)", true},
			{"failed_precondition", "Client version 3000.1.0 is no longer supported", true},
			{"permission_denied", "Your organization does not have access to this model", true},
			{"permission_denied", "high demand", false},
			{"resource_exhausted", "unknown failure with trace 42", false},
		} {
			t.Run(test.code+"/"+test.message+map[bool]string{false: "/translated", true: "/raw"}[raw], func(t *testing.T) {
				payload, _ := json.Marshal(map[string]any{"error": map[string]string{"code": test.code, "type": "original_type", "message": test.message}})
				wire := frame(2, payload)
				client, _ := New("http://127.0.0.1:1", &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/connect+proto"}}, Body: io.NopCloser(bytes.NewReader(wire))}, nil
				})}, nil)
				accept := func(context.Context) error { return nil }
				var err error
				if raw {
					_, err = client.Native(t.Context(), Credentials{Key: "selected"}, fieldText(21, "model"), "model", accept, func(codec.NativeFrame) error { return nil })
				} else {
					_, err = client.Complete(t.Context(), Credentials{Key: "selected"}, codec.EncodedChat{Payload: fieldText(21, "model")}, accept, func(codec.NativeDelta) error { return nil })
				}
				var failure *Error
				if !errors.As(err, &failure) || failure.Facts == nil || !failure.Facts.Complete ||
					failure.Facts.Status != 200 || failure.Facts.Code != test.code || failure.Facts.Type != "original_type" ||
					failure.Facts.Message != test.message || failure.NoEffect != test.rejected || failure.Outcome != "uncertain" {
					t.Fatal("original terminal facts or independent rejection proof lost", err, failure)
				}
				if strings.Contains(err.Error(), test.message) {
					t.Fatal("private vendor text escaped through public error")
				}
			})
		}
	}
}

func TestRequestPolicyTimingKnownnessAndBounds(t *testing.T) {
	for _, test := range []struct {
		message      string
		milliseconds int64
		known        bool
	}{
		{"Reached free model rate limit. Please wait before retrying. Your limit will reset in 42 seconds", 42000, true},
		{"Reached free model rate limit. Please wait before retrying. Your limit will reset in 2 minutes", 120000, true},
		{"Reached free model rate limit. Please wait before retrying. Your limit will reset in 2 HOURS", 7200000, true},
		{"Reached free model rate limit for devin/swe-2.", 0, false},
		{"Reached free model rate limit. Your limit will reset in 2 months", 0, false},
		{"Reached free model rate limit. Your limit will reset in -1 seconds", 0, false},
		{"Reached free model rate limit. Your limit will reset in 99999999999999999 HOURS", 0, false},
		{"Reached free model rate limit. Your limit will reset in 1.5 seconds", 0, false},
		{"Reached free model rate limit. Your limit will reset in 721 hours", 0, false},
	} {
		t.Run(test.message, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"code": "resource_exhausted", "message": test.message})
			err := remoteFailure("chat", 429, body, "uncertain", http.Header{"Content-Type": {"application/json"}})
			failure := err.(*Error)
			complete := test.known || !strings.Contains(test.message, "Your limit will reset in")
			if failure.Facts == nil || failure.Facts.Complete != complete || (failure.Facts.RetryAfterMS != nil) != test.known ||
				test.known && *failure.Facts.RetryAfterMS != test.milliseconds || len(failure.Facts.Headers) != 0 {
				t.Fatal("typed duration lost, guessed or converted to a fabricated header", failure)
			}
		})
	}
}

func TestRequestPolicyUntrustedEnvelopeAndHeadersNeverProveRejection(t *testing.T) {
	for _, body := range []string{
		`{"code":"resource_exhausted","message":"Reached free model rate limit","output":"billed"}`,
		`{"code":"resource_exhausted","code":"resource_exhausted","message":"Reached free model rate limit"}`,
		`{"error":{"code":"unavailable","code":"resource_exhausted","message":"Reached free model rate limit"}}`,
		`{"error":{"code":"resource_exhausted","message":"unknown","message":"Reached free model rate limit"}}`,
		`{"error":{"code":"resource_exhausted","type":"first","type":"second","message":"Reached free model rate limit"}}`,
		`{"code":"resource_exhausted","type":null,"message":"Reached free model rate limit"}`,
		`{"error":{"code":"resource_exhausted","type":null,"message":"Reached free model rate limit"}}`,
		`{"code":"unauthenticated","message":null}`,
		`{"error":{"code":"unauthenticated","message":null}}`,
		`{"Error":{"code":"resource_exhausted","message":"Reached free model rate limit"}}`,
		`{"error":{"code":"resource_exhausted","message":"Reached free model rate limit"},"metadata":{"trace":[null]}}`,
		`{"error":{"code":"resource_exhausted","message":"Reached free model rate limit"},"metadata":null}`,
		`{"error":{"code":"resource_exhausted","message":"Reached free model rate limit"},"metadata":{"trace":["first"],"trace":["second"]}}`,
		`{"code":"resource_exhausted","message":"Reached free model rate limit"}garbage`,
		`{"code":"resource_exhausted","message":"Reached free model rate limit` + strings.Repeat("x", 1024) + `"}`,
		`{"code":"resource_exhausted","message":"Reached free model rate limit","details":[{"usage":0}]}`,
	} {
		failure := remoteFailure("chat", 429, []byte(body), "uncertain", http.Header{"Content-Type": {"application/json"}}).(*Error)
		if failure.NoEffect {
			t.Fatal("incomplete or contradictory envelope proved no effects", body)
		}
	}
	failure := remoteFailure("chat", 429, []byte(`{"code":"resource_exhausted","message":"Reached free model rate limit"}`),
		"uncertain", http.Header{"Content-Type": {"application/json"}, "Retry-After": {"1", "2"}}).(*Error)
	if failure.Facts.Complete || failure.NoEffect {
		t.Fatal("ambiguous timing header acquired replay proof", failure)
	}
}
