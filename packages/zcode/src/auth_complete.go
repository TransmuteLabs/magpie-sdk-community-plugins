package zcode

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func tokenDeadline(token string, ttl any, started time.Time) int64 {
	expiry := int64(math.MaxInt64)
	if seconds, valid := finiteNumber(ttl); valid && seconds > 0 && seconds*1000 < float64(math.MaxInt64-started.UnixMilli()) {
		expiry = started.UnixMilli() + int64(seconds*1000)
	}
	parts := strings.Split(token, ".")
	if len(parts) == 3 {
		data, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err == nil {
			claims, parseErr := protocol.ReadObject(data)
			if parseErr == nil {
				if seconds, valid := finiteNumber(claims["exp"]); valid && seconds > 0 && seconds*1000 < float64(math.MaxInt64) {
					expiry = min(expiry, int64(seconds*1000))
				}
			}
		}
	}
	if expiry == math.MaxInt64 {
		return 0
	}
	return expiry
}

func (owner *module) businessOrigin(account Credentials) string {
	address, _ := url.Parse(owner.options.Endpoints.quotaForAccount(account))
	return address.Scheme + "://" + address.Host
}

func (owner *module) mintPAT(ctx context.Context, stream *sdk.Stream, account Credentials) (Credentials, error) {
	if account.Location == nil || !safeText(account.BusinessToken) {
		return account, invalid("Paid sign-in requires an explicit discovered coding-plan location")
	}
	location := account.Location
	endpoint := owner.businessOrigin(account) + "/api/biz/v1/organization/" +
		url.PathEscape(location.OrganizationID) + "/projects/" + url.PathEscape(location.ProjectID) +
		"/api_keys/" + url.PathEscape(location.APIKeyID) + "/access_tokens"
	started := owner.options.Now()
	response, err := owner.vendorRequest(ctx, stream, http.MethodPost, endpoint,
		http.Header{"Authorization": {"Bearer " + strings.TrimPrefix(account.BusinessToken, "Bearer ")}},
		map[string]string{"clientType": "zcode", "clientVersion": ClientVersion})
	if err != nil || response.Status < 200 || response.Status >= 300 {
		return account, refreshFailure()
	}
	encoded, _ := json.Marshal(response.Data)
	token, expiry, err := parsePAT(encoded, started, owner.options.Now())
	if err != nil {
		return account, err
	}
	account.ProjectToken, account.ExpiresAt = token, expiry
	return account, nil
}

