package minimax

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

func fallbackModels() []sdk.Model {
	return []sdk.Model{
		{ID: "MiniMax-M3.1-Flash-Preview", Name: "M3.1-Flash-Preview", Protocols: []protocolext.Protocol{protocolext.Messages}, ContextWindow: 1048576, MaxOutput: 524288, Tools: true, Vision: true, Reasoning: true, AlwaysThinks: true, Efforts: []string{"low", "medium", "high", "xhigh", "max"}, InputModalities: []string{"text", "image", "video"}, OutputModalities: []string{"text"}},
		{ID: "MiniMax-M3", Name: "MiniMax-M3", Protocols: []protocolext.Protocol{protocolext.Messages}, ContextWindow: 1048576, MaxOutput: 524288, Tools: true, Vision: true, Reasoning: true, Efforts: []string{"none", "high"}, InputModalities: []string{"text", "image", "video"}, OutputModalities: []string{"text"}},
		{ID: "MiniMax-M2.7", Name: "MiniMax-M2.7", Protocols: []protocolext.Protocol{protocolext.Messages}, ContextWindow: 200000, MaxOutput: 128000, Tools: true, Reasoning: true, AlwaysThinks: true, Efforts: []string{}, InputModalities: []string{"text"}, OutputModalities: []string{"text"}},
		{ID: "MiniMax-M2.7-highspeed", Name: "MiniMax-M2.7-highspeed", Protocols: []protocolext.Protocol{protocolext.Messages}, ContextWindow: 200000, MaxOutput: 128000, Tools: true, Reasoning: true, AlwaysThinks: true, Efforts: []string{}, InputModalities: []string{"text"}, OutputModalities: []string{"text"}},
	}
}

func parseModels(body []byte) []sdk.Model {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return nil
	}
	providers, _ := root["providers"].([]any)
	var config map[string]any
	for _, provider := range providers {
		if object(provider)["providerId"] == "minimax" {
			config = object(object(provider)["config"])
			break
		}
	}
	entries := object(config["models"])
	if entries == nil || len(entries) > 1024 {
		return nil
	}
	var ids []string
	seen := map[string]bool{}
	order, _ := config["model_order"].([]any)
	for _, value := range order {
		id := text(value)
		if entries[id] != nil && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	var tail []string
	for id := range entries {
		if !seen[id] {
			tail = append(tail, id)
		}
	}
	sort.Strings(tail)
	ids = append(ids, tail...)
	result := []sdk.Model{}
	for _, id := range ids {
		descriptor := object(entries[id])
		if descriptor == nil || strings.TrimSpace(id) == "" || len(id) > 256 {
			continue
		}
		name := text(descriptor["name"])
		if name == "" {
			name = id
		}
		contextSize := positive(object(descriptor["limit"])["context"])
		output := positive(object(descriptor["limit"])["output"])
		if contains([]string{"MiniMax-M3.1-Flash", "MiniMax-M3.1-Flash-Preview", "MiniMax-M3"}, id) {
			contextSize, output = 1048576, 524288
		}
		if contextSize > 1<<40 || output > 1<<40 {
			continue
		}
		modalities := []string{"text"}
		input, _ := object(descriptor["modalities"])["input"].([]any)
		for _, value := range input {
			kind := strings.ToLower(text(value))
			if contains([]string{"image", "audio", "video", "pdf"}, kind) && !contains(modalities, kind) {
				modalities = append(modalities, kind)
			}
		}
		if descriptor["attachment"] == true && !contains(modalities, "image") {
			modalities = append(modalities, "image")
		}
		mode := text(object(descriptor["thinking_config"])["mode"])
		reasoning := mode != "forced_off" && descriptor["reasoning"] != false
		if mode == "switchable" || mode == "forced_on" {
			reasoning = true
		}
		efforts := []string{}
		if mode == "switchable" {
			efforts = append(efforts, "none")
		}
		options := object(descriptor["thinking"])["effortOptions"]
		if options == nil {
			options = object(descriptor["thinking"])["effort_options"]
		}
		if options == nil {
			options = descriptor["effort_options"]
		}
		list, _ := options.([]any)
		for _, value := range list {
			effort := text(value)
			if contains([]string{"minimal", "low", "medium", "high", "xhigh", "max"}, effort) && !contains(efforts, effort) {
				efforts = append(efforts, effort)
			}
		}
		if mode == "switchable" && len(efforts) == 1 {
			efforts = append(efforts, "high")
		}
		if !reasoning {
			efforts = []string{}
		}
		result = append(result, sdk.Model{ID: id, Name: name, Protocols: []protocolext.Protocol{protocolext.Messages}, ContextWindow: int64(contextSize), MaxOutput: int64(output), Tools: true, Vision: contains(modalities, "image"), Reasoning: reasoning, AlwaysThinks: reasoning && mode != "switchable", Efforts: efforts, InputModalities: modalities, OutputModalities: []string{"text"}})
	}
	return result
}

func (owner *module) models(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.ModelsRequest
	if decode(call.Params, &request) != nil || nonempty(request.Credentials) {
		return nil, denied("Models use only the selected host credentials")
	}
	client, err := owner.SelectedClient(ctx, call, stream, "")
	if err != nil {
		return nil, err
	}
	query := url.Values{"region": {client.site.Region}, "buildEnv": {"prod"}}
	target := client.site.LLM + "/mavis/api/v1/models?" + query.Encode()
	fetch := func(access string) (ByteResponse, error) {
		ask, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		headers := http.Header{"Accept": {"application/json"}}
		if access != "" {
			headers.Set("Authorization", "Bearer "+access)
		}
		response, err := owner.siteRequest(ask, stream, "GET", target, nil, headers)
		if err != nil {
			return ByteResponse{}, err
		}
		return bounded(response, 4<<20)
	}
	response, err := fetch(client.credentials.Access)
	if err == nil && (response.Status == 401 || response.Status == 403) {
		response, err = fetch("")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	models := fallbackModels()
	if err == nil && response.Status >= 200 && response.Status < 300 {
		if live := parseModels(response.Body); len(live) > 0 {
			models = live
		}
	}
	return sdk.ModelsResult{Models: append(models, mediaModels()...)}, nil
}
