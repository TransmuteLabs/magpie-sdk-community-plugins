package devin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

func quotaNumber(value float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(value, 'f', 2, 64), "0"), ".")
}

const ProviderID = "devin"

type HTTPFactory func(string) (http.RoundTripper, func(), error)

type ModuleOptions struct {
	Server      string
	Now         func() time.Time
	HTTPFactory HTTPFactory
}

type providerOwner struct{ options ModuleOptions }

type providerState struct {
	server           string
	instance         string
	bridges          *bridgeStore
	agentAssignments bool
	cacheTasks       bool
	richQuota        bool
	modelSelections  modelSelectionIndex
	cachePolicy      cacheKeepalivePolicy
	cacheIndex       *cacheConversationIndex
	cacheTemplates   *cacheTemplateStore
	geminiThoughts   *geminiThoughtStore
	callbacks        *browserCallbacks
}

func decodeParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if outbound.DecodeParameters(raw, target) != nil {
		return &sdk.Error{Code: "invalid_params", Message: "Invalid Devin module parameters"}
	}
	return nil
}

func NewModule(options ModuleOptions) (sdk.Module, error) {
	base, err := NewInbound()
	if err != nil {
		return sdk.Module{}, err
	}
	var manifest sdk.Manifest
	if err := json.Unmarshal(declaration, &manifest); err != nil {
		return sdk.Module{}, err
	}
	manifest.Entry = base.Manifest.Entry
	if options.Server == "" {
		options.Server = outbound.DefaultServer
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.HTTPFactory == nil {
		options.HTTPFactory = defaultHTTPFactory
	}
	owner := &providerOwner{options: options}
	module := sdk.Module{Manifest: manifest, Start: owner.start, Inbounds: base.Inbounds,
		Handlers: map[string]sdk.Handler{
			sdk.MethodAuthStart: owner.authStart, sdk.MethodAuthFinish: owner.authFinish,
			sdk.MethodAuthRefresh: owner.authRefresh, sdk.MethodAuthLogout: owner.authLogout,
			sdk.MethodProviderModels: owner.models, sdk.MethodProviderUsage: owner.usage,
			sdk.MethodProviderExecute: owner.execute,
		}}
	module.Inbounds[0] = &pairedAdapter{owner: owner}
	return module, module.Validate()
}

func (owner *providerOwner) start(_ context.Context, configuration sdk.ModuleContext) (sdk.ModuleReady, func(), error) {
	var config cacheKeepaliveConfig
	if err := decodeParams(configuration.Config.Values, &config); err != nil {
		return sdk.ModuleReady{}, nil, err
	}
	policy, err := config.policy()
	if err != nil {
		return sdk.ModuleReady{}, nil, &sdk.Error{Code: "invalid_params", Message: "Invalid Devin cache keepalive configuration"}
	}
	server := owner.options.Server
	if config.Server != "" {
		server = config.Server
	}
	server = strings.TrimRight(server, "/")
	if _, err := outbound.New(server, nil, owner.options.Now); err != nil {
		return sdk.ModuleReady{}, nil, &sdk.Error{Code: "invalid_params", Message: "Invalid Devin API server origin"}
	}
	state := &providerState{server: server, instance: configuration.Instance, bridges: newBridgeStore(),
		cachePolicy: policy, cacheIndex: &cacheConversationIndex{}, cacheTemplates: &cacheTemplateStore{},
		geminiThoughts: &geminiThoughtStore{}, callbacks: newBrowserCallbacks()}
	return sdk.ModuleReady{State: state}, func() {
		state.callbacks.close()
		state.cacheTemplates.close()
		state.bridges.close()
		state.geminiThoughts.close()
	}, nil
}

func currentProviderState(ctx context.Context) (*providerState, error) {
	value, found := sdk.ModuleContextFrom(ctx)
	state, valid := value.State.(*providerState)
	if !found || !valid || state == nil {
		return nil, &sdk.Error{Code: "not_ready", Message: "Devin module is not configured"}
	}
	return state, nil
}

func scopeProvider(call sdk.Call, account bool) error {
	if call.Scope.Provider != ProviderID || account && (call.Scope.Account == "" || call.Scope.Generation == 0) ||
		!account && ((call.Scope.Account == "") != (call.Scope.Generation == 0)) {
		return &sdk.Error{Code: "permission_denied", Message: "An explicit matching Devin provider/account scope is required", Outcome: "not-started"}
	}
	return nil
}

func selectedCredentials(ctx context.Context, call sdk.Call) (outbound.Credentials, *providerState, error) {
	account, state, err := selectedAccount(ctx, call)
	return account.Credentials, state, err
}

func selectedAccount(ctx context.Context, call sdk.Call) (accountCredentials, *providerState, error) {
	if err := scopeProvider(call, true); err != nil {
		return accountCredentials{}, nil, err
	}
	state, err := currentProviderState(ctx)
	if err != nil {
		return accountCredentials{}, nil, err
	}
	var account accountCredentials
	if decodeParams(call.Auth, &account) != nil || account.Validate() != nil || !account.matches(state.server, call.Scope.Account) {
		return accountCredentials{}, nil, &sdk.Error{Code: "permission_denied", Message: "The selected Devin account generation does not match its credentials", Outcome: "not-started"}
	}
	return account, state, nil
}

type sdkTransport struct {
	stream  *sdk.Stream
	factory HTTPFactory
}
type sdkBody struct {
	io.ReadCloser
	dispose func()
}

func (body *sdkBody) Close() error { err := body.ReadCloser.Close(); body.dispose(); return err }
func (transport sdkTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	var resolution sdk.NetworkProxyResult
	if err := transport.stream.HostCall(request.Context(), sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: request.URL.String()}, &resolution); err != nil {
		return nil, err
	}
	client, dispose, err := transport.factory(resolution.URL)
	if err != nil {
		return nil, err
	}
	if dispose == nil {
		dispose = func() {}
	}
	response, err := client.RoundTrip(request)
	if err != nil {
		dispose()
		return nil, err
	}
	response.Body = &sdkBody{ReadCloser: response.Body, dispose: dispose}
	return response, nil
}

