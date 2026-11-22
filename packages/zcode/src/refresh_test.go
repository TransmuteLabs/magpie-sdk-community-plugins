package zcode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func renewableCredentials(kind string) Credentials {
	account := fixtureCredentials(kind)
	account.BusinessToken = "fixture-business-token"
	account.BusinessExpiresAt = fixtureNow.Add(2 * time.Hour).UnixMilli()
	account.Location = &ProjectLocation{OrganizationID: "fixture-org", ProjectID: "fixture-project", APIKeyID: "fixture-key-id"}
	return account
}

func refreshScope(account Credentials) sdk.Scope {
	selected := scope(account)
	selected.Session = ""
	return selected
}

func TestPATRefreshUsesBusinessRoleAndReturnsOnlyReplacement(t *testing.T) {
	for _, kind := range []string{"individual", "team"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(request.Body)
				if request.Method != http.MethodPost || request.URL.Path != "/api/biz/v1/organization/fixture-org/projects/fixture-project/api_keys/fixture-key-id/access_tokens" ||
					request.Header.Get("Authorization") != "Bearer fixture-business-token" ||
					request.Header.Get("X-Api-Key") != "" || request.Header.Get("Cookie") != "" ||
					!strings.Contains(string(body), `"clientType":"zcode"`) || !strings.Contains(string(body), `"clientVersion":"3.18.2"`) {
					t.Error("PAT issuer mixed roles, targets or request schema")
				}
				writer.Header().Set("Content-Type", "application/json")
				json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
					"enable": true, "accessToken": "replacement-pat", "tokenType": "Bearer",
					"expiresIn": 600, "expiresAt": fixtureNow.Add(10 * time.Minute).Unix(),
				}})
			}))
			defer server.Close()
			h := newHarness(t, endpointOptions(server))
			account := renewableCredentials(kind)
			expectedName := "fixture-user · ZCode zai individual"
			if kind == "team" {
				account.DisplayName = "fixture@example.invalid"
				expectedName = "fixture@example.invalid · ZCode zai team"
			}
			account.ExpiresAt = fixtureNow.Add(-time.Minute).UnixMilli()
			frame, _ := h.call(sdk.MethodAuthRefresh, refreshScope(account), sdk.AuthRefreshRequest{}, raw(account))
			var result sdk.AuthResult
			var replacement Credentials
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || json.Unmarshal(result.Credentials, &replacement) != nil {
				t.Fatalf("PAT refresh failed: %+v", frame.Error)
			}
			if result.Name != expectedName || result.ID != account.AccountID() || replacement.ProjectToken != "replacement-pat" ||
				result.ExpiresAt != fixtureNow.Add(10*time.Minute).UnixMilli() || replacement.ExpiresAt != result.ExpiresAt ||
				replacement.BusinessToken != account.BusinessToken || replacement.BusinessExpiresAt != account.BusinessExpiresAt ||
				replacement.AccountID() != account.AccountID() || h.accepted.Load() || calls.Load() != 1 {
				t.Fatal("refresh lost account identity, extended lifetime, emitted a stream event or replayed")
			}
		})
	}
}

func TestPATRefreshFailuresAreUncertainAndNeverRetried(t *testing.T) {
	for _, mode := range []string{"503", "malformed", "expired", "wrong-token-type", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				writer.Header().Set("Content-Type", "application/json")
				if mode == "503" {
					writer.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				if mode == "malformed" {
					io.WriteString(writer, `{"code":0}`)
					return
				}
				data := map[string]any{"accessToken": "replacement", "tokenType": "Bearer",
					"expiresIn": 600, "expiresAt": fixtureNow.Add(time.Hour).Unix()}
				switch mode {
				case "expired":
					data["expiresAt"] = fixtureNow.Add(-time.Second).Unix()
				case "wrong-token-type":
					data["tokenType"] = "Unknown"
				case "disabled":
					data["enable"] = false
				}
				json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": data})
			}))
			defer server.Close()
			h := newHarness(t, endpointOptions(server))
			account := renewableCredentials("individual")
			frame, _ := h.call(sdk.MethodAuthRefresh, refreshScope(account), sdk.AuthRefreshRequest{}, raw(account))
			if frame.Error == nil || frame.Error.Outcome != "uncertain" || calls.Load() != 1 || h.accepted.Load() {
				t.Fatal("uncertain issuer outcome was retried or reported as not started")
			}
		})
	}
}

func TestPATRefreshPrerequisitesRejectBeforeNetwork(t *testing.T) {
	for _, mode := range []string{"business-missing", "business-expired", "location", "team-location", "account", "generation", "session", "credentials-param"} {
		t.Run(mode, func(t *testing.T) {
			h := newHarness(t, ModuleOptions{})
			account := renewableCredentials("team")
			selected := refreshScope(account)
			params := sdk.AuthRefreshRequest{}
			switch mode {
			case "business-missing":
				account.BusinessToken, account.BusinessExpiresAt = "", 0
			case "business-expired":
				account.BusinessExpiresAt = fixtureNow.UnixMilli()
			case "location":
				account.Location = nil
				selected.Account = account.AccountID()
			case "team-location":
				account.Location.ProjectID = "another-project"
				selected.Account = account.AccountID()
			case "account":
				selected.Account = "foreign"
			case "generation":
				selected.Generation = 0
			case "session":
				selected.Session = "conversation"
			case "credentials-param":
				params.Credentials = raw(account)
			}
			frame, _ := h.call(sdk.MethodAuthRefresh, selected, params, raw(account))
			if frame.Error == nil || frame.Error.Outcome != "not-started" || h.proxies != 0 || h.accepted.Load() {
				t.Fatalf("invalid refresh reached dispatch: %+v", frame.Error)
			}
		})
	}
}

