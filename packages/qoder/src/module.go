package qoder

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

const (
	ModuleID = "qoder"
	Version  = "0.1.0"

	ProviderQoder              = "qoder"
	ProviderQoderCN            = "qoder-cn"
	ProviderQwenWork           = "qwenwork"
	ProviderQwenWorkEnterprise = "qwenwork-enterprise"

	chatPath        = "/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1"
	modelsPath      = "/algo/api/v2/model/list?Encode=1"
	queueStatusPath = "/algo/api/v2/service/ask/queue/status"
	campaignsPath   = "/sash/api/v1/me/campaigns"
	usagePath       = "/sash/api/v2/me/usage"

	cosyVersionDefault = "1.1.69"
	lingmaVersion      = "2.5.18"
	qwenworkVersion    = "1.1.59"
	qwenworkAppVersion = "1.2.1"
	qwenworkAppRelease = "1.2.1-26092107"

	signInTimeoutMs    = 15 * 60 * 1000
	refreshLeadMs      = 5 * 60 * 1000
	renewLeadMs        = 10 * 60 * 1000
	keepSpentMs        = 60 * 60 * 1000
	reloginGapMs       = 5 * 60 * 1000
	dayMs              = 24 * 60 * 60 * 1000
	lingmaLeadMs       = 5 * 60 * 1000
	campaignMachineTTL = 60 * 60 * 1000
	openRoomMs         = 60 * 1000
	pollIntervalMs     = 2000

	qoderSystemPrompt    = "You are a Qoder agent. Use the instructions below and the tools available to you to assist the user."
	qwenworkSystemPrompt = "You are QwenWork (qw), a helpful assistant. Use the instructions below and the tools available to you to assist the user."

	sessionHeader  = "x-magpie-qoder-session"
	fallbackHeader = "x-magpie-fallback"

	taskMaxEntries = 512
	taskIdleMs     = 2 * 60 * 60 * 1000
)

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type HTTPFactory func(proxyURL string) (Doer, func(), error)

type RuntimeInfoRunner func(ctx context.Context, binaryPath string, args []string, stdinData []byte) ([]byte, error)

type EndpointSet struct {
	Web     string
	OpenAPI string
	API     string
}

type Endpoints struct {
	Qoder              EndpointSet
	QoderCN            EndpointSet
	QwenWork           EndpointSet
	QwenWorkEnterprise EndpointSet
}

type QueueTimings struct {
	Budget     time.Duration
	Held       time.Duration
	Beat       time.Duration
	Shortest   time.Duration
	Longest    time.Duration
	Unsaid     time.Duration
	Recoveries int
	Failures   int
	Ask        time.Duration
	Silent     time.Duration
	Stuck      time.Duration
}

func defaultQueueTimings() QueueTimings {
	return QueueTimings{
		Budget:     4 * time.Minute,
		Held:       60 * time.Minute,
		Beat:       10 * time.Second,
		Shortest:   500 * time.Millisecond,
		Longest:    30 * time.Second,
		Unsaid:     30 * time.Second,
		Recoveries: 10,
		Failures:   3,
		Ask:        30 * time.Second,
		Silent:     120 * time.Second,
		Stuck:      5 * time.Minute,
	}
}

type ModuleOptions struct {
	Now            func() time.Time
	HTTPFactory    HTTPFactory
	Endpoints      Endpoints
	Platform       string
	Arch           string
	Hostname       func() string
	HomeDir        func() string
	Env            func(string) string
	OptDir         string
	RunRuntimeInfo RuntimeInfoRunner
	Queue          *QueueTimings
	SignInTimeout  time.Duration
}

type StaticModel struct {
	ID      string
	Name    string
	Context int64
	Images  bool
	Efforts []string
}

