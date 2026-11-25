package qoder

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

const (
	callOpenTag     = "<tool_call>"
	callCloseTag    = "</tool_call>"
	jsonCallKey     = `{"tool_calls"`
	cutShortMsg     = "the model's tool call, written as text, was cut short; it is not sent on as the answer"
	unreadableMsg   = "the model's tool call, written as text, can't be read; it is not sent on as the answer"
	namelessCallMsg = "the model's native tool call has no name; it is not sent on as a tool"
	queuedCode      = "10605"
	busyWaitSeconds = 30
)

var (
	funcBlockRe    = regexp.MustCompile(`<function=([^>]+)>([\s\S]*?)</function>`)
	paramBlockRe   = regexp.MustCompile(`<parameter=([^>]+)>([\s\S]*?)</parameter>`)
	xmlAtRe        = regexp.MustCompile(`<(?:antml:)?(?:function_calls>|invoke\s+name\s*=|function=)`)
	xmlHoldRe      = regexp.MustCompile(`<(?:antml:)?(?:invoke\s[^<>]{0,300}|function=[^<>]{0,300})$`)
	xmlNextRe      = regexp.MustCompile(`<(?:antml:)?invoke\s+name\s*=|<function=`)
	xmlParamRe     = regexp.MustCompile(`<(?:antml:)?parameter(?:\s+name\s*=\s*"([^"]+)"|=([^\s>]+))\s*>`)
	xmlParamEndRe  = regexp.MustCompile(`</(?:antml:)?parameter>`)
	xmlLeftRe      = regexp.MustCompile(`^(?:\s+|</(?:antml:)?(?:function_calls|invoke|function)>)+`)
	xmlWrapOpenRe  = regexp.MustCompile(`^<(?:antml:)?function_calls>`)
	xmlInvokeRe    = regexp.MustCompile(`^<(?:antml:)?invoke\s+name\s*=\s*"([^"<>]+)"\s*>`)
	xmlInvokeEndRe = regexp.MustCompile(`</(?:antml:)?invoke\s*>`)
	xmlFuncOpenRe  = regexp.MustCompile(`^<function=([^\s<>]+?)\s*>`)
	xmlFuncEndRe   = regexp.MustCompile(`</function\s*>`)
	xmlPartialRe   = regexp.MustCompile(`^<(?:antml:)?(?:invoke\s|function=)[^<>]{0,300}$`)
	xmlHasParamRe  = regexp.MustCompile(`<(?:antml:)?parameter\b`)
	xmlOpenerRe    = regexp.MustCompile(`^<[^<>]*>?`)
	jsonAtAllRe    = regexp.MustCompile(`(^|\n)[ \t]*\{\s*"tool_calls"\s*:`)
	wsAllRe        = regexp.MustCompile(`\s+`)
	leadEdgeNLRe   = regexp.MustCompile(`^\r?\n|\r?\n$`)

	maxTokensRangeRe = regexp.MustCompile(`(?i)\brange of max_tokens should be \[\s*(\d+)\s*,\s*(\d+)\s*\]`)
	upstreamFailedRe = regexp.MustCompile(`(?i)\berror in upstream response\b`)
	lingmaLapsedRe   = regexp.MustCompile(`(?i)"code"\s*:\s*"?105"?|Login (?:expired|timeout)`)
	lingmaNotHereRe  = regexp.MustCompile(`(?i)model_not_found|Model not exist`)

	xmlStarts = []string{"<invoke ", "<antml:invoke ", "<function="}
	xmlMarks  = []string{"<function_calls>", "<antml:function_calls>", "<invoke ", "<antml:invoke ", "<function="}
	xmlCloses = []string{"</function_calls>", "</antml:function_calls>", "</invoke>", "</antml:invoke>", "</function>"}

	quotaCodes = map[string]bool{
		"110": true, "112": true, "113": true, "114": true, "115": true,
		"116": true, "117": true, "118": true, "119": true, "122": true,
	}
	requestCodes = map[string]bool{
		"406": true, "416": true, "430": true, "80411": true, "80412": true,
		"100400": true, "100401": true, "100403": true,
	}
)

type queueInfo struct {
	IsQueued          *bool
	ServiceAvailable  *bool
	ModelKey          string
	QueueType         string
	RetryAfterSeconds *float64
	QueueCount        *float64
	SubCode           string
}

type qoderFailure struct {
	Status     int
	Message    string
	SignIn     string
	HasSignIn  bool
	RetryAfter int
	Lapsed     bool
	Nameless   bool
	MaxTokens  int
	Queue      *queueInfo
}

func boolPtrField(obj map[string]any, key string) *bool {
	raw, ok := obj[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case bool:
		val := typed
		return &val
	case string:
		if typed == "true" {
			val := true
			return &val
		}
		if typed == "false" {
			val := false
			return &val
		}
	}
	return nil
}

