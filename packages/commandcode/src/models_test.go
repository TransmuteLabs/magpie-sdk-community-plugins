package commandcode

import (
	"os"
	"testing"
)

func TestCatalogKeepsResponsesOnlyAndUnknownPrices(t *testing.T) {
	models, err := ParseModels([]byte(`{"data":[{"id":"only","supported_endpoints":["/v1/responses"],"context_length":256000},{"id":"claude-example"},{"id":"typesafe/jev"}]}`), Subscription{}, false, false)
	if err != nil || len(models) != 3 {
		t.Fatalf("models: %+v %v", models, err)
	}
	if len(models[0].Protocols) != 1 || models[0].Protocols[0] != "responses" || models[0].Price != nil || models[0].InputModalities != nil {
		t.Fatalf("invented capabilities: %+v", models[0])
	}
	if models[1].Protocols[0] != "anthropic" {
		t.Fatalf("Claude protocol: %+v", models[1])
	}
}

func TestCatalogPlanAndExtraCredits(t *testing.T) {
	body := []byte(`{"data":[{"id":"claude-opus-5-5"},{"id":"gpt-6-luna"},{"id":"xai/grok-4.7"}]}`)
	models, err := ParseModels(body, Subscription{Plan: "Go"}, false, true)
	if err != nil || len(models) != 2 || models[0].ID != "gpt-6-luna" || models[0].Protocols[0] != "chat" {
		t.Fatalf("Go entitlement: %+v %v", models, err)
	}
	models, err = ParseModels(body, Subscription{Plan: "Go"}, true, true)
	if err != nil || len(models) != 4 {
		t.Fatalf("purchased credit unlock: %+v %v", models, err)
	}
}

func TestPinnedCatalogFixtureIsComplete(t *testing.T) {
	body, err := os.ReadFile("testdata/models-2026-10-13.json")
	if err != nil {
		t.Fatal(err)
	}
	models, err := ParseModels(body, Subscription{}, false, false)
	if err != nil || len(models) < 80 {
		t.Fatalf("full source catalog: %d %v", len(models), err)
	}
}
