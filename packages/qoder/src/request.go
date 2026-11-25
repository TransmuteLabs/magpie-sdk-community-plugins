package qoder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
)

const (
	smallBudget  = 1024
	agentSDKLine = "You are a Claude agent, built on Anthropic's Claude Agent SDK."
)

var (
	claudeCodeLineRe = regexp.MustCompile(`You\s+are\s+Claude\s+Code,\s+Anthropic's\s+official\s+CLI\s+for\s+Claude(?:,\s+running\s+within\s+the\s+Claude\s+Agent\s+SDK)?\.`)
	harnessOpenerRe  = regexp.MustCompile(`(?i)(\bYou are (?:an?|the)\s[^\n.,]{0,60}?\b(?:agent|assistant))\s+(powered by|running on|running in|built on|based on)\b`)
	kimiKeyRe        = regexp.MustCompile(`(?i)^kmodel`)
	kimiNameRe       = regexp.MustCompile(`(?i)kimi`)
	magpieAutoSessRe = regexp.MustCompile(`^magpie-[0-9a-f]{24}$`)

	effortRankOrder = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}

	uncachedBlockTypes = map[string]bool{
		"thinking":          true,
		"redacted_thinking": true,
		"tool_use":          true,
		"tool_result":       true,
	}

	defaultOutputCaps = map[string]int{
		"lmodel": 131072,
	}

	schemaSubOne  = []string{"items", "additionalItems", "unevaluatedItems", "contains", "not", "if", "then", "else", "propertyNames", "additionalProperties", "unevaluatedProperties"}
	schemaSubList = []string{"anyOf", "oneOf", "allOf", "prefixItems"}
	schemaSubMap  = []string{"properties", "patternProperties", "$defs", "definitions", "dependentSchemas"}
)

func asAgentSDK(text string) string {
	return claudeCodeLineRe.ReplaceAllString(text, agentSDKLine)
}

func harnessComma(text string) string {
	return harnessOpenerRe.ReplaceAllString(text, "$1, $2")
}

func fitEffort(want string, levels []string) string {
	if want == "ultra" && !slices.Contains(levels, want) {
		want = "max"
	}
	if len(levels) == 0 || slices.Contains(levels, want) {
		return want
	}
	wantIdx := slices.Index(effortRankOrder, want)
	if wantIdx < 0 {
		return want
	}
	best := want
	bestDist := len(effortRankOrder)
	for _, level := range levels {
		levelIdx := slices.Index(effortRankOrder, level)
		if levelIdx < 0 || level == "none" {
			continue
		}
		dist := levelIdx - wantIdx
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist || (dist == bestDist && levelIdx > wantIdx) {
			best = level
			bestDist = dist
		}
	}
	return best
}

func effortFor(asked string, entry ModelEntry, budget int) (bool, string) {
	want := strings.ToLower(strings.TrimSpace(asked))
	if !slices.Contains(effortRankOrder, want) {
		want = ""
	}
	if want == "" && budget > 0 && budget <= smallBudget {
		want = "none"
	}
	if !entry.Thinks || (want == "none" && !entry.AlwaysThinks) {
		return false, ""
	}
	if len(entry.Efforts) == 0 {
		return true, ""
	}
	if want == "" {
		return true, entry.DefaultEffort
	}
	if want == "none" {
		return true, entry.Efforts[0]
	}
	return true, fitEffort(want, entry.Efforts)
}

func kimiModel(entry ModelEntry) bool {
	if kimiKeyRe.MatchString(entry.Key) {
		return true
	}
	combined := entry.Key + " " + strField(entry.Config, "display_name") + " " + entry.Name
	return kimiNameRe.MatchString(combined)
}

func imageBlock(part map[string]any) map[string]any {
	if part == nil {
		return nil
	}
	var imgURL string
	switch rawURL := part["image_url"].(type) {
	case string:
		imgURL = rawURL
	case map[string]any:
		imgURL = rawStrField(rawURL, "url")
	}
	if imgURL == "" {
		return nil
	}
	return map[string]any{
		"type":      "image_url",
		"image_url": map[string]any{"url": imgURL},
	}
}

