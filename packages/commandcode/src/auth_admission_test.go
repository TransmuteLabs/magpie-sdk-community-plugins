package commandcode

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestAuthAdmissionReclaimsExpiredFlowsAndJoinsBrowser(t *testing.T) {
	c := &controller{ctx: context.Background(), flows: map[string]*authFlow{}}
	var handles []string
	var flows []*authFlow
	for range 32 {
		id, flow, err := c.addFlow("synthetic-key")
		if err != nil {
			t.Fatal(err)
		}
		handles = append(handles, id)
		flows = append(flows, flow)
	}
	t.Cleanup(func() {
		for _, f := range flows {
			f.close()
		}
	})
	if _, _, err := c.addFlow("new-key"); err == nil {
		t.Fatal("live-flow capacity was bypassed")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	expired := flows[0]
	expired.server = &http.Server{Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}
	expired.done = make(chan struct{})
	expired.expires = time.Now().Add(-time.Minute)
	expired.stopTimer = func() bool {
		if !c.mu.TryLock() {
			t.Error("flow cleanup held the controller lock")
		} else {
			c.mu.Unlock()
		}
		return true
	}
	go func() { defer close(expired.done); expired.server.Serve(listener) }()
	id, fresh, err := c.addFlow("new-key")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fresh.close)
	if len(c.flows) != 32 || c.flows[handles[0]] != nil || c.flows[id] != fresh || !expired.closed || expired.key != "" {
		t.Fatal("expired flow retained authority or capacity")
	}
	select {
	case <-expired.done:
	default:
		t.Fatal("expired browser server was not joined")
	}
	if flows[1].closed || flows[1].key != "synthetic-key" {
		t.Fatal("live flow was revoked")
	}
}

func TestAuthAdmissionConcurrentReapAndRemoval(t *testing.T) {
	c := &controller{ctx: context.Background(), flows: map[string]*authFlow{}}
	var wait sync.WaitGroup
	for range 16 {
		wait.Go(func() {
			for range 16 {
				id, flow, err := c.addFlow("synthetic-key")
				if err != nil {
					t.Error(err)
					return
				}
				flow.mu.Lock()
				flow.expires = time.Now().Add(-time.Minute)
				flow.mu.Unlock()
				c.removeFlow(id, flow)
			}
		})
	}
	wait.Wait()
	if len(c.flows) != 0 {
		t.Fatal("removed flow survived")
	}
}

func TestAuthAdmissionAfterDisposeRefusesAndErasesKeys(t *testing.T) {
	ready, dispose, err := start(context.Background(), sdk.ModuleContext{Config: sdk.ConfigSnapshot{Values: []byte(`{}`)}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dispose)
	c := ready.State.(*controller)
	_, flow, err := c.addFlow("synthetic-key")
	if err != nil {
		t.Fatal(err)
	}
	dispose()
	if _, _, err := c.addFlow("new-key"); err == nil {
		t.Fatal("disposed owner admitted a flow")
	}
	if len(c.flows) != 0 || !flow.closed || flow.key != "" {
		t.Fatal("disposed owner kept credential authority")
	}
}