func defaultHTTPFactory(proxy string) (http.RoundTripper, func(), error) {
	transport := &http.Transport{DialContext: (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 30 * time.Second}
	if proxy != "" {
		address, err := url.Parse(proxy)
		if err != nil || address.Host == "" || address.Scheme != "http" && address.Scheme != "https" && address.Scheme != "socks5" && address.Scheme != "socks5h" {
			return nil, nil, errors.New("invalid host proxy resolution")
		}
		transport.Proxy = http.ProxyURL(address)
	}
	return transport, transport.CloseIdleConnections, nil
}

func (owner *providerOwner) client(state *providerState, stream *sdk.Stream) (*outbound.Client, error) {
	return outbound.New(state.server, &http.Client{Transport: sdkTransport{stream: stream, factory: owner.options.HTTPFactory}}, owner.options.Now)
}

func moduleFailure(err error, outcome string) error {
	var sdkFailure *sdk.Error
	if errors.As(err, &sdkFailure) {
		if outcome == "uncertain" && (sdkFailure.Outcome == "" || sdkFailure.Outcome == "not-started") {
			copy := *sdkFailure
			copy.Outcome = outcome
			return &copy
		}
		return sdkFailure
	}
	var failure *outbound.Error
	if errors.As(err, &failure) {
		code := failure.Code
		if code == "" {
			code = "upstream"
		}
		if failure.Outcome != "" {
			outcome = failure.Outcome
		}
		message := failure.Error()
		if code == "unauthenticated" {
			message = "Devin sign-in was refused; sign in again"
		} else if failure.Status == http.StatusUnauthorized {
			message = "Devin upstream returned HTTP 401"
		}
		return &sdk.Error{Code: code, Message: message, Outcome: outcome}
	}
	return &sdk.Error{Code: "upstream", Message: "Devin native operation failed", Outcome: outcome}
}

func (owner *providerOwner) models(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	credentials, state, err := selectedCredentials(ctx, call)
	if err != nil {
		return nil, err
	}
	client, err := owner.client(state, stream)
	if err != nil {
		return nil, moduleFailure(err, "not-started")
	}
	models, err := client.Models(ctx, credentials)
	if err != nil {
		return nil, moduleFailure(err, "")
	}
	projected := codec.ProjectNativeModels(models)
	state.modelSelections.remember(projected)
	result := sdk.ModelsResult{Models: []sdk.Model{}}
	for _, model := range projected {
		input := []string{"text"}
		if model.Images {
			input = append(input, "image")
		}
		var price *sdk.ModelPrice
		if model.Price != nil {
			price = &sdk.ModelPrice{Currency: "USD", Input: model.Price.Input, Output: model.Price.Output, CacheRead: model.Price.CacheRead}
		}
		result.Models = append(result.Models, sdk.Model{ID: model.UID, Name: model.Name, Protocols: []protocolext.Protocol{protocolext.Chat, protocolext.Anthropic},
			ContextWindow: int64(model.Context), MaxOutput: int64(model.MaxOutput), Tools: model.Tools, Vision: model.Images,
			Reasoning: model.Thinking, InputModalities: input, OutputModalities: []string{"text"}, Free: model.Free, Price: price,
			Efforts: model.Efforts, DefaultEffort: model.DefaultEffort})
	}
	return result, nil
}

func (owner *providerOwner) usage(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.UsageRequest
	if err := decodeParams(call.Params, &request); err != nil {
		return nil, err
	}
	credentials, state, err := selectedCredentials(ctx, call)
	if err != nil {
		return nil, err
	}
	client, err := owner.client(state, stream)
	if err != nil {
		return nil, moduleFailure(err, "")
	}
	// Status is the account authority. An unavailable catalog must not hide a
	// rejected sign-in behind the separate free-model quota-scope barrier.
	status, err := client.Status(ctx, credentials, nil)
	if err != nil {
		return nil, moduleFailure(err, "")
	}
	models, err := client.Models(ctx, credentials)
	if err != nil {
		var failure *outbound.Error
		if errors.As(err, &failure) && (failure.Code == "unauthenticated" || failure.Code == "auth_refresh_required" ||
			failure.Status >= 400 && failure.Status < 500) {
			return nil, moduleFailure(err, "")
		}
		return sdk.UsageResult{Revision: request.Revision, ReadDeferred: true, Error: "Devin model-specific quota scope is unreadable", Windows: []sdk.UsageWindow{}}, nil
	}
	var free []string
	projected := codec.ProjectNativeModels(models)
	state.modelSelections.remember(projected)
	for _, model := range projected {
		if model.Free {
			free = append(free, model.UID)
		}
	}
	result := sdk.UsageResult{Revision: request.Revision, Windows: []sdk.UsageWindow{}}
	for _, window := range status.Windows {
		value := sdk.UsageWindow{Name: window.Name, Used: window.Used, Limit: 100,
			ResetsAt: window.ResetsAt * 1000, NotModels: append([]string(nil), free...)}
		if state.richQuota {
			value.Span = window.Span
		}
		result.Windows = append(result.Windows, value)
	}
	if status.ACULimit > 0 || state.richQuota && (status.ACUUsedPresent || status.ACULimitPresent) {
		window := sdk.UsageWindow{Name: "ACUs",
			Limit: 100, Aside: true, Unknown: !status.ACUUsedPresent}
		if status.ACULimit > 0 {
			window.Used = min(100, status.ACUUsed/status.ACULimit*100)
		} else {
			window.Unknown = true
		}
		if state.richQuota {
			window.Limit, window.Unit = status.ACULimit, "ACUs"
			used := "unknown"
			if status.ACUUsedPresent {
				window.Amount = &status.ACUUsed
				used = quotaNumber(status.ACUUsed)
			}
			limit := "unknown"
			if status.ACULimitPresent {
				limit = quotaNumber(status.ACULimit)
			}
			window.Display = used + " / " + limit + " ACUs"
		}
		if !status.Until.IsZero() && status.Until.UnixMilli() > 0 {
			window.ResetsAt = status.Until.UnixMilli()
		}
		result.Windows = append(result.Windows, window)
	} else if state.richQuota && !status.Until.IsZero() && status.Until.UnixMilli() > 0 {
		result.Windows = append(result.Windows, sdk.UsageWindow{Name: "Plan ends", Aside: true, Unknown: true,
			Display: status.Plan, ResetsAt: status.Until.UnixMilli()})
	}
	if state.richQuota && status.BalancePresent {
		balance := float64(status.BalanceMicros)
		result.Windows = append(result.Windows, sdk.UsageWindow{Name: "Extra usage balance", Balance: &balance,
			Unit: "micros", Display: quotaNumber(balance) + " micros", Aside: true, Unknown: true})
	}
	return result, nil
}