func blocksOf(content any) []map[string]any {
	switch typed := content.(type) {
	case string:
		if typed == "" {
			return nil
		}
		return []map[string]any{{"type": "text", "text": typed}}
	case []any:
		var out []map[string]any
		for _, item := range typed {
			part, ok := item.(map[string]any)
			if !ok || part == nil {
				continue
			}
			switch rawStrField(part, "type") {
			case "text":
				if part["text"] != nil {
					out = append(out, map[string]any{"type": "text", "text": rawStrField(part, "text")})
				}
			case "image_url":
				if blk := imageBlock(part); blk != nil {
					out = append(out, blk)
				}
			}
		}
		return out
	}
	return nil
}

func textOf(content any) string {
	if strVal, ok := content.(string); ok {
		return strVal
	}
	var builder strings.Builder
	for _, blk := range blocksOf(content) {
		if rawStrField(blk, "type") == "text" {
			builder.WriteString(rawStrField(blk, "text"))
		}
	}
	return builder.String()
}

func callID(msgIndex, callIndex int, toolName string, rawArgs any) string {
	var argsText string
	switch typed := rawArgs.(type) {
	case string:
		argsText = typed
	case nil:
		argsText = "{}"
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			argsText = "{}"
		} else {
			argsText = string(encoded)
		}
	}
	seed := fmt.Sprintf("%d\x00%d\x00%s\x00%s", msgIndex, callIndex, toolName, argsText)
	sum := sha256.Sum256([]byte(seed))
	return "call_" + hex.EncodeToString(sum[:])[:32]
}

func reasoningOf(msg map[string]any) string {
	var text string
	for _, key := range []string{"reasoning_content", "reasoning"} {
		if text == "" {
			if val, ok := msg[key].(string); ok {
				text = val
			}
		}
	}
	if text == "" {
		if details, ok := msg["reasoning_details"].([]any); ok {
			var builder strings.Builder
			for _, item := range details {
				obj, ok := item.(map[string]any)
				if !ok || obj == nil {
					continue
				}
				if txt, ok := obj["text"].(string); ok {
					builder.WriteString(txt)
				} else if summary, ok := obj["summary"].(string); ok {
					builder.WriteString(summary)
				}
			}
			text = builder.String()
		}
	}
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return text
}

type toolTurnState struct {
	open []string
	kept map[string]bool
}

