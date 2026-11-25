package codearts

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const (
	ToolNameLimit = 64
	ToolNameKeep  = 55
)

var (
	reasoningLevels = map[string][]string{
		"deepseek-v4.1-flash-1017": {"none", "low", "medium", "high", "xhigh", "max"},
		"deepseek-v4.1-flash":      {"none", "low", "medium", "high", "xhigh", "max"},
		"glm-5.3-flash":            {"none", "low", "high", "max"},
		"glm-5.3":                  {"none", "low", "medium", "high", "xhigh", "max"},
		"glm-5.2":                  {"none", "low", "medium", "high", "xhigh", "max"},
		"glm-5.2-sft-harmony":      {"none", "low", "medium", "high", "xhigh", "max"},
		"openpangu-2.0-pro":        {"none", "low", "medium", "high", "xhigh", "max"},
		"openpangu-2.0-flash":      {"none", "low", "medium", "high", "xhigh", "max"},
		"speed":                    {"none", "low", "medium", "high", "xhigh", "max"},
		"kimi-k3":                  {"low", "medium", "high", "xhigh", "max"},
		"kimi-k3.1-pro":            {"low", "medium", "high", "xhigh", "max"},
	}
	noThinkingLevels = map[string]bool{"none": true, "off": true, "disabled": true}

	errorCodePattern    = regexp.MustCompile(`"(?:error_code|code)"\s*:\s*"((?:[A-Za-z]+\.)*[A-Za-z]*[0-9][0-9A-Za-z.]*)"`)
	errorMsgPattern     = regexp.MustCompile(`"(?:error_msg|message)"\s*:\s*"((?:[^"\\]|\\.)*)"`)
	staleCredPattern    = regexp.MustCompile(`(?i)APIG\.0(301|307|602)|security token has expired|verify ak sk signature fail`)
	bodyTooLargePattern = regexp.MustCompile(`(?i)body too large`)
	rateLimitPattern    = regexp.MustCompile(`4291|8111\d|(^|\.)429(\.|$)`)
	clientErrPattern    = regexp.MustCompile(`(^|\.)40[0-4](\.|$)|InferHub\.400[0-9]\.`)
	notMultimodalPat    = regexp.MustCompile(`(?i)InferHub\.001001020\.406|not multimodal`)
	noBenefitChanPat    = regexp.MustCompile(`(?i)InferHub\.APIG\.0101\.404`)
	noModelBenefitPat   = regexp.MustCompile(`(?i)InferHub\.400[45]\.200|benefit not found`)
	doneLinePattern     = regexp.MustCompile(`(?m)^data:\s*\[DONE\]\s*$`)
	oaBasicRejectPat    = regexp.MustCompile(`(?i)APIG\.1009|invalid.*(app|key|secret)|appkey|app_key`)
)

func ReasoningLevelsOf(model string) []string {
	return reasoningLevels[strings.ToLower(strings.TrimSpace(model))]
}

func UpstreamModelID(model string) string {
	trimmed := strings.TrimSpace(model)
	if idx := strings.IndexByte(trimmed, '/'); idx >= 0 {
		return trimmed[idx+1:]
	}
	return trimmed
}

type PreparedChat struct {
	Body     map[string]any
	Fold     bool
	Injected bool
	Names    map[string]string
}

