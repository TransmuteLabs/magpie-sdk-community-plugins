package devin

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"unicode/utf8"
)

type NativeUsage struct {
	Input      uint64
	Output     uint64
	CacheWrite uint64
	CacheRead  uint64
	// Zero denotes a complete field7 protobuf snapshot. Nonzero bits name
	// only the counters actually read from the field28 fallback.
	dimensions uint8
}

type NativeToolDelta struct {
	ID        string
	Name      string
	Arguments string
}

type NativeDelta struct {
	ID             string
	Text           string
	Thinking       string
	ThinkingID     string
	Signature      string
	SignatureType  string
	Phase          string
	ActualModelUID string
	Tools          []NativeToolDelta
	Stop           uint64
	Usage          *NativeUsage
	Facts          NativeExecutionFacts
	Raw            []byte
}

type NativeSummary struct {
	Stop  uint64
	Usage *NativeUsage
	Facts NativeExecutionFacts
}

type NativeError struct {
	Code         string
	Failure      *ResponseFailure
	BeforeEffect bool
}

func (err *NativeError) Error() string {
	switch err.Code {
	case "unauthenticated", "permission_denied", "resource_exhausted", "invalid_argument",
		"unavailable", "deadline_exceeded", "canceled", "unimplemented", "data_loss":
		return "Devin native stream failed: " + err.Code
	default:
		return "Devin native stream failed"
	}
}

func EncodeConnectRequest(payload []byte) ([]byte, bool, error) {
	if len(payload) > maxBody {
		return nil, false, errors.New("native request exceeds its byte limit")
	}
	flags := byte(0)
	body := payload
	if len(payload) > 8<<10 {
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		if _, err := writer.Write(payload); err != nil {
			return nil, false, err
		}
		if err := writer.Close(); err != nil {
			return nil, false, err
		}
		body, flags = compressed.Bytes(), 1
	}
	framed := []byte{flags, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(framed[1:], uint32(len(body)))
	return append(framed, body...), flags == 1, nil
}

func ReadConnectFrame(reader io.Reader, gzipAllowed bool) (byte, []byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return 0, nil, err
	}
	flags := header[0]
	size := binary.BigEndian.Uint32(header[1:])
	if flags > 3 || size > maxBody || flags&1 != 0 && !gzipAllowed {
		return 0, nil, &NativeError{Code: "data_loss"}
	}
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return 0, nil, &NativeError{Code: "data_loss"}
	}
	if flags&1 != 0 {
		source := bytes.NewReader(payload)
		compressed, err := gzip.NewReader(source)
		if err != nil {
			return 0, nil, &NativeError{Code: "data_loss"}
		}
		compressed.Multistream(false)
		expanded, readErr := io.ReadAll(io.LimitReader(compressed, maxBody+1))
		closeErr := compressed.Close()
		if readErr != nil || closeErr != nil || source.Len() != 0 || len(expanded) > maxBody {
			return 0, nil, &NativeError{Code: "data_loss"}
		}
		payload = expanded
	}
	if flags&2 != 0 && len(payload) > 64<<10 {
		return 0, nil, &NativeError{Code: "data_loss"}
	}
	return flags, payload, nil
}

func projectedMessage(payload []byte, schema map[int]int) (message, error) {
	if len(payload) > maxBody {
		return nil, errors.New("native message exceeds its byte limit")
	}
	fields, err := readMessage(payload, nil)
	if err != nil {
		return nil, errors.New("malformed native message")
	}
	for number, spec := range schema {
		for _, item := range fields[number] {
			wire := spec & 7
			if wire == textField {
				wire = 2
			}
			if item.w != wire || spec&repeated == 0 && len(fields[number]) != 1 ||
				spec&7 == textField && !utf8.Valid(item.b) {
				return nil, errors.New("invalid native field type or cardinality")
			}
		}
	}
	return fields, nil
}

