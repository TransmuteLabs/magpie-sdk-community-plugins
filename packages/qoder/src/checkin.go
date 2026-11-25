package qoder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

var (
	campaignClaimPathRe = regexp.MustCompile(`^/sash/api/v1/me/campaigns/[^/]+/claim$`)
	printableASCIIRe    = regexp.MustCompile(`^[\x20-\x7e]+$`)
)

type runtimeSpec struct {
	Name        string
	Environment string
	Override    string
	App         string
	Linux       string
	CLI         string
	Dir         string
	Required    bool
}

var runtimeSpecs = map[string]runtimeSpec{
	ProviderQoder: {
		Name:        "Qoder",
		Environment: "3",
		Override:    "QODER_RUNTIME_INFO",
		App:         "Qoder",
		Linux:       "Qoder",
		CLI:         "QODER_",
		Dir:         ".qoder",
		Required:    true,
	},
	ProviderQoderCN: {
		Name:        "Qoder CN",
		Environment: "0",
		Override:    "QODERCN_RUNTIME_INFO",
		App:         "Qoder CN",
		Linux:       "Qoder-CN",
		CLI:         "QODERCN_",
		Dir:         ".qoder-cn",
		Required:    false,
	},
}

type runtimeMissingError struct {
	Spec runtimeSpec
}

func (err *runtimeMissingError) Error() string {
	spec := err.Spec
	return fmt.Sprintf(
		"%s check-in: device identity runtime not found. %s gives the daily credits only to a device its own app identifies, and this machine has neither %s desktop nor %s CLI to read that identity from. Install %s desktop (or install %s CLI and sign in to it once), or set %s to its runtime-info executable; magpie checks in again the next day",
		spec.Name, spec.Name, spec.Name, spec.Name, spec.Name, spec.Name, spec.Override,
	)
}

func noCampaignMessage(siteName string) string {
	return fmt.Sprintf(
		"%s check-in: %s offered this device's identity, read from this machine's own %s runtime-info, no daily credits campaign right now. %s decides per device whether to offer it; if %s desktop shows today's credits, claim them there",
		siteName, siteName, siteName, siteName, siteName,
	)
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode().IsRegular()
}

