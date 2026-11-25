package commandcode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"runtime"
	"sync"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

const Version = "0.1.6"
const ProviderID = "commandcode-native"
const ModuleID = "protocol-commandcode"
const cliVersion = "1.93.3"

type Config struct {
	APIURL           string `json:"apiURL,omitempty"`
	StudioURL        string `json:"studioURL,omitempty"`
	WorkingDirectory string `json:"workingDirectory,omitempty"`
	ZDR              bool   `json:"zdr,omitempty"`
}

type credentials struct {
	Key    string `json:"apiKey"`
	UserID string `json:"userId,omitempty"`
	Name   string `json:"name,omitempty"`
}

type controller struct {
	config  Config
	ctx     context.Context
	session string
	mu      sync.Mutex
	flows   map[string]*authFlow
	closed  bool
}

func Manifest() sdk.Manifest {
	entry := "lib/commandcode.so"
	if runtime.GOOS == "darwin" {
		entry = "lib/commandcode.dylib"
	}
	if runtime.GOOS == "windows" {
		entry = "lib/commandcode.dll"
	}
	return sdk.Manifest{
		Schema: 1, ID: ModuleID, Version: Version, Runtime: "native", Entry: entry,
		Contract: sdk.Contract{Version: sdk.APIVersion{Major: 1, Minor: 12}, Capabilities: []sdk.Capability{
			sdk.OutboundExecutor, sdk.ModelDiscovery, sdk.UsageReporter, sdk.AccountAuth, sdk.ModuleRPC, sdk.AgentWiring, sdk.RequestPolicy, sdk.ProviderSignIn,
		}},
		Compatibility: &sdk.Compatibility{HostAPI: "^1.12", NativeABI: 1},
		Providers: []sdk.ProviderDescriptor{{ID: ProviderID, Name: "Command Code", Protocols: []protocolext.Protocol{"chat", "responses", "anthropic"},
			Operations: []sdk.ProviderOperation{sdk.OperationSystemOne},
			Options: []sdk.ProviderOption{{
				Key: "zdr", Label: "Zero Data Retention",
				Description: "Sends x-cmd-zdr: 1, as the CLI's CMD_ZDR=1 does: requests go only to upstreams that keep no prompts or replies. They usually cost more, and a model with no such upstream fails (422 cmd_zdr_no_providers).",
				Confirm:     "ZDR requests usually cost more, and a model with no zero-data-retention upstream fails instead of falling back.",
			}}}},
		Agents: []sdk.AgentDescriptor{agentDescriptor()},
		Actions: []sdk.ActionDescriptor{
			{ID: "status", Effect: "read", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)},
			{ID: "systemone", Effect: "read", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		Window:   &sdk.WindowDescriptor{Metadata: "package.json"},
		Bindings: []sdk.BindingDescriptor{{ID: "account.status", From: "window:panel:0", Action: "status", Provider: ProviderID}},
		AuthBindings: []sdk.WindowAuthBindingDescriptor{{ID: "provider.signin", From: sdk.ProviderSignInPart, Provider: ProviderID, Reauthorize: true,
			Methods: []sdk.WindowAuthMethod{
				{ID: "api-key", Mode: "poll", Label: "API key", Fields: []sdk.AuthInputField{{Key: "apiKey", Label: "API key", Type: "secret"}}},
				{ID: "browser", Mode: "poll", Label: "Browser sign-in"},
				{ID: "cli-import", Mode: "poll", Label: "Import CLI sign-in"},
			}}},
	}
}

func Module() sdk.Module {
	return sdk.Module{
		Manifest: Manifest(),
		Start:    start,
		Handlers: map[string]sdk.Handler{
			sdk.MethodProviderExecute: execute,
			sdk.MethodProviderModels:  models,
			sdk.MethodProviderUsage:   usage,
			sdk.MethodAuthStart:       authStart,
			sdk.MethodAuthFinish:      authFinish,
			sdk.MethodAuthRefresh:     authRefresh,
			sdk.MethodAuthLogout:      authLogout,
			sdk.MethodAgentPlan:       agentPlan,
			sdk.MethodAgentInspect:    agentInspect,
		},
		Actions: map[string]sdk.Handler{
			"status": status,
			"systemone": func(context.Context, sdk.Call, *sdk.Stream) (any, error) {
				return nil, &sdk.Error{Code: "unnegotiated_capability", Message: "SystemOne requires the selected-account provider operation; a module action cannot bypass decision routing", Outcome: "not-started"}
			},
		},
	}
}

func start(ctx context.Context, value sdk.ModuleContext) (sdk.ModuleReady, func(), error) {
	config := Config{APIURL: "https://api.commandcode.ai", StudioURL: "https://commandcode.ai"}
	if err := json.Unmarshal(value.Config.Values, &config); err != nil {
		return sdk.ModuleReady{}, nil, invalid("invalid CommandCode configuration")
	}
	for _, raw := range []string{config.APIURL, config.StudioURL} {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
			parsed.Path != "" && parsed.Path != "/" || parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1")) {
			return sdk.ModuleReady{}, nil, invalid("CommandCode endpoints require HTTPS or an explicit loopback fixture")
		}
	}
	session, err := systemOneSession()
	if err != nil {
		return sdk.ModuleReady{}, nil, err
	}
	lifetime, cancel := context.WithCancel(ctx)
	controller := &controller{config: config, ctx: lifetime, session: session, flows: make(map[string]*authFlow)}
	dispose := func() {
		cancel()
		controller.mu.Lock()
		controller.closed = true
		flows := controller.flows
		controller.flows = make(map[string]*authFlow)
		controller.mu.Unlock()
		for _, flow := range flows {
			flow.close()
		}
	}
	return sdk.ModuleReady{State: controller}, dispose, nil
}