type Site struct {
	ID                  string
	Name                string
	ClientID            string
	Web                 string
	OpenAPI             string
	API                 string
	Redirect            string
	DeviceChat          bool
	DeviceOnly          bool
	DeviceRefreshTarget string
	VPCDomain           string
	VPC                 string
	Dedicated           string
	EnterpriseDomain    string
	Lingma              bool
	ClientType          string
	CosyVersion         string
	BusinessProduct     string
	SessionType         string
	BusinessModelKey    bool
	ModelScene          string
	ListAllModels       bool
	UserInfoPath        string
	UsagePath           string
	WalletsPath         string
	RSAKey              string
	SystemPrompt        string
	PAT                 bool
	Checkin             bool
	QwenWorkHeaders     bool
	Models              []StaticModel
	FallbackListing     map[string][]map[string]any
}

type spentEntry struct {
	Got Credential
	At  int64
}

type cachedListing struct {
	Models   []ModelEntry
	Off      map[string]bool
	FreeView bool
}

type runtimeIdentityCache struct {
	At      int64
	Tried   int64
	Headers map[string]string
}

type runtimeIdentityFlight struct {
	At      int64
	Done    chan struct{}
	Headers map[string]string
	Err     error
}

type taskRecord struct {
	Key      string
	ID       string
	Begin    int64
	Answered bool
	Name     string
	Used     int64
}

type providerRuntimeState struct {
	authMu      sync.Mutex
	stateMu     sync.Mutex
	spent       map[string]spentEntry
	listings    map[string]*cachedListing
	served      map[string]bool
	stillFree   map[string]bool
	unsaid      map[string]bool
	identities  map[string]*runtimeIdentityCache
	idFlights   map[string]*runtimeIdentityFlight
	learnedCaps map[string]int
	tasks       map[string]*taskRecord
	taskOrder   []string
	bgWG        sync.WaitGroup
}

func newProviderRuntimeState(site Site) *providerRuntimeState {
	served := map[string]bool{}
	for _, item := range site.Models {
		served[item.ID] = true
	}
	return &providerRuntimeState{
		spent:       map[string]spentEntry{},
		listings:    map[string]*cachedListing{},
		served:      served,
		stillFree:   map[string]bool{},
		unsaid:      map[string]bool{},
		identities:  map[string]*runtimeIdentityCache{},
		idFlights:   map[string]*runtimeIdentityFlight{},
		learnedCaps: map[string]int{},
		tasks:       map[string]*taskRecord{},
	}
}

type authFlow struct {
	Site      Site
	Method    string
	Verifier  string
	Nonce     string
	MachineID string
	UntilMs   int64
	PAT       string
	Dedicated string
	Lingma    *lingmaCachedAccount
	Origin    string
}

type module struct {
	options ModuleOptions
	queue   QueueTimings
	sites   map[string]Site
	states  map[string]*providerRuntimeState
	flowMu  sync.Mutex
	flows   map[string]*authFlow
	ctxMu   sync.RWMutex
	lifeCtx context.Context
	cancel  context.CancelFunc
}