func qoderMessages(rawMsgs []any, callText string) []map[string]any {
	msgs := make([]map[string]any, 0, len(rawMsgs))
	for _, item := range rawMsgs {
		if obj, ok := item.(map[string]any); ok && obj != nil {
			msgs = append(msgs, obj)
		}
	}
	var out []map[string]any
	names := map[string]string{}
	var seen []map[string]any
	flushSeen := func() {
		if len(seen) > 0 {
			out = append(out, map[string]any{"role": "user", "content": seen})
			seen = nil
		}
	}
	var turn *toolTurnState
	asText := func(txt string, imgs []map[string]any) {
		said := strings.TrimSpace(txt)
		if said == "" && len(imgs) == 0 {
			return
		}
		if len(seen) > 0 {
			seen = append(seen, map[string]any{"type": "text", "text": "\n\n"})
		}
		if said != "" {
			seen = append(seen, map[string]any{"type": "text", "text": said})
		}
		seen = append(seen, imgs...)
	}

	for idx, msg := range msgs {
		role := rawStrField(msg, "role")
		if role == "system" || role == "developer" {
			continue
		}
		if role == "user" {
			turn = nil
		}
		if role != "user" && role != "tool" {
			flushSeen()
		}
		if role == "tool" {
			txt := textOf(msg["content"])
			var imgs []map[string]any
			if parts, ok := msg["content"].([]any); ok {
				for _, item := range parts {
					if part, ok := item.(map[string]any); ok && rawStrField(part, "type") == "image_url" {
						if blk := imageBlock(part); blk != nil {
							imgs = append(imgs, blk)
						}
					}
				}
			}
			named := strings.TrimSpace(strField(msg, "tool_call_id"))
			pick := -1
			if turn != nil {
				if named != "" {
					pick = slices.Index(turn.open, named)
				}
				if pick < 0 {
					for openIdx, openID := range turn.open {
						if !turn.kept[openID] {
							pick = openIdx
							break
						}
					}
				}
			}
			if pick < 0 {
				asText(txt, imgs)
				continue
			}
			answered := turn.open[pick]
			turn.open = slices.Delete(turn.open, pick, pick+1)
			if len(imgs) > 0 {
				label := "tool call " + answered
				toolName := firstNonEmpty(names[answered], strField(msg, "name"))
				if toolName != "" {
					label = fmt.Sprintf("%s (%s)", toolName, label)
				}
				seen = append(seen, map[string]any{"type": "text", "text": fmt.Sprintf("[From the result of %s:]", label)})
				seen = append(seen, imgs...)
				note := "[The tool returned an image; it follows in the next message.]"
				if len(imgs) > 1 {
					note = fmt.Sprintf("[The tool returned %d images; they follow in the next message.]", len(imgs))
				}
				if strings.TrimSpace(txt) != "" {
					txt = txt + "\n\n" + note
				} else {
					txt = txt + note
				}
			}
			out = append(out, map[string]any{
				"role":         "tool",
				"tool_call_id": answered,
				"content":      txt,
			})
			continue
		}
		if role == "assistant" {
			if rawCalls, ok := msg["tool_calls"].([]any); ok && len(rawCalls) > 0 {
				calls := make([]map[string]any, 0, len(rawCalls))
				openIDs := make([]string, 0, len(rawCalls))
				for callIdx, rawCall := range rawCalls {
					callObj, _ := rawCall.(map[string]any)
					fnObj, _ := callObj["function"].(map[string]any)
					fnName := rawStrField(fnObj, "name")
					id := strings.TrimSpace(strField(callObj, "id"))
					if id == "" {
						id = callID(idx, callIdx, fnName, fnObj["arguments"])
					}
					names[id] = fnName
					var argsStr string
					switch rawArgs := fnObj["arguments"].(type) {
					case string:
						argsStr = rawArgs
					case nil:
						argsStr = "{}"
					default:
						encoded, err := json.Marshal(rawArgs)
						if err != nil {
							argsStr = "{}"
						} else {
							argsStr = string(encoded)
						}
					}
					calls = append(calls, map[string]any{
						"id":   id,
						"type": "function",
						"function": map[string]any{
							"name":      fnName,
							"arguments": argsStr,
						},
					})
					openIDs = append(openIDs, id)
				}
				contentText := textOf(msg["content"])
				if contentText == "" {
					contentText = callText
				}
				entry := map[string]any{
					"role":       "assistant",
					"content":    contentText,
					"tool_calls": calls,
				}
				if think := reasoningOf(msg); think != "" {
					entry["reasoning_content"] = think
				}
				out = append(out, entry)
				kept := map[string]bool{}
				for scan := idx + 1; scan < len(msgs) && rawStrField(msgs[scan], "role") == "tool"; scan++ {
					tid := strings.TrimSpace(strField(msgs[scan], "tool_call_id"))
					if slices.Contains(openIDs, tid) {
						kept[tid] = true
					}
				}
				turn = &toolTurnState{open: openIDs, kept: kept}
				continue
			}
		}
		blocks := blocksOf(msg["content"])
		if role == "assistant" {
			filtered := make([]map[string]any, 0, len(blocks))
			for _, blk := range blocks {
				if rawStrField(blk, "type") == "text" {
					filtered = append(filtered, blk)
				}
			}
			blocks = filtered
		}
		if len(blocks) == 0 {
			continue
		}
		if role == "user" && len(seen) > 0 {
			merged := make([]map[string]any, 0, len(seen)+len(blocks))
			merged = append(merged, seen...)
			merged = append(merged, blocks...)
			blocks = merged
			seen = nil
		}
		entry := map[string]any{
			"role":    role,
			"content": blocks,
		}
		if role == "assistant" {
			if think := reasoningOf(msg); think != "" {
				entry["reasoning_content"] = think
			}
		}
		out = append(out, entry)
		turn = nil
	}
	flushSeen()
	return out
}

func cloneShallowMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for key, val := range src {
		dst[key] = val
	}
	return dst
}

