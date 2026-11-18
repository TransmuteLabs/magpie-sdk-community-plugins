package devin

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

func cacheTestTemplate(t *testing.T, content string) codec.CacheKeepaliveTemplate {
	t.Helper()
	encoded, err := codec.EncodeChatRequest([]byte(fmt.Sprintf(
		`{"model":"fixture","messages":[{"role":"user","content":%q}]}`, content)), "not-retained")
	if err != nil {
		t.Fatal(err)
	}
	template, err := codec.NewCacheKeepaliveTemplate(encoded.Payload)
	if err != nil {
		t.Fatal(err)
	}
	return template
}

func TestCacheTemplateStoreFencesForegroundCompletionAndCallbackReplay(t *testing.T) {
	var store cacheTemplateStore
	authority := cacheTaskAuthority{Account: "account", Generation: 3, Model: "fixture"}
	policy := defaultCacheKeepalivePolicy()
	policy.Enabled = true
	first, _, err := store.begin("conversation")
	if err != nil {
		t.Fatal(err)
	}
	second, removed, err := store.begin("conversation")
	if err != nil || len(removed) != 1 || removed[0].ID != first || removed[0].Key != "conversation" {
		t.Fatal("next foreground did not invalidate prior work")
	}
	template := cacheTestTemplate(t, "private request")
	if _, armed := store.arm(first, template, authority, policy); armed {
		t.Fatal("stale foreground completion rearmed paid work")
	}
	if _, armed := store.arm(second, template, authority, policy); !armed {
		t.Fatal("current foreground could not arm")
	}
	wrong := authority
	wrong.Generation++
	if _, err := store.claim(second, 1, wrong); err == nil {
		t.Fatal("wrong account generation obtained a template")
	}
	var successes atomic.Int32
	var wait sync.WaitGroup
	for count := 0; count < 16; count++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := store.claim(second, 1, authority); err == nil {
				successes.Add(1)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 || !store.complete(second, 1, 200, &codec.NativeUsage{CacheRead: 100}) {
		t.Fatal("duplicate callback acquired work or first hit was discarded", successes.Load())
	}
	if _, err := store.claim(second, 1, authority); err == nil {
		t.Fatal("completed callback replay dispatched again")
	}
	entry, err := store.claim(second, 2, authority)
	if err != nil || entry.Progress.Sent != 2 {
		t.Fatal("fresh tick did not advance monotonically", entry, err)
	}
	_, removed, err = store.begin("conversation")
	if err != nil || len(removed) != 1 || removed[0].ID != second ||
		store.complete(second, 2, 200, &codec.NativeUsage{CacheRead: 100}) {
		t.Fatal("held tick rearmed after new foreground work")
	}
	store.close()
	if _, _, err := store.begin("later"); err == nil || store.current(second) || store.bytes != 0 {
		t.Fatal("shutdown retained templates or admitted work")
	}
}

func TestCacheTemplateStoreBoundsSlotsAndAggregatePayloads(t *testing.T) {
	var store cacheTemplateStore
	first := ""
	for position := 0; position < 33; position++ {
		identity, removed, err := store.begin(fmt.Sprintf("conversation-%d", position))
		if err != nil {
			t.Fatal(err)
		}
		if position == 0 {
			first = identity
		}
		if position == 32 && (len(removed) != 1 || removed[0].ID != first) {
			t.Fatal("oldest conversation was not evicted")
		}
	}
	if len(store.rows) != 32 || store.current(first) {
		t.Fatal("conversation count is unbounded")
	}
	template := cacheTestTemplate(t, string(bytes.Repeat([]byte("a"), 12<<20)))
	policy := defaultCacheKeepalivePolicy()
	policy.Enabled = true
	authority := cacheTaskAuthority{Account: "account", Generation: 1, Model: "fixture"}
	var eviction []cacheTaskReference
	for position := 0; position < 3; position++ {
		identity, _, err := store.begin(fmt.Sprintf("large-%d", position))
		if err != nil {
			t.Fatal(err)
		}
		removed, armed := store.arm(identity, template, authority, policy)
		if !armed {
			t.Fatal("bounded large template was not armed")
		}
		eviction = append(eviction, removed...)
	}
	if store.bytes > cacheTemplateBytes || len(eviction) == 0 {
		t.Fatal("aggregate retained payload limit was ignored", store.bytes)
	}
	store.close()
	if store.bytes != 0 || len(store.rows) != 0 {
		t.Fatal("closed store retains prompt templates")
	}
}

func TestCacheTemplateStoreCancelsActiveCallbacksOutsideJoinAndOnShutdown(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(fmt.Sprint(shutdown), func(t *testing.T) {
			var store cacheTemplateStore
			identity, _, err := store.begin("conversation")
			if err != nil {
				t.Fatal(err)
			}
			policy := defaultCacheKeepalivePolicy()
			policy.Enabled = true
			authority := cacheTaskAuthority{Account: "account", Generation: 1, Model: "fixture"}
			if _, armed := store.arm(identity, cacheTestTemplate(t, "prefix"), authority, policy); !armed ||
				!store.plan(identity, 1, 1800000000000) {
				t.Fatal("cannot arm test template")
			}
			if _, err := store.claim(identity, 1, authority); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan struct{})
			if !store.attach(identity, 1, 1800000000000, cancel, done) {
				t.Fatal("cannot attach callback lifetime")
			}
			go func() {
				<-ctx.Done()
				store.detach(identity, 1)
				close(done)
			}()
			if shutdown {
				store.close()
			} else {
				_, removed, err := store.begin("conversation")
				if err != nil || len(removed) != 1 || removed[0].Done != done {
					t.Fatal("new turn lost its active callback join")
				}
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("store retained active callback or blocked its completion under lock")
			}
			if store.complete(identity, 1, 200, &codec.NativeUsage{CacheRead: 100}) {
				t.Fatal("canceled callback was allowed to rearm")
			}
		})
	}
}
