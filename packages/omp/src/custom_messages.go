package ompsessions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func ompCustomMessageItem(entry ompCarryEntry, at time.Time, blobs *ompCarryBlobs) (Item, error) {
	kind, blocks, err := ompCustomMessageContent(entry.CustomMessage)
	if err != nil || kind == "" {
		return Item{}, err
	}
	item := Item{Kind: kind, Time: at}
	for _, block := range blocks {
		switch block.Type {
		case "text":
			item.Content = append(item.Content, Content{Type: "text", Text: block.Text})
		case "image":
			image, err := blobs.image(block)
			if err != nil {
				return Item{}, err
			}
			item.Content = append(item.Content, image)
		}
	}
	return item, nil
}

func ompCustomMessageContent(raw json.RawMessage) (string, []ompCarryBlock, error) {
	fields, err := ompUniqueFields(raw, "excludeFromContext", "customType", "attribution", "content")
	if err != nil {
		return "", nil, err
	}
	if exclusion, exists := fields["excludeFromContext"]; exists {
		switch string(exclusion) {
		case "true":
			return "", nil, nil
		case "false":
		default:
			return "", nil, errors.New("OMP custom message excludeFromContext must be a boolean")
		}
	}
	kind := "user"
	switch ompJSONString(fields["customType"]) {
	case "collab-prompt":
		kind = "steering"
	case "skill-prompt":
	default:
		return "", nil, nil
	}
	if ompJSONString(fields["attribution"]) != "user" {
		return "", nil, nil
	}
	content := fields["content"]
	var text *string
	if json.Unmarshal(content, &text) == nil && text != nil {
		if *text == "" {
			return "", nil, nil
		}
		return kind, []ompCarryBlock{{Type: "text", Text: *text}}, nil
	}
	var values []json.RawMessage
	if json.Unmarshal(content, &values) != nil || values == nil {
		return "", nil, errors.New("OMP custom message content must be text or an array of blocks")
	}
	blocks := make([]ompCarryBlock, 0, len(values))
	for _, value := range values {
		blockFields, err := ompUniqueFields(value, "type", "text", "data", "mimeType")
		if err != nil {
			return "", nil, err
		}
		block := ompCarryBlock{Type: ompJSONString(blockFields["type"])}
		switch block.Type {
		case "text":
			var text *string
			if json.Unmarshal(blockFields["text"], &text) != nil || text == nil {
				return "", nil, errors.New("OMP custom message text block requires text")
			}
			block.Text = *text
		case "image":
			block.Data = ompJSONString(blockFields["data"])
			block.MimeType = ompJSONString(blockFields["mimeType"])
			if block.Data == "" || block.MimeType == "" {
				return "", nil, errors.New("OMP custom message image is missing data or media type")
			}
		default:
			return "", nil, errors.New("unsupported OMP custom message content block")
		}
		blocks = append(blocks, block)
	}
	if len(blocks) == 0 {
		return "", nil, nil
	}
	return kind, blocks, nil
}

func ompUniqueFields(raw json.RawMessage, keys ...string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if opening, err := decoder.Token(); err != nil || opening != json.Delim('{') {
		return nil, errors.New("invalid OMP custom message object")
	}
	fields := make(map[string]json.RawMessage, len(keys))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key := token.(string)
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		for _, canonical := range keys {
			if !strings.EqualFold(key, canonical) {
				continue
			}
			if key != canonical || fields[canonical] != nil {
				return nil, fmt.Errorf("ambiguous OMP custom message %s", canonical)
			}
			fields[key] = value
			break
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return fields, nil
}
