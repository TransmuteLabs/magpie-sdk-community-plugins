package zcode

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"
)

const ClientVersion = "3.18.2"

type Credentials struct {
	Site               string           `json:"site"`
	LoginProvider      string           `json:"loginProvider,omitempty"`
	Kind               string           `json:"kind"`
	DisplayName        string           `json:"displayName,omitempty"`
	UserID             string           `json:"userId,omitempty"`
	Organization       string           `json:"organization,omitempty"`
	Project            string           `json:"project,omitempty"`
	Product            string           `json:"product,omitempty"`
	Device             string           `json:"device,omitempty"`
	Key                string           `json:"key,omitempty"`
	ProjectToken       string           `json:"projectToken,omitempty"`
	SessionToken       string           `json:"sessionToken,omitempty"`
	SessionExpiresAt   int64            `json:"sessionExpiresAt,omitempty"`
	ExpiresAt          int64            `json:"expiresAt,omitempty"`
	BusinessToken      string           `json:"businessToken,omitempty"`
	BusinessExpiresAt  int64            `json:"businessExpiresAt,omitempty"`
	OAuthToken         string           `json:"oauthToken,omitempty"`
	OAuthExpiresAt     int64            `json:"oauthExpiresAt,omitempty"`
	Location           *ProjectLocation `json:"location,omitempty"`
	Discovery          bool             `json:"discovery,omitempty"`
	VendorRefreshToken string           `json:"vendorRefreshToken,omitempty"`
}

type ProjectLocation struct {
	OrganizationID string `json:"organizationId"`
	ProjectID      string `json:"projectId"`
	APIKeyID       string `json:"apiKeyId"`
}

var ErrCredentials = errors.New("invalid selected ZCode credential snapshot")
var ErrExpired = errors.New("ZCode project/session credential is due for renewal")

func safeText(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= 8192 &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

func (value Credentials) validateStructure() error {
	if value.Site != "zai" && value.Site != "bigmodel" {
		return ErrCredentials
	}
	if !loginProviderMatches(value.Site, value.LoginProvider) {
		return ErrCredentials
	}
	for _, text := range []string{value.UserID, value.Organization, value.Project, value.Product, value.Device, value.Key, value.ProjectToken, value.SessionToken, value.BusinessToken, value.OAuthToken, value.VendorRefreshToken} {
		if text != "" && !safeText(text) {
			return ErrCredentials
		}
	}
	switch value.Kind {
	case "legacy-key":
		if !safeText(value.Key) || value.ProjectToken != "" || value.SessionToken != "" || value.Product != "" ||
			(value.Organization == "") != (value.Project == "") || value.BusinessToken != "" || value.BusinessExpiresAt != 0 || value.SessionExpiresAt != 0 || value.Location != nil ||
			value.OAuthToken != "" || value.OAuthExpiresAt != 0 {
			return ErrCredentials
		}
	case "individual", "team":
		if !safeText(value.ProjectToken) || !safeText(value.UserID) || !safeText(value.Device) || value.Key != "" {
			return ErrCredentials
		}
		if value.Kind == "team" && (!safeText(value.Organization) || !safeText(value.Project) || !safeText(value.Product)) {
			return ErrCredentials
		}
		if value.Kind == "individual" && (value.Organization != "" || value.Project != "" || value.Product != "") {
			return ErrCredentials
		}
	case "start":
		if !safeText(value.SessionToken) || !safeText(value.UserID) || !safeText(value.Device) || value.Key != "" || value.ProjectToken != "" ||
			value.Organization != "" || value.Project != "" || value.Product != "" || value.BusinessToken != "" || value.BusinessExpiresAt != 0 || value.SessionExpiresAt != 0 || value.Location != nil ||
			value.OAuthToken != "" || value.OAuthExpiresAt != 0 {
			return ErrCredentials
		}
	default:
		return ErrCredentials
	}
	if value.ExpiresAt < 0 || value.BusinessExpiresAt < 0 || value.SessionExpiresAt < 0 || value.OAuthExpiresAt < 0 ||
		value.BusinessToken == "" && value.BusinessExpiresAt != 0 ||
		value.Kind != "start" && value.SessionToken == "" && value.SessionExpiresAt != 0 ||
		value.OAuthToken == "" && value.OAuthExpiresAt != 0 ||
		value.Site != "zai" && (value.OAuthToken != "" || value.OAuthExpiresAt != 0) {
		return ErrCredentials
	}
	if value.VendorRefreshToken != "" && value.Site != "bigmodel" {
		return ErrCredentials
	}
	if location := value.Location; location != nil {
		if !safeText(location.OrganizationID) || !safeText(location.ProjectID) || !safeText(location.APIKeyID) {
			return ErrCredentials
		}
		if value.Kind == "team" && (location.OrganizationID != value.Organization || location.ProjectID != value.Project) {
			return ErrCredentials
		}
	}
	return nil
}

func (value Credentials) Validate(now time.Time) error {
	if err := value.validateStructure(); err != nil {
		return err
	}
	if value.Kind != "legacy-key" && value.ExpiresAt <= now.Add(30*time.Second).UnixMilli() ||
		value.ExpiresAt > 0 && value.ExpiresAt <= now.Add(30*time.Second).UnixMilli() {
		return ErrExpired
	}
	return nil
}

// The donor uses the selected OAuth user's email or name. Display metadata never
// participates in AccountID, including when a phone.local alias changes.
func oauthDisplayName(user map[string]any) string {
	email, _ := user["email"].(string)
	name, _ := user["name"].(string)
	for _, candidate := range []string{strings.TrimSuffix(strings.TrimSpace(email), "@phone.local"), strings.TrimSpace(name)} {
		if safeText(candidate) && len(candidate) <= 220 {
			return candidate
		}
	}
	return ""
}

func (value Credentials) accountName() string {
	if value.Kind == "legacy-key" {
		if value.Site == "bigmodel" {
			return "BigModel · GLM Coding Plan API key"
		}
		return "Z.ai · GLM Coding Plan API key"
	}
	plan := "ZCode " + value.Site + " " + value.Kind
	if safeText(value.DisplayName) && len(value.DisplayName) <= 220 {
		return value.DisplayName + " · " + plan
	}
	if safeText(value.UserID) && len(value.UserID) <= 220 {
		return value.UserID + " · " + plan
	}
	return plan
}

func (value Credentials) AccountID() string {
	identity := value.UserID
	if value.Kind == "legacy-key" {
		identity = value.Key
	}
	parts := []string{value.Site, value.Kind, identity, value.Organization, value.Project, value.Product}
	if location := value.Location; location != nil {
		parts = append(parts, location.OrganizationID, location.ProjectID, location.APIKeyID)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "zcode-" + hex.EncodeToString(sum[:16])
}

func (value Credentials) token() string {
	switch value.Kind {
	case "legacy-key":
		return value.Key
	case "start":
		return value.SessionToken
	default:
		return value.ProjectToken
	}
}

func NativeLane(model string) (string, bool) {
	switch strings.ToLower(model) {
	case "glm-5.3-offpeak", "glm-5.3-flash-offpeak":
		return "offpeak", true
	case "glm-5.3-highspeed", "glm-5.3-flash-highspeed":
		return "highspeed", true
	default:
		return "", false
	}
}
