package mcpwire

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"strings"
	"unicode/utf8"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
)

const Version = "2025-06-18"
const MaxBytes = 4 << 20
const MaxEvents = 256

var ErrProtocol = errors.New("unsupported or malformed MCP response")
var ErrLimit = errors.New("MCP response exceeds its bounds")
var ErrServerRequest = errors.New("MCP server-initiated requests are not supported by this fixed client")

type RPCError struct {
	Code int64
}

func (err *RPCError) Error() string {
	return "MCP server returned a JSON-RPC error"
}

func Request(id, method string, params any) ([]byte, error) {
	if id == "" || len(id) > 128 || method != "initialize" && method != "tools/list" && method != "tools/call" {
		return nil, ErrProtocol
	}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil || len(body) > MaxBytes {
		return nil, ErrLimit
	}
	return body, nil
}

func Notification(method string, params any) ([]byte, error) {
	if method != "notifications/initialized" && method != "notifications/cancelled" {
		return nil, ErrProtocol
	}
	value := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		value["params"] = params
	}
	return json.Marshal(value)
}

func decode(body []byte, id string, notifications bool) (json.RawMessage, bool, error) {
	if len(body) > MaxBytes || !utf8.Valid(body) {
		return nil, false, ErrLimit
	}
	value, err := protocol.ReadObject(body)
	if err != nil || value["jsonrpc"] != "2.0" {
		return nil, false, ErrProtocol
	}
	if method, exists := value["method"]; exists {
		if _, exists := value["id"]; exists {
			return nil, false, ErrServerRequest
		}
		name, valid := method.(string)
		_, hasResult := value["result"]
		_, hasError := value["error"]
		if !notifications || !valid || !strings.HasPrefix(name, "notifications/") ||
			hasResult || hasError {
			return nil, false, ErrProtocol
		}
		return nil, false, nil
	}
	actual, valid := value["id"].(string)
	if !valid || actual != id {
		return nil, false, ErrProtocol
	}
	_, hasResult := value["result"]
	failure, hasError := value["error"]
	if hasResult == hasError {
		return nil, false, ErrProtocol
	}
	if hasError {
		object, valid := failure.(map[string]any)
		number, numberValid := object["code"].(json.Number)
		_, messageValid := object["message"].(string)
		code, numberErr := number.Int64()
		if !valid || !numberValid || !messageValid || numberErr != nil {
			return nil, false, ErrProtocol
		}
		return nil, false, &RPCError{Code: code}
	}
	var raw struct {
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal(body, &raw) != nil || bytes.Equal(raw.Result, []byte("null")) {
		return nil, false, ErrProtocol
	}
	return raw.Result, true, nil
}

func Read(reader io.Reader, contentType, id string) (json.RawMessage, error) {
	kind, parameters, err := mime.ParseMediaType(contentType)
	if err != nil || parameters["charset"] != "" && !strings.EqualFold(parameters["charset"], "utf-8") {
		return nil, ErrProtocol
	}
	limited := &io.LimitedReader{R: reader, N: MaxBytes + 1}
	switch kind {
	case "application/json":
		body, err := io.ReadAll(limited)
		if err != nil {
			return nil, err
		}
		if len(body) > MaxBytes {
			return nil, ErrLimit
		}
		value, terminal, err := decode(body, id, false)
		if err != nil {
			return nil, err
		}
		if !terminal {
			return nil, ErrProtocol
		}
		return value, nil
	case "text/event-stream":
		scanner := bufio.NewScanner(limited)
		scanner.Buffer(make([]byte, 16<<10), 256<<10)
		var data []string
		event := ""
		events, lines := 0, 0
		for scanner.Scan() {
			lines++
			line := scanner.Text()
			if lines > 8192 || limited.N <= 0 || !utf8.ValidString(line) {
				return nil, ErrLimit
			}
			if line == "" {
				if len(data) == 0 {
					event = ""
					continue
				}
				events++
				if events > MaxEvents || event != "" && event != "message" {
					return nil, ErrProtocol
				}
				value, terminal, err := decode([]byte(strings.Join(data, "\n")), id, true)
				if err != nil {
					return nil, err
				}
				if terminal {
					return value, nil
				}
				data, event = nil, ""
				continue
			}
			if strings.HasPrefix(line, ":") {
				continue
			}
			name, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch name {
			case "data":
				data = append(data, value)
			case "event":
				event = value
			case "id", "retry":
			default:
				return nil, ErrProtocol
			}
		}
		if scanner.Err() != nil {
			return nil, scanner.Err()
		}
		return nil, ErrProtocol
	default:
		return nil, ErrProtocol
	}
}

func Session(headers []string) (string, error) {
	if len(headers) == 0 {
		return "", nil
	}
	if len(headers) != 1 || len(headers[0]) == 0 || len(headers[0]) > 256 {
		return "", ErrProtocol
	}
	for _, character := range headers[0] {
		if character < 0x21 || character > 0x7e {
			return "", ErrProtocol
		}
	}
	return headers[0], nil
}