func numPtrField(obj map[string]any, key string) *float64 {
	raw, ok := obj[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return nil
		}
		val := typed
		return &val
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		parsed, err := strconv.ParseFloat(trimmed, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return nil
		}
		return &parsed
	}
	return nil
}

func extractQueueObj(obj map[string]any) *queueInfo {
	if obj == nil {
		return nil
	}
	_, hasQueued := obj["isQueued"]
	_, hasAvail := obj["serviceAvailable"]
	_, hasRetry := obj["retryAfterSeconds"]
	if !hasQueued && !hasAvail && !hasRetry {
		return nil
	}
	return &queueInfo{
		IsQueued:          boolPtrField(obj, "isQueued"),
		ServiceAvailable:  boolPtrField(obj, "serviceAvailable"),
		ModelKey:          rawStrField(obj, "modelKey"),
		QueueType:         rawStrField(obj, "queueType"),
		RetryAfterSeconds: numPtrField(obj, "retryAfterSeconds"),
		QueueCount:        numPtrField(obj, "queueCount"),
		SubCode:           rawStrField(obj, "subCode"),
	}
}

func classifyFailure(status int, text string) *qoderFailure {
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	msg := strings.TrimSpace(text)
	code := ""
	var qInfo *queueInfo
	var doc map[string]any
	if json.Unmarshal([]byte(msg), &doc) == nil && doc != nil {
		outerCode := strField(doc, "code")
		if outerCode == "" || outerCode == "401" || outerCode == "403" {
			if innerStr, ok := doc["message"].(string); ok {
				var inner map[string]any
				if json.Unmarshal([]byte(strings.TrimSpace(innerStr)), &inner) == nil && inner != nil && strField(inner, "code") == queuedCode {
					doc = inner
				}
			}
		}
		code = strField(doc, "code")
		if mVal := rawStrField(doc, "message"); mVal != "" {
			msg = mVal
		}
		var detailsObj map[string]any
		switch rawDetails := doc["details"].(type) {
		case string:
			_ = json.Unmarshal([]byte(strings.TrimSpace(rawDetails)), &detailsObj)
		case map[string]any:
			detailsObj = rawDetails
		}
		if errObj, ok := detailsObj["error"].(map[string]any); ok {
			if errMsg := rawStrField(errObj, "message"); errMsg != "" {
				msg += ": " + errMsg
			}
		}
		if rawQ, ok := doc["queue"].(map[string]any); ok && rawQ != nil {
			qInfo = &queueInfo{
				IsQueued:          boolPtrField(rawQ, "isQueued"),
				ServiceAvailable:  boolPtrField(rawQ, "serviceAvailable"),
				ModelKey:          rawStrField(rawQ, "modelKey"),
				QueueType:         rawStrField(rawQ, "queueType"),
				RetryAfterSeconds: numPtrField(rawQ, "retryAfterSeconds"),
				QueueCount:        numPtrField(rawQ, "queueCount"),
				SubCode:           rawStrField(rawQ, "subCode"),
			}
		}
		if qInfo == nil {
			if mStr, ok := doc["message"].(string); ok && strings.HasPrefix(strings.TrimSpace(mStr), "{") {
				var qMap map[string]any
				if json.Unmarshal([]byte(strings.TrimSpace(mStr)), &qMap) == nil && qMap != nil {
					qInfo = extractQueueObj(qMap)
				}
			}
		}
	}
	if msg == "" {
		if statusText := http.StatusText(status); statusText != "" {
			msg = statusText
		} else {
			msg = fmt.Sprintf("HTTP %d", status)
		}
	}
	isQueued := qInfo != nil && qInfo.IsQueued != nil && *qInfo.IsQueued
	svcUnavailable := qInfo != nil && qInfo.ServiceAvailable != nil && !*qInfo.ServiceAvailable
	if code == queuedCode || isQueued || svcUnavailable {
		wait := busyWaitSeconds
		if qInfo != nil && qInfo.RetryAfterSeconds != nil && *qInfo.RetryAfterSeconds > 0 {
			wait = int(math.Ceil(*qInfo.RetryAfterSeconds))
		}
		modelPart := ""
		if qInfo != nil && qInfo.ModelKey != "" {
			modelPart = " for " + qInfo.ModelKey
		}
		aheadPart := ""
		if qInfo != nil && qInfo.QueueCount != nil && *qInfo.QueueCount > 0 {
			aheadPart = fmt.Sprintf(", %d ahead", int(math.Floor(*qInfo.QueueCount)))
		}
		tierPart := ""
		if qInfo != nil && qInfo.QueueType != "" {
			tierPart = fmt.Sprintf(" (this plan's queue: %s%s)", qInfo.QueueType, aheadPart)
		} else if aheadPart != "" {
			tierPart = fmt.Sprintf(" (%s)", aheadPart[2:])
		}
		what := "is busy"
		if isQueued {
			what = "queued the request"
		}
		if qInfo == nil {
			qInfo = &queueInfo{}
		}
		return &qoderFailure{
			Status:     http.StatusServiceUnavailable,
			Message:    fmt.Sprintf("Qoder %s%s%s; try again in %ds", what, modelPart, tierPart, wait),
			RetryAfter: wait,
			SignIn:     "",
			HasSignIn:  true,
			Queue:      qInfo,
		}
	}
	if quotaCodes[code] {
		return &qoderFailure{Status: http.StatusTooManyRequests, Message: "usage limit reached: " + msg, SignIn: "", HasSignIn: true}
	}
	if requestCodes[code] {
		return &qoderFailure{Status: http.StatusBadRequest, Message: fmt.Sprintf("Qoder refused the request (%s): %s", code, msg), SignIn: "", HasSignIn: true}
	}
	if match := maxTokensRangeRe.FindStringSubmatch(msg); len(match) == 3 {
		capNum, _ := strconv.Atoi(match[2])
		return &qoderFailure{
			Status:    http.StatusBadRequest,
			Message:   "Qoder refused the request's max_tokens: " + msg,
			SignIn:    "",
			HasSignIn: true,
			MaxTokens: capNum,
		}
	}
	if upstreamFailedRe.MatchString(msg) {
		return &qoderFailure{Status: http.StatusBadGateway, Message: msg, SignIn: "", HasSignIn: true}
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		said := ""
		if msg != "" && msg != http.StatusText(status) {
			said = fmt.Sprintf(" (Qoder said %d: %s)", status, truncateText(msg, 300))
		}
		return &qoderFailure{
			Status:  http.StatusUnauthorized,
			Message: "the sign-in lapsed — sign in again" + said,
			Lapsed:  true,
		}
	}
	if status == http.StatusTooManyRequests || strings.Contains(strings.ToLower(msg), "quota") {
		raw := strings.TrimSpace(text)
		seen := fmt.Sprintf("Qoder replied HTTP %d", status)
		if code != "" {
			seen += ", code " + code
		}
		bodyPart := ""
		if raw != "" && raw != msg {
			bodyPart = ": " + truncateText(raw, 400)
		}
		return &qoderFailure{
			Status:  http.StatusTooManyRequests,
			Message: fmt.Sprintf("usage limit reached: %s (%s%s)", msg, seen, bodyPart),
		}
	}
	return &qoderFailure{Status: status, Message: msg}
}

