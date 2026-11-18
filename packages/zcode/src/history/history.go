package history

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
)

const (
	NativeJSONL   = "zcode-native-jsonl"
	LegacySession = "zcode-legacy-session"
	CLILogJSONL   = "zcode-cli-log-jsonl"
	Bundle        = "zcode-history-bundle-v1"
	MaxBytes      = 8 << 20
	MaxRecords    = 16384
)

var ErrInvalid = errors.New("invalid or unsupported offline ZCode history")
var ErrLimit = errors.New("offline ZCode history exceeds its bounds")
var ErrConversion = errors.New("unsupported history conversion; live resume and lossy cross-agent conversion are not provided")

type Summary struct {
	Format                string   `json:"format"`
	SHA256                string   `json:"sha256"`
	Records               int      `json:"records"`
	UserMessages          int      `json:"userMessages"`
	AssistantMessages     int      `json:"assistantMessages"`
	ToolBlocks            int      `json:"toolBlocks"`
	LegacyToolEntries     int      `json:"legacyToolEntries"`
	HasThinking           bool     `json:"hasThinking"`
	HasExternalReferences bool     `json:"hasExternalReferences"`
	OpaqueRecords         int      `json:"opaqueRecords"`
	SessionIDs            []string `json:"sessionIds,omitempty"`
	IncompleteTail        bool     `json:"incompleteTail,omitempty"`
}

type Document struct {
	format  string
	source  []byte
	records []json.RawMessage
	summary Summary
}

