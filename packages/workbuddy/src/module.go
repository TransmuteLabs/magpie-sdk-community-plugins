package workbuddy

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

const (
	ModuleID          = "workbuddy"
	ProviderWorkBuddy = "workbuddy"
	ProviderAI        = "workbuddy-ai"
	Version           = "0.1.0"

	DefaultCNEndpoint = "https://copilot.tencent.com"
	DefaultAIEndpoint = "https://www.workbuddy.ai"

	AppVersion   = "5.9.3"
	CLIVersion   = "2.165.0"
	AgentRuntime = "local-agent"
	OpenAISDKVer = "6.25.0"
	NodeRuntime  = "v22.21.1"

	PollIntervalMs = 1000
	SignInMs       = 5 * 60 * 1000
	EarlyMs        = 60 * 1000
	LeadMs         = 5 * 60 * 1000
	KeepRenewedMs  = 60 * 1000
	PriceTTLMs     = 60 * 1000
	EdgeReadsBytes = 2 * 1024 * 1024
	SmallBudget    = 1024
	TailBytes      = 16384

	SessionHeader = "x-magpie-workbuddy-session"
	FreeOnlyKey   = "free_only"
	HideDSHKey    = "hide_dsh"
	AIFreeOnlyKey = "ai_free_only"
	AIHideDSHKey  = "ai_hide_dsh"

	RefusedDoc      = "https://github.com/magpie-community/plugins/blob/main/packages/workbuddy/REFUSED.md"
	RefusedHint     = "WorkBuddy refused words in this client's prompt (most often its system prompt); use it from Hermes, OpenCode or Pi, or add another provider to this group"
	RestrictedHint  = "WorkBuddy restricted this account for unusual activity; it stays out until WorkBuddy lifts that, which an appeal (Submit feedback in WorkBuddy) asks for"
	ExhaustedHint   = "WorkBuddy requires an active credit package even for 0-rate free models; check your account package balance"
	RestrictedCode  = 11140
	ExhaustedCode   = 14018
	DefaultSystem   = "You are a helpful assistant."
	CallTextSpace   = " "
	GrowthSendEvent = "chat_request_send"
)

type staticModelSpec struct {
	ID      string
	Name    string
	Context int64
	Efforts []string
	Output  int64
}

type Site struct {
	ID       string
	Name     string
	Endpoint string
	AuthID   string
	Platform string
	AI       bool
	App      string
	CLI      string
	AppName  string
	Runtime  string
	Keys     string
	Models   []staticModelSpec
	Messages bool
	NoImages *regexp.Regexp
}

