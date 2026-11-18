package zcode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestTeamHeadlessSelectionAndMissingBoundKey(t *testing.T) {
	for _, scenario := range []string{"selected", "ambiguous", "missing-bound-key", "membership-invalid"} {
		t.Run(scenario, func(t *testing.T) {
			var issued, created atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/biz/customer/getCustomerInfo":
					io.WriteString(writer, `{"code":0,"data":{"organizations":[{"organizationId":"org","projects":[{"projectId":"first","projectType":2},{"projectId":"selected","projectType":2}]}]}}`)
				case "/api/biz/team/subscribe/product/querySubscribeDetail":
					if request.Header.Get("Bigmodel-Organization") != "org" || request.Header.Get("Bigmodel-Project") != "selected" {
						t.Error("Team selection did not scope its entitlement")
					}
					status := "VALID"
					if scenario == "membership-invalid" {
						status = "REVOKED"
					}
					fmt.Fprintf(writer, `{"code":0,"data":{"productId":"product","hasSubscription":true,"status":"EFFECTIVE","memberGrantStatus":%q}}`, status)
				case "/api/biz/v1/organization/org/projects/selected/api_keys":
					if request.Method == http.MethodPost {
						created.Add(1)
						t.Error("existing/bound Team key was unnecessarily created")
					}
					io.WriteString(writer, `{"code":0,"data":[{"apiKey":"key","name":"zcode-team-api-key","keyType":2},{"apiKey":"other","name":"zcode-api-key","keyType":1}]}`)
				case "/api/biz/v1/organization/org/projects/selected/api_keys/key/access_tokens":
					issued.Add(1)
					if request.Header.Get("Authorization") != "Bearer "+fixtureJWT(fixtureNow.Add(2*time.Hour)) {
						t.Error("PAT issuance did not use selected business material")
					}
					fmt.Fprintf(writer, `{"code":0,"data":{"accessToken":"team-pat","tokenType":"Bearer","expiresIn":600,"expiresAt":%d}}`,
						fixtureNow.Add(10*time.Minute).Unix())
				default:
					t.Errorf("unexpected Team auth path %s", request.URL.Path)
					writer.WriteHeader(404)
				}
			}))
			defer server.Close()
			h := newHarness(t, endpointOptions(server))
			flow := readyStartFlow("team-user")
			flow.Selection = authSelection{Site: "bigmodel", Kind: "team", Organization: "org", Project: "selected", Product: "product"}
			if scenario == "ambiguous" {
				flow.Selection.Organization, flow.Selection.Project = "", ""
			}
			if scenario == "missing-bound-key" {
				flow.Selection.APIKey = "previously-bound"
			}
			flow.Ready = raw(map[string]any{
				"status": "ready", "token": fixtureJWT(fixtureNow.Add(time.Hour)),
				"user":     map[string]string{"user_id": "team-user"},
				"bigmodel": map[string]string{"access_token": fixtureJWT(fixtureNow.Add(2 * time.Hour))},
			})
			selected := sdk.Scope{Provider: ProviderID}
			frame, _ := h.call(sdk.MethodAuthFinish, selected, seedFlow(h, selected, flow), nil)
			if scenario != "selected" {
				if frame.Error == nil || created.Load() != 0 || issued.Load() != 0 {
					t.Fatal("unavailable/ambiguous selected Team identity caused a paid mutation")
				}
				return
			}
			var result sdk.AuthResult
			var account Credentials
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil ||
				json.Unmarshal(result.Credentials, &account) != nil || account.Kind != "team" ||
				account.Organization != "org" || account.Project != "selected" || account.Product != "product" ||
				account.Location == nil || account.Location.APIKeyID != "key" || issued.Load() != 1 || created.Load() != 0 {
				t.Fatalf("selected Team auth lost scope: %+v", frame.Error)
			}
		})
	}
}
