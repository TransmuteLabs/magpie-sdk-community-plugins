package zcode

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type projectChoice struct {
	Organization     string
	Project          string
	OrganizationName string
	ProjectName      string
	Team             bool
}

func paidUnavailable() error {
	return &sdk.Error{Code: "entitlement_unavailable", Message: "No available selected Coding Plan entitlement was confirmed", Outcome: "not-started"}
}

func (owner *module) businessRequest(ctx context.Context, stream *sdk.Stream, account Credentials, method, path string, headers http.Header, body any) (vendorResponse, error) {
	if headers == nil {
		headers = http.Header{}
	}
	if headers.Get("Authorization") == "" {
		headers.Set("Authorization", account.BusinessToken)
	}
	return owner.vendorRequest(ctx, stream, method, owner.businessOrigin(account)+path, headers, body)
}

func (owner *module) discoverPaidSelection(ctx context.Context, stream *sdk.Stream, account Credentials, selection authSelection) (Credentials, error) {
	response, err := owner.businessRequest(ctx, stream, account, http.MethodGet, "/api/biz/customer/getCustomerInfo", nil, nil)
	if err != nil {
		return account, err
	}
	data, err := responseData(response)
	organizations, valid := data["organizations"].([]any)
	if err != nil || !valid || len(organizations) > 1024 {
		return account, protocol.ErrInvalid
	}
	choices := []projectChoice{}
	for _, item := range organizations {
		organization, valid := item.(map[string]any)
		id, _ := organization["organizationId"].(string)
		name, _ := organization["organizationName"].(string)
		projects, projectsValid := organization["projects"].([]any)
		if !valid || !safeText(id) || !projectsValid || len(projects) > 4096 {
			return account, protocol.ErrInvalid
		}
		for _, item := range projects {
			project, valid := item.(map[string]any)
			projectID, _ := project["projectId"].(string)
			projectName, _ := project["projectName"].(string)
			team := project["projectType"] == "2"
			if number, valid := finiteNumber(project["projectType"]); valid {
				team = number == 2
			}
			if !valid || !safeText(projectID) {
				return account, protocol.ErrInvalid
			}
			if selection.Kind == "team" && !team || selection.Kind != "team" && team ||
				selection.Organization != "" && selection.Organization != id ||
				selection.Project != "" && selection.Project != projectID {
				continue
			}
			choices = append(choices, projectChoice{Organization: id, Project: projectID,
				OrganizationName: name, ProjectName: projectName, Team: team})
		}
	}
	if len(choices) == 0 {
		if selection.Kind == "auto" {
			selection.Kind = "team"
			return owner.discoverPaidSelection(ctx, stream, account, selection)
		}
		return account, paidUnavailable()
	}
	chosen := choices[0]
	if selection.Kind == "team" && len(choices) != 1 {
		return account, &sdk.Error{Code: "selection_required", Message: "Several Team projects exist; specify organization and project in auth.start Input", Outcome: "not-started"}
	}
	if selection.Kind != "team" && selection.Organization == "" && selection.Project == "" {
		for _, choice := range choices {
			if strings.Contains(choice.OrganizationName, "默认机构") && strings.Contains(choice.ProjectName, "默认项目") {
				chosen = choice
				break
			}
		}
	}
	account.Kind = "individual"
	if selection.Kind == "team" {
		account.Kind, account.Organization, account.Project = "team", chosen.Organization, chosen.Project
		entitlement, err := owner.teamEntitlement(ctx, stream, account)
		if err != nil {
			return account, err
		}
		product, _ := entitlement["productId"].(string)
		available, err := teamAvailable(entitlement, selection.Product)
		if err != nil {
			return account, err
		}
		if !available {
			return account, paidUnavailable()
		}
		account.Product = product
	}
	headers := http.Header{}
	if account.Kind == "team" {
		headers.Set("Bigmodel-Organization", chosen.Organization)
		headers.Set("Bigmodel-Project", chosen.Project)
	} else if account.Site == "zai" {
		headers.Set("Authorization", "Bearer "+account.BusinessToken)
	}
	path := "/api/biz/v1/organization/" + url.PathEscape(chosen.Organization) + "/projects/" +
		url.PathEscape(chosen.Project) + "/api_keys"
	response, err = owner.businessRequest(ctx, stream, account, http.MethodGet, path, headers, nil)
	if err != nil {
		return account, err
	}
	value, err := responseValue(response)
	keys, valid := value.([]any)
	if err != nil || !valid || len(keys) > 4096 {
		return account, protocol.ErrInvalid
	}
	name := "zcode-api-key"
	if account.Kind == "team" {
		name = "zcode-team-api-key"
	}
	keyID := ""
	for _, item := range keys {
		key, valid := item.(map[string]any)
		if !valid {
			return account, protocol.ErrInvalid
		}
		id, _ := key["apiKey"].(string)
		if selection.APIKey != "" && id != selection.APIKey {
			continue
		}
		if key["name"] != name {
			continue
		}
		if account.Kind == "team" {
			keyType, valid := finiteNumber(key["keyType"])
			if !valid || keyType != 2 {
				continue
			}
		}
		if !safeText(id) {
			return account, protocol.ErrInvalid
		}
		if keyID != "" && keyID != id {
			return account, &sdk.Error{Code: "selection_required", Message: "Several coding-plan keys have the official name; import an explicit location instead of guessing", Outcome: "not-started"}
		}
		keyID = id
	}
	if keyID == "" {
		if selection.APIKey != "" {
			return account, unsupported("The explicitly selected coding-plan key no longer exists; renewal cannot create another identity")
		}
		body := map[string]any{"name": name, "usageScene": 1}
		if account.Kind == "team" {
			body["keyType"] = 2
		}
		response, err = owner.businessRequest(ctx, stream, account, http.MethodPost, path, headers, body)
		if err != nil {
			return account, uncertain("Coding-plan key creation has an unconfirmed result; it will not be repeated")
		}
		key, err := responseData(response)
		keyID, _ = key["apiKey"].(string)
		if err != nil || !safeText(keyID) || key["name"] != name {
			return account, uncertain("Coding-plan key creation did not return its exact selected identity")
		}
		if account.Kind == "team" {
			keyType, valid := finiteNumber(key["keyType"])
			if !valid || keyType != 2 {
				return account, uncertain("Team key creation returned another key type")
			}
		}
	}
	account.Location = &ProjectLocation{OrganizationID: chosen.Organization, ProjectID: chosen.Project, APIKeyID: keyID}
	return account, nil
}

