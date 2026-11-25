package kimiwork

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"sync"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

var (
	invalidModelIDPattern = regexp.MustCompile(`[\x00-\x20/]`)
	validEffortPattern    = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	allowedSupportEfforts = map[string]bool{
		"off": true, "low": true, "medium": true, "high": true, "xhigh": true, "max": true,
	}
)

func fallbackModels() []sdk.Model {
	return []sdk.Model{
		{
			ID:               "k3-agent",
			Name:             "K3 · Work",
			Protocols:        []protocolext.Protocol{protocolext.Chat},
			ContextWindow:    262144,
			MaxOutput:        32768,
			Tools:            true,
			Reasoning:        true,
			Efforts:          []string{},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
		{
			ID:               "k2d8-preview",
			Name:             "K2.8 Preview · Work",
			Protocols:        []protocolext.Protocol{protocolext.Chat},
			ContextWindow:    262144,
			MaxOutput:        32768,
			Tools:            true,
			Reasoning:        true,
			Efforts:          []string{},
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	}
}

func ParseGatewayModels(payload map[string]any) ([]sdk.Model, error) {
	rawList, ok := payload["data"].([]any)
	hasMore, _ := payload["has_more"].(bool)
	if !ok || hasMore || len(rawList) > 100 {
		return nil, errors.New("Kimi Work 模型目录暂不可用。")
	}
	out := make([]sdk.Model, 0, len(rawList))
	for _, rawItem := range rawList {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		id, _ := item["id"].(string)
		if id == "" || len(id) > 256 || invalidModelIDPattern.MatchString(id) {
			continue
		}
		efforts := []string{}
		if rawEfforts, ok := item["reasoningEffortOptions"].([]any); ok {
			for _, rawOpt := range rawEfforts {
				optObj, ok := rawOpt.(map[string]any)
				if !ok {
					continue
				}
				eff, _ := optObj["effort"].(string)
				eff = strings.ToLower(strings.TrimPrefix(eff, "REASONING_EFFORT_"))
				if validEffortPattern.MatchString(eff) && !slices.Contains(efforts, eff) {
					efforts = append(efforts, eff)
				}
			}
		}
		displayName, _ := item["display_name"].(string)
		if displayName == "" {
			displayName = id
		}
		ctxLen := int64(262144)
		if rawCtx, ok := asFloat(item["context_length"]); ok && rawCtx > 0 {
			if int64(rawCtx) < ctxLen {
				ctxLen = int64(rawCtx)
			}
		}
		supportsReasoning, _ := item["supports_reasoning"].(bool)
		supportsImage, _ := item["supports_image_in"].(bool)
		inputMods := []string{"text"}
		if supportsImage {
			inputMods = append(inputMods, "image")
		}
		out = append(out, sdk.Model{
			ID:               id,
			Name:             displayName,
			Protocols:        []protocolext.Protocol{protocolext.Chat},
			ContextWindow:    ctxLen,
			MaxOutput:        32768,
			Tools:            true,
			Vision:           supportsImage,
			Reasoning:        supportsReasoning,
			Efforts:          efforts,
			InputModalities:  inputMods,
			OutputModalities: []string{"text"},
		})
	}
	if len(out) == 0 {
		return nil, errors.New("Kimi Work 模型目录为空。")
	}
	return out, nil
}

func ParseWorkConfigModels(payload map[string]any) ([]sdk.Model, map[string]string, error) {
	var combined []any
	if primary, ok := pickAny(payload, "models", "models").([]any); ok {
		combined = append(combined, primary...)
	}
	if more, ok := pickAny(payload, "moreModels", "more_models").([]any); ok {
		combined = append(combined, more...)
	}
	if len(combined) > 100 {
		return nil, nil, errors.New("Kimi Work 模型目录暂不可用。")
	}
	out := make([]sdk.Model, 0, len(combined))
	indexByID := map[string]int{}
	var fastBaseIDs []string
	for _, rawItem := range combined {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		reqParams := pickObject(item, "requestParams", "request_params")
		display := pickObject(item, "display", "display")
		daimonCfg := pickObject(item, "daimonModelConfig", "daimon_model_config")
		id := strings.TrimSpace(pickString(reqParams, "modelId", "model_id"))
		if id == "" || len(id) > 256 || invalidModelIDPattern.MatchString(id) {
			continue
		}
		if _, seen := indexByID[id]; seen {
			continue
		}
		efforts := []string{}
		if rawOpts, ok := pickAny(reqParams, "reasoningEffortOptions", "reasoning_effort_options").([]any); ok {
			for _, rawOpt := range rawOpts {
				optObj, ok := rawOpt.(map[string]any)
				if !ok {
					continue
				}
				rawVal := pickString(optObj, "reasoningEffort", "reasoning_effort")
				if rawVal == "" {
					rawVal = pickString(optObj, "effort", "effort")
				}
				eff := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(rawVal), "REASONING_EFFORT_"))
				if validEffortPattern.MatchString(eff) && eff != "unspecified" && !slices.Contains(efforts, eff) {
					efforts = append(efforts, eff)
				}
			}
		}
		if len(efforts) == 0 {
			if rawSupport, ok := pickAny(daimonCfg, "supportEfforts", "support_efforts").([]any); ok {
				for _, rawVal := range rawSupport {
					eff := strings.ToLower(strings.TrimSpace(stringVal(rawVal)))
					if allowedSupportEfforts[eff] {
						if eff == "off" {
							eff = "none"
						}
						if !slices.Contains(efforts, eff) {
							efforts = append(efforts, eff)
						}
					}
				}
			}
		}
		hasImage := false
		hasThinking := len(efforts) > 0
		if rawCaps, ok := pickAny(daimonCfg, "capabilities", "capabilities").([]any); ok {
			for _, capVal := range rawCaps {
				switch stringVal(capVal) {
				case "image_in":
					hasImage = true
				case "thinking":
					hasThinking = true
				}
			}
		}
		ctxLen := int64(262144)
		if rawMax, ok := asFloat(pickAny(daimonCfg, "maxContextSize", "max_context_size")); ok && rawMax > 0 {
			if int64(rawMax) < ctxLen {
				ctxLen = int64(rawMax)
			}
		}
		maxOut := int64(32768)
		if rawOut, ok := asFloat(pickAny(reqParams, "maxTokens", "max_tokens")); ok && rawOut > 0 {
			maxOut = int64(rawOut)
		}
		if speeds, ok := pickAny(item, "speedOptions", "speed_options").([]any); ok {
			for _, rawSpeed := range speeds {
				speedObj, ok := rawSpeed.(map[string]any)
				if !ok {
					continue
				}
				avail, _ := speedObj["available"].(bool)
				if pickString(speedObj, "speedValue", "speed_value") == "fast" && avail {
					fastBaseIDs = append(fastBaseIDs, id)
					break
				}
			}
		}
		name := strings.TrimSpace(pickString(display, "displayName", "display_name"))
		if name == "" {
			name = id
		}
		inputMods := []string{"text"}
		if hasImage {
			inputMods = append(inputMods, "image")
		}
		indexByID[id] = len(out)
		out = append(out, sdk.Model{
			ID:               id,
			Name:             name,
			Protocols:        []protocolext.Protocol{protocolext.Chat},
			ContextWindow:    ctxLen,
			MaxOutput:        maxOut,
			Tools:            true,
			Vision:           hasImage,
			Reasoning:        hasThinking,
			Efforts:          efforts,
			InputModalities:  inputMods,
			OutputModalities: []string{"text"},
		})
	}
	fastMap := map[string]string{}
	for _, baseID := range fastBaseIDs {
		idx := indexByID[baseID]
		out[idx].ServiceTiers = []string{"priority"}
		fastMap[baseID] = baseID
		fastID := baseID + FastSuffix
		if _, exists := indexByID[fastID]; !exists {
			cloned := out[idx]
			cloned.ID = fastID
			cloned.Name = cloned.Name + " Fast"
			cloned.ServiceTiers = nil
			indexByID[fastID] = len(out)
			out = append(out, cloned)
			fastMap[fastID] = baseID
		}
	}
	return out, fastMap, nil
}

