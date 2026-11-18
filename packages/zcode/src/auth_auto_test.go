package zcode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestCompletionAutoSignInConfirmsTeamBeforeStartAndKeepsUnknownUnselected(t *testing.T) {
	for _, scenario := range []string{"team", "team-optional-code", "personal-expired-team", "no-seat", "expired", "unassigned", "unknown", "new-status", "new-grant", "ambiguous"} {
		t.Run(scenario, func(t *testing.T) {
			var issued, starts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/biz/customer/getCustomerInfo":
					projects := `[{"projectId":"seat","projectType":2}]`
					if scenario == "ambiguous" {
						projects = `[{"projectId":"seat","projectType":2},{"projectId":"other","projectType":2}]`
					} else if scenario == "personal-expired-team" {
						projects = `[{"projectId":"personal","projectType":1},{"projectId":"seat","projectType":2}]`
					}
					fmt.Fprintf(writer, `{"code":0,"data":{"organizations":[{"organizationId":"org","projects":%s}]}}`, projects)
				case "/api/biz/team/subscribe/product/querySubscribeDetail":
					if request.Header.Get("Bigmodel-Organization") != "org" || request.Header.Get("Bigmodel-Project") != "seat" {
						t.Error("auto Team probe escaped its discovered scope")
					}
					if scenario == "unknown" {
						io.WriteString(writer, `{"code":0,"data":{}}`)
					} else if scenario == "no-seat" {
						io.WriteString(writer, `{"code":0,"data":{"hasSubscription":false}}`)
					} else if scenario == "expired" {
						io.WriteString(writer, `{"code":0,"data":{"hasSubscription":true,"status":"EXPIRED"}}`)
					} else if scenario == "unassigned" {
						io.WriteString(writer, `{"code":0,"data":{"hasSubscription":true,"status":"EFFECTIVE","memberGrantStatus":"UNASSIGNED"}}`)
					} else if scenario == "new-status" {
						io.WriteString(writer, `{"code":0,"data":{"hasSubscription":true,"status":"NEW_UNKNOWN","memberGrantStatus":"VALID","productId":"product"}}`)
					} else if scenario == "new-grant" {
						io.WriteString(writer, `{"code":0,"data":{"hasSubscription":true,"status":"EFFECTIVE","memberGrantStatus":"NEW_UNKNOWN","productId":"product"}}`)
					} else if scenario == "team-optional-code" {
						io.WriteString(writer, `{"data":{"hasSubscription":true,"status":"EFFECTIVE","memberGrantStatus":"VALID","productId":"product"}}`)
					} else {
						io.WriteString(writer, `{"code":0,"data":{"hasSubscription":true,"status":"EFFECTIVE","memberGrantStatus":"VALID","productId":"product"}}`)
					}
				case "/api/biz/v1/organization/org/projects/seat/api_keys":
					if request.Method != http.MethodGet {
						t.Error("auto selection created an unsolicited key")
					}
					io.WriteString(writer, `{"code":0,"data":[{"apiKey":"team-key","name":"zcode-team-api-key","keyType":2}]}`)
				case "/api/biz/v1/organization/org/projects/seat/api_keys/team-key/access_tokens":
					issued.Add(1)
					fmt.Fprintf(writer, `{"code":0,"data":{"accessToken":"selected-team-pat","tokenType":"Bearer","expiresIn":600,"expiresAt":%d}}`,
						fixtureNow.Add(10*time.Minute).Unix())
				case "/api/biz/v1/organization/org/projects/personal/api_keys":
					if scenario != "personal-expired-team" || request.Method != http.MethodGet {
						t.Error("unexpected Personal key selection")
					}
					io.WriteString(writer, `{"code":0,"data":[{"apiKey":"personal-key","name":"zcode-api-key"}]}`)
				case "/api/biz/v1/organization/org/projects/personal/api_keys/personal-key/access_tokens":
					issued.Add(1)
					fmt.Fprintf(writer, `{"code":0,"data":{"accessToken":"personal-pat","tokenType":"Bearer","expiresIn":600,"expiresAt":%d}}`,
						fixtureNow.Add(10*time.Minute).Unix())
				case "/api/biz/subscription/list":
					if scenario != "personal-expired-team" {
						t.Error("auto selection probed an unsolicited Personal plan")
					}
					io.WriteString(writer, `{"code":0,"data":[{"productName":"Coding Plan","status":"EXPIRED","inCurrentPeriod":false}]}`)
				case "/api/v1/zcode-plan/billing/balance":
					starts.Add(1)
					io.WriteString(writer, `{"code":0,"data":{"plans":[{"plan_id":"start","status":"active"}],"balances":[]}}`)
				default:
					t.Errorf("unexpected auto auth endpoint %s", request.URL.Path)
					writer.WriteHeader(404)
				}
			}))
			defer server.Close()
			h := newHarness(t, endpointOptions(server))
			flow := readyStartFlow("auto-user")
			flow.Selection = authSelection{Site: "bigmodel", Kind: "auto"}
			flow.Ready = raw(map[string]any{"status": "ready", "token": fixtureJWT(fixtureNow.Add(time.Hour)),
				"user":     map[string]string{"user_id": "auto-user"},
				"bigmodel": map[string]string{"access_token": fixtureJWT(fixtureNow.Add(2 * time.Hour))}})
			selected := sdk.Scope{Provider: ProviderID}
			frame, _ := h.call(sdk.MethodAuthFinish, selected, seedFlow(h, selected, flow), nil)
			if scenario == "unknown" || scenario == "ambiguous" || strings.HasPrefix(scenario, "new-") {
				if frame.Error == nil || issued.Load() != 0 || starts.Load() != 0 {
					t.Fatalf("unknown/ambiguous Team selection minted or fell through to Start: %+v", frame.Error)
				}
				return
			}
			var result sdk.AuthResult
			var account Credentials
			if frame.Error != nil || json.Unmarshal(frame.Result, &result) != nil || json.Unmarshal(result.Credentials, &account) != nil {
				t.Fatalf("automatic selected entitlement did not complete: %+v", frame.Error)
			}
			paid := scenario == "team" || scenario == "team-optional-code" || scenario == "personal-expired-team"
			mints := int32(1)
			if scenario == "personal-expired-team" {
				mints = 2
			}
			if paid && (account.Kind != "team" || account.Organization != "org" || account.Project != "seat" ||
				account.Product != "product" || issued.Load() != mints || starts.Load() != 0) ||
				!paid && (account.Kind != "start" || issued.Load() != 0 || starts.Load() != 1) {
				t.Fatalf("Personal→Team→Start selection lost its confirmed order: %+v", account)
			}
			for _, secret := range []string{account.BusinessToken, account.ProjectToken} {
				if secret != "" && strings.Contains(result.Name, secret) {
					t.Fatal("selected automatic account name contains a credential")
				}
			}
		})
	}
}

func TestCompletionSelectedTeamProductCannotBecomeAbsent(t *testing.T) {
	available, err := teamAvailable(map[string]any{
		"hasSubscription": true, "status": "EFFECTIVE", "memberGrantStatus": "VALID", "productId": "changed-product",
	}, "selected-product")
	if err == nil || available {
		t.Fatal("changed selected Team product became a confirmed absent entitlement")
	}
}

func TestCompletionMalformedInactiveTeamIsNotConfirmedAbsence(t *testing.T) {
	available, err := teamAvailable(map[string]any{
		"hasSubscription": false, "productId": map[string]any{"malformed": true},
	}, "")
	if err == nil || available {
		t.Fatal("malformed inactive Team schema became confirmed absent")
	}
}