func defaultHTTP(proxyURL string) (Doer, func(), error) {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if strings.TrimSpace(proxyURL) != "" {
		parsed, err := url.Parse(strings.TrimSpace(proxyURL))
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

func applyEndpointSet(site Site, override EndpointSet) Site {
	if strings.TrimSpace(override.Web) != "" {
		site.Web = strings.TrimRight(strings.TrimSpace(override.Web), "/")
	}
	if strings.TrimSpace(override.OpenAPI) != "" {
		site.OpenAPI = strings.TrimRight(strings.TrimSpace(override.OpenAPI), "/")
	}
	if strings.TrimSpace(override.API) != "" {
		site.API = strings.TrimRight(strings.TrimSpace(override.API), "/")
	}
	return site
}

func defaultSites(endpoints Endpoints) map[string]Site {
	qoderSite := applyEndpointSet(Site{
		ID:               ProviderQoder,
		Name:             "Qoder",
		ClientID:         "732aef47-9cf2-46a2-95fe-4cebb5d0d1fa",
		Web:              "https://qoder.com",
		OpenAPI:          "https://openapi.qoder.sh",
		API:              "https://api3.qoder.sh",
		Redirect:         "",
		ClientType:       "10",
		CosyVersion:      cosyVersionDefault,
		BusinessProduct:  "app",
		SessionType:      "app",
		BusinessModelKey: true,
		UserInfoPath:     "/api/v1/userinfo",
		RSAKey:           qoderRSAPublicKey,
		SystemPrompt:     qoderSystemPrompt,
		PAT:              true,
		Checkin:          true,
		Models:           defaultQoderModels(),
	}, endpoints.Qoder)

	qoderCNSite := applyEndpointSet(Site{
		ID:               ProviderQoderCN,
		Name:             "Qoder CN",
		ClientID:         "e883ade2-e6e3-4d6d-adf7-f92ceff5fdcb",
		Web:              "https://qoder.cn",
		OpenAPI:          "https://openapi.qoder.com.cn",
		API:              "https://gateway.qoder.com.cn",
		Redirect:         "",
		DeviceChat:       true,
		VPCDomain:        "vpc.qoder.com.cn",
		ClientType:       "10",
		CosyVersion:      cosyVersionDefault,
		BusinessProduct:  "app",
		SessionType:      "app",
		BusinessModelKey: true,
		UserInfoPath:     "/api/v1/userinfo",
		RSAKey:           qoderRSAPublicKey,
		SystemPrompt:     qoderSystemPrompt,
		PAT:              true,
		Checkin:          true,
		Models:           defaultQoderCNModels(),
	}, endpoints.QoderCN)

	qwenCommon := Site{
		ClientID:            "e883ade2-e6e3-4d6d-adf7-f92ceff5fdcb",
		Web:                 "https://gateway.qwenwork.cn",
		OpenAPI:             "https://gateway.qwenwork.cn",
		API:                 "https://gateway.qwenwork.cn",
		Redirect:            "qwenwork-cn://",
		DeviceOnly:          true,
		DeviceRefreshTarget: "c",
		ClientType:          "6",
		CosyVersion:         qwenworkVersion,
		BusinessProduct:     "qoder_work",
		SessionType:         "qoder_work",
		BusinessModelKey:    false,
		ModelScene:          "qwork",
		ListAllModels:       true,
		UserInfoPath:        "/api/v1/adapter/user/account-context?include=user,plan,quota,page,data_sharing",
		UsagePath:           "/api/v1/adapter/user/account-context?include=user,plan,quota",
		RSAKey:              qwenworkRSAPublicKey,
		SystemPrompt:        qwenworkSystemPrompt,
		PAT:                 false,
		Checkin:             false,
		QwenWorkHeaders:     true,
		Models:              defaultQwenWorkModels(),
	}

	qwenWorkSite := qwenCommon
	qwenWorkSite.ID = ProviderQwenWork
	qwenWorkSite.Name = "QwenWork"
	qwenWorkSite.WalletsPath = "/api/v1/adapter/user/wallets"
	qwenWorkSite = applyEndpointSet(qwenWorkSite, endpoints.QwenWork)

	qwenWorkEntSite := qwenCommon
	qwenWorkEntSite.ID = ProviderQwenWorkEnterprise
	qwenWorkEntSite.Name = "QwenWork 企业版"
	qwenWorkEntSite.FallbackListing = qwenworkFallbackListing()
	qwenWorkEntSite = applyEndpointSet(qwenWorkEntSite, endpoints.QwenWorkEnterprise)

	return map[string]Site{
		ProviderQoder:              qoderSite,
		ProviderQoderCN:            qoderCNSite,
		ProviderQwenWork:           qwenWorkSite,
		ProviderQwenWorkEnterprise: qwenWorkEntSite,
	}
}

func NewModule(options ModuleOptions) sdk.Module {
	owner := newOwner(options)
	return sdk.Module{
		Manifest: coreManifest(owner.sites, owner.envValue("QWENWORK_ENT_DOMAIN")),
		Start:    owner.start,
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

func newOwner(options ModuleOptions) *module {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.HTTPFactory == nil {
		options.HTTPFactory = defaultHTTP
	}
	if options.Platform == "" {
		if runtime.GOOS == "windows" {
			options.Platform = "win32"
		} else {
			options.Platform = runtime.GOOS
		}
	}
	if options.Arch == "" {
		switch runtime.GOARCH {
		case "amd64":
			options.Arch = "x64"
		case "arm64":
			options.Arch = "arm64"
		default:
			options.Arch = runtime.GOARCH
		}
	}
	if options.Hostname == nil {
		options.Hostname = func() string {
			name, err := os.Hostname()
			if err != nil {
				return ""
			}
			return name
		}
	}
	if options.HomeDir == nil {
		options.HomeDir = func() string {
			home, err := os.UserHomeDir()
			if err != nil {
				return ""
			}
			return home
		}
	}
	if options.Env == nil {
		options.Env = os.Getenv
	}
	if options.OptDir == "" {
		options.OptDir = "/opt"
	}
	if options.SignInTimeout <= 0 {
		options.SignInTimeout = time.Duration(signInTimeoutMs) * time.Millisecond
	}
	queueCfg := defaultQueueTimings()
	if options.Queue != nil {
		queueCfg = mergeQueueTimings(queueCfg, *options.Queue)
	}
	sites := defaultSites(options.Endpoints)
	states := map[string]*providerRuntimeState{}
	for id, site := range sites {
		states[id] = newProviderRuntimeState(site)
	}
	return &module{
		options: options,
		queue:   queueCfg,
		sites:   sites,
		states:  states,
		flows:   map[string]*authFlow{},
	}
}

func mergeQueueTimings(base QueueTimings, override QueueTimings) QueueTimings {
	if override.Budget > 0 {
		base.Budget = override.Budget
	}
	if override.Held > 0 {
		base.Held = override.Held
	}
	if override.Beat > 0 {
		base.Beat = override.Beat
	}
	if override.Shortest > 0 {
		base.Shortest = override.Shortest
	}
	if override.Longest > 0 {
		base.Longest = override.Longest
	}
	if override.Unsaid > 0 {
		base.Unsaid = override.Unsaid
	}
	if override.Recoveries > 0 {
		base.Recoveries = override.Recoveries
	}
	if override.Failures > 0 {
		base.Failures = override.Failures
	}
	if override.Ask > 0 {
		base.Ask = override.Ask
	}
	if override.Silent > 0 {
		base.Silent = override.Silent
	}
	if override.Stuck > 0 {
		base.Stuck = override.Stuck
	}
	return base
}

func Manifest() sdk.Manifest { return NewModule(ModuleOptions{}).Manifest }

func coreManifest(sites map[string]Site, presetEntRaw string) sdk.Manifest {
	entry := "lib/qoder.so"
	if runtime.GOOS == "darwin" {
		entry = "lib/qoder.dylib"
	}
	if runtime.GOOS == "windows" {
		entry = "lib/qoder.dll"
	}
	qoderSite := sites[ProviderQoder]
	qoderCNSite := sites[ProviderQoderCN]
	qwenWorkSite := sites[ProviderQwenWork]
	qwenWorkEntSite := sites[ProviderQwenWorkEnterprise]

	presetEnt := enterpriseDomain(presetEntRaw)
	entLabel := fmt.Sprintf("Sign in with %s（输入企业域名）", qwenWorkEntSite.Name)
	entFields := []sdk.AuthInputField{
		{
			Key:      "domain",
			Label:    "Enterprise domain (e.g. your-company.vpc.qwenwork.cn)",
			Type:     "text",
			Optional: false,
		},
	}
	if presetEnt != "" {
		entLabel = fmt.Sprintf("Sign in with %s (%s)", qwenWorkEntSite.Name, presetEnt)
		entFields = []sdk.AuthInputField{
			{
				Key:      "domain",
				Label:    "Enterprise domain override",
				Type:     "text",
				Optional: true,
			},
		}
	}

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
				sdk.ProviderSignIn,
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
				ID:          "provider.signin." + ProviderQoder,
				From:        sdk.ProviderSignInPart,
				Provider:    ProviderQoder,
				Reauthorize: true,
				Methods: []sdk.WindowAuthMethod{
					{ID: "browser", Mode: "poll", Label: fmt.Sprintf("Sign in with %s", qoderSite.Name)},
					{
						ID:    "pat",
						Mode:  "poll",
						Label: fmt.Sprintf("Sign in with %s PAT", qoderSite.Name),
						Fields: []sdk.AuthInputField{
							{Key: "pat", Label: "Personal Access Token (pt-...)", Type: "secret"},
						},
					},
				},
			},
			{
				ID:          "provider.signin." + ProviderQoderCN,
				From:        sdk.ProviderSignInPart,
				Provider:    ProviderQoderCN,
				Reauthorize: true,
				Methods: []sdk.WindowAuthMethod{
					{ID: "browser", Mode: "poll", Label: fmt.Sprintf("Sign in with %s", qoderCNSite.Name)},
					{
						ID:    "pat",
						Mode:  "poll",
						Label: fmt.Sprintf("Sign in with %s PAT", qoderCNSite.Name),
						Fields: []sdk.AuthInputField{
							{Key: "pat", Label: "Personal Access Token (pt-...)", Type: "secret"},
						},
					},
					{
						ID:    "vpc",
						Mode:  "poll",
						Label: fmt.Sprintf("Sign in with %s Enterprise (VPC)", qoderCNSite.Name),
						Fields: []sdk.AuthInputField{
							{Key: "vpc", Label: "Enterprise VPC instance", Type: "text"},
						},
					},
					{
						ID:    "dedicated",
						Mode:  "poll",
						Label: fmt.Sprintf("Sign in with %s Enterprise dedicated domain (企业版专属域)", qoderCNSite.Name),
						Fields: []sdk.AuthInputField{
							{Key: "dedicated", Label: "Enterprise dedicated domain", Type: "text"},
						},
					},
				},
			},
			{
				ID:          "provider.signin." + ProviderQwenWork,
				From:        sdk.ProviderSignInPart,
				Provider:    ProviderQwenWork,
				Reauthorize: true,
				Methods: []sdk.WindowAuthMethod{
					{ID: "browser", Mode: "poll", Label: fmt.Sprintf("Sign in with %s", qwenWorkSite.Name)},
				},
			},
			{
				ID:          "provider.signin." + ProviderQwenWorkEnterprise,
				From:        sdk.ProviderSignInPart,
				Provider:    ProviderQwenWorkEnterprise,
				Reauthorize: true,
				Methods: []sdk.WindowAuthMethod{
					{
						ID:     "enterprise",
						Mode:   "poll",
						Label:  entLabel,
						Fields: entFields,
					},
				},
			},
		},
		Providers: []sdk.ProviderDescriptor{
			{
				ID:        ProviderQoder,
				Name:      qoderSite.Name,
				Protocols: []protocolext.Protocol{protocolext.Chat},
				Models:    fallbackSDKModels(qoderSite),
				Checkin:   &sdk.AccountCheckinDescriptor{Action: "account.checkin"},
			},
			{
				ID:        ProviderQoderCN,
				Name:      qoderCNSite.Name,
				Protocols: []protocolext.Protocol{protocolext.Chat},
				Models:    fallbackSDKModels(qoderCNSite),
				Checkin:   &sdk.AccountCheckinDescriptor{Action: "account.checkin"},
			},
			{
				ID:        ProviderQwenWork,
				Name:      qwenWorkSite.Name,
				Protocols: []protocolext.Protocol{protocolext.Chat},
				Models:    fallbackSDKModels(qwenWorkSite),
			},
			{
				ID:        ProviderQwenWorkEnterprise,
				Name:      qwenWorkEntSite.Name,
				Protocols: []protocolext.Protocol{protocolext.Chat},
				Models:    fallbackSDKModels(qwenWorkEntSite),
			},
		},
	}
}

func (owner *module) start(ctx context.Context, _ sdk.ModuleContext) (sdk.ModuleReady, func(), error) {
	lifeCtx, cancel := context.WithCancel(ctx)
	owner.ctxMu.Lock()
	owner.lifeCtx = lifeCtx
	owner.cancel = cancel
	owner.ctxMu.Unlock()
	return sdk.ModuleReady{}, func() {
		cancel()
		for _, state := range owner.states {
			state.bgWG.Wait()
		}
	}, nil
}

func (owner *module) backgroundContext() context.Context {
	owner.ctxMu.RLock()
	defer owner.ctxMu.RUnlock()
	if owner.lifeCtx != nil {
		return owner.lifeCtx
	}
	return context.Background()
}

func (owner *module) siteFor(provider string) (Site, *providerRuntimeState, error) {
	site, ok := owner.sites[provider]
	if !ok {
		return Site{}, nil, invalid(fmt.Sprintf("unknown qoder provider %q", provider))
	}
	return site, owner.states[provider], nil
}

func (owner *module) now() time.Time { return owner.options.Now() }
func (owner *module) nowMs() int64   { return owner.now().UnixMilli() }

func (owner *module) envValue(key string) string {
	if owner.options.Env == nil {
		return ""
	}
	return owner.options.Env(key)
}

func (owner *module) doHTTP(ctx context.Context, stream *sdk.Stream, proxyOverride string, req *http.Request) (*http.Response, func(), error) {
	proxyURL := strings.TrimSpace(proxyOverride)
	if proxyURL == "" && stream != nil {
		var resolved sdk.NetworkProxyResult
		if err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: req.URL.String()}, &resolved); err == nil {
			proxyURL = strings.TrimSpace(resolved.URL)
		}
	}
	client, cleanup, err := owner.options.HTTPFactory(proxyURL)
	if err != nil {
		return nil, nil, upstreamErr(err.Error())
	}
	if cleanup == nil {
		cleanup = func() {}
	}
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return resp, cleanup, nil
}

