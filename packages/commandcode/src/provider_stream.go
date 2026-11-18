package commandcode

import (
	"bytes"
	"encoding/json"
	"errors"
)

type providerStreamCompletion struct {
	protocol  string
	line      []byte
	event     []byte
	ended     bool
	malformed bool
}

func (completion *providerStreamCompletion) write(data []byte) {
	for len(data) > 0 {
		newline := bytes.IndexByte(data, '\n')
		if newline < 0 {
			completion.line = append(completion.line, data...)
			return
		}
		completion.line = append(completion.line, data[:newline]...)
		completion.readLine()
		completion.line = completion.line[:0]
		data = data[newline+1:]
	}
}

func (completion *providerStreamCompletion) readLine() {
	line := bytes.TrimSuffix(completion.line, []byte{'\r'})
	if len(line) == 0 {
		completion.readEvent()
	} else if bytes.HasPrefix(line, []byte("data:")) {
		if len(completion.event) > 0 {
			completion.event = append(completion.event, '\n')
		}
		completion.event = append(completion.event, bytes.TrimPrefix(line[5:], []byte(" "))...)
	}
}

func (completion *providerStreamCompletion) readEvent() {
	data := bytes.TrimSpace(completion.event)
	defer func() { completion.event = completion.event[:0] }()
	if len(data) == 0 {
		return
	}
	if completion.protocol == "chat" && bytes.Equal(data, []byte("[DONE]")) {
		completion.ended = true
		return
	}
	switch completion.protocol {
	case "chat":
		var event struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if json.Unmarshal(data, &event) != nil {
			completion.malformed = true
			return
		}
		for _, choice := range event.Choices {
			completion.ended = completion.ended || choice.FinishReason != ""
		}
	case "responses":
		var event struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &event) != nil {
			completion.malformed = true
			return
		}
		switch event.Type {
		case "response.completed", "response.incomplete", "response.failed", "error":
			completion.ended = true
		}
	case "anthropic":
		var event struct {
			Type  string `json:"type"`
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
		}
		if json.Unmarshal(data, &event) != nil {
			completion.malformed = true
			return
		}
		completion.ended = completion.ended || event.Type == "message_stop" || event.Type == "error" ||
			event.Type == "message_delta" && event.Delta.StopReason != ""
	}
}

func (completion *providerStreamCompletion) finish() error {
	if len(completion.line) > 0 {
		completion.readLine()
	}
	completion.readEvent()
	if completion.malformed {
		return errors.New("CommandCode provider stream contains a malformed event; outcome is uncertain")
	}
	if !completion.ended {
		return errors.New("CommandCode provider stream ended without its protocol terminal; outcome is uncertain")
	}
	return nil
}
