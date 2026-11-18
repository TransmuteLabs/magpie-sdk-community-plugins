package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRefreshRejectsInvalidSelectedCredentialsBeforeDispatch(t *testing.T) {
	var calls atomic.Int32
	client, _ := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		io.WriteString(w, `{"access_token":"new","refresh_token":"rotated","expires_in":3600}`)
	})
	now := time.Now()
	for _, field := range []string{"access", "refresh", "user", "email", "expiry", "whitespace"} {
		t.Run(field, func(t *testing.T) {
			account := credential(now, "a", "org-a")
			switch field {
			case "access":
				account.Access = ""
			case "refresh":
				account.Refresh = ""
			case "user":
				account.UserID = ""
			case "email":
				account.Email = ""
			case "expiry":
				account.Expires = time.Time{}
			case "whitespace":
				account.Refresh = "private\nrefresh"
			}
			replacement, err := client.Refresh(t.Context(), account)
			if err == nil || replacement != (Credentials{}) || calls.Load() != 0 {
				t.Fatalf("invalid selected credentials reached refresh: err=%v calls=%d", err, calls.Load())
			}
			if strings.Contains(err.Error(), "private") {
				t.Fatal("invalid credentials appeared in an error")
			}
		})
	}
}

func TestRefreshValidatesRepliesWithoutPublishingOrRetry(t *testing.T) {
	const sentinel = "PRIVATE-REFRESH-SENTINEL"
	now := time.Now()
	for _, test := range []struct {
		name   string
		status int
		body   string
		code   string
	}{
		{"revoked", 400, `{"error":"invalid_grant","error_description":"` + sentinel + `"}`, "invalid_grant"},
		{"transient", 500, `{"error":"invalid_grant","error_description":"` + sentinel + `"}`, "invalid_grant"},
		{"redirect", 302, `{"access_token":"new","refresh_token":"rotated","expires_in":3600}`, ""},
		{"success-status-with-error", 200, `{"error":"invalid_grant"}`, "invalid_grant"},
		{"unknown-code", 400, `{"error":"` + sentinel + `"}`, ""},
		{"malformed", 200, sentinel, ""},
		{"missing-access", 200, `{"refresh_token":"rotated","expires_in":3600}`, ""},
		{"missing-refresh", 200, `{"access_token":"new","expires_in":3600}`, ""},
		{"zero-expiry", 200, `{"access_token":"new","refresh_token":"rotated","expires_in":0}`, ""},
		{"negative-expiry", 200, `{"access_token":"new","refresh_token":"rotated","expires_in":-1}`, ""},
		{"overflow-expiry", 200, `{"access_token":"new","refresh_token":"rotated","expires_in":9223372037}`, ""},
		{"fraction-expiry", 200, `{"access_token":"new","refresh_token":"rotated","expires_in":1.5}`, ""},
		{"invalid-access", 200, `{"access_token":"private\nvalue","refresh_token":"rotated","expires_in":3600}`, ""},
		{"invalid-refresh", 200, `{"access_token":"new","refresh_token":"private\tvalue","expires_in":3600}`, ""},
		{"oversized", 200, strings.Repeat(sentinel, maxReplyBytes/len(sentinel)+1), ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			client, _ := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(test.status)
				io.WriteString(w, test.body)
			})
			old := credential(now, "a", "org-a")
			before := old
			replacement, err := client.Refresh(t.Context(), old)
			if err == nil || replacement != (Credentials{}) || calls.Load() != 1 || old != before {
				t.Fatalf("invalid reply published, retried or changed source: err=%v calls=%d", err, calls.Load())
			}
			if strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), "private") {
				t.Fatal("refresh error disclosed vendor payload")
			}
			var remote *RemoteError
			if errors.As(err, &remote) && (remote.Status != test.status || remote.Code != test.code) {
				t.Fatalf("unexpected safe refusal classification: %+v", remote)
			}
		})
	}
}

