package devin

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

const (
	cliCredentialsFile = "devin/credentials.toml"
	cliCredentialsMax  = 64 << 10
)

type cliImportIdentity struct {
	TeamID string `json:"teamId,omitempty"`
	OrgID  string `json:"orgId,omitempty"`
}

func cliImportFrom(metadata map[string]json.RawMessage) (*cliImportIdentity, error) {
	marker, present := metadata["cli"]
	if !present {
		return nil, nil
	}
	var text string
	if string(marker) != "true" && (json.Unmarshal(marker, &text) != nil || text != "true") {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Saved Devin CLI sign-in has an invalid link marker"}
	}
	var identity cliImportIdentity
	for _, field := range []struct {
		name  string
		value *string
	}{{"teamId", &identity.TeamID}, {"orgId", &identity.OrgID}} {
		if raw, exists := metadata[field.name]; exists &&
			(json.Unmarshal(raw, field.value) != nil || *field.value == "") {
			return nil, &sdk.Error{Code: "invalid_params", Message: "Saved Devin CLI sign-in has invalid tenant metadata"}
		}
	}
	return &identity, nil
}

func cliIdentityNotVerified() *sdk.Error {
	return &sdk.Error{Code: "permission_denied", Message: "Could not verify the same Devin CLI user, team and organization", Outcome: "not-started"}
}

func cliSubject(server string, status outbound.Status) string {
	if status.TeamID == "" || status.OrgID == "" {
		return ""
	}
	return accountSubject(server, status)
}

func (identity cliImportIdentity) verify(ctx context.Context, client *outbound.Client, server string, saved outbound.Credentials) (outbound.Credentials, outbound.Status, error) {
	current, err := readCLICredentials(server)
	if err != nil {
		return outbound.Credentials{}, outbound.Status{}, err
	}
	previous, err := client.Status(ctx, saved, nil)
	subject := cliSubject(server, previous)
	if err != nil || subject == "" ||
		identity.TeamID != "" && identity.TeamID != previous.TeamID || identity.OrgID != "" && identity.OrgID != previous.OrgID {
		return outbound.Credentials{}, outbound.Status{}, cliIdentityNotVerified()
	}
	if current.Key == saved.Key {
		return current, previous, nil
	}
	status, err := client.Status(ctx, current, nil)
	if err != nil || cliSubject(server, status) != subject {
		return outbound.Credentials{}, outbound.Status{}, cliIdentityNotVerified()
	}
	return current, status, nil
}

func readCLICredentials(server string) (outbound.Credentials, error) {
	unavailable := &sdk.Error{Code: "not_found", Message: "Devin CLI credentials.toml is unavailable or unsupported", Outcome: "not-started"}
	base := os.Getenv("XDG_DATA_HOME")
	if appdata := os.Getenv("APPDATA"); runtime.GOOS == "windows" && appdata != "" {
		base = appdata
	}
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return outbound.Credentials{}, unavailable
		}
		base = filepath.Join(home, ".local", "share")
	}
	if !filepath.IsAbs(base) {
		return outbound.Credentials{}, unavailable
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return outbound.Credentials{}, unavailable
	}
	defer root.Close()
	before, err := root.Lstat(cliCredentialsFile)
	if err != nil || !before.Mode().IsRegular() {
		return outbound.Credentials{}, unavailable
	}
	file, err := root.OpenFile(cliCredentialsFile, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return outbound.Credentials{}, unavailable
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) {
		return outbound.Credentials{}, unavailable
	}
	body, err := io.ReadAll(io.LimitReader(file, cliCredentialsMax+1))
	if err != nil || len(body) > cliCredentialsMax {
		return outbound.Credentials{}, unavailable
	}
	values, ok := parseCLICredentials(string(body))
	credentials := outbound.Credentials{Key: values["windsurf_api_key"]}
	if !ok || credentials.Validate() != nil {
		return outbound.Credentials{}, unavailable
	}
	origin, exists := values["api_server_url"]
	if !exists {
		origin = outbound.DefaultServer
	}
	override := os.Getenv("WINDSURF_API_SERVER_URL")
	if strings.TrimRight(origin, "/") != server || override != "" && strings.TrimRight(override, "/") != server {
		return outbound.Credentials{}, &sdk.Error{Code: "permission_denied", Message: "Devin CLI sign-in belongs to a different server", Outcome: "not-started"}
	}
	return credentials, nil
}

func parseCLICredentials(body string) (map[string]string, bool) {
	if !utf8.ValidString(body) || strings.ContainsRune(body, 0) {
		return nil, false
	}
	values := make(map[string]string)
	for raw := range strings.Lines(body) {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		field, value, found := strings.Cut(line, "=")
		field, value = strings.TrimSpace(field), strings.TrimSpace(value)
		if !found || field == "" || strings.Trim(field, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "" {
			return nil, false
		}
		if _, duplicate := values[field]; duplicate {
			return nil, false
		}
		switch {
		case strings.HasPrefix(value, `"`):
			var decoded string
			if json.Unmarshal([]byte(value), &decoded) != nil {
				return nil, false
			}
			values[field] = decoded
		case strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") && strings.Count(value, "'") == 2:
			values[field] = value[1 : len(value)-1]
		case field == "dangerously_skip_plugin_authentication" && (value == "true" || value == "false"):
			values[field] = value
		default:
			return nil, false
		}
	}
	return values, true
}