func cacheMarked(msgs []map[string]any) []map[string]any {
	out := make([]map[string]any, len(msgs))
	for idx, msg := range msgs {
		copied := cloneShallowMap(msg)
		if rawStrField(copied, "role") == "user" {
			if blocks, ok := copied["content"].([]map[string]any); ok {
				copied["contents"] = blocks
			}
		}
		out[idx] = copied
	}
	if len(out) == 0 {
		return out
	}
	last := out[len(out)-1]
	if rawStrField(last, "role") != "user" {
		return out
	}
	blocks, ok := last["content"].([]map[string]any)
	if !ok || len(blocks) == 0 {
		return out
	}
	markIdx := -1
	for idx := len(blocks) - 1; idx >= 0; idx-- {
		blkType := rawStrField(blocks[idx], "type")
		if blkType != "" && !uncachedBlockTypes[blkType] {
			markIdx = idx
			break
		}
	}
	if markIdx < 0 {
		return out
	}
	copiedBlocks := make([]map[string]any, len(blocks))
	copy(copiedBlocks, blocks)
	marked := cloneShallowMap(copiedBlocks[markIdx])
	marked["cache_control"] = map[string]any{"type": "ephemeral"}
	copiedBlocks[markIdx] = marked
	last["content"] = copiedBlocks
	last["contents"] = copiedBlocks
	out[len(out)-1] = last
	return out
}

func windowFor(chat map[string]any, entry ModelEntry, replyTokens int) int64 {
	base := entry.DefaultWindow
	if base == 0 {
		base = positiveInt64(entry.Config["max_input_tokens"])
	}
	if base == 0 {
		base = entry.Context
	}
	if len(entry.Windows) == 0 {
		return entry.Context
	}
	msgs := chat["messages"]
	if msgs == nil {
		msgs = []any{}
	}
	tools := chat["tools"]
	if tools == nil {
		tools = []any{}
	}
	encoded, _ := json.Marshal(struct {
		M any `json:"m"`
		T any `json:"t"`
	}{M: msgs, T: tools})
	reply := int64(replyTokens)
	if reply < 0 {
		reply = 0
	}
	need := int64(math.Ceil(float64(len(encoded))/3.0)) + reply
	if base > 0 && need <= base {
		return base
	}
	for _, candidate := range entry.Windows {
		if candidate >= need {
			return candidate
		}
	}
	return entry.Windows[len(entry.Windows)-1]
}

func namedSession(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || magpieAutoSessRe.MatchString(trimmed) {
		return ""
	}
	if len(trimmed) > 128 {
		return trimmed[:128]
	}
	return trimmed
}

func sessionOf(chat map[string]any, named string) string {
	var seed string
	switch {
	case named != "":
		seed = "session\x00" + named
	case strings.TrimSpace(strField(chat, "prompt_cache_key")) != "":
		seed = "key\x00" + rawStrField(chat, "prompt_cache_key")
	case strings.TrimSpace(strField(chat, "user")) != "":
		seed = "user\x00" + rawStrField(chat, "user")
	default:
		var sysTexts []string
		var firstUser any = ""
		foundUser := false
		if rawMsgs, ok := chat["messages"].([]any); ok {
			for _, item := range rawMsgs {
				msg, ok := item.(map[string]any)
				if !ok || msg == nil {
					continue
				}
				role := rawStrField(msg, "role")
				if role == "system" || role == "developer" {
					sysTexts = append(sysTexts, textOf(msg["content"]))
				} else if role == "user" && !foundUser {
					foundUser = true
					if msg["content"] != nil {
						firstUser = msg["content"]
					}
				}
			}
		}
		if sysTexts == nil {
			sysTexts = []string{}
		}
		encoded, _ := json.Marshal([]any{sysTexts, firstUser})
		seed = "chat\x00" + string(encoded)
	}
	sum := sha256.Sum256([]byte(seed))
	digest := hex.EncodeToString(sum[:])
	return digest[0:8] + "-" + digest[8:12] + "-" + digest[12:16] + "-" + digest[16:20] + "-" + digest[20:32]
}

func taskName(prompt string) string {
	runes := []rune(prompt)
	if len(runes) > 10 {
		return string(runes[:10])
	}
	return prompt
}

