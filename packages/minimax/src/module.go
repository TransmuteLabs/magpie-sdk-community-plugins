package minimax

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"runtime"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

const ModuleID = "minimax"
const Version = "0.1.1"
const China = "minimax-code"
const Global = "minimax-code-global"

type Site struct {
	ID, Name, Region, Lang                       string
	Account, LLM, Agent, AgentFallback, Platform string
}

func DefaultSites() map[string]Site {
	return map[string]Site{
		China: {
			ID: China, Name: "MiniMax Code", Region: "cn", Lang: "zh",
			Account: "https://account.minimax.cn", LLM: "https://agent.minimax.cn",
			Agent: "https://agent.minimaxi.com", AgentFallback: "https://agent.minimax.cn",
			Platform: "https://www.minimaxi.com",
		},
		Global: {
			ID: Global, Name: "MiniMax Code (Global)", Region: "en", Lang: "en",
			Account: "https://account.minimax.io", LLM: "https://agent.minimax.io",
			Agent: "https://agent.minimax.io", Platform: "https://platform.minimax.io",
		},
	}
}

type ModuleOptions struct {
	Sites          map[string]Site
	Now            func() time.Time
	HTTPFactory    HTTPFactory
	TimezoneOffset func() int
	Platform       string
}

type module struct {
	options ModuleOptions
	session string
}

func NewModule(options ModuleOptions) sdk.Module {
	sites := DefaultSites()
	for id, site := range options.Sites {
		if _, ok := sites[id]; ok {
			site.ID = id
			sites[id] = site
		}
	}
	options.Sites = sites
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.HTTPFactory == nil {
		options.HTTPFactory = defaultHTTP
	}
	if options.TimezoneOffset == nil {
		options.TimezoneOffset = func() int {
			_, offset := time.Now().Zone()
			return offset
		}
	}
	if options.Platform == "" {
		options.Platform = runtime.GOOS
		if options.Platform == "windows" {
			options.Platform = "win32"
		}
	}
	owner := &module{options: options, session: randomID()}
	result := sdk.Module{
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
		Actions: map[string]sdk.Handler{"account.checkin": owner.checkin},
	}
	installTools(owner, &result)
	return result
}

func Manifest() sdk.Manifest { return NewModule(ModuleOptions{}).Manifest }

func coreManifest() sdk.Manifest {
	entry := "lib/minimax.so"
	if runtime.GOOS == "darwin" {
		entry = "lib/minimax.dylib"
	}
	if runtime.GOOS == "windows" {
		entry = "lib/minimax.dll"
	}
	result := sdk.Manifest{
		Schema: 1, ID: ModuleID, Version: Version, Runtime: "native", Entry: entry,
		Contract: sdk.Contract{
			Version:      sdk.APIVersion{Major: 1, Minor: 12},
			Capabilities: []sdk.Capability{sdk.AccountAuth, sdk.OutboundExecutor, sdk.ModelDiscovery, sdk.UsageReporter, sdk.ModuleRPC, sdk.RequestPolicy, sdk.ProviderSignIn},
		},
		Compatibility: &sdk.Compatibility{HostAPI: "^1.12", NativeABI: 1},
		Actions:       []sdk.ActionDescriptor{{ID: "account.checkin", Effect: "write", InputSchema: json.RawMessage(`{"type":"object","properties":{"day":{"type":"string"},"proxy":{"type":"string"}},"required":["day"],"additionalProperties":false}`)}},
	}
	for _, id := range []string{China, Global} {
		result.AuthBindings = append(result.AuthBindings, sdk.WindowAuthBindingDescriptor{
			ID: "provider.signin." + id, From: sdk.ProviderSignInPart, Provider: id, Reauthorize: true,
			Methods: []sdk.WindowAuthMethod{{ID: "device", Mode: "poll", Label: "Device authorization"}},
		})
		result.Providers = append(result.Providers, sdk.ProviderDescriptor{
			ID: id, Name: DefaultSites()[id].Name,
			Protocols: []protocolext.Protocol{protocolext.Messages, protocolext.Chat},
			Models:    append(fallbackModels(), mediaModels()...), Operations: mediaOperations(),
			Checkin:   &sdk.AccountCheckinDescriptor{Action: "account.checkin"},
			KeepAlive: &sdk.AccountKeepAliveDescriptor{LeadMs: 600000},
		})
	}
	return result
}

