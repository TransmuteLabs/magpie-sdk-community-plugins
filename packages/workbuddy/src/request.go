package workbuddy

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

var (
	flaggedBillingRe    = regexp.MustCompile(`(?i)x-anthropic-billing-header:[^\n]*\n?`)
	flaggedClaudeCodeRe = regexp.MustCompile(`(?i)You are Claude Code, Anthropic['’]s official CLI for Claude`)
	flaggedMainBranchRe = regexp.MustCompile(`(?i)Main branch \(you will usually use this for PRs\)`)
	flaggedCodexRe      = regexp.MustCompile(`(?i)Codex CLI is an open source project led by OpenAI\.`)
	flaggedDSHOpenerRe  = regexp.MustCompile(`(?i)You are an AI agent powered by DeepSeek Harness\.?`)
	flaggedDSHWebGUIRe  = regexp.MustCompile(`(?i)Web GUI whose URL the runtime context names\.\s+When the user refers`)

	openerCommaRe  = regexp.MustCompile(`(?i)(\bYou are (?:an?|the)\s[^\n.,]{0,60}?\b(?:agent|assistant))\s+(powered by|running on|running in|built on|based on)\b`)
	callTextRe     = regexp.MustCompile(`(?i)^kimi-k3\.1-pro(?:-|$)`)
	safeNameOKRe   = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	safeNameBadRe  = regexp.MustCompile(`[^A-Za-z0-9_-]`)
	messagesPathRe = regexp.MustCompile(`/messages(/count_tokens)?$`)
	chatPathRe     = regexp.MustCompile(`/chat/completions$`)

	dshLineOpenerRe   = regexp.MustCompile(`You are an AI agent,? powered by DeepSeek Harness\.?`)
	dshLineWebGUIRe   = regexp.MustCompile(`DeepSeek Harness (Web GUI)`)
	dshLineCheckoutRe = regexp.MustCompile(`DeepSeek Harness (implementation checkout)`)
	dshLineItselfRe   = regexp.MustCompile(`\bDSH itself\b`)

	refusedRe       = regexp.MustCompile(`(?i)unapproved channel|illegal api invocation`)
	badSignatureRe  = regexp.MustCompile(`(?i)thinking_signature_invalid|invalid\s+` + "`?" + `signature` + "`?" + `\s+in\s+` + "`?" + `thinking|block is bound to a different conversation|"internal_code"\s*:\s*"?11159`)
	restrictedMsgRe = regexp.MustCompile(`(?i)account has been restricted due to unusual activity|账号存在异常行为|帳號存在異常行為`)
	exhaustedMsgRe  = regexp.MustCompile(`(?i)credits exhausted|额度已用尽`)
	afterToolsMsgRe = regexp.MustCompile(`(?i)must end with a user message`)
	jsonCodeFieldRe = regexp.MustCompile(`(?i)["']code["']\s*:\s*["']?(\d+)`)
	stringErrCodeRe = regexp.MustCompile(`^(\d{5}):`)

	finishReasonRe = regexp.MustCompile(`"finish_reason"\s*:\s*"([^"]+)"`)
	promptTokRe    = regexp.MustCompile(`"prompt_tokens"\s*:\s*(\d+)`)
	compTokRe      = regexp.MustCompile(`"completion_tokens"\s*:\s*(\d+)`)
	totalTokRe     = regexp.MustCompile(`"total_tokens"\s*:\s*(\d+)`)
	cachedTokRe    = regexp.MustCompile(`"cached_tokens"\s*:\s*(\d+)`)
	sseDataLineRe  = regexp.MustCompile(`^data:\s?(.*)$`)

	filterSays = []string{"Sorry, I can't respond to this question."}

	schemaTypes = map[string]bool{
		"string": true, "number": true, "integer": true, "boolean": true,
		"object": true, "array": true, "null": true,
	}
	subOneKeys  = []string{"items", "additionalItems", "unevaluatedItems", "contains", "not", "if", "then", "else", "propertyNames", "additionalProperties", "unevaluatedProperties"}
	subListKeys = []string{"anyOf", "oneOf", "allOf", "prefixItems"}
	subMapKeys  = []string{"properties", "patternProperties", "$defs", "definitions", "dependentSchemas"}
	boolOKKeys  = map[string]bool{"additionalProperties": true, "unevaluatedProperties": true}
)

func hasExactCodeInText(text string, want string) bool {
	matches := jsonCodeFieldRe.FindAllStringSubmatchIndex(text, -1)
	for _, loc := range matches {
		if len(loc) < 4 {
			continue
		}
		codeText := text[loc[2]:loc[3]]
		if codeText != want {
			continue
		}
		endIdx := loc[3]
		if endIdx < len(text) {
			next := text[endIdx]
			if (next >= '0' && next <= '9') || next == '.' {
				continue
			}
		}
		return true
	}
	return false
}

func isAfterToolsRefused(text string) bool {
	return afterToolsMsgRe.MatchString(text) || hasExactCodeInText(text, "11153")
}

func isRestrictedText(text string) bool {
	return restrictedMsgRe.MatchString(text) || hasExactCodeInText(text, "11140")
}

func isExhaustedText(text string) bool {
	return exhaustedMsgRe.MatchString(text) || hasExactCodeInText(text, "14018")
}

func (owner *module) idFor(kind string, pieces []string) string {
	hasher := sha256.New()
	hasher.Write([]byte(owner.salt))
	hasher.Write([]byte("|"))
	hasher.Write([]byte(kind))
	hasher.Write([]byte("|"))
	for _, piece := range pieces {
		hasher.Write([]byte(piece))
	}
	return hex.EncodeToString(hasher.Sum(nil))[:32]
}

func signatureParts(content any) []string {
	if str, ok := content.(string); ok {
		if str != "" {
			return []string{str}
		}
		return nil
	}
	arr, ok := content.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, rawPart := range arr {
		part, ok := rawPart.(map[string]any)
		if !ok {
			continue
		}
		partType := strField(part, "type")
		if partType == "" || partType == "text" {
			if text, isStr := part["text"].(string); isStr && text != "" {
				out = append(out, text)
			}
		} else if partType != "tool_result" && partType != "tool-result" {
			encoded, _ := json.Marshal(part)
			sum := sha256.Sum256(encoded)
			out = append(out, fmt.Sprintf("[%s:%s]", partType, hex.EncodeToString(sum[:])[:8]))
		}
	}
	return out
}

func signatureOf(content any) string {
	return strings.Join(signatureParts(content), "")
}

func hasBlock(msg map[string]any, blockType string) bool {
	arr, ok := msg["content"].([]any)
	if !ok {
		return false
	}
	for _, rawPart := range arr {
		part, ok := rawPart.(map[string]any)
		if ok && strField(part, "type") == blockType {
			return true
		}
	}
	return false
}

func answeredBefore(msgs []any, idx int) bool {
	cur, _ := msgs[idx].(map[string]any)
	if hasBlock(cur, "tool_result") {
		return true
	}
	prevIdx := idx - 1
	for prevIdx >= 0 {
		prevObj, _ := msgs[prevIdx].(map[string]any)
		if strField(prevObj, "role") != "system" {
			break
		}
		prevIdx--
	}
	if prevIdx < 0 {
		return false
	}
	prev, ok := msgs[prevIdx].(map[string]any)
	if !ok || prev == nil {
		return false
	}
	role := strField(prev, "role")
	if role == "tool" {
		return true
	}
	if role == "user" && hasBlock(prev, "tool_result") {
		return true
	}
	if role != "assistant" {
		return false
	}
	if tcs, ok := prev["tool_calls"].([]any); ok && len(tcs) > 0 {
		return true
	}
	return hasBlock(prev, "tool_use")
}

func turnParts(chat map[string]any) []string {
	msgs, _ := chat["messages"].([]any)
	for idx := len(msgs) - 1; idx >= 0; idx-- {
		msg, ok := msgs[idx].(map[string]any)
		if !ok || strField(msg, "role") != "user" || answeredBefore(msgs, idx) {
			continue
		}
		sig := signatureParts(msg["content"])
		if len(sig) == 0 {
			return nil
		}
		return append([]string{fmt.Sprintf("u%d:", idx)}, sig...)
	}
	return nil
}

func conversationParts(chat map[string]any, session string) []string {
	if session != "" {
		return []string{"session:" + session}
	}
	msgs, _ := chat["messages"].([]any)
	for _, rawMsg := range msgs {
		msg, ok := rawMsg.(map[string]any)
		if !ok || strField(msg, "role") != "user" {
			continue
		}
		sig := signatureParts(msg["content"])
		if len(sig) > 0 {
			return append([]string{"first:"}, sig...)
		}
	}
	return nil
}

type attendIDs struct {
	set  []string
	turn string
	conv string
}

func scopeOf(acct Credential) string {
	keyPart := ""
	if acct.Source == "key" {
		keyPart = acct.Key
	}
	return acct.UID + "|" + acct.EnterpriseID + "|" + keyPart
}

