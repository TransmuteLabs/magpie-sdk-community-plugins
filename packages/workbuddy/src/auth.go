package workbuddy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type Credential struct {
	Type             string `json:"type,omitempty"`
	Source           string `json:"source,omitempty"`
	Key              string `json:"key,omitempty"`
	Access           string `json:"access,omitempty"`
	Refresh          string `json:"refresh,omitempty"`
	Expires          int64  `json:"expires,omitempty"`
	RefreshExpiresAt int64  `json:"refreshExpiresAt,omitempty"`
	Domain           string `json:"domain,omitempty"`
	TokenType        string `json:"tokenType,omitempty"`
	UID              string `json:"uid,omitempty"`
	EnterpriseID     string `json:"enterpriseId,omitempty"`
	AccountID        string `json:"accountId,omitempty"`
	Name             string `json:"name,omitempty"`
	User             string `json:"user,omitempty"`
	Plan             string `json:"plan,omitempty"`
}

func (cred Credential) isAPIKey() bool {
	return cred.Type == "api" || cred.Source == "key" || (cred.Key != "" && cred.Access == "" && cred.Source == "")
}

func (cred Credential) isOAuth() bool {
	if cred.isAPIKey() {
		return false
	}
	return cred.Access != "" || cred.Source == "desktop"
}

func keyAccount(key string) (Credential, bool) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return Credential{}, false
	}
	suffix := trimmed
	if len(suffix) > 8 {
		suffix = suffix[len(suffix)-8:]
	}
	return Credential{
		Type:   "api",
		Key:    trimmed,
		Access: trimmed,
		UID:    "anonymous_" + suffix,
		Source: "key",
	}, true
}

var (
	remoteConfigRe = regexp.MustCompile(`^(ck_|pt_)`)
	cnDomainRe     = regexp.MustCompile(`(?i)(^|\.)(codebuddy\.cn|tencent\.com|qq\.com)$`)
	aiDomainRe     = regexp.MustCompile(`(?i)(^|\.)workbuddy\.ai$`)
	invalidTokenRe = regexp.MustCompile(`(?i)invalid_token`)
	invalidGrantRe = regexp.MustCompile(`(?i)invalid_grant`)
)

func remoteConfigKey(key string) bool {
	return remoteConfigRe.MatchString(key)
}

func mergeToken(acct Credential, got map[string]any, nowMs int64) Credential {
	out := acct
	out.Type = "oauth"
	if access := strField(got, "accessToken"); access != "" {
		out.Access = access
	}
	if refresh := strField(got, "refreshToken"); refresh != "" {
		out.Refresh = refresh
	}
	if domain := strField(got, "domain"); domain != "" {
		out.Domain = domain
	}
	if tokenType := strField(got, "tokenType"); tokenType != "" {
		out.TokenType = tokenType
	}
	if expAt, ok := asInt64(got["expiresAt"]); ok && expAt > 0 {
		out.Expires = expAt
	} else if expIn, ok := asInt64(got["expiresIn"]); ok && expIn > 0 {
		out.Expires = nowMs + expIn*1000
	}
	if refExpAt, ok := asInt64(got["refreshExpiresAt"]); ok && refExpAt > 0 {
		out.RefreshExpiresAt = refExpAt
	} else if refExpIn, ok := asInt64(got["refreshExpiresIn"]); ok && refExpIn > 0 {
		out.RefreshExpiresAt = nowMs + refExpIn*1000
	}
	return out
}

func mergeTokenFields(base, refreshed Credential) Credential {
	out := base
	if refreshed.Access != "" {
		out.Access = refreshed.Access
	}
	if refreshed.Refresh != "" {
		out.Refresh = refreshed.Refresh
	}
	if refreshed.Expires != 0 {
		out.Expires = refreshed.Expires
	}
	if refreshed.RefreshExpiresAt != 0 {
		out.RefreshExpiresAt = refreshed.RefreshExpiresAt
	}
	if refreshed.Domain != "" {
		out.Domain = refreshed.Domain
	}
	if refreshed.TokenType != "" {
		out.TokenType = refreshed.TokenType
	}
	return out
}

func (owner *module) stale(acct Credential) bool {
	return acct.Expires > 0 && owner.nowMs() >= acct.Expires-EarlyMs
}

func (owner *module) refreshable(acct Credential) bool {
	if acct.Refresh == "" {
		return false
	}
	if acct.RefreshExpiresAt > 0 && owner.nowMs() >= acct.RefreshExpiresAt {
		return false
	}
	return true
}

func (owner *module) persistCredential(ctx context.Context, stream *sdk.Stream, acct Credential) {
	if stream == nil || acct.Source == "desktop" || acct.Source == "key" {
		return
	}
	raw, err := json.Marshal(acct)
	if err != nil {
		return
	}
	var rep sdk.AccountReplaceResult
	_ = stream.HostCall(ctx, sdk.MethodAccountReplace, sdk.AccountReplaceRequest{Credentials: raw}, &rep)
}