type Credentials struct {
	Site        string `json:"site"`
	ID          string `json:"id"`
	Access      string `json:"access"`
	Refresh     string `json:"refresh,omitempty"`
	ExpiresAt   int64  `json:"expiresAt"`
	UserID      string `json:"userId,omitempty"`
	Subject     string `json:"subject,omitempty"`
	SubjectKind string `json:"subjectKind,omitempty"`
	Email       string `json:"email,omitempty"`
	Name        string `json:"name,omitempty"`
}

func accountID(site, subject string) string {
	sum := sha256.Sum256([]byte(site + "\x00" + subject))
	return "mm-" + hex.EncodeToString(sum[:16])
}
func randomID() string {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		panic(err)
	}
	entropy[6] = entropy[6]&15 | 64
	entropy[8] = entropy[8]&63 | 128
	literal := hex.EncodeToString(entropy[:])
	return literal[:8] + "-" + literal[8:12] + "-" + literal[12:16] + "-" + literal[16:20] + "-" + literal[20:]
}
func raw(value any) json.RawMessage {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return body
}
func decode(body json.RawMessage, value any) error {
	if len(body) > sdk.MaxNativeBodyBytes || len(bytes.TrimSpace(body)) == 0 || bytes.TrimSpace(body)[0] != '{' {
		return invalid("Expected a bounded JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		return invalid("Invalid JSON object")
	}
	return nil
}
func invalid(message string) error { return &sdk.Error{Code: "invalid_params", Message: message} }
func denied(message string) error  { return &sdk.Error{Code: "permission_denied", Message: message} }
func uncertain(message string) error {
	return &sdk.Error{Code: "upstream", Message: message, Outcome: "uncertain"}
}
func safeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var moduleError *sdk.Error
	if errors.As(err, &moduleError) {
		return keepFailureDetails(moduleError, err)
	}
	return keepFailureDetails(&sdk.Error{Code: "upstream", Message: "MiniMax request failed"}, err)
}
func text(value any) string {
	literal, _ := value.(string)
	return strings.TrimSpace(literal)
}
func object(value any) map[string]any {
	fields, _ := value.(map[string]any)
	return fields
}

func (owner *module) site(id string) (Site, error) {
	site, ok := owner.options.Sites[id]
	if !ok {
		return Site{}, invalid("Unknown MiniMax provider")
	}
	return site, nil
}
func (owner *module) SelectedClient(ctx context.Context, call sdk.Call, stream *sdk.Stream, siteID string) (*SelectedClient, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if siteID == "" {
		siteID = call.Scope.Provider
	}
	if siteID != call.Scope.Provider || call.Scope.Account == "" || call.Scope.Generation == 0 {
		return nil, denied("An exact selected MiniMax account is required")
	}
	site, err := owner.site(siteID)
	if err != nil {
		return nil, err
	}
	var credentials Credentials
	if decode(call.Auth, &credentials) != nil || credentials.Site != siteID || credentials.ID != call.Scope.Account || credentials.Access == "" {
		return nil, denied("Selected MiniMax credentials do not match the call")
	}
	if call.Method != sdk.MethodAuthRefresh && call.Method != sdk.MethodAuthFinish && credentials.ExpiresAt > 0 && credentials.ExpiresAt <= owner.options.Now().UnixMilli() {
		return nil, &sdk.Error{Code: "auth_refresh_required", Message: "Renew the selected MiniMax account through the host before dispatch", Outcome: "not-started"}
	}
	return &SelectedClient{owner: owner, site: site, credentials: credentials, call: call, stream: stream}, nil
}
