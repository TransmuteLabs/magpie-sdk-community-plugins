package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type geminiTerminal struct {
	frame    []byte
	lineFrom int
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

func (terminal *geminiTerminal) write(body []byte, emit func([]byte) error) error {
	for _, value := range body {
		if len(terminal.frame) >= maxReplyBytes {
			return errors.New("Gemini stream event exceeds its byte limit")
		}
		terminal.frame = append(terminal.frame, value)
		if value != '\n' {
			continue
		}
		line := bytes.TrimSuffix(terminal.frame[terminal.lineFrom:len(terminal.frame)-1], []byte("\r"))
		if len(line) == 0 {
			if terminal.hasData {
				if err := terminal.event(); err != nil {
					return err
				}
			}
			if emit != nil {
				if err := emit(terminal.frame); err != nil {
					return err
				}
			}
			terminal.frame = terminal.frame[:0]
			terminal.data, terminal.hasData = terminal.data[:0], false
		} else if data, found := bytes.CutPrefix(line, []byte("data:")); found {
			if terminal.hasData {
				terminal.data = append(terminal.data, '\n')
			}
			terminal.hasData = true
			terminal.data = append(terminal.data, bytes.TrimPrefix(data, []byte(" "))...)
		}
		terminal.lineFrom = len(terminal.frame)
	}
	return nil
}

func (terminal *geminiTerminal) event() error {
	var event *struct {
		Error      *json.RawMessage `json:"error"`
		Candidates []struct {
			Index        int    `json:"index"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		PromptFeedback struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
	}
	if json.Unmarshal(terminal.data, &event) != nil || event == nil {
		return errors.New("invalid Gemini stream event")
	}
	if event.Error != nil {
		return errors.New("Gemini stream returned an error")
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
	return len(terminal.frame) == 0 &&
		(terminal.blocked || len(terminal.finished) == terminal.expected)
}

func (terminal *geminiTerminal) relay(ctx context.Context, reader io.Reader, stream *sdk.Stream) error {
	buffer := make([]byte, sdk.MaxChunkBytes)
	var emitErr error
	emit := func(frame []byte) error {
		emitErr = emitResponseData(ctx, stream, frame)
		return emitErr
	}
	for {
		count, readErr := reader.Read(buffer)
		if err := terminal.write(buffer[:count], emit); err != nil {
			if emitErr != nil {
				return emitErr
			}
			return failure(err, "uncertain")
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return failure(readErr, "uncertain")
		}
	}
	if !terminal.complete() {
		return &sdk.Error{Code: "truncated", Message: "Gemini stream ended without its terminal event", Outcome: "uncertain"}
	}
	return nil
}
