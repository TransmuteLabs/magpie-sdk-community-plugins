package codearts

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

const (
	ModuleID          = "codearts"
	CodeArtsProvider  = "codearts"
	OfficeAceProvider = "officeace"
	Version           = "0.1.0"

	DefaultSnapMainland = "https://snap-access.cn-north-4.myhuaweicloud.com"
	DefaultSnapIntl     = "https://snap-access.ap-southeast-1.myhuaweicloud.com"
	DefaultOpenGWHost   = "https://opengw.developer.huaweicloud.com"
	DefaultIAMHost      = "https://iam.cn-north-4.myhuaweicloud.com"
	DefaultSTSTokenURL  = "https://sts.cn-north-4.myhuaweicloud.com/v1/oauth2/tokens"

	DefaultOAClawBase         = "https://agentarts.cn-southwest-2.myhuaweicloud.com"
	DefaultOASubscriptionBase = "https://officeace.cn-southwest-2.huaweicloud-agentarts.com"
	DefaultOAMaaSBase         = "https://api.modelarts-maas.com"
	OAPermissionPath          = "/v1/claw/client-permission-validate"
	OASubscriptionPath        = "/v1/subscription"
	OATokenRatesPath          = "/v1/model-token-rates"
	OABonusClaimPath          = "/v1/subscription/bonus/claim"
	OAModelsPath              = "/v2/models"
	OAChatPath                = "/v2/chat/completions"
	OARegion                  = "cn-southwest-2"
	OAService                 = "apic"

	ClientID               = "codearts-agent"
	OAAgentArtsClientID    = "pdp5_for_agentarts"
	RedirectPath           = "/oauth/callback"
	LoginPluginName        = "snap_AIIDE"
	LoginPluginVersion     = "5.2.0"
	OAuthTheme             = "2"
	GrantAuthorizationCode = "authorization_code"
	GrantRefreshToken      = "refresh_token"
	MinCallbackPort        = 10000
	BenefitOK              = "0000"
	BenefitModelSuffix     = ":benefit"
	BenefitWindowLabel     = "免费额度"
	MainAgentFallbackID    = "a8bcb36232554267a5142361cc25a393"
	SessionLimitError      = "TM.00001041"
	SessionHeader          = "x-ot-session-id"
	AccountMaxInflight     = 3
	MaxOutputTokens        = 65536
	MaxSignedBodyBytes     = 12 * 1024 * 1024
	SSEPeekLimit           = 16 * 1024
	RefreshLeadMs          = 30 * 60 * 1000
	OARefreshLeadMs        = 10 * 60 * 1000
	ClaimRetryMs           = 60 * 60 * 1000
	OARatesTTLMs           = 20 * 60 * 1000
	CheckinCreditDays      = 30
)

type Station struct {
	ID      string
	Snap    string
	OpenGW  string
	Console string
	Portal  string
	Locale  string
	Login   string
}

func DefaultStations() map[string]Station {
	return map[string]Station{
		"mainland": {
			ID:      "mainland",
			Snap:    DefaultSnapMainland,
			OpenGW:  DefaultOpenGWHost,
			Console: "https://codearts.huaweicloud.com",
			Portal:  "https://codearts.huaweicloud.com/portal",
			Locale:  "zh-cn",
		},
		"international": {
			ID:      "international",
			Snap:    DefaultSnapIntl,
			OpenGW:  "",
			Console: "https://console-intl.huaweicloud.com",
			Portal:  "https://codearts.ap-southeast-1.huaweicloud.com/portal",
			Locale:  "en-us",
			Login:   "https://auth.huaweicloud.com/authui/login?site=intl",
		},
	}
}

type Endpoints struct {
	Stations           map[string]Station
	STSToken           string
	IAMHost            string
	OAClawBase         string
	OASubscriptionBase string
	OAMaaSBase         string
}

func DefaultEndpoints() Endpoints {
	return Endpoints{
		Stations:           DefaultStations(),
		STSToken:           DefaultSTSTokenURL,
		IAMHost:            DefaultIAMHost,
		OAClawBase:         DefaultOAClawBase,
		OASubscriptionBase: DefaultOASubscriptionBase,
		OAMaaSBase:         DefaultOAMaaSBase,
	}
}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type HTTPFactory func(proxyURL string) (Doer, func(), error)

type ModuleOptions struct {
	Endpoints   Endpoints
	Now         func() time.Time
	HTTPFactory HTTPFactory
	Thinking    *bool
}

type claimState struct {
	Day     string
	OK      bool
	At      int64
	Code    string
	Message string
}

type dailyReadState struct {
	Day    string
	Fresh  bool
	Window *sdk.UsageWindow
}

type creditBatch struct {
	Metric    string
	Amount    float64
	Left      float64
	ExpiresAt int64
	Estimated bool
}

