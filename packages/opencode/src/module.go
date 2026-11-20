package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type HTTPFactory func(proxy string) (Doer, func(), error)

type ModuleOptions struct {
	Endpoints   Endpoints
	Now         func() time.Time
	HTTPFactory HTTPFactory
}

type moduleConfig struct {
	ProjectID         string             `json:"projectId"`
	AgentFormat       string             `json:"agentFormat,omitempty"`
	ConsoleWatches    bool               `json:"consoleWatches,omitempty"`
	PublicEnvironment *publicEnvironment `json:"publicEnvironment,omitempty"`
}

type instance struct {
	headers   *Headers
	config    moduleConfig
	console   *consoleCache
	goCatalog goCatalog
}

type module struct {
	options ModuleOptions
	zen     bool
}

type deviceFlow struct {
	State    string          `json:"state"`
	Device   string          `json:"device,omitempty"`
	Deadline time.Time       `json:"deadline"`
	Next     time.Time       `json:"next"`
	Interval time.Duration   `json:"interval"`
	Provider string          `json:"provider,omitempty"`
	Method   string          `json:"method,omitempty"`
	Private  json.RawMessage `json:"private,omitempty"`
}

type continuation struct {
	ID string `json:"id"`
}

func NewModule(options ModuleOptions) sdk.Module {
	return newModule(options, false)
}

func newModule(options ModuleOptions, zen bool) sdk.Module {
	if options.Endpoints == (Endpoints{}) {
		options.Endpoints = DefaultEndpoints()
	}
	options.Endpoints.Console = strings.TrimRight(options.Endpoints.Console, "/")
	options.Endpoints.Inference = strings.TrimRight(options.Endpoints.Inference, "/")
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.HTTPFactory == nil {
		options.HTTPFactory = defaultHTTP
	}
	owner := &module{options: options, zen: zen}
	result := sdk.Module{
		Manifest: Manifest(),
		Start:    owner.start,
		Handlers: map[string]sdk.Handler{
			sdk.MethodAuthStart:   owner.authStart,
			sdk.MethodAuthFinish:  owner.authFinish,
			sdk.MethodAuthRefresh: owner.authRefresh,
			sdk.MethodAuthLogout: func(context.Context, sdk.Call, *sdk.Stream) (any, error) {
				// The donor has local account removal, not a discovered vendor
				// revocation endpoint. The host owns deleting the chosen generation.
				return nil, nil
			},
			sdk.MethodProviderModels:  owner.models,
			sdk.MethodProviderExecute: owner.execute,
			sdk.MethodProviderUsage:   owner.usage,
		},
	}
	if zen {
		result.Actions = map[string]sdk.Handler{
			"console.snapshot":    owner.consoleSnapshot,
			"console.skill":       owner.consoleSkill,
			"console.watch.list":  owner.consoleWatchList,
			"console.watch.types": owner.consoleWatchTypes,
			"console.watch.poll":  owner.consoleWatchPoll,
			"console.watch.ack":   owner.consoleWatchAck,
		}
	}
	return result
}

func decode(raw json.RawMessage, out any) error {
	input := bytes.TrimSpace(raw)
	if len(input) < 2 || input[0] != '{' || input[len(input)-1] != '}' {
		return &sdk.Error{Code: "invalid_params", Message: "OpenCode module parameters require a JSON object"}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return &sdk.Error{Code: "invalid_params", Message: "Invalid OpenCode module parameters"}
	}
	if decoder.Decode(new(any)) != io.EOF {
		return &sdk.Error{Code: "invalid_params", Message: "Invalid OpenCode module parameters"}
	}
	return nil
}

func (owner *module) start(_ context.Context, context sdk.ModuleContext) (sdk.ModuleReady, func(), error) {
	if err := owner.options.Endpoints.validate(); err != nil {
		return sdk.ModuleReady{}, nil, err
	}
	var config moduleConfig
	if err := decode(context.Config.Values, &config); err != nil {
		return sdk.ModuleReady{}, nil, err
	}
	if config.ProjectID != "" && !validProject(config.ProjectID) {
		return sdk.ModuleReady{}, nil, &sdk.Error{Code: "invalid_params", Message: "projectId must be an explicit OpenCode project hash or global"}
	}
	if config.AgentFormat != "" && config.AgentFormat != "v1" && config.AgentFormat != "v2" {
		return sdk.ModuleReady{}, nil, &sdk.Error{Code: "invalid_params", Message: "agentFormat must be v1 or v2"}
	}
	if config.PublicEnvironment != nil && !config.PublicEnvironment.valid() {
		return sdk.ModuleReady{}, nil, &sdk.Error{Code: "invalid_params", Message: "publicEnvironment requires explicit directory, workspace, git, platform and temporary facts"}
	}
	return sdk.ModuleReady{State: &instance{headers: NewHeaders(), config: config, console: newConsoleCache()}}, nil, nil
}

