package devin

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
)

type CacheKeepaliveTemplate struct {
	payload []byte
}

type orderedChunk struct {
	number int
	value  []byte
	raw    []byte
}

func orderedChunks(payload []byte) ([]orderedChunk, error) {
	if _, err := readMessage(payload, nil); err != nil {
		return nil, err
	}
	var chunks []orderedChunk
	for len(payload) > 0 {
		start := payload
		tag, count := binary.Uvarint(payload)
		payload = payload[count:]
		var value []byte
		switch tag & 7 {
		case 0:
			_, count = binary.Uvarint(payload)
			payload = payload[count:]
		case 1:
			value, payload = payload[:8], payload[8:]
		case 5:
			value, payload = payload[:4], payload[4:]
		case 2:
			size, used := binary.Uvarint(payload)
			payload = payload[used:]
			value, payload = payload[:int(size)], payload[int(size):]
		}
		chunks = append(chunks, orderedChunk{number: int(tag >> 3), value: value, raw: start[:len(start)-len(payload)]})
	}
	return chunks, nil
}

func NewCacheKeepaliveTemplate(payload []byte) (CacheKeepaliveTemplate, error) {
	var empty CacheKeepaliveTemplate
	if len(payload) == 0 || len(payload) > maxBody {
		return empty, errors.New("cache keepalive request exceeds its byte limit")
	}
	fields, err := readMessage(payload, map[int]int{
		1: 2, 2: textField, 3: 2 | repeated, 5: 0, 6: 0, 7: 0, 8: 2,
		10: 2 | repeated, 11: 0, 12: 2, 13: 2, 14: textField, 15: 2,
		16: textField, 17: textField, 18: 0, 19: 0, 20: 0, 21: textField,
		22: textField, 27: textField, 28: textField,
	})
	if err != nil || len(fields[3]) == 0 || len(fields[3]) > 2048 || len(fields[10]) > 256 ||
		fields.text(21) == "" || len(fields[8]) != 1 || fields.number(5) != 0 || fields.number(6) != 0 ||
		fields.number(7) != 0 && fields.number(7) != 5 || fields.number(11) > 1 {
		return empty, errors.New("unsupported cache keepalive request")
	}
	if err := validateNativeMetadata(fields.data(1)); err != nil {
		return empty, errors.New("unsupported cache keepalive authority")
	}
	for _, prompt := range fields[3] {
		parts, err := readMessage(prompt.b, map[int]int{
			1: textField, 2: 0, 3: textField, 4: 0, 5: 0, 6: 2 | repeated, 7: textField,
			8: 2, 9: 0, 10: 2 | repeated, 11: textField, 12: textField, 13: 0,
			15: textField, 16: textField, 17: 2, 18: textField, 19: textField, 22: textField,
		})
		if err != nil || parts.number(2) != 1 && parts.number(2) != 2 &&
			parts.number(2) != 4 && parts.number(2) != 5 {
			return empty, errors.New("unsupported cache keepalive prompt")
		}
		if len(parts[8]) > 0 {
			if _, err := cacheOptions(parts.data(8)); err != nil {
				return empty, errors.New("unsupported cache keepalive cache control")
			}
		}
	}
	configuration, err := readMessage(fields.data(8), map[int]int{
		1: 0, 2: 0, 3: 0, 4: 1, 5: 1, 6: 1, 7: 0, 8: 1,
		9: textField | repeated, 10: 0, 11: 1, 12: 0, 13: 0, 14: 0,
		15: 0, 16: 0, 17: textField, 18: textField, 19: textField, 20: textField,
	})
	if err != nil || configuration.number(1) > 1 || configuration.number(2) == 0 ||
		configuration.number(2) > math.MaxInt32 {
		return empty, errors.New("unsupported cache keepalive configuration")
	}
	sampling, _ := orderedChunks(fields.data(8))
	var limited []byte
	for _, chunk := range sampling {
		if chunk.number == 2 {
			limited = addNumber(limited, 2, 1)
		} else {
			limited = append(limited, chunk.raw...)
		}
	}
	extra, err := encodeOutboundMessage(outboundMessage{Role: "user", Content: []byte(`"continue"`)})
	if err != nil {
		return empty, err
	}
	chunks, _ := orderedChunks(payload)
	last := -1
	for index, chunk := range chunks {
		if chunk.number == 3 {
			last = index
		}
	}
	var result []byte
	for index, chunk := range chunks {
		switch chunk.number {
		case 1:
		case 8:
			result = addBytes(result, 8, limited)
		default:
			result = append(result, chunk.raw...)
		}
		if index == last {
			result = addBytes(result, 3, extra)
		}
	}
	if len(result) > maxBody {
		return empty, errors.New("cache keepalive template exceeds its byte limit")
	}
	metadata, _ := readMessage(fields.data(1), nil)
	if credential := metadata.text(3); credential != "" && bytes.Contains(result, []byte(credential)) {
		return empty, errors.New("cache keepalive content contains account credentials")
	}
	return CacheKeepaliveTemplate{payload: result}, nil
}

func (template CacheKeepaliveTemplate) Size() int {
	return len(template.payload)
}

func (template CacheKeepaliveTemplate) Payload(key string) ([]byte, error) {
	if len(template.payload) == 0 {
		return nil, errors.New("cache keepalive template is missing")
	}
	metadata, err := NativeMetadata(key)
	if err != nil {
		return nil, err
	}
	if bytes.Contains(template.payload, []byte(key)) {
		return nil, errors.New("cache keepalive content contains selected account credentials")
	}
	payload := addBytes(bytes.Clone(template.payload), 1, metadata)
	if len(payload) > maxBody {
		return nil, errors.New("cache keepalive request exceeds its byte limit")
	}
	return payload, nil
}
