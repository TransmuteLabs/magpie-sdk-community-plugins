package minimax

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestSelectedDeviceReauthorizationKeepsIdentity(t *testing.T) {
	for _, item := range []struct {
		name, provider, oldUser, newUser, newSubject string
		allowed                                      bool
	}{
		{"profile-unavailable", China, "fixture-user", "", "fixture-subject", true},
		{"profile-restored", Global, "", "fixture-user", "fixture-subject", true},
		{"conflicting-profile", Global, "fixture-user", "another-user", "fixture-subject", false},
		{"conflicting-token", China, "fixture-user", "fixture-user", "another-subject", false},
	} {
		t.Run(item.name, func(t *testing.T) {
			var now atomic.Int64
			now.Store(1790000000000)
			before := Credentials{Site: item.provider, Access: "old-access", Refresh: "old-refresh",
				Subject: "fixture-subject", SubjectKind: "sub", UserID: item.oldUser, Email: "known@example.invalid"}
			identity := item.oldUser
			if identity == "" {
				identity = before.Subject
			}
			before.ID = accountID(item.provider, identity)
			scope := sdk.Scope{Provider: item.provider, Account: before.ID, Generation: 7}
			transport := coreDoer(func(request *http.Request) (*http.Response, error) {
				switch request.URL.Path {
				case "/oauth2/device/code":
					return coreResponse(200, `{"device_code":"fixture-code","user_code":"FIXTURE","verification_uri":"https://approve.example.invalid/","expires_in":60,"interval":1}`, "application/json"), nil
				case "/oauth2/token":
					payload := base64.RawURLEncoding.EncodeToString(raw(map[string]string{"sub": item.newSubject}))
					return coreResponse(200, fmt.Sprintf(`{"access_token":"e30.%s.fixture","refresh_token":"new-refresh","expires_in":60}`, payload), "application/json"), nil
				case "/v1/api/user/info":
					if item.newUser == "" {
						return coreResponse(503, `{"message":"fixture unavailable"}`, "application/json"), nil
					}
					return coreResponse(200, fmt.Sprintf(`{"data":{"userInfo":{"realUserID":%q,"email":"updated@example.invalid"}}}`, item.newUser), "application/json"), nil
				default:
					t.Errorf("unexpected sign-in request: %s", request.URL.Path)
					return coreResponse(404, `{}`, "application/json"), nil
				}
			})
			fixture := newCoreHarness(t, NewModule(coreOptions(&now, transport)))
			started, _ := fixture.invoke(sdk.MethodAuthStart, scope, sdk.AuthStartRequest{Method: "device"}, &before)
			var challenge sdk.AuthStartResult
			if started.Error != nil || json.Unmarshal(started.Result, &challenge) != nil || len(challenge.Continuation) == 0 {
				t.Fatalf("start failed: %v", started.Error)
			}
			now.Add(1000)
			finish, _ := fixture.invoke(sdk.MethodAuthFinish, scope, sdk.AuthFinishRequest{Continuation: challenge.Continuation}, &before)
			if !item.allowed {
				if finish.Error == nil || len(finish.Result) != 0 {
					t.Fatalf("foreign identity replaced the selected account: %v", finish.Error)
				}
				return
			}
			var result sdk.AuthResult
			var credentials Credentials
			name := "updated@example.invalid"
			if item.newUser == "" {
				name = before.Email
			}
			if finish.Error != nil || json.Unmarshal(finish.Result, &result) != nil ||
				json.Unmarshal(result.Credentials, &credentials) != nil ||
				result.ID != before.ID || result.Name != name || credentials.ID != before.ID ||
				credentials.UserID != "fixture-user" || credentials.Refresh != "new-refresh" {
				t.Fatalf("reauthorization lost the selected account or name: ID=%q name=%q error=%v", result.ID, result.Name, finish.Error)
			}
		})
	}
}

func TestRotatingRefreshRecoversAReadablyNamedProfile(t *testing.T) {
	for _, item := range []struct {
		name     string
		profile  string
		wantName string
	}{
		{"profile-restored", "user-7", "fixture@example.invalid"},
		{"profile-unavailable", "", "MiniMax [4105d879e2a5]"},
		{"conflicting-profile", "different-user", ""},
	} {
		t.Run(item.name, func(t *testing.T) {
			var now atomic.Int64
			now.Store(1790000000000)
			scope, previous := coreAccount(Global)
			var exchanges, profiles atomic.Int32
			transport := coreDoer(func(request *http.Request) (*http.Response, error) {
				switch request.URL.Path {
				case "/oauth2/token":
					exchanges.Add(1)
					if err := request.ParseForm(); err != nil || request.Form.Get("refresh_token") != previous.Refresh {
						t.Error("refresh chose another account")
					}
					return coreResponse(200, `{"access_token":"renewed-access","refresh_token":"renewed-refresh","expires_in":3600}`, "application/json"), nil
				case "/v1/api/user/info":
					profiles.Add(1)
					if request.Header.Get("Authorization") != "Bearer renewed-access" {
						t.Error("profile read did not use the rotated token")
					}
					if item.profile == "" {
						return coreResponse(503, `{"message":"unavailable"}`, "application/json"), nil
					}
					return coreResponse(200, fmt.Sprintf(`{"data":{"userInfo":{"realUserID":%q,"email":"fixture@example.invalid"}}}`, item.profile), "application/json"), nil
				default:
					t.Errorf("unexpected refresh request: %s", request.URL.Path)
					return coreResponse(404, `{}`, "application/json"), nil
				}
			})
			fixture := newCoreHarness(t, NewModule(coreOptions(&now, transport)))
			frame, _ := fixture.invoke(sdk.MethodAuthRefresh, scope, sdk.AuthRefreshRequest{}, &previous)
			if exchanges.Load() != 1 || profiles.Load() != 1 {
				t.Fatalf("refresh sent %d exchanges and %d profile reads", exchanges.Load(), profiles.Load())
			}
			if item.wantName == "" {
				if frame.Error == nil || frame.Error.Outcome != "uncertain" || len(frame.Result) != 0 {
					t.Fatalf("conflicting identity accepted after a rotating refresh: %v", frame.Error)
				}
				return
			}
			var result sdk.AuthResult
			var replacement Credentials
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil ||
				json.Unmarshal(result.Credentials, &replacement) != nil || result.ID != previous.ID ||
				result.Name != item.wantName || replacement.Refresh != "renewed-refresh" {
				t.Fatalf("refresh did not preserve the account and publish its best known name: %q %q %v", result.ID, result.Name, frame.Error)
			}
		})
	}
}
