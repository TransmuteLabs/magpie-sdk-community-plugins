package devin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func finishFixtureSignIn(t *testing.T, peer *sdkPeer, method string, input any) sdk.Frame {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	start, _ := peer.call(t, sdk.MethodAuthStart, sdk.AuthStartRequest{Method: method, Input: raw})
	if start.Error != nil {
		return start
	}
	var pending sdk.AuthStartResult
	if err := json.Unmarshal(start.Result, &pending); err != nil || len(pending.Continuation) == 0 {
		t.Fatal("sign-in did not return an opaque continuation")
	}
	finish, _ := peer.call(t, sdk.MethodAuthFinish, sdk.AuthFinishRequest{Continuation: pending.Continuation})
	return finish
}

func readFixtureAccount(t *testing.T, result sdk.Frame) sdk.AuthResult {
	t.Helper()
	var account sdk.AuthResult
	if result.Error != nil || json.Unmarshal(result.Result, &account) != nil ||
		account.ID == "" || account.Pending || len(account.Credentials) == 0 {
		t.Fatalf("sign-in did not produce an account: %v", result.Error)
	}
	return account
}

func useFixtureAccount(peer *sdkPeer, account sdk.AuthResult) {
	peer.scope = sdk.Scope{Provider: ProviderID, Account: account.ID, Generation: 3}
	peer.auth = account.Credentials
}

func identityFixture(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Metadata struct {
				APIKey string `json:"apiKey"`
			} `json:"metadata"`
		}
		if request.URL.Path != outbound.StatusPath || json.NewDecoder(request.Body).Decode(&body) != nil {
			t.Error("verification used an unexpected vendor request")
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		userID, orgID, teamID := "user-123", "organization-1", "team-1"
		email, orgName := "same@example.invalid", "Fixture organization"
		switch body.Metadata.APIKey {
		case "identity-first":
		case "identity-replacement":
			email = "new@example.invalid"
		case "identity-other-user":
			userID = "user-456"
		case "identity-other-organization":
			orgID = "organization-2"
		case "identity-other-team":
			teamID = "team-2"
		case "identity-unknown":
			userID = ""
		case "identity-empty-status":
			_, _ = writer.Write([]byte(`{"userStatus":{}}`))
			return
		case "identity-email-only":
			_, _ = writer.Write([]byte(`{"userStatus":{"email":"same@example.invalid"}}`))
			return
		case "identity-no-organization":
			orgID = ""
		case "identity-revoked":
			writer.WriteHeader(http.StatusUnauthorized)
			_, _ = writer.Write([]byte(`{"code":"unauthenticated"}`))
			return
		default:
			t.Error("verification used an unexpected credential")
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"userStatus": map[string]any{
			"userId": userID, "teamId": teamID, "email": email,
			"planStatus": map[string]any{"planInfo": map[string]any{"devinInfo": map[string]string{
				"orgId": orgID, "accountDisplayName": orgName,
			}}},
		}})
	}))
	t.Cleanup(server.Close)
	return server
}

func TestProviderSignInRetainsVerifiedAccount(t *testing.T) {
	server := identityFixture(t)
	peer, _ := providerPeer(t, server.URL)
	first := readFixtureAccount(t, finishFixtureSignIn(t, peer, "api-key", outbound.Credentials{Key: "identity-first"}))
	replacement := readFixtureAccount(t, finishFixtureSignIn(t, peer, "api-key", outbound.Credentials{Key: "identity-replacement"}))
	if replacement.ID != first.ID || replacement.Name != "new@example.invalid · Fixture organization" {
		t.Fatal("key rotation or renamed email created a different identity")
	}
	useFixtureAccount(peer, first)
	for _, key := range []string{"identity-other-user", "identity-other-organization", "identity-other-team", "identity-unknown"} {
		t.Run(key, func(t *testing.T) {
			result := finishFixtureSignIn(t, peer, "api-key", outbound.Credentials{Key: key})
			if result.Error == nil || result.Error.Code != "permission_denied" {
				t.Fatal("reauthorization accepted a different or unverified vendor identity")
			}
		})
	}
	rotated := readFixtureAccount(t, finishFixtureSignIn(t, peer, "api-key", outbound.Credentials{Key: "identity-replacement"}))
	if rotated.ID != first.ID || rotated.Name != replacement.Name {
		t.Fatal("reauthorization changed the account slot or lost its display name")
	}
	useFixtureAccount(peer, rotated)
	refreshed, _ := peer.call(t, sdk.MethodAuthRefresh, sdk.AuthRefreshRequest{})
	snapshot := readFixtureAccount(t, refreshed)
	if snapshot.ID != rotated.ID || snapshot.Name != replacement.Name || snapshot.ExpiresAt != 0 {
		t.Fatal("account refresh lost verified metadata or invented expiry")
	}
	logout, _ := peer.call(t, sdk.MethodAuthLogout, sdk.AuthLogoutRequest{})
	if logout.Error != nil {
		t.Fatal("logout rejected the rotated account", logout.Error)
	}
	peer.scope.Account = "other-slot"
	logout, _ = peer.call(t, sdk.MethodAuthLogout, sdk.AuthLogoutRequest{})
	if logout.Error == nil || logout.Error.Code != "permission_denied" {
		t.Fatal("logout accepted credentials for another slot")
	}
}