var (
	cloudHostedPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^[a-z0-9-]+\.sso\.copilot\.tencent\.com$`),
		regexp.MustCompile(`^[a-z0-9-]+\.copilot\.qq\.com$`),
		regexp.MustCompile(`^[a-z0-9-]+\.sso\.codebuddy\.cn$`),
	}
	ioaHosts = map[string]bool{
		"tencent.sso.copilot.tencent.com": true,
		"tencent.sso.codebuddy.cn":        true,
	}
	aiGLMNoImages = regexp.MustCompile(`^glm-`)
	claudeModelRe = regexp.MustCompile(`(?i)^claude-`)
	sdkOSNames    = map[string]string{
		"darwin":  "MacOS",
		"windows": "Windows",
		"win32":   "Windows",
		"linux":   "Linux",
		"freebsd": "FreeBSD",
		"openbsd": "OpenBSD",
		"android": "Android",
	}
	sdkArchNames = map[string]string{
		"amd64": "x64",
		"x64":   "x64",
		"arm64": "arm64",
		"arm":   "arm",
		"386":   "x32",
		"ia32":  "x32",
		"x32":   "x32",
	}
)

func defaultSiteCN(endpoint string) Site {
	if endpoint == "" {
		endpoint = DefaultCNEndpoint
	}
	return Site{
		ID:       ProviderWorkBuddy,
		Name:     "WorkBuddy",
		Endpoint: strings.TrimRight(endpoint, "/"),
		AuthID:   "workbuddy-desktop",
		Platform: "workbuddy",
		AI:       false,
		App:      AppVersion,
		CLI:      CLIVersion,
		AppName:  "WorkBuddy",
		Runtime:  AgentRuntime,
		Keys:     "https://www.workbuddy.cn/profile/keys",
		Models:   cnModels,
		Messages: true,
	}
}

func defaultSiteAI(endpoint string) Site {
	if endpoint == "" {
		endpoint = DefaultAIEndpoint
	}
	return Site{
		ID:       ProviderAI,
		Name:     "WorkBuddy AI",
		Endpoint: strings.TrimRight(endpoint, "/"),
		AuthID:   "workbuddy-desktop-ai",
		Platform: "workbuddy-ai",
		AI:       true,
		App:      AppVersion,
		CLI:      CLIVersion,
		AppName:  "workbuddy-ai",
		Runtime:  AgentRuntime,
		Keys:     "https://www.workbuddy.ai/profile/keys",
		Models:   aiModels,
		Messages: false,
		NoImages: aiGLMNoImages,
	}
}

type Endpoints struct {
	CN string
	AI string
}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type HTTPFactory func(proxyURL string) (Doer, func(), error)

type ModuleOptions struct {
	Endpoints    Endpoints
	Now          func() time.Time
	HTTPFactory  HTTPFactory
	Platform     string
	Arch         string
	HomeDir      func() string
	WorkDir      func() string
	Env          func(string) string
	DesktopPath  func(Site) string
	CheckinWaits []time.Duration
	PollInterval time.Duration
	SignInWindow time.Duration
	SyncReports  bool
	FreeOnlyCN   *bool
	HideDSHCN    *bool
	FreeOnlyAI   *bool
	HideDSHAI    *bool
	Salt         string
	MachineID    string
}

type moduleConfig struct {
	FreeOnly   *bool `json:"free_only,omitempty"`
	HideDSH    *bool `json:"hide_dsh,omitempty"`
	AIFreeOnly *bool `json:"ai_free_only,omitempty"`
	AIHideDSH  *bool `json:"ai_hide_dsh,omitempty"`
}

type renewResult struct {
	next Credential
	at   int64
}

type renewFlight struct {
	done chan struct{}
	next Credential
	err  error
}

type modelListCache struct {
	at   int64
	list []liveModel
}

type authFlow struct {
	site     Site
	method   string
	state    string
	authURL  string
	deadline int64
	apiKey   string
	record   string
}

type siteRuntimeState struct {
	mu             sync.Mutex
	endsAfterTools map[string]bool
	refusedSystems []string
	refusedSet     map[string]bool
	commaSystems   []string
	commaSet       map[string]bool
	lists          map[string]modelListCache
	listOrder      []string
	unlisted       map[string]int64
	unlistedOrder  []string
}

func newSiteRuntimeState() *siteRuntimeState {
	return &siteRuntimeState{
		endsAfterTools: map[string]bool{},
		refusedSet:     map[string]bool{},
		commaSet:       map[string]bool{},
		lists:          map[string]modelListCache{},
		unlisted:       map[string]int64{},
	}
}

type module struct {
	options       ModuleOptions
	sites         map[string]Site
	salt          string
	reportSession string
	machineOnce   sync.Once
	machineID     string

	mu             sync.Mutex
	renewing       map[string]*renewFlight
	renewed        map[string]renewResult
	desktopHeld    map[string]Credential
	flows          map[string]*authFlow
	siteState      map[string]*siteRuntimeState
	ledgerMu       sync.Mutex
	ledgerParsed   *ledgerCacheEntry
	ledgerBad      ledgerBadState
	routingMu      sync.Mutex
	routingParsed  map[string]*routingCacheEntry
	routingReading map[string]*routingReadFlight
	routingReads   int64

	reportWG sync.WaitGroup
}

func defaultHTTP(proxyURL string) (Doer, func(), error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL != "" {
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
	owner := newOwner(options)
	return sdk.Module{
		Manifest: coreManifest(owner.sites[ProviderWorkBuddy], owner.sites[ProviderAI]),
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
		options.Arch = runtime.GOARCH
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
	if options.WorkDir == nil {
		options.WorkDir = func() string {
			wd, err := os.Getwd()
			if err != nil {
				return ""
			}
			return wd
		}
	}
	if options.Env == nil {
		options.Env = os.Getenv
	}
	if options.PollInterval <= 0 {
		options.PollInterval = time.Duration(PollIntervalMs) * time.Millisecond
	}
	if options.SignInWindow <= 0 {
		options.SignInWindow = time.Duration(SignInMs) * time.Millisecond
	}
	if options.CheckinWaits == nil {
		options.CheckinWaits = []time.Duration{2 * time.Second, 4 * time.Second}
	}
	salt := options.Salt
	if salt == "" {
		salt = randomHex(16)
	}
	siteCN := defaultSiteCN(options.Endpoints.CN)
	siteAI := defaultSiteAI(options.Endpoints.AI)
	return &module{
		options:       options,
		sites:         map[string]Site{ProviderWorkBuddy: siteCN, ProviderAI: siteAI},
		salt:          salt,
		reportSession: randomHex(16),
		machineID:     options.MachineID,
		renewing:      map[string]*renewFlight{},
		renewed:       map[string]renewResult{},
		desktopHeld:   map[string]Credential{},
		flows:         map[string]*authFlow{},
		siteState: map[string]*siteRuntimeState{
			ProviderWorkBuddy: newSiteRuntimeState(),
			ProviderAI:        newSiteRuntimeState(),
		},
		routingParsed:  map[string]*routingCacheEntry{},
		routingReading: map[string]*routingReadFlight{},
	}
}

func Manifest() sdk.Manifest { return NewModule(ModuleOptions{}).Manifest }

func providerOptionsFor(site Site, freeKey, hideKey string) []sdk.ProviderOption {
	return []sdk.ProviderOption{
		{
			Key:         freeKey,
			Label:       "Free models only",
			Description: fmt.Sprintf(`Lists only the models %s serves at no cost, and turns away a request for any other here instead of sending it, so no credits are spent. A model free only at night counts as paid.`, site.Name),
			Labels: map[string]string{
				"zh":    "仅免费模型",
				"zh-TW": "僅免費模型",
				"ja":    "無料モデルのみ",
				"de":    "Nur kostenlose Modelle",
			},
			Descriptions: map[string]string{
				"zh":    fmt.Sprintf("只列出 %s 免费提供的模型；其他模型的请求在这里就被拒绝、不会发出，因此不消耗积分。只在夜间免费的模型按付费算。", site.Name),
				"zh-TW": fmt.Sprintf("只列出 %s 免費提供的模型；其他模型的請求在這裡就被拒絕、不會送出，因此不消耗積分。只在夜間免費的模型以付費計。", site.Name),
				"ja":    fmt.Sprintf("%s が無料で提供するモデルだけを表示し、ほかのモデルへのリクエストは送信せずここで断るので、クレジットを消費しません。夜間だけ無料のモデルは有料として扱います。", site.Name),
				"de":    fmt.Sprintf("Listet nur die Modelle, die %s kostenlos anbietet, und weist eine Anfrage an jedes andere hier ab, statt sie zu senden, sodass keine Credits verbraucht werden. Ein Modell, das nur nachts kostenlos ist, gilt als kostenpflichtig.", site.Name),
			},
			FiltersModels: true,
		},
		{
			Key:         hideKey,
			Label:       "Hide DeepSeek Harness's name",
			Description: fmt.Sprintf("Takes DeepSeek Harness's name out of what it sends to %s: its opening line, and the lines naming its checkout and Web GUI. Paths, commands and tools stay as they are.", site.Name),
			Default:     true,
			Labels: map[string]string{
				"zh":    "隐去 DeepSeek Harness 的名字",
				"zh-TW": "隱去 DeepSeek Harness 的名字",
				"ja":    "DeepSeek Harness の名前を伏せる",
				"de":    "Namen von DeepSeek Harness ausblenden",
			},
			Descriptions: map[string]string{
				"zh":    fmt.Sprintf("把 DeepSeek Harness 的名字从发给 %s 的内容里去掉：开头那句，以及说明它源码目录和 Web GUI 的几行。路径、命令和工具保持不变。", site.Name),
				"zh-TW": fmt.Sprintf("把 DeepSeek Harness 的名字從送給 %s 的內容裡去掉：開頭那句，以及說明它原始碼目錄和 Web GUI 的幾行。路徑、指令和工具保持不變。", site.Name),
				"ja":    fmt.Sprintf("%s に送る内容から DeepSeek Harness の名前を外します。冒頭の一文と、チェックアウトや Web GUI を示す行が対象です。パス、コマンド、ツールはそのままです。", site.Name),
				"de":    fmt.Sprintf("Entfernt den Namen von DeepSeek Harness aus dem, was an %s gesendet wird: die erste Zeile sowie die Zeilen zu seinem Checkout und seiner Web-GUI. Pfade, Befehle und Tools bleiben unverändert.", site.Name),
			},
		},
	}
}

func coreManifest(siteCN, siteAI Site) sdk.Manifest {
	entry := "lib/workbuddy.so"
	if runtime.GOOS == "darwin" {
		entry = "lib/workbuddy.dylib"
	}
	if runtime.GOOS == "windows" {
		entry = "lib/workbuddy.dll"
	}
	authMethodsFor := func(site Site) []sdk.WindowAuthMethod {
		return []sdk.WindowAuthMethod{
			{ID: "browser", Mode: "poll", Label: fmt.Sprintf("%s account (browser)", site.Name)},
			{ID: "desktop", Mode: "poll", Label: fmt.Sprintf("%s desktop's sign-in", site.Name)},
			{
				ID:    "api-key",
				Mode:  "poll",
				Label: fmt.Sprintf("%s API key", site.Name),
				Fields: []sdk.AuthInputField{
					{Key: "apiKey", Label: "API key", Type: "secret"},
				},
			},
			{
				ID:    sdk.SavedSignInMethod,
				Mode:  "poll",
				Label: fmt.Sprintf("Import saved %s sign-in", site.Name),
				Fields: []sdk.AuthInputField{
					{Key: "record", Label: "Saved sign-in", Type: "secret"},
				},
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
				ID:          "provider.signin." + ProviderWorkBuddy,
				From:        sdk.ProviderSignInPart,
				Provider:    ProviderWorkBuddy,
				Reauthorize: true,
				Methods:     authMethodsFor(siteCN),
			},
			{
				ID:          "provider.signin." + ProviderAI,
				From:        sdk.ProviderSignInPart,
				Provider:    ProviderAI,
				Reauthorize: true,
				Methods:     authMethodsFor(siteAI),
			},
		},
		Providers: []sdk.ProviderDescriptor{
			{
				ID:        ProviderWorkBuddy,
				Name:      siteCN.Name,
				Protocols: []protocolext.Protocol{protocolext.Chat, protocolext.Messages},
				Models:    fallbackModels(siteCN),
				Checkin:   &sdk.AccountCheckinDescriptor{Action: "account.checkin"},
				KeepAlive: &sdk.AccountKeepAliveDescriptor{LeadMs: LeadMs},
				Options:   providerOptionsFor(siteCN, FreeOnlyKey, HideDSHKey),
			},
			{
				ID:        ProviderAI,
				Name:      siteAI.Name,
				Protocols: []protocolext.Protocol{protocolext.Chat},
				Models:    fallbackModels(siteAI),
				KeepAlive: &sdk.AccountKeepAliveDescriptor{LeadMs: LeadMs},
				Options:   providerOptionsFor(siteAI, AIFreeOnlyKey, AIHideDSHKey),
			},
		},
	}
}

func (owner *module) start(_ context.Context, modCtx sdk.ModuleContext) (sdk.ModuleReady, func(), error) {
	var cfg moduleConfig
	trimmed := bytes.TrimSpace(modCtx.Config.Values)
	if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
		if err := json.Unmarshal(trimmed, &cfg); err != nil {
			return sdk.ModuleReady{}, nil, invalid("invalid WorkBuddy configuration")
		}
	}
	dispose := func() {
		owner.reportWG.Wait()
	}
	return sdk.ModuleReady{State: cfg}, dispose, nil
}

func (owner *module) configOf(ctx context.Context) moduleConfig {
	if modCtx, found := sdk.ModuleContextFrom(ctx); found {
		if cfg, ok := modCtx.State.(moduleConfig); ok {
			return cfg
		}
		trimmed := bytes.TrimSpace(modCtx.Config.Values)
		if len(trimmed) > 0 && trimmed[0] == '{' {
			var cfg moduleConfig
			if json.Unmarshal(trimmed, &cfg) == nil {
				return cfg
			}
		}
	}
	return moduleConfig{}
}

func (owner *module) freeOnly(ctx context.Context, site Site) bool {
	cfg := owner.configOf(ctx)
	if site.AI {
		if cfg.AIFreeOnly != nil {
			return *cfg.AIFreeOnly
		}
		if owner.options.FreeOnlyAI != nil {
			return *owner.options.FreeOnlyAI
		}
		return false
	}
	if cfg.FreeOnly != nil {
		return *cfg.FreeOnly
	}
	if owner.options.FreeOnlyCN != nil {
		return *owner.options.FreeOnlyCN
	}
	return false
}

func (owner *module) hideDSHEnabled(ctx context.Context, site Site) bool {
	cfg := owner.configOf(ctx)
	if site.AI {
		if cfg.AIHideDSH != nil {
			return *cfg.AIHideDSH
		}
		if owner.options.HideDSHAI != nil {
			return *owner.options.HideDSHAI
		}
		return true
	}
	if cfg.HideDSH != nil {
		return *cfg.HideDSH
	}
	if owner.options.HideDSHCN != nil {
		return *owner.options.HideDSHCN
	}
	return true
}

func (owner *module) siteFor(provider string) (Site, bool) {
	site, ok := owner.sites[provider]
	return site, ok
}

func (owner *module) stateFor(site Site) *siteRuntimeState {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	st, ok := owner.siteState[site.ID]
	if !ok {
		st = newSiteRuntimeState()
		owner.siteState[site.ID] = st
	}
	return st
}

func (owner *module) nowMs() int64 {
	return owner.options.Now().UnixMilli()
}

func (owner *module) machineHash() string {
	owner.machineOnce.Do(func() {
		if owner.machineID != "" {
			return
		}
		host, _ := os.Hostname()
		uname := ""
		if usr, err := user.Current(); err == nil {
			uname = usr.Username
		}
		sum := sha256.Sum256([]byte("magpie-workbuddy|" + host + "|" + uname))
		owner.machineID = hex.EncodeToString(sum[:])
	})
	return owner.machineID
}

func (owner *module) desktopFile(site Site) string {
	if owner.options.DesktopPath != nil {
		return owner.options.DesktopPath(site)
	}
	home := owner.options.HomeDir()
	var base string
	switch owner.options.Platform {
	case "darwin":
		base = filepath.Join(home, "Library", "Application Support", "CodeBuddyExtension")
	case "win32", "windows":
		base = filepath.Join(home, "AppData", "Local", "CodeBuddyExtension")
	default:
		base = filepath.Join(home, ".local", "share", "CodeBuddyExtension")
	}
	return filepath.Join(base, "Data", "Public", "auth", site.AuthID+".info")
}

func (owner *module) doHTTP(ctx context.Context, stream *sdk.Stream, req *http.Request) (*http.Response, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	proxyURL := ""
	if stream != nil {
		var proxy sdk.NetworkProxyResult
		if err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: req.URL.String()}, &proxy); err != nil {
			return nil, "", err
		}
		proxyURL = proxy.URL
	}
	client, cleanup, err := owner.options.HTTPFactory(proxyURL)
	if err != nil {
		return nil, proxyURL, err
	}
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		cleanup()
		return nil, proxyURL, err
	}
	if resp == nil || resp.Body == nil {
		cleanup()
		return nil, proxyURL, errors.New("empty HTTP response")
	}
	resp.Body = &closingBody{ReadCloser: resp.Body, cleanup: cleanup}
	return resp, proxyURL, nil
}

type closingBody struct {
	io.ReadCloser
	cleanup func()
	once    sync.Once
}

func (body *closingBody) Close() error {
	err := body.ReadCloser.Close()
	body.once.Do(body.cleanup)
	return err
}

func nativeOf(site Site, model string) bool {
	return site.Messages && claudeModelRe.MatchString(model)
}

func textOnly(site Site, model string) bool {
	return site.NoImages != nil && site.NoImages.MatchString(model)
}

func domainOf(site Site, acct Credential) string {
	if strings.TrimSpace(acct.Domain) != "" {
		return strings.TrimSpace(acct.Domain)
	}
	parsed, err := url.Parse(site.Endpoint)
	if err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return ""
}

func endpointOf(site Site, acct Credential) string {
	dom := strings.ToLower(strings.TrimSpace(acct.Domain))
	if dom == "" || ioaHosts[dom] || acct.Source == "key" {
		return site.Endpoint
	}
	for _, pat := range cloudHostedPatterns {
		if pat.MatchString(dom) {
			return "https://" + dom
		}
	}
	return site.Endpoint
}

func siteAt(site Site, acct Credential) Site {
	end := endpointOf(site, acct)
	if end == site.Endpoint {
		return site
	}
	copySite := site
	copySite.Endpoint = end
	return copySite
}

func movedURL(site Site, acct Credential, rawURL string) string {
	end := endpointOf(site, acct)
	if end == site.Endpoint || !strings.HasPrefix(rawURL, site.Endpoint+"/") {
		return rawURL
	}
	return end + rawURL[len(site.Endpoint):]
}

func cliAgent(site Site) string {
	return fmt.Sprintf("WorkBuddy/%s %s/%s CLI/%s", site.App, site.AppName, site.App, site.CLI)
}

func enterpriseHeaders(acct Credential) map[string]string {
	entID := strings.TrimSpace(acct.EnterpriseID)
	if entID == "" {
		return nil
	}
	return map[string]string{
		"X-Enterprise-Id": entID,
		"X-Tenant-Id":     entID,
	}
}

func signHeaders(site Site, acct Credential, headers http.Header) {
	headers.Set("Authorization", "Bearer "+acct.Access)
	headers.Set("X-User-Id", acct.UID)
	if acct.Source == "key" {
		headers.Set("X-API-Key", acct.Key)
		headers.Del("X-Domain")
	} else {
		headers.Del("X-API-Key")
		headers.Set("X-Domain", domainOf(site, acct))
	}
	headers.Set("X-Product", "SaaS")
	headers.Set("X-IDE-Type", "WorkBuddy")
	headers.Set("User-Agent", cliAgent(site))
	headers.Set("X-Requested-With", "XMLHttpRequest")
	headers.Set("X-Agent-Intent", "craft")
	headers.Set("X-Agent-Purpose", "conversation")
	headers.Set("X-IDE-Name", "WorkBuddy")
	headers.Set("X-IDE-Version", site.App)
	headers.Del("X-Enterprise-Id")
	headers.Del("X-Tenant-Id")
	for key, val := range enterpriseHeaders(acct) {
		headers.Set(key, val)
	}
}

func (owner *module) chatHeaders(site Site, headers http.Header) {
	headers.Set("X-Agent-Type", "main")
	if site.Runtime != "" {
		headers.Set("X-Agent-Runtime", site.Runtime)
	} else {
		headers.Del("X-Agent-Runtime")
	}
	headers.Set("Accept", "application/json")
	headers.Set("X-Stainless-Retry-Count", "0")
	headers.Set("X-Stainless-Timeout", "1200")
	headers.Set("X-Stainless-Lang", "js")
	headers.Set("X-Stainless-Package-Version", OpenAISDKVer)
	osLabel, ok := sdkOSNames[owner.options.Platform]
	if !ok {
		if owner.options.Platform != "" {
			osLabel = "Other:" + owner.options.Platform
		} else {
			osLabel = "Unknown"
		}
	}
	archLabel, ok := sdkArchNames[owner.options.Arch]
	if !ok {
		if owner.options.Arch != "" {
			archLabel = "other:" + owner.options.Arch
		} else {
			archLabel = "unknown"
		}
	}
	headers.Set("X-Stainless-OS", osLabel)
	headers.Set("X-Stainless-Arch", archLabel)
	headers.Set("X-Stainless-Runtime", "node")
	headers.Set("X-Stainless-Runtime-Version", NodeRuntime)
}

type wbError struct {
	Message string
	Code    int
	Status  int
	Said    string
	Bare    bool
}

func (err *wbError) Error() string { return err.Message }

func statusText(code int) string {
	if text := http.StatusText(code); text != "" {
		return text
	}
	return fmt.Sprintf("HTTP %d", code)
}

func (owner *module) callAPI(ctx context.Context, stream *sdk.Stream, site Site, method, path string, extraHeaders map[string]string, body any) (map[string]any, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(reqCtx, method, site.Endpoint+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "WorkBuddy/"+site.App)
	for key, val := range extraHeaders {
		req.Header.Set(key, val)
	}
	resp, _, err := owner.doHTTP(reqCtx, stream, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	var env map[string]any
	_ = json.Unmarshal(raw, &env)
	code := parseIntCode(env["code"])
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		why := firstNonEmpty(
			strField(env, "msg", "message"),
			nestedStr(env, "error", "message"),
			truncateRunes(string(raw), 200),
			resp.Status,
		)
		said := strField(env, "msg")
		if code != 0 {
			if said == "" {
				said = fmt.Sprintf("error %d", code)
			}
		} else if said == "" {
			said = statusText(resp.StatusCode)
		}
		codeSuffix := ""
		if code != 0 {
			codeSuffix = fmt.Sprintf(", code %d", code)
		}
		return nil, &wbError{
			Message: fmt.Sprintf("%s: %s (HTTP %d%s)", site.Name, why, resp.StatusCode, codeSuffix),
			Code:    code,
			Status:  resp.StatusCode,
			Said:    said,
		}
	}
	if code != 0 {
		msg := firstNonEmpty(strField(env, "msg"), "error")
		said := strField(env, "msg")
		if said == "" {
			said = fmt.Sprintf("error %d", code)
		}
		return nil, &wbError{
			Message: fmt.Sprintf("%s: %s (%d)", site.Name, msg, code),
			Code:    code,
			Status:  resp.StatusCode,
			Said:    said,
		}
	}
	if env == nil {
		return nil, nil
	}
	data, _ := env["data"].(map[string]any)
	return data, nil
}

func randomHex(byteCount int) string {
	buf := make([]byte, byteCount)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

func decodeObject(body json.RawMessage, target any) error {
	trimmed := bytes.TrimSpace(body)
	if len(body) > sdk.MaxNativeBodyBytes || len(trimmed) == 0 || trimmed[0] != '{' {
		return invalid("Expected a bounded JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	if decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		return invalid("Invalid JSON object")
	}
	return nil
}

func invalid(message string) error {
	return &sdk.Error{Code: "invalid_request", Message: message, Outcome: "not-started"}
}

func denied(message string) error {
	return &sdk.Error{Code: "permission_denied", Message: message, Outcome: "not-started"}
}

func unauthenticated(message string) error {
	return &sdk.Error{Code: "unauthenticated", Message: message, Outcome: "not-started"}
}

func upstreamErr(message string) error {
	return &sdk.Error{Code: "upstream", Message: message, Outcome: "not-started"}
}

func nonempty(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null" && trimmed != "{}"
}

func strField(obj map[string]any, keys ...string) string {
	if obj == nil {
		return ""
	}
	for _, key := range keys {
		if raw, ok := obj[key].(string); ok {
			trimmed := strings.TrimSpace(raw)
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func rawStrField(obj map[string]any, key string) (string, bool) {
	if obj == nil {
		return "", false
	}
	val, ok := obj[key].(string)
	return val, ok
}

func nestedStr(obj map[string]any, outer, inner string) string {
	if obj == nil {
		return ""
	}
	sub, _ := obj[outer].(map[string]any)
	return strField(sub, inner)
}

func firstNonEmpty(values ...string) string {
	for _, val := range values {
		if strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

func truncateRunes(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}

func parseIntCode(raw any) int {
	switch typed := raw.(type) {
	case float64:
		if !math.IsNaN(typed) && !math.IsInf(typed, 0) {
			return int(typed)
		}
	case int:
		return typed
	case int64:
		return int(typed)
	case string:
		trimmed := strings.TrimSpace(typed)
		if parsed, err := strconv.Atoi(trimmed); err == nil {
			return parsed
		}
	}
	return 0
}

func asFloat(raw any) (float64, bool) {
	switch typed := raw.(type) {
	case float64:
		if !math.IsNaN(typed) && !math.IsInf(typed, 0) {
			return typed, true
		}
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		if err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0) {
			return parsed, true
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed != "" {
			parsed, err := strconv.ParseFloat(trimmed, 64)
			if err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0) {
				return parsed, true
			}
		}
	}
	return 0, false
}

func asInt64(raw any) (int64, bool) {
	val, ok := asFloat(raw)
	if !ok {
		return 0, false
	}
	return int64(val), true
}

func isSealedObj(raw any) bool {
	obj, ok := raw.(map[string]any)
	if !ok || obj == nil {
		return false
	}
	val, exists := obj["$wbEncrypted"]
	if !exists || val == nil {
		return false
	}
	switch typed := val.(type) {
	case bool:
		return typed
	case float64:
		return typed != 0
	case string:
		return typed != ""
	default:
		return true
	}
}

func sleepCtx(ctx context.Context, dur time.Duration) error {
	if dur <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(dur)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