type parsedToolCall struct {
	Name string
	Args string
}

type splitPiece struct {
	Text  string
	Call  *parsedToolCall
	Error string
}

func toolProps(tools []map[string]any, name string) map[string]any {
	for _, toolObj := range tools {
		fnObj, _ := toolObj["function"].(map[string]any)
		if fnObj == nil {
			fnObj = toolObj
		}
		if rawStrField(fnObj, "name") == name {
			params, _ := fnObj["parameters"].(map[string]any)
			props, _ := params["properties"].(map[string]any)
			return props
		}
	}
	return nil
}

func hasDeclaredTool(tools []map[string]any, name string) bool {
	for _, toolObj := range tools {
		fnObj, _ := toolObj["function"].(map[string]any)
		if fnObj == nil {
			fnObj = toolObj
		}
		if rawStrField(fnObj, "name") == name {
			return true
		}
	}
	return false
}

func typedParamValue(rawText string, prop any) any {
	if propMap, ok := prop.(map[string]any); ok && rawStrField(propMap, "type") == "string" {
		return rawText
	}
	var parsed any
	if json.Unmarshal([]byte(strings.TrimSpace(rawText)), &parsed) == nil {
		return parsed
	}
	return rawText
}

func parseCallBlock(block string, tools []map[string]any) *parsedToolCall {
	var obj map[string]any
	if json.Unmarshal([]byte(strings.TrimSpace(block)), &obj) == nil && obj != nil {
		name := strings.TrimSpace(rawStrField(obj, "name"))
		argsMap, isMap := obj["arguments"].(map[string]any)
		if name != "" && isMap && argsMap != nil {
			encoded, err := json.Marshal(argsMap)
			if err == nil {
				return &parsedToolCall{Name: name, Args: string(encoded)}
			}
		}
	}
	match := funcBlockRe.FindStringSubmatch(block)
	if len(match) != 3 || strings.TrimSpace(match[1]) == "" {
		return nil
	}
	name := strings.TrimSpace(match[1])
	props := toolProps(tools, name)
	args := map[string]any{}
	for _, paramMatch := range paramBlockRe.FindAllStringSubmatch(match[2], -1) {
		if len(paramMatch) != 3 {
			continue
		}
		key := strings.TrimSpace(paramMatch[1])
		if key == "" {
			continue
		}
		var prop any
		if props != nil {
			prop = props[key]
		}
		args[key] = typedParamValue(strings.TrimSpace(paramMatch[2]), prop)
	}
	encoded, _ := json.Marshal(args)
	return &parsedToolCall{Name: name, Args: string(encoded)}
}

