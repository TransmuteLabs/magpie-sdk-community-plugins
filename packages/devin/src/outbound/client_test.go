package outbound

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (transport transportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func fieldText(number int, value string) []byte {
	out := binary.AppendUvarint(nil, uint64(number<<3|2))
	out = binary.AppendUvarint(out, uint64(len(value)))
	return append(out, value...)
}

func fieldNumber(number int, value uint64) []byte {
	return binary.AppendUvarint(binary.AppendUvarint(nil, uint64(number<<3)), value)
}

func frame(flags byte, body []byte) []byte {
	out := []byte{flags, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(out[1:], uint32(len(body)))
	return append(out, body...)
}

func goodStream() []byte {
	data := append(fieldText(3, "answer"), fieldNumber(5, 2)...)
	data = append(data, fieldText(7, string(append(fieldNumber(2, 7), fieldNumber(3, 0)...)))...)
	return append(frame(0, data), frame(2, []byte(`{"metadata":{}}`))...)
}

func fixtureClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := New(server.URL, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return client, server
}

var validChat = []byte(`{"model":"swe-2-high","stream":true,"messages":[{"role":"user","content":"hello"}]}`)

func TestClientClaimsExactlyOnceBeforeNativeDispatch(t *testing.T) {
	var accepted, requests atomic.Int32
	client, _ := fixtureClient(t, func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if accepted.Load() != 1 || request.URL.Path != ChatPath || request.Header.Get("Authorization") != "Basic selected-key" ||
			request.Header.Get("Connect-Protocol-Version") != "1" {
			t.Error("native dispatch preceded claim or changed its auth/path")
		}
		body, _ := io.ReadAll(request.Body)
		_, payload, err := codec.ReadConnectFrame(bytes.NewReader(body), request.Header.Get("Connect-Content-Encoding") == "gzip")
		if err != nil || !bytes.Contains(payload, []byte("selected-key")) || !bytes.Contains(payload, []byte("swe-2-high")) {
			t.Error("native request envelope changed")
		}
		writer.Header().Set("Content-Type", "application/connect+proto")
		for _, value := range goodStream() {
			_, _ = writer.Write([]byte{value})
			writer.(http.Flusher).Flush()
		}
	})
	var text string
	summary, err := client.Chat(t.Context(), Credentials{Key: "selected-key"}, validChat,
		func(context.Context) error { accepted.Add(1); return nil },
		func(delta codec.NativeDelta) error { text += delta.Text; return nil })
	if err != nil || text != "answer" || summary.Stop != 2 || summary.Usage == nil || summary.Usage.Output != 0 ||
		accepted.Load() != 1 || requests.Load() != 1 {
		t.Fatalf("native response or claim changed: text=%q summary=%+v accepted=%d requests=%d err=%v",
			text, summary, accepted.Load(), requests.Load(), err)
	}
}

func TestClientRefusesBeforeClaimAndNetwork(t *testing.T) {
	var requests, accepted atomic.Int32
	client, _ := fixtureClient(t, func(writer http.ResponseWriter, request *http.Request) { requests.Add(1) })
	for name, test := range map[string]struct {
		credentials Credentials
		body        []byte
		accept      func(context.Context) error
	}{
		"invalid credential": {Credentials{Key: "PRIVATE\nkey"}, validChat, func(context.Context) error { accepted.Add(1); return nil }},
		"expired":            {Credentials{Key: "selected", ExpiresAt: time.Now().Add(-time.Hour)}, validChat, func(context.Context) error { accepted.Add(1); return nil }},
		"malformed":          {Credentials{Key: "selected"}, []byte(`{"bad":"PRIVATE"}`), func(context.Context) error { accepted.Add(1); return nil }},
		"no claim owner":     {Credentials{Key: "selected"}, validChat, nil},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := client.Chat(t.Context(), test.credentials, test.body, test.accept, nil)
			var failure *Error
			if !errors.As(err, &failure) || failure.Outcome != "not-started" || requests.Load() != 0 || accepted.Load() != 0 ||
				strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("invalid request crossed acceptance boundary: %+v requests=%d accepted=%d", err, requests.Load(), accepted.Load())
			}
		})
	}
	_, err := client.Chat(t.Context(), Credentials{Key: "selected"}, validChat,
		func(context.Context) error { accepted.Add(1); return errors.New("PRIVATE claim") }, nil)
	if err == nil || requests.Load() != 0 || accepted.Load() != 1 || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("failed durable claim dispatched request or leaked private error")
	}
}