func currentInstance(ctx context.Context) (*instance, error) {
	context, found := sdk.ModuleContextFrom(ctx)
	state, valid := context.State.(*instance)
	if !found || !valid {
		return nil, &sdk.Error{Code: "not_ready", Message: "OpenCode module is not configured"}
	}
	return state, nil
}

func providerScope(call sdk.Call) error {
	if call.Scope.Provider != "" && call.Scope.Provider != ProviderGo && call.Scope.Provider != ProviderZen {
		return &sdk.Error{Code: "permission_denied", Message: "An explicit OpenCode OAuth provider is required"}
	}
	return nil
}

func selected(call sdk.Call) (Credentials, error) {
	if err := providerScope(call); err != nil {
		return Credentials{}, err
	}
	var credentials Credentials
	if call.Scope.Provider == ProviderZen {
		identity, err := selectedZen(call)
		if err != nil || identity.Kind != "oauth" {
			return credentials, &sdk.Error{Code: "permission_denied", Message: "A selected OpenCode OAuth account is required"}
		}
		return *identity.OAuth, nil
	}
	if call.Scope.Account == "" || call.Scope.Generation == 0 ||
		json.Unmarshal(call.Auth, &credentials) != nil || credentials.Validate() != nil ||
		credentials.AccountID(ProviderGo) != call.Scope.Account {
		return Credentials{}, &sdk.Error{Code: "permission_denied", Message: "An explicit matching OpenCode account generation is required"}
	}
	return credentials, nil
}

func failure(err error, outcome string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, errOpenCodeContentRefusal) {
		return &sdk.Error{Code: "content_refusal", Message: errOpenCodeContentRefusal.Error(), Outcome: outcome}
	}
	if errors.Is(err, ErrRefreshRequired) {
		return &sdk.Error{Code: "auth_refresh_required", Message: "OpenCode selected account requires renewal before dispatch", Outcome: outcome}
	}
	if errors.Is(err, ErrAmbiguousOrganization) {
		return &sdk.Error{Code: "unsupported", Message: ErrAmbiguousOrganization.Error(), Outcome: outcome}
	}
	return &sdk.Error{Code: "upstream", Message: "OpenCode upstream request failed", Outcome: outcome}
}

func notStarted(err error) error {
	var rejected *sdk.Error
	if errors.As(err, &rejected) {
		return &sdk.Error{Code: rejected.Code, Message: rejected.Message, Outcome: "not-started"}
	}
	return &sdk.Error{Code: "invalid_params", Message: "OpenCode request was rejected before dispatch", Outcome: "not-started"}
}

type proxyHTTP struct {
	stream  *sdk.Stream
	factory HTTPFactory
}

type ownedBody struct {
	io.ReadCloser
	close func()
}

func (body *ownedBody) Close() error {
	err := body.ReadCloser.Close()
	body.close()
	return err
}

func (client proxyHTTP) Do(req *http.Request) (*http.Response, error) {
	var proxy sdk.NetworkProxyResult
	if err := client.stream.HostCall(req.Context(), sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: req.URL.String()}, &proxy); err != nil {
		return nil, err
	}
	httpClient, closeClient, err := client.factory(proxy.URL)
	if err != nil {
		return nil, err
	}
	if closeClient == nil {
		closeClient = func() {}
	}
	response, err := httpClient.Do(req)
	if err != nil {
		closeClient()
		return nil, err
	}
	response.Body = &ownedBody{ReadCloser: response.Body, close: closeClient}
	return response, nil
}

func defaultHTTP(proxy string) (Doer, func(), error) {
	transport := &http.Transport{
		DialContext:       (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 30 * time.Second,
	}
	if proxy != "" {
		address, err := url.Parse(proxy)
		if err != nil || address.Host == "" ||
			(address.Scheme != "http" && address.Scheme != "https" && address.Scheme != "socks5" && address.Scheme != "socks5h") {
			return nil, nil, errors.New("invalid host proxy resolution")
		}
		transport.Proxy = http.ProxyURL(address)
	}
	return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}, transport.CloseIdleConnections, nil
}