func xmlArgs(body string, props map[string]any) (map[string]any, bool) {
	opens := xmlParamRe.FindAllStringSubmatchIndex(body, -1)
	args := map[string]any{}
	closed := len(opens) > 0
	for idx, loc := range opens {
		valueStart := loc[1]
		valueEnd := len(body)
		if idx+1 < len(opens) {
			valueEnd = opens[idx+1][0]
		}
		val := body[valueStart:valueEnd]
		if closeLoc := xmlParamEndRe.FindStringIndex(val); closeLoc != nil {
			val = val[:closeLoc[0]]
		} else {
			closed = false
		}
		var paramKey string
		if loc[2] >= 0 && loc[3] >= 0 {
			paramKey = body[loc[2]:loc[3]]
		} else if loc[4] >= 0 && loc[5] >= 0 {
			paramKey = body[loc[4]:loc[5]]
		}
		var prop any
		if props != nil {
			prop = props[paramKey]
		}
		trimmedVal := leadEdgeNLRe.ReplaceAllString(val, "")
		args[paramKey] = typedParamValue(trimmedVal, prop)
	}
	return args, closed
}

type xmlCallResult struct {
	Wait  bool
	Text  bool
	Error string
	Call  *parsedToolCall
	Rest  string
}

func parseXMLCall(input string, atEnd bool, tools []map[string]any) xmlCallResult {
	cur := input
	if wrap := xmlWrapOpenRe.FindString(cur); wrap != "" {
		rest := strings.TrimLeft(cur[len(wrap):], " \t\r\n")
		if !atEnd && (rest == "" || anyPrefixOf(xmlStarts, rest)) {
			return xmlCallResult{Wait: true}
		}
		cur = rest
	}
	var match []string
	var closeRe *regexp.Regexp
	if inv := xmlInvokeRe.FindStringSubmatch(cur); len(inv) == 2 {
		match = inv
		closeRe = xmlInvokeEndRe
	} else if fn := xmlFuncOpenRe.FindStringSubmatch(cur); len(fn) == 2 {
		match = fn
		closeRe = xmlFuncEndRe
	}
	if match == nil {
		if !atEnd && (anyPrefixOf(xmlStarts, cur) || xmlPartialRe.MatchString(cur)) {
			return xmlCallResult{Wait: true}
		}
		return xmlCallResult{Text: true}
	}
	name := strings.TrimSpace(match[1])
	if !hasDeclaredTool(tools, name) {
		return xmlCallResult{Text: true}
	}
	body := cur[len(match[0]):]
	closeLoc := closeRe.FindStringIndex(body)
	nextLoc := xmlNextRe.FindStringIndex(body)
	stop := -1
	if closeLoc != nil {
		stop = closeLoc[0]
	}
	if nextLoc != nil && (stop < 0 || nextLoc[0] < stop) {
		stop = nextLoc[0]
	}
	if stop < 0 {
		if !atEnd {
			return xmlCallResult{Wait: true}
		}
		stop = len(body)
	}
	whole := closeLoc != nil && closeLoc[0] == stop
	argsMap, closed := xmlArgs(body[:stop], toolProps(tools, name))
	if !whole && !closed {
		if !xmlHasParamRe.MatchString(body[:stop]) {
			return xmlCallResult{Text: true}
		}
		return xmlCallResult{Error: cutShortMsg}
	}
	encoded, _ := json.Marshal(argsMap)
	rest := body[stop:]
	if whole {
		rest = body[closeLoc[1]:]
	}
	return xmlCallResult{
		Call: &parsedToolCall{Name: name, Args: string(encoded)},
		Rest: rest,
	}
}

func anyPrefixOf(targets []string, candidate string) bool {
	for _, target := range targets {
		if strings.HasPrefix(target, candidate) {
			return true
		}
	}
	return false
}

func isQuoted(text string) bool {
	if strings.Count(text, "```")%2 == 1 {
		return true
	}
	lastNL := strings.LastIndexByte(text, '\n')
	line := text[lastNL+1:]
	line = strings.ReplaceAll(line, "```", "")
	return strings.Count(line, "`")%2 == 1
}

func isCallish(body string) bool {
	return strings.HasPrefix(strings.TrimLeft(body, " \t\r\n"), "{") || strings.Contains(body, "<function=")
}

func jsonHoldLen(comb string, atLineStart bool) int {
	nlIdx := strings.LastIndexByte(comb, '\n')
	if nlIdx < 0 && !atLineStart {
		return 0
	}
	line := comb[nlIdx+1:]
	compact := wsAllRe.ReplaceAllString(line, "")
	if !strings.HasPrefix(jsonCallKey, compact) {
		return 0
	}
	return len(line)
}

func jsonObjectEnd(input string) int {
	depth := 0
	inStr := false
	for idx := 0; idx < len(input); idx++ {
		ch := input[idx]
		if inStr {
			if ch == '\\' {
				idx++
			} else if ch == '"' {
				inStr = false
			}
		} else if ch == '"' {
			inStr = true
		} else if ch == '{' || ch == '[' {
			depth++
		} else if ch == '}' || ch == ']' {
			depth--
			if depth == 0 {
				return idx + 1
			}
		}
	}
	return -1
}