func PrepareChatBody(parsed map[string]any, thinking bool, stationID string) *PreparedChat {
	if parsed == nil {
		return nil
	}
	rawModel, ok := parsed["model"].(string)
	if !ok || rawModel == "" {
		return nil
	}
	if _, ok := parsed["messages"].([]any); !ok {
		return nil
	}
	body := make(map[string]any, len(parsed)+2)
	for k, v := range parsed {
		body[k] = v
	}
	body["model"] = UpstreamModelID(rawModel)
	names := ShortenToolNames(body)

	spoken := ""
	if rawEffort, ok := body["reasoning_effort"].(string); ok {
		spoken = strings.ToLower(strings.TrimSpace(rawEffort))
	}
	levels := ReasoningLevelsOf(body["model"].(string))
	if spoken != "" {
		if noThinkingLevels[spoken] {
			delete(body, "reasoning_effort")
			if _, hasThinking := body["thinking"]; !hasThinking {
				body["thinking"] = map[string]any{"type": "disabled"}
			}
		} else if len(levels) == 0 || slices.Contains(levels, spoken) {
			body["reasoning_effort"] = spoken
		} else {
			delete(body, "reasoning_effort")
		}
	}
	_, hasThinking := body["thinking"]
	injected := thinking && spoken == "" && !hasThinking
	if injected {
		body["thinking"] = map[string]any{"type": "enabled"}
	}
	for _, key := range []string{"max_tokens", "max_completion_tokens"} {
		if rawNum, ok := asFloat(body[key]); ok && rawNum > MaxOutputTokens {
			body[key] = MaxOutputTokens
		}
	}
	NormalizeStop(body, stationID)
	streamBool, _ := body["stream"].(bool)
	fold := !streamBool
	body["stream"] = true
	return &PreparedChat{Body: body, Fold: fold, Injected: injected, Names: names}
}

func NormalizeStop(body map[string]any, stationID string) {
	rawStop, exists := body["stop"]
	if !exists {
		return
	}
	var kept []string
	switch typed := rawStop.(type) {
	case string:
		if typed != "" {
			kept = []string{typed}
		}
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok && s != "" {
				kept = append(kept, s)
			}
		}
	}
	if stationID == "international" && len(kept) > 0 {
		body["stop"] = kept
	} else {
		delete(body, "stop")
	}
}

func CutToolName(name string, salt int) string {
	head := truncateRunes(name, ToolNameKeep)
	input := name
	if salt != 0 {
		input = fmt.Sprintf("%s\x00%d", name, salt)
	}
	digest := sha256Hex([]byte(input))
	return head + "_" + digest[:8]
}

func truncateRunes(s string, maxUnits int) string {
	if len(s) <= maxUnits {
		return s
	}
	// Keep UTF-8 boundary valid and <= maxUnits UTF-16/rune length
	count := 0
	lastIdx := 0
	for idx, r := range s {
		units := 1
		if r > 0xFFFF {
			units = 2
		}
		if count+units > maxUnits {
			break
		}
		count += units
		lastIdx = idx + utf8.RuneLen(r)
	}
	return s[:lastIdx]
}

type toolSlot struct {
	obj map[string]any
	key string
}

func toolNameSlots(body map[string]any) []toolSlot {
	var slots []toolSlot
	add := func(obj map[string]any, key string) {
		if obj == nil {
			return
		}
		if _, ok := obj[key].(string); ok {
			slots = append(slots, toolSlot{obj: obj, key: key})
		}
	}
	if tools, ok := body["tools"].([]any); ok {
		for _, rawTool := range tools {
			if toolMap, ok := rawTool.(map[string]any); ok {
				if fn, ok := toolMap["function"].(map[string]any); ok {
					add(fn, "name")
				}
			}
		}
	}
	if fns, ok := body["functions"].([]any); ok {
		for _, rawFn := range fns {
			if fn, ok := rawFn.(map[string]any); ok {
				add(fn, "name")
			}
		}
	}
	if choice, ok := body["tool_choice"].(map[string]any); ok {
		if fn, ok := choice["function"].(map[string]any); ok {
			add(fn, "name")
		}
	}
	if fc, ok := body["function_call"].(map[string]any); ok {
		add(fc, "name")
	}
	if msgs, ok := body["messages"].([]any); ok {
		for _, rawMsg := range msgs {
			msg, ok := rawMsg.(map[string]any)
			if !ok {
				continue
			}
			if tcs, ok := msg["tool_calls"].([]any); ok {
				for _, rawTC := range tcs {
					if tc, ok := rawTC.(map[string]any); ok {
						if fn, ok := tc["function"].(map[string]any); ok {
							add(fn, "name")
						}
					}
				}
			}
			if fc, ok := msg["function_call"].(map[string]any); ok {
				add(fc, "name")
			}
			if role, _ := msg["role"].(string); role == "tool" || role == "function" {
				add(msg, "name")
			}
		}
	}
	return slots
}