type accountState struct {
	mu               sync.Mutex
	benefitModels    map[string]bool
	benefitModelsGen uint64
	hasBenefitSet    bool
	benefit          map[string]bool
	hasActiveBen     bool
	regular          map[string]bool
	hasRegular       bool
	noBenefit        map[string]bool
	aliasRefusals    map[string]bool
	claim            *claimState
	daily            *dailyReadState
	dailyRead        *dailyReadState
	agentID          string
	batches          []creditBatch
	batchesAt        int64
	gate             chan struct{}
}

type oaRatesEntry struct {
	at    int64
	rates map[string]float64
}

type oauthPending struct {
	mu       sync.Mutex
	done     bool
	cred     json.RawMessage
	id       string
	name     string
	expires  int64
	err      error
	closeFn  func()
	station  string
	provider string
}

type module struct {
	options   ModuleOptions
	mu        sync.Mutex
	accounts  map[string]*accountState
	oaRates   map[string]oaRatesEntry
	phones    map[string]phoneEntry
	oauthFlow map[string]*oauthPending
}

type phoneEntry struct {
	phone string
	at    int64
}

func defaultHTTP(proxyURL string) (Doer, func(), error) {
	transport := &http.Transport{
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        16,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second,
	}
	if strings.TrimSpace(proxyURL) != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, nil, err
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return client, transport.CloseIdleConnections, nil
}

