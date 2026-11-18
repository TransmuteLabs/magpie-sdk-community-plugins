package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func credential(now time.Time, user, org string) Credentials {
	return Credentials{Access: "access-" + user, Refresh: "refresh-" + user, Expires: now.Add(time.Hour),
		UserID: user, Email: user + "@example.invalid", InitialOrgID: org, OrgID: org, OrgName: org}
}

func fixtureClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(server.Client(), Endpoints{Console: server.URL + "/console", Inference: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return client, server
}

func TestDeviceWireAndSelectedOrganization(t *testing.T) {
	var poll atomic.Int32
	client, _ := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("User-Agent") != "opencode/latest/2.3.0/cli" {
			t.Errorf("unexpected client identity")
		}
		switch r.URL.Path {
		case "/console/auth/device/code":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["client_id"] != ClientID || body["supports_org_scope"] != true || r.Header.Get("Authorization") != "" {
				t.Error("device request changed")
			}
			io.WriteString(w, `{"device_code":"device","user_code":"CODE","verification_uri_complete":"verify?code=CODE","expires_in":60,"interval":0}`)
		case "/console/auth/device/token":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["grant_type"] != "urn:ietf:params:oauth:grant-type:device_code" || body["device_code"] != "device" || body["client_id"] != ClientID {
				t.Error("token request changed")
			}
			switch poll.Add(1) {
			case 1:
				w.WriteHeader(400)
				io.WriteString(w, `{"error":"authorization_pending"}`)
			case 2:
				w.WriteHeader(400)
				io.WriteString(w, `{"error":"slow_down"}`)
			default:
				io.WriteString(w, `{"access_token":"access","refresh_token":"refresh","expires_in":3600,"org_id":"org-b"}`)
			}
		case "/console/api/user":
			if r.Header.Get("Authorization") != "Bearer access" {
				t.Error("identity used another credential")
			}
			io.WriteString(w, `{"id":"user","email":"user@example.invalid"}`)
		case "/console/api/orgs":
			io.WriteString(w, `[{"id":"org-a","name":"Alpha"},{"id":"org-b","name":"Beta"}]`)
		default:
			t.Errorf("unexpected fixture route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	code, err := client.StartDevice(t.Context())
	if err != nil || !strings.Contains(code.URL, "/console/verify?code=CODE") || code.Interval != 1 {
		t.Fatalf("device result: %+v %v", code, err)
	}
	first, err := client.PollDevice(t.Context(), code.Device)
	if err != nil || !first.Pending || first.SlowDown {
		t.Fatalf("pending: %+v %v", first, err)
	}
	second, err := client.PollDevice(t.Context(), code.Device)
	if err != nil || !second.Pending || !second.SlowDown {
		t.Fatalf("slow down: %+v %v", second, err)
	}
	granted, err := client.PollDevice(t.Context(), code.Device)
	if err != nil || granted.Token == nil {
		t.Fatalf("grant: %+v %v", granted, err)
	}
	value, err := client.Identity(t.Context(), *granted.Token)
	if err != nil || value.OrgID != "org-b" || value.InitialOrgID != "org-b" || value.OrgName != "Beta" {
		t.Fatalf("organization authority: %+v %v", value, err)
	}
	if value.AccountID(ProviderGo) == value.AccountID(ProviderZen) {
		t.Fatal("Go and Zen share account identity")
	}
}

func TestIdentityRejectsMissingSelectedOrgAndAmbiguousFallback(t *testing.T) {
	client, _ := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/user") {
			io.WriteString(w, `{"id":"user","email":"user@example.invalid"}`)
		} else {
			io.WriteString(w, `[{"id":"b","name":"Beta"},{"id":"a","name":"Alpha"}]`)
		}
	})
	token := TokenReply{Access: "access", Refresh: "refresh", ExpiresIn: 60}
	if _, err := client.Identity(t.Context(), token); !errors.Is(err, ErrAmbiguousOrganization) {
		t.Fatalf("silently invented organization order: %v", err)
	}
	missing := "missing"
	token.OrgID = &missing
	if _, err := client.Identity(t.Context(), token); err == nil {
		t.Fatal("token-selected organization was silently replaced")
	}
}