func (owner *module) renew(ctx context.Context, stream *sdk.Stream, site Site, acct Credential, save bool) (Credential, error) {
	key := site.ID + "\n" + acct.Refresh
	now := owner.nowMs()
	owner.mu.Lock()
	if done, ok := owner.renewed[key]; ok && now-done.at < KeepRenewedMs {
		owner.mu.Unlock()
		return mergeTokenFields(acct, done.next), nil
	}
	if flight, ok := owner.renewing[key]; ok {
		owner.mu.Unlock()
		select {
		case <-flight.done:
			if flight.err != nil {
				return Credential{}, flight.err
			}
			return mergeTokenFields(acct, flight.next), nil
		case <-ctx.Done():
			return Credential{}, ctx.Err()
		}
	}
	flight := &renewFlight{done: make(chan struct{})}
	owner.renewing[key] = flight
	owner.mu.Unlock()

	headers := map[string]string{
		"X-Refresh-Token":       acct.Refresh,
		"X-Auth-Refresh-Source": "plugin",
		"X-Domain":              domainOf(site, acct),
	}
	for hKey, hVal := range enterpriseHeaders(acct) {
		headers[hKey] = hVal
	}
	got, err := owner.callAPI(ctx, stream, siteAt(site, acct), http.MethodPost, "/v2/plugin/auth/token/refresh", headers, map[string]any{})
	if err == nil && strField(got, "accessToken") == "" {
		err = &wbError{Message: "WorkBuddy gave no refreshed token", Bare: true}
	}
	var next Credential
	if err == nil {
		next = mergeToken(acct, got, owner.nowMs())
		owner.mu.Lock()
		owner.renewed[key] = renewResult{next: next, at: owner.nowMs()}
		delete(owner.renewing, key)
		owner.mu.Unlock()
		if save {
			owner.persistCredential(ctx, stream, next)
		}
		flight.next = next
		close(flight.done)
		return next, nil
	}
	owner.mu.Lock()
	delete(owner.renewing, key)
	owner.mu.Unlock()
	flight.err = err
	close(flight.done)
	return Credential{}, err
}

func (owner *module) fresh(ctx context.Context, stream *sdk.Stream, site Site, acct Credential, save bool) (Credential, error) {
	if !owner.stale(acct) {
		return acct, nil
	}
	if !owner.refreshable(acct) {
		if acct.Access != "" {
			return acct, nil
		}
		return Credential{}, unauthenticated("this WorkBuddy account is signed out; sign in again")
	}
	next, err := owner.renew(ctx, stream, site, acct, save)
	if err != nil {
		if acct.Access != "" {
			return acct, nil
		}
		var wErr *wbError
		if errors.As(err, &wErr) {
			if wErr.Bare {
				return Credential{}, upstreamErr(wErr.Message)
			}
			if wErr.Said != "" {
				return Credential{}, upstreamErr("WorkBuddy token refresh: " + wErr.Said)
			}
		}
		return Credential{}, upstreamErr("WorkBuddy token refresh: " + err.Error())
	}
	return next, nil
}