func NewModule(options ModuleOptions) sdk.Module {
	defaults := DefaultEndpoints()
	if len(options.Endpoints.Stations) == 0 {
		options.Endpoints.Stations = defaults.Stations
	}
	if options.Endpoints.STSToken == "" {
		options.Endpoints.STSToken = defaults.STSToken
	}
	if options.Endpoints.IAMHost == "" {
		options.Endpoints.IAMHost = defaults.IAMHost
	}
	if options.Endpoints.OAClawBase == "" {
		options.Endpoints.OAClawBase = defaults.OAClawBase
	}
	if options.Endpoints.OASubscriptionBase == "" {
		options.Endpoints.OASubscriptionBase = defaults.OASubscriptionBase
	}
	if options.Endpoints.OAMaaSBase == "" {
		options.Endpoints.OAMaaSBase = defaults.OAMaaSBase
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.HTTPFactory == nil {
		options.HTTPFactory = defaultHTTP
	}
	owner := &module{
		options:   options,
		accounts:  map[string]*accountState{},
		oaRates:   map[string]oaRatesEntry{},
		phones:    map[string]phoneEntry{},
		oauthFlow: map[string]*oauthPending{},
	}
	return sdk.Module{
		Manifest: coreManifest(),
		Handlers: map[string]sdk.Handler{
			sdk.MethodAuthStart:   owner.authStart,
			sdk.MethodAuthFinish:  owner.authFinish,
			sdk.MethodAuthRefresh: owner.authRefresh,
			sdk.MethodAuthLogout: func(context.Context, sdk.Call, *sdk.Stream) (any, error) {
				return nil, nil
			},
			sdk.MethodProviderModels:  owner.models,
			sdk.MethodProviderExecute: owner.execute,
			sdk.MethodProviderUsage:   owner.usage,
		},
		Actions: map[string]sdk.Handler{
			"account.checkin": owner.checkin,
		},
	}
}

func Manifest() sdk.Manifest { return NewModule(ModuleOptions{}).Manifest }

func coreManifest() sdk.Manifest {
	entry := "lib/codearts.so"
	if runtime.GOOS == "darwin" {
		entry = "lib/codearts.dylib"
	}
	if runtime.GOOS == "windows" {
		entry = "lib/codearts.dll"
	}
	maxConc := AccountMaxInflight
	return sdk.Manifest{
		Schema:  1,
		ID:      ModuleID,
		Version: Version,
		Runtime: "native",
		Entry:   entry,
		Contract: sdk.Contract{
			Version: sdk.APIVersion{Major: 1, Minor: 12},
			Capabilities: []sdk.Capability{
				sdk.AccountAuth,
				sdk.OutboundExecutor,
				sdk.ModelDiscovery,
				sdk.UsageReporter,
				sdk.RequestPolicy,
				sdk.ProviderSignIn,
				sdk.ProviderSavedSignIn,
				sdk.ModuleRPC,
			},
		},
		Compatibility: &sdk.Compatibility{HostAPI: "^1.12", NativeABI: 1},
		Actions: []sdk.ActionDescriptor{
			{
				ID:          "account.checkin",
				Effect:      "write",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"day":{"type":"string"},"proxy":{"type":"string"}},"additionalProperties":false}`),
			},
		},
		AuthBindings: []sdk.WindowAuthBindingDescriptor{
			{
				ID:          "provider.signin." + CodeArtsProvider,
				From:        sdk.ProviderSignInPart,
				Provider:    CodeArtsProvider,
				Reauthorize: true,
				Methods: []sdk.WindowAuthMethod{
					{ID: "oauth", Mode: "poll", Label: "Sign in with Huawei Cloud (China)"},
					{ID: "oauth-intl", Mode: "poll", Label: "Sign in with Huawei Cloud (International)"},
					{
						ID:    sdk.SavedSignInMethod,
						Mode:  "poll",
						Label: "Import saved CodeArts sign-in",
						Fields: []sdk.AuthInputField{
							{Key: "record", Label: "Saved sign-in", Type: "secret"},
						},
					},
				},
			},
			{
				ID:          "provider.signin." + OfficeAceProvider,
				From:        sdk.ProviderSignInPart,
				Provider:    OfficeAceProvider,
				Reauthorize: true,
				Methods: []sdk.WindowAuthMethod{
					{ID: "oauth", Mode: "poll", Label: "Sign in with Huawei Cloud (OfficeAce)"},
					{
						ID:    sdk.SavedSignInMethod,
						Mode:  "poll",
						Label: "Import saved OfficeAce sign-in",
						Fields: []sdk.AuthInputField{
							{Key: "record", Label: "Saved sign-in", Type: "secret"},
						},
					},
				},
			},
		},
		Providers: []sdk.ProviderDescriptor{
			{
				ID:             CodeArtsProvider,
				Name:           "CodeArts (码道)",
				Protocols:      []protocolext.Protocol{protocolext.Chat},
				Models:         codeArtsFallbackModels(),
				Checkin:        &sdk.AccountCheckinDescriptor{Action: "account.checkin"},
				KeepAlive:      &sdk.AccountKeepAliveDescriptor{LeadMs: RefreshLeadMs},
				MaxConcurrency: &maxConc,
			},
			{
				ID:        OfficeAceProvider,
				Name:      "OfficeAce (AgentArts)",
				Protocols: []protocolext.Protocol{protocolext.Chat},
				Models:    officeAceStaticModels(),
				Checkin:   &sdk.AccountCheckinDescriptor{Action: "account.checkin"},
				KeepAlive: &sdk.AccountKeepAliveDescriptor{LeadMs: OARefreshLeadMs},
			},
		},
	}
}

func (owner *module) stationOf(cred *CodeArtsCredential) Station {
	id := "mainland"
	if cred != nil && strings.TrimSpace(cred.Station) == "international" {
		id = "international"
	}
	if st, ok := owner.options.Endpoints.Stations[id]; ok {
		return st
	}
	return DefaultStations()["mainland"]
}

func (owner *module) stateFor(key string) *accountState {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if st, ok := owner.accounts[key]; ok {
		return st
	}
	st := &accountState{
		noBenefit:     map[string]bool{},
		aliasRefusals: map[string]bool{},
		gate:          make(chan struct{}, AccountMaxInflight),
	}
	owner.accounts[key] = st
	return st
}

func (owner *module) thinkingDefault() bool {
	if owner.options.Thinking != nil {
		return *owner.options.Thinking
	}
	return true
}

func (owner *module) doHTTP(ctx context.Context, stream *sdk.Stream, req *http.Request) (*http.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if stream == nil {
		return nil, denied("Selected host network policy is required")
	}
	var proxy sdk.NetworkProxyResult
	if err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: req.URL.String()}, &proxy); err != nil {
		return nil, err
	}
	client, cleanup, err := owner.options.HTTPFactory(proxy.URL)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		cleanup()
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		cleanup()
		return nil, errors.New("empty HTTP response")
	}
	resp.Body = &closingBody{ReadCloser: resp.Body, cleanup: cleanup}
	return resp, nil
}

type closingBody struct {
	io.ReadCloser
	cleanup func()
	once    sync.Once
}

func (c *closingBody) Close() error {
	err := c.ReadCloser.Close()
	c.once.Do(func() {
		if c.cleanup != nil {
			c.cleanup()
		}
	})
	return err
}

func invalid(message string) error {
	return &sdk.Error{Code: "invalid_argument", Message: message, Outcome: "not-started"}
}

func denied(message string) error {
	return &sdk.Error{Code: "permission_denied", Message: message, Outcome: "not-started"}
}

func unauthenticated(message string) error {
	return &sdk.Error{Code: "unauthenticated", Message: message, Outcome: "not-started"}
}

func uncertain(message string) error {
	return &sdk.Error{Code: "unavailable", Message: message, Outcome: "uncertain"}
}

func nonempty(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null" && trimmed != "{}"
}

func decodeObject(raw json.RawMessage, target any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(raw) > sdk.MaxNativeBodyBytes || len(trimmed) == 0 || trimmed[0] != '{' {
		return invalid("Expected a bounded JSON object")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	if dec.Decode(target) != nil || dec.Decode(new(any)) != io.EOF {
		return invalid("Invalid JSON object")
	}
	return nil
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

func firstOf(values ...string) string {
	for _, val := range values {
		if trimmed := strings.TrimSpace(val); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

var beijingLoc = time.FixedZone("CST", 8*3600)

func beijingDay(ms int64) string {
	return time.UnixMilli(ms).In(beijingLoc).Format("2006-01-02")
}

func nextBeijingMidnight(ms int64) int64 {
	t := time.UnixMilli(ms).In(beijingLoc)
	next := time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, beijingLoc)
	return next.UnixMilli()
}