func TestProviderLogoutAfterServerChange(test *testing.T) {
	original := identityFixture(test)
	replacement := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		test.Errorf("saved credentials reached the replacement server: %s", request.URL.Path)
		writer.WriteHeader(http.StatusForbidden)
	}))
	test.Cleanup(replacement.Close)

	for _, fixture := range []struct {
		name string
		key  string
	}{
		{name: "verified subject", key: "identity-first"},
		{name: "legacy fingerprint", key: "identity-email-only"},
	} {
		test.Run(fixture.name, func(test *testing.T) {
			oldPeer, _ := providerPeer(test, original.URL)
			account := readFixtureAccount(test, finishFixtureSignIn(test, oldPeer, "api-key", outbound.Credentials{Key: fixture.key}))
			peer, _ := providerPeer(test, replacement.URL)
			useFixtureAccount(peer, account)

			result, _ := peer.call(test, sdk.MethodAuthRefresh, sdk.AuthRefreshRequest{})
			if result.Error == nil || result.Error.Code != "permission_denied" || result.Error.Outcome != "not-started" {
				test.Fatalf("refresh did not reject credentials from another server: %v", result.Error)
			}
			result, _ = peer.call(test, sdk.MethodAuthLogout, sdk.AuthLogoutRequest{})
			if result.Error != nil {
				test.Fatal("local logout failed after changing the server", result.Error)
			}

			peer.scope.Generation = 0
			result, _ = peer.call(test, sdk.MethodAuthLogout, sdk.AuthLogoutRequest{})
			if result.Error == nil || result.Error.Code != "permission_denied" {
				test.Fatal("logout admitted a missing generation")
			}
		})
	}
}

func TestProviderSignInPreservesLegacySlot(t *testing.T) {
	server := identityFixture(t)
	for _, test := range []struct {
		name     string
		previous outbound.Credentials
		rejected bool
	}{
		{"active", outbound.Credentials{Key: "identity-first"}, false},
		{"expired", outbound.Credentials{Key: "identity-first", ExpiresAt: time.Now().Add(-time.Hour)}, true},
		{"revoked", outbound.Credentials{Key: "identity-revoked"}, true},
		{"unknown", outbound.Credentials{Key: "identity-unknown"}, true},
		{"different organization", outbound.Credentials{Key: "identity-other-organization"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			peer, _ := providerPeer(t, server.URL)
			previous, _ := json.Marshal(test.previous)
			slot := test.previous.AccountID(server.URL)
			useFixtureAccount(peer, sdk.AuthResult{ID: slot, Credentials: previous})
			result := finishFixtureSignIn(t, peer, "api-key", outbound.Credentials{Key: "identity-replacement"})
			if test.rejected {
				if result.Error == nil || result.Error.Code != "permission_denied" {
					t.Fatal("legacy slot changed without proof of its prior subject")
				}
				return
			}
			migrated := readFixtureAccount(t, result)
			if migrated.ID != slot {
				t.Fatal("legacy upgrade changed the selected host account ID")
			}
			useFixtureAccount(peer, migrated)
			again := readFixtureAccount(t, finishFixtureSignIn(t, peer, "api-key", outbound.Credentials{Key: "identity-first"}))
			if again.ID != slot {
				t.Fatal("subsequent key rotation abandoned the migrated slot")
			}
		})
	}
}

