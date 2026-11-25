package qoder

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const (
	dedicatedClientID    = "f5a7f67c-11a8-491e-8b8e-a07f2d0df4b7"
	dedicatedVersionPath = "/algo/api/v1/version"
)

var (
	schemeRe     = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*://`)
	ipv4Re       = regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}$`)
	vpcNameRe    = regexp.MustCompile(`(?i)^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	entLocalRe   = regexp.MustCompile(`^(localhost|.*\.localhost|.*\.local|.*\.internal)$`)
	entDomainRe  = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$`)
	allDigitsRe  = regexp.MustCompile(`^\d+$`)
	phoneEmailRe = regexp.MustCompile(`(?i)^phone_[0-9a-f-]+@phone\.local$`)
	fourDigitsRe = regexp.MustCompile(`^\d{4}$`)
	nonDigitsRe  = regexp.MustCompile(`\D`)
)

type Credential struct {
	Type           string `json:"type"`
	Access         string `json:"access"`
	Refresh        string `json:"refresh"`
	Expires        int64  `json:"expires"`
	AccountID      string `json:"accountId,omitempty"`
	PhoneLabel     string `json:"phoneLabel,omitempty"`
	CardName       string `json:"cardName,omitempty"`
	UID            string `json:"uid"`
	Email          string `json:"email,omitempty"`
	Name           string `json:"name,omitempty"`
	OrgID          string `json:"orgId,omitempty"`
	MachineID      string `json:"machineId"`
	DeviceToken    string `json:"deviceToken"`
	DeviceRefresh  string `json:"deviceRefresh"`
	DeviceChat     bool   `json:"deviceChat,omitempty"`
	PATAuth        bool   `json:"patAuth,omitempty"`
	PAT            string `json:"pat,omitempty"`
	Identity       string `json:"identity,omitempty"`
	TeamID         string `json:"teamId,omitempty"`
	VPC            string `json:"vpc,omitempty"`
	EnterpriseHost string `json:"enterpriseHost,omitempty"`
	Dedicated      string `json:"dedicated,omitempty"`
	DedicatedAPI   string `json:"dedicatedApi,omitempty"`
	Lingma         bool   `json:"lingma,omitempty"`
}

func decodeCredential(raw json.RawMessage) (Credential, bool) {
	if !nonemptyRaw(raw) {
		return Credential{}, false
	}
	var cred Credential
	if err := json.Unmarshal(raw, &cred); err != nil {
		return Credential{}, false
	}
	if cred.Type == "" && cred.Access != "" && cred.UID != "" {
		cred.Type = "oauth"
	}
	return cred, true
}

type signInGoneError struct {
	Message string
	Expired bool
}

func (err *signInGoneError) Error() string { return err.Message }

type patRefusedError struct {
	Status  int
	Message string
}

func (err *patRefusedError) Error() string { return err.Message }

type openAPIResp struct {
	Status int
	JSON   map[string]any
	Text   string
}

func (owner *module) qwenworkClientHeaders() http.Header {
	buildPart := qwenworkAppRelease
	if parts := strings.SplitN(qwenworkAppRelease, "-", 2); len(parts) == 2 {
		buildPart = parts[1]
	}
	hdr := http.Header{}
	hdr.Set("User-Agent", "qoderwork/"+qwenworkAppVersion)
	hdr.Set("X-Request-Id", randomUUID())
	hdr.Set("X-QwenWork-Version", qwenworkAppVersion)
	hdr.Set("X-QwenWork-Release-Version", qwenworkAppRelease)
	hdr.Set("X-QwenWork-Build", buildPart)
	hdr.Set("X-QwenWork-Platform", owner.options.Platform)
	hdr.Set("X-QwenWork-Arch", owner.options.Arch)
	hdr.Set("X-QwenWork-Channel", "stable")
	return hdr
}

func (owner *module) openAPICall(ctx context.Context, stream *sdk.Stream, proxyOverride string, site Site, reqPath string, method string, token string, body any, query map[string]string) (openAPIResp, error) {
	if method == "" {
		method = http.MethodGet
	}
	parsedURL, err := url.Parse(site.OpenAPI + reqPath)
	if err != nil {
		return openAPIResp{}, err
	}
	if len(query) > 0 {
		values := parsedURL.Query()
		for key, val := range query {
			values.Set(key, val)
		}
		parsedURL.RawQuery = values.Encode()
	}
	var bodyReader *bytes.Reader
	if body != nil {
		encoded, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return openAPIResp{}, marshalErr
		}
		bodyReader = bytes.NewReader(encoded)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, method, parsedURL.String(), bodyReader)
	if err != nil {
		return openAPIResp{}, err
	}
	req.Header.Set("Accept", "application/json")
	if site.QwenWorkHeaders {
		for key, vals := range owner.qwenworkClientHeaders() {
			for _, val := range vals {
				req.Header.Set(key, val)
			}
		}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, cleanup, err := owner.doHTTP(callCtx, stream, proxyOverride, req)
	if err != nil {
		return openAPIResp{}, err
	}
	defer cleanup()
	defer resp.Body.Close()
	rawBytes, err := readBodyLimited(resp.Body, 4<<20)
	if err != nil {
		return openAPIResp{}, err
	}
	text := string(rawBytes)
	var parsed map[string]any
	_ = json.Unmarshal(rawBytes, &parsed)
	return openAPIResp{Status: resp.StatusCode, JSON: parsed, Text: text}, nil
}

func whenSaidMs(raw any) int64 {
	if raw == nil {
		return 0
	}
	switch typed := raw.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed <= 0 {
			return 0
		}
		if typed < 1e12 {
			return int64(typed * 1000)
		}
		return int64(typed)
	case int64:
		if typed <= 0 {
			return 0
		}
		if typed < 1e12 {
			return typed * 1000
		}
		return typed
	case json.Number:
		if numVal, err := typed.Float64(); err == nil && numVal > 0 {
			if numVal < 1e12 {
				return int64(numVal * 1000)
			}
			return int64(numVal)
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0
		}
		if allDigitsRe.MatchString(trimmed) {
			if numVal, err := strconv.ParseInt(trimmed, 10, 64); err == nil && numVal > 0 {
				if numVal < 1e12 {
					return numVal * 1000
				}
				return numVal
			}
			return 0
		}
		if parsedTime, ok := parseJSDate(trimmed); ok {
			return parsedTime.UnixMilli()
		}
	}
	return 0
}

func parseJSDate(text string) (time.Time, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func floatField(obj map[string]any, key string) (float64, bool) {
	if obj == nil {
		return 0, false
	}
	raw, ok := obj[key]
	if !ok || raw == nil {
		return 0, false
	}
	switch typed := raw.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		return typed, true
	case int64:
		return float64(typed), true
	case int:
		return float64(typed), true
	case json.Number:
		val, err := typed.Float64()
		if err != nil || math.IsNaN(val) || math.IsInf(val, 0) {
			return 0, false
		}
		return val, true
	}
	return 0, false
}

func expiresAtMs(jobToken map[string]any, nowMs int64) int64 {
	for _, field := range []string{"expire_time", "expires_at"} {
		if atMs := whenSaidMs(jobToken[field]); atMs > nowMs {
			return atMs
		}
	}
	if expiresIn, ok := floatField(jobToken, "expires_in"); ok && expiresIn > 0 {
		return nowMs + int64(expiresIn)
	}
	return nowMs + dayMs
}

func deviceExpiryMs(deviceToken map[string]any, nowMs int64) int64 {
	if parsed, ok := parseJSDate(strField(deviceToken, "expires_at")); ok {
		return parsed.UnixMilli()
	}
	if expiresIn, ok := floatField(deviceToken, "expires_in"); ok && expiresIn > 0 {
		return nowMs + int64(expiresIn*1000)
	}
	return nowMs + dayMs
}

func vpcInstance(input, domain string) string {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	if trimmed == "" || domain == "" {
		return ""
	}
	if schemeRe.MatchString(trimmed) {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return ""
		}
		if parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return ""
		}
		normalized := strings.TrimSuffix(trimmed, "/")
		if parsed.Hostname() != strings.TrimPrefix(normalized, "https://") {
			return ""
		}
		trimmed = parsed.Hostname()
	}
	if ipv4Re.MatchString(trimmed) {
		return ""
	}
	name := trimmed
	suffix := "." + strings.ToLower(domain)
	if strings.HasSuffix(trimmed, suffix) {
		name = trimmed[:len(trimmed)-len(suffix)]
		if strings.HasSuffix(name, "-openapi") {
			name = name[:len(name)-len("-openapi")]
		} else if strings.HasSuffix(name, "-gateway") {
			name = name[:len(name)-len("-gateway")]
		}
	} else if strings.Contains(trimmed, ".") {
		return ""
	}
	if !vpcNameRe.MatchString(name) {
		return ""
	}
	return name
}

func vpcSite(site Site, instance string) (Site, bool) {
	name := vpcInstance(instance, site.VPCDomain)
	if name == "" {
		return Site{}, false
	}
	next := site
	domain := site.VPCDomain
	next.Web = "https://" + name + "." + domain
	next.OpenAPI = "https://" + name + "-openapi." + domain
	next.API = "https://" + name + "-gateway." + domain
	next.VPC = name
	return next, true
}

func dedicatedOrigin(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if !schemeRe.MatchString(trimmed) {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	if strings.ToLower(parsed.Scheme) != "https" || parsed.User != nil || parsed.Hostname() == "" {
		return ""
	}
	return "https://" + strings.ToLower(parsed.Host)
}

func dedicatedHost(origin, suffix string) string {
	parsed, err := url.Parse(origin)
	if err != nil {
		return origin
	}
	hostName := parsed.Hostname()
	if hostName == "" || (strings.HasPrefix(hostName, "[") && strings.HasSuffix(hostName, "]")) || strings.Contains(hostName, ":") || ipv4Re.MatchString(hostName) {
		return origin
	}
	dot := strings.IndexByte(hostName, '.')
	if dot <= 0 || dot == len(hostName)-1 {
		return origin
	}
	rewritten := hostName[:dot] + suffix + "." + hostName[dot+1:]
	if port := parsed.Port(); port != "" {
		return parsed.Scheme + "://" + rewritten + ":" + port
	}
	return parsed.Scheme + "://" + rewritten
}

func dedicatedSite(site Site, domain, api string) (Site, bool) {
	origin := dedicatedOrigin(domain)
	if origin == "" {
		return Site{}, false
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return Site{}, false
	}
	host := parsed.Host
	next := site
	next.Web = origin
	next.OpenAPI = dedicatedHost(origin, "-openapi")
	if api != "" {
		if apiOrigin := dedicatedOrigin(api); apiOrigin != "" {
			next.API = apiOrigin
		} else {
			next.API = dedicatedHost(origin, "-gateway")
		}
	} else {
		next.API = dedicatedHost(origin, "-gateway")
	}
	next.ClientID = dedicatedClientID
	next.Redirect = ""
	next.VPC = host
	next.Dedicated = host
	return next, true
}

func enterpriseDomain(input string) string {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	if trimmed == "" {
		return ""
	}
	if schemeRe.MatchString(trimmed) {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return ""
		}
		if parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return ""
		}
		normalized := strings.TrimSuffix(trimmed, "/")
		if parsed.Hostname() != strings.TrimPrefix(normalized, "https://") {
			return ""
		}
		trimmed = parsed.Hostname()
	}
	if ipv4Re.MatchString(trimmed) || entLocalRe.MatchString(trimmed) || !entDomainRe.MatchString(trimmed) {
		return ""
	}
	return trimmed
}

func lingmaSite(site Site, domain string) (Site, bool) {
	origin := dedicatedOrigin(domain)
	if origin == "" {
		return Site{}, false
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return Site{}, false
	}
	host := parsed.Host
	next := site
	next.Web = origin
	next.OpenAPI = origin
	next.API = origin
	next.VPC = host
	next.Dedicated = host
	next.Lingma = true
	next.ClientType = "0"
	next.CosyVersion = lingmaVersion
	next.ModelScene = "assistant"
	next.SessionType = "assistant"
	return next, true
}

func siteOf(site Site, cred Credential) (Site, error) {
	if cred.EnterpriseHost != "" {
		base := "https://" + cred.EnterpriseHost
		next := site
		next.Web = base
		next.OpenAPI = base
		next.API = base
		next.VPC = cred.EnterpriseHost
		next.Redirect = ""
		return next, nil
	}
	if cred.Dedicated != "" && cred.Lingma {
		resolved, ok := lingmaSite(site, cred.Dedicated)
		if !ok {
			return Site{}, &signInGoneError{Message: fmt.Sprintf(`%s: the account's dedicated domain "%s" isn't one; sign in again`, site.Name, cred.Dedicated)}
		}
		return resolved, nil
	}
	if cred.Dedicated != "" {
		resolved, ok := dedicatedSite(site, cred.Dedicated, cred.DedicatedAPI)
		if !ok {
			return Site{}, &signInGoneError{Message: fmt.Sprintf(`%s: the account's dedicated domain "%s" isn't one; sign in again`, site.Name, cred.Dedicated)}
		}
		return resolved, nil
	}
	if cred.VPC == "" {
		return site, nil
	}
	resolved, ok := vpcSite(site, cred.VPC)
	if !ok {
		return Site{}, &signInGoneError{Message: fmt.Sprintf(`%s: the account's VPC instance "%s" isn't one; sign in again`, site.Name, cred.VPC)}
	}
	return resolved, nil
}

func (owner *module) dedicatedProbe(ctx context.Context, stream *sdk.Stream, origin string) (int, bool) {
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, origin+dedicatedVersionPath, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("User-Agent", "Qoder")
	resp, cleanup, err := owner.doHTTP(callCtx, stream, "", req)
	if err != nil {
		return 0, false
	}
	defer cleanup()
	defer resp.Body.Close()
	return resp.StatusCode, true
}

type lingmaCachedAccount struct {
	Root      string
	MachineID string
	UID       string
	Access    string
	Email     string
	Name      string
	OrgID     string
	Expires   int64
	Reason    string
}

func (owner *module) lingmaRoots() []string {
	var roots []string
	add := func(candidate string) {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			return
		}
		for _, existing := range roots {
			if existing == trimmed {
				return
			}
		}
		roots = append(roots, trimmed)
	}
	home := firstNonEmpty(owner.envValue("HOME"), owner.envValue("USERPROFILE"), owner.options.HomeDir())
	add(owner.envValue("QODERCN_COSY_HOME"))
	switch owner.options.Platform {
	case "darwin":
		add(filepath.Join(home, "Library", "Application Support", "QoderCN", "SharedClientCache"))
	case "win32", "windows":
		appData := firstNonEmpty(owner.envValue("APPDATA"), filepath.Join(home, "AppData", "Roaming"))
		add(filepath.Join(appData, "QoderCN", "SharedClientCache"))
	default:
		xdg := firstNonEmpty(owner.envValue("XDG_CONFIG_HOME"), filepath.Join(home, ".config"))
		add(filepath.Join(xdg, "QoderCN", "SharedClientCache"))
	}
	add(filepath.Join(home, ".config", "QoderCN", "SharedClientCache"))
	add(filepath.Join(home, ".config", "Qoder CN", "SharedClientCache"))
	add(filepath.Join(home, ".lingma"))
	return roots
}

func lingmaDecode(root string) lingmaCachedAccount {
	readTrimmed := func(filePath string) string {
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(raw))
	}
	machineID := firstNonEmpty(readTrimmed(filepath.Join(root, "cache", "id")), readTrimmed(filepath.Join(root, "id")))
	blob := readTrimmed(filepath.Join(root, "cache", "user"))
	if machineID == "" || blob == "" {
		return lingmaCachedAccount{Root: root, Reason: "no cache/id and cache/user"}
	}
	if len(machineID) < 16 {
		return lingmaCachedAccount{Root: root, Reason: "the machine id is too short"}
	}
	key := []byte(machineID[:16])
	if len(key) != 16 {
		return lingmaCachedAccount{Root: root, Reason: "the machine id is too short"}
	}
	cipherBytes, err := base64.StdEncoding.DecodeString(blob)
	if err != nil || len(cipherBytes) == 0 || len(cipherBytes)%aes.BlockSize != 0 {
		return lingmaCachedAccount{Root: root, Reason: "cache/user doesn't open with the machine id"}
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return lingmaCachedAccount{Root: root, Reason: "cache/user doesn't open with the machine id"}
	}
	plainPadded := make([]byte, len(cipherBytes))
	cipher.NewCBCDecrypter(block, key).CryptBlocks(plainPadded, cipherBytes)
	plain, err := pkcs7Unpad(plainPadded, aes.BlockSize)
	if err != nil {
		return lingmaCachedAccount{Root: root, Reason: "cache/user doesn't open with the machine id"}
	}
	var doc map[string]any
	if err := json.Unmarshal(plain, &doc); err != nil || doc == nil {
		return lingmaCachedAccount{Root: root, Reason: "cache/user doesn't open with the machine id"}
	}
	access := strField(doc, "security_oauth_token")
	uid := strField(doc, "uid")
	if access == "" || uid == "" {
		return lingmaCachedAccount{Root: root, Reason: "cache/user names no signed-in account"}
	}
	var expires int64
	if numVal, ok := floatField(doc, "expire_time"); ok && numVal > 0 {
		expires = int64(numVal)
	}
	return lingmaCachedAccount{
		Root:      root,
		MachineID: machineID,
		UID:       uid,
		Access:    access,
		Email:     strField(doc, "email"),
		Name:      strField(doc, "name"),
		OrgID:     strField(doc, "organization_id"),
		Expires:   expires,
	}
}

func (owner *module) lingmaRead(site Site) (lingmaCachedAccount, error) {
	var seen []string
	for _, root := range owner.lingmaRoots() {
		got := lingmaDecode(root)
		if got.Access != "" {
			return got, nil
		}
		seen = append(seen, fmt.Sprintf("%s: %s", root, got.Reason))
	}
	return lingmaCachedAccount{}, &signInGoneError{
		Message: fmt.Sprintf("%s: no Qoder CN IDE sign-in found on this computer. Sign in to your dedicated domain once in Qoder CN IDE (Settings → 企业版专属域登录), then sign in here again. Looked in %s", site.Name, strings.Join(seen, "; ")),
		Expired: true,
	}
}

func lingmaLive(got lingmaCachedAccount, nowMs int64) bool {
	return got.Expires == 0 || got.Expires-nowMs > lingmaLeadMs
}

func lingmaAccount(origin string, got lingmaCachedAccount, nowMs int64) Credential {
	host := origin
	if parsed, err := url.Parse(origin); err == nil && parsed.Host != "" {
		host = parsed.Host
	}
	expires := got.Expires
	if expires == 0 {
		expires = nowMs + dayMs
	}
	label := firstNonEmpty(got.Email, got.UID)
	card := firstNonEmpty(got.Email, got.Name, got.UID)
	return Credential{
		Type:          "oauth",
		Access:        got.Access,
		Refresh:       "",
		Expires:       expires,
		AccountID:     fmt.Sprintf("%s (%s)", label, host),
		CardName:      fmt.Sprintf("%s (%s)", card, host),
		UID:           got.UID,
		Email:         got.Email,
		Name:          got.Name,
		OrgID:         got.OrgID,
		MachineID:     got.MachineID,
		DeviceToken:   "",
		DeviceRefresh: "",
		Dedicated:     host,
		Lingma:        true,
	}
}

func (owner *module) lingmaRenew(site Site, cred Credential) (Credential, error) {
	got, err := owner.lingmaRead(site)
	if err != nil {
		return Credential{}, err
	}
	if got.UID != cred.UID {
		return Credential{}, &signInGoneError{
			Message: fmt.Sprintf("%s: Qoder CN IDE is now signed in as another account (%s); sign in here again", site.Name, firstNonEmpty(got.Email, got.UID)),
			Expired: true,
		}
	}
	nowMs := owner.nowMs()
	if got.Access == cred.Access || !lingmaLive(got, nowMs) {
		return Credential{}, &signInGoneError{
			Message: fmt.Sprintf("%s: the sign-in lapsed and Qoder CN IDE holds none newer; open Qoder CN IDE so it signs in again", site.Name),
			Expired: true,
		}
	}
	next := cred
	next.Access = got.Access
	next.Expires = got.Expires
	if next.Expires == 0 {
		next.Expires = nowMs + dayMs
	}
	next.MachineID = got.MachineID
	return next, nil
}

func maskPhone(raw string) string {
	digits := nonDigitsRe.ReplaceAllString(raw, "")
	if strings.HasPrefix(digits, "0086") && len(digits)-4 >= 7 {
		digits = digits[4:]
	} else if strings.HasPrefix(digits, "86") && len(digits)-2 >= 7 {
		digits = digits[2:]
	}
	if len(digits) < 11 {
		return ""
	}
	return digits[:3] + "****" + digits[len(digits)-4:]
}

func realEmailOf(email string) string {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" || phoneEmailRe.MatchString(trimmed) {
		return ""
	}
	return trimmed
}

func phoneLabelOf(user map[string]any, email, name string) string {
	phoneRaw := firstNonEmpty(
		strField(user, "security_mobile"),
		strField(user, "phoneNumber"),
		strField(user, "phone"),
		strField(user, "mobile"),
	)
	if masked := maskPhone(phoneRaw); masked != "" {
		return masked
	}
	if phoneEmailRe.MatchString(email) {
		tail := strings.TrimSpace(name)
		if fourDigitsRe.MatchString(tail) {
			return "****" + tail
		}
	}
	return ""
}

func normalizeSiteUser(site Site, doc map[string]any) map[string]any {
	if !site.QwenWorkHeaders {
		return doc
	}
	base := doc
	if dataObj, ok := doc["data"].(map[string]any); ok && dataObj != nil {
		base = dataObj
	}
	if userObj, ok := base["user"].(map[string]any); ok && userObj != nil {
		return userObj
	}
	return base
}

func accountIdentity(doc map[string]any) (string, string) {
	if doc == nil {
		return "", ""
	}
	base := doc
	if dataObj, ok := doc["data"].(map[string]any); ok && dataObj != nil {
		base = dataObj
	}
	merged := map[string]any{}
	for key, val := range base {
		merged[key] = val
	}
	if userObj, ok := base["user"].(map[string]any); ok && userObj != nil {
		for key, val := range userObj {
			merged[key] = val
		}
	}
	orgObj, _ := merged["organization"].(map[string]any)
	isBiz := false
	if rawBiz, ok := merged["is_biz"].(bool); ok {
		isBiz = rawBiz
	} else if rawBizCamel, ok := merged["isBiz"].(bool); ok {
		isBiz = rawBizCamel
	}
	firstStr := func(obj map[string]any, keys ...string) string {
		for _, key := range keys {
			if val := rawStrField(obj, key); val != "" {
				return val
			}
		}
		return ""
	}
	orgID := firstNonEmpty(firstStr(merged, "orgId", "organization_id", "organizationId"), firstStr(orgObj, "org_id", "orgId", "id"))
	teamID := firstNonEmpty(firstStr(merged, "team_id", "teamId"), firstStr(orgObj, "team_id", "teamId"))
	if isBiz && orgID != "" && teamID != "" {
		return "biz", teamID
	}
	return "", ""
}

func (owner *module) refreshedNames(ctx context.Context, stream *sdk.Stream, site Site, token string) (string, string) {
	if strings.TrimSpace(token) == "" {
		return "", ""
	}
	infoPath := site.UserInfoPath
	if infoPath == "" {
		infoPath = "/api/v1/userinfo"
	}
	resp, err := owner.openAPICall(ctx, stream, "", site, infoPath, http.MethodGet, token, nil, nil)
	if err != nil || resp.Status < 200 || resp.Status >= 300 || resp.JSON == nil {
		return "", ""
	}
	user := normalizeSiteUser(site, resp.JSON)
	email := strField(user, "email")
	name := strField(user, "name")
	label := phoneLabelOf(user, email, name)
	withSuffix := func(val string) string {
		if val == "" {
			return ""
		}
		if site.VPC != "" {
			return fmt.Sprintf("%s (%s)", val, site.VPC)
		}
		return val
	}
	return withSuffix(label), withSuffix(firstNonEmpty(realEmailOf(email), label))
}

func exchangeToken(doc map[string]any) string {
	return firstNonEmpty(strField(doc, "token"), strField(doc, "device_token"), strField(doc, "access_token"))
}

func (owner *module) patSignIn(ctx context.Context, stream *sdk.Stream, site Site, pat string) (Credential, error) {
	machineID := randomUUID()
	resp, err := owner.openAPICall(ctx, stream, "", site, "/api/v1/jobToken/exchange", http.MethodPost, "", map[string]any{"personal_token": pat}, nil)
	if err != nil {
		return Credential{}, err
	}
	if resp.Status == http.StatusBadRequest || resp.Status == http.StatusUnauthorized || resp.Status == http.StatusForbidden {
		return Credential{}, &patRefusedError{
			Status:  resp.Status,
			Message: fmt.Sprintf("%s refused the PAT — check it is still valid", site.Name),
		}
	}
	if resp.Status != http.StatusOK {
		return Credential{}, fmt.Errorf("%s PAT exchange: status %d", site.Name, resp.Status)
	}
	token := exchangeToken(resp.JSON)
	if token == "" {
		return Credential{}, fmt.Errorf("%s PAT exchange: empty token in response", site.Name)
	}
	refresh := firstNonEmpty(strField(resp.JSON, "refresh_token"), strField(resp.JSON, "refreshToken"))
	expires := expiresAtMs(resp.JSON, owner.nowMs())
	var email, name, uid string
	var user map[string]any
	if uiResp, uiErr := owner.openAPICall(ctx, stream, "", site, "/api/v1/userinfo", http.MethodGet, token, nil, nil); uiErr == nil && uiResp.Status >= 200 && uiResp.Status < 300 && uiResp.JSON != nil {
		user = uiResp.JSON
		email = strField(user, "email")
		name = strField(user, "name")
		uid = firstNonEmpty(strField(user, "id"), strField(user, "user_id"), strField(user, "uid"))
	}
	if uid == "" {
		return Credential{}, fmt.Errorf("%s PAT sign-in: could not determine user id", site.Name)
	}
	phoneLabel := phoneLabelOf(user, email, name)
	return Credential{
		Type:          "oauth",
		Access:        token,
		Refresh:       refresh,
		Expires:       expires,
		AccountID:     firstNonEmpty(email, uid),
		PhoneLabel:    phoneLabel,
		CardName:      firstNonEmpty(realEmailOf(email), phoneLabel),
		UID:           uid,
		Email:         email,
		Name:          name,
		MachineID:     machineID,
		DeviceToken:   "",
		DeviceRefresh: "",
		PATAuth:       true,
		PAT:           pat,
	}, nil
}

func deviceRefreshBody(site Site, refresh string, cred Credential) map[string]any {
	if site.DeviceRefreshTarget == "" {
		return map[string]any{"refresh_token": refresh}
	}
	body := map[string]any{"refresh_token": refresh}
	if cred.Identity == "biz" && cred.TeamID != "" {
		body["target"] = "biz"
		body["team_id"] = cred.TeamID
	} else {
		body["target"] = site.DeviceRefreshTarget
	}
	if site.ClientID != "" && site.Redirect != "" {
		body["client_id"] = site.ClientID
		body["redirect_uri"] = site.Redirect
	}
	return body
}

func (owner *module) refreshDevice(ctx context.Context, stream *sdk.Stream, proxyOverride string, site Site, refresh string, chat bool, cred Credential) (token string, nextRefresh string, expires int64, err error) {
	if strings.TrimSpace(refresh) == "" {
		if chat {
			return "", "", 0, &signInGoneError{Message: fmt.Sprintf("%s: the sign-in lapsed; sign in again", site.Name)}
		}
		return "", "", 0, fmt.Errorf("qoder device token refresh: missing refresh token; sign in again")
	}
	resp, reqErr := owner.openAPICall(ctx, stream, proxyOverride, site, "/api/v1/deviceToken/refresh", http.MethodPost, "", deviceRefreshBody(site, refresh, cred), nil)
	if reqErr != nil {
		return "", "", 0, fmt.Errorf("qoder device token refresh: request failed: %v", reqErr)
	}
	if resp.Status != http.StatusOK {
		baseErr := fmt.Sprintf("qoder device token refresh: upstream HTTP %d", resp.Status)
		if resp.Status == http.StatusUnauthorized || resp.Status == http.StatusForbidden {
			if chat {
				return "", "", 0, &signInGoneError{Message: fmt.Sprintf("%s sign-in has expired — sign in again (%s)", site.Name, baseErr), Expired: true}
			}
			return "", "", 0, fmt.Errorf("%s usage is unavailable: %s refused the account-page sign-in (chat still works) — sign in again to see usage (%s)", site.Name, site.Name, baseErr)
		}
		return "", "", 0, errors.New(baseErr)
	}
	tok := firstNonEmpty(strField(resp.JSON, "token"), strField(resp.JSON, "device_token"))
	ref := strField(resp.JSON, "refresh_token")
	if tok == "" || ref == "" {
		return "", "", 0, fmt.Errorf("qoder device token refresh: incomplete token pair")
	}
	return tok, ref, deviceExpiryMs(resp.JSON, owner.nowMs()), nil
}

func (owner *module) refreshJob(ctx context.Context, stream *sdk.Stream, proxyOverride string, site Site, refresh string, user string) (map[string]any, error) {
	if strings.TrimSpace(refresh) == "" {
		return nil, &signInGoneError{Message: fmt.Sprintf("%s: the sign-in lapsed; sign in again", site.Name)}
	}
	resp, err := owner.openAPICall(ctx, stream, proxyOverride, site, "/api/v1/jobToken/refresh", http.MethodPost, "", map[string]any{"refresh_token": refresh}, nil)
	if err != nil {
		return nil, err
	}
	if resp.Status == http.StatusUnauthorized || resp.Status == http.StatusForbidden {
		return nil, &signInGoneError{
			Message: fmt.Sprintf("%s's %s sign-in has expired — sign in again (qoder job token refresh: status %d)", user, site.Name, resp.Status),
			Expired: true,
		}
	}
	if resp.Status != http.StatusOK {
		return nil, fmt.Errorf("qoder job token refresh: status %d", resp.Status)
	}
	if strField(resp.JSON, "token") == "" || strField(resp.JSON, "refresh_token") == "" {
		return nil, fmt.Errorf("qoder job token refresh: incomplete token pair")
	}
	return resp.JSON, nil
}

func latestLocked(state *providerRuntimeState, cred Credential) Credential {
	cur := cred
	for hop := 0; hop < 16 && cur.Refresh != ""; hop++ {
		entry, ok := state.spent[cur.Refresh]
		if !ok {
			break
		}
		cur = mergeSpentFields(cur, entry.Got)
	}
	return cur
}

func mergeSpentFields(base, update Credential) Credential {
	out := base
	if update.Access != "" {
		out.Access = update.Access
	}
	if update.Refresh != "" {
		out.Refresh = update.Refresh
	}
	if update.Expires != 0 {
		out.Expires = update.Expires
	}
	if update.DeviceToken != "" {
		out.DeviceToken = update.DeviceToken
	}
	if update.DeviceRefresh != "" {
		out.DeviceRefresh = update.DeviceRefresh
	}
	if update.MachineID != "" {
		out.MachineID = update.MachineID
	}
	return out
}

func spendLocked(state *providerRuntimeState, oldRefresh string, got Credential, nowMs int64) Credential {
	for key, entry := range state.spent {
		if nowMs-entry.At > keepSpentMs {
			delete(state.spent, key)
		}
	}
	if oldRefresh != "" {
		state.spent[oldRefresh] = spentEntry{Got: got, At: nowMs}
	}
	return got
}

func (owner *module) persistCredential(ctx context.Context, stream *sdk.Stream, cred Credential) {
	if stream == nil {
		return
	}
	raw, err := json.Marshal(cred)
	if err != nil {
		return
	}
	var rep sdk.AccountReplaceResult
	_ = stream.HostCall(ctx, sdk.MethodAccountReplace, sdk.AccountReplaceRequest{Credentials: raw}, &rep)
}

func (owner *module) renewLocked(ctx context.Context, stream *sdk.Stream, proxyOverride string, site Site, state *providerRuntimeState, cred Credential) (Credential, error) {
	resolved, err := siteOf(site, cred)
	if err != nil {
		return Credential{}, err
	}
	if cred.Lingma {
		return owner.lingmaRenew(resolved, cred)
	}
	nowMs := owner.nowMs()
	if cred.DeviceChat {
		tok, ref, exp, refreshErr := owner.refreshDevice(ctx, stream, proxyOverride, resolved, cred.Refresh, true, cred)
		if refreshErr != nil {
			return Credential{}, refreshErr
		}
		delta := Credential{
			Access:        tok,
			Refresh:       ref,
			Expires:       exp,
			DeviceToken:   tok,
			DeviceRefresh: ref,
		}
		spendLocked(state, cred.Refresh, delta, nowMs)
		return mergeSpentFields(cred, delta), nil
	}
	user := firstNonEmpty(cred.AccountID, cred.Email, cred.UID)
	jobTok, err := owner.refreshJob(ctx, stream, proxyOverride, resolved, cred.Refresh, user)
	if err != nil {
		var gone *signInGoneError
		if !errors.As(err, &gone) || !cred.PATAuth || cred.PAT == "" {
			return Credential{}, err
		}
		exchanged, patErr := owner.patSignIn(ctx, stream, resolved, cred.PAT)
		if patErr != nil {
			var refused *patRefusedError
			if errors.As(patErr, &refused) {
				return Credential{}, &signInGoneError{
					Message: fmt.Sprintf("%s refused the PAT — sign in again with a new one", site.Name),
					Expired: true,
				}
			}
			return Credential{}, patErr
		}
		delta := Credential{
			Access:  exchanged.Access,
			Refresh: exchanged.Refresh,
			Expires: exchanged.Expires,
		}
		spendLocked(state, cred.Refresh, delta, nowMs)
		return mergeSpentFields(cred, delta), nil
	}
	delta := Credential{
		Access:  strField(jobTok, "token"),
		Refresh: strField(jobTok, "refresh_token"),
		Expires: expiresAtMs(jobTok, nowMs),
	}
	spendLocked(state, cred.Refresh, delta, nowMs)
	return mergeSpentFields(cred, delta), nil
}

func (owner *module) fresh(ctx context.Context, stream *sdk.Stream, proxyOverride string, site Site, state *providerRuntimeState, stored Credential) (Credential, bool, error) {
	state.authMu.Lock()
	defer state.authMu.Unlock()
	if stored.Type != "oauth" || stored.Access == "" || stored.UID == "" {
		return Credential{}, false, &signInGoneError{Message: fmt.Sprintf("%s: not signed in", site.Name)}
	}
	if _, err := siteOf(site, stored); err != nil {
		return Credential{}, false, err
	}
	cur := latestLocked(state, stored)
	if cur.Expires-owner.nowMs() > refreshLeadMs {
		return cur, false, nil
	}
	next, err := owner.renewLocked(ctx, stream, proxyOverride, site, state, cur)
	if err != nil {
		return Credential{}, false, err
	}
	owner.persistCredential(ctx, stream, next)
	return next, true, nil
}

func (owner *module) relogin(ctx context.Context, stream *sdk.Stream, proxyOverride string, site Site, state *providerRuntimeState, stored Credential, refused string) (Credential, bool, error) {
	state.authMu.Lock()
	defer state.authMu.Unlock()
	if stored.Type != "oauth" || stored.Access == "" || stored.UID == "" {
		return Credential{}, false, &signInGoneError{Message: fmt.Sprintf("%s: not signed in", site.Name)}
	}
	if _, err := siteOf(site, stored); err != nil {
		return Credential{}, false, err
	}
	cur := latestLocked(state, stored)
	if cur.Access != refused {
		return cur, false, nil
	}
	nowMs := owner.nowMs()
	for _, entry := range state.spent {
		if entry.Got.Access == refused && nowMs-entry.At < reloginGapMs {
			return Credential{}, false, nil
		}
	}
	next, err := owner.renewLocked(ctx, stream, proxyOverride, site, state, cur)
	if err != nil {
		return Credential{}, false, err
	}
	owner.persistCredential(ctx, stream, next)
	return next, true, nil
}

func (owner *module) rotateDeviceToken(ctx context.Context, stream *sdk.Stream, proxyOverride string, site Site, state *providerRuntimeState, stored Credential, attempted string) (Credential, string, error) {
	state.authMu.Lock()
	defer state.authMu.Unlock()
	cur := latestLocked(state, stored)
	if cur.DeviceToken != attempted {
		return cur, cur.DeviceToken, nil
	}
	resolved, err := siteOf(site, cur)
	if err != nil {
		return Credential{}, "", err
	}
	tok, ref, exp, err := owner.refreshDevice(ctx, stream, proxyOverride, resolved, cur.DeviceRefresh, cur.DeviceChat, cur)
	if err != nil {
		return Credential{}, "", err
	}
	next := cur
	next.DeviceToken = tok
	next.DeviceRefresh = ref
	if cur.DeviceChat {
		delta := Credential{
			Access:        tok,
			Refresh:       ref,
			Expires:       exp,
			DeviceToken:   tok,
			DeviceRefresh: ref,
		}
		spendLocked(state, cur.Refresh, delta, owner.nowMs())
		next = mergeSpentFields(next, delta)
	}
	owner.persistCredential(ctx, stream, next)
	return next, tok, nil
}

func b64urlRandom(numBytes int) string {
	buf := make([]byte, numBytes)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (owner *module) startDeviceFlow(site Site, method string) (sdk.AuthStartResult, error) {
	verifier := b64urlRandom(64)
	challenge := pkceChallenge(verifier)
	nonce := randomUUID()
	machineID := randomUUID()
	params := url.Values{}
	params.Set("challenge", challenge)
	params.Set("challenge_method", "S256")
	params.Set("nonce", nonce)
	params.Set("machine_id", machineID)
	params.Set("client_id", site.ClientID)
	if site.Redirect != "" {
		params.Set("redirect_uri", site.Redirect)
	}
	flowID := randomUUID()
	owner.flowMu.Lock()
	owner.flows[flowID] = &authFlow{
		Site:      site,
		Method:    method,
		Verifier:  verifier,
		Nonce:     nonce,
		MachineID: machineID,
		UntilMs:   owner.nowMs() + owner.options.SignInTimeout.Milliseconds(),
	}
	owner.flowMu.Unlock()
	cont, _ := json.Marshal(map[string]string{"flow": flowID})
	return sdk.AuthStartResult{
		URL:          fmt.Sprintf("%s/device/selectAccounts?%s", site.Web, params.Encode()),
		PollAfterMs:  pollIntervalMs,
		Continuation: cont,
	}, nil
}

func (owner *module) authStart(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	site, _, err := owner.siteFor(call.Scope.Provider)
	if err != nil {
		return nil, err
	}
	var req sdk.AuthStartRequest
	if err := json.Unmarshal(call.Params, &req); err != nil {
		return nil, invalid("invalid authStart params")
	}
	method := strings.TrimSpace(req.Method)
	if method == "" {
		if site.ID == ProviderQwenWorkEnterprise {
			method = "enterprise"
		} else {
			method = "browser"
		}
	}
	switch method {
	case "browser":
		if site.ID == ProviderQwenWorkEnterprise {
			return nil, invalid("qwenwork-enterprise requires enterprise domain sign-in")
		}
		return owner.startDeviceFlow(site, "browser")

	case "pat":
		if !site.PAT {
			return nil, invalid(fmt.Sprintf("%s does not support PAT sign-in", site.Name))
		}
		var input struct {
			PAT string `json:"pat"`
		}
		if nonemptyRaw(req.Input) {
			_ = json.Unmarshal(req.Input, &input)
		}
		pat := strings.TrimSpace(input.PAT)
		if pat != "" && !strings.HasPrefix(pat, "pt-") {
			return nil, invalid("PAT must start with pt-")
		}
		flowID := randomUUID()
		owner.flowMu.Lock()
		owner.flows[flowID] = &authFlow{Site: site, Method: "pat", PAT: pat}
		owner.flowMu.Unlock()
		cont, _ := json.Marshal(map[string]string{"flow": flowID})
		return sdk.AuthStartResult{
			URL:          site.Web + "/account/integrations",
			Continuation: cont,
		}, nil

	case "vpc":
		if site.VPCDomain == "" {
			return nil, invalid(fmt.Sprintf("%s does not support VPC sign-in", site.Name))
		}
		var input struct {
			VPC string `json:"vpc"`
		}
		if nonemptyRaw(req.Input) {
			_ = json.Unmarshal(req.Input, &input)
		}
		resolved, ok := vpcSite(site, input.VPC)
		if !ok {
			return nil, invalid(fmt.Sprintf("Not a VPC instance: %s", truncateText(input.VPC, 100)))
		}
		return owner.startDeviceFlow(resolved, "vpc")

	case "dedicated":
		if site.VPCDomain == "" {
			return nil, invalid(fmt.Sprintf("%s does not support dedicated domain sign-in", site.Name))
		}
		var input struct {
			Dedicated string `json:"dedicated"`
		}
		if nonemptyRaw(req.Input) {
			_ = json.Unmarshal(req.Input, &input)
		}
		origin := dedicatedOrigin(input.Dedicated)
		if origin == "" {
			return nil, invalid(fmt.Sprintf("Not a dedicated domain: %s", truncateText(input.Dedicated, 100)))
		}
		status, answered := owner.dedicatedProbe(ctx, stream, origin)
		if !answered {
			return nil, upstreamErr(fmt.Sprintf("%s: %s didn't answer (%s); check the address and your network", site.Name, origin, dedicatedVersionPath))
		}
		if status == http.StatusNotFound {
			got, readErr := owner.lingmaRead(site)
			if readErr != nil {
				return nil, unauthenticated(readErr.Error())
			}
			if !lingmaLive(got, owner.nowMs()) {
				return nil, unauthenticated(fmt.Sprintf("%s: Qoder CN IDE's sign-in (%s) ended %s; open Qoder CN IDE so it signs in again, then sign in here again",
					site.Name, firstNonEmpty(got.Email, got.UID), time.UnixMilli(got.Expires).UTC().Format(time.RFC3339)))
			}
			flowID := randomUUID()
			owner.flowMu.Lock()
			owner.flows[flowID] = &authFlow{Site: site, Method: "lingma", Lingma: &got, Origin: origin}
			owner.flowMu.Unlock()
			cont, _ := json.Marshal(map[string]string{"flow": flowID})
			return sdk.AuthStartResult{URL: origin, Continuation: cont}, nil
		}
		gateway := dedicatedHost(origin, "-gateway")
		apiOverride := ""
		if gateway != origin {
			if _, ok := owner.dedicatedProbe(ctx, stream, gateway); !ok {
				apiOverride = origin
			}
		}
		resolved, ok := dedicatedSite(site, origin, apiOverride)
		if !ok {
			return nil, invalid(fmt.Sprintf("Not a dedicated domain: %s", truncateText(input.Dedicated, 100)))
		}
		return owner.startDeviceFlow(resolved, "dedicated")

	case "enterprise":
		if site.ID != ProviderQwenWorkEnterprise {
			return nil, invalid(fmt.Sprintf("%s does not support enterprise flagship sign-in", site.Name))
		}
		var input struct {
			Domain string `json:"domain"`
		}
		if nonemptyRaw(req.Input) {
			_ = json.Unmarshal(req.Input, &input)
		}
		domain := enterpriseDomain(owner.envValue("QWENWORK_ENT_DOMAIN"))
		if domain == "" {
			domain = enterpriseDomain(input.Domain)
		}
		if domain == "" {
			return nil, invalid("请输入企业域名（或设置 QWENWORK_ENT_DOMAIN 环境变量）")
		}
		resolved := site
		resolved.Web = "https://" + domain
		resolved.OpenAPI = "https://" + domain
		resolved.API = "https://" + domain
		resolved.Redirect = ""
		resolved.EnterpriseDomain = domain
		resolved.VPC = domain
		return owner.startDeviceFlow(resolved, "enterprise")

	default:
		return nil, invalid(fmt.Sprintf("unsupported sign-in method %q", method))
	}
}

func (owner *module) authFinish(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var req sdk.AuthFinishRequest
	if err := json.Unmarshal(call.Params, &req); err != nil {
		return nil, invalid("invalid authFinish params")
	}
	var cont struct {
		Flow string `json:"flow"`
	}
	if nonemptyRaw(req.Continuation) {
		_ = json.Unmarshal(req.Continuation, &cont)
	}
	owner.flowMu.Lock()
	flow, ok := owner.flows[cont.Flow]
	owner.flowMu.Unlock()
	if !ok || flow == nil {
		return nil, invalid("sign-in flow not found or expired")
	}
	deleteFlow := func() {
		owner.flowMu.Lock()
		delete(owner.flows, cont.Flow)
		owner.flowMu.Unlock()
	}

	switch flow.Method {
	case "lingma":
		deleteFlow()
		cred := lingmaAccount(flow.Origin, *flow.Lingma, owner.nowMs())
		rawCred, _ := json.Marshal(cred)
		return sdk.AuthResult{
			ID:          cred.AccountID,
			Name:        firstNonEmpty(cred.CardName, cred.AccountID),
			Credentials: rawCred,
			ExpiresAt:   cred.Expires / 1000,
		}, nil

	case "pat":
		deleteFlow()
		pat := flow.PAT
		if pat == "" && nonemptyRaw(req.Input) {
			var input struct {
				PAT string `json:"pat"`
			}
			_ = json.Unmarshal(req.Input, &input)
			pat = strings.TrimSpace(input.PAT)
		}
		if pat == "" {
			pat = strings.TrimSpace(req.Code)
		}
		if pat == "" {
			return nil, invalid("enter your Personal Access Token")
		}
		if !strings.HasPrefix(pat, "pt-") {
			return nil, invalid("PAT must start with pt-")
		}
		cred, err := owner.patSignIn(ctx, stream, flow.Site, pat)
		if err != nil {
			var refused *patRefusedError
			if errors.As(err, &refused) {
				return nil, unauthenticated(refused.Message)
			}
			return nil, upstreamErr(err.Error())
		}
		rawCred, _ := json.Marshal(cred)
		return sdk.AuthResult{
			ID:          cred.AccountID,
			Name:        firstNonEmpty(cred.CardName, cred.AccountID),
			Credentials: rawCred,
			ExpiresAt:   cred.Expires / 1000,
		}, nil

	default:
		if owner.nowMs() > flow.UntilMs {
			deleteFlow()
			return nil, upstreamErr(fmt.Sprintf("%s: the sign-in timed out; start again", flow.Site.Name))
		}
		pollResp, pollErr := owner.openAPICall(ctx, stream, "", flow.Site, "/api/v1/deviceToken/poll", http.MethodGet, "", nil, map[string]string{
			"nonce":            flow.Nonce,
			"verifier":         flow.Verifier,
			"challenge_method": "S256",
		})
		if pollErr != nil || pollResp.Status != http.StatusOK || strField(pollResp.JSON, "token") == "" {
			return sdk.AuthResult{Pending: true}, nil
		}
		deleteFlow()
		cred, err := owner.completeDeviceSignIn(ctx, stream, flow.Site, flow.MachineID, pollResp.JSON)
		if err != nil {
			return nil, upstreamErr(err.Error())
		}
		rawCred, _ := json.Marshal(cred)
		return sdk.AuthResult{
			ID:          cred.AccountID,
			Name:        firstNonEmpty(cred.CardName, cred.AccountID),
			Credentials: rawCred,
			ExpiresAt:   cred.Expires / 1000,
		}, nil
	}
}

func (owner *module) completeDeviceSignIn(ctx context.Context, stream *sdk.Stream, site Site, machineID string, dt map[string]any) (Credential, error) {
	nowMs := owner.nowMs()
	dtToken := strField(dt, "token")
	dtRefresh := strField(dt, "refresh_token")
	var access, refresh string
	var expires int64
	var deviceChat bool

	if site.DeviceOnly {
		access = dtToken
		refresh = dtRefresh
		expires = deviceExpiryMs(dt, nowMs)
		deviceChat = true
	} else {
		jr, err := owner.openAPICall(ctx, stream, "", site, "/api/v1/me/jobToken", http.MethodPost, dtToken, map[string]any{"clientId": site.ClientID}, nil)
		if err != nil {
			return Credential{}, err
		}
		if jr.Status == http.StatusOK {
			tok := strField(jr.JSON, "token")
			if tok == "" {
				return Credential{}, fmt.Errorf("%s job token: empty token in response", site.Name)
			}
			access = tok
			refresh = strField(jr.JSON, "refresh_token")
			expires = expiresAtMs(jr.JSON, nowMs)
		} else if site.DeviceChat && jr.Status >= 400 && jr.Status < 500 {
			access = dtToken
			refresh = dtRefresh
			expires = deviceExpiryMs(dt, nowMs)
			deviceChat = true
		} else {
			return Credential{}, fmt.Errorf("%s job token: status %d: %s", site.Name, jr.Status, truncateText(strings.TrimSpace(jr.Text), 300))
		}
	}

	var email, name string
	uid := strField(dt, "user_id")
	var user map[string]any
	var identityKind, teamID string
	infoPath := site.UserInfoPath
	if infoPath == "" {
		infoPath = "/api/v1/userinfo"
	}
	if uiResp, uiErr := owner.openAPICall(ctx, stream, "", site, infoPath, http.MethodGet, dtToken, nil, nil); uiErr == nil && uiResp.Status >= 200 && uiResp.Status < 300 && uiResp.JSON != nil {
		user = normalizeSiteUser(site, uiResp.JSON)
		if site.DeviceRefreshTarget != "" {
			identityKind, teamID = accountIdentity(uiResp.JSON)
		}
		email = strField(user, "email")
		name = strField(user, "name")
		if site.DeviceOnly {
			uid = firstNonEmpty(strField(user, "id"), strField(user, "user_id"), strField(user, "uid"), uid)
		}
		if site.VPC != "" && uid == "" {
			uid = firstNonEmpty(strField(uiResp.JSON, "id"), strField(uiResp.JSON, "user_id"), strField(uiResp.JSON, "uid"))
		}
	}
	if site.DeviceOnly && uid == "" {
		return Credential{}, fmt.Errorf("%s: the sign-in named no user", site.Name)
	}
	if site.VPC != "" && uid == "" {
		return Credential{}, fmt.Errorf("%s (%s): the sign-in named no user", site.Name, site.VPC)
	}

	var accountID string
	switch {
	case site.VPC != "":
		accountID = fmt.Sprintf("%s (%s)", firstNonEmpty(email, uid), site.VPC)
	case site.DeviceOnly:
		accountID = firstNonEmpty(email, uid, name, strField(dt, "user_id"))
	default:
		accountID = firstNonEmpty(email, strField(dt, "user_id"))
	}
	phoneLabelRaw := phoneLabelOf(user, email, name)
	withSuffix := func(val string) string {
		if val == "" {
			return ""
		}
		if site.VPC != "" {
			return fmt.Sprintf("%s (%s)", val, site.VPC)
		}
		return val
	}
	cred := Credential{
		Type:          "oauth",
		Access:        access,
		Refresh:       refresh,
		Expires:       expires,
		AccountID:     accountID,
		PhoneLabel:    withSuffix(phoneLabelRaw),
		CardName:      withSuffix(firstNonEmpty(realEmailOf(email), phoneLabelRaw)),
		UID:           uid,
		Email:         email,
		Name:          firstNonEmpty(name, strField(dt, "user_name")),
		MachineID:     machineID,
		DeviceToken:   dtToken,
		DeviceRefresh: dtRefresh,
		DeviceChat:    deviceChat,
		Identity:      identityKind,
		TeamID:        teamID,
		VPC:           site.VPC,
	}
	if site.EnterpriseDomain != "" {
		cred.VPC = site.EnterpriseDomain
		cred.EnterpriseHost = site.EnterpriseDomain
	}
	if site.Dedicated != "" {
		cred.Dedicated = site.Dedicated
		if site.API != dedicatedHost(site.Web, "-gateway") {
			cred.DedicatedAPI = site.API
		}
	}
	return cred, nil
}

func (owner *module) authRefresh(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	site, state, err := owner.siteFor(call.Scope.Provider)
	if err != nil {
		return nil, err
	}
	var req sdk.AuthRefreshRequest
	if nonemptyRaw(call.Params) {
		_ = json.Unmarshal(call.Params, &req)
	}
	rawCred := req.Credentials
	if !nonemptyRaw(rawCred) {
		rawCred = call.Auth
	}
	stored, ok := decodeCredential(rawCred)
	if !ok || stored.Type != "oauth" || stored.Access == "" || stored.UID == "" {
		return nil, unauthenticated(fmt.Sprintf("%s: not signed in", site.Name))
	}
	if stored.Lingma {
		state.authMu.Lock()
		cur := latestLocked(state, stored)
		state.authMu.Unlock()
		resolved, siteErr := siteOf(site, cur)
		if siteErr != nil {
			return nil, unauthenticated(siteErr.Error())
		}
		next, renewErr := owner.lingmaRenew(resolved, cur)
		if renewErr != nil {
			encoded, _ := json.Marshal(cur)
			return sdk.AuthResult{ID: cur.AccountID, Name: firstNonEmpty(cur.CardName, cur.AccountID), Credentials: encoded, ExpiresAt: cur.Expires / 1000}, nil
		}
		encoded, _ := json.Marshal(next)
		return sdk.AuthResult{ID: next.AccountID, Name: firstNonEmpty(next.CardName, next.AccountID), Credentials: encoded, ExpiresAt: next.Expires / 1000}, nil
	}
	if strings.TrimSpace(stored.Refresh) == "" {
		encoded, _ := json.Marshal(stored)
		return sdk.AuthResult{ID: stored.AccountID, Name: firstNonEmpty(stored.CardName, stored.AccountID), Credentials: encoded, ExpiresAt: stored.Expires / 1000}, nil
	}

	state.authMu.Lock()
	cur := latestLocked(state, stored)
	var next Credential
	didRenew := false
	if cur.Access != stored.Access && cur.Expires-owner.nowMs() > renewLeadMs {
		next = cur
	} else {
		renewedCred, renewErr := owner.renewLocked(ctx, stream, "", site, state, cur)
		if renewErr != nil {
			state.authMu.Unlock()
			var gone *signInGoneError
			if errors.As(renewErr, &gone) {
				return nil, unauthenticated(gone.Message)
			}
			return nil, upstreamErr(renewErr.Error())
		}
		next = renewedCred
		didRenew = true
	}
	state.authMu.Unlock()

	if didRenew {
		if resolved, siteErr := siteOf(site, next); siteErr == nil {
			phoneLabel, cardName := owner.refreshedNames(ctx, stream, resolved, next.Access)
			if phoneLabel != "" {
				next.PhoneLabel = phoneLabel
			}
			if cardName != "" {
				next.CardName = cardName
			}
		}
	}
	encoded, _ := json.Marshal(next)
	return sdk.AuthResult{
		ID:          next.AccountID,
		Name:        firstNonEmpty(next.CardName, next.AccountID),
		Credentials: encoded,
		ExpiresAt:   next.Expires / 1000,
	}, nil
}

func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen]
}
