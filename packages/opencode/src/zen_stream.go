package opencode

import (
	"bytes"
	"encoding/json"
	"errors"
)

type zenTerminal struct {
	protocol string
	line     []byte
	data     []byte
	done     bool
}

func (terminal *zenTerminal) complete() bool {
	return terminal.done
}

func (terminal *zenTerminal) write(body []byte) error {
	for _, value := range body {
		if value != '\n' {
			if len(terminal.line)+len(terminal.data) >= maxReplyBytes {
				return errors.New("Zen SSE event exceeds its byte limit")
			}
			terminal.line = append(terminal.line, value)
			continue
		}
		line := bytes.TrimSuffix(terminal.line, []byte("\r"))
		if len(line) == 0 {
			if len(terminal.data) != 0 {
				if err := terminal.event(); err != nil {
					return err
				}
			}
			terminal.data = terminal.data[:0]
		} else if data, found := bytes.CutPrefix(line, []byte("data:")); found {
			if len(terminal.data) != 0 {
				terminal.data = append(terminal.data, '\n')
			}
			terminal.data = append(terminal.data, bytes.TrimPrefix(data, []byte(" "))...)
		}
		terminal.line = terminal.line[:0]
	}
	return nil
}

func (terminal *zenTerminal) event() error {
	if terminal.protocol == "chat" && bytes.Equal(terminal.data, []byte("[DONE]")) {
		terminal.done = true
		return nil
	}
	var event struct {
		Type     string          `json:"type"`
		Error    json.RawMessage `json:"error"`
		Response struct {
			Status string `json:"status"`
		} `json:"response"`
	}
	if json.Unmarshal(terminal.data, &event) != nil {
		return errors.New("invalid Zen SSE event")
	}
	if len(event.Error) != 0 && !bytes.Equal(event.Error, []byte("null")) || event.Type == "error" ||
		event.Type == "response.failed" || event.Type == "response.incomplete" {
		return errors.New("Zen stream reported an upstream error")
	}
	switch terminal.protocol {
	case "responses":
		terminal.done = terminal.done || event.Type == "response.completed" && event.Response.Status == "completed"
	case "messages":
		terminal.done = terminal.done || event.Type == "message_stop"
	}
	return nil
}