func TestPATExpiryStartsBeforeSlowExchangeAndCannotOverflow(t *testing.T) {
	reply := []byte(`{"code":0,"data":{"accessToken":"fixture","tokenType":"Bearer","expiresIn":600,"expiresAt":9999999999}}`)
	_, expiry, err := parsePAT(reply, fixtureNow, fixtureNow.Add(4*time.Minute))
	if err != nil || expiry != fixtureNow.Add(10*time.Minute).UnixMilli() {
		t.Fatal("slow reply extended relative TTL")
	}
	for _, body := range []string{
		`{"code":0,"data":{"accessToken":"fixture","tokenType":"Bearer","expiresIn":1e100,"expiresAt":9999999999}}`,
		`{"code":0,"data":{"accessToken":"fixture","tokenType":"Bearer","expiresIn":600,"expiresAt":1e100}}`,
		`{"code":0,"data":{"accessToken":"fixture","tokenType":"Bearer","expiresIn":-1,"expiresAt":9999999999}}`,
	} {
		if _, _, err := parsePAT([]byte(body), fixtureNow, fixtureNow); err == nil {
			t.Fatal("issuer lifetime overflow accepted")
		}
	}
}

func TestPATRefreshRequiresTransactionalHostFloor(t *testing.T) {
	manifest := Manifest()
	old := sdk.FullContract()
	old.Version.Minor = 2
	if old.Check(manifest.Contract) == nil || manifest.Compatibility.HostAPI != "^1.20" {
		t.Fatal("a host without transactional refresh can admit this module")
	}
	old.Version.Minor = 3
	if old.Check(manifest.Contract) == nil {
		t.Fatal("a host without model-scoped quotas and business cancellation can admit this module")
	}
	old.Version.Minor = 19
	if old.Check(manifest.Contract) == nil {
		t.Fatal("a host without typed rate-limited check-in can admit this module")
	}
}

func TestBusinessBootstrapAndPATPublishOneUnaryReplacement(t *testing.T) {
	var logins, mints atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/auth/z/login":
			logins.Add(1)
			var body map[string]string
			json.NewDecoder(request.Body).Decode(&body)
			if body["token"] != "fixture-bootstrap" || request.Header.Get("Authorization") != "" {
				t.Error("business exchange received a PAT or session token")
			}
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"access_token": "new-business", "expires_in": 1200,
			}})
		case "/api/biz/v1/organization/fixture-org/projects/fixture-project/api_keys/fixture-key-id/access_tokens":
			mints.Add(1)
			if request.Header.Get("Authorization") != "Bearer new-business" {
				t.Error("PAT issuance used stale business material")
			}
			json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{
				"accessToken": "new-pat", "tokenType": "Bearer", "expiresIn": 600,
				"expiresAt": fixtureNow.Add(10 * time.Minute).Unix(),
			}})
		default:
			t.Error("unexpected bootstrap target")
		}
	}))
	defer server.Close()
	h := newHarness(t, endpointOptions(server))
	account := renewableCredentials("individual")
	account.BusinessExpiresAt = fixtureNow.Add(-time.Minute).UnixMilli()
	account.OAuthToken, account.OAuthExpiresAt = "fixture-bootstrap", fixtureNow.Add(time.Hour).UnixMilli()
	account.ExpiresAt = fixtureNow.Add(-time.Minute).UnixMilli()
	frame, _ := h.call(sdk.MethodAuthRefresh, refreshScope(account), sdk.AuthRefreshRequest{}, raw(account))
	var result sdk.AuthResult
	var replacement Credentials
	if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil ||
		json.Unmarshal(result.Credentials, &replacement) != nil {
		t.Fatalf("business/PAT exchange failed: %+v", frame.Error)
	}
	if logins.Load() != 1 || mints.Load() != 1 || h.accepted.Load() ||
		replacement.BusinessToken != "new-business" || replacement.ProjectToken != "new-pat" ||
		replacement.BusinessExpiresAt != fixtureNow.Add(20*time.Minute).UnixMilli() ||
		replacement.OAuthToken != account.OAuthToken || replacement.AccountID() != account.AccountID() {
		t.Fatal("two-stage refresh lost identity, replayed or streamed partial credentials")
	}
}

func TestBusinessRenewalWithoutLifetimeEvidenceRemainsUncertain(t *testing.T) {
	for _, body := range []string{
		`{"code":0,"data":{"access_token":"opaque"}}`,
		`{"code":0,"data":{"access_token":"opaque","expires_in":1e100}}`,
		`{"code":0,"success":false,"data":{"access_token":"opaque","expires_in":600}}`,
	} {
		if _, _, err := businessReply([]byte(body), fixtureNow, fixtureNow); err == nil {
			t.Fatal("unknown business expiry was promoted into valid renewable credentials")
		}
	}
}