func TestClientNeverRetriesOrFollowsNativeFailures(t *testing.T) {
	for _, test := range []struct {
		name     string
		status   int
		body     []byte
		kind     string
		expected int
	}{
		{"quota", 429, []byte(`{"code":"resource_exhausted","message":"PRIVATE"}`), "application/json", 429},
		{"terminal quota", 200, frame(2, []byte(`{"error":{"code":"resource_exhausted","message":"PRIVATE"}}`)), "application/connect+proto", 429},
		{"truncated", 200, frame(0, fieldText(3, "partial")), "application/connect+proto", 502},
		{"wrong mime", 200, goodStream(), "application/json", 502},
	} {
		t.Run(test.name, func(t *testing.T) {
			var attempts, accepted atomic.Int32
			client, _ := fixtureClient(t, func(writer http.ResponseWriter, request *http.Request) {
				attempts.Add(1)
				writer.Header().Set("Content-Type", test.kind)
				writer.WriteHeader(test.status)
				_, _ = writer.Write(test.body)
			})
			_, err := client.Chat(t.Context(), Credentials{Key: "selected"}, validChat,
				func(context.Context) error { accepted.Add(1); return nil }, nil)
			var failure *Error
			if !errors.As(err, &failure) || failure.Outcome != "uncertain" || failure.Status != test.expected || attempts.Load() != 1 || accepted.Load() != 1 ||
				strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("post-dispatch error lost uncertainty or retried: %+v attempts=%d", err, attempts.Load())
			}
		})
	}
	var redirected atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	t.Cleanup(other.Close)
	client, _ := fixtureClient(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Location", other.URL)
		writer.WriteHeader(http.StatusTemporaryRedirect)
	})
	_, err := client.Chat(t.Context(), Credentials{Key: "selected"}, validChat, func(context.Context) error { return nil }, nil)
	if err == nil || redirected.Load() != 0 {
		t.Fatal("native credential followed a redirect")
	}
}

func TestProviderAuthenticationFailuresKeepHTTPAndConnectMeaning(t *testing.T) {
	for _, test := range []struct {
		name, body, code string
		status, want     int
	}{
		{"bare unauthorized is transport only", "PRIVATE refused token", "", 401, 401},
		{"empty unauthorized is transport only", "", "", 401, 401},
		{"typed unauthorized", `{"code":"unauthenticated","message":"PRIVATE"}`, "unauthenticated", 401, 401},
		{"wrapped connect authentication", `{"error":{"code":"unauthenticated","message":"PRIVATE"}}`, "unauthenticated", 502, 401},
		{"HTTP cannot erase capacity code", `{"code":"unavailable","message":"PRIVATE"}`, "unavailable", 401, 401},
		{"HTTP cannot erase permission code", `{"code":"permission_denied","message":"model access is not allowed for this account"}`, "permission_denied", 401, 401},
		{"permission stays distinct", "PRIVATE", "", 403, 403},
		{"quota stays distinct", `{"code":"resource_exhausted","message":"PRIVATE"}`, "resource_exhausted", 429, 429},
		{"message is not policy", "PRIVATE token quota expired", "", 502, 502},
	} {
		t.Run(test.name, func(t *testing.T) {
			var attempts atomic.Int32
			client, _ := fixtureClient(t, func(writer http.ResponseWriter, request *http.Request) {
				attempts.Add(1)
				writer.WriteHeader(test.status)
				io.WriteString(writer, test.body)
			})
			credentials := Credentials{Key: "synthetic-selected-key"}
			operations := []func() error{
				func() error { _, err := client.Status(t.Context(), credentials, nil); return err },
				func() error { _, err := client.Models(t.Context(), credentials); return err },
				func() error {
					_, err := client.Chat(t.Context(), credentials, validChat,
						func(context.Context) error { return nil }, func(codec.NativeDelta) error { return nil })
					return err
				},
			}
			for index, operation := range operations {
				err := operation()
				var failure *Error
				if !errors.As(err, &failure) || failure.Code != test.code || failure.Status != test.want ||
					strings.Contains(err.Error(), "PRIVATE") || attempts.Load() != int32(index+1) {
					t.Fatalf("operation %d lost auth classification or retried: failure=%+v attempts=%d", index, failure, attempts.Load())
				}
				if index == 2 && failure.Outcome != "uncertain" {
					t.Fatal("dispatched inference became replayable", failure)
				}
			}
		})
	}
}

func TestClientTransportFailureRemainsUncertainAndPrivate(t *testing.T) {
	var attempts atomic.Int32
	client, err := New("https://server.example.invalid", &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		attempts.Add(1)
		return nil, errors.New("PRIVATE network credential")
	})}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Chat(t.Context(), Credentials{Key: "selected"}, validChat, func(context.Context) error { return nil }, nil)
	var failure *Error
	if !errors.As(err, &failure) || failure.Outcome != "uncertain" || attempts.Load() != 1 || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("native transport failure was retried, leaked, or declared safe to retry")
	}
}

