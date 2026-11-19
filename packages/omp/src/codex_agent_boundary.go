package ompsessions

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"
)

// A private replay marker lets rollback count instructions without exporting
// hidden native messages or changing how an existing assistant message renders.
const codexAgentBoundary = "codex-agent-boundary"

var codexAgentPath = regexp.MustCompile(`^(/morpheus|/root(/[a-z0-9_]+)*)$`)

func validCodexAgentPath(path string) bool {
	return codexAgentPath.MatchString(path) && !strings.Contains(strings.TrimPrefix(path, "/root")+"/", "/root/")
}

// Exact field names and unique keys matter when ordinary assistant text
// happens to contain JSON. A similar-looking object must remain ordinary.
func codexAgentObject(raw []byte) map[string]json.RawMessage {
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return nil
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		name, ok := token.(string)
		if err != nil || !ok || fields[name] != nil {
			return nil
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return nil
		}
		fields[name] = value
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') || d.Decode(new(any)) != io.EOF {
		return nil
	}
	return fields
}

func codexAgentString(raw json.RawMessage) (string, bool) {
	var text *string
	if json.Unmarshal(raw, &text) != nil || text == nil {
		return "", false
	}
	return *text, true
}

func codexAgentOptionalFields(fields map[string]json.RawMessage) bool {
	for _, key := range []string{"id", "encrypted_content"} {
		if raw := fields[key]; raw != nil && string(raw) != "null" {
			if _, ok := codexAgentString(raw); !ok {
				return false
			}
		}
	}
	if raw := fields["internal_chat_message_metadata_passthrough"]; raw != nil && string(raw) != "null" {
		meta := codexAgentObject(raw)
		if meta == nil {
			return false
		}
		if turn := meta["turn_id"]; turn != nil && string(turn) != "null" {
			if _, ok := codexAgentString(turn); !ok {
				return false
			}
		}
		if created := meta["create_time"]; created != nil && string(created) != "null" && !(created[0] == '-' || created[0] >= '0' && created[0] <= '9') {
			return false
		}
	}
	return true
}

func codexAgentCommunication(raw []byte) bool {
	fields := codexAgentObject(raw)
	author, _ := codexAgentString(fields["author"])
	recipient, _ := codexAgentString(fields["recipient"])
	_, content := codexAgentString(fields["content"])
	trigger := string(fields["trigger_turn"])
	if !validCodexAgentPath(author) || !validCodexAgentPath(recipient) || !content || (trigger != "true" && trigger != "false") || !codexAgentOptionalFields(fields) {
		return false
	}
	if raw := fields["other_recipients"]; raw != nil {
		var others []string
		if json.Unmarshal(raw, &others) != nil || others == nil {
			return false
		}
		for _, other := range others {
			if !validCodexAgentPath(other) {
				return false
			}
		}
	}
	return true
}

func codexNativeAgentMessage(raw []byte) bool {
	fields := codexAgentObject(raw)
	kind, _ := codexAgentString(fields["type"])
	_, author := codexAgentString(fields["author"])
	_, recipient := codexAgentString(fields["recipient"])
	var parts []json.RawMessage
	if kind != "agent_message" || !author || !recipient || !codexAgentOptionalFields(fields) || json.Unmarshal(fields["content"], &parts) != nil || parts == nil {
		return false
	}
	for _, part := range parts {
		block := codexAgentObject(part)
		kind, _ := codexAgentString(block["type"])
		key := "text"
		switch kind {
		case "input_text":
		case "encrypted_content":
			key = "encrypted_content"
		default:
			return false
		}
		if _, ok := codexAgentString(block[key]); !ok {
			return false
		}
	}
	return true
}
