package zcode

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
)

func newRequestID() (string, error) {
	var requestID [16]byte
	if _, err := rand.Read(requestID[:]); err != nil {
		return "", err
	}
	requestID[6] = requestID[6]&15 | 64
	requestID[8] = requestID[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", requestID[:4], requestID[4:6], requestID[6:8], requestID[8:10], requestID[10:]), nil
}

func queryID(body []byte, session string) string {
	request, err := protocol.ReadObject(body)
	if err != nil {
		return ""
	}
	messages, valid := request["messages"].([]any)
	if !valid {
		return ""
	}
	turns := 0
	for _, entry := range messages {
		message := mapObject(entry)
		if message["role"] != "user" {
			continue
		}
		switch content := message["content"].(type) {
		case string:
			if strings.TrimSpace(content) != "" {
				turns++
			}
		case []any:
			for _, block := range content {
				value := mapObject(block)
				if value != nil && value["type"] != "tool_result" {
					turns++
					break
				}
			}
		}
	}
	if turns == 0 {
		return ""
	}
	return seededUUID("zcode-query\x00" + session + "\x00" + strconv.Itoa(turns))
}

func seededUUID(seed string) string {
	digest := sha256.Sum256([]byte(seed))
	digest[6] = digest[6]&15 | 64
	digest[8] = digest[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", digest[:4], digest[4:6], digest[6:8], digest[8:10], digest[10:16])
}

func accountDevice(account Credentials) string {
	if account.Device != "" {
		return account.Device
	}
	return seededUUID("zcode-device\x00" + account.Key)
}

func sessionID(session, device string) string {
	own := strings.TrimSpace(session)
	if len(own) == 36 && own[8] == '-' && own[13] == '-' && own[18] == '-' && own[23] == '-' {
		if decoded, err := hex.DecodeString(strings.ReplaceAll(own, "-", "")); err == nil && len(decoded) == 16 {
			return strings.ToLower(own)
		}
	}
	if own != "" {
		return seededUUID("zcode-session\x00" + own)
	}
	return seededUUID("zcode-session-device\x00" + device)
}

func markIdentity(body []byte, device, session string) ([]byte, error) {
	request, err := protocol.ReadObject(body)
	if err != nil {
		return nil, invalid("Invalid Messages body")
	}
	metadata, valid := request["metadata"].(map[string]any)
	if request["metadata"] != nil && !valid {
		return nil, invalid("Messages metadata must be an object")
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	if existing, exists := metadata["user_id"]; exists && existing != nil {
		text, valid := existing.(string)
		if !valid {
			return nil, invalid("Messages metadata user_id must be a string")
		}
		if strings.TrimSpace(text) != "" {
			return body, nil
		}
	}
	identity, _ := json.Marshal(map[string]string{"device_id": device, "account_uuid": "", "session_id": session})
	metadata["user_id"] = string(identity)
	request["metadata"] = metadata
	marked, err := json.Marshal(request)
	if err != nil || len(marked) > protocol.MaxBody {
		return nil, invalid("Messages body with identity exceeds its byte limit")
	}
	return marked, nil
}