func nativeResponseFields(payload []byte) (message, error) {
	fields, err := projectedMessage(payload, map[int]int{
		1: textField, 2: 2, 3: textField, 4: 0, 5: 0, 6: 2 | repeated, 7: 2, 8: 0,
		9: textField, 10: textField, 11: 0, 12: 1, 13: 2, 14: 0, 15: textField,
		16: textField, 17: textField, 18: 0, 19: textField, 20: 2, 21: textField,
		22: 1, 23: textField, 24: 0, 25: textField, 26: 0, 27: 0, 28: 2 | repeated,
		29: 2 | repeated, 30: 2,
	})
	if err != nil {
		return nil, &NativeError{Code: "data_loss"}
	}
	for number := range fields {
		if number > 30 {
			return nil, &NativeError{Code: "unimplemented"}
		}
	}
	if fields.number(5) > 13 {
		return nil, &NativeError{Code: "unimplemented"}
	}
	if fields.number(4) > math.MaxUint32 || fields.number(8) > 1 || fields.number(11) > 1 ||
		fields.number(24) > 1 || len(fields[6]) > 256 {
		return nil, &NativeError{Code: "data_loss"}
	}
	for _, item := range fields[6] {
		call, err := readMessage(item.b, map[int]int{1: textField, 2: textField, 3: textField, 4: textField, 5: textField, 6: 0})
		if err != nil || call.number(6) > 1 ||
			call.text(1) == "" && call.text(2) == "" && call.text(3) == "" &&
				call.text(4) == "" && call.text(5) == "" && call.number(6) == 0 {
			return nil, &NativeError{Code: "data_loss"}
		}
	}
	return fields, nil
}

func nativeUsage(payload []byte) (*NativeUsage, error) {
	usage, err := readMessage(payload, map[int]int{
		1: 0, 2: 0, 3: 0, 4: 0, 5: 0, 6: 0, 7: textField,
		8: 2 | repeated, 9: textField, 10: textField, 11: textField, 12: 0, 13: 0,
	})
	if err != nil || usage.number(12) > 1 || usage.number(13) > 1 {
		return nil, &NativeError{Code: "data_loss"}
	}
	if usage.number(12) != 0 {
		return nil, &NativeError{Code: "unimplemented"}
	}
	for _, header := range usage[8] {
		if _, err := readMessage(header.b, map[int]int{1: textField, 2: textField}); err != nil {
			return nil, &NativeError{Code: "data_loss"}
		}
	}
	for _, number := range []int{2, 3, 4, 5} {
		if usage.number(number) > math.MaxInt32 {
			return nil, &NativeError{Code: "data_loss"}
		}
	}
	return &NativeUsage{Input: usage.number(2), Output: usage.number(3), CacheWrite: usage.number(4), CacheRead: usage.number(5)}, nil
}

func nativeCompletion(stop uint64, content, tools bool) error {
	switch stop {
	case 2, 3, 4, 5, 6, 8:
	case 10:
		if !tools {
			return &NativeError{Code: "data_loss"}
		}
	case 11:
		return &NativeError{Code: "permission_denied"}
	case 7, 13:
		return &NativeError{Code: "internal"}
	case 0, 1, 9, 12:
		return &NativeError{Code: "data_loss"}
	default:
		return &NativeError{Code: "unimplemented"}
	}
	if !content {
		return &NativeError{Code: "data_loss"}
	}
	return nil
}

func DecodeNativeDelta(payload []byte) (NativeDelta, error) {
	fields, err := nativeResponseFields(payload)
	if err != nil {
		return NativeDelta{}, err
	}
	if fields.number(8) != 0 || fields.number(11) != 0 || len(fields.data(20)) != 0 ||
		fields.number(24) != 0 || len(fields[29]) != 0 || len(fields[30]) != 0 {
		return NativeDelta{}, &NativeError{Code: "unimplemented"}
	}
	delta := NativeDelta{
		ID: fields.text(1), Text: fields.text(3), Thinking: fields.text(9), ThinkingID: fields.text(16),
		Signature: fields.text(10), SignatureType: fields.text(21), Stop: fields.number(5),
		ActualModelUID: fields.text(23), Phase: fields.text(25),
		Raw: bytes.Clone(payload),
	}
	for _, item := range fields[6] {
		call, err := projectedMessage(item.b, map[int]int{1: textField, 2: textField, 3: textField, 4: textField, 5: textField, 6: 0})
		if err != nil || call.text(4) != "" || call.text(5) != "" || call.number(6) != 0 {
			return NativeDelta{}, &NativeError{Code: "unimplemented"}
		}
		delta.Tools = append(delta.Tools, NativeToolDelta{ID: call.text(1), Name: call.text(2), Arguments: call.text(3)})
	}
	metrics, err := nativeMetrics(fields)
	if err != nil {
		return NativeDelta{}, err
	}
	delta.Usage, delta.Facts = metrics.Usage, metrics.Facts
	return delta, nil
}

