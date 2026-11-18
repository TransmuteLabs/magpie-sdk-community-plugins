package devin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

type cacheKeepalivePolicy struct {
	Enabled  bool
	Interval time.Duration
	MaxPings int
}

type cacheKeepaliveConfig struct {
	Server   string          `json:"server,omitempty"`
	Enabled  json.RawMessage `json:"keepAlive"`
	Interval json.RawMessage `json:"cacheKeepaliveInterval"`
	MaxPings json.RawMessage `json:"cacheKeepaliveMaxPings"`
}

func (configuration *cacheKeepaliveConfig) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := decodeParams(raw, &fields); err != nil {
		return err
	}
	next := cacheKeepaliveConfig{}
	for name, value := range fields {
		switch name {
		case "server":
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &next.Server) != nil {
				return errors.New("cache keepalive server requires an explicit string")
			}
		case "keepAlive":
			next.Enabled = value
		case "cacheKeepaliveInterval":
			next.Interval = value
		case "cacheKeepaliveMaxPings":
			next.MaxPings = value
		default:
			return errors.New("unknown cache keepalive option")
		}
	}
	*configuration = next
	return nil
}

func (configuration cacheKeepaliveConfig) policy() (cacheKeepalivePolicy, error) {
	var enabled *bool
	var interval *float64
	var maximum *int
	for _, field := range []struct {
		raw    json.RawMessage
		target any
	}{
		{configuration.Enabled, &enabled},
		{configuration.Interval, &interval},
		{configuration.MaxPings, &maximum},
	} {
		if len(field.raw) > 0 && (bytes.Equal(bytes.TrimSpace(field.raw), []byte("null")) ||
			json.Unmarshal(field.raw, field.target) != nil) {
			return cacheKeepalivePolicy{}, errors.New("cache keepalive options require their declared non-null types")
		}
	}
	return cachePolicy(enabled, interval, maximum)
}

func defaultCacheKeepalivePolicy() cacheKeepalivePolicy {
	return cacheKeepalivePolicy{Interval: 285 * time.Second, MaxPings: 6}
}

func cachePolicy(enabled *bool, interval *float64, maximum *int) (cacheKeepalivePolicy, error) {
	result := defaultCacheKeepalivePolicy()
	if enabled != nil {
		result.Enabled = *enabled
	}
	if interval != nil {
		if math.IsNaN(*interval) || math.IsInf(*interval, 0) || *interval < 30 || *interval > 299 {
			return result, errors.New("cache keepalive interval must be from 30 through 299 seconds")
		}
		result.Interval = time.Duration(*interval * float64(time.Second))
	}
	if maximum != nil {
		if *maximum < 1 || *maximum > 50 {
			return result, errors.New("cache keepalive maximum must be from 1 through 50 pings")
		}
		result.MaxPings = *maximum
	}
	return result, nil
}

func (policy cacheKeepalivePolicy) due(sent, now time.Time) time.Time {
	due := sent.Add(policy.Interval)
	if due.Before(now) {
		return now
	}
	return due
}

type cachePingProgress struct {
	Sent   int
	Misses int
}

func (progress *cachePingProgress) keep(policy cacheKeepalivePolicy, status int, usage *codec.NativeUsage) bool {
	if !policy.Enabled || status == 401 || status == 402 || status == 429 || progress.Sent >= policy.MaxPings {
		return false
	}
	if !usage.Measured(codec.InputCount | codec.CacheReadCount | codec.CacheWriteCount) {
		return false
	}
	hit := status == 200 && usage != nil && usage.CacheRead > 0 &&
		usage.CacheRead >= usage.Input && usage.CacheRead-usage.Input >= usage.CacheWrite
	if hit {
		progress.Misses = 0
	} else {
		progress.Misses++
	}
	return progress.Misses < 2
}

type cacheFirstMessage struct {
	Group  string
	Blocks []string
	Last   string
	Length int
	Count  int
	Order  uint64
}

type cacheConversationIndex struct {
	mu    sync.Mutex
	first map[string]cacheFirstMessage
	order uint64
}

var cacheBillingLine = regexp.MustCompile(`^x-anthropic-billing-header:(\s*[A-Za-z_]+=[^;\s]*;)*\s*(\n|$)`)

func cacheDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:16])
}

func cacheTexts(raw json.RawMessage) ([]string, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []string{text}, nil
	}
	if len(raw) == 0 || string(raw) == "null" {
		return []string{}, nil
	}
	var blocks []json.RawMessage
	if json.Unmarshal(raw, &blocks) != nil || blocks == nil {
		return nil, errors.New("invalid cache conversation content")
	}
	texts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if json.Unmarshal(block, &text) == nil {
			texts = append(texts, text)
			continue
		}
		var part struct {
			Type string  `json:"type"`
			Text *string `json:"text"`
		}
		if json.Unmarshal(block, &part) != nil {
			return nil, errors.New("invalid cache conversation block")
		}
		if part.Text != nil {
			texts = append(texts, *part.Text)
		} else {
			texts = append(texts, part.Type)
		}
	}
	return texts, nil
}