func TestProviderImportsSelectedCommunityEntry(t *testing.T) {
	server := identityFixture(t)
	peer, _ := providerPeer(t, server.URL)
	record, _ := json.Marshal(map[string]any{
		"type": "api", "key": "identity-first", "accountId": "unverified-slot",
		"metadata": map[string]any{"email": "unverified-name", "server": server.URL + "/",
			"orgId": "unverified-organization", "teamId": "unverified-team", "plan": "unverified-plan"},
	})
	saved := map[string]string{"record": string(record)}
	imported := readFixtureAccount(t, finishFixtureSignIn(t, peer, sdk.SavedSignInMethod, saved))
	direct := readFixtureAccount(t, finishFixtureSignIn(t, peer, "api-key", outbound.Credentials{Key: "identity-first"}))
	if imported.ID != direct.ID || imported.Name != direct.Name ||
		strings.Contains(string(imported.Credentials), "unverified") {
		t.Fatal("saved record overrode vendor identity or persisted unrelated metadata")
	}
	for _, raw := range []string{
		`{"devin":` + string(record) + `}`,
		`{"type":"api","key":"identity-first","metadata":{"server":"https://elsewhere.invalid"}}`,
		`{"type":"api","key":"identity-first"}`,
		`{"type":"api","key":"identity-first","metadata":{"server":null}}`,
		`{"type":"api","key":"identity-first","key":"identity-other-user"}`,
	} {
		rejected := finishFixtureSignIn(t, peer, sdk.SavedSignInMethod, map[string]string{"record": raw})
		if rejected.Error == nil || rejected.Error.Code != "invalid_params" {
			t.Fatal("ambiguous sign-in or different origin was accepted")
		}
	}
}