func ShortenToolNames(body map[string]any) map[string]string {
	slots := toolNameSlots(body)
	hasLong := false
	for _, slot := range slots {
		if len(slot.obj[slot.key].(string)) > ToolNameLimit {
			hasLong = true
			break
		}
	}
	if !hasLong {
		return nil
	}
	taken := map[string]bool{}
	longSet := map[string]bool{}
	for _, slot := range slots {
		name := slot.obj[slot.key].(string)
		if len(name) <= ToolNameLimit {
			taken[name] = true
		} else {
			longSet[name] = true
		}
	}
	longNames := make([]string, 0, len(longSet))
	for name := range longSet {
		longNames = append(longNames, name)
	}
	slices.Sort(longNames)
	shortByOrig := map[string]string{}
	origByShort := map[string]string{}
	for _, name := range longNames {
		cut := CutToolName(name, 0)
		for salt := 1; taken[cut] || origByShort[cut] != ""; salt++ {
			cut = CutToolName(name, salt)
		}
		shortByOrig[name] = cut
		origByShort[cut] = name
	}
	for _, slot := range slots {
		name := slot.obj[slot.key].(string)
		if cut, ok := shortByOrig[name]; ok {
			slot.obj[slot.key] = cut
		}
	}
	return origByShort
}

func RestoreToolNames(completion map[string]any, names map[string]string) {
	if len(names) == 0 || completion == nil {
		return
	}
	choices, _ := completion["choices"].([]any)
	for _, rawChoice := range choices {
		choice, ok := rawChoice.(map[string]any)
		if !ok {
			continue
		}
		msg, ok := choice["message"].(map[string]any)
		if !ok {
			continue
		}
		if tcs, ok := msg["tool_calls"].([]any); ok {
			for _, rawTC := range tcs {
				if tc, ok := rawTC.(map[string]any); ok {
					if fn, ok := tc["function"].(map[string]any); ok {
						if n, ok := fn["name"].(string); ok && names[n] != "" {
							fn["name"] = names[n]
						}
					}
				}
			}
		}
		if fc, ok := msg["function_call"].(map[string]any); ok {
			if n, ok := fc["name"].(string); ok && names[n] != "" {
				fc["name"] = names[n]
			}
		}
	}
}

func RestoreToolNamesInSSELine(line string, names map[string]string) string {
	if len(names) == 0 {
		return line
	}
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "data:") {
		return line
	}
	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if payload == "" || payload == "[DONE]" {
		return line
	}
	var obj map[string]any
	if json.Unmarshal([]byte(payload), &obj) != nil || obj == nil {
		return line
	}
	changed := false
	choices, _ := obj["choices"].([]any)
	for _, rawChoice := range choices {
		choice, ok := rawChoice.(map[string]any)
		if !ok {
			continue
		}
		delta, ok := choice["delta"].(map[string]any)
		if !ok {
			continue
		}
		if tcs, ok := delta["tool_calls"].([]any); ok {
			for _, rawTC := range tcs {
				if tc, ok := rawTC.(map[string]any); ok {
					if fn, ok := tc["function"].(map[string]any); ok {
						if n, ok := fn["name"].(string); ok && names[n] != "" {
							fn["name"] = names[n]
							changed = true
						}
					}
				}
			}
		}
		if fc, ok := delta["function_call"].(map[string]any); ok {
			if n, ok := fc["name"].(string); ok && names[n] != "" {
				fc["name"] = names[n]
				changed = true
			}
		}
	}
	if !changed {
		return line
	}
	rewritten, err := json.Marshal(obj)
	if err != nil {
		return line
	}
	return "data: " + string(rewritten)
}

func ParseSSEEvents(text string) []map[string]any {
	var events []map[string]any
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var parsed map[string]any
		if json.Unmarshal([]byte(payload), &parsed) == nil && parsed != nil {
			events = append(events, parsed)
		}
	}
	return events
}