func (owner *module) setIDs(headers http.Header, ids attendIDs, who string) attendIDs {
	msgID := randomHex(16)
	headers.Set("X-Conversation-Message-ID", msgID)
	headers.Set("X-Request-ID", msgID)
	var set []string
	own := func(kind, base string) string {
		if who == "" {
			return base
		}
		return owner.idFor(kind, []string{"account:" + who, "|", base})
	}
	if headers.Get("X-Conversation-Request-ID") == "" {
		if ids.turn != "" {
			headers.Set("X-Conversation-Request-ID", own("turn", ids.turn))
		} else {
			headers.Set("X-Conversation-Request-ID", randomHex(16))
		}
		set = append(set, "X-Conversation-Request-ID")
	}
	if ids.conv != "" && headers.Get("X-Conversation-ID") == "" {
		headers.Set("X-Conversation-ID", own("conversation", ids.conv))
		set = append(set, "X-Conversation-ID")
	}
	ids.set = set
	return ids
}

func (owner *module) attendChat(headers http.Header, chat map[string]any, session, who string) attendIDs {
	conv := conversationParts(chat, session)
	turn := turnParts(chat)
	ids := attendIDs{}
	if len(turn) > 0 {
		combined := make([]string, 0, len(conv)+1+len(turn))
		combined = append(combined, conv...)
		combined = append(combined, "|")
		combined = append(combined, turn...)
		ids.turn = owner.idFor("turn", combined)
	}
	if len(conv) > 0 {
		ids.conv = owner.idFor("conversation", conv)
	}
	return owner.setIDs(headers, ids, who)
}

func (owner *module) reattend(headers http.Header, ids attendIDs, prevWho, nextWho string) attendIDs {
	if prevWho == nextWho {
		return ids
	}
	for _, headerName := range ids.set {
		headers.Del(headerName)
	}
	return owner.setIDs(headers, ids, nextWho)
}

func sessionOf(headers http.Header, fallbackSession string) string {
	sess := strings.TrimSpace(headers.Get(SessionHeader))
	headers.Del(SessionHeader)
	if sess == "" {
		sess = strings.TrimSpace(fallbackSession)
	}
	if len(sess) > 128 {
		sess = sess[:128]
	}
	return sess
}

func isMessagesRequest(protocol, rawURL string, chat map[string]any) bool {
	if protocol == "messages" || protocol == "anthropic" {
		return true
	}
	if rawURL != "" {
		if parsed, err := url.Parse(rawURL); err == nil && parsed.Path != "" {
			return messagesPathRe.MatchString(parsed.Path)
		}
	}
	if chat == nil {
		return false
	}
	if _, isArr := chat["system"].([]any); isArr {
		return true
	}
	if tools, ok := chat["tools"].([]any); ok {
		for _, rawTool := range tools {
			tool, ok := rawTool.(map[string]any)
			if !ok {
				continue
			}
			_, hasInputSchema := tool["input_schema"]
			_, hasFn := tool["function"]
			if hasInputSchema && !hasFn {
				return true
			}
		}
	}
	return false
}

func isCompletionRequest(rawURL string, chat map[string]any) bool {
	if rawURL != "" {
		if parsed, err := url.Parse(rawURL); err == nil && parsed.Path != "" {
			return chatPathRe.MatchString(parsed.Path)
		}
	}
	if chat == nil {
		return false
	}
	_, hasMsgs := chat["messages"].([]any)
	return hasMsgs
}

func unsignedOn(chat map[string]any) bool {
	msgs, ok := chat["messages"].([]any)
	if !ok {
		return false
	}
	changed := false
	for _, rawMsg := range msgs {
		msg, ok := rawMsg.(map[string]any)
		if !ok || strField(msg, "role") != "assistant" {
			continue
		}
		parts, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		var kept []any
		for _, rawPart := range parts {
			part, isMap := rawPart.(map[string]any)
			if isMap && strField(part, "type") == "thinking" && strField(part, "signature") == "" {
				continue
			}
			kept = append(kept, rawPart)
		}
		if len(kept) == len(parts) {
			continue
		}
		changed = true
		if len(kept) == 0 {
			msg["content"] = []any{map[string]any{"type": "text", "text": ""}}
		} else {
			msg["content"] = kept
		}
	}
	return changed
}

func withoutSignedThinking(body []byte) ([]byte, bool) {
	var chat map[string]any
	if json.Unmarshal(body, &chat) != nil || chat == nil {
		return nil, false
	}
	msgs, ok := chat["messages"].([]any)
	if !ok {
		return nil, false
	}
	dropped := 0
	for _, rawMsg := range msgs {
		msg, ok := rawMsg.(map[string]any)
		if !ok || strField(msg, "role") != "assistant" {
			continue
		}
		parts, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		var kept []any
		for _, rawPart := range parts {
			part, isMap := rawPart.(map[string]any)
			if isMap && strField(part, "type") == "thinking" {
				dropped++
				continue
			}
			kept = append(kept, rawPart)
		}
		if len(kept) == len(parts) {
			continue
		}
		if len(kept) == 0 {
			msg["content"] = []any{map[string]any{"type": "text", "text": ""}}
		} else {
			msg["content"] = kept
		}
	}
	if dropped == 0 {
		return nil, false
	}
	encoded, err := json.Marshal(chat)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

func replaceCodexCLI(text string) string {
	locs := flaggedCodexRe.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return text
	}
	var builder strings.Builder
	cursor := 0
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		if start >= 4 && strings.EqualFold(text[start-4:start], "the ") {
			continue
		}
		builder.WriteString(text[cursor:start])
		builder.WriteString("The Codex CLI is an open source project led by OpenAI.")
		cursor = end
	}
	if cursor == 0 {
		return text
	}
	builder.WriteString(text[cursor:])
	return builder.String()
}

func unflagged(text string) string {
	text = flaggedBillingRe.ReplaceAllString(text, "")
	text = flaggedClaudeCodeRe.ReplaceAllString(text, "You are Claude Code, Anthropic's CLI for Claude")
	text = flaggedMainBranchRe.ReplaceAllString(text, "Main branch (usually the base for PRs)")
	text = replaceCodexCLI(text)
	text = flaggedDSHOpenerRe.ReplaceAllString(text, "You are an AI agent, powered by DeepSeek Harness.")
	text = flaggedDSHWebGUIRe.ReplaceAllString(text, "Web GUI whose URL the runtime context names. The interface, when the user refers")
	return text
}

func unflagMessages(chat map[string]any) bool {
	msgs, ok := chat["messages"].([]any)
	if !ok {
		return false
	}
	changed := false
	for idx := len(msgs) - 1; idx >= 0; idx-- {
		msg, ok := msgs[idx].(map[string]any)
		if !ok {
			continue
		}
		role := strField(msg, "role")
		if contentStr, isStr := msg["content"].(string); isStr {
			next := unflagged(contentStr)
			if next == contentStr {
				continue
			}
			changed = true
			if strings.TrimSpace(next) != "" || role != "system" {
				msg["content"] = next
			} else {
				msgs = append(msgs[:idx], msgs[idx+1:]...)
			}
		} else if parts, isArr := msg["content"].([]any); isArr {
			var nextParts []any
			localChanged := false
			for _, rawPart := range parts {
				part, isMap := rawPart.(map[string]any)
				if !isMap {
					nextParts = append(nextParts, rawPart)
					continue
				}
				partText, isText := part["text"].(string)
				if !isText {
					nextParts = append(nextParts, rawPart)
					continue
				}
				next := unflagged(partText)
				if next == partText {
					nextParts = append(nextParts, rawPart)
					continue
				}
				localChanged = true
				if strings.TrimSpace(next) != "" {
					cloned := cloneMap(part)
					cloned["text"] = next
					nextParts = append(nextParts, cloned)
				}
			}
			if !localChanged {
				continue
			}
			changed = true
			if len(nextParts) > 0 || role != "system" {
				msg["content"] = nextParts
			} else {
				msgs = append(msgs[:idx], msgs[idx+1:]...)
			}
		}
	}
	if changed {
		chat["messages"] = msgs
	}
	return changed
}

func reasoningOf(msg map[string]any) string {
	if rc, ok := msg["reasoning_content"].(string); ok && rc != "" {
		return rc
	}
	if details, ok := msg["reasoning_details"].([]any); ok {
		var builder strings.Builder
		for _, rawDetail := range details {
			detail, ok := rawDetail.(map[string]any)
			if !ok {
				continue
			}
			if text, isStr := detail["text"].(string); isStr {
				builder.WriteString(text)
			} else if summary, isStr := detail["summary"].(string); isStr {
				builder.WriteString(summary)
			}
		}
		return builder.String()
	}
	return ""
}