func readBodyLimited(body io.Reader, limit int64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	return io.ReadAll(io.LimitReader(body, limit))
}

func invalid(message string) error {
	return &sdk.Error{Code: "invalid_request", Message: message, Outcome: "not-started"}
}

func unauthenticated(message string) error {
	return &sdk.Error{Code: "unauthenticated", Message: message, Outcome: "not-started"}
}

func upstreamErr(message string) error {
	return &sdk.Error{Code: "upstream", Message: message, Outcome: "not-started"}
}

func randomUUID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		now := time.Now().UnixNano()
		for idx := range raw {
			raw[idx] = byte(now >> uint((idx%8)*8))
		}
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(raw[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

func hexID() string {
	return strings.ReplaceAll(randomUUID(), "-", "")
}

func firstNonEmpty(values ...string) string {
	for _, val := range values {
		if trimmed := strings.TrimSpace(val); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func nonemptyRaw(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func strField(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	raw, ok := obj[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func rawStrField(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	val, ok := obj[key].(string)
	if !ok {
		return ""
	}
	return val
}

func cloneHeaders(hdr http.Header) http.Header {
	out := http.Header{}
	for key, values := range hdr {
		copied := make([]string, len(values))
		copy(copied, values)
		out[key] = copied
	}
	return out
}

func ptrFloat(val float64) *float64 {
	copyVal := val
	return &copyVal
}

func ptrBool(val bool) *bool {
	copyVal := val
	return &copyVal
}