func ErrorCodeOf(text string) string {
	m := errorCodePattern.FindStringSubmatch(text)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func ErrorMessageOf(text string) string {
	m := errorMsgPattern.FindStringSubmatch(text)
	if len(m) >= 2 {
		var unescaped string
		if json.Unmarshal([]byte(`"`+m[1]+`"`), &unescaped) == nil {
			return unescaped
		}
		return m[1]
	}
	return ""
}

func IsBodyTooLarge(text string) bool      { return bodyTooLargePattern.MatchString(text) }
func IsStaleCredential(text string) bool   { return staleCredPattern.MatchString(text) }
func IsRateLimitCode(code string) bool     { return rateLimitPattern.MatchString(code) }
func IsNotMultimodal(text string) bool     { return notMultimodalPat.MatchString(text) }
func IsNoBenefitChannelCode(t string) bool { return noBenefitChanPat.MatchString(t) }
func IsNoModelBenefitCode(t string) bool   { return noModelBenefitPat.MatchString(t) }
func IsOABasicRejection(st int, text string) bool {
	if st == http.StatusUnauthorized || st == http.StatusForbidden {
		return true
	}
	return st >= 400 && st < 500 && oaBasicRejectPat.MatchString(text)
}

func EmbeddedErrorStatus(code string) int {
	if IsRateLimitCode(code) {
		return http.StatusTooManyRequests
	}
	if clientErrPattern.MatchString(code) || IsNotMultimodal(code) {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

func eventHasContent(event map[string]any) bool {
	choices, _ := event["choices"].([]any)
	if len(choices) == 0 {
		return false
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return false
	}
	if choice["finish_reason"] != nil || choice["message"] != nil {
		return true
	}
	delta, _ := choice["delta"].(map[string]any)
	if delta == nil {
		return false
	}
	if s, _ := delta["content"].(string); s != "" {
		return true
	}
	if s, _ := delta["reasoning_content"].(string); s != "" {
		return true
	}
	if tcs, ok := delta["tool_calls"].([]any); ok && len(tcs) > 0 {
		return true
	}
	return false
}

func ClassifyHead(text string, done bool) string {
	for _, ev := range ParseSSEEvents(text) {
		if eventHasContent(ev) {
			return "answer"
		}
	}
	if done {
		var doc map[string]any
		if json.Unmarshal([]byte(text), &doc) == nil {
			if _, ok := doc["choices"].([]any); ok {
				return "answer"
			}
		}
	}
	if code := ErrorCodeOf(text); code != "" {
		idx := strings.Index(text, code)
		if done || (idx >= 0 && strings.Contains(text[idx:], "\n")) {
			return code
		}
		return ""
	}
	if done || doneLinePattern.MatchString(text) {
		return "answer"
	}
	return ""
}

type PeekResult struct {
	Code   string
	Head   []byte
	EOF    bool
	Reader io.ReadCloser
}

func PeekAnswer(body io.ReadCloser) (PeekResult, error) {
	res := PeekResult{Reader: body}
	buf := make([]byte, 4096)
	for len(res.Head) < SSEPeekLimit {
		n, err := body.Read(buf)
		if n > 0 {
			res.Head = append(res.Head, buf[:n]...)
		}
		eof := err == io.EOF
		if err != nil && !eof {
			return res, err
		}
		verdict := ClassifyHead(string(res.Head), eof)
		if verdict == "answer" {
			res.EOF = eof
			return res, nil
		}
		if verdict != "" {
			res.Code = verdict
			res.EOF = eof
			return res, nil
		}
		if eof {
			res.EOF = true
			return res, nil
		}
	}
	return res, nil
}

func CompletionFromChunks(events []map[string]any, names map[string]string) map[string]any {
	var (
		id           string
		model        string
		created      any
		role         = "assistant"
		content      strings.Builder
		reasoning    strings.Builder
		finishReason any
		usage        any
	)
	type tcAccum struct {
		id   string
		typ  string
		name strings.Builder
		args strings.Builder
	}
	toolCalls := map[int]*tcAccum{}
	var tcOrder []int
	for _, ev := range events {
		if s, _ := ev["id"].(string); s != "" && id == "" {
			id = s
		}
		if s, _ := ev["model"].(string); s != "" && model == "" {
			model = s
		}
		if ev["created"] != nil && created == nil {
			created = ev["created"]
		}
		if ev["usage"] != nil {
			usage = ev["usage"]
		}
		choices, _ := ev["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]any)
		if choice == nil {
			continue
		}
		if choice["finish_reason"] != nil {
			finishReason = choice["finish_reason"]
		}
		delta, _ := choice["delta"].(map[string]any)
		if delta == nil {
			continue
		}
		if r, _ := delta["role"].(string); r != "" {
			role = r
		}
		if c, _ := delta["content"].(string); c != "" {
			content.WriteString(c)
		}
		if rc, _ := delta["reasoning_content"].(string); rc != "" {
			reasoning.WriteString(rc)
		}
		if tcs, ok := delta["tool_calls"].([]any); ok {
			for pos, rawTC := range tcs {
				tc, ok := rawTC.(map[string]any)
				if !ok {
					continue
				}
				idx := pos
				if f, ok := asFloat(tc["index"]); ok {
					idx = int(f)
				}
				acc, exists := toolCalls[idx]
				if !exists {
					acc = &tcAccum{typ: "function"}
					toolCalls[idx] = acc
					tcOrder = append(tcOrder, idx)
				}
				if s, _ := tc["id"].(string); s != "" {
					acc.id = s
				}
				if s, _ := tc["type"].(string); s != "" {
					acc.typ = s
				}
				if fn, ok := tc["function"].(map[string]any); ok {
					if n, _ := fn["name"].(string); n != "" {
						acc.name.WriteString(n)
					}
					if a, _ := fn["arguments"].(string); a != "" {
						acc.args.WriteString(a)
					}
				}
			}
		}
	}
	msg := map[string]any{
		"role":    role,
		"content": content.String(),
	}
	if reasoning.Len() > 0 {
		msg["reasoning_content"] = reasoning.String()
	}
	if len(tcOrder) > 0 {
		slices.Sort(tcOrder)
		outTCs := make([]any, 0, len(tcOrder))
		for _, idx := range tcOrder {
			acc := toolCalls[idx]
			outTCs = append(outTCs, map[string]any{
				"id":   acc.id,
				"type": acc.typ,
				"function": map[string]any{
					"name":      acc.name.String(),
					"arguments": acc.args.String(),
				},
			})
		}
		msg["tool_calls"] = outTCs
	}
	if finishReason == nil {
		if len(tcOrder) > 0 {
			finishReason = "tool_calls"
		} else {
			finishReason = "stop"
		}
	}
	out := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": created,
		"model":   model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"message":       msg,
				"finish_reason": finishReason,
			},
		},
	}
	if usage != nil {
		out["usage"] = usage
	}
	RestoreToolNames(out, names)
	return out
}

func extractChatUsage(raw []byte) *sdk.Usage {
	var doc struct {
		Usage *struct {
			PromptTokens        int64 `json:"prompt_tokens"`
			CompletionTokens    int64 `json:"completion_tokens"`
			CachedTokens        int64 `json:"cached_tokens"`
			PromptTokensDetails *struct {
				CachedTokens int64 `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &doc) != nil || doc.Usage == nil {
		return nil
	}
	cached := doc.Usage.CachedTokens
	if cached == 0 && doc.Usage.PromptTokensDetails != nil {
		cached = doc.Usage.PromptTokensDetails.CachedTokens
	}
	return &sdk.Usage{
		InputTokens:  doc.Usage.PromptTokens,
		OutputTokens: doc.Usage.CompletionTokens,
		CachedTokens: cached,
	}
}

func extractSSEUsage(text string) *sdk.Usage {
	var latest *sdk.Usage
	for _, ev := range ParseSSEEvents(text) {
		raw, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		if u := extractChatUsage(raw); u != nil {
			latest = u
		}
	}
	return latest
}

func asFloat(v any) (float64, bool) {
	switch typed := v.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		f, err := typed.Float64()
		return f, err == nil
	case string:
		var f float64
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%f", &f); err == nil {
			return f, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func officeAceStripPrefix(raw []byte) []byte {
	var parsed map[string]any
	if json.Unmarshal(raw, &parsed) != nil || parsed == nil {
		return raw
	}
	if m, ok := parsed["model"].(string); ok && strings.HasPrefix(m, "officeace/") {
		parsed["model"] = strings.TrimPrefix(m, "officeace/")
		if encoded, err := json.Marshal(parsed); err == nil {
			return encoded
		}
	}
	return raw
}

func encodeJSONNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
