package devin

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

var toolImageNote = regexp.MustCompile(`(?:\n\n)?\[The tool returned (?:an image; it|([0-9]+) images; they) follows? in the next message\.\]$`)

type deferredToolImages struct {
	index int
	left  int
	text  string
	parts []json.RawMessage
}

func attachDeferredToolImages(messages []outboundMessage) ([]outboundMessage, error) {
	result := append([]outboundMessage(nil), messages...)
	results := map[string]*deferredToolImages{}
	empty := map[int]bool{}
	for index := range result {
		entry := &result[index]
		if entry.Role == "tool" {
			var text string
			if json.Unmarshal(entry.Content, &text) == nil {
				note := toolImageNote.FindStringSubmatchIndex(text)
				if note != nil {
					count := 1
					if note[2] >= 0 {
						var err error
						count, err = strconv.Atoi(text[note[2]:note[3]])
						if err != nil || count < 1 || count > 256 {
							return nil, errors.New("invalid deferred tool image count")
						}
					}
					results[entry.ToolCallID] = &deferredToolImages{index: index, left: count, text: text[:note[0]]}
				}
			}
			continue
		}
		if entry.Role != "user" {
			results = map[string]*deferredToolImages{}
			continue
		}
		var parts []json.RawMessage
		if len(results) == 0 || json.Unmarshal(entry.Content, &parts) != nil {
			results = map[string]*deferredToolImages{}
			continue
		}
		var target *deferredToolImages
		var remaining []json.RawMessage
		for _, part := range parts {
			var block struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal(part, &block) != nil {
				return nil, errors.New("invalid deferred tool image block")
			}
			if block.Type == "text" {
				target = nil
				if strings.HasPrefix(block.Text, "[From the result of ") {
					for id, candidate := range results {
						if candidate.left > 0 && (block.Text == "[From the result of tool call "+id+":]" ||
							strings.HasSuffix(block.Text, " (tool call "+id+"):]")) {
							target = candidate
							break
						}
					}
				}
				if target != nil {
					continue
				}
			}
			if block.Type == "image_url" && target != nil {
				if _, images, err := encodeOutboundContent(append(append([]byte{'['}, part...), ']')); err != nil || len(images) != 1 {
					return nil, errors.New("deferred tool image must be a supported inline image")
				}
				target.parts = append(target.parts, part)
				target.left--
				if target.left == 0 {
					target = nil
				}
				continue
			}
			remaining = append(remaining, part)
		}
		for _, candidate := range results {
			if candidate.left != 0 {
				return nil, errors.New("deferred tool result images are incomplete")
			}
			text, _ := json.Marshal(map[string]string{"type": "text", "text": candidate.text})
			body := append([]json.RawMessage{text}, candidate.parts...)
			result[candidate.index].Content, _ = json.Marshal(body)
		}
		entry.Content, _ = json.Marshal(remaining)
		omit, err := omitEmptyOutboundUser(*entry)
		if err != nil {
			return nil, err
		}
		empty[index] = omit
		results = map[string]*deferredToolImages{}
	}
	filtered := result[:0]
	for index, entry := range result {
		if !empty[index] {
			filtered = append(filtered, entry)
		}
	}
	return filtered, nil
}
