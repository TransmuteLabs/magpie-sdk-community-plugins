package devin

import (
	"encoding/json"
	"errors"
)

func normalizePromptCache(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var cache struct {
		Type string `json:"type"`
		TTL  string `json:"ttl"`
	}
	if strictJSON(raw, &cache) != nil || cache.Type != "ephemeral" {
		return nil, errors.New("unsupported native prompt cache control")
	}
	fields, _ := catalogObject(raw)
	if _, present := fields["ttl"]; present && cache.TTL != "5m" {
		return nil, errors.New("unsupported native prompt cache lifetime")
	}
	return json.RawMessage(`{"type":"ephemeral"}`), nil
}

func expandOutboundCacheContent(messages []outboundMessage) ([]outboundMessage, error) {
	var result []outboundMessage
	for _, entry := range messages {
		cache, err := normalizePromptCache(entry.Cache)
		if err != nil {
			return nil, err
		}
		entry.Cache = cache
		var parts []json.RawMessage
		if json.Unmarshal(entry.Content, &parts) != nil || len(parts) == 0 {
			result = append(result, entry)
			continue
		}
		marks := make([]json.RawMessage, len(parts))
		marked, textOnly := false, true
		for index, part := range parts {
			fields, err := catalogObject(part)
			if err != nil {
				return nil, errors.New("unsupported canonical content block")
			}
			kind, err := catalogString(fields, "type")
			if err != nil {
				return nil, err
			}
			textOnly = textOnly && kind == "text"
			marks[index], err = normalizePromptCache(fields["cache_control"])
			if err != nil {
				return nil, err
			}
			if len(marks[index]) > 0 {
				marked = true
				delete(fields, "cache_control")
				parts[index], err = json.Marshal(fields)
				if err != nil {
					return nil, err
				}
			}
		}
		mixedAssistant := entry.Role == "assistant" &&
			(len(entry.ToolCalls) > 0 || entry.ReasoningContent != "" || entry.Reasoning != "" || len(entry.ReasoningDetails) > 0)
		if entry.Role == "tool" || mixedAssistant {
			for index, mark := range marks {
				if len(mark) == 0 {
					continue
				}
				if entry.Role != "tool" || index != len(parts)-1 {
					return nil, errors.New("content cache boundary has no single native prompt mapping")
				}
				entry.Cache = mark
			}
			entry.Content, err = json.Marshal(parts)
			if err != nil {
				return nil, err
			}
			result = append(result, entry)
			continue
		}
		if !marked && !textOnly {
			result = append(result, entry)
			continue
		}
		for index, part := range parts {
			next := entry
			next.Content = append(append([]byte{'['}, part...), ']')
			next.Cache = marks[index]
			next.blockBoundary = true
			if index == len(parts)-1 && len(entry.Cache) > 0 {
				next.Cache = entry.Cache
			}
			result = append(result, next)
		}
		if len(result) > 2048 {
			return nil, errors.New("too many canonical prompt blocks")
		}
	}
	return result, nil
}
