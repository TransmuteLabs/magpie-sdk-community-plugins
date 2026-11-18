package opencode

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const maxZenToolBytes = 32 << 20

type zenToolPolicy struct {
	declared map[string]bool
	renamed  map[string]string
	byCase   map[string]string
	byShape  map[string]string
}

func zenToolJSON(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("invalid Zen tool JSON object")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("trailing Zen tool JSON")
	}
	return value, nil
}

func zenToolObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func zenToolString(value any) string {
	text, _ := value.(string)
	return text
}

func zenToolList(value any) ([]any, error) {
	if value == nil {
		return nil, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, errors.New("invalid Zen tool snapshot array")
	}
	return list, nil
}

func zenToolShape(name string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsSpace(character) || character == '\ufeff' || strings.ContainsRune("_.-", character) {
			return -1
		}
		return character
	}, strings.ToLower(name))
}

func (policy *zenToolPolicy) name(value any) string {
	name := zenToolString(value)
	if name == "" {
		return ""
	}
	if original, found := policy.renamed[name]; found {
		return original
	}
	if policy.declared[name] {
		return name
	}
	if match, found := policy.byCase[strings.ToLower(name)]; found {
		if match != "" {
			return match
		}
		return name
	}
	if match := policy.byShape[zenToolShape(name)]; match != "" {
		return match
	}
	return name
}

func zenToolDescription(description string) string {
	switch strings.TrimFunc(description, func(character rune) bool {
		return character == '\ufeff' || character != '\u0085' && unicode.IsSpace(character)
	}) {
	case "Edit an existing UTF-8 text file by replacing literal text.":
		return "Replace an exact piece of text in a UTF-8 file that already exists."
	case "Read a UTF-8 text file and return line-numbered content.":
		return "Read a UTF-8 text file. Each line comes back with its number."
	case "Load the full instructions for a skill. Call it before acting on a task that names or clearly matches a skill in the session skill catalog.":
		return "Open a skill's whole instructions. Use it first when a task names a skill from the session's skill catalog, or plainly fits one."
	}
	return description
}

func zenServedTool(name string, taken map[string]bool, properties map[string]any) string {
	for _, rule := range []struct {
		names []string
		as    map[string]string
	}{
		{[]string{"ReportFindings", "ScheduleWakeup"}, map[string]string{"ReportFindings": "findings_report", "ScheduleWakeup": "wakeup_schedule"}},
		{[]string{"subagent_fork"}, map[string]string{"subagent_fork": "conversation_delegate"}},
		{[]string{"read_image", "working_directory"}, map[string]string{"working_directory": "workspace_directory"}},
		{[]string{"CreateWorkflow", "GetWorkflowRun"}, map[string]string{"CreateWorkflow": "workflow_create", "GetWorkflowRun": "workflow_inspect"}},
		{[]string{"AmendWorkflow", "FillWorkflowHole"}, map[string]string{"AmendWorkflow": "workflow_amend", "FillWorkflowHole": "workflow_answer"}},
	} {
		if alias := rule.as[name]; alias != "" && !slices.ContainsFunc(rule.names, func(other string) bool { return !taken[other] }) {
			return alias
		}
	}
	for _, rule := range []struct {
		name, as   string
		properties []string
	}{
		{"edit", "text_replace", []string{"file_path", "old_string", "new_string"}},
		{"read", "file_view", []string{"file_path", "offset", "limit"}},
	} {
		if name == rule.name && !slices.ContainsFunc(rule.properties, func(property string) bool {
			_, present := properties[property]
			return !present
		}) {
			return rule.as
		}
	}
	served := name
	for _, rule := range []struct{ word, as string }{{"terminal", "console"}, {"run_code", "exec_program"}} {
		var result strings.Builder
		remaining := served
		for {
			position := strings.Index(strings.ToLower(remaining), rule.word)
			if position < 0 {
				result.WriteString(remaining)
				break
			}
			result.WriteString(remaining[:position])
			found := remaining[position : position+len(rule.word)]
			alias := rule.as
			if found == strings.ToUpper(found) {
				alias = strings.ToUpper(alias)
			} else if found[:1] == strings.ToUpper(found[:1]) {
				parts := strings.Split(alias, "_")
				for index := range parts {
					parts[index] = strings.ToUpper(parts[index][:1]) + parts[index][1:]
				}
				alias = strings.Join(parts, "_")
			}
			result.WriteString(alias)
			remaining = remaining[position+len(rule.word):]
		}
		served = result.String()
	}
	return served
}