func (state *providerRuntimeState) taskOf(sid string, chat map[string]any, nowMs int64) *taskRecord {
	rawMsgs, _ := chat["messages"].([]any)
	lastUserIdx := -1
	var lastUserContent any
	for idx := len(rawMsgs) - 1; idx >= 0; idx-- {
		if msg, ok := rawMsgs[idx].(map[string]any); ok && rawStrField(msg, "role") == "user" {
			lastUserIdx = idx
			lastUserContent = msg["content"]
			break
		}
	}
	keyBytes, _ := json.Marshal([]any{sid, lastUserIdx, lastUserContent})
	sum := sha256.Sum256(keyBytes)
	key := hex.EncodeToString(sum[:])

	state.stateMu.Lock()
	defer state.stateMu.Unlock()

	for len(state.taskOrder) > 0 {
		oldestKey := state.taskOrder[0]
		oldest := state.tasks[oldestKey]
		if oldest != nil && nowMs-oldest.Used <= taskIdleMs {
			break
		}
		delete(state.tasks, oldestKey)
		state.taskOrder = state.taskOrder[1:]
	}
	rec := state.tasks[key]
	if rec != nil && nowMs-rec.Used > taskIdleMs {
		rec = nil
	}
	if existingIdx := slices.Index(state.taskOrder, key); existingIdx >= 0 {
		state.taskOrder = slices.Delete(state.taskOrder, existingIdx, existingIdx+1)
	}
	if rec == nil {
		promptText := ""
		if lastUserIdx >= 0 {
			promptText = textOf(lastUserContent)
		}
		rec = &taskRecord{
			Key:      key,
			ID:       randomUUID(),
			Begin:    nowMs,
			Answered: false,
			Name:     taskName(promptText),
		}
	}
	rec.Used = nowMs
	state.tasks[key] = rec
	state.taskOrder = append(state.taskOrder, key)
	for len(state.taskOrder) > taskMaxEntries {
		evictKey := state.taskOrder[0]
		delete(state.tasks, evictKey)
		state.taskOrder = state.taskOrder[1:]
	}
	return rec
}

func (state *providerRuntimeState) markTaskAnswered(rec *taskRecord) {
	if rec == nil {
		return
	}
	state.stateMu.Lock()
	rec.Answered = true
	state.stateMu.Unlock()
}

func (state *providerRuntimeState) taskSnapshot(rec *taskRecord) (id string, name string, begin int64, stage string) {
	state.stateMu.Lock()
	defer state.stateMu.Unlock()
	stage = "start"
	if rec.Answered {
		stage = "processing"
	}
	return rec.ID, rec.Name, rec.Begin, stage
}

func deepCloneJSON(raw any) any {
	switch typed := raw.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			out[key] = deepCloneJSON(val)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for idx, val := range typed {
			out[idx] = deepCloneJSON(val)
		}
		return out
	default:
		return raw
	}
}

func typedUnions(schema any) any {
	root, ok := deepCloneJSON(schema).(map[string]any)
	if !ok || root == nil {
		return schema
	}
	target := func(ref string) map[string]any {
		if !strings.HasPrefix(ref, "#/") {
			return nil
		}
		var cur any = root
		for _, segment := range strings.Split(ref[2:], "/") {
			unescaped := strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
			obj, isObj := cur.(map[string]any)
			if !isObj || obj == nil {
				return nil
			}
			cur = obj[unescaped]
		}
		obj, _ := cur.(map[string]any)
		return obj
	}
	var isObjectBranch func(branch any, seen []string) bool
	isObjectBranch = func(branch any, seen []string) bool {
		obj, isObj := branch.(map[string]any)
		if !isObj || obj == nil {
			return false
		}
		if rawType, hasType := obj["type"]; hasType {
			strType, _ := rawType.(string)
			return strType == "object"
		}
		ref, hasRef := obj["$ref"].(string)
		if !hasRef || slices.Contains(seen, ref) || len(seen) > 16 {
			return false
		}
		return isObjectBranch(target(ref), append(slices.Clone(seen), ref))
	}
	var walk func(node any, depth int)
	walk = func(node any, depth int) {
		if depth > 64 {
			return
		}
		obj, isObj := node.(map[string]any)
		if !isObj || obj == nil {
			return
		}
		if _, hasType := obj["type"]; !hasType {
			if oneOf, isSlice := obj["oneOf"].([]any); isSlice && len(oneOf) > 0 {
				allObjects := true
				for _, branch := range oneOf {
					if !isObjectBranch(branch, nil) {
						allObjects = false
						break
					}
				}
				if allObjects {
					obj["type"] = "object"
				}
			}
		}
		for _, key := range schemaSubOne {
			walk(obj[key], depth+1)
		}
		for _, key := range schemaSubList {
			if list, isSlice := obj[key].([]any); isSlice {
				for _, item := range list {
					walk(item, depth+1)
				}
			}
		}
		for _, key := range schemaSubMap {
			if subMap, isMap := obj[key].(map[string]any); isMap {
				for _, val := range subMap {
					walk(val, depth+1)
				}
			}
		}
	}
	walk(root, 0)
	return root
}