func (owner *module) client(stream *sdk.Stream) *Client {
	return &Client{HTTP: proxyHTTP{stream: stream, factory: owner.options.HTTPFactory},
		Endpoints: owner.options.Endpoints, Now: owner.options.Now, Version: ClientVersion}
}

func stateRead(ctx context.Context, stream *sdk.Stream, id string) (deviceFlow, string, error) {
	var value sdk.StateValue
	if err := stream.HostCall(ctx, sdk.MethodStateGet, sdk.StateGetRequest{Key: "device." + id}, &value); err != nil {
		return deviceFlow{}, "", err
	}
	var flow deviceFlow
	if !value.Found || json.Unmarshal(value.Value, &flow) != nil || flow.Deadline.IsZero() {
		return flow, "", &sdk.Error{Code: "invalid_params", Message: "OpenCode device continuation is missing or unreadable"}
	}
	return flow, value.Version, nil
}

func stateWrite(ctx context.Context, stream *sdk.Stream, id, version string, flow deviceFlow) (string, error) {
	value, err := json.Marshal(flow)
	if err != nil {
		return "", err
	}
	var result sdk.StateCASResult
	if err := stream.HostCall(ctx, sdk.MethodStateCAS, sdk.StateCASRequest{Key: "device." + id, Version: version, Value: value}, &result); err != nil {
		return "", err
	}
	if !result.Swapped {
		return "", &sdk.Error{Code: "conflict", Message: "OpenCode device continuation changed"}
	}
	return result.Version, nil
}

func (owner *module) authStart(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if err := owner.provider(call); err != nil {
		return nil, err
	}
	var request sdk.AuthStartRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	if request.Method == "key" || request.Method == "public" {
		return owner.startZenIdentity(ctx, call, stream, request)
	}
	if err := providerScope(call); err != nil {
		return nil, err
	}
	if request.Method != "" && request.Method != "device" || request.RedirectURI != "" || len(request.Input) != 0 && string(request.Input) != "null" && string(request.Input) != "{}" {
		return nil, &sdk.Error{Code: "invalid_params", Message: "This slice supports device authorization without redirect or custom input"}
	}
	started := owner.options.Now()
	code, err := owner.client(stream).StartDevice(ctx)
	if err != nil {
		return nil, failure(err, "")
	}
	id, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	now := owner.options.Now()
	if int64(code.Interval) > int64((1<<63-1)/time.Second) {
		return nil, &sdk.Error{Code: "invalid_params", Message: "OpenCode returned an invalid poll interval"}
	}
	interval := time.Duration(code.Interval) * time.Second
	deadline := now.Add(time.Duration(code.ExpiresIn) * time.Second)
	// The donor limits each device flow to signInTimeout (ten minutes).
	if maximum := started.Add(10 * time.Minute); deadline.After(maximum) {
		deadline = maximum
	}
	if !now.Before(deadline) {
		return nil, &sdk.Error{Code: "expired", Message: "OpenCode device authorization expired during startup"}
	}
	if _, err := stateWrite(ctx, stream, id, "0", deviceFlow{State: "pending", Device: code.Device, Deadline: deadline, Next: now.Add(interval), Interval: interval, Provider: call.Scope.Provider, Method: "device"}); err != nil {
		return nil, err
	}
	value, _ := json.Marshal(continuation{ID: id})
	return sdk.AuthStartResult{URL: code.URL, UserCode: code.User, PollAfterMs: interval.Milliseconds(), Continuation: value}, nil
}