func ReadNativeStream(ctx context.Context, reader io.Reader, gzipAllowed bool, emit func(NativeDelta) error) (NativeSummary, error) {
	var summary NativeSummary
	said := false
	seen := false
	type accumulatedTool struct {
		id, name string
		args     strings.Builder
	}
	var tools []*accumulatedTool
	byID := map[string]*accumulatedTool{}
	toolBytes := 0
	for {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		flags, payload, err := ReadConnectFrame(reader, gzipAllowed)
		if err != nil {
			if err := ctx.Err(); err != nil {
				return summary, err
			}
			return summary, &NativeError{Code: "data_loss"}
		}
		if flags&2 != 0 {
			var terminal struct {
				Error *struct {
					Code    string          `json:"code"`
					Type    string          `json:"type"`
					Message string          `json:"message"`
					Details json.RawMessage `json:"details"`
				} `json:"error"`
				Metadata map[string][]string `json:"metadata"`
			}
			if strictJSON(payload, &terminal) != nil {
				return summary, &NativeError{Code: "data_loss"}
			}
			if terminal.Error != nil {
				failure := ParseResponseFailure(payload, true)
				var extra [1]byte
				count, tailErr := io.ReadFull(reader, extra[:])
				if ctx.Err() != nil {
					return summary, ctx.Err()
				}
				if count != 0 || tailErr != io.EOF {
					if failure != nil {
						failure.Complete = false
					}
					return summary, &NativeError{Code: "data_loss", Failure: failure}
				}
				code := terminal.Error.Code
				switch code {
				case "unauthenticated", "permission_denied", "resource_exhausted", "invalid_argument",
					"unavailable", "deadline_exceeded", "canceled", "unimplemented", "data_loss", "internal":
				default:
					code = "internal"
				}
				return summary, &NativeError{Code: code, Failure: failure, BeforeEffect: !seen && failure.RejectsBeforeEffect()}
			}
			var extra [1]byte
			count, err := io.ReadFull(reader, extra[:])
			if ctx.Err() != nil {
				return summary, ctx.Err()
			}
			if count != 0 || !errors.Is(err, io.EOF) {
				return summary, &NativeError{Code: "data_loss"}
			}
			if err := nativeCompletion(summary.Stop, said, len(tools) != 0); err != nil {
				return summary, err
			}
			for _, tool := range tools {
				if tool.id == "" || tool.name == "" || !jsonObject([]byte(tool.args.String())) {
					return summary, &NativeError{Code: "data_loss"}
				}
			}
			return summary, nil
		}
		seen = true
		delta, err := DecodeNativeDelta(payload)
		if err != nil {
			return summary, err
		}
		if summary.Stop != 0 && (delta.Text != "" || delta.Thinking != "" || len(delta.Tools) != 0) {
			return summary, &NativeError{Code: "data_loss"}
		}
		if delta.Stop != 0 {
			if summary.Stop != 0 && delta.Stop != summary.Stop {
				return summary, &NativeError{Code: "data_loss"}
			}
			summary.Stop = delta.Stop
		}
		summary.Merge(NativeSummary{Usage: delta.Usage, Facts: delta.Facts})
		for _, change := range delta.Tools {
			toolBytes += len(change.ID) + len(change.Name) + len(change.Arguments)
			if toolBytes > 1<<20 {
				return summary, &NativeError{Code: "data_loss"}
			}
			var tool *accumulatedTool
			if change.ID != "" {
				tool = byID[change.ID]
			}
			if tool == nil && change.Name != "" && change.ID != "" {
				if len(tools) >= 256 {
					return summary, &NativeError{Code: "data_loss"}
				}
				tool = &accumulatedTool{id: change.ID, name: change.Name}
				byID[change.ID] = tool
				tools = append(tools, tool)
			}
			if tool == nil && change.ID == "" && change.Name == "" && len(tools) > 0 {
				tool = tools[len(tools)-1]
			}
			if tool == nil || change.Name != "" && change.Name != tool.name {
				return summary, &NativeError{Code: "data_loss"}
			}
			tool.args.WriteString(change.Arguments)
		}
		said = said || delta.Text != "" || delta.Thinking != "" || len(delta.Tools) != 0
		if emit != nil {
			if err := emit(delta); err != nil {
				return summary, err
			}
		}
	}
}