func (owner *module) readDesktop(site Site) (Credential, bool) {
	raw, err := os.ReadFile(owner.desktopFile(site))
	if err != nil {
		return Credential{}, false
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil || doc == nil {
		return Credential{}, false
	}
	authObj, _ := doc["auth"].(map[string]any)
	acctObj, _ := doc["account"].(map[string]any)
	access, ok := rawStrField(authObj, "accessToken")
	uid := strField(acctObj, "uid")
	if !ok || access == "" || uid == "" {
		return Credential{}, false
	}
	now := owner.nowMs()
	refresh, _ := rawStrField(authObj, "refreshToken")
	domain, _ := rawStrField(authObj, "domain")
	tokenType, _ := rawStrField(authObj, "tokenType")
	name := firstNonEmpty(strField(acctObj, "nickname"), strField(acctObj, "phoneNumber"), uid)
	cred := Credential{
		Type:             "oauth",
		Source:           "desktop",
		Access:           access,
		Refresh:          refresh,
		Expires:          atMsVal(authObj["expiresAt"], authObj["expiresIn"], now),
		RefreshExpiresAt: atMsVal(authObj["refreshExpiresAt"], authObj["refreshExpiresIn"], now),
		Domain:           domain,
		TokenType:        tokenType,
		UID:              uid,
		EnterpriseID:     strField(acctObj, "enterpriseId"),
		Name:             name,
		AccountID:        name,
	}
	return cred, true
}

func (owner *module) notDesktop(site Site) string {
	raw, err := os.ReadFile(owner.desktopFile(site))
	if err == nil {
		var doc map[string]any
		if json.Unmarshal(raw, &doc) == nil {
			if authObj, ok := doc["auth"].(map[string]any); ok {
				if isSealedObj(authObj["accessToken"]) || isSealedObj(authObj["refreshToken"]) {
					return fmt.Sprintf(`%s desktop keeps its sign-in encrypted (WorkBuddy 5.6 and later), which magpie can't read; sign in with "%s account (browser)" instead`, site.Name, site.Name)
				}
			}
		}
	}
	return fmt.Sprintf("%s desktop isn't signed in", site.Name)
}

func (owner *module) current(ctx context.Context, stream *sdk.Stream, site Site, auth Credential) (Credential, error) {
	if auth.Source != "desktop" {
		return owner.fresh(ctx, stream, site, auth, true)
	}
	desk, ok := owner.readDesktop(site)
	if !ok {
		return Credential{}, unauthenticated(owner.notDesktop(site))
	}
	key := site.ID + "|" + desk.UID
	owner.mu.Lock()
	held, hasHeld := owner.desktopHeld[key]
	owner.mu.Unlock()
	chosen := desk
	if hasHeld && held.Expires >= desk.Expires {
		chosen = held
	}
	acct, err := owner.fresh(ctx, stream, site, chosen, false)
	if err != nil {
		return Credential{}, err
	}
	if auth.AccountID != "" && acct.AccountID == "" {
		acct.AccountID = auth.AccountID
	}
	owner.mu.Lock()
	owner.desktopHeld[key] = acct
	owner.mu.Unlock()
	return acct, nil
}

type enterpriseInfo struct {
	ID   string
	Name string
}

func (owner *module) enterpriseOf(ctx context.Context, stream *sdk.Stream, site Site, acct Credential, who map[string]any) enterpriseInfo {
	own := strField(who, "enterpriseId")
	ownName := strField(who, "enterpriseName")
	acctType := strings.ToLower(strField(who, "type"))
	if own == "" && (acctType == "" || acctType == "personal") {
		return enterpriseInfo{}
	}
	if own != "" && ownName != "" {
		return enterpriseInfo{ID: own, Name: ownName}
	}
	list, err := owner.callAPI(ctx, stream, siteAt(site, acct), http.MethodGet, "/v2/plugin/accounts", map[string]string{
		"Authorization": "Bearer " + acct.Access,
		"X-Domain":      domainOf(site, acct),
	}, nil)
	if err != nil || list == nil {
		return enterpriseInfo{ID: own}
	}
	rawAccounts, _ := list["accounts"].([]any)
	uid := strField(who, "uid")
	var mine []map[string]any
	for _, item := range rawAccounts {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strField(entry, "uid") == uid && strField(entry, "enterpriseId") != "" {
			mine = append(mine, entry)
		}
	}
	if own != "" {
		for _, entry := range mine {
			if strField(entry, "enterpriseId") == own {
				return enterpriseInfo{ID: own, Name: strField(entry, "enterpriseName")}
			}
		}
		return enterpriseInfo{ID: own}
	}
	var same, last []map[string]any
	for _, entry := range mine {
		if strings.ToLower(strField(entry, "type")) == acctType {
			same = append(same, entry)
		}
		if truthy(entry["lastLogin"]) {
			last = append(last, entry)
		}
	}
	var pick map[string]any
	switch {
	case len(same) == 1:
		pick = same[0]
	case len(last) == 1:
		pick = last[0]
	case len(mine) == 1:
		pick = mine[0]
	}
	if pick == nil {
		return enterpriseInfo{}
	}
	return enterpriseInfo{ID: strField(pick, "enterpriseId"), Name: strField(pick, "enterpriseName")}
}

func accountName(who map[string]any, ent enterpriseInfo) string {
	person := firstNonEmpty(strField(who, "nickname"), strField(who, "phoneNumber"), strField(who, "uid"))
	if ent.ID == "" {
		return person
	}
	team := ent.Name
	if team == "" {
		tail := ent.ID
		if len(tail) > 6 {
			tail = tail[len(tail)-6:]
		}
		team = "enterprise " + tail
	}
	if person != "" {
		return person + " · " + team
	}
	return team
}

func truthy(val any) bool {
	switch typed := val.(type) {
	case bool:
		return typed
	case float64:
		return typed != 0
	case string:
		return typed != "" && typed != "0" && strings.ToLower(typed) != "false"
	default:
		return false
	}
}

func atMsVal(atRaw, inSecRaw any, nowMs int64) int64 {
	if atVal, ok := asInt64(atRaw); ok && atVal > 0 {
		return atVal
	}
	if inVal, ok := asInt64(inSecRaw); ok && inVal > 0 {
		return nowMs + inVal*1000
	}
	return 0
}

func plainAccount(who map[string]any) map[string]any {
	if who == nil {
		return nil
	}
	out := map[string]any{}
	for key, val := range who {
		out[key] = val
	}
	out["nickname"] = strField(who, "nickname")
	out["phoneNumber"] = strField(who, "phoneNumber")
	out["uid"] = strField(who, "uid")
	return out
}

type parsedSignIn struct {
	Cred   Credential
	Who    map[string]any
	Sealed bool
	Error  string
}

func signInsIn(text string, nowMs int64) []parsedSignIn {
	var root any
	if json.Unmarshal([]byte(text), &root) != nil {
		return []parsedSignIn{{Error: "not JSON"}}
	}
	var out []parsedSignIn
	var walk func(node any, depth int)
	walk = func(node any, depth int) {
		if node == nil || depth > 4 {
			return
		}
		switch typed := node.(type) {
		case []any:
			for _, elem := range typed {
				walk(elem, depth+1)
			}
		case map[string]any:
			if authObj, ok := typed["auth"].(map[string]any); ok && authObj != nil {
				_, hasAcc := authObj["accessToken"]
				_, hasRef := authObj["refreshToken"]
				if hasAcc || hasRef {
					if isSealedObj(authObj["accessToken"]) || isSealedObj(authObj["refreshToken"]) {
						out = append(out, parsedSignIn{Sealed: true})
						return
					}
					acctObj, _ := typed["account"].(map[string]any)
					cred := Credential{
						Type:             "oauth",
						Access:           strField(authObj, "accessToken"),
						Refresh:          strField(authObj, "refreshToken"),
						Expires:          atMsVal(authObj["expiresAt"], authObj["expiresIn"], nowMs),
						RefreshExpiresAt: atMsVal(authObj["refreshExpiresAt"], authObj["refreshExpiresIn"], nowMs),
						Domain:           strField(authObj, "domain"),
						TokenType:        strField(authObj, "tokenType"),
						EnterpriseID:     strField(acctObj, "enterpriseId"),
					}
					var who map[string]any
					if strField(acctObj, "uid") != "" {
						who = plainAccount(acctObj)
					}
					out = append(out, parsedSignIn{Cred: cred, Who: who})
					return
				}
			}
			accessRaw := firstPresent(typed, "access", "accessToken", "access_token")
			refreshRaw := firstPresent(typed, "refresh", "refreshToken", "refresh_token")
			if isSealedObj(accessRaw) || isSealedObj(refreshRaw) {
				out = append(out, parsedSignIn{Sealed: true})
				return
			}
			access := strings.TrimSpace(stringOrEmpty(accessRaw))
			refresh := strings.TrimSpace(stringOrEmpty(refreshRaw))
			if access != "" || refresh != "" {
				uid := strField(typed, "uid")
				cred := Credential{
					Type:             "oauth",
					Access:           access,
					Refresh:          refresh,
					Expires:          atMsVal(firstPresent(typed, "expires", "expiresAt"), firstPresent(typed, "expiresIn", "expires_in"), nowMs),
					RefreshExpiresAt: atMsVal(typed["refreshExpiresAt"], firstPresent(typed, "refreshExpiresIn", "refresh_expires_in"), nowMs),
					Domain:           strField(typed, "domain"),
					TokenType:        strField(typed, "tokenType", "token_type"),
					EnterpriseID:     strField(typed, "enterpriseId"),
				}
				var who map[string]any
				if uid != "" {
					who = map[string]any{
						"uid":      uid,
						"nickname": firstNonEmpty(strField(typed, "nickname"), strField(typed, "accountId")),
					}
				}
				out = append(out, parsedSignIn{Cred: cred, Who: who})
				return
			}
			for _, child := range typed {
				walk(child, depth+1)
			}
		}
	}
	walk(root, 0)
	if len(out) == 0 {
		return []parsedSignIn{{Error: "no WorkBuddy sign-in in it"}}
	}
	return out
}

func firstPresent(obj map[string]any, keys ...string) any {
	for _, key := range keys {
		if val, ok := obj[key]; ok && val != nil {
			return val
		}
	}
	return nil
}

func stringOrEmpty(val any) string {
	str, _ := val.(string)
	return str
}

func otherSite(site Site, acct Credential) string {
	dom := strings.ToLower(strings.TrimSpace(acct.Domain))
	if dom == "" {
		return ""
	}
	isCN := cnDomainRe.MatchString(dom)
	isAI := aiDomainRe.MatchString(dom)
	if site.AI && isCN {
		return "WorkBuddy"
	}
	if !site.AI && isAI {
		return "WorkBuddy AI"
	}
	return ""
}

func (owner *module) importSingleRecord(ctx context.Context, stream *sdk.Stream, site Site, text string) (Credential, string, error) {
	entries := signInsIn(text, owner.nowMs())
	var lastErr error
	for _, entry := range entries {
		if entry.Error != "" {
			lastErr = invalid(entry.Error)
			continue
		}
		if entry.Sealed {
			lastErr = invalid(fmt.Sprintf(`%s desktop keeps this sign-in encrypted (WorkBuddy 5.6 and later), which magpie can't read; sign in with "%s account (browser)" instead`, site.Name, site.Name))
			continue
		}
		acct := entry.Cred
		if other := otherSite(site, acct); other != "" {
			lastErr = invalid(fmt.Sprintf("this is a %s sign-in; import it under %s", other, other))
			continue
		}
		var err error
		if owner.refreshable(acct) {
			acct, err = owner.renew(ctx, stream, site, acct, false)
			if err != nil {
				lastErr = err
				continue
			}
		} else if acct.Access == "" || owner.stale(acct) {
			lastErr = unauthenticated(fmt.Sprintf("the sign-in has ended; sign in to %s again where it came from", site.Name))
			continue
		}
		who := entry.Who
		if strField(who, "uid") == "" {
			hdrs := map[string]string{
				"Authorization": "Bearer " + acct.Access,
				"X-Domain":      domainOf(site, acct),
			}
			for hKey, hVal := range enterpriseHeaders(acct) {
				hdrs[hKey] = hVal
			}
			list, callErr := owner.callAPI(ctx, stream, siteAt(site, acct), http.MethodGet, "/v2/plugin/accounts", hdrs, nil)
			if callErr != nil {
				lastErr = callErr
				continue
			}
			rawAccounts, _ := list["accounts"].([]any)
			var all []map[string]any
			for _, item := range rawAccounts {
				row, ok := item.(map[string]any)
				if ok && strField(row, "uid") != "" {
					all = append(all, row)
				}
			}
			var pick []map[string]any
			if acct.EnterpriseID != "" {
				for _, row := range all {
					if strField(row, "enterpriseId") == acct.EnterpriseID {
						pick = append(pick, row)
					}
				}
			} else {
				for _, row := range all {
					if truthy(row["lastLogin"]) {
						pick = append(pick, row)
					}
				}
			}
			var chosen map[string]any
			if len(pick) == 1 {
				chosen = pick[0]
			} else if len(all) == 1 {
				chosen = all[0]
			}
			who = plainAccount(chosen)
			if who == nil {
				lastErr = upstreamErr(fmt.Sprintf("%s didn't say whose sign-in this is", site.Name))
				continue
			}
		}
		acct.UID = strField(who, "uid")
		ent := owner.enterpriseOf(ctx, stream, site, acct, who)
		if ent.ID != "" {
			acct.EnterpriseID = ent.ID
		}
		name := firstNonEmpty(accountName(who, ent), site.Name)
		acct.AccountID = name
		acct.Name = name
		acct.Type = "oauth"
		return acct, name, nil
	}
	if lastErr == nil {
		lastErr = invalid("no WorkBuddy sign-in in it")
	}
	return Credential{}, "", lastErr
}

func isInvalidToken(resp *http.Response) bool {
	return resp != nil && resp.StatusCode == http.StatusUnauthorized && invalidTokenRe.MatchString(resp.Header.Get("WWW-Authenticate"))
}

func isRefreshRefused(err error) bool {
	var wErr *wbError
	if !errors.As(err, &wErr) {
		return false
	}
	return wErr.Status == http.StatusUnauthorized || invalidGrantRe.MatchString(wErr.Message)
}

type reissueResult struct {
	HasCred bool
	Cred    Credential
	Why     string
	Kind    string
}

func (owner *module) reissued(ctx context.Context, stream *sdk.Stream, site Site, acct Credential) reissueResult {
	if acct.Source == "desktop" {
		owner.mu.Lock()
		delete(owner.desktopHeld, site.ID+"|"+acct.UID)
		owner.mu.Unlock()
		desk, ok := owner.readDesktop(site)
		if !ok {
			return reissueResult{Why: owner.notDesktop(site), Kind: "unknown"}
		}
		if desk.Access != acct.Access {
			owner.mu.Lock()
			owner.desktopHeld[site.ID+"|"+desk.UID] = desk
			owner.mu.Unlock()
			return reissueResult{HasCred: true, Cred: desk}
		}
		return reissueResult{Why: fmt.Sprintf("%s desktop's own sign-in is the one refused", site.Name), Kind: "unknown"}
	}
	if !owner.refreshable(acct) {
		if acct.Refresh != "" {
			return reissueResult{Why: "its refresh token has ended", Kind: "nothing to renew with"}
		}
		return reissueResult{Why: "there is no refresh token to renew it with", Kind: "nothing to renew with"}
	}
	next, err := owner.renew(ctx, stream, site, acct, true)
	if err != nil {
		if isRefreshRefused(err) {
			var wErr *wbError
			said := err.Error()
			if errors.As(err, &wErr) && wErr.Said != "" {
				said = wErr.Said
			}
			return reissueResult{Why: fmt.Sprintf("it refused the refresh token too (%s)", said), Kind: "refresh refused"}
		}
		var wErr *wbError
		said := err.Error()
		if errors.As(err, &wErr) && wErr.Said != "" {
			said = wErr.Said
		}
		return reissueResult{Why: "renewing it failed: " + said, Kind: "unknown"}
	}
	if next.Access != acct.Access {
		return reissueResult{HasCred: true, Cred: next}
	}
	return reissueResult{Why: "renewing it gave the same token", Kind: "unknown"}
}

func isGone(acct Credential, res reissueResult, tokenRefused bool) bool {
	if acct.Source == "desktop" {
		return false
	}
	return res.Kind == "refresh refused" || (res.Kind == "nothing to renew with" && tokenRefused)
}

func refusedText(site Site, acct Credential, why, note string) string {
	whyPart := ""
	if why != "" {
		whyPart = "; " + why
	}
	notePart := ""
	if note != "" {
		notePart = note + " "
	}
	tail := fmt.Sprintf("Sign in to it again in magpie: Providers → %s → this account's Sign in again.", site.Name)
	if acct.Source == "desktop" {
		tail = fmt.Sprintf("Sign in to %s desktop again; magpie reads its sign-in on the next request.", site.Name)
	}
	return fmt.Sprintf("%s refused this account's sign-in (401)%s. %s%s", site.Name, whyPart, notePart, tail)
}

func keyRefusedPayload(site Site, expired bool, note string) ([]byte, http.Header) {
	notePart := ""
	if note != "" {
		notePart = note + " "
	}
	message := fmt.Sprintf("%s refused this API key (401). %sCheck the key at %s, then add it again in magpie: Providers → %s.", site.Name, notePart, site.Keys, site.Name)
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "authentication_error",
			"code":    401,
		},
	})
	signIn := "kept"
	if expired {
		signIn = "expired"
	}
	return body, http.Header{
		"Content-Type":     {"application/json"},
		"X-Magpie-Sign-In": {signIn},
	}
}

