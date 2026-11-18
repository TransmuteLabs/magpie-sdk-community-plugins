package ompsessions

import (
	"encoding/json"
	"errors"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func sourceEntryParts(entry ompCarryEntry) ([]sdk.SessionSourcePart, error) {
	if entry.Type == "compaction" || entry.Type == "branch_summary" {
		return []sdk.SessionSourcePart{{Role: "user", Kind: "context", Text: entry.Summary}}, nil
	}
	if entry.Type != "message" {
		return nil, nil
	}
	var message sourceMessage
	if err := json.Unmarshal(entry.Message, &message); err != nil {
		return nil, err
	}
	role := message.Role
	kind := "text"
	switch role {
	case "system", "developer":
		return nil, nil
	case "fileMention":
		var parts []sdk.SessionSourcePart
		for _, file := range message.Files {
			if file.Image != nil {
				if err := sourceImage(*file.Image); err != nil {
					return nil, err
				}
				parts = append(parts, sdk.SessionSourcePart{Role: "user", Kind: "image", Text: "[image]"})
			}
		}
		return parts, nil
	case "bashExecution", "pythonExecution":
		name, command := "bash", message.Command
		if role == "pythonExecution" {
			name, command = "python", message.Code
		}
		output := message.Output
		if message.Cancelled || message.ExitCode != nil && *message.ExitCode != 0 {
			output = "[error] " + output
		}
		return []sdk.SessionSourcePart{
			{Role: "assistant", Kind: "tool_use", Name: name, Text: command},
			{Role: "tool", Kind: "tool_result", Name: name, Text: output},
		}, nil
	case "toolResult":
		role, kind = "tool", "tool_result"
	case "user", "assistant":
	default:
		return nil, errors.New("unsupported OMP Source role")
	}
	var text string
	if json.Unmarshal(message.Content, &text) == nil {
		if message.IsError {
			text = "[error] " + text
		}
		return []sdk.SessionSourcePart{{Role: role, Kind: kind, Name: message.ToolName, Text: text, Model: message.Model}}, nil
	}
	var blocks []ompCarryBlock
	if err := json.Unmarshal(message.Content, &blocks); err != nil {
		return nil, errors.New("unsupported OMP Source content")
	}
	var parts []sdk.SessionSourcePart
	for _, block := range blocks {
		part := sdk.SessionSourcePart{Role: role, Kind: kind, Text: block.Text, Model: message.Model, Name: message.ToolName}
		switch block.Type {
		case "text":
			if message.IsError {
				part.Text = "[error] " + part.Text
			}
		case "image":
			if err := sourceImage(block); err != nil {
				return nil, err
			}
			part.Kind, part.Text, part.Name = "image", "[image]", ""
		case "thinking":
			if role != "assistant" {
				return nil, errors.New("non-assistant thinking block")
			}
			part.Kind, part.Text = "thinking", block.Thinking
		case "redactedThinking":
			if role != "assistant" {
				return nil, errors.New("non-assistant redacted thinking block")
			}
			continue
		case "toolCall":
			if role != "assistant" || block.ID == "" || block.Name == "" {
				return nil, errors.New("invalid OMP Source tool call")
			}
			arguments := block.Arguments
			if !ompCarryPresent(arguments) {
				arguments = block.Input
			}
			if !json.Valid(arguments) {
				return nil, errors.New("invalid OMP Source tool arguments")
			}
			part.Kind, part.Name, part.Text = "tool_use", block.Name, pretty(arguments)
		default:
			return nil, errors.New("unsupported OMP Source content block")
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func sourceImage(block ompCarryBlock) error {
	if block.MimeType == "" || block.Data == "" {
		return errors.New("OMP Source image requires MIME and data")
	}
	blob := false
	if err := visitImageReference(block.Data, func(string) error { blob = true; return nil }); err != nil {
		return err
	}
	if !blob {
		if _, err := decodeImage(block.Data); err != nil {
			return err
		}
	}
	return nil
}