type jsonCallsStatus int

const (
	jsonCallsNone jsonCallsStatus = iota
	jsonCallsInvalid
	jsonCallsOK
)

func parseJSONCalls(raw string) ([]parsedToolCall, jsonCallsStatus) {
	var obj map[string]any
	if json.Unmarshal([]byte(raw), &obj) != nil || obj == nil {
		return nil, jsonCallsInvalid
	}
	list, isSlice := obj["tool_calls"].([]any)
	if !isSlice || len(list) == 0 {
		return nil, jsonCallsNone
	}
	out := make([]parsedToolCall, 0, len(list))
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok || entry == nil {
			return nil, jsonCallsInvalid
		}
		fnObj, _ := entry["function"].(map[string]any)
		if fnObj == nil {
			fnObj = entry
		}
		argsVal := fnObj["arguments"]
		if argsStr, isStr := argsVal.(string); isStr {
			var decoded any
			if json.Unmarshal([]byte(argsStr), &decoded) != nil {
				return nil, jsonCallsInvalid
			}
			argsVal = decoded
		}
		argsMap, isMap := argsVal.(map[string]any)
		name := strings.TrimSpace(rawStrField(fnObj, "name"))
		if name == "" || !isMap || argsMap == nil {
			return nil, jsonCallsInvalid
		}
		encoded, err := json.Marshal(argsMap)
		if err != nil {
			return nil, jsonCallsInvalid
		}
		out = append(out, parsedToolCall{Name: name, Args: string(encoded)})
	}
	return out, jsonCallsOK
}

type Splitter struct {
	tools      []map[string]any
	textBuf    string
	callBuf    string
	inCall     bool
	sawTool    bool
	afterBare  bool
	sent       string
	hasJSONBuf bool
	jsonBuf    string
}

func newSplitter(tools []map[string]any) *Splitter {
	declared := make([]map[string]any, 0, len(tools))
	for _, toolObj := range tools {
		fnObj, _ := toolObj["function"].(map[string]any)
		if fnObj == nil {
			fnObj = toolObj
		}
		if rawStrField(fnObj, "name") != "" {
			declared = append(declared, toolObj)
		}
	}
	return &Splitter{tools: declared}
}

func (sp *Splitter) pushText(out *[]splitPiece, text string) {
	sp.sent += text
	*out = append(*out, splitPiece{Text: text})
}

func (sp *Splitter) handleBare(out *[]splitPiece, comb string, atIdx int, atEnd bool) (rest string, hold bool, failed bool) {
	head := comb[:atIdx]
	at := comb[atIdx:]
	opener := xmlOpenerRe.FindString(at)
	if isQuoted(sp.sent + head) {
		sp.pushText(out, head+opener)
		return at[len(opener):], false, false
	}
	res := parseXMLCall(at, atEnd, sp.tools)
	if res.Wait {
		if head != "" {
			sp.pushText(out, head)
		}
		sp.textBuf = at
		return "", true, false
	}
	if res.Text {
		sp.pushText(out, head+opener)
		return at[len(opener):], false, false
	}
	if head != "" {
		sp.pushText(out, head)
	}
	if res.Error != "" {
		*out = append(*out, splitPiece{Error: res.Error})
		return "", false, true
	}
	sp.sawTool = true
	*out = append(*out, splitPiece{Call: res.Call})
	sp.afterBare = true
	return res.Rest, false, false
}

func (sp *Splitter) consumeLeft(comb string, atEnd bool) (string, bool) {
	rest := xmlLeftRe.ReplaceAllString(comb, "")
	if !atEnd {
		if rest == "" {
			return "", true
		}
		for _, closeTag := range xmlCloses {
			if len(closeTag) > len(rest) && strings.HasPrefix(closeTag, rest) {
				return "", true
			}
		}
	}
	sp.afterBare = false
	return rest, false
}

func (sp *Splitter) handleJSON(out *[]splitPiece, atEnd bool) (rest string, hold bool, failed bool) {
	buf := sp.jsonBuf
	endIdx := jsonObjectEnd(buf)
	if endIdx < 0 {
		if !atEnd {
			return "", true, false
		}
		sp.hasJSONBuf = false
		sp.jsonBuf = ""
		*out = append(*out, splitPiece{Error: cutShortMsg})
		return "", false, true
	}
	sp.hasJSONBuf = false
	sp.jsonBuf = ""
	calls, status := parseJSONCalls(buf[:endIdx])
	if status == jsonCallsInvalid {
		*out = append(*out, splitPiece{Error: unreadableMsg})
		return "", false, true
	}
	if status == jsonCallsNone {
		sp.pushText(out, buf[:endIdx])
		return buf[endIdx:], false, false
	}
	sp.sawTool = true
	for idx := range calls {
		copied := calls[idx]
		*out = append(*out, splitPiece{Call: &copied})
	}
	return strings.TrimLeft(buf[endIdx:], " \t\r\n"), false, false
}