func buildModelConfig(entry ModelEntry, thinking bool) (map[string]any, error) {
	raw := entry.Config
	if raw == nil {
		raw = map[string]any{}
	}
	key := firstNonEmpty(strField(raw, "key"), entry.Key)
	if isReservedModelKey(key) || key != entry.Key {
		return nil, fmt.Errorf(`no Qoder model key for "%s"`, entry.Key)
	}
	maxInput := positiveInt64(raw["max_input_tokens"])
	if maxInput == 0 {
		maxInput = 200_000
	}
	formatVal := firstNonEmpty(rawStrField(raw, "format"), "openai")
	sourceVal := firstNonEmpty(strField(raw, "source"), "system")
	cfg := map[string]any{
		"key":              key,
		"display_name":     firstNonEmpty(strField(raw, "display_name"), key),
		"format":           formatVal,
		"is_vl":            boolVal(raw["is_vl"]),
		"is_reasoning":     boolVal(raw["is_reasoning"]) && thinking,
		"api_key":          "",
		"url":              "",
		"source":           sourceVal,
		"max_input_tokens": maxInput,
	}
	if outer := raw["outer_provider"]; outer != nil && outer != "" && outer != false {
		cfg["outer_provider"] = outer
	} else {
		cfg["model"] = ""
	}
	return cfg, nil
}

func (state *providerRuntimeState) outputCap(entry ModelEntry) int {
	state.stateMu.Lock()
	learned := state.learnedCaps[entry.Key]
	state.stateMu.Unlock()
	if learned > 0 {
		return learned
	}
	if val := positiveInt64(entry.Config["max_output_tokens"]); val > 0 {
		return int(val)
	}
	if val := positiveInt64(entry.Config["maxOutputTokens"]); val > 0 {
		return int(val)
	}
	return defaultOutputCaps[entry.Key]
}

func (state *providerRuntimeState) reclamp(fail *qoderFailure, body map[string]any, entry ModelEntry) bool {
	if fail == nil || fail.MaxTokens <= 0 {
		return false
	}
	params, ok := body["parameters"].(map[string]any)
	if !ok || params == nil {
		return false
	}
	curMax := int(positiveInt64(params["max_tokens"]))
	if curMax <= fail.MaxTokens {
		return false
	}
	if entry.Key != "" {
		state.stateMu.Lock()
		state.learnedCaps[entry.Key] = fail.MaxTokens
		state.stateMu.Unlock()
	}
	params["max_tokens"] = fail.MaxTokens
	return true
}

