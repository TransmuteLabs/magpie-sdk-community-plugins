package zcode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
	codec "github.com/yetone/magpie/pkg/protocolext"
)

const ProviderID = "zcode"
const Version = "0.7.3"

type ModuleOptions struct {
	Endpoints   Endpoints
	Now         func() time.Time
	HTTPFactory HTTPFactory
}

type module struct {
	options       ModuleOptions
	observability bool
	richQuota     bool
}

func Models() []sdk.Model {
	return []sdk.Model{
		{ID: "GLM-5.3", Name: "GLM-5.3", Protocols: []codec.Protocol{codec.Messages}, ContextWindow: 1000000, MaxOutput: 128000, Tools: true, Reasoning: true, Efforts: []string{"low", "high", "max"}},
		{ID: "GLM-5.3-Flash", Name: "GLM-5.3-Flash", Protocols: []codec.Protocol{codec.Messages}, ContextWindow: 1000000, MaxOutput: 128000, Tools: true, Vision: true, Reasoning: true, Efforts: []string{"low", "high", "max"}},
		{ID: "GLM-5.2", Name: "GLM-5.2", Protocols: []codec.Protocol{codec.Messages}, ContextWindow: 1000000, MaxOutput: 128000, Tools: true, Reasoning: true, Efforts: []string{"none", "high", "max"}},
		{ID: "GLM-5-Turbo", Name: "GLM-5-Turbo", Protocols: []codec.Protocol{codec.Messages}, ContextWindow: 200000, MaxOutput: 64000, Tools: true, Reasoning: true, Efforts: []string{"none", "high"}},
	}
}

func Manifest() sdk.Manifest {
	entry := "lib/zcode.so"
	if runtime.GOOS == "darwin" {
		entry = "lib/zcode.dylib"
	} else if runtime.GOOS == "windows" {
		entry = "lib/zcode.dll"
	}
	return sdk.Manifest{
		Schema: sdk.ManifestSchema, ID: ProviderID, Version: Version, Runtime: "native", Entry: entry,
		Contract: sdk.Contract{
			Version:      sdk.APIVersion{Major: 1, Minor: 12},
			Capabilities: []sdk.Capability{sdk.AccountAuth, sdk.ModelDiscovery, sdk.OutboundExecutor, sdk.UsageReporter, sdk.OperationControl, sdk.ModuleRPC, sdk.ToolProvider, sdk.SessionMigration, sdk.RequestPolicy},
		},
		Compatibility: &sdk.Compatibility{HostAPI: "^1.12", NativeABI: 1, Platforms: []string{runtime.GOOS + "/" + runtime.GOARCH}},
		Providers:     []sdk.ProviderDescriptor{{ID: ProviderID, Name: "ZCode native", Protocols: []codec.Protocol{codec.Messages}, Models: append(Models(), aliasModels()...)}},
		Tools:         toolDescriptors(),
		Sessions: []sdk.SessionDescriptor{
			{ID: historyAdapterID, Name: "ZCode offline source-preserving history converter"},
			{ID: sqliteAdapterID, Name: "ZCode immutable SQLite snapshot converter"},
		},
		Actions: []sdk.ActionDescriptor{
			{ID: "execution.list", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), Effect: "read"},
			{ID: "execution.settle", InputSchema: json.RawMessage(`{"type":"object","properties":{"operationId":{"type":"string"},"reference":{"type":"object"}},"additionalProperties":false}`), Effect: "write"},
			{ID: "execution.reconcile", InputSchema: json.RawMessage(`{"type":"object","properties":{"operationId":{"type":"string"},"reference":{"type":"object"},"ticketId":{"type":"string"},"acknowledgeUncertainOutput":{"type":"boolean"}},"required":["reference"],"additionalProperties":false}`), Effect: "write"},
			{ID: "execution.abandon", InputSchema: json.RawMessage(`{"type":"object","properties":{"reference":{"type":"object"},"acknowledgment":{"type":"string"}},"required":["reference","acknowledgment"],"additionalProperties":false}`), Effect: "write"},
			{ID: "billing.status", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), Effect: "read"},
			{ID: "billing.claim", InputSchema: json.RawMessage(`{"type":"object","properties":{"planId":{"type":"string"}},"required":["planId"],"additionalProperties":false}`), Effect: "write"},
			{ID: "billing.offers", InputSchema: json.RawMessage(`{"type":"object","properties":{"locale":{"type":"string","enum":["zh-CN","en-US","ja-JP","ru-RU","ko-KR"]}},"additionalProperties":false}`), Effect: "read"},
			{ID: "billing.checkin", InputSchema: json.RawMessage(`{"type":"object","properties":{"locale":{"type":"string","enum":["zh-CN","en-US","ja-JP","ru-RU","ko-KR"]}},"additionalProperties":false}`), Effect: "write"},
			{ID: "reset.status", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), Effect: "read"},
			{ID: "reset.summary", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), Effect: "read"},
			{ID: "reset.reconcile", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), Effect: "write"},
			{ID: "reset.use", InputSchema: json.RawMessage(`{"type":"object","properties":{"window":{"type":"string","enum":["FIVE_HOUR","WEEK"]}},"required":["window"],"additionalProperties":false}`), Effect: "write"},
			{ID: "reset.resolve", InputSchema: json.RawMessage(`{"type":"object","properties":{"acknowledgment":{"type":"string"}},"required":["acknowledgment"],"additionalProperties":false}`), Effect: "write"},
			{ID: "reset.opportunity", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), Effect: "write"},
			{ID: "reset.opportunity.resolve", InputSchema: json.RawMessage(`{"type":"object","properties":{"acknowledgment":{"type":"string"}},"required":["acknowledgment"],"additionalProperties":false}`), Effect: "write"},
			{ID: "reset.ack", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), Effect: "write"},
		},
	}
}