func signedOutPayload(site Site, acct Credential, why string, expired bool, note string) ([]byte, http.Header) {
	message := refusedText(site, acct, why, note)
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "authentication_error",
			"code":    401,
		},
	})
	signIn := "kept"
	if expired && acct.Source != "desktop" {
		signIn = "expired"
	}
	return body, http.Header{
		"Content-Type":     {"application/json"},
		"X-Magpie-Sign-In": {signIn},
	}
}

type authContinuation struct {
	FlowID string `json:"flowId"`
}

func (owner *module) authStart(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var req sdk.AuthStartRequest
	if decodeObject(call.Params, &req) != nil {
		return nil, invalid("Auth start requires a JSON object")
	}
	site, ok := owner.siteFor(call.Scope.Provider)
	if !ok {
		return nil, invalid("Unknown WorkBuddy provider")
	}
	flowID := randomHex(16)
	contBytes, _ := json.Marshal(authContinuation{FlowID: flowID})

	switch req.Method {
	case "browser":
		stateData, err := owner.callAPI(ctx, stream, site, http.MethodPost, "/v2/plugin/auth/state?platform="+url.QueryEscape(site.Platform), map[string]string{
			"X-No-Authorization":   "true",
			"X-No-User-Id":         "true",
			"X-No-Enterprise-Id":   "true",
			"X-No-Department-Info": "true",
		}, map[string]any{})
		if err != nil {
			return nil, upstreamErr(err.Error())
		}
		stateToken := strField(stateData, "state")
		authURLRaw := strField(stateData, "authUrl")
		if stateToken == "" || authURLRaw == "" {
			return nil, upstreamErr(fmt.Sprintf("%s gave no sign-in page", site.Name))
		}
		parsedURL, err := url.Parse(authURLRaw)
		if err != nil || parsedURL.Scheme != "https" {
			return nil, upstreamErr(fmt.Sprintf("%s's sign-in page isn't https", site.Name))
		}
		query := parsedURL.Query()
		query.Set("version", site.App)
		query.Set("loginSessionId", randomHex(16))
		parsedURL.RawQuery = query.Encode()
		owner.mu.Lock()
		owner.flows[flowID] = &authFlow{
			site:     site,
			method:   "browser",
			state:    stateToken,
			authURL:  parsedURL.String(),
			deadline: owner.nowMs() + owner.options.SignInWindow.Milliseconds(),
		}
		owner.mu.Unlock()
		return sdk.AuthStartResult{
			URL:          parsedURL.String(),
			PollAfterMs:  owner.options.PollInterval.Milliseconds(),
			Continuation: contBytes,
		}, nil

	case "desktop":
		owner.mu.Lock()
		owner.flows[flowID] = &authFlow{site: site, method: "desktop"}
		owner.mu.Unlock()
		return sdk.AuthStartResult{Continuation: contBytes}, nil

	case "api-key":
		var input struct {
			APIKey string `json:"apiKey"`
		}
		if nonempty(req.Input) {
			_ = json.Unmarshal(req.Input, &input)
		}
		owner.mu.Lock()
		owner.flows[flowID] = &authFlow{site: site, method: "api-key", apiKey: strings.TrimSpace(input.APIKey)}
		owner.mu.Unlock()
		return sdk.AuthStartResult{Continuation: contBytes}, nil

	case sdk.SavedSignInMethod:
		var input struct {
			Record string `json:"record"`
		}
		if nonempty(req.Input) {
			_ = json.Unmarshal(req.Input, &input)
		}
		owner.mu.Lock()
		owner.flows[flowID] = &authFlow{site: site, method: sdk.SavedSignInMethod, record: input.Record}
		owner.mu.Unlock()
		return sdk.AuthStartResult{Continuation: contBytes}, nil

	default:
		return nil, invalid("Unsupported WorkBuddy sign-in method")
	}
}

