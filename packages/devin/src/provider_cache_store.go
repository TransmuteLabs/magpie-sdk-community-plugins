package devin

import (
	"context"
	"errors"
	"sync"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

const cacheTemplateBytes = 32 << 20

type cacheTaskAuthority struct {
	Account    string
	Generation uint64
	Model      string
}

type cacheTaskEntry struct {
	ID        string
	Key       string
	Authority cacheTaskAuthority
	Policy    cacheKeepalivePolicy
	Template  codec.CacheKeepaliveTemplate
	Progress  cachePingProgress
	Order     uint64
	Tick      uint64
	Running   bool
	DueAt     int64
	cancel    context.CancelFunc
	done      chan struct{}
}

type cacheTaskReference struct {
	ID   string
	Key  string
	Done <-chan struct{}
}

type cacheTemplateStore struct {
	mu     sync.Mutex
	rows   map[string]*cacheTaskEntry
	keys   map[string]string
	bytes  int
	order  uint64
	closed bool
}

func (store *cacheTemplateStore) removeLocked(identity string) {
	entry := store.rows[identity]
	if entry == nil {
		return
	}
	delete(store.rows, identity)
	if store.keys[entry.Key] == identity {
		delete(store.keys, entry.Key)
	}
	store.bytes -= entry.Template.Size()
	if entry.cancel != nil && entry.Running {
		entry.cancel()
	}
}

func (store *cacheTemplateStore) oldestLocked(except string) string {
	oldest, order := "", store.order
	for identity, entry := range store.rows {
		if identity != except && (oldest == "" || entry.Order < order) {
			oldest, order = identity, entry.Order
		}
	}
	return oldest
}

func (store *cacheTemplateStore) begin(key string) (string, []cacheTaskReference, error) {
	identity, err := randomIdentity()
	if err != nil || key == "" {
		return "", nil, errors.New("cache conversation identity is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return "", nil, errors.New("cache keepalive module is closed")
	}
	if store.rows == nil {
		store.rows, store.keys = make(map[string]*cacheTaskEntry), make(map[string]string)
	}
	var removed []cacheTaskReference
	if previous := store.keys[key]; previous != "" {
		removed = append(removed, store.referenceLocked(previous))
		store.removeLocked(previous)
	}
	for len(store.rows) >= 32 {
		oldest := store.oldestLocked("")
		removed = append(removed, store.referenceLocked(oldest))
		store.removeLocked(oldest)
	}
	store.order++
	store.rows[identity] = &cacheTaskEntry{ID: identity, Key: key, Order: store.order, Tick: 1}
	store.keys[key] = identity
	return identity, removed, nil
}

func (store *cacheTemplateStore) arm(identity string, template codec.CacheKeepaliveTemplate,
	authority cacheTaskAuthority, policy cacheKeepalivePolicy) ([]cacheTaskReference, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	entry := store.rows[identity]
	if store.closed || entry == nil || entry.Template.Size() != 0 || template.Size() == 0 ||
		template.Size() > cacheTemplateBytes || !policy.Enabled ||
		authority.Account == "" || authority.Generation == 0 || authority.Model == "" {
		return nil, false
	}
	var removed []cacheTaskReference
	for store.bytes+template.Size() > cacheTemplateBytes {
		oldest := store.oldestLocked(identity)
		if oldest == "" {
			return removed, false
		}
		removed = append(removed, store.referenceLocked(oldest))
		store.removeLocked(oldest)
	}
	entry.Template, entry.Authority, entry.Policy = template, authority, policy
	store.bytes += template.Size()
	return removed, true
}

func (store *cacheTemplateStore) referenceLocked(identity string) cacheTaskReference {
	entry := store.rows[identity]
	return cacheTaskReference{ID: identity, Key: entry.Key, Done: entry.done}
}

func (store *cacheTemplateStore) plan(identity string, tick uint64, dueAt int64) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	entry := store.rows[identity]
	if store.closed || entry == nil || entry.Running || entry.Tick != tick || entry.Template.Size() == 0 {
		return false
	}
	entry.DueAt = dueAt
	return true
}

func (store *cacheTemplateStore) attach(identity string, tick uint64, dueAt int64,
	cancel context.CancelFunc, done chan struct{}) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	entry := store.rows[identity]
	if store.closed || entry == nil || !entry.Running || entry.Tick != tick+1 || entry.DueAt != dueAt {
		return false
	}
	entry.cancel, entry.done = cancel, done
	return true
}

func (store *cacheTemplateStore) detach(identity string, tick uint64) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if entry := store.rows[identity]; entry != nil && entry.Tick == tick+1 {
		entry.cancel, entry.done = nil, nil
	}
}

func (store *cacheTemplateStore) claim(identity string, tick uint64, authority cacheTaskAuthority) (cacheTaskEntry, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	entry := store.rows[identity]
	if store.closed || entry == nil || entry.Running || entry.Tick != tick ||
		entry.Authority != authority || entry.Template.Size() == 0 {
		return cacheTaskEntry{}, errors.New("cache task is stale or belongs to another selected account")
	}
	entry.Running = true
	entry.Tick++
	entry.Progress.Sent++
	return *entry, nil
}

func (store *cacheTemplateStore) complete(identity string, tick uint64, status int, usage *codec.NativeUsage) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	entry := store.rows[identity]
	if store.closed || entry == nil || !entry.Running || entry.Tick != tick+1 {
		return false
	}
	entry.Running = false
	if !entry.Progress.keep(entry.Policy, status, usage) {
		store.removeLocked(identity)
		return false
	}
	return true
}

func (store *cacheTemplateStore) current(identity string) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	return !store.closed && store.rows[identity] != nil
}

func (store *cacheTemplateStore) remove(identity string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.removeLocked(identity)
}

func (store *cacheTemplateStore) close() {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.closed = true
	for identity := range store.rows {
		store.removeLocked(identity)
	}
	store.rows, store.keys, store.bytes = nil, nil, 0
}
