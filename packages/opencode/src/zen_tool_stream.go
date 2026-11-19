package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type zenToolFrame struct {
	raw   []byte
	lines []string
	data  []byte
	event string
}

var errZenToolsTruncated = errors.New("Zen stream ended without its native terminal event")

func readZenToolFrame(reader *bufio.Reader, first *bool) (zenToolFrame, error) {
	frame := zenToolFrame{}
	var line []byte
	for {
		character, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF && len(frame.raw) != 0 {
				err = errZenToolsTruncated
			}
			return frame, err
		}
		frame.raw = append(frame.raw, character)
		if len(frame.raw) > maxReplyBytes {
			return frame, errors.New("Zen SSE event exceeds its byte limit")
		}
		if character != '\n' && character != '\r' {
			line = append(line, character)
			continue
		}
		if character == '\r' {
			if next, _ := reader.Peek(1); len(next) != 0 && next[0] == '\n' {
				reader.ReadByte()
				frame.raw = append(frame.raw, '\n')
			}
		}
		if !utf8.Valid(line) {
			return frame, errors.New("invalid UTF-8 in Zen SSE")
		}
		if *first {
			line = bytes.TrimPrefix(line, []byte("\xef\xbb\xbf"))
			*first = false
		}
		if len(line) == 0 {
			if len(frame.data) != 0 {
				frame.data = frame.data[:len(frame.data)-1]
			}
			return frame, nil
		}
		text := string(line)
		frame.lines = append(frame.lines, text)
		key, value, _ := strings.Cut(text, ":")
		value = strings.TrimPrefix(value, " ")
		if key == "data" {
			frame.data = append(frame.data, value...)
			frame.data = append(frame.data, '\n')
		}
		if key == "event" {
			frame.event = value
		}
		line = line[:0]
	}
}

func encodeZenToolEvent(value map[string]any, event string) []byte {
	body, _ := json.Marshal(value)
	prefix := ""
	if event != "" {
		prefix = "event: " + event + "\n"
	}
	return append(append([]byte(prefix+"data: "), body...), '\n', '\n')
}

func (frame zenToolFrame) rewrite(value map[string]any) []byte {
	lines := []string{}
	for _, line := range frame.lines {
		if line != "data" && !strings.HasPrefix(line, "data:") {
			lines = append(lines, line)
		}
	}
	body, _ := json.Marshal(value)
	lines = append(lines, "data: "+string(body))
	return []byte(strings.Join(lines, "\n") + "\n\n")
}

func emitResponseData(ctx context.Context, stream *sdk.Stream, body []byte) error {
	for len(body) != 0 {
		count := min(len(body), sdk.MaxChunkBytes)
		if err := stream.Emit(ctx, "data", body[:count]); err != nil {
			return err
		}
		body = body[count:]
	}
	return nil
}

type zenChatTools struct {
	policy  *zenToolPolicy
	pending map[int]*zenChatToolChoice
	wrote   map[int]bool
	ended   map[int]bool
	bytes   int
}

type zenChatToolChoice struct {
	metadata map[string]any
	calls    []map[string]any
	open     map[int]int
	legacy   map[string]any
	collided bool
	bytes    int
}

func zenToolIndex(value any) (int, error) {
	if value == nil {
		return 0, nil
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, errors.New("invalid Zen tool index")
	}
	index, err := number.Float64()
	if err != nil || index < 0 || index > 9007199254740991 || math.Trunc(index) != index {
		return 0, errors.New("invalid Zen tool index")
	}
	return int(index), nil
}

func zenAppendFunction(target map[string]any, fragment any) error {
	object := zenToolObject(fragment)
	if object == nil {
		return errors.New("invalid Zen function delta")
	}
	for key, value := range object {
		if key == "name" || key == "arguments" {
			if value == nil {
				continue
			}
			text, ok := value.(string)
			if !ok {
				return errors.New("invalid Zen function delta string")
			}
			target[key] = zenToolString(target[key]) + text
		} else {
			target[key] = value
		}
	}
	return nil
}

func (chat *zenChatTools) charge(state *zenChatToolChoice, value any) error {
	body, _ := json.Marshal(value)
	chat.bytes += len(body)
	state.bytes += len(body)
	if chat.bytes > maxZenToolBytes {
		return errors.New("pending Zen tools exceed their byte limit")
	}
	return nil
}