func asReasoning(msgs []any) bool {
	changed := false
	for _, rawMsg := range msgs {
		msg, ok := rawMsg.(map[string]any)
		if !ok || strField(msg, "role") != "assistant" {
			continue
		}
		_, hasRC := msg["reasoning_content"]
		_, hasRD := msg["reasoning_details"]
		if !hasRC && !hasRD {
			continue
		}
		text := reasoningOf(msg)
		existing, _ := msg["reasoning"].(string)
		if text != "" && existing == "" {
			msg["reasoning"] = text
		}
		delete(msg, "reasoning_content")
		delete(msg, "reasoning_details")
		changed = true
	}
	return changed
}

func callTextModel(modelID string) bool {
	return callTextRe.MatchString(modelID)
}

func emptyContent(content any) bool {
	if content == nil {
		return true
	}
	if str, ok := content.(string); ok {
		return str == ""
	}
	arr, ok := content.([]any)
	if !ok {
		return false
	}
	for _, rawPart := range arr {
		part, isMap := rawPart.(map[string]any)
		if !isMap || strField(part, "type") != "text" {
			return false
		}
		if text, _ := part["text"].(string); text != "" {
			return false
		}
	}
	return true
}

func callText(chat map[string]any) bool {
	if !callTextModel(strField(chat, "model")) {
		return false
	}
	msgs, ok := chat["messages"].([]any)
	if !ok {
		return false
	}
	changed := false
	for _, rawMsg := range msgs {
		msg, ok := rawMsg.(map[string]any)
		if !ok || strField(msg, "role") != "assistant" {
			continue
		}
		tcs, hasTC := msg["tool_calls"].([]any)
		if !hasTC || len(tcs) == 0 || !emptyContent(msg["content"]) {
			continue
		}
		msg["content"] = CallTextSpace
		changed = true
	}
	return changed
}

func systemOn(chat map[string]any) bool {
	msgs, ok := chat["messages"].([]any)
	if !ok || len(msgs) == 0 {
		return false
	}
	changed := unflagMessages(chat)
	msgs, _ = chat["messages"].([]any)
	if maxComp, hasMaxComp := chat["max_completion_tokens"]; hasMaxComp {
		if _, hasMax := chat["max_tokens"]; !hasMax {
			chat["max_tokens"] = maxComp
		}
		delete(chat, "max_completion_tokens")
		changed = true
	}
	for _, rawMsg := range msgs {
		msg, ok := rawMsg.(map[string]any)
		if ok && strField(msg, "role") == "developer" {
			msg["role"] = "system"
			changed = true
		}
	}
	if asReasoning(msgs) {
		changed = true
	}
	if callText(chat) {
		changed = true
	}
	var firstRole string
	if len(msgs) > 0 {
		if firstMsg, ok := msgs[0].(map[string]any); ok {
			firstRole = strField(firstMsg, "role")
		}
	}
	if firstRole != "system" {
		chat["messages"] = append([]any{map[string]any{"role": "system", "content": DefaultSystem}}, msgs...)
		changed = true
	}
	return changed
}

