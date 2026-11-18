package opencode

import (
	"container/list"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const identityLimit = 1000
const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

type keptIdentity struct {
	key   string
	value string
}

// Headers is owned by one module instance, never shared across Serve lifetimes.
type Headers struct {
	mu           sync.Mutex
	last         int64
	counter      uint64
	sessions     map[string]*list.Element
	sessionOrder *list.List
	traces       map[string]*list.Element
	traceOrder   *list.List
}

func NewHeaders() *Headers {
	return &Headers{sessions: map[string]*list.Element{}, sessionOrder: list.New(),
		traces: map[string]*list.Element{}, traceOrder: list.New()}
}

func cachedIdentity(values map[string]*list.Element, order *list.List, key, value string) string {
	if found := values[key]; found != nil {
		order.MoveToBack(found)
		return found.Value.(keptIdentity).value
	}
	if order.Len() >= identityLimit {
		oldest := order.Front()
		order.Remove(oldest)
		delete(values, oldest.Value.(keptIdentity).key)
	}
	values[key] = order.PushBack(keptIdentity{key, value})
	return value
}

func randomHex(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func RealSession(id string) bool {
	if len(id) != 30 || !strings.HasPrefix(id, "ses_") {
		return false
	}
	if _, err := hex.DecodeString(id[4:16]); err != nil || strings.ToLower(id[4:16]) != id[4:16] {
		return false
	}
	for _, c := range id[16:] {
		if !strings.ContainsRune(base62, c) {
			return false
		}
	}
	low, err := strconv.ParseUint(id[13:16], 16, 16)
	return err == nil && 0xfff-low < 16
}

func (headers *Headers) session(account, caller, seed string, now time.Time) (string, error) {
	if RealSession(seed) {
		return seed, nil
	}
	var random [14]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	headers.mu.Lock()
	defer headers.mu.Unlock()
	ms := now.UnixMilli()
	if ms != headers.last {
		headers.last, headers.counter = ms, 0
	}
	headers.counter++
	current := ^(uint64(ms)*0x1000 + headers.counter)
	id := []byte(fmt.Sprintf("ses_%012x", current&0xffffffffffff))
	for _, c := range random {
		id = append(id, base62[int(c)%len(base62)])
	}
	if seed == "" {
		return string(id), nil
	}
	return cachedIdentity(headers.sessions, headers.sessionOrder, account+"\x00"+caller+"\x00"+seed, string(id)), nil
}

// Session adds only the stateless attribution used by the donor's usage read.
func (headers *Headers) Session(h http.Header, account string, now time.Time) error {
	session, err := headers.session(account, "", "", now)
	if err != nil {
		return err
	}
	h.Set("x-opencode-session", session)
	h.Set("x-opencode-session-id", session)
	return nil
}

func validProject(value string) bool {
	if value == "global" {
		return true // explicitly selected config, never an inferred fallback
	}
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// DirectoryProject is the donor's no-repository formula. It does not inspect
// a directory, Git config, an agent installation or the process working dir.
func DirectoryProject(directory string) string {
	sum := sha1.Sum([]byte("directory:" + directory))
	return hex.EncodeToString(sum[:])
}

func (headers *Headers) Stamp(h http.Header, account, caller, seed, project, version string, now time.Time) error {
	if !validProject(project) {
		return errors.New("an explicit OpenCode project identity is required")
	}
	session, err := headers.session(account, caller, seed, now)
	if err != nil {
		return err
	}
	freshTrace, err := randomHex(16)
	if err != nil {
		return err
	}
	headers.mu.Lock()
	trace := cachedIdentity(headers.traces, headers.traceOrder, account+"\x00"+caller+"\x00"+session, freshTrace)
	headers.mu.Unlock()
	span, err := randomHex(8)
	if err != nil {
		return err
	}
	parent, err := randomHex(8)
	if err != nil {
		return err
	}
	for key := range h {
		switch strings.ToLower(key) {
		case "user-agent", "b3", "traceparent", "x-opencode-client", "x-opencode-project",
			"x-opencode-session", "x-opencode-session-id", "x-session-affinity", "x-session-id",
			"x-opencode-request", "accept":
			delete(h, key)
		}
	}
	h.Set("User-Agent", "opencode/latest/"+version+"/cli")
	h.Set("b3", trace+"-"+span+"-1-"+parent)
	h.Set("traceparent", "00-"+trace+"-"+span+"-01")
	h.Set("x-opencode-client", "cli")
	h.Set("x-opencode-project", project)
	for _, key := range []string{"x-opencode-session", "x-opencode-session-id", "x-session-affinity", "x-session-id"} {
		h.Set(key, session)
	}
	h.Set("Accept", "*/*")
	return nil
}