func extractFunctionTools(chat map[string]any) []map[string]any {
	if rawStrField(chat, "tool_choice") == "none" {
		return nil
	}
	rawTools, ok := chat["tools"].([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, item := range rawTools {
		toolObj, ok := item.(map[string]any)
		if !ok || toolObj == nil || rawStrField(toolObj, "type") != "function" {
			continue
		}
		fnObj, _ := toolObj["function"].(map[string]any)
		if rawStrField(fnObj, "name") == "" {
			continue
		}
		out = append(out, toolObj)
	}
	return out
}

func (owner *module) qoderBody(state *providerRuntimeState, chat map[string]any, entry ModelEntry, named string, task *taskRecord, site Site) (map[string]any, error) {
	rawMsgs, _ := chat["messages"].([]any)
	var sysParts []string
	for _, item := range rawMsgs {
		msg, ok := item.(map[string]any)
		if !ok || msg == nil {
			continue
		}
		role := rawStrField(msg, "role")
		if role == "system" || role == "developer" {
			if txt := textOf(msg["content"]); txt != "" {
				sysParts = append(sysParts, txt)
			}
		}
	}
	givenSystem := strings.Join(sysParts, "\n\n")
	adaptedSystem := harnessComma(asAgentSDK(givenSystem))
	tools := extractFunctionTools(chat)
	baseSystem := site.SystemPrompt
	if baseSystem == "" {
		baseSystem = qoderSystemPrompt
	}
	sysText := baseSystem
	if adaptedSystem != "" {
		sysText = baseSystem + "\n\n" + adaptedSystem
	}
	if rawStrField(chat, "tool_choice") == "required" && len(tools) > 0 {
		sysText += "\nYou must call an available function in this response."
	}
	sysBlock := map[string]any{"type": "text", "text": sysText}

	budget := int(positiveInt64(chat["max_completion_tokens"]))
	if budget == 0 {
		budget = int(positiveInt64(chat["max_tokens"]))
	}
	thinking, effort := effortFor(rawStrField(chat, "reasoning_effort"), entry, budget)
	asked := budget
	if asked == 0 {
		asked = 32000
	}
	capTokens := state.outputCap(entry)
	maxTokens := asked
	if capTokens > 0 && asked > capTokens {
		maxTokens = capTokens
	}
	params := map[string]any{
		"enable_thinking": thinking,
		"max_tokens":      maxTokens,
	}
	if effort != "" {
		params["reasoning_effort"] = effort
	}
	if win := windowFor(chat, entry, maxTokens); win > 0 {
		params["context_length"] = win
	}

	requestID := randomUUID()
	sid := sessionOf(chat, named)
	if task == nil {
		task = state.taskOf(sid, chat, owner.nowMs())
	}
	taskID, taskTitle, taskBegin, taskStage := state.taskSnapshot(task)

	modelCfg, err := buildModelConfig(entry, thinking)
	if err != nil {
		return nil, err
	}
	cosyVer := site.CosyVersion
	if cosyVer == "" {
		cosyVer = cosyVersionDefault
	}
	product := site.BusinessProduct
	if product == "" {
		product = "app"
	}
	sessionType := site.SessionType
	if sessionType == "" {
		sessionType = "app"
	}
	business := map[string]any{
		"product":  product,
		"version":  cosyVer,
		"type":     "agent",
		"id":       taskID,
		"name":     taskTitle,
		"begin_at": taskBegin,
		"stage":    taskStage,
	}
	if site.BusinessModelKey {
		business["model_key"] = entry.Key
	}

	callText := ""
	if kimiModel(entry) {
		callText = " "
	}
	converted := cacheMarked(qoderMessages(rawMsgs, callText))
	allMessages := make([]map[string]any, 0, len(converted)+1)
	allMessages = append(allMessages, map[string]any{"role": "system", "content": []map[string]any{sysBlock}})
	allMessages = append(allMessages, converted...)

	body := map[string]any{
		"request_id":     requestID,
		"request_set_id": taskID,
		"chat_record_id": requestID,
		"session_id":     sid,
		"parameters":     params,
		"business":       business,
		"agent_id":       "agent_common",
		"task_id":        "common",
		"session_type":   sessionType,
		"model_config":   modelCfg,
		"system":         []map[string]any{sysBlock},
		"messages":       allMessages,
	}
	if len(tools) > 0 {
		outTools := make([]map[string]any, 0, len(tools))
		for _, toolObj := range tools {
			fnObj, _ := toolObj["function"].(map[string]any)
			paramSchema := typedUnions(fnObj["parameters"])
			if paramSchema == nil {
				paramSchema = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			outTools = append(outTools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        rawStrField(fnObj, "name"),
					"description": rawStrField(fnObj, "description"),
					"parameters":  paramSchema,
				},
			})
		}
		body["tools"] = outTools
	}
	return body, nil
}
