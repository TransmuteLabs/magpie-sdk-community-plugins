package devin

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

func joinHistoryText(first, second string) string {
	if first == "" {
		return second
	}
	if second == "" {
		return first
	}
	return first + "\n\n" + second
}

func omitEmptyOutboundUser(entry outboundMessage) (bool, error) {
	text, images, err := encodeOutboundContent(entry.Content)
	if err != nil || strings.TrimSpace(text) != "" || len(images) != 0 {
		return false, nil
	}
	if len(entry.Cache) > 0 {
		return false, errors.New("whitespace-only user cache boundary is unsupported")
	}
	return true, nil
}

func normalizeOutboundTurns(messages []outboundMessage, model string) ([]outboundMessage, error) {
	result := make([]outboundMessage, 0, len(messages))
	claude := isClaudeModel(model)
	for _, entry := range messages {
		if entry.ReasoningContent != "" && entry.Reasoning != "" && entry.ReasoningContent != entry.Reasoning {
			return nil, errors.New("conflicting canonical reasoning history")
		}
		if entry.ReasoningContent == "" {
			entry.ReasoningContent = entry.Reasoning
		}
		entry.Reasoning = ""
		if entry.Role == "user" {
			omit, err := omitEmptyOutboundUser(entry)
			if err != nil {
				return nil, err
			}
			if omit {
				continue
			}
		}
		if entry.Role == "assistant" && claude && len(entry.ReasoningDetails) == 0 {
			entry.ReasoningContent = ""
		}
		if entry.Role == "assistant" && len(entry.ToolCalls) == 0 {
			text, images, err := encodeOutboundContent(entry.Content)
			if err != nil {
				return nil, err
			}
			if text == "" && len(images) == 0 && len(entry.ReasoningDetails) == 0 {
				continue
			}
		}
		if len(result) > 0 && entry.Role == "assistant" && result[len(result)-1].Role == "assistant" &&
			len(result[len(result)-1].ToolCalls) > 0 {
			previous := &result[len(result)-1]
			if previous.blockBoundary || entry.blockBoundary || !bytes.Equal(bytes.TrimSpace(previous.Cache), bytes.TrimSpace(entry.Cache)) {
				return nil, errors.New("split assistant cache boundaries require the native paired route")
			}
			first, firstImages, err := encodeOutboundContent(previous.Content)
			if err != nil {
				return nil, err
			}
			second, secondImages, err := encodeOutboundContent(entry.Content)
			if err != nil || len(firstImages)+len(secondImages) != 0 {
				return nil, errors.New("split assistant images have no canonical native mapping")
			}
			previous.Content, err = json.Marshal(joinHistoryText(first, second))
			if err != nil {
				return nil, err
			}
			previous.ReasoningContent = joinHistoryText(previous.ReasoningContent, entry.ReasoningContent)
			if len(entry.ReasoningDetails) > 0 {
				if len(previous.ReasoningDetails) > 0 {
					return nil, errors.New("split assistant signatures require the native paired route")
				}
				previous.ReasoningDetails = entry.ReasoningDetails
			}
			previous.ToolCalls = append(append(previous.ToolCalls[:0:0], previous.ToolCalls...), entry.ToolCalls...)
			continue
		}
		result = append(result, entry)
	}
	return result, nil
}