func (owner *module) pollOnce(ctx context.Context, stream *sdk.Stream, site Site, path string, headers map[string]string, retryCodes []int) (map[string]any, bool, error) {
	data, err := owner.callAPI(ctx, stream, site, http.MethodGet, path, headers, nil)
	if err != nil {
		var wErr *wbError
		if errors.As(err, &wErr) {
			if slices.Contains(retryCodes, wErr.Code) || wErr.Status == http.StatusRequestTimeout || wErr.Status == http.StatusTooManyRequests {
				return nil, true, nil
			}
		}
		return nil, false, err
	}
	if len(data) == 0 {
		return nil, true, nil
	}
	return data, false, nil
}

func (owner *module) authFinish(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var req sdk.AuthFinishRequest
	if decodeObject(call.Params, &req) != nil {
		return nil, invalid("Auth finish requires a JSON object")
	}
	var cont authContinuation
	if decodeObject(req.Continuation, &cont) != nil || cont.FlowID == "" {
		return nil, invalid("Auth finish requires a valid continuation")
	}
	owner.mu.Lock()
	flow, ok := owner.flows[cont.FlowID]
	owner.mu.Unlock()
	if !ok {
		return nil, invalid("Unknown or expired WorkBuddy sign-in flow")
	}
	if call.Scope.Provider != "" && call.Scope.Provider != flow.site.ID {
		return nil, denied("Sign-in continuation belongs to another provider")
	}
	deleteFlow := func() {
		owner.mu.Lock()
		delete(owner.flows, cont.FlowID)
		owner.mu.Unlock()
	}

	switch flow.method {
	case "browser":
		if owner.nowMs() > flow.deadline {
			deleteFlow()
			return nil, upstreamErr(fmt.Sprintf("%s sign-in timed out", flow.site.Name))
		}
		queryState := url.QueryEscape(flow.state)
		tokenData, pending, err := owner.pollOnce(ctx, stream, flow.site, "/v2/plugin/auth/token?state="+queryState, nil, []int{11217})
		if err != nil {
			deleteFlow()
			return nil, upstreamErr(err.Error())
		}
		if pending {
			return sdk.AuthResult{Pending: true}, nil
		}
		if strField(tokenData, "accessToken") == "" {
			deleteFlow()
			return nil, upstreamErr(fmt.Sprintf("%s sent back no token", flow.site.Name))
		}
		acct := mergeToken(Credential{}, tokenData, owner.nowMs())
		var who map[string]any
		for {
			if owner.nowMs() > flow.deadline {
				deleteFlow()
				return nil, upstreamErr(fmt.Sprintf("%s sign-in timed out", flow.site.Name))
			}
			gotWho, whoPending, whoErr := owner.pollOnce(ctx, stream, flow.site, "/v2/plugin/login/account?state="+queryState, map[string]string{
				"Authorization":      "Bearer " + acct.Access,
				"X-Domain":           domainOf(flow.site, acct),
				"X-No-User-Id":       "true",
				"X-No-Enterprise-Id": "true",
			}, []int{12151})
			if whoErr != nil {
				deleteFlow()
				return nil, upstreamErr(whoErr.Error())
			}
			if !whoPending {
				who = gotWho
				break
			}
			if sleepErr := sleepCtx(ctx, owner.options.PollInterval); sleepErr != nil {
				return nil, sleepErr
			}
		}
		deleteFlow()
		acct.UID = strField(who, "uid")
		ent := owner.enterpriseOf(ctx, stream, flow.site, acct, who)
		acct.EnterpriseID = ent.ID
		name := firstNonEmpty(accountName(who, ent), flow.site.Name)
		acct.AccountID = name
		acct.Name = name
		rawCred, _ := json.Marshal(acct)
		return sdk.AuthResult{
			ID:          firstNonEmpty(acct.UID, name),
			Name:        name,
			Credentials: rawCred,
			ExpiresAt:   acct.Expires,
		}, nil

	case "desktop":
		deleteFlow()
		desk, found := owner.readDesktop(flow.site)
		if !found {
			return nil, unauthenticated(owner.notDesktop(flow.site))
		}
		stored := Credential{
			Type:      "oauth",
			Source:    "desktop",
			UID:       desk.UID,
			AccountID: firstNonEmpty(desk.Name, flow.site.Name),
			Name:      firstNonEmpty(desk.Name, flow.site.Name),
		}
		rawCred, _ := json.Marshal(stored)
		return sdk.AuthResult{
			ID:          firstNonEmpty(desk.UID, stored.AccountID),
			Name:        stored.AccountID,
			Credentials: rawCred,
		}, nil

	case "api-key":
		deleteFlow()
		apiKey := flow.apiKey
		if apiKey == "" && nonempty(req.Input) {
			var input struct {
				APIKey string `json:"apiKey"`
			}
			_ = json.Unmarshal(req.Input, &input)
			apiKey = strings.TrimSpace(input.APIKey)
		}
		if apiKey == "" {
			apiKey = strings.TrimSpace(req.Code)
		}
		acct, ok := keyAccount(apiKey)
		if !ok {
			return nil, invalid(fmt.Sprintf("%s API key is required", flow.site.Name))
		}
		name := flow.site.Name + " (" + acct.UID + ")"
		acct.AccountID = name
		acct.Name = name
		rawCred, _ := json.Marshal(acct)
		return sdk.AuthResult{
			ID:          acct.UID,
			Name:        name,
			Credentials: rawCred,
		}, nil

	case sdk.SavedSignInMethod:
		deleteFlow()
		record := flow.record
		if record == "" && nonempty(req.Input) {
			var input struct {
				Record string `json:"record"`
			}
			_ = json.Unmarshal(req.Input, &input)
			record = input.Record
		}
		if record == "" {
			record = req.Code
		}
		acct, name, err := owner.importSingleRecord(ctx, stream, flow.site, record)
		if err != nil {
			return nil, err
		}
		rawCred, _ := json.Marshal(acct)
		return sdk.AuthResult{
			ID:          firstNonEmpty(acct.UID, name),
			Name:        name,
			Credentials: rawCred,
			ExpiresAt:   acct.Expires,
		}, nil

	default:
		deleteFlow()
		return nil, invalid("Unsupported WorkBuddy sign-in method")
	}
}

