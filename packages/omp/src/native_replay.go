package ompsessions

import "encoding/json"

const ompNotExecuted = `[{"type":"text","text":"Tool call was not executed because the provider stream ended with an error before the tool could run."}]`

type ompReplayMessage struct {
	ompCarryMessage
	StopReason  json.RawMessage `json:"stopReason"`
	StopDetails json.RawMessage `json:"stopDetails"`
	Details     json.RawMessage `json:"details"`
}

type ompNativeReplay struct {
	pending map[string]bool
}

func (replay *ompNativeReplay) keep(message *ompReplayMessage) bool {
	switch message.Role {
	case "user", "fileMention", "bashExecution", "pythonExecution":
		clear(replay.pending)
	case "assistant":
		clear(replay.pending)
		if ompJSONString(message.StopReason) == "error" {
			var stopDetails map[string]json.RawMessage
			if json.Unmarshal(message.StopDetails, &stopDetails) == nil {
				switch ompJSONString(stopDetails["type"]) {
				case "refusal", "sensitive":
					return false
				}
			}
		}
		var blocks []struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if json.Unmarshal(message.Content, &blocks) == nil {
			for _, block := range blocks {
				if block.Type == "toolCall" && block.ID != "" {
					if replay.pending == nil {
						replay.pending = make(map[string]bool)
					}
					replay.pending[block.ID] = true
				}
			}
		}
	case "toolResult":
		paired := replay.pending[message.ToolCallID]
		delete(replay.pending, message.ToolCallID)
		var details map[string]json.RawMessage
		if json.Unmarshal(message.Details, &details) == nil && string(details["__synthetic"]) == "true" &&
			ompJSONString(details["source"]) == "assistant_stop_error" {
			if !paired {
				return false
			}
			message.Content = json.RawMessage(ompNotExecuted)
			message.IsError = true
		}
	}
	return true
}

func ompJSONString(raw json.RawMessage) string {
	var value string
	json.Unmarshal(raw, &value)
	return value
}