func stringVal(value any) string {
	str, _ := value.(string)
	return str
}

func (owner *module) fetchMenu(ctx context.Context, stream *sdk.Stream, session DesktopSession, creds Credentials) ([]sdk.Model, map[string]string, error) {
	token := session.WebToken
	if creds.HasPair() {
		token = creds.Access
	}
	if token != "" {
		if payload, err := owner.jsonRequest(ctx, stream, owner.options.Endpoints.Config, token, map[string]any{}, ""); err == nil {
			rows, fastMap, parseErr := ParseWorkConfigModels(payload)
			if parseErr == nil && len(rows) > 0 {
				return rows, fastMap, nil
			}
		}
	}
	payload, err := owner.jsonRequest(ctx, stream, owner.options.Endpoints.Config, "", map[string]any{}, "")
	if err != nil {
		return nil, nil, err
	}
	return ParseWorkConfigModels(payload)
}

func (owner *module) fastOffered(ctx context.Context, stream *sdk.Stream, session DesktopSession, creds Credentials) map[string]string {
	owner.mu.Lock()
	cached, ok := owner.fastCache[session.UID]
	owner.mu.Unlock()
	if ok {
		return cached
	}
	_, fastMap, err := owner.fetchMenu(ctx, stream, session, creds)
	if err != nil || fastMap == nil {
		fastMap = map[string]string{}
	}
	owner.mu.Lock()
	owner.fastCache[session.UID] = fastMap
	owner.mu.Unlock()
	return fastMap
}