func (owner *module) campaignRuntime(spec runtimeSpec) (string, error) {
	if override := strings.TrimSpace(owner.envValue(spec.Override)); override != "" {
		abs, err := filepath.Abs(override)
		if err == nil {
			return abs, nil
		}
		return override, nil
	}
	platform := owner.options.Platform
	arch := owner.options.Arch
	home := owner.options.HomeDir()
	optDir := owner.options.OptDir
	binName := "runtime-info"
	if platform == "win32" {
		binName = "runtime-info.exe"
	}

	var candidates []string
	switch platform {
	case "win32":
		localApp := owner.envValue("LOCALAPPDATA")
		if localApp == "" && home != "" {
			localApp = filepath.Join(home, "AppData", "Local")
		}
		if localApp != "" {
			candidates = append(candidates, filepath.Join(localApp, "Programs", spec.App, "resources", "umid", binName))
		}
		progFiles := firstNonEmpty(owner.envValue("ProgramFiles"), owner.envValue("PROGRAMFILES"))
		for _, baseDir := range []string{progFiles, owner.envValue("ProgramW6432")} {
			if baseDir != "" {
				candidates = append(candidates, filepath.Join(baseDir, spec.App, "resources", "umid", binName))
			}
		}
	case "darwin":
		dirs := []string{"/Applications"}
		if home != "" {
			dirs = append(dirs, filepath.Join(home, "Applications"))
		}
		for _, baseDir := range dirs {
			candidates = append(candidates, filepath.Join(baseDir, spec.App+".app", "Contents", "Resources", "umid", binName))
		}
	case "linux":
		candidates = append(candidates, filepath.Join(optDir, spec.Linux, "resources", "umid", binName))
	}
	for _, candidate := range candidates {
		if isRegularFile(candidate) {
			return candidate, nil
		}
	}

	configDir := owner.envValue(spec.CLI + "CONFIG_DIR")
	if configDir == "" {
		cliHome := firstNonEmpty(owner.envValue(spec.CLI+"CLI_HOME"), owner.envValue("GEMINI_CLI_HOME"), home)
		dirName := firstNonEmpty(owner.envValue(spec.CLI+"CONFIG_DIR_NAME"), spec.Dir)
		if cliHome != "" {
			configDir = filepath.Join(cliHome, dirName)
		}
	}
	if configDir != "" {
		cacheDir := filepath.Join(configDir, ".bin")
		entries, err := os.ReadDir(cacheDir)
		if err == nil {
			type foundBin struct {
				path  string
				mtime int64
			}
			var found []foundBin
			for _, entry := range entries {
				var binPath string
				entryName := entry.Name()
				if platform == "win32" && entry.IsDir() {
					if strings.HasPrefix(entryName, "umid-win32-"+arch+"-") {
						binPath = filepath.Join(cacheDir, entryName, binName)
					}
				} else if platform == "win32" {
					if !entry.IsDir() && strings.HasPrefix(entryName, "runtime-info-win32-"+arch+"-") && strings.HasSuffix(strings.ToLower(entryName), ".exe") {
						binPath = filepath.Join(cacheDir, entryName)
					}
				} else if !entry.IsDir() && strings.HasPrefix(entryName, "runtime-info-"+platform+"-"+arch+"-") && !strings.HasSuffix(entryName, ".tmp") {
					binPath = filepath.Join(cacheDir, entryName)
				}
				if binPath != "" && isRegularFile(binPath) {
					if info, statErr := os.Stat(binPath); statErr == nil {
						found = append(found, foundBin{path: binPath, mtime: info.ModTime().UnixNano()})
					}
				}
			}
			if len(found) > 0 {
				sort.Slice(found, func(left, right int) bool {
					return found[left].mtime > found[right].mtime
				})
				return found[0].path, nil
			}
		}
	}
	return "", &runtimeMissingError{Spec: spec}
}

func (owner *module) campaignMachine(ctx context.Context, uid string, spec runtimeSpec) (map[string]string, error) {
	if strings.TrimSpace(uid) == "" {
		return nil, errors.New("Qoder check-in: device identity needs the account uid; sign in again")
	}
	binPath, err := owner.campaignRuntime(spec)
	if err != nil {
		return nil, err
	}
	var args []string
	var stdinPayload []byte
	if owner.options.Platform == "linux" {
		args = []string{spec.Environment}
	} else {
		args = []string{spec.Environment, "--account-stdin"}
		encoded, _ := json.Marshal(map[string]string{"account": uid})
		stdinPayload = append(encoded, '\n')
	}

	runCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var rawOut []byte
	if owner.options.RunRuntimeInfo != nil {
		rawOut, err = owner.options.RunRuntimeInfo(runCtx, binPath, args, stdinPayload)
	} else {
		cmd := exec.CommandContext(runCtx, binPath, args...)
		if len(stdinPayload) > 0 {
			cmd.Stdin = bytes.NewReader(stdinPayload)
		}
		var stdoutBuf bytes.Buffer
		cmd.Stdout = &limitedBuffer{buf: &stdoutBuf, max: 1 << 20}
		cmd.Stderr = io.Discard
		err = cmd.Run()
		rawOut = stdoutBuf.Bytes()
	}
	if err != nil {
		return nil, fmt.Errorf("%s check-in: could not read device identity; restart or update %s, or check %s", spec.Name, spec.Name, spec.Override)
	}

	firstLine := strings.TrimSpace(string(rawOut))
	if nlIdx := strings.IndexAny(firstLine, "\r\n"); nlIdx >= 0 {
		firstLine = firstLine[:nlIdx]
	}
	var identity map[string]any
	if json.Unmarshal([]byte(firstLine), &identity) != nil || identity == nil {
		return nil, fmt.Errorf("%s check-in: invalid device identity from %s; restart or update %s", spec.Name, spec.Name, spec.Name)
	}
	headers := map[string]string{}
	pairs := [][2]string{
		{"Cosy-MachineToken", "machineToken"},
		{"Cosy-MachineType", "machineType"},
	}
	for _, pair := range pairs {
		val := strings.TrimSpace(rawStrField(identity, pair[1]))
		if val == "" || len(val) > 4096 || !printableASCIIRe.MatchString(val) {
			return nil, fmt.Errorf("%s check-in: invalid device identity from %s; restart or update %s", spec.Name, spec.Name, spec.Name)
		}
		headers[pair[0]] = val
	}
	return headers, nil
}

