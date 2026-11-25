package kimiwork

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"runtime"
	"strings"
	"sync"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

const (
	ModuleID    = "kimi-work"
	ProviderID  = "kimi-work"
	Version     = "0.1.0"
	DefaultBase = "https://agent-gw.kimi.com/coding/v1"
	MemberBase  = "https://www.kimi.com/apiv2/kimi.gateway.membership.v2.MembershipService/"
	RefreshURL  = "https://auth.kimi.com/api/account.gateway.v1.AuthService/RefreshToken"
	ConfigURL   = "https://www.kimi.com/apiv2/kimi.gateway.config.v1.ConfigService/DescribeKimiWorkConfig"

	KimiCodeVersion = "0.36.0"
	DaimonVersion   = "0.6.10"
	DesktopVersion  = "3.2.20"
	AuthVersion     = "3.2.17"
	UserAgent       = "kimi-code-cli/" + KimiCodeVersion
	SpeedHeader     = "X-Kimi-Model-Speed"
	FastSuffix      = "-fast"
	RefreshLeadMs   = 60000
	SourceTag       = "kimi-work-desktop"
)

type Endpoints struct {
	Base    string
	Member  string
	Refresh string
	Config  string
}

func DefaultEndpoints() Endpoints {
	return Endpoints{
		Base:    DefaultBase,
		Member:  MemberBase,
		Refresh: RefreshURL,
		Config:  ConfigURL,
	}
}

type ModuleOptions struct {
	Endpoints   Endpoints
	Now         func() time.Time
	HTTPFactory HTTPFactory
	ReadSession func() (DesktopSession, error)
	Platform    string
	Hostname    string
	OSRelease   string
	OSType      string
	OSArch      string
	Timezone    string
	Locale      string
}

type tokenPair struct {
	Access  string
	Refresh string
	Expires int64
}

type renewCall struct {
	done chan struct{}
	pair tokenPair
	err  error
}

type module struct {
	options   ModuleOptions
	mu        sync.Mutex
	renewing  map[string]*renewCall
	renewed   map[string]tokenPair
	fastCache map[string]map[string]string
}

func NewModule(options ModuleOptions) sdk.Module {
	defaults := DefaultEndpoints()
	if options.Endpoints.Base == "" {
		options.Endpoints.Base = defaults.Base
	}
	if options.Endpoints.Member == "" {
		options.Endpoints.Member = defaults.Member
	}
	if options.Endpoints.Refresh == "" {
		options.Endpoints.Refresh = defaults.Refresh
	}
	if options.Endpoints.Config == "" {
		options.Endpoints.Config = defaults.Config
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.HTTPFactory == nil {
		options.HTTPFactory = defaultHTTP
	}
	if options.Platform == "" {
		options.Platform = runtime.GOOS
		if options.Platform == "windows" {
			options.Platform = "win32"
		}
	}
	if options.ReadSession == nil {
		base := strings.TrimRight(options.Endpoints.Base, "/")
		options.ReadSession = func() (DesktopSession, error) {
			return readDesktop(DesktopFile(DesktopPathOptions{}), base)
		}
	}
	if options.Timezone == "" {
		options.Timezone = localTimezone()
	}
	if options.Locale == "" {
		options.Locale = "en-US"
	}
	owner := &module{
		options:   options,
		renewing:  map[string]*renewCall{},
		renewed:   map[string]tokenPair{},
		fastCache: map[string]map[string]string{},
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
	}
}

func Manifest() sdk.Manifest { return NewModule(ModuleOptions{}).Manifest }

func coreManifest() sdk.Manifest {
	entry := "lib/kimi-work.so"
	if runtime.GOOS == "darwin" {
		entry = "lib/kimi-work.dylib"
	}
	if runtime.GOOS == "windows" {
		entry = "lib/kimi-work.dll"
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
			},
		},
		Compatibility: &sdk.Compatibility{HostAPI: "^1.12", NativeABI: 1},
		AuthBindings: []sdk.WindowAuthBindingDescriptor{{
			ID:          "provider.signin." + ProviderID,
			From:        sdk.ProviderSignInPart,
			Provider:    ProviderID,
			Reauthorize: true,
			Methods: []sdk.WindowAuthMethod{
				{ID: "desktop", Mode: "poll", Label: "Use local Kimi Work sign-in"},
				{
					ID:    sdk.SavedSignInMethod,
					Mode:  "poll",
					Label: "Import saved Kimi Work sign-in",
					Fields: []sdk.AuthInputField{
						{Key: "record", Label: "Saved sign-in", Type: "secret"},
					},
				},
			},
		}},
		Providers: []sdk.ProviderDescriptor{{
			ID:        ProviderID,
			Name:      "Kimi Work",
			Protocols: []protocolext.Protocol{protocolext.Chat},
			Models:    fallbackModels(),
			KeepAlive: &sdk.AccountKeepAliveDescriptor{LeadMs: RefreshLeadMs},
		}},
	}
}

type Credentials struct {
	Source    string `json:"source"`
	UID       string `json:"uid"`
	AccountID string `json:"accountId,omitempty"`
	Access    string `json:"access,omitempty"`
	Refresh   string `json:"refresh,omitempty"`
	ExpiresAt int64  `json:"expires,omitempty"`
}

func (creds Credentials) Valid() bool {
	return creds.Source == SourceTag && strings.TrimSpace(creds.UID) != ""
}

func (creds Credentials) HasPair() bool {
	return creds.Valid() && creds.Access != "" && creds.Refresh != ""
}

func AccountUID(userID string) string {
	sum := sha256.Sum256([]byte("kimi-work-account\x00" + userID))
	return hex.EncodeToString(sum[:])
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

func uncertain(message string) error {
	return &sdk.Error{Code: "upstream", Message: message, Outcome: "uncertain"}
}

func nonempty(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null" && trimmed != "{}"
}

func localTimezone() string {
	name := time.Now().Location().String()
	if name == "" || name == "Local" {
		return "UTC"
	}
	return name
}
