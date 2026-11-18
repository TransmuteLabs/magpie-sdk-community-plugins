package opencode

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	ProviderGo    = "opencode-go"
	ProviderZen   = "opencode-zen"
	ClientID      = "opencode-cli"
	ClientVersion = "2.3.0"
)

var (
	ErrRefreshRequired       = errors.New("OpenCode credentials require refresh")
	ErrAmbiguousOrganization = errors.New("OpenCode returned multiple organizations without selecting one; locale-collation fallback is outside this slice")
)

type Credentials struct {
	Access       string    `json:"access"`
	Refresh      string    `json:"refresh"`
	Expires      time.Time `json:"expires"`
	UserID       string    `json:"userId"`
	Email        string    `json:"email"`
	InitialOrgID string    `json:"initialOrgId,omitempty"`
	OrgID        string    `json:"orgId,omitempty"`
	OrgName      string    `json:"orgName,omitempty"`
}

func (value Credentials) Validate() error {
	if value.Access == "" || value.Refresh == "" || value.UserID == "" || value.Email == "" || value.Expires.IsZero() {
		return errors.New("OpenCode credentials are incomplete")
	}
	for _, token := range []string{value.Access, value.Refresh} {
		if strings.ContainsAny(token, " \t\r\n") {
			return errors.New("OpenCode credentials contain invalid token whitespace")
		}
	}
	return nil
}

func (value Credentials) Due(now time.Time) bool {
	return value.Expires.Sub(now) <= 30*time.Second
}

func (value Credentials) AccountID(product string) string {
	sum := sha256.Sum256([]byte("OpenCode Console\x00" + product + "\x00" + value.UserID + "\x00" + value.InitialOrgID))
	return hex.EncodeToString(sum[:16])
}

type TokenReply struct {
	Access    string  `json:"access_token"`
	Refresh   string  `json:"refresh_token"`
	ExpiresIn int64   `json:"expires_in"`
	OrgID     *string `json:"org_id"`
	Error     string  `json:"error"`
	expiresAt time.Time
}

func (value TokenReply) valid() bool {
	return value.Access != "" && value.Refresh != "" && value.ExpiresIn > 0 &&
		value.ExpiresIn <= int64((1<<63-1)/time.Second)
}

type DeviceCode struct {
	Device    string `json:"device_code"`
	User      string `json:"user_code"`
	URL       string `json:"verification_uri_complete"`
	ExpiresIn int64  `json:"expires_in"`
	Interval  int    `json:"interval"`
}

type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PollResult struct {
	Token    *TokenReply
	Pending  bool
	SlowDown bool
}

type RemoteError struct {
	Operation string
	Status    int
	Code      string
}

func (value *RemoteError) Error() string {
	// Vendor descriptions/bodies may contain tokens; only a fixed operation
	// and an allow-listed error code cross the public SDK boundary.
	if value.Code != "" {
		return "OpenCode " + value.Operation + " failed: " + value.Code
	}
	return "OpenCode " + value.Operation + " failed"
}

func publicCode(value string) string {
	switch value {
	case "authorization_pending", "slow_down", "access_denied", "expired_token", "invalid_grant":
		return value
	default:
		return ""
	}
}