type bundle struct {
	Kind         string `json:"kind"`
	Version      int    `json:"version"`
	SourceFormat string `json:"sourceFormat"`
	Source       []byte `json:"source"`
	SHA256       string `json:"sha256"`
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (document Document) Source() []byte {
	return bytes.Clone(document.source)
}

func (document Document) Summary() Summary {
	summary := document.summary
	summary.SessionIDs = slices.Clone(summary.SessionIDs)
	return summary
}

func (document Document) Records() []json.RawMessage {
	records := make([]json.RawMessage, len(document.records))
	for index, record := range document.records {
		records[index] = bytes.Clone(record)
	}
	return records
}

func text(value any) string {
	valueText, _ := value.(string)
	return strings.TrimSpace(valueText)
}

func timestamp(value any) bool {
	number, valid := value.(json.Number)
	if !valid {
		return false
	}
	millis, err := number.Int64()
	return err == nil && millis >= 0
}

func scanFeatures(value any, summary *Summary) {
	switch value := value.(type) {
	case map[string]any:
		switch value["type"] {
		case "tool_use", "tool_result":
			summary.ToolBlocks++
		case "thinking", "redacted_thinking":
			summary.HasThinking = true
		}
		for name, item := range value {
			switch name {
			case "thought", "thinking", "signature", "thinkingSignature":
				if item != nil {
					summary.HasThinking = true
				}
			case "bodyRefs", "contentRefs":
				if values, valid := item.([]any); valid && len(values) > 0 {
					summary.HasExternalReferences = true
				}
			}
			scanFeatures(item, summary)
		}
	case []any:
		for _, item := range value {
			scanFeatures(item, summary)
		}
	}
}

func Read(format string, body []byte) (Document, error) {
	if format == Bundle {
		return readBundle(body)
	}
	if len(body) == 0 || len(body) > MaxBytes || !utf8.Valid(body) {
		return Document{}, ErrLimit
	}
	document := Document{format: format, source: bytes.Clone(body)}
	document.summary = Summary{Format: format, SHA256: digest(body)}
	sessions := map[string]bool{}
	switch format {
	case NativeJSONL, CLILogJSONL:
		input := body
		if format == CLILogJSONL && !bytes.HasSuffix(input, []byte("\n")) {
			document.summary.IncompleteTail = true
			input = input[:bytes.LastIndexByte(input, '\n')+1]
		}
		scanner := bufio.NewScanner(bytes.NewReader(input))
		scanner.Buffer(make([]byte, 64<<10), MaxBytes)
		lines := 0
		for scanner.Scan() {
			lines++
			if lines > MaxRecords*4 {
				return Document{}, ErrLimit
			}
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			value, err := protocol.ReadObject(line)
			if err != nil {
				return Document{}, ErrInvalid
			}
			if format == CLILogJSONL {
				if _, err := time.Parse(time.RFC3339Nano, text(value["timestamp"])); err != nil ||
					!slices.Contains([]string{"debug", "info", "warn", "error"}, text(value["level"])) {
					return Document{}, ErrInvalid
				}
			} else {
				kind := text(value["type"])
				if kind == "" {
					return Document{}, ErrInvalid
				}
				message, _ := value["message"].(map[string]any)
				role := text(message["role"])
				if kind == "user" && role == "user" {
					document.summary.UserMessages++
				} else if kind == "assistant" && role == "assistant" {
					document.summary.AssistantMessages++
				} else {
					document.summary.OpaqueRecords++
				}
				scanFeatures(value, &document.summary)
			}
			if session := text(value["sessionId"]); session != "" {
				sessions[session] = true
			}
			document.records = append(document.records, bytes.Clone(line))
			if len(document.records) > MaxRecords {
				return Document{}, ErrLimit
			}
		}
		if scanner.Err() != nil {
			return Document{}, ErrLimit
		}
	case LegacySession:
		value, err := protocol.ReadObject(body)
		if err != nil {
			return Document{}, ErrInvalid
		}
		meta, valid := value["meta"].(map[string]any)
		messages, messagesValid := value["messages"].([]any)
		if !valid || !messagesValid || text(meta["taskId"]) == "" || text(meta["traceId"]) == "" ||
			text(meta["workspacePath"]) == "" || !timestamp(meta["createdAt"]) || !timestamp(meta["updatedAt"]) ||
			len(messages) > MaxRecords {
			return Document{}, ErrInvalid
		}
		sessions[text(meta["taskId"])] = true
		var raw struct {
			Messages []json.RawMessage `json:"messages"`
		}
		if json.Unmarshal(body, &raw) != nil {
			return Document{}, ErrInvalid
		}
		for index, item := range messages {
			message, valid := item.(map[string]any)
			if !valid {
				return Document{}, ErrInvalid
			}
			if _, valid := message["content"].(string); !valid || !timestamp(message["timestamp"]) {
				return Document{}, ErrInvalid
			}
			switch message["role"] {
			case "user":
				document.summary.UserMessages++
			case "assistant":
				document.summary.AssistantMessages++
			default:
				return Document{}, ErrInvalid
			}
			if tools, present := message["tools"]; present {
				values, valid := tools.([]any)
				if !valid {
					return Document{}, ErrInvalid
				}
				document.summary.LegacyToolEntries += len(values)
			}
			document.records = append(document.records, bytes.Clone(raw.Messages[index]))
		}
		scanFeatures(value, &document.summary)
	default:
		return Document{}, ErrInvalid
	}
	if len(document.records) == 0 && format == NativeJSONL {
		return Document{}, ErrInvalid
	}
	document.summary.Records = len(document.records)
	for session := range sessions {
		document.summary.SessionIDs = append(document.summary.SessionIDs, session)
	}
	sort.Strings(document.summary.SessionIDs)
	return document, nil
}

func readBundle(body []byte) (Document, error) {
	if len(body) == 0 || len(body) > 12<<20 || !utf8.Valid(body) {
		return Document{}, ErrLimit
	}
	if _, err := protocol.ReadObject(body); err != nil {
		return Document{}, ErrInvalid
	}
	var value bundle
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF ||
		value.Kind != "zcode-history" || value.Version != 1 ||
		value.SourceFormat != NativeJSONL && value.SourceFormat != LegacySession && value.SourceFormat != CLILogJSONL ||
		value.SHA256 != digest(value.Source) {
		return Document{}, ErrInvalid
	}
	return Read(value.SourceFormat, value.Source)
}

func Convert(from, to string, body []byte) ([]byte, error) {
	document, err := Read(from, body)
	if err != nil {
		return nil, err
	}
	if from == to || to == document.format {
		if from == to {
			return bytes.Clone(body), nil
		}
		return document.Source(), nil
	}
	if to != Bundle {
		return nil, ErrConversion
	}
	return json.Marshal(bundle{Kind: "zcode-history", Version: 1, SourceFormat: document.format,
		Source: document.source, SHA256: document.summary.SHA256})
}
