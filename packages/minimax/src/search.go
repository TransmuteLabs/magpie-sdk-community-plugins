package minimax

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const webSearchPath = "/mavis/api/v1/mcp/web_search"

func webToolSchema() map[string]any {
	return toolSchema(map[string]any{
		"query": stringSchema(), "count": map[string]any{"type": "integer", "minimum": float64(1)},
		"freshness":   map[string]any{"type": "string", "enum": []any{"noLimit", "day", "week", "month", "year"}},
		"search_type": map[string]any{"type": "string", "enum": []any{"search", "videos", "places", "news", "shopping"}},
	}, "query")
}

func (engine matrixEngine) webSearch(ctx context.Context, args map[string]any) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	data, err := engine.client.JSON(ctx, webSearchPath, args, toolResponseLimit)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if json.Unmarshal(data, &result) != nil || result == nil {
		return nil, toolError("Malformed MiniMax search result")
	}
	delete(result, "base_resp")
	if _, hasCode := result["code"]; hasCode {
		delete(result, "code")
		delete(result, "message")
	}
	return result, nil
}

func installSearch(owner *module, target *sdk.Module) {
	for _, site := range []string{China, Global} {
		action := "search." + site
		target.Manifest.Actions = append(target.Manifest.Actions, sdk.ActionDescriptor{ID: action, Effect: "read", InputSchema: raw(toolSchema(map[string]any{
			"source": stringSchema(), "query": stringSchema(), "count": map[string]any{"type": "integer", "minimum": 1, "maximum": sdk.MaxSearchPages}, "proxy": map[string]any{"type": "string"},
		}, "source", "query", "count"))})
		target.Manifest.SearchSources = append(target.Manifest.SearchSources, sdk.SearchSourceDescriptor{ID: site, Name: DefaultSites()[site].Name, Action: action, Provider: site})
		target.Actions[action] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
			return owner.search(ctx, call, stream, site)
		}
	}
}

func (owner *module) search(ctx context.Context, call sdk.Call, stream *sdk.Stream, site string) (any, error) {
	var request sdk.SearchRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	if request.Validate() != nil || request.Source != site {
		return nil, invalid("Invalid MiniMax search request")
	}
	client, err := owner.SelectedClient(ctx, call, stream, site)
	if err != nil {
		return nil, err
	}
	result, err := (matrixEngine{client: client}).webSearch(ctx, map[string]any{"query": strings.TrimSpace(request.Query), "count": request.Count})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return sdk.SearchResult{Pages: []sdk.SearchPage{}, Status: toolHTTPStatus(err), Error: "MiniMax search request was refused or failed"}, nil
	}
	return searchPages(result, request.Count), nil
}

func searchText(value string, limit int, controls bool) string {
	value = strings.ToValidUTF8(value, "")
	value = strings.Map(func(character rune) rune {
		if character == 0 || controls && unicode.IsControl(character) {
			return -1
		}
		return character
	}, value)
	if len(value) > limit {
		value = value[:limit]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	return value
}
func firstMediaText(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if valueText := text(value[key]); valueText != "" {
			return valueText
		}
	}
	return ""
}

func searchPages(value any, count int) sdk.SearchResult {
	result := sdk.SearchResult{Pages: []sdk.SearchPage{}}
	var rows func(any, int) []any
	rows = func(candidate any, depth int) []any {
		if depth > 32 {
			return nil
		}
		if list, ok := candidate.([]any); ok {
			return list
		}
		fields := object(candidate)
		out := []any{}
		found := false
		for _, key := range []string{"organic", "results", "news", "videos", "places", "shopping", "web"} {
			if list, ok := fields[key].([]any); ok {
				out = append(out, list...)
				found = true
			}
		}
		if !found {
			return rows(fields["data"], depth+1)
		}
		return out
	}
	seen := map[string]bool{}
	for _, row := range rows(value, 0) {
		fields := object(row)
		address := firstMediaText(fields, "url", "link", "href")
		addressURL, err := url.Parse(address)
		if err != nil || addressURL.Host == "" || addressURL.User != nil || (addressURL.Scheme != "http" && addressURL.Scheme != "https") || len(address) > 4096 || strings.ContainsFunc(address, unicode.IsControl) || seen[address] {
			continue
		}
		title := firstMediaText(fields, "title", "name")
		if title == "" {
			title = address
		}
		page := sdk.SearchPage{URL: address, Title: searchText(title, 2048, true), Text: searchText(firstMediaText(fields, "text", "content", "snippet", "description", "summary"), 64<<10, false), Age: searchText(firstMediaText(fields, "age", "date", "published_time", "published"), 256, true)}
		result.Pages = append(result.Pages, page)
		if result.Validate(count) != nil {
			result.Pages = result.Pages[:len(result.Pages)-1]
			break
		}
		seen[address] = true
		if len(result.Pages) >= count {
			break
		}
	}
	return result
}
