package outbound

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

func decodeReply(body []byte, target any) error {
	if len(body) > maxReply || !utf8.Valid(body) {
		return errors.New("invalid native JSON reply")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := uniqueValue(decoder, 0); err != nil {
		return errors.New("invalid or ambiguous native JSON reply")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing native JSON reply data")
	}
	return json.Unmarshal(body, target)
}

func DecodeParameters(body []byte, target any) error {
	if len(bytes.TrimSpace(body)) == 0 || bytes.TrimSpace(body)[0] != '{' {
		return errors.New("expected parameter object")
	}
	if err := decodeReply(body, new(any)); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func uniqueValue(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("native JSON nesting exceeds its bound")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := token.(string)
			if !ok || seen[key] {
				return errors.New("duplicate native JSON key")
			}
			seen[key] = true
			if err := uniqueValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("invalid native JSON object")
		}
	case '[':
		for decoder.More() {
			if err := uniqueValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("invalid native JSON array")
		}
	default:
		return errors.New("invalid native JSON delimiter")
	}
	return nil
}