func (owner *module) completeFlow(ctx context.Context, call sdk.Call, stream *sdk.Stream, flow authFlow) (sdk.AuthResult, error) {
	ready, err := protocol.ReadObject(flow.Ready)
	if err != nil {
		return sdk.AuthResult{}, invalid("Authorization ready receipt is unreadable")
	}
	user, _ := ready["user"].(map[string]any)
	userID, _ := user["user_id"].(string)
	session, _ := ready["token"].(string)
	issued := time.UnixMilli(flow.ReadyAt)
	if flow.ReadyAt <= 0 {
		return sdk.AuthResult{}, invalid("Authorization receipt is missing its original issuance time")
	}
	sessionExpiry := tokenDeadline(session, ready["expires_in"], issued)
	if !safeText(userID) || !safeText(session) || sessionExpiry <= owner.options.Now().Add(30*time.Second).UnixMilli() {
		return sdk.AuthResult{}, invalid("Authorization lacks an authoritative user and unexpired ZCode session")
	}
	if flow.TargetUser != "" && flow.TargetUser != userID {
		return sdk.AuthResult{}, &sdk.Error{Code: "permission_denied", Message: "Reauthorization returned another vendor user", Outcome: "not-started"}
	}
	account := Credentials{Site: flow.Selection.Site, Kind: flow.Selection.Kind, LoginProvider: flow.Selection.LoginProvider,
		UserID: userID, Device: flow.Device, SessionToken: session,
		SessionExpiresAt: sessionExpiry, Discovery: true}
	var material map[string]any
	var access string
	if flow.Selection.Kind != "start" {
		if value, exists := ready[account.Site]; exists && value != nil {
			var valid bool
			material, valid = value.(map[string]any)
			if !valid {
				return sdk.AuthResult{}, invalid("Authorization returned malformed selected-family material")
			}
			for _, name := range []string{"access_token", "accessToken"} {
				if value, exists := material[name]; exists {
					text, valid := value.(string)
					if !valid || text != "" && !safeText(text) {
						return sdk.AuthResult{}, invalid("Authorization returned malformed selected-family access material")
					}
					if access == "" {
						access = text
					}
				}
			}
		}
	}
	if account.Kind == "auto" {
		account.Kind = "individual"
	}
	if flow.Selection.Kind == "start" || flow.Selection.Kind == "auto" && access == "" {
		account.Kind, account.ExpiresAt, account.SessionExpiresAt = "start", sessionExpiry, 0
		if _, err := owner.startEntitlement(ctx, stream, account, true); err != nil {
			return sdk.AuthResult{}, err
		}
	} else {
		if !safeText(access) {
			return sdk.AuthResult{}, invalid("Authorization did not return the selected family's business/bootstrap material")
		}
		if account.Site == "zai" {
			account.OAuthToken = access
			account.OAuthExpiresAt = tokenDeadline(access, material["expires_in"], issued)
			account, err = owner.refreshBusiness(ctx, stream, account)
			if err != nil {
				return sdk.AuthResult{}, err
			}
		} else {
			account.BusinessToken = access
			account.BusinessExpiresAt = tokenDeadline(access, material["expires_in"], issued)
			account.VendorRefreshToken, _ = material["refresh_token"].(string)
			if account.VendorRefreshToken == "" {
				account.VendorRefreshToken, _ = material["refreshToken"].(string)
			}
		}
		account, err = owner.completePaidFlow(ctx, stream, account, flow.Selection)
		if err != nil {
			var failure *sdk.Error
			if flow.Selection.Kind != "auto" || flow.TargetAccount != "" ||
				!errors.As(err, &failure) || failure.Code != "entitlement_unavailable" {
				return sdk.AuthResult{}, err
			}
			start := Credentials{Site: account.Site, LoginProvider: account.LoginProvider, Kind: "start", UserID: userID,
				Device: flow.Device, SessionToken: session, ExpiresAt: sessionExpiry, Discovery: true}
			if _, err := owner.startEntitlement(ctx, stream, start, true); err != nil {
				return sdk.AuthResult{}, unsupported("Neither a paid nor Start entitlement was confirmed")
			}
			account = start
		}
	}
	if err := account.Validate(owner.options.Now()); err != nil {
		return sdk.AuthResult{}, invalid("Completed authorization did not yield usable selected credentials")
	}
	if flow.TargetAccount != "" && account.AccountID() != flow.TargetAccount {
		return sdk.AuthResult{}, &sdk.Error{Code: "permission_denied", Message: "Reauthorization changed the selected account identity or plan", Outcome: "not-started"}
	}
	credentials, err := json.Marshal(account)
	if err != nil {
		return sdk.AuthResult{}, err
	}
	return sdk.AuthResult{ID: account.AccountID(), Name: "ZCode " + account.Site + " " + account.Kind,
		Credentials: credentials, ExpiresAt: account.ExpiresAt}, nil
}

func (owner *module) completePaidFlow(ctx context.Context, stream *sdk.Stream, account Credentials, selection authSelection) (Credentials, error) {
	account, err := owner.discoverPaidSelection(ctx, stream, account, selection)
	if err != nil {
		return account, err
	}
	account, err = owner.mintPAT(ctx, stream, account)
	if err != nil {
		return account, err
	}
	available, err := owner.paidEntitlement(ctx, stream, account)
	if err != nil {
		return account, err
	}
	if available {
		return account, nil
	}
	if selection.Kind == "auto" && account.Kind != "team" {
		selection.Kind = "team"
		return owner.completePaidFlow(ctx, stream, account, selection)
	}
	return account, paidUnavailable()
}