func prepareZenTools(protocol string, body []byte) ([]byte, *zenToolPolicy, error) {
	value, err := zenToolJSON(body)
	if err != nil {
		return nil, nil, err
	}
	tools := []any{}
	if supplied, exists := value["tools"]; exists {
		var ok bool
		tools, ok = supplied.([]any)
		if !ok {
			return nil, nil, errors.New("Zen tools must be an array")
		}
	}
	definition := func(tool any) map[string]any {
		object := zenToolObject(tool)
		if protocol == "chat" {
			return zenToolObject(object["function"])
		}
		return object
	}
	policy := &zenToolPolicy{declared: map[string]bool{}, renamed: map[string]string{},
		byCase: map[string]string{}, byShape: map[string]string{}}
	properties := map[string]map[string]any{}
	for _, tool := range tools {
		current := definition(tool)
		if name, ok := current["name"].(string); ok {
			policy.declared[name] = true
			schema := current["parameters"]
			if protocol == "messages" {
				schema = current["input_schema"]
			}
			if supplied := zenToolObject(zenToolObject(schema)["properties"]); supplied != nil {
				properties[name] = supplied
			}
		}
		if description, ok := current["description"].(string); ok {
			current["description"] = zenToolDescription(description)
		}
	}
	var functions []any
	if protocol == "chat" {
		functions, _ = value["functions"].([]any)
		for _, function := range functions {
			if name, ok := zenToolObject(function)["name"].(string); ok {
				policy.declared[name] = true
			}
		}
	}
	taken, order := map[string]bool{}, []string{}
	definitions := make([]map[string]any, 0, len(tools)+len(functions))
	for _, tool := range tools {
		definitions = append(definitions, definition(tool))
	}
	for _, function := range functions {
		definitions = append(definitions, zenToolObject(function))
	}
	for _, tool := range definitions {
		if name, ok := tool["name"].(string); ok && !taken[name] {
			taken[name] = true
			order = append(order, name)
		}
	}
	served := map[string]string{}
	for _, name := range order {
		wanted := zenServedTool(name, taken, properties[name])
		if wanted == name {
			continue
		}
		alias := wanted
		for suffix := 2; taken[alias] || policy.renamed[alias] != ""; suffix++ {
			alias = wanted + "_" + strconv.Itoa(suffix)
		}
		served[name], policy.renamed[alias] = alias, name
	}
	rename := func(object map[string]any) {
		if alias := served[zenToolString(object["name"])]; alias != "" {
			object["name"] = alias
		}
	}
	for _, tool := range definitions {
		rename(tool)
	}
	present := map[string]bool{}
	for _, tool := range tools {
		if name, ok := definition(tool)["name"].(string); ok {
			present[name] = true
		}
	}
	for _, core := range []struct{ name, parameter string }{
		{"edit", "path"}, {"glob", "pattern"}, {"grep", "pattern"}, {"read", "path"}, {"shell", "command"},
	} {
		if present[core.name] {
			continue
		}
		parameters := map[string]any{"type": "object", "properties": map[string]any{core.parameter: map[string]any{"type": "string"}},
			"required": []any{core.parameter}, "additionalProperties": false}
		tool := map[string]any{"name": core.name, "description": "Not available. Never call this tool."}
		switch protocol {
		case "chat":
			tool["parameters"], tool["strict"] = parameters, false
			tool = map[string]any{"type": "function", "function": tool}
		case "messages":
			tool["input_schema"] = parameters
		default:
			tool["type"], tool["parameters"] = "function", parameters
		}
		tools = append(tools, tool)
	}
	value["tools"] = tools
	messages, _ := value["messages"].([]any)
	if protocol == "responses" {
		messages, _ = value["input"].([]any)
	}
	for _, item := range messages {
		message := zenToolObject(item)
		switch protocol {
		case "responses":
			if message["type"] == "function_call" {
				rename(message)
			}
		case "messages":
			content, _ := message["content"].([]any)
			for _, item := range content {
				part := zenToolObject(item)
				if part["type"] == "tool_use" {
					rename(part)
				}
			}
		default:
			calls, _ := message["tool_calls"].([]any)
			for _, call := range calls {
				rename(zenToolObject(zenToolObject(call)["function"]))
			}
			rename(zenToolObject(message["function_call"]))
		}
	}
	choice := zenToolObject(value["tool_choice"])
	if function := zenToolObject(choice["function"]); function != nil {
		rename(function)
	} else {
		rename(choice)
	}
	rename(zenToolObject(value["function_call"]))
	for name := range policy.declared {
		for _, folded := range []struct {
			key   string
			table map[string]string
		}{{strings.ToLower(name), policy.byCase}, {zenToolShape(name), policy.byShape}} {
			if folded.key == "" {
				continue
			}
			if _, exists := folded.table[folded.key]; exists {
				folded.table[folded.key] = ""
			} else {
				folded.table[folded.key] = name
			}
		}
	}
	encoded, err := json.Marshal(value)
	if err == nil && len(encoded) > sdk.MaxNativeBodyBytes {
		err = errors.New("shaped Zen request exceeds its byte limit")
	}
	return encoded, policy, err
}

func zenDroppedToolNote() string {
	return "(The model called a tool without a name. The call was left out.)"
}

func zenChatHasCalls(message map[string]any) bool {
	calls, _ := message["tool_calls"].([]any)
	return message["function_call"] != nil || len(calls) != 0
}