func (sp *Splitter) Feed(input string) []splitPiece {
	var out []splitPiece
	for input != "" {
		if sp.hasJSONBuf {
			sp.jsonBuf += input
			next, hold, failed := sp.handleJSON(&out, false)
			if failed {
				return out
			}
			if hold {
				break
			}
			input = next
			continue
		}
		if !sp.inCall {
			comb := sp.textBuf + input
			sp.textBuf = ""
			if sp.afterBare {
				rest, hold := sp.consumeLeft(comb, false)
				if hold {
					sp.textBuf = comb
					break
				}
				comb = rest
				if comb == "" {
					break
				}
			}
			callIdx := strings.Index(comb, callOpenTag)
			xmlIdx := -1
			if len(sp.tools) > 0 {
				if loc := xmlAtRe.FindStringIndex(comb); loc != nil {
					xmlIdx = loc[0]
				}
			}
			atStart := sp.sent == "" || strings.HasSuffix(sp.sent, "\n")
			jsonIdx := -1
			for _, loc := range jsonAtAllRe.FindAllStringSubmatchIndex(comb, -1) {
				hasNewline := loc[2] >= 0 && loc[3] > loc[2]
				at := loc[0]
				if hasNewline {
					at = loc[3]
				}
				if (hasNewline || atStart) && !isQuoted(sp.sent+comb[:at]) {
					jsonIdx = at
					break
				}
			}
			if jsonIdx >= 0 && (callIdx < 0 || jsonIdx < callIdx) && (xmlIdx < 0 || jsonIdx < xmlIdx) {
				if jsonIdx > 0 {
					sp.pushText(&out, comb[:jsonIdx])
				}
				sp.hasJSONBuf = true
				sp.jsonBuf = strings.TrimLeft(comb[jsonIdx:], " \t")
				input = ""
				next, hold, failed := sp.handleJSON(&out, false)
				if failed {
					return out
				}
				if hold {
					break
				}
				input = next
				continue
			}
			if xmlIdx >= 0 && (callIdx < 0 || xmlIdx < callIdx) {
				next, hold, failed := sp.handleBare(&out, comb, xmlIdx, false)
				if failed {
					return out
				}
				if hold {
					break
				}
				input = next
				continue
			}
			if callIdx < 0 {
				keep := 0
				marks := []string{callOpenTag}
				if len(sp.tools) > 0 {
					marks = append(marks, xmlMarks...)
				}
				for _, mark := range marks {
					maxN := len(mark) - 1
					if len(comb) < maxN {
						maxN = len(comb)
					}
					for suffixLen := maxN; suffixLen > keep; suffixLen-- {
						if strings.HasSuffix(comb, mark[:suffixLen]) {
							keep = suffixLen
							break
						}
					}
				}
				if len(sp.tools) > 0 {
					if loc := xmlHoldRe.FindStringIndex(comb); loc != nil {
						if tailLen := len(comb) - loc[0]; tailLen > keep {
							keep = tailLen
						}
					}
				}
				if holdJSON := jsonHoldLen(comb, atStart); holdJSON > keep {
					keep = holdJSON
				}
				if keep < len(comb) {
					sp.pushText(&out, comb[:len(comb)-keep])
				}
				sp.textBuf = comb[len(comb)-keep:]
				break
			}
			if callIdx > 0 {
				sp.pushText(&out, comb[:callIdx])
			}
			input = comb[callIdx+len(callOpenTag):]
			sp.inCall = true
			continue
		}
		comb := sp.callBuf + input
		sp.callBuf = ""
		closeIdx := strings.Index(comb, callCloseTag)
		if closeIdx < 0 {
			sp.callBuf = comb
			break
		}
		body := comb[:closeIdx]
		if parsed := parseCallBlock(body, sp.tools); parsed != nil {
			sp.sawTool = true
			out = append(out, splitPiece{Call: parsed})
		} else if isCallish(body) && !isQuoted(sp.sent) {
			sp.inCall = false
			out = append(out, splitPiece{Error: unreadableMsg})
			return out
		} else {
			sp.pushText(&out, callOpenTag+body+callCloseTag)
		}
		input = comb[closeIdx+len(callCloseTag):]
		sp.inCall = false
	}
	return out
}

