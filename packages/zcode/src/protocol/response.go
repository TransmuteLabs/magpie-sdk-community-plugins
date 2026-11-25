package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
)

const MaxEvent = 1 << 20

var ErrTruncated = errors.New("ZCode stream ended without a complete message_stop")
var ErrStreamFailure = errors.New("ZCode stream reported an error after dispatch")
var ErrFrameTooLarge = errors.New("ZCode stream event exceeds its limit")

func IsProviderCodeShaped(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		valid := character >= 'A' && character <= 'Z' ||
			character >= 'a' && character <= 'z' ||
			character >= '0' && character <= '9' ||
			character == '_' || character == '.' || character == ':' || character == '-'
		if !valid {
			return false
		}
	}
	return true
}

func Failure(body []byte) (int, []byte, bool) {
	value, err := ReadObject(body)
	if err != nil {
		return 0, nil, false
	}
	var code string
	rawCode := value["code"]
	nested := object(value["error"])
	if rawCode == nil {
		rawCode = nested["code"]
	}
	switch raw := rawCode.(type) {
	case json.Number:
		if trimmed := strings.TrimSpace(string(raw)); IsProviderCodeShaped(trimmed) {
			code = trimmed
		}
	case string:
		if trimmed := strings.TrimSpace(raw); IsProviderCodeShaped(trimmed) {
			code = trimmed
		}
	}
	failure := value["success"] == false || code != "" && code != "0" && code != "200" ||
		value["type"] == "error" || value["error"] != nil
	if !failure {
		return 0, nil, false
	}
	status, kind := 502, "api_error"
	switch code {
	case "1005", "1113", "1302", "1303", "1304", "1305", "1308", "1310", "1313", "3002", "3008", "3009", "3010":
		status, kind = 429, "rate_limit_error"
	case "500", "1120", "1230", "1234", "1312", "2007":
		status, kind = 503, "overloaded_error"
	}
	if code == "" {
		switch nested["type"] {
		case "rate_limit_error":
			status, kind = 429, "rate_limit_error"
		case "overloaded_error":
			status, kind = 503, "overloaded_error"
		case "invalid_request_error":
			status, kind = 400, "invalid_request_error"
		case "authentication_error":
			status, kind = 401, "authentication_error"
		case "permission_error":
			status, kind = 403, "permission_error"
		}
	}
	if _, err := strconv.ParseInt(code, 10, 32); err != nil {
		code = ""
	}
	response := map[string]any{"type": "error", "error": map[string]any{
		"type": kind, "message": "ZCode rejected the request", "code": code,
	}}
	encoded, _ := json.Marshal(response)
	return status, encoded, true
}

func RelaySSE(input io.Reader, renamed bool, emit func([]byte) error, responseModel ...string) error {
	return RelaySSEObserved(input, renamed, emit, nil, responseModel...)
}

func RelaySSEObserved(input io.Reader, renamed bool, emit func([]byte) error, observe func([]byte) error, responseModel ...string) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), MaxEvent)
	lines, size := []string{}, 0
	for scanner.Scan() {
		line := scanner.Text()
		size += len(line) + 1
		if size > MaxEvent {
			return ErrFrameTooLarge
		}
		if line != "" {
			lines = append(lines, line)
			continue
		}
		if len(lines) == 0 {
			size = 0
			continue
		}
		var data []string
		for _, line := range lines {
			if strings.HasPrefix(line, "data:") {
				value := strings.TrimPrefix(line, "data:")
				data = append(data, strings.TrimPrefix(value, " "))
			}
		}
		if len(data) == 0 {
			if err := emit([]byte(strings.Join(lines, "\n") + "\n\n")); err != nil {
				return err
			}
			lines, size = nil, 0
			continue
		}
		body := []byte(strings.Join(data, "\n"))
		value, err := ReadObject(body)
		if err != nil {
			return ErrInvalid
		}
		if observe != nil {
			if err := observe(body); err != nil {
				return err
			}
		}
		_, failure, failed := Failure(body)
		if failed {
			if err := emit(append(append([]byte("event: error\ndata: "), failure...), '\n', '\n')); err != nil {
				return err
			}
			return ErrStreamFailure
		}
		restored, err := Restore(body, renamed, responseModel...)
		if err != nil {
			return err
		}
		if len(restored) > MaxEvent {
			return ErrFrameTooLarge
		}
		var compact bytes.Buffer
		if json.Compact(&compact, restored) != nil {
			return ErrInvalid
		}
		restored = compact.Bytes()
		var frame bytes.Buffer
		wrote := false
		for _, line := range lines {
			if strings.HasPrefix(line, "data:") {
				if !wrote {
					frame.WriteString("data: ")
					frame.Write(restored)
					frame.WriteByte('\n')
					wrote = true
				}
				continue
			}
			frame.WriteString(line)
			frame.WriteByte('\n')
		}
		frame.WriteByte('\n')
		if err := emit(frame.Bytes()); err != nil {
			return err
		}
		if value["type"] == "message_stop" {
			return nil
		}
		lines, size = nil, 0
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return ErrTruncated
}
