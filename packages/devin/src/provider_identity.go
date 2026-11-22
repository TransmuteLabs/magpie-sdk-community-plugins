package devin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

const subjectPrefix = "dvn-"

type accountBinding struct {
	Server    string `json:"server"`
	Subject   string `json:"subject"`
	Slot      string `json:"slot,omitempty"`
	FollowCLI bool   `json:"followCli,omitempty"`
}

type accountCredentials struct {
	outbound.Credentials
	Binding *accountBinding `json:"binding,omitempty"`
}

func accountHash(value string) bool {
	return len(value) == 32 && strings.Trim(value, "0123456789abcdef") == ""
}

func (credentials accountCredentials) matches(server, account string) bool {
	binding := credentials.Binding
	if binding == nil {
		return credentials.AccountID(server) == account
	}
	return binding.Server == server && binding.matchesSlot(account)
}

func (credentials accountCredentials) matchesStoredSlot(account string) bool {
	binding := credentials.Binding
	if binding == nil {
		return accountHash(account)
	}
	return binding.Server != "" && binding.matchesSlot(account)
}

func (binding accountBinding) matchesSlot(account string) bool {
	digest, prefixed := strings.CutPrefix(binding.Subject, subjectPrefix)
	if !prefixed || !accountHash(digest) {
		return false
	}
	if binding.Slot != "" {
		return accountHash(binding.Slot) && binding.Slot == account
	}
	return binding.Subject == account
}

func accountSubject(server string, status outbound.Status) string {
	if status.UserID == "" {
		return ""
	}
	for _, value := range []string{status.UserID, status.TeamID, status.OrgID} {
		if len(value) > 256 || strings.IndexFunc(value, func(char rune) bool {
			return unicode.IsSpace(char) || unicode.IsControl(char) || char == unicode.ReplacementChar
		}) >= 0 {
			return ""
		}
	}
	identity, _ := json.Marshal([4]string{server, status.UserID, status.TeamID, status.OrgID})
	sum := sha256.Sum256(identity)
	return subjectPrefix + hex.EncodeToString(sum[:16])
}

func sameAccount(ctx context.Context, client *outbound.Client, server string, previous accountCredentials,
	next outbound.Credentials, subject string, now time.Time) bool {
	if previous.Binding != nil {
		return previous.Binding.Subject == subject
	}
	if previous.Key == next.Key {
		return true
	}
	if subject == "" || previous.Due(now) {
		return false
	}
	status, err := client.Status(ctx, previous.Credentials, nil)
	return err == nil && subject == accountSubject(server, status)
}

func accountResult(id, server, subject string, credentials outbound.Credentials, status outbound.Status, followsCLI bool) sdk.AuthResult {
	stored := accountCredentials{Credentials: credentials}
	if subject != "" {
		stored.Binding = &accountBinding{Server: server, Subject: subject, FollowCLI: followsCLI}
		if id != subject {
			stored.Binding.Slot = id
		}
	}
	auth, _ := json.Marshal(stored)
	result := sdk.AuthResult{ID: id, Name: accountName(id, status), Credentials: auth}
	if !credentials.ExpiresAt.IsZero() {
		result.ExpiresAt = credentials.ExpiresAt.UnixMilli()
	}
	return result
}

func accountName(id string, status outbound.Status) string {
	clean := func(value string) string {
		return strings.Join(strings.FieldsFunc(value, func(char rune) bool {
			return unicode.IsSpace(char) || unicode.IsControl(char)
		}), " ")
	}
	name := clean(status.Email)
	if name == "" {
		name = clean(status.Name)
	}
	if name == "" {
		name = "Devin [" + id[:min(len(id), 12)] + "]"
	}
	if organization := clean(status.OrgName); organization != "" && !strings.EqualFold(name, organization) {
		name += " · " + organization
	}
	if len(name) > 256 {
		end := 256
		for !utf8.RuneStart(name[end]) {
			end--
		}
		name = strings.TrimSpace(name[:end])
	}
	return name
}