func (sp *Splitter) Flush() []splitPiece {
	if sp.hasJSONBuf {
		var out []splitPiece
		next, _, failed := sp.handleJSON(&out, true)
		if !failed && next != "" {
			sp.pushText(&out, next)
		}
		return out
	}
	if sp.inCall {
		body := sp.callBuf
		sp.inCall = false
		sp.callBuf = ""
		if isCallish(body) && !isQuoted(sp.sent) {
			parsed := parseCallBlock(body, sp.tools)
			if parsed == nil {
				return []splitPiece{{Error: cutShortMsg}}
			}
			sp.sawTool = true
			return []splitPiece{{Call: parsed}}
		}
		raw := callOpenTag + body
		sp.sent += raw
		return []splitPiece{{Text: raw}}
	}
	if sp.textBuf != "" {
		rem := sp.textBuf
		sp.textBuf = ""
		if sp.afterBare {
			rem, _ = sp.consumeLeft(rem, true)
		}
		var out []splitPiece
		for rem != "" {
			xmlIdx := -1
			if len(sp.tools) > 0 {
				if loc := xmlAtRe.FindStringIndex(rem); loc != nil {
					xmlIdx = loc[0]
				}
			}
			if xmlIdx < 0 {
				sp.pushText(&out, rem)
				break
			}
			next, _, failed := sp.handleBare(&out, rem, xmlIdx, true)
			if failed {
				break
			}
			if sp.afterBare {
				rem, _ = sp.consumeLeft(next, true)
			} else {
				rem = next
			}
		}
		return out
	}
	return nil
}

type streamToolEvent struct {
	Index int
	ID    string
	Name  string
	Args  string
}

type streamArgsEvent struct {
	Index int
	Text  string
}

type streamUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	CachedTokens     int64
	ReasoningTokens  int64
}

func (usage *streamUsage) toOpenAI() map[string]any {
	if usage == nil {
		return nil
	}
	out := map[string]any{
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	}
	if usage.CachedTokens > 0 {
		out["prompt_tokens_details"] = map[string]any{"cached_tokens": usage.CachedTokens}
	}
	if usage.ReasoningTokens > 0 {
		out["completion_tokens_details"] = map[string]any{"reasoning_tokens": usage.ReasoningTokens}
	}
	return out
}

type streamEvent struct {
	Text      string
	Reasoning string
	Tool      *streamToolEvent
	Args      *streamArgsEvent
	Stop      string
	Usage     *streamUsage
	Error     *qoderFailure
}

type nativeCallTrack struct {
	ID       string
	Args     string
	HasIndex bool
	Index    int
}

type sseEventReader struct {
	reader   *bufio.Reader
	splitter *Splitter
	index    int
	native   map[int]*nativeCallTrack
	begun    []*nativeCallTrack
	usage    *streamUsage
	finish   string
	said     bool
	eof      bool
	done     bool
	pending  []streamEvent
}

func newSSEEventReader(body io.Reader, tools []map[string]any) *sseEventReader {
	return &sseEventReader{
		reader:   bufio.NewReaderSize(body, 64*1024),
		splitter: newSplitter(tools),
		index:    -1,
		native:   map[int]*nativeCallTrack{},
	}
}

func (sr *sseEventReader) pieceToEvent(piece splitPiece) streamEvent {
	if piece.Call != nil {
		sr.index++
		return streamEvent{
			Tool: &streamToolEvent{
				Index: sr.index,
				ID:    "call_" + hexID(),
				Name:  piece.Call.Name,
				Args:  piece.Call.Args,
			},
		}
	}
	if piece.Error != "" {
		return streamEvent{
			Error: &qoderFailure{Status: http.StatusBadGateway, Message: piece.Error},
		}
	}
	return streamEvent{Text: piece.Text}
}