func TestTokenExpirySurvivesIdentityLookupsAndDispatchBoundary(t *testing.T) {
	var clock atomic.Int64
	start := time.Date(2026, time.November, 13, 12, 0, 0, 0, time.UTC)
	clock.Store(start.UnixNano())
	var dispatches atomic.Int32
	client, _ := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/console/auth/device/token":
			clock.Add(int64(5 * time.Second))
			io.WriteString(w, `{"access_token":"access","refresh_token":"refresh","expires_in":90}`)
		case "/console/api/user":
			clock.Add(int64(20 * time.Second))
			io.WriteString(w, `{"id":"user","email":"user@example.invalid"}`)
		case "/console/api/orgs":
			clock.Add(int64(10 * time.Second))
			io.WriteString(w, `[{"id":"org","name":"Workspace"}]`)
		case "/inference/go/openai/v1/chat/completions":
			dispatches.Add(1)
			io.WriteString(w, `{"choices":[]}`)
		case "/zen/go/v1/usage":
			dispatches.Add(1)
			io.WriteString(w, `{"usage":{"rolling":{"percent":1}}}`)
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	client.Now = func() time.Time { return time.Unix(0, clock.Load()) }
	poll, err := client.PollDevice(t.Context(), "device")
	if err != nil || poll.Token == nil {
		t.Fatalf("token response: %v", err)
	}
	wantExpiry := client.Now().Add(90 * time.Second)
	// Even work between the token response and Identity consumes token TTL.
	clock.Add(int64(10 * time.Second))
	account, err := client.Identity(t.Context(), *poll.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !account.Expires.Equal(wantExpiry) {
		t.Errorf("identity lookup extended token expiry: got %s, want %s", account.Expires, wantExpiry)
	}
	for _, remaining := range []time.Duration{30*time.Second + time.Nanosecond, 30 * time.Second, 29 * time.Second, 0} {
		t.Run(remaining.String(), func(t *testing.T) {
			clock.Store(wantExpiry.Add(-remaining).UnixNano())
			before := dispatches.Load()
			response, chatErr := client.Chat(t.Context(), account, nil, []byte(`{"model":"deepseek-v4.1-flash","messages":[]}`))
			if response != nil {
				response.Body.Close()
			}
			_, usageErr := client.Usage(t.Context(), account, nil)
			if remaining > 30*time.Second {
				if chatErr != nil || usageErr != nil || dispatches.Load() != before+2 {
					t.Fatalf("fresh account was rejected: chat=%v usage=%v", chatErr, usageErr)
				}
			} else if !errors.Is(chatErr, ErrRefreshRequired) || !errors.Is(usageErr, ErrRefreshRequired) || dispatches.Load() != before {
				t.Errorf("due account dispatched: remaining=%s chat=%v usage=%v requests=%d", remaining, chatErr, usageErr, dispatches.Load()-before)
			}
		})
	}
}

func TestRefreshWireKeepsInitialIdentityAndSeparatesRefusal(t *testing.T) {
	now := time.Now()
	var status atomic.Int32
	status.Store(200)
	client, _ := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path != "/console/auth/device/token" || body["grant_type"] != "refresh_token" ||
			body["refresh_token"] != "refresh-user" || body["client_id"] != ClientID || r.Header.Get("Authorization") != "" {
			t.Error("refresh request changed")
		}
		w.WriteHeader(int(status.Load()))
		if status.Load() == 200 {
			io.WriteString(w, `{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600,"org_id":"new-org"}`)
		} else {
			io.WriteString(w, `{"error":"invalid_grant","error_description":"PRIVATE token details"}`)
		}
	})
	client.Now = func() time.Time { return now }
	old := credential(now, "user", "old-org")
	fresh, err := client.Refresh(t.Context(), old)
	if err != nil || fresh.Access != "new-access" || fresh.Refresh != "new-refresh" || fresh.OrgID != "new-org" ||
		fresh.InitialOrgID != old.InitialOrgID || fresh.AccountID(ProviderGo) != old.AccountID(ProviderGo) {
		t.Fatalf("refresh changed identity: %+v %v", fresh, err)
	}
	for _, code := range []int32{400, 500} {
		status.Store(code)
		_, err := client.Refresh(t.Context(), old)
		var remote *RemoteError
		if !errors.As(err, &remote) || remote.Status != int(code) || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatalf("refresh refusal classification: %v", err)
		}
	}
	if old.Access != "access-user" || old.Refresh != "refresh-user" || old.OrgID != "old-org" {
		t.Fatal("refresh mutated caller credentials")
	}
}