func TestClientAcceptanceFailureDoesNotClaimTheJournalIsUnstarted(t *testing.T) {
	var requests atomic.Int32
	client, _ := fixtureClient(t, func(http.ResponseWriter, *http.Request) { requests.Add(1) })
	_, err := client.Chat(t.Context(), Credentials{Key: "selected"}, validChat,
		func(context.Context) error { return errors.New("acknowledgement lost after durable acceptance") }, nil)
	var failure *Error
	if !errors.As(err, &failure) || failure.Outcome != "uncertain" || requests.Load() != 0 {
		t.Fatalf("ambiguous acceptance was reported safe to replay: %+v requests=%d", err, requests.Load())
	}
}

func TestClientCancellationReachesNativeHTTPBody(t *testing.T) {
	cancelled := make(chan struct{})
	client, _ := fixtureClient(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/connect+proto")
		_, _ = writer.Write(frame(0, fieldText(3, "partial")))
		writer.(http.Flusher).Flush()
		<-request.Context().Done()
		close(cancelled)
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, err := client.Chat(ctx, Credentials{Key: "selected"}, validChat, func(context.Context) error { return nil },
		func(codec.NativeDelta) error { cancel(); return nil })
	var failure *Error
	if !errors.Is(err, context.Canceled) || !errors.As(err, &failure) || failure.Outcome != "uncertain" {
		t.Fatal("native cancellation identity or uncertainty lost:", err)
	}
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not observe cancellation")
	}
}

func TestClientModelsAndUsageKeepAccountAuthority(t *testing.T) {
	client, _ := fixtureClient(t, func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if !bytes.Contains(body, []byte("selected-key")) || request.Header.Get("Authorization") != "" {
			t.Error("read-only native RPC changed its credential envelope")
		}
		switch request.URL.Path {
		case ModelsPath:
			info := append(fieldNumber(4, 32768), fieldNumber(13, 4096)...)
			model := append(fieldText(22, "swe-2-high"), fieldNumber(5, 1)...)
			model = append(model, fieldNumber(24, 4)...)
			model = append(model, fieldText(23, string(info))...)
			writer.Header().Set("Content-Type", "application/proto")
			_, _ = writer.Write(fieldText(1, string(model)))
		case StatusPath:
			var value struct {
				Metadata map[string]string `json:"metadata"`
			}
			if json.Unmarshal(body, &value) != nil || value.Metadata["apiKey"] != "selected-key" || value.Metadata["ideName"] != "devin-cli" {
				t.Error("status request metadata mismatch")
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"userStatus":{"email":"fixture@example.invalid","teamId":"team-a","planStatus":{"dailyQuotaRemainingPercent":75,"dailyQuotaResetAtUnix":"1900000000","weeklyQuotaResetAtUnix":"1900100000","planEnd":"2030-01-01T00:00:00Z","acuConsumed":"1.5","acuLimit":"5","overageBalanceMicros":"1200000","planInfo":{"billingStrategy":"BILLING_STRATEGY_QUOTA","planName":"Pro","devinInfo":{"orgId":"org-a","accountDisplayName":"Fixture"}}}}}`)
		default:
			t.Error("unexpected native read-only RPC path")
		}
	})
	models, err := client.Models(t.Context(), Credentials{Key: "selected-key"})
	if err != nil || len(models) != 1 || !models[0].Free || !models[0].Images {
		t.Fatalf("native models lost: %+v %v", models, err)
	}
	status, err := client.Status(t.Context(), Credentials{Key: "selected-key"}, []string{models[0].UID})
	if err != nil || status.Email != "fixture@example.invalid" || status.OrgID != "org-a" || len(status.Windows) != 2 ||
		status.Windows[0].Used != 25 || status.Windows[1].Used != 100 || status.Windows[1].NotModels[0] != "swe-2-high" ||
		status.ACUUsed != 1.5 || status.BalanceMicros != 1200000 {
		t.Fatalf("native quota or free model exclusion lost: %+v %v", status, err)
	}
}

func TestStatusNeverInventsMissingQuotaAndRejectsInvalidCounters(t *testing.T) {
	value, err := ParseStatus([]byte(`{"userStatus":{"planStatus":{"dailyQuotaResetAtUnix":"1900000000"}}}`), nil)
	if err != nil || len(value.Windows) != 0 {
		t.Fatal("missing quota in non-quota plan became used-up quota")
	}
	for _, body := range []string{
		`{}`,
		`{"userStatus":{"planStatus":{"dailyQuotaRemainingPercent":101}}}`,
		`{"userStatus":{"planStatus":{"dailyQuotaRemainingPercent":"NaN"}}}`,
		`{"userStatus":{"planStatus":{"weeklyQuotaResetAtUnix":"1.5"}}}`,
		`{"userStatus":{"planStatus":{"planEnd":"private-date"}}}`,
		`{"userStatus":{"planStatus":{"acuConsumed":-1}}}`,
		`{"userStatus":{"planStatus":{"overageBalanceMicros":"9007199254740993"}}}`,
	} {
		if _, err := ParseStatus([]byte(body), nil); err == nil {
			t.Fatal("invalid native status became a trustworthy reading")
		}
	}
}

func TestPKCEUsesRecoveredExchangeAndCallbackOwnership(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	flow, address, err := PreparePKCE("http://127.0.0.1:12345/callback", now)
	if err != nil {
		t.Fatal(err)
	}
	authorization, _ := url.Parse(address)
	if authorization.Query().Get("code_challenge_method") != "S256" || authorization.Query().Get("cli_pkce_marker") != "1" ||
		strings.Contains(address, flow.Verifier) {
		t.Fatal("PKCE verifier exposed or native authorization parameters lost")
	}
	var exchanges atomic.Int32
	client, _ := fixtureClient(t, func(writer http.ResponseWriter, request *http.Request) {
		exchanges.Add(1)
		if request.URL.Path != ExchangePath || request.Header.Get("Authorization") != "" {
			t.Error("wrong native PKCE exchange")
		}
		body, _ := io.ReadAll(request.Body)
		var fields map[string]string
		if json.Unmarshal(body, &fields) != nil || fields["code"] != "callback-code" || fields["code_verifier"] != flow.Verifier || len(fields) != 2 {
			t.Error("PKCE exchange fields diverged from generated request")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"sessionToken":"devin-session-token$synthetic","devinWebappHost":"app.devin.ai","devinApiUrl":"https://api.devin.ai"}`)
	})
	client.now = func() time.Time { return now.Add(time.Second) }
	callback := flow.Redirect + "?" + url.Values{"state": {flow.State}, "code": {"callback-code"}}.Encode()
	result, err := client.Exchange(t.Context(), flow, callback)
	if err != nil || result.Credentials.Key != "devin-session-token$synthetic" || exchanges.Load() != 1 {
		t.Fatal("native PKCE exchange failed:", err)
	}
	for _, bad := range []string{
		flow.Redirect + "?code=callback-code&state=wrong",
		callback + "&state=" + flow.State,
		strings.Replace(callback, "127.0.0.1", "localhost", 1),
		callback + "&error=private",
	} {
		if _, err := client.Exchange(t.Context(), flow, bad); err == nil || exchanges.Load() != 1 {
			t.Fatal("unowned callback reached native exchange")
		}
	}
	client.now = func() time.Time { return now.Add(11 * time.Minute) }
	if _, err := client.Exchange(t.Context(), flow, callback); err == nil || exchanges.Load() != 1 {
		t.Fatal("expired PKCE flow exchanged a code")
	}
}