func textOfContent(content any) string {
	if str, ok := content.(string); ok {
		return str
	}
	arr, ok := content.([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, rawPart := range arr {
		part, isMap := rawPart.(map[string]any)
		if !isMap {
			continue
		}
		if text, isStr := part["text"].(string); isStr && text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func systemKeyOf(chat map[string]any) string {
	msgs, ok := chat["messages"].([]any)
	if !ok {
		return ""
	}
	var lines []string
	for _, rawMsg := range msgs {
		msg, isMap := rawMsg.(map[string]any)
		if isMap && strField(msg, "role") == "system" {
			lines = append(lines, textOfContent(msg["content"]))
		}
	}
	key := strings.Join(lines, "\n")
	if strings.TrimSpace(key) == DefaultSystem {
		return ""
	}
	return key
}

func systemAsUserOn(chat map[string]any) bool {
	msgs, ok := chat["messages"].([]any)
	if !ok {
		return false
	}
	idx := 0
	var lead []string
	for idx < len(msgs) {
		msg, isMap := msgs[idx].(map[string]any)
		if !isMap || strField(msg, "role") != "system" {
			break
		}
		lead = append(lead, textOfContent(msg["content"]))
		idx++
	}
	said := strings.TrimSpace(strings.Join(lead, "\n\n"))
	rest := make([]any, 0, len(msgs)-idx)
	convertedLater := false
	for remIdx := idx; remIdx < len(msgs); remIdx++ {
		msg, isMap := msgs[remIdx].(map[string]any)
		if isMap && strField(msg, "role") == "system" {
			cloned := cloneMap(msg)
			cloned["role"] = "user"
			rest = append(rest, cloned)
			convertedLater = true
		} else {
			rest = append(rest, msgs[remIdx])
		}
	}
	movedLead := said != "" && said != DefaultSystem
	if !movedLead && !convertedLater {
		return false
	}
	if movedLead {
		wrapped := "<instructions>\n" + said + "\n</instructions>"
		userIdx := -1
		for rIdx, rawMsg := range rest {
			msg, isMap := rawMsg.(map[string]any)
			if isMap && strField(msg, "role") == "user" {
				userIdx = rIdx
				break
			}
		}
		if userIdx < 0 {
			rest = append([]any{map[string]any{"role": "user", "content": wrapped}}, rest...)
		} else {
			userMsg := cloneMap(rest[userIdx].(map[string]any))
			switch existing := userMsg["content"].(type) {
			case string:
				userMsg["content"] = wrapped + "\n\n" + existing
			case []any:
				userMsg["content"] = append([]any{map[string]any{"type": "text", "text": wrapped}}, existing...)
			default:
				userMsg["content"] = wrapped
			}
			rest[userIdx] = userMsg
		}
	}
	chat["messages"] = append([]any{map[string]any{"role": "system", "content": DefaultSystem}}, rest...)
	return true
}

func systemAsUser(body []byte) ([]byte, bool) {
	var chat map[string]any
	if json.Unmarshal(body, &chat) != nil || chat == nil {
		return nil, false
	}
	if !systemAsUserOn(chat) {
		return nil, false
	}
	encoded, err := json.Marshal(chat)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

func openedComma(text string) string {
	return openerCommaRe.ReplaceAllString(text, "$1, $2")
}

func commaPausedOn(chat map[string]any, roles map[string]bool) bool {
	msgs, ok := chat["messages"].([]any)
	if !ok {
		return false
	}
	changed := false
	for _, rawMsg := range msgs {
		msg, isMap := rawMsg.(map[string]any)
		if !isMap {
			continue
		}
		role := strField(msg, "role")
		if role != "system" && role != "assistant" {
			continue
		}
		localChanged := false
		if contentStr, isStr := msg["content"].(string); isStr {
			next := openedComma(contentStr)
			if next != contentStr {
				msg["content"] = next
				localChanged = true
			}
		} else if parts, isArr := msg["content"].([]any); isArr {
			for _, rawPart := range parts {
				part, isPartMap := rawPart.(map[string]any)
				if !isPartMap {
					continue
				}
				partText, isText := part["text"].(string)
				if !isText {
					continue
				}
				next := openedComma(partText)
				if next != partText {
					part["text"] = next
					localChanged = true
				}
			}
		}
		if !localChanged {
			continue
		}
		changed = true
		if roles != nil {
			roles[role] = true
		}
	}
	return changed
}

func commaPaused(body []byte, roles map[string]bool) ([]byte, bool) {
	var chat map[string]any
	if json.Unmarshal(body, &chat) != nil || chat == nil {
		return nil, false
	}
	if !commaPausedOn(chat, roles) {
		return nil, false
	}
	encoded, err := json.Marshal(chat)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

func replyAfterToolsOn(chat map[string]any) bool {
	msgs, ok := chat["messages"].([]any)
	if !ok {
		return false
	}
	idx := len(msgs) - 1
	for idx >= 0 {
		msg, isMap := msgs[idx].(map[string]any)
		if !isMap || strField(msg, "role") != "user" {
			break
		}
		idx--
	}
	if idx == len(msgs)-1 || idx < 0 {
		return false
	}
	msg, isMap := msgs[idx].(map[string]any)
	if !isMap || strField(msg, "role") != "tool" {
		return false
	}
	next := make([]any, 0, len(msgs)+1)
	next = append(next, msgs[:idx+1]...)
	next = append(next, map[string]any{"role": "assistant", "content": ""})
	next = append(next, msgs[idx+1:]...)
	chat["messages"] = next
	return true
}

func replyAfterTools(body []byte) ([]byte, bool) {
	var chat map[string]any
	if json.Unmarshal(body, &chat) != nil || chat == nil {
		return nil, false
	}
	if !replyAfterToolsOn(chat) {
		return nil, false
	}
	encoded, err := json.Marshal(chat)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

func isASCIIWord(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

func isPathBeforeChar(ch byte) bool {
	return isASCIIWord(ch) || ch == '/' || ch == '\\' || ch == '.' || ch == '~' || ch == ':' || ch == '@' || ch == '-'
}

func isPathAfterAt(text string, pos int) bool {
	if pos >= len(text) {
		return false
	}
	ch := text[pos]
	if isASCIIWord(ch) || ch == '/' || ch == '\\' || ch == '@' || ch == '-' {
		return true
	}
	if ch == '.' && pos+1 < len(text) && isASCIIWord(text[pos+1]) {
		return true
	}
	return false
}

func replaceDSHProseToken(text, target string, allowThe bool) string {
	var builder strings.Builder
	cursor := 0
	for cursor < len(text) {
		idx := strings.Index(text[cursor:], target)
		if idx < 0 {
			break
		}
		matchStart := cursor + idx
		matchEnd := matchStart + len(target)
		actualStart := matchStart
		thePrefix := ""
		if allowThe && matchStart >= 4 {
			candidate := text[matchStart-4 : matchStart]
			if candidate == "The " || candidate == "the " {
				if matchStart-4 == 0 || !isPathBeforeChar(text[matchStart-5]) {
					actualStart = matchStart - 4
					thePrefix = candidate
				}
			}
		}
		beforeBlocked := actualStart > 0 && isPathBeforeChar(text[actualStart-1])
		afterBlocked := isPathAfterAt(text, matchEnd)
		if beforeBlocked || afterBlocked {
			builder.WriteString(text[cursor:matchEnd])
			cursor = matchEnd
			continue
		}
		builder.WriteString(text[cursor:actualStart])
		if thePrefix != "" {
			builder.WriteString(thePrefix)
		} else {
			builder.WriteString("the ")
		}
		builder.WriteString("agent harness")
		cursor = matchEnd
	}
	if cursor == 0 {
		return text
	}
	builder.WriteString(text[cursor:])
	return builder.String()
}

func dshHidden(text string, said bool) string {
	text = dshLineOpenerRe.ReplaceAllString(text, "You are an AI agent.")
	text = dshLineWebGUIRe.ReplaceAllString(text, "$1")
	text = dshLineCheckoutRe.ReplaceAllString(text, "agent $1")
	text = dshLineItselfRe.ReplaceAllString(text, "the agent itself")
	if !said {
		text = replaceDSHProseToken(text, "DeepSeek Harness", true)
		text = replaceDSHProseToken(text, "DSH", false)
	}
	return text
}

func hideDSH(chat map[string]any) bool {
	if chat == nil {
		return false
	}
	changed := false
	rewriteContent := func(content any, said bool) any {
		if contentStr, isStr := content.(string); isStr {
			next := dshHidden(contentStr, said)
			if next != contentStr {
				changed = true
			}
			return next
		}
		parts, isArr := content.([]any)
		if !isArr {
			return content
		}
		out := make([]any, len(parts))
		for idx, rawPart := range parts {
			part, isMap := rawPart.(map[string]any)
			if !isMap {
				out[idx] = rawPart
				continue
			}
			partText, isText := part["text"].(string)
			partType, hasType := part["type"].(string)
			if !isText || (hasType && partType != "text") {
				out[idx] = rawPart
				continue
			}
			next := dshHidden(partText, said)
			if next == partText {
				out[idx] = rawPart
				continue
			}
			changed = true
			cloned := cloneMap(part)
			cloned["text"] = next
			out[idx] = cloned
		}
		return out
	}
	if msgs, ok := chat["messages"].([]any); ok {
		for _, rawMsg := range msgs {
			msg, isMap := rawMsg.(map[string]any)
			if !isMap {
				continue
			}
			role := strField(msg, "role")
			if role == "tool" {
				continue
			}
			msg["content"] = rewriteContent(msg["content"], role != "system" && role != "developer")
		}
	}
	if sysVal, exists := chat["system"]; exists {
		chat["system"] = rewriteContent(sysVal, false)
	}
	var rewriteDescriptions func(node any)
	rewriteDescriptions = func(node any) {
		switch typed := node.(type) {
		case []any:
			for _, elem := range typed {
				rewriteDescriptions(elem)
			}
		case map[string]any:
			for key, val := range typed {
				if key == "description" {
					if desc, isStr := val.(string); isStr {
						next := dshHidden(desc, false)
						if next != desc {
							typed[key] = next
							changed = true
						}
					}
				} else {
					rewriteDescriptions(val)
				}
			}
		}
	}
	if tools, ok := chat["tools"].([]any); ok {
		rewriteDescriptions(tools)
	}
	return changed
}

func toolChoiceOn(chat map[string]any) bool {
	choice, exists := chat["tool_choice"]
	if !exists {
		return false
	}
	if choiceStr, isStr := choice.(string); isStr {
		if strings.ToLower(strings.TrimSpace(choiceStr)) != "none" {
			return false
		}
		delete(chat, "tool_choice")
		delete(chat, "tools")
		delete(chat, "functions")
		return true
	}
	choiceObj, isMap := choice.(map[string]any)
	if !isMap || choiceObj == nil {
		delete(chat, "tool_choice")
		return true
	}
	choiceType := strings.ToLower(strField(choiceObj, "type"))
	switch choiceType {
	case "none":
		delete(chat, "tool_choice")
		delete(chat, "tools")
		delete(chat, "functions")
	case "auto", "required":
		chat["tool_choice"] = choiceType
	case "function":
		fnObj, _ := choiceObj["function"].(map[string]any)
		name := strField(fnObj, "name")
		if name == "" {
			name = "auto"
		}
		chat["tool_choice"] = name
	default:
		delete(chat, "tool_choice")
	}
	return true
}

func safeName(name string, taken map[string]bool) string {
	base := safeNameBadRe.ReplaceAllString(name, "_")
	if base == "" {
		base = "tool"
	}
	candidate := base
	for suffix := 2; taken[candidate]; suffix++ {
		candidate = fmt.Sprintf("%s_%d", base, suffix)
	}
	taken[candidate] = true
	return candidate
}

func replaceNulEscape(pattern string) string {
	if !strings.Contains(pattern, `\0`) {
		return pattern
	}
	var builder strings.Builder
	changed := false
	for idx := 0; idx < len(pattern); {
		if pattern[idx] == '\\' && idx+1 < len(pattern) {
			next := pattern[idx+1]
			if next == '\\' {
				builder.WriteString(`\\`)
				idx += 2
				continue
			}
			if next == '0' && (idx+2 >= len(pattern) || pattern[idx+2] < '0' || pattern[idx+2] > '9') {
				builder.WriteString(`\u0000`)
				changed = true
				idx += 2
				continue
			}
		}
		builder.WriteByte(pattern[idx])
		idx++
	}
	if !changed {
		return pattern
	}
	return builder.String()
}

func fitSchema(node any, depth int) bool {
	if node == nil || depth > 64 {
		return false
	}
	changed := false
	switch typed := node.(type) {
	case []any:
		for _, elem := range typed {
			if fitSchema(elem, depth+1) {
				changed = true
			}
		}
		return changed
	case map[string]any:
		if pat, ok := typed["pattern"].(string); ok {
			if next := replaceNulEscape(pat); next != pat {
				typed["pattern"] = next
				pat = next
				changed = true
			}
			if pat == `^[^\uD800-\uDFFF]*$` {
				delete(typed, "pattern")
				changed = true
			}
		}
		if itemsArr, ok := typed["items"].([]any); ok {
			var objs []any
			for _, elem := range itemsArr {
				if elemObj, isMap := elem.(map[string]any); isMap && elemObj != nil {
					objs = append(objs, elemObj)
				}
			}
			switch len(objs) {
			case 0:
				typed["items"] = map[string]any{}
			case 1:
				typed["items"] = objs[0]
			default:
				typed["items"] = map[string]any{"anyOf": objs}
			}
			delete(typed, "additionalItems")
			changed = true
		}
		for _, child := range typed {
			if fitSchema(child, depth+1) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

func jsonValueType(val any) string {
	if val == nil {
		return "null"
	}
	switch typed := val.(type) {
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		if !math.IsNaN(typed) && !math.IsInf(typed, 0) && math.Trunc(typed) == typed {
			return "integer"
		}
		return "number"
	case int, int64:
		return "integer"
	default:
		return "string"
	}
}

func jsonEqual(left, right any) bool {
	leftBytes, errA := json.Marshal(left)
	rightBytes, errB := json.Marshal(right)
	if errA != nil || errB != nil {
		return false
	}
	return bytes.Equal(leftBytes, rightBytes)
}

func fitEnum(schema map[string]any) bool {
	rawEnum, exists := schema["enum"]
	if !exists {
		return false
	}
	enumVals, ok := rawEnum.([]any)
	if !ok || len(enumVals) == 0 {
		delete(schema, "enum")
		return true
	}
	var listed []string
	switch rawType := schema["type"].(type) {
	case string:
		listed = []string{rawType}
	case []any:
		for _, elem := range rawType {
			if str, isStr := elem.(string); isStr {
				listed = append(listed, str)
			}
		}
	}
	nonNull := 0
	for _, tName := range listed {
		if tName != "null" {
			nonNull++
		}
	}
	if nonNull == 1 {
		return false
	}
	typeOrder := []string{}
	byType := map[string][]any{}
	for _, val := range enumVals {
		tName := jsonValueType(val)
		if _, seen := byType[tName]; !seen {
			typeOrder = append(typeOrder, tName)
		}
		byType[tName] = append(byType[tName], val)
	}
	if _, hasInt := byType["integer"]; hasInt {
		if _, hasNum := byType["number"]; hasNum {
			byType["number"] = append(byType["number"], byType["integer"]...)
			delete(byType, "integer")
			typeOrder = slices.DeleteFunc(typeOrder, func(s string) bool { return s == "integer" })
		}
	}
	_, hasNull := byType["null"]
	delete(byType, "null")
	typeOrder = slices.DeleteFunc(typeOrder, func(s string) bool { return s == "null" })
	if len(byType) <= 1 {
		var want any
		if len(typeOrder) == 0 {
			want = "null"
		} else if hasNull {
			want = []any{typeOrder[0], "null"}
		} else {
			want = typeOrder[0]
		}
		if jsonEqual(want, schema["type"]) {
			return false
		}
		schema["type"] = want
		return true
	}
	split := make([]any, 0, len(typeOrder)+1)
	for _, tName := range typeOrder {
		split = append(split, map[string]any{"type": tName, "enum": byType[tName]})
	}
	if hasNull {
		split = append(split, map[string]any{"type": "null"})
	}
	delete(schema, "enum")
	delete(schema, "type")
	if existingAny, ok := schema["anyOf"].([]any); ok && existingAny != nil {
		existingAll, _ := schema["allOf"].([]any)
		schema["allOf"] = append(existingAll, map[string]any{"anyOf": split})
	} else {
		schema["anyOf"] = split
	}
	return true
}

func subSchema(val any, key string) (any, bool) {
	bVal, isBool := val.(bool)
	if !isBool || boolOKKeys[key] {
		return val, false
	}
	if bVal {
		return map[string]any{}, true
	}
	return map[string]any{"not": map[string]any{}}, true
}

func fitShape(schema map[string]any, depth int) bool {
	if schema == nil || depth > 64 {
		return false
	}
	changed := false
	if rawType, exists := schema["type"]; exists {
		var list []any
		if arr, isArr := rawType.([]any); isArr {
			list = arr
		} else {
			list = []any{rawType}
		}
		var uniq []string
		seen := map[string]bool{}
		for _, elem := range list {
			str, isStr := elem.(string)
			if !isStr {
				continue
			}
			lower := strings.ToLower(str)
			if schemaTypes[lower] && !seen[lower] {
				seen[lower] = true
				uniq = append(uniq, lower)
			}
		}
		var want any
		switch len(uniq) {
		case 0:
			want = nil
		case 1:
			want = uniq[0]
		default:
			arr := make([]any, len(uniq))
			for idx, val := range uniq {
				arr[idx] = val
			}
			want = arr
		}
		if want == nil {
			delete(schema, "type")
			changed = true
		} else if !jsonEqual(want, rawType) {
			schema["type"] = want
			changed = true
		}
	}
	if rawReq, exists := schema["required"]; exists {
		reqArr, isArr := rawReq.([]any)
		if !isArr {
			delete(schema, "required")
			changed = true
		} else {
			var filtered []any
			hasBad := false
			for _, elem := range reqArr {
				if _, isStr := elem.(string); isStr {
					filtered = append(filtered, elem)
				} else {
					hasBad = true
				}
			}
			if hasBad {
				schema["required"] = filtered
				changed = true
			}
		}
	}
	if fitEnum(schema) {
		changed = true
	}
	for _, key := range subOneKeys {
		rawVal, exists := schema[key]
		if !exists {
			continue
		}
		nextVal, replaced := subSchema(rawVal, key)
		if replaced {
			schema[key] = nextVal
			changed = true
		}
		if subMap, isMap := nextVal.(map[string]any); isMap {
			if fitShape(subMap, depth+1) {
				changed = true
			}
		}
	}
	for _, key := range subListKeys {
		arr, ok := schema[key].([]any)
		if !ok {
			continue
		}
		for idx, rawVal := range arr {
			nextVal, replaced := subSchema(rawVal, key)
			if replaced {
				arr[idx] = nextVal
				changed = true
			}
			if subMap, isMap := nextVal.(map[string]any); isMap {
				if fitShape(subMap, depth+1) {
					changed = true
				}
			}
		}
	}
	for _, key := range subMapKeys {
		mapping, ok := schema[key].(map[string]any)
		if !ok || mapping == nil {
			continue
		}
		for name, rawVal := range mapping {
			nextVal, replaced := subSchema(rawVal, key)
			if replaced {
				mapping[name] = nextVal
				changed = true
			}
			if subMap, isMap := nextVal.(map[string]any); isMap {
				if fitShape(subMap, depth+1) {
					changed = true
				}
			}
		}
	}
	return changed
}

func localRef(root, schema map[string]any) map[string]any {
	ref, ok := schema["$ref"].(string)
	if !ok || !strings.HasPrefix(ref, "#/") {
		return schema
	}
	var cur any = root
	for _, seg := range strings.Split(ref[2:], "/") {
		seg = strings.ReplaceAll(seg, "~1", "/")
		seg = strings.ReplaceAll(seg, "~0", "~")
		curMap, isMap := cur.(map[string]any)
		if !isMap {
			return schema
		}
		cur = curMap[seg]
	}
	if target, isMap := cur.(map[string]any); isMap && target != nil {
		return target
	}
	return schema
}

func deepCloneMap(src map[string]any) map[string]any {
	encoded, err := json.Marshal(src)
	if err != nil {
		return cloneMap(src)
	}
	var dst map[string]any
	if json.Unmarshal(encoded, &dst) != nil {
		return cloneMap(src)
	}
	return dst
}

func cloneMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for key, val := range src {
		dst[key] = val
	}
	return dst
}

func hasAnyUnion(schema map[string]any) bool {
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		if _, ok := schema[key].([]any); ok {
			return true
		}
	}
	return false
}

func objectRoot(ps, root map[string]any, depth int) bool {
	var unions []string
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		if _, ok := ps[key].([]any); ok {
			unions = append(unions, key)
		}
	}
	if ps["type"] == "object" && len(unions) == 0 {
		return false
	}
	props, ok := ps["properties"].(map[string]any)
	if !ok || props == nil {
		props = map[string]any{}
	}
	var required []string
	if rawReq, ok := ps["required"].([]any); ok {
		for _, elem := range rawReq {
			if str, isStr := elem.(string); isStr {
				required = append(required, str)
			}
		}
	}
	if allOf, ok := ps["allOf"].([]any); ok {
		for _, rawBranch := range allOf {
			branchMap, isMap := rawBranch.(map[string]any)
			if !isMap {
				continue
			}
			branch := localRef(root, branchMap)
			if depth < 8 && hasAnyUnion(branch) {
				branch = deepCloneMap(branch)
				objectRoot(branch, root, depth+1)
			}
			if bProps, hasProps := branch["properties"].(map[string]any); hasProps {
				for key, val := range bProps {
					if _, exists := props[key]; !exists {
						props[key] = val
					}
				}
			}
			if bReq, hasReq := branch["required"].([]any); hasReq {
				for _, elem := range bReq {
					if str, isStr := elem.(string); isStr {
						required = append(required, str)
					}
				}
			}
		}
	}
	var branches []map[string]any
	for _, key := range []string{"anyOf", "oneOf"} {
		arr, ok := ps[key].([]any)
		if !ok {
			continue
		}
		for _, rawBranch := range arr {
			branchMap, isMap := rawBranch.(map[string]any)
			if !isMap {
				continue
			}
			branch := localRef(root, branchMap)
			if depth < 8 && hasAnyUnion(branch) {
				branch = deepCloneMap(branch)
				objectRoot(branch, root, depth+1)
			}
			_, hasProps := branch["properties"].(map[string]any)
			if branch["type"] != "object" && !hasProps {
				continue
			}
			branches = append(branches, branch)
		}
	}
	from := map[string][]any{}
	var fromOrder []string
	for _, branch := range branches {
		bProps, _ := branch["properties"].(map[string]any)
		for key, val := range bProps {
			if _, exists := props[key]; exists {
				continue
			}
			seen, hasKey := from[key]
			if !hasKey {
				fromOrder = append(fromOrder, key)
			}
			dup := false
			for _, existing := range seen {
				if jsonEqual(existing, val) {
					dup = true
					break
				}
			}
			if !dup {
				from[key] = append(seen, val)
			}
		}
	}
	for _, key := range fromOrder {
		list := from[key]
		if len(list) == 1 {
			props[key] = list[0]
		} else {
			props[key] = map[string]any{"anyOf": list}
		}
	}
	if len(branches) > 0 {
		firstProps, _ := branches[0]["properties"].(map[string]any)
		for key := range firstProps {
			allReq := true
			for _, branch := range branches {
				bReq, ok := branch["required"].([]any)
				if !ok {
					allReq = false
					break
				}
				found := false
				for _, elem := range bReq {
					if str, isStr := elem.(string); isStr && str == key {
						found = true
						break
					}
				}
				if !found {
					allReq = false
					break
				}
			}
			if allReq {
				required = append(required, key)
			}
		}
	}
	for _, key := range unions {
		delete(ps, key)
	}
	ps["type"] = "object"
	ps["properties"] = props
	var finalReq []any
	seenReq := map[string]bool{}
	for _, key := range required {
		if _, exists := props[key]; exists && !seenReq[key] {
			seenReq[key] = true
			finalReq = append(finalReq, key)
		}
	}
	if len(finalReq) > 0 {
		ps["required"] = finalReq
	} else {
		delete(ps, "required")
	}
	return true
}

func objectBranch(root, branch map[string]any, seen []string) bool {
	if branch == nil {
		return false
	}
	if rawType, exists := branch["type"]; exists {
		return rawType == "object"
	}
	ref, ok := branch["$ref"].(string)
	if !ok || slices.Contains(seen, ref) || len(seen) > 16 {
		return false
	}
	target := localRef(root, branch)
	if fmt.Sprintf("%p", target) == fmt.Sprintf("%p", branch) {
		return false
	}
	return objectBranch(root, target, append(seen, ref))
}

func typeUnions(schema, root map[string]any, depth int) bool {
	if schema == nil || depth > 64 {
		return false
	}
	changed := false
	if _, hasType := schema["type"]; !hasType {
		if oneOf, ok := schema["oneOf"].([]any); ok && len(oneOf) > 0 {
			allObj := true
			for _, rawBranch := range oneOf {
				branchMap, isMap := rawBranch.(map[string]any)
				if !isMap || !objectBranch(root, branchMap, nil) {
					allObj = false
					break
				}
			}
			if allObj {
				schema["type"] = "object"
				changed = true
			}
		}
	}
	for _, key := range subOneKeys {
		if subMap, isMap := schema[key].(map[string]any); isMap {
			if typeUnions(subMap, root, depth+1) {
				changed = true
			}
		}
	}
	for _, key := range subListKeys {
		if arr, ok := schema[key].([]any); ok {
			for _, elem := range arr {
				if subMap, isMap := elem.(map[string]any); isMap {
					if typeUnions(subMap, root, depth+1) {
						changed = true
					}
				}
			}
		}
	}
	for _, key := range subMapKeys {
		if mapping, ok := schema[key].(map[string]any); ok {
			for _, elem := range mapping {
				if subMap, isMap := elem.(map[string]any); isMap {
					if typeUnions(subMap, root, depth+1) {
						changed = true
					}
				}
			}
		}
	}
	return changed
}

func fitTools(chat map[string]any) (bool, map[string]string) {
	names := map[string]string{}
	tools, ok := chat["tools"].([]any)
	if !ok || len(tools) == 0 {
		return false, names
	}
	changed := false
	taken := map[string]bool{}
	for _, rawTool := range tools {
		tool, isMap := rawTool.(map[string]any)
		if !isMap {
			continue
		}
		fn, _ := tool["function"].(map[string]any)
		if name, isStr := fn["name"].(string); isStr && safeNameOKRe.MatchString(name) {
			taken[name] = true
		}
	}
	theirs := map[string]string{}
	for _, rawTool := range tools {
		tool, isMap := rawTool.(map[string]any)
		if !isMap {
			continue
		}
		fn, _ := tool["function"].(map[string]any)
		if fn == nil {
			continue
		}
		if name, isStr := fn["name"].(string); isStr && !safeNameOKRe.MatchString(name) {
			safe := safeName(name, taken)
			names[safe] = name
			theirs[name] = safe
			fn["name"] = safe
			changed = true
		}
		rawParams, hasParams := fn["parameters"]
		if !hasParams {
			continue
		}
		params, isMap := rawParams.(map[string]any)
		if !isMap || params == nil {
			fn["parameters"] = map[string]any{"type": "object", "properties": map[string]any{}}
			changed = true
			continue
		}
		if fitSchema(params, 0) {
			changed = true
		}
		if fitShape(params, 0) {
			changed = true
		}
		if typeUnions(params, params, 0) {
			changed = true
		}
		if objectRoot(params, params, 0) {
			changed = true
		}
	}
	if len(theirs) > 0 {
		if msgs, ok := chat["messages"].([]any); ok {
			for _, rawMsg := range msgs {
				msg, isMap := rawMsg.(map[string]any)
				if !isMap {
					continue
				}
				if tcs, ok := msg["tool_calls"].([]any); ok {
					for _, rawTC := range tcs {
						tc, isTCMap := rawTC.(map[string]any)
						if !isTCMap {
							continue
						}
						fn, _ := tc["function"].(map[string]any)
						if fn != nil {
							if name, isStr := fn["name"].(string); isStr {
								if safe, mapped := theirs[name]; mapped {
									fn["name"] = safe
								}
							}
						}
					}
				}
				if strField(msg, "role") == "tool" {
					if name, isStr := msg["name"].(string); isStr {
						if safe, mapped := theirs[name]; mapped {
							msg["name"] = safe
						}
					}
				}
			}
		}
		if choiceObj, ok := chat["tool_choice"].(map[string]any); ok {
			if fn, ok := choiceObj["function"].(map[string]any); ok {
				if name, isStr := fn["name"].(string); isStr {
					if safe, mapped := theirs[name]; mapped {
						fn["name"] = safe
					}
				}
			}
		}
	}
	return changed, names
}

func namesEffort(chat map[string]any) bool {
	if _, exists := chat["reasoning_effort"]; exists {
		return true
	}
	if reasoning, ok := chat["reasoning"].(map[string]any); ok && reasoning != nil {
		if _, exists := reasoning["effort"]; exists {
			return true
		}
	}
	return false
}

func withDefaultEffort(chat map[string]any, list []liveModel) bool {
	if chat == nil || namesEffort(chat) {
		return false
	}
	if limit, ok := asFloat(firstPresent(chat, "max_tokens", "max_completion_tokens")); ok && limit > 0 && limit <= SmallBudget {
		return false
	}
	modelID := strField(chat, "model")
	for _, model := range list {
		if model.ID == modelID && model.Effort != "" {
			chat["reasoning_effort"] = model.Effort
			return true
		}
	}
	return false
}

func inputLengthOf(msgs []any) int {
	for idx := len(msgs) - 1; idx >= 0; idx-- {
		msg, isMap := msgs[idx].(map[string]any)
		if !isMap || strField(msg, "role") != "user" || answeredBefore(msgs, idx) {
			continue
		}
		switch content := msg["content"].(type) {
		case string:
			return len([]rune(content))
		case []any:
			total := 0
			for _, rawPart := range content {
				part, isPartMap := rawPart.(map[string]any)
				if !isPartMap {
					continue
				}
				partType := strField(part, "type")
				if partType == "" || partType == "text" {
					if text, isStr := part["text"].(string); isStr {
						total += len([]rune(text))
					}
				}
			}
			return total
		default:
			return 0
		}
	}
	return 0
}

func siteModelName(site Site, modelID string) string {
	for _, spec := range site.Models {
		if spec.ID == modelID {
			return spec.Name
		}
	}
	return modelID
}

func withGrowth(site Site, chat map[string]any, conv, prevConv string) bool {
	if !site.AI || conv == "" || chat == nil {
		return false
	}
	msgs, ok := chat["messages"].([]any)
	if !ok {
		return false
	}
	vars := map[string]any{}
	if existing, isMap := chat["extra_vars"].(map[string]any); isMap && existing != nil {
		for key, val := range existing {
			vars[key] = val
		}
	}
	var events []any
	if rawGE, isStr := vars["growthEvent"].(string); isStr && strings.TrimSpace(rawGE) != "" {
		var decoded any
		if json.Unmarshal([]byte(rawGE), &decoded) == nil {
			if arr, isArr := decoded.([]any); isArr {
				events = arr
			} else if decoded != nil {
				events = []any{decoded}
			}
		}
	}
	if prevConv != "" {
		var kept []any
		for _, ev := range events {
			evMap, isMap := ev.(map[string]any)
			if isMap && strField(evMap, "eventCode") == GrowthSendEvent && strField(evMap, "id") == prevConv {
				continue
			}
			kept = append(kept, ev)
		}
		events = kept
	}
	modelID := strField(chat, "model")
	exists := false
	for _, ev := range events {
		evMap, isMap := ev.(map[string]any)
		if isMap && strField(evMap, "eventCode") == GrowthSendEvent && strField(evMap, "id") == conv {
			exists = true
			break
		}
	}
	if !exists {
		events = append(events, map[string]any{
			"eventCode": GrowthSendEvent,
			"id":        conv,
			"extra": map[string]any{
				"inputLength":      inputLengthOf(msgs),
				"requestModelId":   modelID,
				"requestModelName": siteModelName(site, modelID),
				"mode":             "craft",
				"command":          "",
			},
		})
	}
	encodedEvents, _ := json.Marshal(events)
	vars["growthEvent"] = string(encodedEvents)
	if _, hasInit := vars["initiator"]; !hasInit {
		vars["initiator"] = "user"
	}
	chat["extra_vars"] = vars
	return true
}

func regrown(site Site, body []byte, prevConv, nextConv string) ([]byte, bool) {
	var chat map[string]any
	if json.Unmarshal(body, &chat) != nil || chat == nil {
		return nil, false
	}
	if !withGrowth(site, chat, nextConv, prevConv) {
		return nil, false
	}
	encoded, err := json.Marshal(chat)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

func gzipBody(completion bool, body []byte) ([]byte, bool) {
	if !completion || len(body) == 0 {
		return nil, false
	}
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(body); err != nil {
		_ = writer.Close()
		return nil, false
	}
	if err := writer.Close(); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

func namedBack(doc map[string]any, names map[string]string) bool {
	if len(names) == 0 || doc == nil {
		return false
	}
	choices, ok := doc["choices"].([]any)
	if !ok {
		return false
	}
	changed := false
	for _, rawChoice := range choices {
		choice, isMap := rawChoice.(map[string]any)
		if !isMap {
			continue
		}
		for _, field := range []string{"message", "delta"} {
			container, _ := choice[field].(map[string]any)
			if container == nil {
				continue
			}
			tcs, ok := container["tool_calls"].([]any)
			if !ok {
				continue
			}
			for _, rawTC := range tcs {
				tc, isTC := rawTC.(map[string]any)
				if !isTC {
					continue
				}
				fn, _ := tc["function"].(map[string]any)
				if fn == nil {
					continue
				}
				if currentName, isStr := fn["name"].(string); isStr {
					if orig, mapped := names[currentName]; mapped {
						fn["name"] = orig
						changed = true
					}
				}
			}
		}
	}
	return changed
}

func renameJSONBody(raw []byte, names map[string]string) []byte {
	if len(names) == 0 {
		return raw
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil || doc == nil {
		return raw
	}
	if !namedBack(doc, names) {
		return raw
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return encoded
}

func renameSSELine(line string, names map[string]string) string {
	if len(names) == 0 {
		return line
	}
	match := sseDataLineRe.FindStringSubmatch(line)
	if len(match) < 2 || match[1] == "[DONE]" {
		return line
	}
	var doc map[string]any
	if json.Unmarshal([]byte(match[1]), &doc) != nil || doc == nil {
		return line
	}
	if !namedBack(doc, names) {
		return line
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return line
	}
	return "data: " + string(encoded)
}

func filterSaidMatch(text string) bool {
	normalized := strings.TrimSpace(strings.ReplaceAll(text, "’", "'"))
	for _, phrase := range filterSays {
		if strings.HasPrefix(phrase, normalized) {
			return true
		}
	}
	return false
}

type unsaidFilter struct {
	open bool
	said string
	held []string
	rest string
}

func newUnsaidFilter() *unsaidFilter {
	return &unsaidFilter{open: true}
}

func parseSSEEventJSON(ev string) map[string]any {
	for _, line := range strings.Split(ev, "\n") {
		match := sseDataLineRe.FindStringSubmatch(line)
		if len(match) < 2 || match[1] == "[DONE]" {
			continue
		}
		var doc map[string]any
		if json.Unmarshal([]byte(match[1]), &doc) == nil && doc != nil {
			return doc
		}
	}
	return nil
}

func stripFilterContent(ev string) string {
	doc := parseSSEEventJSON(ev)
	if doc == nil {
		return ev
	}
	choices, ok := doc["choices"].([]any)
	if !ok || len(choices) == 0 {
		return ev
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return ev
	}
	delta, ok := choice["delta"].(map[string]any)
	if !ok {
		return ev
	}
	if _, isStr := delta["content"].(string); !isStr {
		return ev
	}
	delete(delta, "content")
	remainingKeys := 0
	for key := range delta {
		if key != "role" {
			remainingKeys++
		}
	}
	if remainingKeys == 0 && choice["finish_reason"] == nil && doc["usage"] == nil {
		return ""
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return ev
	}
	return "data: " + string(encoded)
}

func (filter *unsaidFilter) handleEvent(ev string) []string {
	if !filter.open {
		return []string{ev}
	}
	doc := parseSSEEventJSON(ev)
	var choice map[string]any
	if doc != nil {
		if choices, ok := doc["choices"].([]any); ok && len(choices) > 0 {
			choice, _ = choices[0].(map[string]any)
		}
	}
	var delta map[string]any
	if choice != nil {
		delta, _ = choice["delta"].(map[string]any)
	}
	text, _ := delta["content"].(string)
	finish, _ := choice["finish_reason"].(string)
	hasCalls := false
	if delta != nil {
		if rawCalls, exists := delta["tool_calls"]; exists && rawCalls != nil {
			if arr, isArr := rawCalls.([]any); isArr {
				hasCalls = len(arr) > 0
			} else {
				hasCalls = true
			}
		}
	}
	if hasCalls || (text != "" && !filterSaidMatch(filter.said+text)) {
		filter.open = false
		out := append(slices.Clone(filter.held), ev)
		filter.held = nil
		return out
	}
	filter.said += text
	if finish == "content_filter" && filter.said != "" {
		filter.open = false
		combined := append(slices.Clone(filter.held), ev)
		filter.held = nil
		var out []string
		for _, item := range combined {
			if stripped := stripFilterContent(item); stripped != "" {
				out = append(out, stripped)
			}
		}
		return out
	}
	if finish != "" {
		filter.open = false
		out := append(slices.Clone(filter.held), ev)
		filter.held = nil
		return out
	}
	if filter.said == "" {
		return []string{ev}
	}
	filter.held = append(filter.held, ev)
	return nil
}

func (filter *unsaidFilter) push(chunk string) string {
	filter.rest = strings.ReplaceAll(filter.rest+chunk, "\r\n", "\n")
	parts := strings.Split(filter.rest, "\n\n")
	filter.rest = parts[len(parts)-1]
	events := parts[:len(parts)-1]
	var builder strings.Builder
	for _, ev := range events {
		for _, emitted := range filter.handleEvent(ev) {
			builder.WriteString(emitted)
			builder.WriteString("\n\n")
		}
	}
	return builder.String()
}

func (filter *unsaidFilter) flush() string {
	var builder strings.Builder
	for _, ev := range filter.held {
		builder.WriteString(ev)
		builder.WriteString("\n\n")
	}
	filter.held = nil
	builder.WriteString(filter.rest)
	filter.rest = ""
	return builder.String()
}

type stepInfo struct {
	Model     string
	Input     int
	UserSend  bool
	Step      int
	ToolCalls int
	History   int
	Conv      string
	Turn      string
	Message   string
}

func stepOf(chat map[string]any) *stepInfo {
	msgs, ok := chat["messages"].([]any)
	if !ok {
		return nil
	}
	stepCount := 1
	toolCalls := 0
	for idx := len(msgs) - 1; idx >= 0; idx-- {
		msg, isMap := msgs[idx].(map[string]any)
		if !isMap {
			continue
		}
		role := strField(msg, "role")
		if role == "user" {
			break
		}
		if role == "assistant" {
			stepCount++
			if tcs, ok := msg["tool_calls"].([]any); ok {
				toolCalls += len(tcs)
			}
		}
	}
	userSend := false
	if len(msgs) > 0 {
		if last, isMap := msgs[len(msgs)-1].(map[string]any); isMap && strField(last, "role") == "user" {
			userSend = true
		}
	}
	history := 0
	for _, rawMsg := range msgs {
		if msg, isMap := rawMsg.(map[string]any); isMap {
			role := strField(msg, "role")
			if role == "user" || role == "assistant" {
				history++
			}
		}
	}
	return &stepInfo{
		Model:     strField(chat, "model"),
		Input:     inputLengthOf(msgs),
		UserSend:  userSend,
		Step:      stepCount,
		ToolCalls: toolCalls,
		History:   history,
	}
}

type replyStats struct {
	Finish string
	Input  int64
	Output int64
	Total  int64
	Cached int64
}

func lastMatchedInt(re *regexp.Regexp, text string) int64 {
	all := re.FindAllStringSubmatch(text, -1)
	if len(all) == 0 {
		return 0
	}
	val, _ := strconv.ParseInt(all[len(all)-1][1], 10, 64)
	return val
}

func replyFacts(tail string) replyStats {
	out := replyStats{}
	reasons := finishReasonRe.FindAllStringSubmatch(tail, -1)
	if len(reasons) > 0 {
		out.Finish = reasons[len(reasons)-1][1]
	}
	out.Input = lastMatchedInt(promptTokRe, tail)
	out.Output = lastMatchedInt(compTokRe, tail)
	out.Total = lastMatchedInt(totalTokRe, tail)
	if out.Total == 0 {
		out.Total = out.Input + out.Output
	}
	out.Cached = lastMatchedInt(cachedTokRe, tail)
	return out
}

func (owner *module) reportBase(site Site, acct Credential, code string, nowMs int64) map[string]any {
	tzName := owner.options.Now().Location().String()
	if tzName == "Local" {
		tzName = ""
	}
	return map[string]any{
		"eventCode":    code,
		"buddyId":      "",
		"buddyName":    "",
		"timezone":     tzName,
		"userId":       acct.UID,
		"username":     acct.Name,
		"userNickname": acct.Name,
		"product":      "SaaS",
		"commit":       "",
		"ideName":      "WorkBuddy",
		"ideType":      "WorkBuddy",
		"ideVersion":   site.App,
		"machineId":    owner.machineHash(),
		"sessionId":    owner.reportSession,
		"extName":      site.AppName,
		"extVersion":   site.CLI,
		"os":           owner.options.Platform,
		"arch":         owner.options.Arch,
		"osVersion":    "",
		"cpuModel":     "",
		"timestamp":    nowMs,
		"reportDelay":  0,
		"releaseDate":  0,
		"cpuCores":     runtime.NumCPU(),
		"memorySize":   0,
	}
}

func (owner *module) reportEvents(site Site, acct Credential, step *stepInfo, facts replyStats, startMs, endMs int64) [][]map[string]any {
	name := siteModelName(site, step.Model)
	finish := facts.Finish
	if finish == "" {
		finish = "stop"
	}
	last := facts.Finish != "tool_calls"
	missTokens := facts.Input - facts.Cached
	if missTokens < 0 {
		missTokens = 0
	}
	mergeEvent := func(code string, extra map[string]any) map[string]any {
		ev := owner.reportBase(site, acct, code, endMs)
		for key, val := range extra {
			ev[key] = val
		}
		return ev
	}
	withIDs := func(target map[string]any, includeTrace bool) {
		target["conversationId"] = step.Conv
		target["requestId"] = step.Turn
		if includeTrace {
			target["traceId"] = step.Message
		}
		target["rootRequestId"] = step.Turn
		target["parentConversationId"] = ""
		target["agentName"] = ""
		target["agentType"] = "main"
		target["codebuddy.session_id"] = step.Conv
		target["codebuddy.conversation_request_id"] = step.Turn
	}
	withModel := func(target map[string]any) {
		target["requestModelId"] = step.Model
		target["requestModelName"] = name
	}
	withTokens := func(target map[string]any) {
		target["inputToken"] = facts.Input
		target["outputToken"] = facts.Output
		target["totalToken"] = facts.Total
		target["cachedTokens"] = facts.Cached
		target["cachedWriteTokens"] = 0
		target["cachedMissTokens"] = missTokens
	}
	withOK := func(target map[string]any) {
		target["isSuccessful"] = true
		target["messageErrorCode"] = ""
		target["finishReason"] = finish
	}

	msgSend := mergeEvent("chat_message_send", map[string]any{
		"messageId":          step.Message,
		"historyCount":       step.History,
		"currentStepCount":   step.Step,
		"totalStepCount":     step.Step,
		"compactionCount":    0,
		"presentAt":          startMs,
		"isContextTruncated": false,
	})
	withIDs(msgSend, true)
	withModel(msgSend)
	first := []map[string]any{msgSend}

	var second []map[string]any
	if step.UserSend {
		reqSend := mergeEvent("chat_request_send", map[string]any{
			"mode":                  "craft",
			"source":                "work",
			"command":               "",
			"inputLength":           step.Input,
			"mentionContextCount":   0,
			"presentAt":             startMs,
			"isPlan":                false,
			"isAutoExecuteTerminal": false,
			"isAutoModify":          false,
			"codebaseEnable":        false,
			"isMaxMode":             true,
			"mentionContexts":       []any{},
			"knowledgeId":           []any{},
			"knowledgeName":         []any{},
			"codebaseId":            "",
			"inputAttemptId":        "",
			"recommendId":           "",
			"skillId":               "",
			"vcsType":               "",
			"vcsRepo":               "",
			"vcsBranchName":         "",
			"vcsRevId":              "",
		})
		withIDs(reqSend, true)
		withModel(reqSend)
		second = append(second, reqSend)
	}
	msgResp := mergeEvent("chat_message_response", map[string]any{
		"messageId":       step.Message,
		"responseModelId": step.Model,
		"totalStepCount":  step.Step,
		"firstTokenAt":    startMs,
		"presentAt":       endMs,
	})
	withIDs(msgResp, true)
	withModel(msgResp)
	withTokens(msgResp)
	withOK(msgResp)
	second = append(second, msgResp)

	msgStatus := mergeEvent("chat_message_status", map[string]any{
		"messageId":        step.Message,
		"messageErrorCode": "0",
	})
	withIDs(msgStatus, true)
	withModel(msgStatus)
	second = append(second, msgStatus)

	var third []map[string]any
	if last {
		reqResp := mergeEvent("chat_request_response", map[string]any{
			"mode":          "craft",
			"toolCallCount": step.ToolCalls,
			"presentAt":     endMs,
		})
		withIDs(reqResp, false)
		withModel(reqResp)
		withTokens(reqResp)
		withOK(reqResp)
		second = append(second, reqResp)

		durMs := endMs - startMs
		if durMs < 0 {
			durMs = 0
		}
		third = append(third, mergeEvent("agent_task_completed", map[string]any{
			"mode":        "LOCAL",
			"task_id":     step.Turn,
			"duration_ms": durMs,
			"total_steps": step.Step,
		}))
		third = append(third, mergeEvent("chat_response_complete", map[string]any{
			"conversationId": step.Conv,
			"requestId":      step.Turn,
			"artifactType":   "",
			"source":         "work",
		}))
	}
	batches := [][]map[string]any{first, second}
	if len(third) > 0 {
		batches = append(batches, third)
	}
	return batches
}

func explainErrorBody(status int, origHeaders http.Header, raw []byte) ([]byte, http.Header) {
	text := string(raw)
	hdr := cloneHeaders(origHeaders)
	hdr.Del("Content-Length")
	hdr.Del("Content-Encoding")
	hdr.Set("X-Magpie-Sign-In", "kept")

	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	var errObj map[string]any
	if doc != nil {
		if sub, ok := doc["error"].(map[string]any); ok {
			errObj = sub
		} else {
			errObj = doc
		}
	}
	var codes []int
	addCode := func(val any) {
		if val == nil {
			return
		}
		if code := parseIntCode(val); code != 0 {
			codes = append(codes, code)
		}
	}
	if errObj != nil {
		addCode(errObj["internal_code"])
		if dataObj, ok := errObj["data"].(map[string]any); ok {
			addCode(dataObj["code"])
		}
		addCode(errObj["code"])
	}
	if doc != nil {
		if errStr, ok := doc["error"].(string); ok {
			if match := stringErrCodeRe.FindStringSubmatch(errStr); len(match) >= 2 {
				addCode(match[1])
			}
		}
		addCode(doc["code"])
		if dataObj, ok := doc["data"].(map[string]any); ok {
			addCode(dataObj["code"])
		}
	}
	isRefused := refusedRe.MatchString(text)
	isRestricted := !isRefused && (slices.Contains(codes, RestrictedCode) || isRestrictedText(text))
	isExhausted := !isRestricted && (slices.Contains(codes, ExhaustedCode) || isExhaustedText(text))
	if !isRefused && !isRestricted && !isExhausted {
		return raw, hdr
	}

	msg := strings.TrimSpace(text)
	switch {
	case errObj != nil && strField(errObj, "message") != "":
		msg = strField(errObj, "message")
	case errObj != nil && nestedStr(errObj, "data", "msg") != "":
		msg = nestedStr(errObj, "data", "msg")
	case doc != nil && strField(doc, "error") != "":
		msg = strField(doc, "error")
	case doc != nil && strField(doc, "msg") != "":
		msg = strField(doc, "msg")
	case doc != nil && nestedStr(doc, "data", "msg") != "":
		msg = nestedStr(doc, "data", "msg")
	}

	var hint, errType, where string
	var errCode any
	switch {
	case isRefused:
		hint = RefusedHint
		errType = "permission_error"
		where = RefusedDoc + " — "
		errCode = nil
	case isRestricted:
		hint = RestrictedHint
		errType = "permission_error"
		errCode = RestrictedCode
	default:
		hint = ExhaustedHint
		errType = "insufficient_quota"
		errCode = ExhaustedCode
	}
	fullMsg := fmt.Sprintf("%s — %s%s", msg, where, hint)
	encoded, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": fullMsg,
			"type":    errType,
			"code":    errCode,
		},
	})
	hdr.Set("Content-Type", "application/json")
	return encoded, hdr
}

func cloneHeaders(src http.Header) http.Header {
	dst := http.Header{}
	for key, vals := range src {
		dst[key] = slices.Clone(vals)
	}
	return dst
}