func TestProviderImportsCommunityDefaultServer(t *testing.T) {
	for _, test := range []struct {
		name, record string
		linked       bool
	}{
		{"API key", `{"type":"api","key":"fixture-key","metadata":{"email":"fixture@example.invalid","orgId":"org-1","teamId":"team-1","plan":"Devin Pro"}}`, false},
		{"CLI-linked", `{"type":"api","key":"fixture-key","metadata":{"email":"fixture@example.invalid","orgId":"org-1","teamId":"team-1","plan":"Devin Pro","cli":"true"}}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, err := json.Marshal(map[string]string{"record": test.record})
			if err != nil {
				t.Fatal(err)
			}
			credentials, identity, err := importedSignIn(input, outbound.DefaultServer)
			if err != nil || credentials.Key != "fixture-key" || (identity != nil) != test.linked {
				t.Fatal("default community record failed to preserve the credential or sign-in type")
			}
			if identity != nil && (identity.TeamID != "team-1" || identity.OrgID != "org-1") {
				t.Fatal("default community CLI record lost its tenant")
			}
		})
	}
}

func TestProviderSavedSignInRequiresVerifiedSubject(t *testing.T) {
	server := identityFixture(t)
	peer, _ := providerPeer(t, server.URL)
	for _, key := range []string{"identity-empty-status", "identity-email-only", "identity-unknown"} {
		t.Run(key, func(t *testing.T) {
			record, err := json.Marshal(map[string]any{
				"type": "api", "key": key, "accountId": "uid-from-saved-file",
				"metadata": map[string]string{"server": server.URL, "email": "same@example.invalid"},
			})
			if err != nil {
				t.Fatal(err)
			}
			result := finishFixtureSignIn(t, peer, sdk.SavedSignInMethod, map[string]string{"record": string(record)})
			if result.Error == nil || result.Error.Code != "reauthorization_required" || len(result.Result) != 0 ||
				strings.Contains(result.Error.Message, key) {
				t.Fatal("saved sign-in did not refuse an unverified identity without exposing credentials")
			}
			direct := readFixtureAccount(t, finishFixtureSignIn(t, peer, "api-key", outbound.Credentials{Key: key}))
			if direct.ID != (outbound.Credentials{Key: key}).AccountID(server.URL) {
				t.Fatal("direct API-key sign-in lost its fingerprint fallback")
			}
		})
	}
}

func fixtureCLISignIn(t *testing.T) func(string, string) {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("XDG_DATA_HOME", directory)
	t.Setenv("APPDATA", directory)
	t.Setenv("WINDSURF_API_SERVER_URL", "")
	path := filepath.Join(directory, "devin")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return func(key, server string) {
		t.Helper()
		body := "windsurf_api_key = \"" + key + "\"\napi_server_url = \"" + server + "\"\n"
		if err := os.WriteFile(filepath.Join(path, "credentials.toml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func fixtureCLIRecord(key, server, team, org string, flag any) map[string]string {
	record, _ := json.Marshal(map[string]any{
		"type": "api", "key": key, "metadata": map[string]any{
			"server": server + "/", "cli": flag, "orgId": org, "teamId": team,
		},
	})
	return map[string]string{"record": string(record)}
}

func TestProviderImportsVerifiedCLISignIn(t *testing.T) {
	server := identityFixture(t)
	writeCLI := fixtureCLISignIn(t)
	for _, cli := range []any{true, "true"} {
		peer, _ := providerPeer(t, server.URL)
		writeCLI("identity-replacement", server.URL)
		legacy := fixtureCLIRecord("identity-first", server.URL, "team-1", "organization-1", cli)
		imported := readFixtureAccount(t, finishFixtureSignIn(t, peer, sdk.SavedSignInMethod, legacy))
		if imported.Name != "new@example.invalid · Fixture organization" ||
			!strings.Contains(string(imported.Credentials), `"key":"identity-replacement"`) || imported.ExpiresAt != 0 {
			t.Fatal("CLI import did not publish the verified current credential")
		}
		useFixtureAccount(peer, imported)
		writeCLI("identity-other-user", server.URL)
		refreshed, _ := peer.call(t, sdk.MethodAuthRefresh, sdk.AuthRefreshRequest{})
		if refreshed.Error == nil || refreshed.Error.Code != "permission_denied" || len(refreshed.Result) != 0 {
			t.Fatal("refresh followed another CLI user or exposed credentials")
		}
		writeCLI("identity-first", server.URL)
		refreshed, _ = peer.call(t, sdk.MethodAuthRefresh, sdk.AuthRefreshRequest{})
		active := readFixtureAccount(t, refreshed)
		if active.ID != imported.ID || active.Name != "same@example.invalid · Fixture organization" ||
			!strings.Contains(string(active.Credentials), `"key":"identity-first"`) || active.ExpiresAt != 0 {
			t.Fatal("verified CLI refresh changed the account slot or lost the new credential")
		}
	}
}

func TestProviderRefusesUnverifiedCLIImport(t *testing.T) {
	server := identityFixture(t)
	writeCLI := fixtureCLISignIn(t)
	for _, test := range []struct {
		name, saved, current, team, org, cliServer string
		flag                                       any
	}{
		{"different user", "identity-first", "identity-other-user", "team-1", "organization-1", server.URL, "true"},
		{"different team", "identity-first", "identity-other-team", "team-1", "organization-1", server.URL, "true"},
		{"different organization", "identity-first", "identity-other-organization", "team-1", "organization-1", server.URL, true},
		{"saved key rejected", "identity-revoked", "identity-first", "team-1", "organization-1", server.URL, true},
		{"saved key has no UID", "identity-unknown", "identity-first", "team-1", "organization-1", server.URL, true},
		{"saved tenant contradicts status", "identity-first", "identity-replacement", "team-2", "organization-1", server.URL, true},
		{"team fallback without an organization", "identity-no-organization", "identity-no-organization", "team-1", "team-1", server.URL, true},
		{"different server", "identity-first", "identity-first", "team-1", "organization-1", "https://other.invalid", true},
		{"ambiguous marker", "identity-first", "identity-first", "team-1", "organization-1", server.URL, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			peer, _ := providerPeer(t, server.URL)
			writeCLI(test.current, test.cliServer)
			entry := fixtureCLIRecord(test.saved, server.URL, test.team, test.org, test.flag)
			refused := finishFixtureSignIn(t, peer, sdk.SavedSignInMethod, entry)
			if refused.Error == nil || len(refused.Result) != 0 ||
				strings.Contains(refused.Error.Message, test.saved) || strings.Contains(refused.Error.Message, test.current) {
				t.Fatal("unverified CLI import published an account or exposed credentials")
			}
		})
	}
}

func TestProviderCLICredentialsFlatFormat(t *testing.T) {
	valid := "# Official CLI configuration\r\n" +
		"windsurf_api_key = 'synthetic-key'\r\n" +
		"api_server_url = \"https://server.codeium.com\"\r\n" +
		"devin_webapp_host = \"app.devin.ai\"\r\n" +
		"dangerously_skip_plugin_authentication = false\r\n"
	fields, validFormat := parseCLICredentials(valid)
	if !validFormat || fields["windsurf_api_key"] != "synthetic-key" ||
		fields["api_server_url"] != outbound.DefaultServer {
		t.Fatal("supported CLI file format was refused or decoded differently")
	}
	for _, malformed := range []string{
		valid + "windsurf_api_key = \"replacement\"\n",
		valid + "[account]\nwindsurf_api_key = \"replacement\"\n",
		valid + "devin_api_url = \"unterminated\n",
		valid + "devin_api_url = \"\\x61\"\n",
		valid + "# \xff",
	} {
		if _, accepted := parseCLICredentials(malformed); accepted {
			t.Fatal("ambiguous or malformed CLI file was accepted")
		}
	}
}
