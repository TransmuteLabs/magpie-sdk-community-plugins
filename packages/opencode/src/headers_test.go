package opencode

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClientIdentityPreservesConversationAndSeparatesAccounts(t *testing.T) {
	state := NewHeaders()
	now := time.Now()
	project := DirectoryProject("/fixture")
	build := func(account, caller, seed string) http.Header {
		t.Helper()
		h := http.Header{"X-Opencode-Request": {"old"}}
		if err := state.Stamp(h, account, caller, seed, project, ClientVersion, now); err != nil {
			t.Fatal(err)
		}
		return h
	}
	first, next := build("a", "caller", "session"), build("a", "caller", "session")
	if !RealSession(first.Get("X-Opencode-Session")) || first.Get("X-Opencode-Session") != next.Get("X-Opencode-Session") ||
		first.Get("Traceparent")[:35] != next.Get("Traceparent")[:35] || first.Get("Traceparent") == next.Get("Traceparent") {
		t.Fatal("conversation session/trace changed or span was reused")
	}
	for _, h := range []http.Header{build("b", "caller", "session"), build("a", "other", "session")} {
		if h.Get("X-Opencode-Session") == first.Get("X-Opencode-Session") {
			t.Fatal("account/caller identities collided")
		}
	}
	for _, key := range []string{"X-Opencode-Session-Id", "X-Session-Affinity", "X-Session-Id"} {
		if first.Get(key) != first.Get("X-Opencode-Session") {
			t.Fatalf("session attribution %s changed", key)
		}
	}
	if first.Get("X-Opencode-Request") != "" || first.Get("X-Opencode-Project") != project ||
		first.Get("User-Agent") != "opencode/latest/2.3.0/cli" || first.Get("X-Opencode-Client") != "cli" {
		t.Fatal("client header shape changed")
	}
	real := "ses_ecc5d8f28ffe9Cy14V8Em7lI2u"
	if got := build("a", "caller", real).Get("X-Opencode-Session"); got != real {
		t.Fatalf("real OpenCode session changed: %s", got)
	}
}

func TestSessionCachesAreBoundedAndConcurrent(t *testing.T) {
	state := NewHeaders()
	var group sync.WaitGroup
	for i := range 1100 {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			if err := state.Stamp(http.Header{}, "account", "caller", strings.Repeat("x", i+1), "global", ClientVersion, time.Now()); err != nil {
				t.Error(err)
			}
		}(i)
	}
	group.Wait()
	if state.sessionOrder.Len() > identityLimit || state.traceOrder.Len() > identityLimit {
		t.Fatal("identity cache grew beyond its donor bound")
	}
}

func TestStampReplacesAttributionAcrossHeaderCasings(t *testing.T) {
	headers := http.Header{"User-Agent": {"foreign-canonical"}, "user-agent": {"foreign-lower"},
		"x-opencode-client": {"foreign"}, "x-opencode-project": {"foreign"},
		"x-opencode-session": {"foreign"}, "x-opencode-session-id": {"foreign"},
		"x-session-affinity": {"foreign"}, "x-session-id": {"foreign"},
		"x-opencode-request": {"foreign"}, "traceparent": {"foreign"}, "b3": {"foreign"},
		"accept": {"foreign"}, "X-Fixed": {"kept"}}
	if err := NewHeaders().Stamp(headers, "a", "caller", "seed", "global", ClientVersion, time.Now()); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for key, values := range headers {
		name := strings.ToLower(key)
		seen[name]++
		for _, value := range values {
			if strings.Contains(value, "foreign") {
				t.Errorf("foreign attribution survived in %s", key)
			}
		}
	}
	for name, count := range seen {
		if count != 1 {
			t.Errorf("duplicate attribution header %s", name)
		}
	}
	if headers.Get("X-Opencode-Project") != "global" || headers.Get("X-Opencode-Request") != "" ||
		headers.Get("X-Fixed") != "kept" {
		t.Fatal("configured attribution or unrelated header changed")
	}
}

func TestChatTerminalIgnoresTextAndHandlesLongStreams(t *testing.T) {
	var state chatTerminal
	state.write([]byte("data: {\"content\":\"[DONE]\"}\n\n"))
	state.write([]byte("data: " + strings.Repeat("x", 2<<20) + "\n\n"))
	if state.done {
		t.Fatal("model text was mistaken for completion")
	}
	for _, part := range []string{"da", "ta: [DO", "NE]\r", "\n\r\n"} {
		state.write([]byte(part))
	}
	if !state.done || cap(state.line) > 128 {
		t.Fatal("terminal event was lost or stream body was buffered")
	}
	var multiple chatTerminal
	multiple.write([]byte("data:\ndata: [DONE]\n\n"))
	if multiple.done {
		t.Fatal("multi-line event was changed into a terminal marker")
	}
	var commented chatTerminal
	commented.write([]byte(":" + strings.Repeat("keepalive", 30) + "\ndata: [DONE]\n\n"))
	if !commented.done {
		t.Fatal("a comment changed the terminal data event")
	}
}