func (owner *module) authRefresh(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var req sdk.AuthRefreshRequest
	if decodeObject(call.Params, &req) != nil {
		return nil, invalid("Auth refresh requires credentials")
	}
	site, ok := owner.siteFor(call.Scope.Provider)
	if !ok {
		return nil, invalid("Unknown WorkBuddy provider")
	}
	rawAuth := req.Credentials
	if !nonempty(rawAuth) {
		rawAuth = call.Auth
	}
	var cred Credential
	if decodeObject(rawAuth, &cred) != nil {
		return nil, invalid("Invalid WorkBuddy credentials")
	}
	if cred.isAPIKey() || cred.Source == "desktop" || cred.Access == "" || cred.Refresh == "" {
		rawOut, _ := json.Marshal(cred)
		return sdk.AuthResult{
			ID:          firstNonEmpty(cred.UID, cred.AccountID),
			Name:        firstNonEmpty(cred.AccountID, cred.Name, site.Name),
			Credentials: rawOut,
			ExpiresAt:   cred.Expires,
		}, nil
	}
	if !owner.refreshable(cred) {
		return nil, unauthenticated("this WorkBuddy account is signed out; sign in again")
	}
	next, err := owner.renew(ctx, stream, site, cred, false)
	if err != nil {
		var wErr *wbError
		if errors.As(err, &wErr) {
			if wErr.Bare {
				return nil, upstreamErr(wErr.Message)
			}
			if wErr.Said != "" {
				return nil, upstreamErr("WorkBuddy token refresh: " + wErr.Said)
			}
		}
		return nil, upstreamErr("WorkBuddy token refresh: " + err.Error())
	}
	rawOut, _ := json.Marshal(next)
	return sdk.AuthResult{
		ID:          firstNonEmpty(next.UID, next.AccountID),
		Name:        firstNonEmpty(next.AccountID, next.Name, site.Name),
		Credentials: rawOut,
		ExpiresAt:   next.Expires,
	}, nil
}