func (chat *zenChatTools) complete(state *zenChatToolChoice, index int) (map[string]any, error) {
	delta := map[string]any{}
	if len(state.calls) != 0 {
		slices.SortStableFunc(state.calls, func(left, right map[string]any) int {
			first, _ := zenToolIndex(left["index"])
			second, _ := zenToolIndex(right["index"])
			if first < second {
				return -1
			}
			if first > second {
				return 1
			}
			return 0
		})
		calls := make([]any, len(state.calls))
		for position, call := range state.calls {
			if state.collided {
				call["index"] = json.Number(strconv.Itoa(position))
			}
			calls[position] = call
		}
		delta["tool_calls"] = calls
	}
	if state.legacy != nil {
		delta["function_call"] = state.legacy
	}
	_, notes, err := chat.policy.chatMessage(delta)
	if err != nil {
		return nil, err
	}
	if len(notes) != 0 {
		calls, _ := delta["tool_calls"].([]any)
		for position, call := range calls {
			zenToolObject(call)["index"] = json.Number(strconv.Itoa(position))
		}
		prefix := ""
		if chat.wrote[index] {
			prefix = "\n\n"
		}
		delta["content"] = prefix + strings.Join(notes, "\n\n")
		if !zenChatHasCalls(delta) {
			chat.ended[index] = true
		}
	}
	return delta, nil
}

func (chat *zenChatTools) apply(value map[string]any) (map[string]any, bool, error) {
	changed, _, _, err := chat.policy.snapshots(value, "chat")
	if err != nil {
		return nil, false, err
	}
	choices, _ := zenToolList(value["choices"])
	for _, item := range choices {
		choice := zenToolObject(item)
		delta := zenToolObject(choice["delta"])
		if delta == nil {
			continue
		}
		index, err := zenToolIndex(choice["index"])
		if err != nil {
			return nil, false, err
		}
		if zenToolString(delta["content"]) != "" {
			chat.wrote[index] = true
		}
		if delta["tool_calls"] == nil && delta["function_call"] == nil {
			continue
		}
		changed = true
		state := chat.pending[index]
		if state == nil {
			state = &zenChatToolChoice{metadata: map[string]any{}, open: map[int]int{}}
			for _, key := range []string{"id", "object", "created", "model", "system_fingerprint", "service_tier"} {
				if own, found := value[key]; found {
					state.metadata[key] = own
				}
			}
			chat.pending[index] = state
			if err := chat.charge(state, state.metadata); err != nil {
				return nil, false, err
			}
		}
		fragments, err := zenToolList(delta["tool_calls"])
		if err != nil {
			return nil, false, err
		}
		for _, fragment := range fragments {
			object := zenToolObject(fragment)
			if object == nil || object["type"] != nil && object["type"] != "function" {
				return nil, false, errors.New("unsupported Zen tool delta")
			}
			if err := chat.charge(state, object); err != nil {
				return nil, false, err
			}
			position, err := zenToolIndex(object["index"])
			if err != nil {
				return nil, false, err
			}
			slot, found := state.open[position]
			if found && zenToolString(object["id"]) != "" && zenToolString(state.calls[slot]["id"]) != "" &&
				object["id"] != state.calls[slot]["id"] {
				state.collided, found = true, false
			}
			if !found {
				slot = len(state.calls)
				state.open[position] = slot
				state.calls = append(state.calls, map[string]any{"index": json.Number(strconv.Itoa(position)), "function": map[string]any{}})
			}
			call := state.calls[slot]
			for key, own := range object {
				if key != "function" && !(key == "id" && zenToolString(call["id"]) != "" && zenToolString(own) == "") {
					call[key] = own
				}
			}
			call["index"] = json.Number(strconv.Itoa(position))
			if object["function"] != nil {
				if err := zenAppendFunction(zenToolObject(call["function"]), object["function"]); err != nil {
					return nil, false, err
				}
			}
		}
		if fragment := delta["function_call"]; fragment != nil {
			if err := chat.charge(state, fragment); err != nil {
				return nil, false, err
			}
			if state.legacy == nil {
				state.legacy = map[string]any{}
			}
			if err := zenAppendFunction(state.legacy, fragment); err != nil {
				return nil, false, err
			}
		}
		delete(delta, "tool_calls")
		delete(delta, "function_call")
	}
	for _, item := range choices {
		choice := zenToolObject(item)
		if choice["finish_reason"] == nil {
			continue
		}
		index, err := zenToolIndex(choice["index"])
		if err != nil {
			return nil, false, err
		}
		if state := chat.pending[index]; state != nil {
			delta, err := chat.complete(state, index)
			if err != nil {
				return nil, false, err
			}
			own := zenToolObject(choice["delta"])
			if own == nil {
				own = map[string]any{}
			}
			if text := zenToolString(delta["content"]); text != "" {
				delta["content"] = zenToolString(own["content"]) + text
			}
			maps.Copy(own, delta)
			choice["delta"] = own
			chat.bytes -= state.bytes
			delete(chat.pending, index)
			changed = true
		}
		if chat.ended[index] && (choice["finish_reason"] == "tool_calls" || choice["finish_reason"] == "function_call") {
			choice["finish_reason"], changed = "stop", true
		}
	}
	if !changed {
		return value, false, nil
	}
	kept := []any{}
	for _, item := range choices {
		choice := zenToolObject(item)
		keep := choice["finish_reason"] != nil || len(zenToolObject(choice["delta"])) != 0
		for key := range choice {
			keep = keep || key != "index" && key != "delta" && key != "finish_reason"
		}
		if keep {
			kept = append(kept, choice)
		}
	}
	value["choices"] = kept
	if len(kept) == 0 && value["usage"] == nil {
		return nil, true, nil
	}
	return value, true, nil
}

