package minimax

import (
	"context"
	"strings"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestSearchWebToolPreservesFiltersAndNormalizesEnvelope(test *testing.T) {
	fixture, engine, accepted := toolsFixture(test)
	fixture.respond = func(_ context.Context, endpoint string, arguments map[string]any) (any, error) {
		if endpoint != webSearchPath || arguments["query"] != "fixture" || arguments["freshness"] != "day" || arguments["search_type"] != "news" || arguments["count"] != float64(3) {
			test.Fatalf("wrong search body: %s %#v", endpoint, arguments)
		}
		return map[string]any{"base_resp": map[string]any{"status_code": 0}, "code": 0, "message": "ok", "organic": []any{}, "metadata": "kept"}, nil
	}
	value, err := engine.runTool(context.Background(), "web_search", map[string]any{"query": "fixture", "count": float64(3), "freshness": "day", "search_type": "news"})
	if err != nil || object(value)["metadata"] != "kept" || len(object(value)) != 2 || *accepted != 1 || len(fixture.calls) != 1 {
		test.Fatalf("search parity: %v %v", value, err)
	}
	if fixture.calls[0].Budget < 119*time.Second || fixture.calls[0].Budget > 120*time.Second {
		test.Fatal("search deadline lost")
	}
}

func TestSearchInvalidToolInputNeverCallsVendor(test *testing.T) {
	fixture, engine, accepted := toolsFixture(test)
	for _, input := range []map[string]any{{"query": " "}, {"query": "fixture", "count": float64(0)}, {"query": "fixture", "count": 1.5}, {"query": "fixture", "freshness": "century"}, {"query": "fixture", "search_type": "web_fetch"}, {"query": "fixture", "unknown": true}} {
		if _, err := engine.runTool(context.Background(), "web_search", input); err == nil {
			test.Fatalf("invalid input accepted: %#v", input)
		}
	}
	if len(fixture.calls) != 0 || *accepted != 0 {
		test.Fatal("invalid query reached vendor")
	}
}

func TestSearchPageAliasesDeduplicationAndSDKBounds(test *testing.T) {
	value := map[string]any{
		"organic": []any{map[string]any{"name": "first", "link": "https://example.test/one", "snippet": "body", "published_time": "2026"}, map[string]any{"title": "bad", "url": "https://user:secret@example.test/"}},
		"news":    []any{map[string]any{"title": "duplicate", "url": "https://example.test/one"}, map[string]any{"href": "https://example.test/two", "content": "two"}},
	}
	result := searchPages(value, 2)
	if result.Validate(2) != nil || len(result.Pages) != 2 || result.Pages[0].Title != "first" || result.Pages[0].Age != "2026" || result.Pages[1].Title != "https://example.test/two" {
		test.Fatalf("invalid mapped pages: %#v", result)
	}
	nested := map[string]any{"data": map[string]any{"results": []any{map[string]any{"title": strings.Repeat("界", 2000) + "\n", "url": "https://example.test/", "text": strings.Repeat("a", 100000), "age": strings.Repeat("b", 1000)}}}}
	bounded := searchPages(nested, 1)
	if bounded.Validate(1) != nil || len(bounded.Pages) != 1 || len(bounded.Pages[0].Text) != 64<<10 {
		test.Fatalf("SDK bounds violated: %#v", bounded)
	}
	if empty := searchPages(map[string]any{"results": []any{}}, 2); empty.Pages == nil || empty.Validate(2) != nil {
		test.Fatal("empty success must retain a non-null page array")
	}
}

func TestSearchSelectedSourceCannotCrossProviders(test *testing.T) {
	module := NewModule(ModuleOptions{})
	call := sdk.Call{Scope: sdk.Scope{Provider: China, Account: "cn-account", Generation: 1}, Params: raw(sdk.SearchRequest{Source: Global, Query: "fixture", Count: 1})}
	if _, err := module.Actions["search."+China](context.Background(), call, nil); err == nil {
		test.Fatal("source mismatch accepted")
	}
	call.Params = raw(sdk.SearchRequest{Source: Global, Query: "fixture", Count: 1})
	if _, err := module.Actions["search."+Global](context.Background(), call, nil); err == nil {
		test.Fatal("provider mismatch accepted")
	}
	if toolHTTPStatus(&APIError{Status: 429}) != 429 || toolHTTPStatus(&APIError{Status: 402}) != 402 {
		test.Fatal("search account refusal status lost")
	}
}