func (owner *module) authFinish(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if err := owner.provider(call); err != nil {
		return nil, err
	}
	var request sdk.AuthFinishRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	var key continuation
	if request.Code != "" || len(request.Input) != 0 && string(request.Input) != "null" || decode(request.Continuation, &key) != nil ||
		len(key.ID) != 32 || strings.Trim(key.ID, "0123456789abcdef") != "" {
		return nil, &sdk.Error{Code: "invalid_params", Message: "An OpenCode device continuation is required"}
	}
	flow, version, err := stateRead(ctx, stream, key.ID)
	if err != nil {
		return nil, err
	}
	if flow.Provider != call.Scope.Provider {
		return nil, &sdk.Error{Code: "permission_denied", Message: "OpenCode continuation belongs to another provider"}
	}
	if flow.Method == "key" || flow.Method == "public" {
		return owner.finishZenIdentity(ctx, call, stream, key.ID, version, flow)
	}
	if err := providerScope(call); err != nil {
		return nil, err
	}
	now := owner.options.Now()
	if flow.State != "pending" || !now.Before(flow.Deadline) {
		return nil, &sdk.Error{Code: "expired", Message: "OpenCode device flow ended or is uncertain; start sign-in again"}
	}
	if now.Before(flow.Next) {
		return sdk.AuthResult{Pending: true}, nil
	}
	ctx, cancel := context.WithDeadline(ctx, flow.Deadline)
	defer cancel()
	flow.State = "polling"
	version, err = stateWrite(ctx, stream, key.ID, version, flow)
	if err != nil {
		return nil, err
	}
	poll, err := owner.client(stream).PollDevice(ctx, flow.Device)
	if err != nil {
		flow.State, flow.Device = "failed", ""
		_, _ = stateWrite(ctx, stream, key.ID, version, flow)
		return nil, failure(err, "")
	}
	if poll.Pending {
		if poll.SlowDown {
			if flow.Interval > time.Duration(1<<63-1)-5*time.Second {
				return nil, &sdk.Error{Code: "invalid_params", Message: "OpenCode poll interval exceeds its duration limit"}
			}
			flow.Interval += 5 * time.Second
		}
		flow.State, flow.Next = "pending", owner.options.Now().Add(flow.Interval)
		if _, err := stateWrite(ctx, stream, key.ID, version, flow); err != nil {
			return nil, err
		}
		return sdk.AuthResult{Pending: true}, nil
	}
	credentials, err := owner.client(stream).Identity(ctx, *poll.Token)
	flow.State, flow.Device = "finished", ""
	if _, writeErr := stateWrite(ctx, stream, key.ID, version, flow); writeErr != nil {
		return nil, writeErr
	}
	if err != nil {
		return nil, failure(err, "")
	}
	if !owner.options.Now().Before(flow.Deadline) {
		return nil, &sdk.Error{Code: "expired", Message: "OpenCode device flow expired before publication"}
	}
	if call.Scope.Account != "" {
		previous, err := selected(call)
		if err != nil || credentials.UserID != previous.UserID {
			return nil, &sdk.Error{Code: "permission_denied", Message: "OpenCode authorization did not match the selected account", Outcome: "uncertain"}
		}
		credentials.InitialOrgID = previous.InitialOrgID
	}
	auth := oauthPayload(call.Scope.Provider, credentials)
	id := credentials.AccountID(oauthProvider(call.Scope.Provider))
	return sdk.AuthResult{ID: id, Name: credentials.Email + " [" + id[:12] + "]",
		Credentials: auth, ExpiresAt: credentials.Expires.UnixMilli()}, nil
}

func (owner *module) authRefresh(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if err := owner.provider(call); err != nil {
		return nil, notStarted(err)
	}
	var request sdk.AuthRefreshRequest
	if decode(call.Params, &request) != nil {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Invalid OpenCode refresh parameters", Outcome: "not-started"}
	}
	if len(request.Credentials) != 0 && !bytes.Equal(bytes.TrimSpace(request.Credentials), []byte("null")) {
		return nil, &sdk.Error{Code: "permission_denied", Message: "Refresh credentials must come from the host-selected account", Outcome: "not-started"}
	}
	credentials, err := selected(call)
	if err != nil {
		return nil, notStarted(err)
	}
	replacement, err := owner.client(stream).Refresh(ctx, credentials)
	if err != nil {
		var refused *RemoteError
		if errors.As(err, &refused) && refused.Status == http.StatusBadRequest && refused.Code == "invalid_grant" {
			return nil, &sdk.Error{Code: "auth_revoked", Message: "OpenCode refresh token was rejected; sign in again", Outcome: "uncertain"}
		}
		return nil, failure(err, "uncertain")
	}
	auth := oauthPayload(call.Scope.Provider, replacement)
	return sdk.AuthResult{ID: call.Scope.Account, Name: replacement.Email + " [" + call.Scope.Account[:12] + "]",
		Credentials: auth, ExpiresAt: replacement.Expires.UnixMilli()}, nil
}

func (owner *module) models(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if err := owner.provider(call); err != nil {
		return nil, err
	}
	if call.Scope.Provider == ProviderZen || call.Scope.Provider == ProviderZenPublic {
		return owner.zenModels(ctx, call, stream)
	}
	if err := providerScope(call); err != nil {
		return nil, err
	}
	return owner.goModels(ctx, call, stream)
}

