package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type ConsoleInstruction struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type ConsoleSkill struct {
	Slug    string `json:"slug"`
	Version string `json:"version"`
}

type ConsoleSnapshot struct {
	Providers    []string             `json:"providers"`
	Policies     json.RawMessage      `json:"policies,omitempty"`
	Instructions []ConsoleInstruction `json:"instructions"`
	Skills       []ConsoleSkill       `json:"skills"`
	Lists        bool                 `json:"lists"`
	Watches      bool                 `json:"watches"`
	Status       string               `json:"status,omitempty"`
	Stale        bool                 `json:"stale,omitempty"`
}

type consoleEntry struct {
	config         json.RawMessage
	configAt       time.Time
	lists          bool
	watches        bool
	instructions   []ConsoleInstruction
	instructionsAt time.Time
	skills         []ConsoleSkill
	skillsAt       time.Time
	bundles        map[string]consoleBundle
}

type consoleBundle struct {
	version string
	body    SkillBundle
}

type watchReceipt struct {
	owner   string
	ids     map[string]bool
	expires time.Time
}

type consoleCache struct {
	gate     chan struct{}
	entries  map[string]*consoleEntry
	receipts map[string]watchReceipt
	catalog  zenLists
}

func newConsoleCache() *consoleCache {
	return &consoleCache{gate: make(chan struct{}, 1), entries: map[string]*consoleEntry{},
		receipts: map[string]watchReceipt{}, catalog: zenLists{values: map[string]zenListing{}}}
}

func (cache *consoleCache) lock(ctx context.Context) error {
	select {
	case cache.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("OpenCode reply exceeds its limit")
	}
	return body, nil
}

func consoleKey(call sdk.Call, identity ZenCredentials) string {
	return call.Scope.Provider + ":" + call.Scope.Account + ":" + strconv.FormatUint(call.Scope.Generation, 10) + ":" + identity.organization()
}

func (owner *module) consoleIdentity(call sdk.Call) (ZenCredentials, error) {
	if !owner.zen || call.Scope.Provider != ProviderZen {
		return ZenCredentials{}, &sdk.Error{Code: "permission_denied", Message: "Console requires a selected personal Zen account"}
	}
	identity, err := selectedZen(call)
	if err != nil {
		return identity, err
	}
	if identity.OAuth != nil && identity.OAuth.Due(owner.options.Now()) {
		return identity, failure(ErrRefreshRequired, "not-started")
	}
	return identity, nil
}

func (owner *module) consoleRequest(ctx context.Context, stream *sdk.Stream, identity ZenCredentials, method, endpoint string, input any, timeout time.Duration) (*http.Response, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	var body io.Reader
	if input != nil {
		value, err := json.Marshal(input)
		if err != nil {
			cancel()
			return nil, nil, err
		}
		body = bytes.NewReader(value)
	}
	req, err := http.NewRequestWithContext(ctx, method, owner.options.Endpoints.Console+endpoint, body)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "magpie-opencode/"+windowVersion+" (console-inspection; opencode-schema/"+ClientVersion+")")
	req.Header.Set("Authorization", "Bearer "+identity.access())
	if identity.organization() != "" {
		req.Header.Set("x-org-id", identity.organization())
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := owner.client(stream).HTTP.Do(req)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return response, cancel, nil
}