func TestRefreshPreservesIdentityAndAnchorsReplacementExpiry(t *testing.T) {
	for _, test := range []struct {
		name  string
		extra string
		org   string
		label string
	}{
		{"org-omitted", "", "org-a", "Organization A"},
		{"org-null", `,"org_id":null`, "org-a", "Organization A"},
		{"same-org", `,"org_id":"org-a"`, "org-a", "Organization A"},
		{"org-changed", `,"org_id":"org-b"`, "org-b", "org-b"},
		{"org-cleared", `,"org_id":""`, "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			var clock atomic.Int64
			start := time.Date(2026, time.November, 13, 12, 0, 0, 0, time.UTC)
			clock.Store(start.UnixNano())
			var calls atomic.Int32
			client, _ := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				clock.Add(int64(15 * time.Second))
				io.WriteString(w, `{"access_token":"new-a","refresh_token":"rotated-a","expires_in":60`+test.extra+`}`)
			})
			client.Now = func() time.Time { return time.Unix(0, clock.Load()) }
			old := credential(start, "a", "org-a")
			old.Expires = start.Add(-time.Hour)
			old.OrgName = "Organization A"
			next, err := client.Refresh(t.Context(), old)
			if err != nil {
				t.Fatal(err)
			}
			if next.Access != "new-a" || next.Refresh != "rotated-a" ||
				!next.Expires.Equal(start.Add(75*time.Second)) || next.OrgID != test.org || next.OrgName != test.label ||
				next.UserID != old.UserID || next.Email != old.Email || next.InitialOrgID != old.InitialOrgID ||
				next.AccountID(ProviderGo) != old.AccountID(ProviderGo) || calls.Load() != 1 {
				t.Fatal("rotation changed stable account identity, extended TTL or fetched unrelated identity")
			}
		})
	}
}

func TestRefreshKeepsConcurrentAccountsSeparate(t *testing.T) {
	var calls atomic.Int32
	client, _ := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/console/auth/device/token" ||
			r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Opencode-Org-Id") != "" ||
			len(input) != 3 || input["grant_type"] != "refresh_token" || input["client_id"] != ClientID {
			t.Error("refresh request acquired foreign authority")
		}
		user := strings.TrimPrefix(input["refresh_token"], "refresh-")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "new-" + user, "refresh_token": "rotated-" + user, "expires_in": 3600})
	})
	now := time.Now()
	var wg sync.WaitGroup
	for _, user := range []string{"a", "b"} {
		wg.Go(func() {
			old := credential(now, user, "org-"+user)
			next, err := client.Refresh(t.Context(), old)
			if err != nil || next.Access != "new-"+user || next.Refresh != "rotated-"+user ||
				next.OrgID != old.OrgID || next.AccountID(ProviderGo) != old.AccountID(ProviderGo) {
				t.Error("refresh crossed account boundaries")
			}
		})
	}
	wg.Wait()
	if calls.Load() != 2 {
		t.Fatalf("refresh dispatched %d times, want exactly once for each account", calls.Load())
	}
}

func TestRefreshCancellationDoesNotRetry(t *testing.T) {
	dispatched := make(chan struct{})
	canceled := make(chan struct{})
	var calls atomic.Int32
	client, _ := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.Copy(io.Discard, r.Body)
		close(dispatched)
		<-r.Context().Done()
		close(canceled)
	})
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() {
		next, err := client.Refresh(ctx, credential(time.Now(), "a", "org-a"))
		if next != (Credentials{}) {
			t.Error("canceled rotation returned credentials")
		}
		done <- err
	}()
	select {
	case <-dispatched:
	case <-time.After(3 * time.Second):
		t.Fatal("fixture refresh did not dispatch")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("refresh lost cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("refresh did not cancel")
	}
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream request was not canceled")
	}
	if calls.Load() != 1 {
		t.Fatal("canceled refresh retried")
	}
}