func (owner *module) execute(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if err := owner.provider(call); err != nil {
		return nil, notStarted(err)
	}
	if call.Scope.Provider == ProviderZen || call.Scope.Provider == ProviderZenPublic {
		return owner.executeZen(ctx, call, stream)
	}
	credentials, err := selected(call)
	if err != nil {
		return nil, notStarted(err)
	}
	if credentials.Due(owner.options.Now()) {
		return nil, failure(ErrRefreshRequired, "not-started")
	}
	var request sdk.ExecuteRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, notStarted(err)
	}
	var options struct {
		Method      string          `json:"method"`
		Path        string          `json:"path"`
		EscapedPath string          `json:"escapedPath"`
		Query       string          `json:"query"`
		Proxy       json.RawMessage `json:"proxy"`
		CallerID    string          `json:"callerId"`
		Native      json.RawMessage `json:"native"`
		RequestID   string          `json:"requestId"`
	}
	if len(request.Options) != 0 {
		if err := decode(request.Options, &options); err != nil {
			return nil, notStarted(err)
		}
	}
	if !validInferenceModelID(request.Model) || request.Protocol != "chat" && request.Protocol != "gemini" ||
		request.Operation != "" || call.Scope.Operation == "" ||
		options.Method != "" && options.Method != http.MethodPost ||
		options.EscapedPath != "" && options.EscapedPath != options.Path {
		return nil, &sdk.Error{Code: "unsupported", Message: "OpenCode execution requires a Go model and its native conversational protocol", Outcome: "not-started"}
	}
	geminiStream := false
	switch request.Protocol {
	case "chat":
		if options.Path != "" && options.Path != "/v1/chat/completions" && options.Path != "/chat/completions" || options.Query != "" {
			return nil, &sdk.Error{Code: "unsupported", Message: "Unsupported OpenCode chat endpoint or query", Outcome: "not-started"}
		}
	case "gemini":
		endpoint := strings.TrimPrefix(options.Path, "/v1beta")
		switch endpoint {
		case "", "/models/" + request.Model + ":generateContent":
			if options.Query != "" {
				return nil, &sdk.Error{Code: "unsupported", Message: "Unsupported OpenCode Gemini query", Outcome: "not-started"}
			}
		case "/models/" + request.Model + ":streamGenerateContent":
			if options.Query != "alt=sse" {
				return nil, &sdk.Error{Code: "unsupported", Message: "OpenCode Gemini streaming requires alt=sse", Outcome: "not-started"}
			}
			geminiStream = true
		default:
			return nil, &sdk.Error{Code: "unsupported", Message: "Unsupported OpenCode Gemini endpoint", Outcome: "not-started"}
		}
	}
	var body map[string]json.RawMessage
	var model string
	if len(request.Body) > sdk.MaxNativeBodyBytes || json.Unmarshal(request.Body, &body) != nil || body == nil {
		return nil, &sdk.Error{Code: "invalid_params", Message: "OpenCode execution requires a bounded JSON object", Outcome: "not-started"}
	}
	if request.Protocol == "chat" || len(body["model"]) != 0 {
		if json.Unmarshal(body["model"], &model) != nil || model != request.Model {
			return nil, &sdk.Error{Code: "invalid_params", Message: "Body model must match the selected packaged model", Outcome: "not-started"}
		}
	}
	var gemini geminiTerminal
	var chatStream bool
	if request.Protocol == "gemini" {
		if err := gemini.configure(body); err != nil {
			return nil, &sdk.Error{Code: "invalid_params", Message: "Invalid OpenCode Gemini generation body", Outcome: "not-started"}
		}
	} else if streamFlag, ok := body["stream"]; ok {
		// Only an explicit JSON true requests chat streaming.
		chatStream = bytes.Equal(bytes.TrimSpace(streamFlag), []byte("true"))
	}
	state, err := currentInstance(ctx)
	if err != nil {
		return nil, notStarted(err)
	}
	headers := request.Headers.Clone()
	if headers == nil {
		headers = http.Header{}
	}
	project := state.config.ProjectID
	if project == "" {
		project = headers.Get("x-opencode-project")
	}
	if err := state.headers.Stamp(headers, call.Scope.Account, options.CallerID, call.Scope.Session, project, ClientVersion, owner.options.Now()); err != nil {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Configure an explicit projectId before OpenCode chat execution", Outcome: "not-started"}
	}
	supported, err := owner.canExecuteGo(ctx, call, stream, state, credentials, request.Model, request.Protocol)
	if err != nil {
		return nil, err
	}
	if !supported {
		return nil, &sdk.Error{Code: "unsupported", Message: "Go model is absent or uses another native protocol", Outcome: "not-started"}
	}
	var response *http.Response
	if request.Protocol == "gemini" {
		response, err = owner.client(stream).generateGemini(ctx, credentials, headers, request.Body, request.Model, geminiStream)
	} else {
		response, err = owner.client(stream).Chat(ctx, credentials, headers, request.Body)
	}
	if err != nil {
		if errors.Is(err, ErrRefreshRequired) {
			return nil, failure(err, "not-started")
		}
		if emitErr := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); emitErr != nil {
			return nil, emitErr
		}
		return nil, failure(err, "uncertain")
	}
	defer response.Body.Close()
	if refused, proved, err := executionRefusal(ctx, stream, call.Scope, response, credentials.Access); err != nil {
		return nil, err
	} else if proved {
		return refused, nil
	}
	success := response.StatusCode >= 200 && response.StatusCode < 300
	contentType, _, _ := strings.Cut(response.Header.Get("Content-Type"), ";")
	streaming := success && strings.EqualFold(strings.TrimSpace(contentType), "text/event-stream")
	var reader io.Reader = response.Body
	if success && !streaming && chatStream {
		buffered := bufio.NewReaderSize(reader, 4096)
		streaming = startsChatEvents(buffered)
		reader = buffered
	}
	if success && !streaming && (chatStream || geminiStream) {
		return nil, &sdk.Error{Code: "truncated", Message: "OpenCode streaming returned a non-SSE response", Outcome: "uncertain"}
	}
	if streaming && strings.TrimSpace(contentType) != "text/event-stream" {
		response.Header.Set("Content-Type", "text/event-stream")
	}
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: response.StatusCode, Headers: response.Header}); err != nil {
		return nil, err
	}
	if streaming && request.Protocol == "gemini" {
		if err := gemini.relay(ctx, reader, stream); err != nil {
			return nil, err
		}
		return sdk.ExecuteResult{Status: response.StatusCode, Outcome: "completed"}, nil
	}
	var terminal chatTerminal
	buffer := make([]byte, sdk.MaxChunkBytes)
	for {
		n, readErr := reader.Read(buffer)
		if n != 0 {
			if streaming {
				terminal.write(buffer[:n])
			}
			if err := stream.Emit(ctx, "data", buffer[:n]); err != nil {
				return nil, err
			}
			if streaming && terminal.done {
				break
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, failure(readErr, "uncertain")
		}
	}
	if streaming && !terminal.done {
		return nil, &sdk.Error{Code: "truncated", Message: "OpenCode stream ended without its terminal marker", Outcome: "uncertain"}
	}
	return sdk.ExecuteResult{Status: response.StatusCode, Outcome: "completed"}, nil
}