func headerPresent(headers http.Header, name string) bool {
	for key := range headers {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func consoleStatus(status int, body []byte) string {
	var problem struct {
		Tag          string `json:"_tag"`
		OrgID        string `json:"orgId"`
		ConnectionID string `json:"connectionId"`
	}
	if json.Unmarshal(body, &problem) != nil {
		return ""
	}
	if status == 401 && problem.Tag == "Unauthorized" {
		return "needs_auth"
	}
	if status == 403 && problem.Tag == "SsoRequired" && problem.OrgID != "" && problem.ConnectionID != "" {
		return "needs_sso"
	}
	return ""
}

func (owner *module) consoleSnapshot(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if len(call.Params) != 0 && string(call.Params) != "{}" && string(call.Params) != "null" {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Console snapshot accepts no substituted authority"}
	}
	identity, err := owner.consoleIdentity(call)
	if err != nil {
		return nil, err
	}
	state, err := currentInstance(ctx)
	if err != nil {
		return nil, err
	}
	cache := state.console
	if err := cache.lock(ctx); err != nil {
		return nil, err
	}
	defer func() { <-cache.gate }()
	key := consoleKey(call, identity)
	entry := cache.entries[key]
	if entry == nil {
		if len(cache.entries) >= 128 {
			clear(cache.entries)
		}
		entry = &consoleEntry{bundles: map[string]consoleBundle{}}
		cache.entries[key] = entry
	}
	now := owner.options.Now()
	stale, status := false, ""
	if entry.configAt.IsZero() || now.Sub(entry.configAt) >= time.Minute {
		response, cancel, err := owner.consoleRequest(ctx, stream, identity, http.MethodGet, "/api/v2/config", nil, 5*time.Second)
		if err != nil {
			stale = true
		} else {
			body, readErr := readBounded(response.Body, maxReplyBytes)
			response.Body.Close()
			cancel()
			var document map[string]json.RawMessage
			switch {
			case response.StatusCode == 404:
				*entry = consoleEntry{configAt: now, bundles: map[string]consoleBundle{}}
			case readErr == nil && response.StatusCode == 200 && json.Unmarshal(body, &document) == nil && validConsoleDocument(document):
				entry.config, _ = json.Marshal(document)
				entry.configAt = now
				entry.lists = headerPresent(response.Header, "x-opencode-instructions-skills")
				entry.watches = headerPresent(response.Header, "x-opencode-event-watches")
			default:
				stale, status = true, consoleStatus(response.StatusCode, body)
			}
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !entry.lists {
		entry.instructions, entry.skills = nil, nil
		entry.instructionsAt, entry.skillsAt = time.Time{}, time.Time{}
		clear(entry.bundles)
	} else {
		var group sync.WaitGroup
		var instructionErr, skillErr error
		if entry.instructionsAt.IsZero() || now.Sub(entry.instructionsAt) >= time.Minute {
			group.Go(func() {
				var body struct {
					Instructions []struct {
						ID      *string `json:"id"`
						Content *string `json:"content"`
					} `json:"instructions"`
				}
				instructionErr = owner.consoleList(ctx, stream, identity, "/api/v2/instructions", &body)
				if instructionErr == nil {
					instructions := []ConsoleInstruction{}
					for _, item := range body.Instructions {
						if item.ID == nil || item.Content == nil {
							instructionErr = errors.New("invalid Console instruction")
							return
						}
						instructions = append(instructions, ConsoleInstruction{ID: *item.ID, Content: *item.Content})
					}
					entry.instructions, entry.instructionsAt = instructions, now
				}
			})
		}
		if entry.skillsAt.IsZero() || now.Sub(entry.skillsAt) >= time.Minute {
			group.Go(func() {
				var body struct {
					Skills *[]ConsoleSkill `json:"skills"`
				}
				skillErr = owner.consoleList(ctx, stream, identity, "/api/v2/skills", &body)
				if skillErr == nil {
					if body.Skills == nil {
						body.Skills = new([]ConsoleSkill)
					}
					for _, item := range *body.Skills {
						if !safeSegment(item.Slug) || item.Version == "" {
							skillErr = errors.New("invalid Console skill")
							return
						}
					}
					entry.skills, entry.skillsAt = *body.Skills, now
					for slug := range entry.bundles {
						if !slices.ContainsFunc(entry.skills, func(skill ConsoleSkill) bool { return skill.Slug == slug }) {
							delete(entry.bundles, slug)
						}
					}
				}
			})
		}
		group.Wait()
		stale = stale || instructionErr != nil || skillErr != nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	result := ConsoleSnapshot{Providers: []string{}, Instructions: append([]ConsoleInstruction{}, entry.instructions...),
		Skills: append([]ConsoleSkill{}, entry.skills...), Lists: entry.lists,
		Watches: entry.watches && state.config.ConsoleWatches, Status: status, Stale: stale}
	var document struct {
		Providers map[string]json.RawMessage `json:"providers"`
		Policies  json.RawMessage            `json:"policies"`
	}
	if json.Unmarshal(entry.config, &document) == nil {
		for id := range document.Providers {
			result.Providers = append(result.Providers, id)
		}
		sort.Strings(result.Providers)
		result.Policies = document.Policies
	}
	return result, nil
}

func (owner *module) consoleList(ctx context.Context, stream *sdk.Stream, identity ZenCredentials, endpoint string, out any) error {
	response, cancel, err := owner.consoleRequest(ctx, stream, identity, http.MethodGet, endpoint, nil, 5*time.Second)
	if err != nil {
		return err
	}
	defer cancel()
	defer response.Body.Close()
	body, err := readBounded(response.Body, maxReplyBytes)
	if err != nil {
		return err
	}
	if response.StatusCode == 404 || response.StatusCode == 403 && consoleStatus(403, body) != "needs_sso" {
		if endpoint == "/api/v2/instructions" {
			return json.Unmarshal([]byte(`{"instructions":[]}`), out)
		}
		return json.Unmarshal([]byte(`{"skills":[]}`), out)
	}
	if response.StatusCode != 200 {
		return errors.New("Console list unavailable")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || fields == nil ||
		endpoint == "/api/v2/instructions" && (len(fields["instructions"]) == 0 || bytes.Equal(fields["instructions"], []byte("null"))) ||
		endpoint == "/api/v2/skills" && (len(fields["skills"]) == 0 || bytes.Equal(fields["skills"], []byte("null"))) {
		return errors.New("Console list is malformed")
	}
	return json.Unmarshal(body, out)
}

type SkillFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type SkillBundle struct {
	Slug    string      `json:"slug"`
	Files   []SkillFile `json:"files"`
	Version string      `json:"version,omitempty"`
	Stale   bool        `json:"stale,omitempty"`
}

func safeSegment(value string) bool {
	return value != "" && value != "." && value != ".." && len(value) <= 256 && !strings.ContainsAny(value, "/\\\x00?#:%") &&
		!strings.ContainsFunc(value, func(character rune) bool { return character < 0x20 || character == 0x7f })
}

func safeRelative(value string) bool {
	if value == "" || strings.ContainsAny(value, "\\\x00?#:") || path.IsAbs(value) {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil || !safeSegment(decoded) {
			return false
		}
	}
	return true
}

func (owner *module) consoleSkill(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request struct {
		Slug string `json:"slug"`
	}
	if decode(call.Params, &request) != nil || !safeSegment(request.Slug) {
		return nil, &sdk.Error{Code: "invalid_params", Message: "Console skill requires a safe listed slug"}
	}
	identity, err := owner.consoleIdentity(call)
	if err != nil {
		return nil, err
	}
	snapshotCall := call
	snapshotCall.Params = json.RawMessage("{}")
	snapshot, err := owner.consoleSnapshot(ctx, snapshotCall, stream)
	if err != nil {
		return nil, err
	}
	if snapshot.(ConsoleSnapshot).Status != "" {
		return nil, &sdk.Error{Code: "permission_denied", Message: "Console skill requires renewed workspace authentication"}
	}
	state, _ := currentInstance(ctx)
	cache := state.console
	if err := cache.lock(ctx); err != nil {
		return nil, err
	}
	defer func() { <-cache.gate }()
	entry := cache.entries[consoleKey(call, identity)]
	var version string
	for _, skill := range entry.skills {
		if skill.Slug == request.Slug {
			version = skill.Version
		}
	}
	if version == "" {
		return nil, &sdk.Error{Code: "permission_denied", Message: "Console skill is not listed for the selected workspace"}
	}
	if bundle, found := entry.bundles[request.Slug]; found && bundle.version == version {
		return bundle.body, nil
	}
	bundle, err := owner.downloadSkill(ctx, stream, identity, request.Slug)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if previous, found := entry.bundles[request.Slug]; found {
			body := previous.body
			body.Stale = true
			return body, nil
		}
		return nil, failure(err, "")
	}
	if len(entry.bundles) >= 128 {
		clear(entry.bundles)
	}
	bundle.Version, bundle.Stale = version, false
	entry.bundles[request.Slug] = consoleBundle{version: version, body: bundle}
	return bundle, nil
}

func (owner *module) downloadSkill(ctx context.Context, stream *sdk.Stream, identity ZenCredentials, slug string) (SkillBundle, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	target := owner.options.Endpoints.Console + "/api/v2/skills/" + url.PathEscape(slug)
	headers := http.Header{"Authorization": {"Bearer " + identity.access()}, "Accept": {"application/json"},
		"User-Agent": {"opencode/latest/" + ClientVersion + "/cli"}}
	if identity.organization() != "" {
		headers.Set("x-org-id", identity.organization())
	}
	for hops := 0; ; hops++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return SkillBundle{}, err
		}
		req.Header = headers.Clone()
		response, err := owner.client(stream).HTTP.Do(req)
		if err != nil {
			return SkillBundle{}, err
		}
		body, readErr := readBounded(response.Body, maxReplyBytes)
		response.Body.Close()
		if slices.Contains([]int{301, 302, 303, 307, 308}, response.StatusCode) && response.Header.Get("Location") != "" {
			if hops >= 5 {
				return SkillBundle{}, errors.New("Console skill redirect limit exceeded")
			}
			next, err := req.URL.Parse(response.Header.Get("Location"))
			if err != nil || next.User != nil || next.Host == "" || next.Fragment != "" ||
				(Endpoints{Console: next.Scheme + "://" + next.Host, Inference: next.Scheme + "://" + next.Host}).validate() != nil {
				return SkillBundle{}, errors.New("unsafe Console skill redirect")
			}
			if next.Scheme != req.URL.Scheme || next.Host != req.URL.Host {
				ClearAuth(headers)
			}
			target = next.String()
			continue
		}
		if readErr != nil || response.StatusCode != 200 {
			return SkillBundle{}, errors.New("Console skill download failed")
		}
		var fields struct {
			Slug  *string `json:"slug"`
			Files []struct {
				Path    *string `json:"path"`
				Content *string `json:"content"`
			} `json:"files"`
		}
		if json.Unmarshal(body, &fields) != nil || fields.Slug == nil {
			return SkillBundle{}, errors.New("invalid Console skill fields")
		}
		for _, file := range fields.Files {
			if file.Path == nil || file.Content == nil {
				return SkillBundle{}, errors.New("invalid Console skill file fields")
			}
		}
		var bundle SkillBundle
		if json.Unmarshal(body, &bundle) != nil || bundle.Slug != slug || len(bundle.Files) == 0 || len(bundle.Files) > 256 {
			return bundle, errors.New("invalid Console skill bundle")
		}
		main := false
		names := map[string]bool{}
		for _, file := range bundle.Files {
			if !safeRelative(file.Path) || names[file.Path] {
				return SkillBundle{}, errors.New("unsafe Console skill file")
			}
			names[file.Path] = true
			if file.Path == "SKILL.md" {
				main = validSkillFrontmatter(file.Content)
			}
		}
		if !main {
			return SkillBundle{}, errors.New("Console skill requires SKILL.md frontmatter")
		}
		return bundle, nil
	}
}

func validConsoleDocument(document map[string]json.RawMessage) bool {
	if document == nil {
		return false
	}
	if value, found := document["providers"]; found {
		var providers map[string]map[string]json.RawMessage
		if json.Unmarshal(value, &providers) != nil || providers == nil {
			return false
		}
		for _, provider := range providers {
			if provider == nil {
				return false
			}
		}
	}
	if value, found := document["policies"]; found {
		var policies []map[string]json.RawMessage
		if json.Unmarshal(value, &policies) != nil || policies == nil {
			return false
		}
		filtered := []map[string]string{}
		for _, policy := range policies {
			var action, effect, resource string
			if policy == nil || json.Unmarshal(policy["action"], &action) != nil {
				return false
			}
			if !slices.Contains([]string{"provider.use", "tool.use", "integration.connect", "integration.use", "mcp.use", "skill.use", "plugin.use", "websearch.use"}, action) {
				continue
			}
			if json.Unmarshal(policy["effect"], &effect) != nil {
				return false
			}
			if effect != "allow" && effect != "deny" {
				continue
			}
			if json.Unmarshal(policy["resource"], &resource) != nil {
				return false
			}
			if action == "integration.use" {
				prefix, rest, found := strings.Cut(resource, ":")
				if found && slices.Contains([]string{"mcp", "skill", "plugin"}, prefix) {
					action, resource = prefix+".use", rest
				}
			}
			filtered = append(filtered, map[string]string{"action": action, "effect": effect, "resource": resource})
		}
		document["policies"], _ = json.Marshal(filtered)
	}
	return true
}
