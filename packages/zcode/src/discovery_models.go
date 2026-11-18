package zcode

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func (owner *module) discoveredModels(ctx context.Context, stream *sdk.Stream, account Credentials) ([]sdk.Model, error) {
	path := "/api/v1/client/configs?app_version=" + url.QueryEscape(ClientVersion) + "&platform=" + url.QueryEscape(platform())
	response, err := owner.vendorRequest(ctx, stream, http.MethodGet, owner.zcodeOrigin()+path, nil, nil)
	if err != nil {
		return nil, err
	}
	data, err := responseData(response)
	configs := mapObject(data["configs"])
	location, valid := configs["builtin_provider_config_json"].(string)
	if err != nil || !valid {
		return nil, protocol.ErrInvalid
	}
	address, err := url.Parse(location)
	if err != nil || address.User != nil || address.Fragment != "" || address.Host == "" ||
		address.Scheme != "https" && !(address.Scheme == "http" && net.ParseIP(address.Hostname()).IsLoopback()) {
		return nil, unsupported("Provider configuration names an untrusted or malformed public resource")
	}
	public, err := owner.vendorRequest(ctx, stream, http.MethodGet, address.String(), nil, nil)
	if err != nil || public.Status < 200 || public.Status >= 300 {
		return nil, &sdk.Error{Code: "upstream", Message: "Provider configuration could not be read"}
	}
	body, err := json.Marshal(public.Data)
	if err != nil {
		return nil, err
	}
	plan := "account:" + account.Site + "-" + account.Kind + "-coding-plan"
	endpoint := strings.TrimSuffix(owner.options.Endpoints.forAccount(account), "/v1/messages")
	if account.Kind == "start" {
		plan = "account:" + account.Site + "-start-plan"
	}
	models, err := configuredModels(body, plan, endpoint)
	if err != nil {
		return nil, err
	}
	if account.Kind == "start" {
		available, err := owner.startEntitlement(ctx, stream, account, false)
		if err != nil {
			return nil, err
		}
		filtered := make([]sdk.Model, 0, len(models))
		for _, model := range models {
			if available.Models[strings.ToLower(model.ID)] {
				model.Free = true
				filtered = append(filtered, model)
			}
		}
		filtered = appendGiftModels(filtered, models, available, false)
		return filtered, nil
	}
	available, err := owner.paidEntitlement(ctx, stream, account)
	if err != nil {
		return nil, err
	}
	if !available {
		if giftSession(account, owner.options.Now()) {
			if gift, err := owner.startEntitlement(ctx, stream, account, false); err == nil {
				free := appendGiftModels(nil, models, gift, false)
				return appendGiftModels(free, models, gift, true), nil
			}
		}
		return nil, unsupported("The selected paid entitlement is not currently available")
	}
	if giftSession(account, owner.options.Now()) {
		if gift, err := owner.startEntitlement(ctx, stream, account, false); err == nil {
			models = appendGiftModels(models, models, gift, true)
		}
	}
	return models, nil
}
