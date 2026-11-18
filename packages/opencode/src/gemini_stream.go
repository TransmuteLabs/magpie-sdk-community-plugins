package opencode

import (
	"bytes"
	"encoding/json"
	"errors"
)

type geminiTerminal struct {
	line     []byte
	data     []byte
	hasData  bool
	expected int
	finished map[int]bool
	blocked  bool
}

func (terminal *geminiTerminal) configure(body map[string]json.RawMessage) error {
	var contents []json.RawMessage
	if json.Unmarshal(body["contents"], &contents) != nil || len(contents) == 0 {
		return errors.New("Gemini generation requires contents")
	}
	var config struct {
		CandidateCount *int `json:"candidateCount"`
	}
	if len(body["generationConfig"]) != 0 && json.Unmarshal(body["generationConfig"], &config) != nil {
		return errors.New("invalid Gemini generation configuration")
	}
	terminal.expected = 1
	if config.CandidateCount != nil {
		terminal.expected = *config.CandidateCount
	}
	if terminal.expected < 1 || terminal.expected > 1024 {
		return errors.New("Gemini candidate count exceeds the module observation limit")
	}
	terminal.finished = map[int]bool{}
	return nil
}

func (terminal *geminiTerminal) write(body []byte) error {
	for _, value := range body {
		if value != '\n' {
			if len(terminal.line)+len(terminal.data) >= maxReplyBytes {
				return errors.New("Gemini stream event exceeds its byte limit")
			}
			terminal.line = append(terminal.line, value)
			continue
		}
		line := bytes.TrimSuffix(terminal.line, []byte("\r"))
		if len(line) == 0 {
			if terminal.hasData {
				if err := terminal.event(); err != nil {
					return err
				}
			}
			terminal.data, terminal.hasData = terminal.data[:0], false
		} else if data, found := bytes.CutPrefix(line, []byte("data:")); found {
			if terminal.hasData {
				terminal.data = append(terminal.data, '\n')
			}
			terminal.hasData = true
			terminal.data = append(terminal.data, bytes.TrimPrefix(data, []byte(" "))...)
		}
		terminal.line = terminal.line[:0]
	}
	return nil
}

func (terminal *geminiTerminal) event() error {
	var event struct {
		Candidates []struct {
			Index        int    `json:"index"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		PromptFeedback struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
	}
	if json.Unmarshal(terminal.data, &event) != nil {
		return errors.New("invalid Gemini stream event")
	}
	if event.PromptFeedback.BlockReason != "" && event.PromptFeedback.BlockReason != "BLOCK_REASON_UNSPECIFIED" {
		terminal.blocked = true
	}
	for _, candidate := range event.Candidates {
		if candidate.Index < 0 || candidate.Index >= terminal.expected {
			return errors.New("Gemini stream returned an unexpected candidate")
		}
		if candidate.FinishReason != "" && candidate.FinishReason != "FINISH_REASON_UNSPECIFIED" {
			terminal.finished[candidate.Index] = true
		}
	}
	return nil
}

func (terminal *geminiTerminal) complete() bool {
	return terminal.blocked || len(terminal.finished) == terminal.expected
}
