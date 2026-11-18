package zcode

import (
	"encoding/json"
	"testing"
)

func TestAccountDeviceAndSessionAreStableAndSeparate(t *testing.T) {
	first := fixtureCredentials("legacy-key")
	first.Device = ""
	second := first
	second.Key = "another-key"
	if accountDevice(first) != accountDevice(first) || accountDevice(first) == accountDevice(second) {
		t.Fatal("legacy key device identity was unstable or shared")
	}
	device := accountDevice(first)
	if sessionID("conversation-a", device) != sessionID("conversation-a", device) ||
		sessionID("conversation-a", device) == sessionID("conversation-b", device) ||
		sessionID("", device) == sessionID("", accountDevice(second)) {
		t.Fatal("sessions are unstable or collide across conversation/device")
	}
	if sessionID("AAAABBBB-CCCC-4DDD-8EEE-1234567890AB", device) != "aaaabbbb-cccc-4ddd-8eee-1234567890ab" {
		t.Fatal("native UUID session was replaced")
	}
}

func TestMetadataKeepsExistingUserIdentityAndUnknownFields(t *testing.T) {
	body := []byte(`{"model":"GLM-5.3","messages":[],"metadata":{"user_id":"own-signed-identity","extra":9007199254740993}}`)
	marked, err := markIdentity(body, "device", "session")
	if err != nil || string(marked) != string(body) {
		t.Fatal("existing agent user_id changed")
	}
	body = []byte(`{"model":"GLM-5.3","messages":[],"metadata":{"extra":9007199254740993}}`)
	marked, err = markIdentity(body, "device", "session")
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Metadata struct {
			UserID string      `json:"user_id"`
			Extra  json.Number `json:"extra"`
		} `json:"metadata"`
	}
	var identity map[string]string
	if json.Unmarshal(marked, &request) != nil || json.Unmarshal([]byte(request.Metadata.UserID), &identity) != nil ||
		identity["device_id"] != "device" || identity["session_id"] != "session" ||
		request.Metadata.Extra != "9007199254740993" {
		t.Fatal("identity stamping lost native metadata")
	}
}

func TestMalformedMetadataIsRefusedInsteadOfDiscarded(t *testing.T) {
	for _, body := range []string{`{"metadata":[]}`, `{"metadata":false}`, `{"metadata":{"user_id":123}}`} {
		if _, err := markIdentity([]byte(body), "device", "session"); err == nil {
			t.Fatal("malformed metadata silently replaced")
		}
	}
}