func (sr *sseEventReader) Next() (streamEvent, bool, error) {
	for {
		if len(sr.pending) > 0 {
			ev := sr.pending[0]
			sr.pending = sr.pending[1:]
			return ev, true, nil
		}
		if sr.done {
			return streamEvent{}, false, nil
		}
		if sr.eof {
			sr.done = true
			for _, piece := range sr.splitter.Flush() {
				ev := sr.pieceToEvent(piece)
				sr.pending = append(sr.pending, ev)
				if ev.Error != nil {
					return sr.Next()
				}
				sr.said = true
			}
			namelessCount := 0
			for _, tracked := range sr.begun {
				if !tracked.HasIndex {
					namelessCount++
				}
			}
			if namelessCount > 0 && !sr.said {
				sr.pending = append(sr.pending, streamEvent{
					Error: &qoderFailure{
						Status:   http.StatusBadGateway,
						Message:  namelessCallMsg,
						Nameless: true,
					},
				})
				return sr.Next()
			}
			stopReason := "stop"
			if sr.splitter.sawTool {
				stopReason = "tool_calls"
			} else if sr.finish == "length" {
				stopReason = "length"
			}
			sr.pending = append(sr.pending, streamEvent{
				Stop:  stopReason,
				Usage: sr.usage,
			})
			return sr.Next()
		}

		line, readErr := sr.reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return streamEvent{}, false, readErr
		}
		if errors.Is(readErr, io.EOF) {
			sr.eof = true
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "data:") {
			continue
		}
		payload := strings.TrimSpace(trimmed[5:])
		var env map[string]any
		if json.Unmarshal([]byte(payload), &env) != nil || env == nil {
			continue
		}
		inner := rawStrField(env, "body")
		if statusVal, hasStatus := floatField(env, "statusCodeValue"); hasStatus && int(statusVal) != http.StatusOK {
			errText := inner
			if errText == "" {
				errText = payload
			}
			sr.done = true
			return streamEvent{Error: classifyFailure(int(statusVal), errText)}, true, nil
		}
		if strings.TrimSpace(inner) == "[DONE]" {
			sr.eof = true
			continue
		}
		var chunk map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(inner)), &chunk) != nil || chunk == nil {
			continue
		}
		choices, hasChoices := chunk["choices"].([]any)
		if !hasChoices && (chunk["error"] != nil || (chunk["code"] != nil && chunk["message"] != nil)) {
			sr.done = true
			return streamEvent{Error: classifyFailure(http.StatusBadGateway, inner)}, true, nil
		}
		var delta map[string]any
		var choiceFinish string
		if len(choices) > 0 {
			if choice0, ok := choices[0].(map[string]any); ok && choice0 != nil {
				delta, _ = choice0["delta"].(map[string]any)
				choiceFinish = rawStrField(choice0, "finish_reason")
			}
		}
		if delta != nil {
			if reasoning := rawStrField(delta, "reasoning_content"); reasoning != "" {
				sr.said = true
				sr.pending = append(sr.pending, streamEvent{Reasoning: reasoning})
			}
			if content, hasContent := delta["content"].(string); hasContent {
				for _, piece := range sr.splitter.Feed(content) {
					if piece.Call == nil && piece.Error == "" && piece.Text == "" {
						continue
					}
					ev := sr.pieceToEvent(piece)
					sr.pending = append(sr.pending, ev)
					if ev.Error != nil {
						sr.done = true
						return sr.Next()
					}
					sr.said = true
				}
			}
			if rawToolCalls, ok := delta["tool_calls"].([]any); ok {
				for _, item := range rawToolCalls {
					tcObj, ok := item.(map[string]any)
					if !ok || tcObj == nil {
						continue
					}
					upIdx := 0
					if idxNum, ok := floatField(tcObj, "index"); ok {
						upIdx = int(idxNum)
					}
					tcID := rawStrField(tcObj, "id")
					fnObj, _ := tcObj["function"].(map[string]any)
					tcName := strings.TrimSpace(rawStrField(fnObj, "name"))
					tcArgs := rawStrField(fnObj, "arguments")
					if tcID == "" && tcName == "" && tcArgs == "" {
						continue
					}
					tracked := sr.native[upIdx]
					if tracked == nil || (tcID != "" && tracked.ID != "" && tcID != tracked.ID) {
						tracked = &nativeCallTrack{}
						sr.native[upIdx] = tracked
						sr.begun = append(sr.begun, tracked)
					}
					if tcID != "" {
						tracked.ID = tcID
					}
					if !tracked.HasIndex {
						tracked.Args += tcArgs
						if tcName == "" {
							continue
						}
						for _, piece := range sr.splitter.Flush() {
							ev := sr.pieceToEvent(piece)
							sr.pending = append(sr.pending, ev)
							if ev.Error != nil {
								sr.done = true
								return sr.Next()
							}
						}
						sr.splitter.sawTool = true
						sr.said = true
						sr.index++
						tracked.Index = sr.index
						tracked.HasIndex = true
						callIDOut := tracked.ID
						if callIDOut == "" {
							callIDOut = "call_" + hexID()
						}
						sr.pending = append(sr.pending, streamEvent{
							Tool: &streamToolEvent{
								Index: tracked.Index,
								ID:    callIDOut,
								Name:  tcName,
								Args:  tracked.Args,
							},
						})
					} else if tcArgs != "" {
						sr.pending = append(sr.pending, streamEvent{
							Args: &streamArgsEvent{
								Index: tracked.Index,
								Text:  tcArgs,
							},
						})
					}
				}
			}
		}
		if choiceFinish != "" {
			sr.finish = choiceFinish
		}
		if usageMap, ok := chunk["usage"].(map[string]any); ok && usageMap != nil {
			promptTok := int64(0)
			if val, ok := floatField(usageMap, "prompt_tokens"); ok {
				promptTok = int64(val)
			}
			compTok := int64(0)
			if val, ok := floatField(usageMap, "completion_tokens"); ok {
				compTok = int64(val)
			}
			var cachedTok int64
			if details, ok := usageMap["prompt_tokens_details"].(map[string]any); ok {
				if val, ok := floatField(details, "cached_tokens"); ok {
					cachedTok = int64(val)
				}
			}
			var reasonTok int64
			if details, ok := usageMap["completion_tokens_details"].(map[string]any); ok {
				if val, ok := floatField(details, "reasoning_tokens"); ok {
					reasonTok = int64(val)
				}
			}
			sr.usage = &streamUsage{
				PromptTokens:     promptTok,
				CompletionTokens: compTok,
				TotalTokens:      promptTok + compTok,
				CachedTokens:     cachedTok,
				ReasoningTokens:  reasonTok,
			}
		}
	}
}