func (owner *module) models(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.ModelsRequest
	if len(call.Params) > 0 && decodeObject(call.Params, &request) != nil {
		return nil, invalid("Invalid Kimi Work models request")
	}
	rawCreds := call.Auth
	if !nonempty(rawCreds) {
		rawCreds = request.Credentials
	}
	var creds Credentials
	if decodeObject(rawCreds, &creds) != nil || !creds.Valid() {
		return sdk.ModelsResult{Models: fallbackModels()}, nil
	}
	session, err := owner.currentSession(creds)
	if err != nil {
		return nil, err
	}
	var (
		wg         sync.WaitGroup
		menuModels []sdk.Model
		fastMap    map[string]string
		menuErr    error
		gwModels   []sdk.Model
		gwErr      error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		menuModels, fastMap, menuErr = owner.fetchMenu(ctx, stream, session, creds)
	}()
	go func() {
		defer wg.Done()
		modelsURL := strings.TrimRight(owner.options.Endpoints.Base, "/") + "/models"
		payload, err := owner.jsonRequest(ctx, stream, modelsURL, session.WorkToken, nil, session.DeviceID)
		if err != nil {
			gwErr = err
			return
		}
		gwModels, gwErr = ParseGatewayModels(payload)
	}()
	wg.Wait()

	if menuErr == nil {
		owner.mu.Lock()
		owner.fastCache[session.UID] = fastMap
		owner.mu.Unlock()
	}
	if gwErr != nil && len(menuModels) == 0 {
		return nil, gwErr
	}
	merged := make([]sdk.Model, 0, len(menuModels)+len(gwModels))
	indexByID := map[string]int{}
	for _, row := range menuModels {
		indexByID[row.ID] = len(merged)
		merged = append(merged, row)
	}
	for _, gwRow := range gwModels {
		if idx, exists := indexByID[gwRow.ID]; exists {
			existing := merged[idx]
			gwRow.Name = existing.Name
			if len(existing.ServiceTiers) > 0 {
				gwRow.ServiceTiers = existing.ServiceTiers
			}
			merged[idx] = gwRow
		} else {
			indexByID[gwRow.ID] = len(merged)
			merged = append(merged, gwRow)
		}
	}
	if len(merged) == 0 {
		merged = fallbackModels()
	}
	return sdk.ModelsResult{Models: merged}, nil
}
