// Package ompsessions converts explicitly supplied OMP journals without reading
// or writing a session store. The host publishes its returned documents.
package ompsessions

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	OMPFormat      = "omp-v3"
	DocumentFormat = "omp-conversation-v1"
	MaxBytes       = 8 << 20
	MaxFiles       = 128
)

type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// Item follows the archived carry codec's portable conversation vocabulary.
// Calls and results remain separate and ordered; outputs are never truncated.
type Item struct {
	Kind    string    `json:"kind"`
	Text    string    `json:"text,omitempty"`
	Content []Content `json:"content,omitempty"`
	Name    string    `json:"name,omitempty"`
	Input   string    `json:"input,omitempty"`
	Shell   string    `json:"shell,omitempty"`
	Workdir string    `json:"workdir,omitempty"`
	ID      string    `json:"id,omitempty"`
	IsError bool      `json:"isError,omitempty"`
	Time    time.Time `json:"time,omitempty"`

	assistantPhase string
	assistantStop  string
}

type Asset struct {
	SHA256 string `json:"sha256"`
	Body   []byte `json:"body"`
}

// Archive retains the entire original journal, including nonportable roles,
// reasoning, off-branch messages, compaction payloads and title changes.
type Archive struct {
	Journal          []byte  `json:"journal"`
	JournalSHA256    string  `json:"journalSHA256"`
	Assets           []Asset `json:"assets,omitempty"`
	ProjectionSHA256 string  `json:"projectionSHA256"`
}

type Document struct {
	Schema    int       `json:"schema"`
	ID        string    `json:"id"`
	Cwd       string    `json:"cwd"`
	Title     string    `json:"title"`
	Timestamp time.Time `json:"timestamp"`
	Items     []Item    `json:"items"`
	Archive   *Archive  `json:"ompArchive,omitempty"`
}

func strictJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("expected one JSON value")
	}
	return nil
}

func projectionHash(document Document) string {
	document.Archive = nil
	body, _ := json.Marshal(document)
	return hashBytes(body)
}

func hashBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func validHash(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	for _, c := range hash {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func decodeImage(data string) ([]byte, error) {
	encoding := base64.StdEncoding
	if strings.ContainsAny(data, "-_") {
		encoding = base64.URLEncoding
	}
	if !strings.Contains(data, "=") {
		encoding = encoding.WithPadding(base64.NoPadding)
	}
	body, err := encoding.Strict().DecodeString(data)
	if err != nil || len(body) == 0 {
		return nil, errors.New("image must contain valid nonempty base64 bytes")
	}
	return body, nil
}

func eachLine(ctx context.Context, body []byte, visit func([]byte)) error {
	for len(body) != 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, rest, found := bytes.Cut(body, []byte{'\n'})
		if !found {
			rest = nil
		}
		if len(bytes.TrimSpace(line)) != 0 {
			visit(line)
		}
		body = rest
	}
	return nil
}

func pretty(raw json.RawMessage) string {
	var result bytes.Buffer
	if len(raw) > 4000 || json.Indent(&result, raw, "", "  ") != nil {
		return string(raw)
	}
	return result.String()
}

// resultText is the donor's portable display projection. Full result blocks,
// including images and provider fields, remain in the archived journal.
func resultText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []struct{ Type, Text string }
	if json.Unmarshal(raw, &blocks) != nil {
		return string(raw)
	}
	var parts []string
	for _, block := range blocks {
		switch block.Type {
		case "text":
			parts = append(parts, block.Text)
		case "image":
			parts = append(parts, "[image]")
		}
	}
	return strings.Join(parts, "\n")
}

// UUID uses no third-party dependency. Session IDs use RFC 9562 UUIDv7;
// independent journal entry IDs use UUIDv4.
func newUUID(at time.Time, version byte) (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	if version == 7 {
		milliseconds := at.UnixMilli()
		if milliseconds < 0 || milliseconds >= 1<<48 {
			return "", errors.New("UUIDv7 timestamp is outside its 48-bit range")
		}
		for i := 5; i >= 0; i-- {
			value[i] = byte(milliseconds)
			milliseconds >>= 8
		}
	} else if version != 4 {
		return "", errors.New("unsupported UUID version")
	}
	value[6], value[8] = value[6]&0x0f|version<<4, value[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:]), nil
}

func validUUID(id string) bool {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return false
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	return err == nil && len(raw) == 16 && raw[8]&0xc0 == 0x80 && (raw[6]>>4 == 4 || raw[6]>>4 == 7)
}