func TestClientRejectsUnsafeOriginsAndSeparatesCredentials(t *testing.T) {
	for _, server := range []string{"http://example.invalid", "https://user:pass@example.invalid", "https://example.invalid/path", "https://example.invalid?key=private"} {
		if _, err := New(server, nil, nil); err == nil {
			t.Fatal("unsafe credential target accepted")
		}
	}
	first := Credentials{Key: "key-a"}
	second := Credentials{Key: "key-b"}
	if first.AccountID(DefaultServer) == second.AccountID(DefaultServer) || first.AccountID(DefaultServer) == first.AccountID("https://other.invalid") {
		t.Fatal("unidentified accounts collapsed")
	}
	if first.Due(time.Now()) {
		t.Fatal("static key was assigned an invented expiry")
	}
}

func TestPKCERefusesDuplicateCredentialFields(t *testing.T) {
	now := time.Now()
	flow, _, err := PreparePKCE("http://127.0.0.1:12345/callback", now)
	if err != nil {
		t.Fatal(err)
	}
	client, _ := fixtureClient(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"sessionToken":"PRIVATE-first","sessionToken":"PRIVATE-last"}`)
	})
	callback := flow.Redirect + "?" + url.Values{"state": {flow.State}, "code": {"code"}}.Encode()
	value, err := client.Exchange(t.Context(), flow, callback)
	if err == nil || value.Credentials.Key != "" || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatalf("ambiguous exchange credentials were published: err=%v", err)
	}
}

func TestStatusRefusesDuplicateQuotaFields(t *testing.T) {
	if _, err := ParseStatus([]byte(`{"userStatus":{"planStatus":{"dailyQuotaRemainingPercent":0,"dailyQuotaRemainingPercent":100}}}`), nil); err == nil {
		t.Fatal("ambiguous quota became a trustworthy reading")
	}
}