func NewModule(options ModuleOptions) sdk.Module {
	return newModule(options, Manifest())
}

func newModule(options ModuleOptions, manifest sdk.Manifest) sdk.Module {
	if options.Endpoints == (Endpoints{}) {
		options.Endpoints = DefaultEndpoints()
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.HTTPFactory == nil {
		options.HTTPFactory = defaultHTTP
	}
	owner := &module{options: options, observability: manifest.Window != nil,
		richQuota: manifest.Window != nil}
	result := sdk.Module{
		Manifest: manifest,
		Start:    owner.start,
		Handlers: map[string]sdk.Handler{
			sdk.MethodProviderExecute: owner.execute,
			sdk.MethodProviderModels:  owner.models,
			sdk.MethodProviderUsage:   owner.usage,
			sdk.MethodToolCall:        owner.tool,
			sdk.MethodSessionConvert:  owner.sessionConvert,
			sdk.MethodOperationStatus: owner.operationStatus,
			sdk.MethodOperationCancel: owner.operationCancel,
			sdk.MethodAuthStart:       owner.authStart,
			sdk.MethodAuthFinish:      owner.authFinish,
			sdk.MethodAuthRefresh:     owner.refresh,
			sdk.MethodAuthLogout: func(context.Context, sdk.Call, *sdk.Stream) (any, error) {
				return nil, nil
			},
		},
		Actions: map[string]sdk.Handler{"execution.list": owner.executionList, "execution.settle": owner.settle,
			"execution.reconcile": owner.reconcile, "execution.abandon": owner.abandon,
			"billing.status": owner.billingStatus, "billing.claim": owner.billingClaim,
			"billing.offers": owner.billingOffers, "billing.checkin": owner.billingCheckin,
			"reset.status": owner.resetStatus, "reset.use": owner.resetUse, "reset.resolve": owner.resetResolve,
			"reset.summary": owner.resetSummary, "reset.reconcile": owner.resetReconcile,
			"reset.opportunity": owner.resetOpportunity, "reset.opportunity.resolve": owner.resetOpportunityResolve,
			"reset.ack": owner.resetAcknowledge},
	}
	if manifest.Contract.Has(sdk.AgentProcess) {
		result.Actions["native.run"] = owner.nativeRun
		result.Actions["native.stop"] = owner.nativeStop
	}
	return result
}

func decode(raw json.RawMessage, target any) error {
	if _, err := protocol.ReadObject(raw); err != nil {
		return invalid("Invalid ZCode parameters")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		return invalid("Invalid ZCode parameters")
	}
	return nil
}

func invalid(message string) error {
	return &sdk.Error{Code: "invalid_params", Message: message, Outcome: "not-started"}
}

func unsupported(message string) error {
	return &sdk.Error{Code: "unsupported", Message: message, Outcome: "not-started"}
}

func blocked(message string) error {
	return &sdk.Error{Code: "host_contract_required", Message: message, Outcome: "not-started"}
}

func providerScope(call sdk.Call) error {
	if call.Scope.Provider != "" && call.Scope.Provider != ProviderID {
		return &sdk.Error{Code: "permission_denied", Message: "The selected provider is not ZCode", Outcome: "not-started"}
	}
	return nil
}

func (owner *module) identity(call sdk.Call) (Credentials, error) {
	if err := providerScope(call); err != nil {
		return Credentials{}, err
	}
	var credentials Credentials
	if call.Scope.Account == "" || call.Scope.Generation == 0 || decode(call.Auth, &credentials) != nil ||
		credentials.AccountID() != call.Scope.Account {
		return credentials, &sdk.Error{Code: "permission_denied", Message: "An explicit matching ZCode account generation is required", Outcome: "not-started"}
	}
	if credentials.validateStructure() != nil {
		return credentials, invalid("Invalid selected ZCode credential snapshot")
	}
	return credentials, nil
}

func (owner *module) selected(call sdk.Call) (Credentials, error) {
	credentials, err := owner.identity(call)
	if err != nil {
		return credentials, err
	}
	if err := credentials.Validate(owner.options.Now()); err != nil {
		if errors.Is(err, ErrExpired) {
			return credentials, &sdk.Error{Code: "auth_refresh_required", Message: "The selected ZCode credential is due for host-owned renewal or sign-in", Outcome: "not-started"}
		}
		return credentials, invalid("Invalid selected ZCode credential snapshot")
	}
	return credentials, nil
}

func (owner *module) authFinish(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if err := providerScope(call); err != nil {
		return nil, err
	}
	var request sdk.AuthFinishRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	if len(request.Continuation) != 0 {
		return owner.finishFlow(ctx, call, stream, request)
	}
	if request.Code != "" || len(request.Continuation) != 0 {
		return nil, unsupported("Snapshot import does not exchange authorization codes")
	}
	var credentials Credentials
	if decode(request.Input, &credentials) != nil || credentials.Validate(owner.options.Now()) != nil {
		return nil, invalid("Import requires a valid unexpired ZCode credential snapshot")
	}
	return sdk.AuthResult{
		ID: credentials.AccountID(), Name: "ZCode " + credentials.Site + " " + credentials.Kind,
		Credentials: bytes.Clone(request.Input), ExpiresAt: credentials.ExpiresAt,
	}, nil
}

func (owner *module) models(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if err := providerScope(call); err != nil {
		return nil, err
	}
	var request sdk.ModelsRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	if len(request.Credentials) != 0 {
		return nil, invalid("Credentials must come from the host account scope")
	}
	models := Models()
	if call.Scope.Account != "" {
		credentials, err := owner.selected(call)
		if err != nil {
			return nil, err
		}
		if credentials.Discovery {
			models, err = owner.discoveredModels(ctx, stream, credentials)
			if err != nil {
				return nil, err
			}
		}
		if credentials.Kind == "start" && !credentials.Discovery {
			gift, err := owner.startEntitlement(ctx, stream, credentials, false)
			if err != nil {
				return nil, err
			}
			models = appendGiftModels(nil, models, gift, false)
		}
		if credentials.Kind != "start" && !credentials.Discovery && giftSession(credentials, owner.options.Now()) {
			if gift, err := owner.startEntitlement(ctx, stream, credentials, false); err == nil {
				models = appendGiftModels(models, models, gift, true)
			}
		}
		if credentials.Kind == "individual" || credentials.Kind == "team" {
			if state, err := currentInstance(ctx); err == nil && state.dataDir != "" &&
				credentials.SessionToken != "" && credentials.SessionExpiresAt > owner.options.Now().Add(30*time.Second).UnixMilli() {
				for _, alias := range aliasModels() {
					if credentials.Discovery {
						found := false
						for _, model := range models {
							found = found || !model.Free && strings.EqualFold(model.ID, rawLaneModel(alias.ID))
						}
						if !found {
							continue
						}
					}
					lane, _ := NativeLane(alias.ID)
					if lane == "highspeed" && (credentials.BusinessToken == "" || credentials.BusinessExpiresAt <= owner.options.Now().Add(30*time.Second).UnixMilli()) {
						continue
					}
					models = append(models, alias)
				}
			}
		}
	}
	return sdk.ModelsResult{Models: models}, nil
}

func knownModel(model string, credentials Credentials) bool {
	for _, entry := range Models() {
		if entry.ID == model && (credentials.Kind != "start" || model != "GLM-5.3") {
			return true
		}
	}
	return false
}

type executeOptions struct {
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	EscapedPath string          `json:"escapedPath"`
	Query       string          `json:"query"`
	Proxy       json.RawMessage `json:"proxy"`
	CallerID    string          `json:"callerId"`
	Native      json.RawMessage `json:"native"`
	RequestID   string          `json:"requestId"`
}

type executeInput struct {
	Protocol    string          `json:"protocol"`
	Operation   string          `json:"operation,omitempty"`
	Body        []byte          `json:"body,omitempty"`
	Headers     http.Header     `json:"headers,omitempty"`
	Model       string          `json:"model"`
	OperationID string          `json:"operationId,omitempty"`
	Credentials json.RawMessage `json:"credentials,omitempty"`
	Options     json.RawMessage `json:"options,omitempty"`
}

func (owner *module) execute(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	credentials, err := owner.selected(call)
	if err != nil {
		return nil, err
	}
	var request executeInput
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	lane, native := NativeLane(request.Model)
	trial := strings.HasSuffix(strings.ToLower(request.Model), "-trial")
	allowedModel := knownModel(request.Model, credentials) || credentials.Kind == "start" && !native
	giftOnly := false
	if credentials.Discovery && !trial {
		models, err := owner.discoveredModels(ctx, stream, credentials)
		if err != nil {
			return nil, err
		}
		allowedModel = false
		target := request.Model
		if native {
			target = rawLaneModel(request.Model)
		}
		for _, model := range models {
			if model.ID == target && (!native || !model.Free) {
				allowedModel = true
				giftOnly = model.Free && !trial
			}
		}
	}
	options := executeOptions{}
	if len(request.Options) != 0 {
		if err := decode(request.Options, &options); err != nil {
			return nil, blocked("Only ordinary Messages paths are supported; caller options cannot admit native execution")
		}
	}
	if len(options.Native) != 0 && !bytes.Equal(bytes.TrimSpace(options.Native), []byte("null")) {
		metadata, err := protocol.ReadObject(options.Native)
		if err != nil || len(metadata) != 0 {
			return nil, blocked("Native execution metadata requires a host admission contract; ordinary paid fallback is not authorized")
		}
	}
	for name := range request.Headers {
		switch strings.ToLower(name) {
		case "x-off-peak-ticket-id", "x-highspeed-card-id", "x-coding-plan-api-key", "x-bigmodel-authorization":
			return nil, blocked("Native lane headers cannot authorize ordinary or ticket/card execution")
		}
	}
	if request.Protocol != "messages" && request.Protocol != "anthropic" || request.Operation != "" ||
		!allowedModel && !trial && (!native || credentials.Discovery) || call.Scope.Operation == "" ||
		request.OperationID != "" && request.OperationID != call.Scope.Operation ||
		options.Method != "" && options.Method != http.MethodPost ||
		options.EscapedPath != "" && options.EscapedPath != options.Path ||
		options.Path != "" && options.Path != "/v1/messages" && options.Path != "/messages" || options.Query != "" ||
		len(request.Credentials) != 0 {
		return nil, unsupported("This ZCode slice serves ordinary packaged Messages models using host-selected credentials only")
	}
	if native {
		return owner.executeLane(ctx, call, stream, credentials, request, lane)
	}
	target, dispatch, gift, err := owner.giftRoute(ctx, call, stream, credentials, request.Model, trial, giftOnly)
	if err != nil {
		return nil, err
	}
	input, err := protocol.ReadObject(request.Body)
	if err != nil || input["model"] != request.Model {
		return nil, invalid("Messages body does not match its selected model")
	}
	input["model"] = target
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	body, renamed, err := protocol.Prepare(encoded, target)
	if err != nil {
		return nil, invalid("Invalid Messages body or conflicting ToolSearch tool names")
	}
	device := accountDevice(credentials)
	session := sessionID(call.Scope.Session, device)
	query := queryID(body, session)
	body, err = markIdentity(body, device, session)
	if err != nil {
		return nil, err
	}
	plainBody := body
	if dispatch.Kind == "start" {
		body, err = protocol.PrepareStart(body, owner.options.Now())
		if err != nil {
			return nil, invalid("Start request has an unsupported system shape or exceeds its byte budget")
		}
	}
	endpoint := owner.options.Endpoints.forAccount(dispatch)
	var proxy sdk.NetworkProxyResult
	if err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: endpoint}, &proxy); err != nil {
		return nil, err
	}
	attribution, err := newRequestID()
	if err != nil {
		return nil, err
	}
	outbound, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, invalid("Invalid ZCode dispatch")
	}
	outbound.Header = requestHeaders(request.Headers, dispatch, attribution)
	outbound.Header.Set("X-Session-Id", session)
	outbound.Header.Set("X-ZCode-Trace-Id", seededUUID("zcode-trace\x00"+session))
	if query != "" {
		outbound.Header.Set("X-Query-Id", query)
	}
	var response *http.Response
	workflowAccepted := gift && !trial && !giftOnly && credentials.Kind != "start"
	if workflowAccepted {
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
			return nil, err
		}
	}
	if dispatch.Kind == "start" {
		var release func()
		response, release, err = owner.startDispatch(ctx, stream, dispatch, outbound, proxy.URL)
		if release != nil {
			defer release()
		}
	} else {
		response, err = owner.paidDispatch(ctx, stream, proxy.URL, outbound)
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var failure *sdk.Error
		if errors.As(err, &failure) {
			return nil, failure
		}
		return nil, &sdk.Error{Code: "upstream", Message: "ZCode transport failed after dispatch; no retry was made", Outcome: "uncertain"}
	}
	if gift {
		var paid bool
		response, paid, err = owner.giftFallback(ctx, call, stream, credentials, target, !trial && !giftOnly && credentials.Kind != "start", request.Headers, plainBody, outbound, response)
		if err != nil {
			return nil, err
		}
		gift = !paid
	}
	defer response.Body.Close()
	if !workflowAccepted {
		refusal, err := owner.finalRefusal(response, request.Model, gift)
		if err != nil {
			return nil, err
		}
		if refusal != nil {
			return *refusal, nil
		}
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
			return nil, err
		}
	}
	replyModel := ""
	if gift && credentials.Kind != "start" {
		replyModel = wireGiftModel(target) + "-Trial"
	}
	return streamResponse(ctx, response, stream, renamed, owner.observability, replyModel)
}