func (chat *zenChatTools) finish() (map[string]any, error) {
	if len(chat.pending) == 0 {
		return nil, nil
	}
	choices := []any{}
	var metadata map[string]any
	for _, index := range slices.Sorted(maps.Keys(chat.pending)) {
		state := chat.pending[index]
		delta, err := chat.complete(state, index)
		if err != nil {
			return nil, err
		}
		if metadata == nil {
			metadata = maps.Clone(state.metadata)
		}
		choices = append(choices, map[string]any{"index": index, "delta": delta, "finish_reason": nil})
	}
	metadata["choices"] = choices
	clear(chat.pending)
	chat.bytes = 0
	return metadata, nil
}

type zenSnapshotTools struct {
	policy   *zenToolPolicy
	protocol string
	left     map[string]bool
	leftIDs  map[string]bool
	kept     int
	dropped  int
}

func (snapshot *zenSnapshotTools) apply(value map[string]any) (map[string]any, bool, []map[string]any, error) {
	kind := zenToolString(value["type"])
	extra := []map[string]any{}
	if snapshot.protocol == "responses" {
		item := zenToolObject(value["item"])
		if kind == "response.output_item.added" && item["type"] == "function_call" && snapshot.policy.name(item["name"]) == "" {
			message := zenToolReplacement("responses", item)
			snapshot.left[string(rawZenIndex(value["output_index"]))], snapshot.leftIDs[zenToolString(message["id"])] = true, true
			if id := zenToolString(item["id"]); id != "" {
				snapshot.leftIDs[id] = true
			}
			message["status"], message["content"] = "in_progress", []any{}
			value["item"] = message
			where := map[string]any{"item_id": message["id"], "output_index": value["output_index"], "content_index": 0}
			for _, kind := range []string{"response.content_part.added", "response.output_text.delta", "response.output_text.done", "response.content_part.done"} {
				event := maps.Clone(where)
				event["type"] = kind
				switch kind {
				case "response.output_text.delta":
					event["delta"] = zenDroppedToolNote()
				case "response.output_text.done":
					event["text"] = zenDroppedToolNote()
				default:
					text := ""
					if kind == "response.content_part.done" {
						text = zenDroppedToolNote()
					}
					event["part"] = map[string]any{"type": "output_text", "text": text, "annotations": []any{}}
				}
				extra = append(extra, event)
			}
			return value, true, extra, nil
		}
		if strings.HasPrefix(kind, "response.function_call_arguments.") &&
			(value["output_index"] != nil && snapshot.left[string(rawZenIndex(value["output_index"]))] || snapshot.leftIDs[zenToolString(value["item_id"])]) {
			return nil, true, nil, nil
		}
	} else {
		if kind == "message_start" {
			clear(snapshot.left)
			snapshot.kept, snapshot.dropped = 0, 0
		}
		block := zenToolObject(value["content_block"])
		if kind == "content_block_start" && (block["type"] == "tool_use" || block["type"] == "server_tool_use") && snapshot.policy.name(block["name"]) == "" {
			snapshot.left[string(rawZenIndex(value["index"]))] = true
			snapshot.dropped++
			value["content_block"] = map[string]any{"type": "text", "text": ""}
			extra = append(extra, map[string]any{"type": "content_block_delta", "index": value["index"],
				"delta": map[string]any{"type": "text_delta", "text": zenDroppedToolNote()}})
			return value, true, extra, nil
		}
		if kind == "content_block_delta" && snapshot.left[string(rawZenIndex(value["index"]))] {
			return nil, true, nil, nil
		}
		delta := zenToolObject(value["delta"])
		if kind == "message_delta" && delta["stop_reason"] == "tool_use" && snapshot.dropped != 0 && snapshot.kept == 0 {
			delta["stop_reason"] = "end_turn"
			return value, true, nil, nil
		}
	}
	changed, kept, left, err := snapshot.policy.snapshots(value, snapshot.protocol)
	snapshot.kept, snapshot.dropped = snapshot.kept+kept, snapshot.dropped+left
	return value, changed, nil, err
}