func (owner *module) usage(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if err := owner.provider(call); err != nil {
		return nil, err
	}
	var request sdk.UsageRequest
	if len(call.Params) != 0 {
		if err := decode(call.Params, &request); err != nil {
			return nil, err
		}
	}
	if len(request.Credentials) != 0 && string(request.Credentials) != "null" {
		return nil, &sdk.Error{Code: "invalid_params", Message: "OpenCode usage credentials must come from the selected account"}
	}
	if request.Revision != "" {
		if _, err := sdk.ParseUsageRevision(request.Revision); err != nil {
			return nil, &sdk.Error{Code: "invalid_params", Message: "OpenCode usage requires the canonical host revision"}
		}
	}
	result := sdk.UsageResult{Revision: request.Revision, Windows: []sdk.UsageWindow{}}
	if call.Scope.Provider == ProviderZen || call.Scope.Provider == ProviderZenPublic {
		if _, err := selectedZen(call); err != nil {
			return nil, err
		}
		return result, nil
	}
	credentials, err := selected(call)
	if err != nil {
		return nil, err
	}
	state, err := currentInstance(ctx)
	if err != nil {
		return nil, err
	}
	headers := http.Header{}
	if err := state.headers.Session(headers, call.Scope.Account, owner.options.Now()); err != nil {
		return nil, err
	}
	windows, err := owner.client(stream).Usage(ctx, credentials, headers)
	if err != nil {
		return nil, failure(err, "")
	}
	for _, window := range windows {
		value := sdk.UsageWindow{Name: window.Name, Used: window.Used, Limit: 100}
		if window.ResetsAt != nil {
			value.ResetsAt = window.ResetsAt.UnixMilli()
		}
		result.Windows = append(result.Windows, value)
	}
	return result, nil
}