type limitedBuffer struct {
	buf *bytes.Buffer
	max int
}

func (lb *limitedBuffer) Write(data []byte) (int, error) {
	rem := lb.max - lb.buf.Len()
	if rem > 0 {
		chunk := data
		if len(chunk) > rem {
			chunk = chunk[:rem]
		}
		_, _ = lb.buf.Write(chunk)
	}
	return len(data), nil
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for key, val := range src {
		out[key] = val
	}
	return out
}

func (owner *module) machineForCampaign(ctx context.Context, state *providerRuntimeState, uid string, spec runtimeSpec) (map[string]string, error) {
	nowMs := owner.nowMs()
	state.stateMu.Lock()
	flight := state.idFlights[uid]
	if flight == nil || nowMs-flight.At >= campaignMachineTTL {
		flight = &runtimeIdentityFlight{
			At:   nowMs,
			Done: make(chan struct{}),
		}
		state.idFlights[uid] = flight
		state.stateMu.Unlock()

		headers, err := owner.campaignMachine(ctx, uid, spec)
		state.stateMu.Lock()
		flight.Headers = copyStringMap(headers)
		flight.Err = err
		if err == nil {
			if state.idFlights[uid] == flight {
				state.identities[uid] = &runtimeIdentityCache{
					At:      flight.At,
					Headers: copyStringMap(headers),
				}
			}
		} else if state.idFlights[uid] == flight {
			delete(state.idFlights, uid)
		}
		close(flight.Done)
		state.stateMu.Unlock()
		return copyStringMap(headers), err
	}
	state.stateMu.Unlock()

	select {
	case <-flight.Done:
		return copyStringMap(flight.Headers), flight.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (owner *module) chatIdentity(site Site, state *providerRuntimeState, uid string) map[string]string {
	if uid == "" {
		return nil
	}
	spec, hasSpec := runtimeSpecs[site.ID]
	state.stateMu.Lock()
	known := state.identities[uid]
	if known == nil {
		state.stateMu.Unlock()
		return nil
	}
	headers := copyStringMap(known.Headers)
	nowMs := owner.nowMs()
	lastTry := known.Tried
	if lastTry == 0 {
		lastTry = known.At
	}
	shouldRefresh := hasSpec && nowMs-lastTry >= campaignMachineTTL
	if shouldRefresh {
		known.Tried = nowMs
		state.bgWG.Add(1)
	}
	state.stateMu.Unlock()

	if shouldRefresh {
		bgCtx := owner.backgroundContext()
		go func() {
			defer state.bgWG.Done()
			_, _ = owner.machineForCampaign(bgCtx, state, uid, spec)
		}()
	}
	return headers
}

func (owner *module) campaignIdentity(ctx context.Context, state *providerRuntimeState, uid string, spec runtimeSpec) (map[string]string, error, error) {
	headers, err := owner.machineForCampaign(ctx, state, uid, spec)
	if spec.Required {
		return headers, nil, err
	}
	if err != nil {
		return nil, err, nil
	}
	return headers, nil, nil
}

func isCampaignPage(site Site, rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	base, err := url.Parse(site.OpenAPI)
	if err != nil {
		return false
	}
	if parsed.Scheme != base.Scheme || parsed.Host != base.Host {
		return false
	}
	return parsed.Path == campaignsPath || campaignClaimPathRe.MatchString(parsed.Path)
}

func campaignTarget(base Site, resolved Site, rawURL string) string {
	if isCampaignPage(resolved, rawURL) {
		return rawURL
	}
	if resolved.OpenAPI == base.OpenAPI || !isCampaignPage(base, rawURL) {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	target, err := url.Parse(resolved.OpenAPI)
	if err != nil {
		return ""
	}
	parsed.Scheme = target.Scheme
	parsed.Host = target.Host
	return parsed.String()
}

func (owner *module) campaignCall(ctx context.Context, stream *sdk.Stream, proxyOverride, targetURL, method, body, token string, machine map[string]string) (*http.Response, func(), error) {
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	var bodyReader io.Reader
	if method != http.MethodGet {
		payload := body
		if payload == "" {
			payload = "{}"
		}
		bodyReader = strings.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(reqCtx, method, targetURL, bodyReader)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Cosy-ClientType", "10")
	req.Header.Set("User-Agent", "Qoder")
	for key, val := range machine {
		req.Header.Set(key, val)
	}
	resp, cleanup, err := owner.doHTTP(reqCtx, stream, proxyOverride, req)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return resp, func() {
		cleanup()
		cancel()
	}, nil
}

func atTimeMs(raw any) (int64, bool) {
	if raw == nil {
		return 0, false
	}
	if text, ok := raw.(string); ok && strings.TrimSpace(text) == "" {
		return 0, false
	}
	val, ok := floatField(map[string]any{"v": raw}, "v")
	if !ok || math.IsNaN(val) || math.IsInf(val, 0) || val <= 0 {
		return 0, false
	}
	if val > 1e12 {
		return int64(val), true
	}
	return int64(val * 1000), true
}

func nextOpensMs(campaign map[string]any) (int64, bool) {
	if campaign == nil {
		return 0, false
	}
	endMs, hasEnd := atTimeMs(campaign["endAt"])
	if !hasEnd {
		return 0, false
	}
	startMs, hasStart := atTimeMs(campaign["startAt"])
	if !hasStart {
		return endMs, true
	}
	dayAfter := startMs + dayMs
	if dayAfter > endMs {
		return dayAfter, true
	}
	return endMs, true
}

func askAgainUntil(targetMs int64, ok bool) string {
	if !ok || targetMs <= 0 {
		return ""
	}
	return time.UnixMilli(targetMs + openRoomMs).UTC().Format(time.RFC3339Nano)
}

func (owner *module) checkin(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	site, state, err := owner.siteFor(call.Scope.Provider)
	if err != nil {
		return nil, err
	}
	if !site.Checkin {
		return sdk.AccountCheckinResult{
			Outcome: "ineligible",
			Message: fmt.Sprintf("%s has no daily check-in", site.Name),
		}, nil
	}
	spec, hasSpec := runtimeSpecs[site.ID]
	if !hasSpec {
		return sdk.AccountCheckinResult{
			Outcome: "ineligible",
			Message: fmt.Sprintf("%s has no daily check-in", site.Name),
		}, nil
	}
	var req sdk.AccountCheckinRequest
	if nonemptyRaw(call.Params) {
		if err := json.Unmarshal(call.Params, &req); err != nil {
			return nil, invalid("Invalid account check-in request")
		}
		if req.Day != "" {
			if err := req.Validate(); err != nil {
				return nil, invalid(err.Error())
			}
		}
	}
	stored, hasCred := decodeCredential(call.Auth)
	if !hasCred {
		return sdk.AccountCheckinResult{Outcome: "failed", Message: fmt.Sprintf("%s: not signed in", site.Name)}, nil
	}
	if stored.Lingma {
		return sdk.AccountCheckinResult{
			Outcome: "inactive",
			Message: "a Lingma 1.0 dedicated domain has no daily credits",
		}, nil
	}
	cred, _, err := owner.fresh(ctx, stream, req.Proxy, site, state, stored)
	if err != nil {
		var gone *signInGoneError
		if errors.As(err, &gone) && gone.Expired {
			return nil, unauthenticated(gone.Message)
		}
		return sdk.AccountCheckinResult{Outcome: "failed", Message: err.Error()}, nil
	}
	resolved, err := siteOf(site, cred)
	if err != nil {
		return sdk.AccountCheckinResult{Outcome: "failed", Message: err.Error()}, nil
	}
	baseURL := campaignTarget(site, resolved, site.OpenAPI+campaignsPath)
	if baseURL == "" {
		return sdk.AccountCheckinResult{Outcome: "inactive", Message: "no campaigns page for this account"}, nil
	}

	machine, unreadErr, fatalErr := owner.campaignIdentity(ctx, state, cred.UID, spec)
	if fatalErr != nil {
		var missing *runtimeMissingError
		if errors.As(fatalErr, &missing) {
			return sdk.AccountCheckinResult{Outcome: "ineligible", Message: missing.Error()}, nil
		}
		return sdk.AccountCheckinResult{Outcome: "failed", Message: fatalErr.Error()}, nil
	}

	patToken := ""
	if cred.PATAuth && cred.DeviceToken == "" {
		patToken = cred.Access
	}

	doCampaign := func(targetURL, method, body string) (int, []byte, error) {
		tok := patToken
		if tok == "" {
			tok = cred.DeviceToken
		}
		resp, cleanup, callErr := owner.campaignCall(ctx, stream, req.Proxy, targetURL, method, body, tok, machine)
		if callErr != nil {
			return 0, nil, callErr
		}
		raw, readErr := readBodyLimited(resp.Body, 4<<20)
		status := resp.StatusCode
		_ = resp.Body.Close()
		cleanup()
		if readErr != nil {
			return 0, nil, readErr
		}
		if patToken == "" && (status == http.StatusUnauthorized || status == http.StatusForbidden) {
			nextCred, rotatedToken, rotErr := owner.rotateDeviceToken(ctx, stream, req.Proxy, site, state, stored, cred.DeviceToken)
			if rotErr != nil {
				return 0, nil, rotErr
			}
			cred = nextCred
			cred.DeviceToken = rotatedToken
			resp2, cleanup2, callErr2 := owner.campaignCall(ctx, stream, req.Proxy, targetURL, method, body, rotatedToken, machine)
			if callErr2 != nil {
				return 0, nil, callErr2
			}
			raw2, readErr2 := readBodyLimited(resp2.Body, 4<<20)
			status2 := resp2.StatusCode
			_ = resp2.Body.Close()
			cleanup2()
			if readErr2 != nil {
				return 0, nil, readErr2
			}
			return status2, raw2, nil
		}
		return status, raw, nil
	}

	listStatus, listRaw, err := doCampaign(baseURL, http.MethodGet, "")
	if err != nil {
		var gone *signInGoneError
		if errors.As(err, &gone) && gone.Expired {
			return nil, unauthenticated(gone.Message)
		}
		return sdk.AccountCheckinResult{Outcome: "failed", Message: err.Error()}, nil
	}
	var listObj map[string]any
	if json.Unmarshal(listRaw, &listObj) != nil || listObj == nil {
		snippet := strings.TrimSpace(string(listRaw))
		if len(snippet) > 120 {
			snippet = snippet[:120]
		}
		if snippet == "" {
			snippet = "(empty)"
		}
		return sdk.AccountCheckinResult{
			Outcome: "failed",
			Message: "campaigns: unreadable reply " + snippet,
		}, nil
	}
	if listStatus != http.StatusOK {
		return sdk.AccountCheckinResult{
			Outcome: "failed",
			Message: fmt.Sprintf("campaigns: HTTP %d", listStatus),
		}, nil
	}

	nowMs := owner.nowMs()
	rawCampaigns, _ := listObj["campaigns"].([]any)
	var campaigns []map[string]any
	for _, item := range rawCampaigns {
		camp, ok := item.(map[string]any)
		if ok && camp != nil && rawStrField(camp, "actionType") == "CLAIM_BENEFIT" {
			campaigns = append(campaigns, camp)
		}
	}

	var claimable map[string]any
	for _, camp := range campaigns {
		if rawStrField(camp, "claimStatus") == "CLAIMABLE" {
			claimable = camp
			break
		}
	}
	if claimable == nil {
		for _, camp := range campaigns {
			if rawStrField(camp, "claimStatus") != "CLAIMED" {
				continue
			}
			startMs, hasStart := atTimeMs(camp["startAt"])
			endMs, hasEnd := atTimeMs(camp["endAt"])
			if (!hasStart || startMs <= nowMs) && (!hasEnd || nowMs < endMs) {
				result := sdk.AccountCheckinResult{
					Outcome: "done",
					Until:   askAgainUntil(nextOpensMs(camp)),
				}
				if benefitObj, ok := camp["benefit"].(map[string]any); ok {
					if amt, ok := floatField(benefitObj, "amount"); ok && amt > 0 {
						result.Credit = ptrFloat(amt)
					}
				}
				return result, nil
			}
		}
		if len(campaigns) > 0 {
			var minNext int64
			hasNext := false
			consider := func(ts int64, ok bool) {
				if !ok || ts <= nowMs {
					return
				}
				if !hasNext || ts < minNext {
					minNext = ts
					hasNext = true
				}
			}
			for _, camp := range campaigns {
				startMs, hasStart := atTimeMs(camp["startAt"])
				consider(startMs, hasStart)
				openMs, hasOpen := nextOpensMs(camp)
				consider(openMs, hasOpen)
			}
			return sdk.AccountCheckinResult{
				Outcome: "failed",
				Message: "campaigns: today's credits aren't offered yet",
				Until:   askAgainUntil(minNext, hasNext),
			}, nil
		}
		if unreadErr != nil {
			var missing *runtimeMissingError
			if errors.As(unreadErr, &missing) {
				return sdk.AccountCheckinResult{Outcome: "ineligible", Message: missing.Error()}, nil
			}
			return sdk.AccountCheckinResult{Outcome: "failed", Message: unreadErr.Error()}, nil
		}
		return sdk.AccountCheckinResult{
			Outcome: "inactive",
			Message: noCampaignMessage(site.Name),
		}, nil
	}

	campID := rawStrField(claimable, "campaignId")
	claimURL := strings.TrimRight(baseURL, "/") + "/" + url.PathEscape(campID) + "/claim"
	claimStatus, claimRaw, err := doCampaign(claimURL, http.MethodPost, "{}")
	if err != nil {
		var gone *signInGoneError
		if errors.As(err, &gone) && gone.Expired {
			return nil, unauthenticated(gone.Message)
		}
		return sdk.AccountCheckinResult{Outcome: "failed", Message: err.Error()}, nil
	}
	var claimObj map[string]any
	if json.Unmarshal(claimRaw, &claimObj) != nil || claimObj == nil {
		snippet := strings.TrimSpace(string(claimRaw))
		if len(snippet) > 120 {
			snippet = snippet[:120]
		}
		if snippet == "" {
			snippet = "(empty)"
		}
		return sdk.AccountCheckinResult{
			Outcome: "failed",
			Message: "claim: unreadable reply " + snippet,
		}, nil
	}
	if claimStatus != http.StatusOK {
		return sdk.AccountCheckinResult{
			Outcome: "failed",
			Message: fmt.Sprintf("claim: HTTP %d", claimStatus),
		}, nil
	}
	said := rawStrField(claimObj, "status")
	dataObj, _ := claimObj["data"].(map[string]any)
	if said == "" && dataObj != nil {
		said = rawStrField(dataObj, "status")
	}
	if said == "CLAIMED" {
		result := sdk.AccountCheckinResult{
			Outcome: "claimed",
			Until:   askAgainUntil(nextOpensMs(claimable)),
		}
		var amt float64
		var hasAmt bool
		if ben, ok := claimObj["benefit"].(map[string]any); ok {
			amt, hasAmt = floatField(ben, "amount")
		}
		if !hasAmt && dataObj != nil {
			if ben, ok := dataObj["benefit"].(map[string]any); ok {
				amt, hasAmt = floatField(ben, "amount")
			}
		}
		if !hasAmt {
			if ben, ok := claimable["benefit"].(map[string]any); ok {
				amt, hasAmt = floatField(ben, "amount")
			}
		}
		if hasAmt && amt > 0 {
			result.Credit = ptrFloat(amt)
		}
		return result, nil
	}
	if said == "" {
		said = "unknown answer"
	}
	return sdk.AccountCheckinResult{
		Outcome: "failed",
		Message: "claim: " + said,
	}, nil
}