func (index *cacheConversationIndex) key(caller, protocol string, body []byte, headers http.Header) (string, error) {
	if caller == "" || len(body) > 16<<20 {
		return "", errors.New("cache conversation requires a bounded request and host caller")
	}
	var request struct {
		System   json.RawMessage `json:"system"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Tools []struct {
			Name     string `json:"name"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}
	if json.Unmarshal(body, &request) != nil || len(request.Messages) == 0 {
		return "", errors.New("invalid cache conversation")
	}
	session := ""
	for _, name := range []string{"X-Magpie-Session", "X-Claude-Code-Session-Id", "X-Session-Id", "Session_Id",
		"Session-Id", "X-Opencode-Session", "X-Session-Affinity", "X-Conversation-Id"} {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			session = value
			break
		}
	}
	agent := strings.TrimSpace(headers.Get("X-Magpie-Subagent"))
	if agent == "" {
		agent = strings.TrimSpace(headers.Get("X-Claude-Code-Agent-Id"))
	}
	var system any
	var first []string
	tools := make([]string, 0, len(request.Tools))
	switch protocol {
	case "anthropic":
		texts, err := cacheTexts(request.System)
		if err != nil {
			return "", err
		}
		for position := range texts {
			texts[position] = cacheBillingLine.ReplaceAllString(texts[position], "")
		}
		system = texts
		first, err = cacheTexts(request.Messages[0].Content)
		if err != nil {
			return "", err
		}
		for _, tool := range request.Tools {
			tools = append(tools, tool.Name)
		}
	case "chat":
		parts := make([][]string, 0)
		foundFirst := false
		for _, message := range request.Messages {
			texts, err := cacheTexts(message.Content)
			if err != nil {
				return "", err
			}
			if message.Role == "system" || message.Role == "developer" {
				parts = append(parts, texts)
			}
			if message.Role == "user" && !foundFirst {
				first, foundFirst = texts, true
			}
		}
		system = parts
		for _, tool := range request.Tools {
			tools = append(tools, tool.Function.Name)
		}
	default:
		return "", errors.New("unsupported cache conversation protocol")
	}
	prefix, err := json.Marshal([]any{system, tools})
	if err != nil {
		return "", err
	}
	groupBytes, _ := json.Marshal([]string{caller, protocol, session, agent, cacheDigest(prefix)})
	group := cacheDigest(groupBytes)
	if session != "" {
		return group, nil
	}
	return index.firstKey(group, first), nil
}

func (index *cacheConversationIndex) firstKey(group string, first []string) string {
	index.mu.Lock()
	defer index.mu.Unlock()
	if index.first == nil {
		index.first = make(map[string]cacheFirstMessage)
	}
	hashes := make([]string, len(first))
	for position, text := range first {
		hashes[position] = cacheDigest([]byte(text))
	}
	best := ""
	var most cacheFirstMessage
	for identity, previous := range index.first {
		if previous.Group != group || previous.Count > len(first) || (previous.Count == 0) != (len(first) == 0) {
			continue
		}
		matches := true
		for position, hash := range previous.Blocks {
			if hashes[position] != hash {
				matches = false
				break
			}
		}
		if matches && previous.Count > 0 {
			units := utf16.Encode([]rune(first[previous.Count-1]))
			matches = len(units) >= previous.Length &&
				cacheDigest([]byte(string(utf16.Decode(units[:min(previous.Length, len(units))])))) == previous.Last
		}
		if matches && (best == "" || previous.Count > most.Count ||
			previous.Count == most.Count && previous.Length > most.Length) {
			best, most = identity, previous
		}
	}
	if best == "" {
		encoded, _ := json.Marshal(first)
		best = group + "-" + cacheDigest(encoded)
	}
	value := cacheFirstMessage{Group: group, Count: len(first)}
	if len(first) > 0 {
		value.Blocks = append([]string(nil), hashes[:len(first)-1]...)
		value.Last = hashes[len(first)-1]
		value.Length = len(utf16.Encode([]rune(first[len(first)-1])))
	}
	index.order++
	value.Order = index.order
	index.first[best] = value
	if len(index.first) > 256 {
		oldest, order := "", index.order
		for identity, previous := range index.first {
			if previous.Order < order {
				oldest, order = identity, previous.Order
			}
		}
		delete(index.first, oldest)
	}
	return best
}