func (policy *zenToolPolicy) chatMessage(message map[string]any) (bool, []string, error) {
	changed, notes := false, []string{}
	if value := message["function_call"]; value != nil {
		function := zenToolObject(value)
		if function == nil {
			return false, nil, errors.New("invalid Zen function call")
		}
		name := policy.name(function["name"])
		if name == "" {
			delete(message, "function_call")
			changed, notes = true, append(notes, zenDroppedToolNote())
		} else if function["name"] != name {
			function["name"], changed = name, true
		}
	}
	if value := message["tool_calls"]; value != nil {
		calls, err := zenToolList(value)
		if err != nil {
			return false, nil, err
		}
		kept := []any{}
		for _, item := range calls {
			call := zenToolObject(item)
			function := zenToolObject(call["function"])
			if call == nil || call["type"] != nil && call["type"] != "function" || function == nil {
				return false, nil, errors.New("unsupported Zen function call")
			}
			name := policy.name(function["name"])
			if name == "" {
				changed, notes = true, append(notes, zenDroppedToolNote())
				continue
			}
			if function["name"] != name {
				function["name"], changed = name, true
			}
			kept = append(kept, call)
		}
		if len(kept) != len(calls) {
			if len(kept) == 0 {
				delete(message, "tool_calls")
			} else {
				message["tool_calls"] = kept
			}
		}
	}
	return changed, notes, nil
}

func zenAppendToolNotes(message map[string]any, notes []string) {
	text := strings.Join(notes, "\n\n")
	switch content := message["content"].(type) {
	case string:
		if content != "" {
			text = content + "\n\n" + text
		}
		message["content"] = text
	case []any:
		message["content"] = append(content, map[string]any{"type": "text", "text": text})
	default:
		message["content"] = text
	}
}

func zenToolReplacement(protocol string, call map[string]any) map[string]any {
	if protocol == "messages" {
		return map[string]any{"type": "text", "text": zenDroppedToolNote()}
	}
	id := zenToolString(call["id"])
	if id == "" {
		id = "msg_" + zenToolString(call["call_id"])
		if id == "msg_" {
			id += "dropped"
		}
	}
	status := call["status"]
	if status == nil {
		status = "completed"
	}
	return map[string]any{"type": "message", "id": id, "role": "assistant", "status": status,
		"content": []any{map[string]any{"type": "output_text", "text": zenDroppedToolNote(), "annotations": []any{}}}}
}

func (policy *zenToolPolicy) snapshots(value map[string]any, protocol string) (bool, int, int, error) {
	changed, kept, left := false, 0, 0
	if protocol == "chat" {
		choices, err := zenToolList(value["choices"])
		if err != nil {
			return false, 0, 0, err
		}
		for _, item := range choices {
			choice := zenToolObject(item)
			if choice == nil {
				return false, 0, 0, errors.New("invalid Zen choice")
			}
			message := zenToolObject(choice["message"])
			own, notes, err := policy.chatMessage(message)
			if err != nil {
				return false, 0, 0, err
			}
			changed = changed || own
			if len(notes) != 0 {
				zenAppendToolNotes(message, notes)
				if !zenChatHasCalls(message) && (choice["finish_reason"] == "tool_calls" || choice["finish_reason"] == "function_call") {
					choice["finish_reason"] = "stop"
				}
			}
		}
		return changed, 0, 0, nil
	}
	type entry struct {
		value   any
		replace func(map[string]any)
	}
	stack := []entry{{value: value}}
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		object := zenToolObject(current.value)
		if object == nil {
			return false, 0, 0, errors.New("invalid Zen tool snapshot")
		}
		kind := zenToolString(object["type"])
		if protocol == "responses" && strings.HasSuffix(kind, "_call") && kind != "function_call" {
			return false, 0, 0, errors.New("unsupported Zen tool call type")
		}
		if protocol == "responses" && kind == "function_call" || protocol == "messages" && (kind == "tool_use" || kind == "server_tool_use") {
			name := policy.name(object["name"])
			if name == "" {
				if current.replace == nil {
					return false, 0, 0, errors.New("Zen root tool call has no name")
				}
				current.replace(zenToolReplacement(protocol, object))
				changed, left = true, left+1
				continue
			}
			if object["name"] != name {
				object["name"], changed = name, true
			}
			kept++
		}
		arrays, objects := []string{"content"}, []string{"message", "content_block"}
		if protocol == "responses" {
			arrays, objects = []string{"output", "content"}, []string{"response", "item", "part"}
		}
		for _, name := range arrays {
			if _, text := object[name].(string); text {
				continue
			}
			items, err := zenToolList(object[name])
			if err != nil {
				return false, 0, 0, err
			}
			for index, item := range items {
				stack = append(stack, entry{item, func(replacement map[string]any) { items[index] = replacement }})
			}
		}
		for _, name := range objects {
			if item := object[name]; item != nil {
				stack = append(stack, entry{item, func(replacement map[string]any) { object[name] = replacement }})
			}
		}
	}
	if protocol == "messages" && left != 0 && kept == 0 && value["type"] == "message" && value["stop_reason"] == "tool_use" {
		value["stop_reason"] = "end_turn"
	}
	return changed, kept, left, nil
}