func TestSelectedAccountSigningBodyAndExpiry(t *testing.T) {
	now := time.Now()
	var mu sync.Mutex
	var seen []http.Header
	var paths, bodies []string
	client, _ := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen, paths, bodies = append(seen, r.Header.Clone()), append(paths, r.URL.Path), append(bodies, string(body))
		mu.Unlock()
		io.WriteString(w, `{"choices":[{"message":{"content":"fixture"}}]}`)
	})
	client.Now = func() time.Time { return now }
	input := http.Header{"Authorization": {"Bearer other"}, "Cookie": {"other=private"}, "X-Api-Key": {"other"},
		"X-Org-Id": {"wrong"}, "X-Opencode-Org-Id": {"wrong"}, "X-Fixed": {"kept"}}
	for _, user := range []string{"a", "b"} {
		res, err := client.Chat(t.Context(), credential(now, user, "org-"+user), input, []byte(`{"model":"deepseek-v4.1-flash","messages":[]}`))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	mu.Lock()
	defer mu.Unlock()
	for i, user := range []string{"a", "b"} {
		h := seen[i]
		if paths[i] != "/inference/go/openai/v1/chat/completions" || h.Get("Authorization") != "Bearer access-"+user ||
			h.Get("X-Opencode-Org-Id") != "org-"+user || h.Get("Cookie") != "" || h.Get("X-Api-Key") != "" ||
			h.Get("X-Org-Id") != "" || h.Get("X-Fixed") != "kept" ||
			bodies[i] != `{"model":"deepseek-v4.1-flash","messages":[]}` {
			t.Errorf("selected account %s changed wire", user)
		}
	}
	due := credential(now, "a", "org-a")
	due.Expires = now.Add(30 * time.Second)
	if _, err := client.Chat(t.Context(), due, input, nil); !errors.Is(err, ErrRefreshRequired) {
		t.Fatalf("due token was used or silently refreshed: %v", err)
	}
	if len(seen) != 2 || input.Get("Authorization") != "Bearer other" || input.Get("Cookie") == "" {
		t.Fatal("expiry dispatched or headers were mutated")
	}
}

func TestUsageWireAndZeroWindowReset(t *testing.T) {
	now := time.Now()
	client, _ := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zen/go/v1/usage" || r.Header.Get("Authorization") != "Bearer access-user" ||
			r.Header.Get("X-Opencode-Org-Id") != "org" || r.Header.Get("X-Opencode-Session") == "" {
			t.Error("usage account/path/attribution changed")
		}
		io.WriteString(w, `{"usage":{"rolling":{"percent":0,"resetsAt":"2026-11-13T14:00:00Z"},"weekly":{"percent":75,"resetsAt":"2026-11-20T14:00:00Z"},"monthly":{"percent":1}}}`)
	})
	client.Now = func() time.Time { return now }
	headers := http.Header{}
	NewHeaders().Session(headers, "account", now)
	windows, err := client.Usage(t.Context(), credential(now, "user", "org"), headers)
	if err != nil || len(windows) != 3 || windows[0].Used != 0 || windows[0].ResetsAt != nil ||
		windows[1].Used != 75 || windows[1].ResetsAt == nil || windows[0].Span != 5*time.Hour {
		t.Fatalf("quota semantics changed: %+v %v", windows, err)
	}
	for _, body := range []string{`{}`, `{"usage":{"rolling":{}}}`, `{"usage":null}`, `not-json`} {
		if _, err := ParseUsage([]byte(body)); err == nil {
			t.Fatalf("unknown usage became empty success: %s", body)
		}
	}
}

func TestHTTPFailureAndCancellationDoNotExposeVendorBodies(t *testing.T) {
	client, _ := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("wait") != "" {
			<-r.Context().Done()
			return
		}
		w.WriteHeader(401)
		io.WriteString(w, `{"error":"denied","token":"PRIVATE"}`)
	})
	if _, err := client.StartDevice(t.Context()); err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatalf("vendor body leaked: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.StartDevice(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was replaced: %v", err)
	}
}

func TestEndpointRootsRejectRemotePlaintextAndCredentialURLs(t *testing.T) {
	for _, root := range []string{"http://vendor.invalid", "https://user:password@vendor.invalid", "https://vendor.invalid?token=value", "file:///tmp/source"} {
		if _, err := NewClient(http.DefaultClient, Endpoints{Console: root, Inference: root}); err == nil {
			t.Fatalf("accepted invalid roots %q", root)
		}
	}
}

func TestDefaultHTTPDoesNotRedirectAccountCredentials(t *testing.T) {
	var received atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		t.Error("account request followed a redirect")
	}))
	t.Cleanup(other.Close)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/identity", http.StatusFound)
	}))
	t.Cleanup(origin.Close)
	transport, closeTransport, err := defaultHTTP("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeTransport)
	client, err := NewClient(transport, Endpoints{Console: origin.URL + "/console", Inference: origin.URL})
	if err != nil {
		t.Fatal(err)
	}
	org := "org"
	_, err = client.Identity(t.Context(), TokenReply{Access: "private-access", Refresh: "private-refresh", ExpiresIn: 60, OrgID: &org})
	if err == nil || received.Load() != 0 {
		t.Fatal("redirect was treated as identity success")
	}
}
