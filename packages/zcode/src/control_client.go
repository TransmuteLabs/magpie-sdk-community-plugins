package zcode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func (owner *module) zcodeOrigin() string {
	address, _ := url.Parse(owner.options.Endpoints.Start)
	return address.Scheme + "://" + address.Host
}

func (owner *module) controlHeaders(account Credentials, lane string) http.Header {
	headers := http.Header{
		"Authorization":       {"Bearer " + account.SessionToken},
		"X-Zcode-App-Version": {ClientVersion},
		"User-Agent":          {"ZCode/" + ClientVersion},
		"Content-Type":        {"application/json"},
	}
	if lane == "offpeak" {
		headers.Set("X-Coding-Plan-Api-Key", account.ProjectToken)
		if account.Site == "bigmodel" && account.Kind == "team" {
			headers.Set("Bigmodel-Organization", account.Organization)
			headers.Set("Bigmodel-Project", account.Project)
		}
	} else if lane == "highspeed" {
		headers.Set("X-Bigmodel-Authorization", "Bearer "+account.BusinessToken)
		headers.Set("Bigmodel-Target-Type", "PERSONAL")
		if account.Kind == "team" {
			headers.Set("Bigmodel-Target-Type", "TEAM")
			headers.Set("Bigmodel-Organization", account.Organization)
			headers.Set("Bigmodel-Project", account.Project)
		}
	} else if lane == "reset" {
		headers.Set("X-Bigmodel-Authorization", account.BusinessToken)
		headers.Set("Bigmodel-Target-Type", "PERSONAL")
		if account.Kind == "team" {
			headers.Set("Bigmodel-Target-Type", "TEAM")
			headers.Set("Bigmodel-Organization", account.Organization)
			headers.Set("Bigmodel-Project", account.Project)
		}
	} else if lane == "reset-history" {
		headers.Set("X-Bigmodel-Authorization", account.BusinessToken)
	}
	return headers
}

func (owner *module) control(ctx context.Context, stream *sdk.Stream, account Credentials, lane, method, path string, body any) (int, []byte, error) {
	status, data, _, err := owner.controlDetailed(ctx, stream, account, lane, method, path, body)
	return status, data, err
}

func (owner *module) controlDetailed(ctx context.Context, stream *sdk.Stream, account Credentials, lane, method, path string, body any) (int, []byte, http.Header, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	endpoint := owner.zcodeOrigin() + path
	var proxy sdk.NetworkProxyResult
	if err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: endpoint}, &proxy); err != nil {
		return 0, nil, nil, err
	}
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return 0, nil, nil, err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return 0, nil, nil, err
	}
	request.Header = owner.controlHeaders(account, lane)
	request.Header.Set("X-Platform", platform())
	if locale, valid := ctx.Value(marketingLocaleKey{}).(string); valid {
		request.Header.Set("X-Client-Language", locale)
	}
	requestID, err := nonce()
	if err != nil {
		return 0, nil, nil, err
	}
	request.Header.Set("X-Request-Id", seededUUID("zcode-request\x00"+requestID))
	request.Header.Set("X-Device-Mid", accountDevice(account))
	response, err := send(ctx, owner.options.HTTPFactory, proxy.URL, request)
	if err != nil {
		return 0, nil, nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return response.StatusCode, nil, response.Header, &sdk.Error{Code: "upstream", Message: "ZCode control response is incomplete or too large"}
	}
	return response.StatusCode, data, response.Header, nil
}

func (owner *module) principal(ctx context.Context, stream *sdk.Stream, account Credentials, lane string) (string, string, error) {
	expiry := account.SessionExpiresAt
	kindAllowed := account.Kind == "individual" || account.Kind == "team"
	if lane == "" && account.Kind == "start" {
		kindAllowed, expiry = true, account.ExpiresAt
	}
	if !kindAllowed || !safeText(account.SessionToken) || expiry <= owner.options.Now().Add(30*time.Second).UnixMilli() {
		return "", "", &sdk.Error{Code: "credentials_required", Message: "Native controls require the selected account's unexpired ZCode session", Outcome: "not-started"}
	}
	if lane == "highspeed" && (!safeText(account.BusinessToken) || account.BusinessExpiresAt <= owner.options.Now().Add(30*time.Second).UnixMilli()) {
		return "", "", &sdk.Error{Code: "credentials_required", Message: "Highspeed requires the selected account's unexpired business credential", Outcome: "not-started"}
	}
	status, body, err := owner.control(ctx, stream, account, "", http.MethodGet, "/api/v1/oauth/profile", nil)
	if err != nil || status < 200 || status >= 300 {
		return "", "", &sdk.Error{Code: "permission_denied", Message: "ZCode profile identity could not be verified", Outcome: "not-started"}
	}
	root, err := protocol.ReadObject(body)
	code, valid := finiteNumber(root["code"])
	data, _ := root["data"].(map[string]any)
	user, _ := data["user"].(map[string]any)
	identity, _ := user["user_id"].(string)
	if err != nil || root["success"] == false || !valid || code != 0 || identity == "" || identity != account.UserID {
		return "", "", &sdk.Error{Code: "permission_denied", Message: "ZCode profile does not match the selected account", Outcome: "not-started"}
	}
	fingerprint := sha256.Sum256([]byte(account.SessionToken))
	return controller.Principal(owner.zcodeOrigin(), identity), hex.EncodeToString(fingerprint[:]), nil
}

func controlObject(status int, body []byte, envelope bool) (map[string]any, error) {
	if status < 200 || status >= 300 {
		return nil, &sdk.Error{Code: "upstream", Message: "ZCode control request was refused", Outcome: "uncertain"}
	}
	root, err := protocol.ReadObject(body)
	if err != nil {
		return nil, err
	}
	if root["success"] == false || root["error"] != nil {
		return nil, &sdk.Error{Code: "upstream", Message: "ZCode control response reports failure", Outcome: "uncertain"}
	}
	if value, exists := root["code"]; exists {
		code, valid := finiteNumber(value)
		if !valid || code != 0 {
			return nil, &sdk.Error{Code: "upstream", Message: "ZCode control request did not confirm success", Outcome: "uncertain"}
		}
		data, valid := root["data"].(map[string]any)
		if !valid {
			return nil, protocol.ErrInvalid
		}
		return data, nil
	}
	if envelope {
		return nil, protocol.ErrInvalid
	}
	return root, nil
}