func rawZenIndex(value any) []byte {
	if number, ok := value.(json.Number); ok {
		if parsed, err := number.Float64(); err == nil {
			return []byte(strconv.FormatFloat(parsed, 'f', -1, 64))
		}
	}
	body, _ := json.Marshal(value)
	return body
}

func relayZenTools(ctx context.Context, reader io.Reader, policy *zenToolPolicy, protocol string, streaming bool, emit func([]byte) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	publish := func(body []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(body) > maxZenToolBytes {
			return errors.New("shaped Zen reply exceeds its byte limit")
		}
		return emit(body)
	}
	if !streaming {
		body, err := readBounded(reader, maxZenToolBytes)
		if err != nil {
			return err
		}
		value, err := zenToolJSON(body)
		if err != nil {
			return err
		}
		changed, _, _, err := policy.snapshots(value, protocol)
		if err != nil {
			return err
		}
		if changed {
			body, err = json.Marshal(value)
			if err != nil {
				return err
			}
		}
		return publish(body)
	}
	terminal := zenTerminal{protocol: protocol}
	chat := zenChatTools{policy: policy, pending: map[int]*zenChatToolChoice{}, wrote: map[int]bool{}, ended: map[int]bool{}}
	snapshot := zenSnapshotTools{policy: policy, protocol: protocol, left: map[string]bool{}, leftIDs: map[string]bool{}}
	buffer, first := bufio.NewReader(reader), true
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		frame, err := readZenToolFrame(buffer, &first)
		if err == io.EOF {
			return errZenToolsTruncated
		}
		if err != nil {
			return err
		}
		if err := terminal.write([]byte(strings.Join(frame.lines, "\n") + "\n\n")); err != nil {
			return err
		}
		if frame.event == "error" {
			return errors.New("Zen stream reported an upstream error")
		}
		body := frame.raw
		trimmed := bytes.TrimSpace(frame.data)
		if protocol == "chat" && bytes.Equal(trimmed, []byte("[DONE]")) {
			final, err := chat.finish()
			if err != nil {
				return err
			}
			if final != nil {
				body = append(encodeZenToolEvent(final, ""), body...)
			}
		} else if len(trimmed) != 0 {
			value, err := zenToolJSON(frame.data)
			if err != nil {
				return err
			}
			var result map[string]any
			var changed bool
			var extra []map[string]any
			if protocol == "chat" {
				result, changed, err = chat.apply(value)
			} else {
				result, changed, extra, err = snapshot.apply(value)
			}
			if err != nil {
				return err
			}
			if result == nil {
				body = nil
			} else if changed {
				body = frame.rewrite(result)
			}
			for _, event := range extra {
				body = append(body, encodeZenToolEvent(event, zenToolString(event["type"]))...)
			}
		}
		if len(chat.pending)+len(chat.wrote)+len(chat.ended)+len(snapshot.left)+len(snapshot.leftIDs) > 10000 {
			return errors.New("Zen tool stream exceeds its position limit")
		}
		if err := publish(body); err != nil {
			return err
		}
		if terminal.complete() {
			return nil
		}
	}
}