func (owner *module) teamEntitlement(ctx context.Context, stream *sdk.Stream, account Credentials) (map[string]any, error) {
	auth := account.BusinessToken
	if account.Site == "zai" {
		auth = "Bearer " + auth
	}
	response, err := owner.businessRequest(ctx, stream, account, http.MethodGet,
		"/api/biz/team/subscribe/product/querySubscribeDetail",
		http.Header{"Authorization": {auth}, "Bigmodel-Organization": {account.Organization}, "Bigmodel-Project": {account.Project}}, nil)
	if err != nil {
		return nil, err
	}
	value, err := codingEntitlementValue(response)
	data, valid := value.(map[string]any)
	if err != nil || !valid {
		return nil, protocol.ErrInvalid
	}
	return data, nil
}

func codingEntitlementValue(response vendorResponse) (any, error) {
	if response.Status < 200 || response.Status >= 300 {
		return nil, protocol.ErrInvalid
	}
	if value, present := response.Data["success"]; present {
		success, valid := value.(bool)
		if !valid || !success {
			return nil, protocol.ErrInvalid
		}
	}
	if value, present := response.Data["code"]; present {
		code, valid := finiteNumber(value)
		if !valid || code != 0 && code != 200 {
			return nil, protocol.ErrInvalid
		}
	}
	value, present := response.Data["data"]
	if !present || value == nil {
		return nil, protocol.ErrInvalid
	}
	return value, nil
}

func teamAvailable(data map[string]any, selectedProduct string) (bool, error) {
	subscribed, valid := data["hasSubscription"].(bool)
	if !valid {
		return false, protocol.ErrInvalid
	}
	for _, name := range []string{"status", "memberGrantStatus", "productId", "productName", "subscribeEndTime", "subscribePeriod"} {
		if value := data[name]; value != nil {
			if _, valid := value.(string); !valid {
				return false, protocol.ErrInvalid
			}
		}
	}
	if !subscribed {
		return false, nil
	}
	status, validStatus := data["status"].(string)
	grant, validGrant := data["memberGrantStatus"].(string)
	if status == "EXPIRED" || status == "EFFECTIVE" && grant == "UNASSIGNED" {
		return false, nil
	}
	if !validStatus || !validGrant || status != "EFFECTIVE" || grant != "VALID" {
		return false, protocol.ErrInvalid
	}
	product, validProduct := data["productId"].(string)
	if !validProduct || !safeText(product) {
		return false, protocol.ErrInvalid
	}
	if selectedProduct != "" && product != selectedProduct {
		return false, unsupported("The selected Team product changed during confirmation")
	}
	return true, nil
}

func (owner *module) paidEntitlement(ctx context.Context, stream *sdk.Stream, account Credentials) (bool, error) {
	if account.Kind == "team" {
		data, err := owner.teamEntitlement(ctx, stream, account)
		if err != nil {
			return false, err
		}
		return teamAvailable(data, account.Product)
	}
	response, err := owner.vendorRequest(ctx, stream, http.MethodGet, owner.businessOrigin(account)+"/api/biz/subscription/list",
		http.Header{"Authorization": {account.ProjectToken}}, nil)
	if err != nil {
		return false, err
	}
	value, err := codingEntitlementValue(response)
	list, valid := value.([]any)
	if err != nil || !valid || len(list) > 4096 {
		return false, protocol.ErrInvalid
	}
	malformed := false
	for _, item := range list {
		entry, valid := item.(map[string]any)
		if !valid {
			continue
		}
		product, _ := entry["productId"].(string)
		name, _ := entry["productName"].(string)
		if !strings.Contains(strings.ToLower(product+" "+name), "coding") {
			continue
		}
		status, validStatus := entry["status"].(string)
		current, validCurrent := entry["inCurrentPeriod"].(bool)
		validEntry := validStatus && validCurrent
		for _, name := range []string{"productId", "productName", "nextRenewTime", "valid", "billingCycle"} {
			if value, present := entry[name]; present {
				_, valid := value.(string)
				validEntry = validEntry && valid
			}
		}
		if value, present := entry["autoRenew"]; present {
			_, boolean := value.(bool)
			_, number := finiteNumber(value)
			validEntry = validEntry && (boolean || number)
		}
		if !validEntry {
			malformed = true
			continue
		}
		if status == "VALID" && current {
			return true, nil
		}
	}
	if malformed {
		return false, protocol.ErrInvalid
	}
	return false, nil
}