func owner(ctx context.Context) (*controller, error) {
	value, found := sdk.ModuleContextFrom(ctx)
	controller, ok := value.State.(*controller)
	if !found || !ok || controller.ctx.Err() != nil {
		return nil, &sdk.Error{Code: "unavailable", Message: "CommandCode component is not ready", Outcome: "not-started"}
	}
	return controller, nil
}

func account(call sdk.Call) (credentials, error) {
	var auth credentials
	if len(call.Auth) == 0 || json.Unmarshal(call.Auth, &auth) != nil || auth.Key == "" || len(auth.Key) > 8192 {
		return credentials{}, &sdk.Error{Code: "unauthenticated", Message: "Select a signed-in CommandCode account", Outcome: "not-started"}
	}
	if err := validKey(auth.Key); err != nil {
		return credentials{}, err
	}
	return auth, nil
}

func invalid(message string) error {
	return &sdk.Error{Code: "invalid_params", Message: message, Outcome: "not-started"}
}

func (controller *controller) subscription(ctx context.Context, stream *sdk.Stream, key string, policy ...bool) (Subscription, error) {
	response, err := controller.read(ctx, stream, "/alpha/billing/subscriptions", key, policy...)
	if err == nil {
		var sub Subscription
		sub, err = ParseSubscription(response)
		if err == nil {
			return sub, nil
		}
	}
	return Subscription{}, &sdk.Error{Code: "unavailable", Message: "CommandCode subscription is unknown", Outcome: "not-started"}
}

func (controller *controller) credits(ctx context.Context, stream *sdk.Stream, key string, sub Subscription) (Credits, error) {
	response, err := controller.read(ctx, stream, "/alpha/billing/credits", key)
	if err != nil {
		return Credits{}, err
	}
	return ParseCredits(response, sub)
}

func models(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	controller, err := owner(ctx)
	if err != nil {
		return nil, err
	}
	auth, err := account(call)
	if err != nil {
		return nil, err
	}
	sub, err := controller.subscription(ctx, stream, auth.Key)
	if err != nil {
		return nil, err
	}
	credits, _ := controller.credits(ctx, stream, auth.Key, sub)
	extra := credits.ExtraUSD != nil && *credits.ExtraUSD > 0
	key := auth.Key
	goPlan := sub.Plan == "Go"
	if goPlan {
		key = ""
	}
	body, err := controller.read(ctx, stream, "/provider/v1/models", key)
	if err != nil {
		return nil, err
	}
	items, err := ParseModels(body, sub, extra, goPlan)
	if err != nil {
		return nil, &sdk.Error{Code: "upstream_error", Message: err.Error()}
	}
	return sdk.ModelsResult{Models: items}, nil
}

func usage(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.UsageRequest
	if decode(call.Params, &request) != nil {
		return nil, invalid("invalid CommandCode usage request")
	}
	if request.Revision != "" {
		if _, err := sdk.ParseUsageRevision(request.Revision); err != nil {
			return nil, invalid("CommandCode usage requires the canonical host revision")
		}
	}
	if len(request.Credentials) > 0 && string(request.Credentials) != "null" {
		return nil, invalid("CommandCode usage credentials must come from the selected SDK account")
	}
	controller, err := owner(ctx)
	if err != nil {
		return nil, err
	}
	auth, err := account(call)
	if err != nil {
		return nil, err
	}
	sub, subErr := controller.subscription(ctx, stream, auth.Key)
	value, err := controller.credits(ctx, stream, auth.Key, sub)
	if err != nil {
		return sdk.UsageResult{Revision: request.Revision, Error: "CommandCode allowance is unknown"}, nil
	}
	result := sdk.UsageResult{Revision: request.Revision, Windows: value.Windows}
	balanceOnly := sub.Monthly == 0 && len(value.Windows) == 0
	separateBalance := sub.Unit == "credits" && value.ExtraUSD != nil && *value.ExtraUSD > 0
	if subErr != nil {
		result.Error = "CommandCode subscription is unknown; only independently known windows are shown"
	} else if value.ExtraUSD != nil && (balanceOnly || separateBalance) {
		result.Windows = append(result.Windows, sdk.UsageWindow{Name: "Balance", Balance: value.ExtraUSD, Unit: "USD", Aside: true})
	}
	return result, nil
}

func status(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	controller, err := owner(ctx)
	if err != nil {
		return nil, err
	}
	auth, err := account(call)
	if err != nil {
		return nil, err
	}
	sub, subErr := controller.subscription(ctx, stream, auth.Key)
	credits, creditsErr := controller.credits(ctx, stream, auth.Key, sub)
	return map[string]any{"subscription": sub, "subscriptionKnown": subErr == nil, "credits": credits, "creditsKnown": creditsErr == nil,
		"zdr": controller.config.ZDR, "systemone": map[string]any{"model": Jev, "negotiated": true,
			"operation": sdk.OperationSystemOne, "minimumSDK": "1.7"}}, nil
}

func readAllBounded(response *http.Response, limit int64) ([]byte, error) {
	defer response.Body.Close()
	body, err := bounded(response.Body, limit)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &sdk.Error{Code: "upstream_error", Message: "CommandCode account endpoint returned " + response.Status}
	}
	return body, nil
}

func boundedBodyError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &sdk.Error{Code: "upstream_error", Message: "CommandCode returned an unreadable or oversized body"}
}
