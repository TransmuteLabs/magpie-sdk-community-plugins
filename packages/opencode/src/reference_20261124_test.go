package opencode

import (
	"slices"
	"testing"
)

func TestCatalogFiltersGoNonChatAndNewlyDeprecatedModels(t *testing.T) {
	const consolePayload = `{"opencode-go":{"id":"opencode-go","models":{
		"longcat-2.5":{"id":"longcat-2.5","type":"chat","limit":{"context":1000000,"output":65536},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/go/openai/v1"}}},
		"hungerg-free":{"id":"hungerg-free","type":"chat","limit":{"context":262144,"output":32768},
			"cost":{"text":{"input":{"price":0},"output":{"price":0}}},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/go/openai/v1"}}},
		"longcat-2.0":{"id":"longcat-2.0","type":"chat","deprecated":true,"limit":{"context":1000000,"output":65536},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/go/openai/v1"}}},
		"longcat-2.5-free":{"id":"longcat-2.5-free","type":"chat","status":"deprecated","limit":{"context":1000000,"output":65536},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/go/openai/v1"}}},
		"go-embed-1":{"id":"go-embed-1","type":"embedding","limit":{"context":8192,"output":1},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/go/openai/v1"}}},
		"go-decide-1":{"id":"go-decide-1","type":"decision","limit":{"context":32768,"output":1024},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/go/openai/v1"}}}
	}}}`
	models, ok := goDiscoverModels([]byte(consolePayload))
	if !ok {
		t.Fatal("goDiscoverModels failed on valid 2.4.1 catalog shape")
	}
	var ids []string
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	want := []string{"hungerg-free", "longcat-2.5"}
	if !slices.Equal(ids, want) {
		t.Fatalf("goDiscoverModels = %v, want %v", ids, want)
	}

	const zenPayload = `{"object":"list","data":[
		{"id":"kimi-k3.1-pro","type":"chat","limit":{"context":262144,"output":65536},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/openai/v1"}}},
		{"id":"jev-1.14","type":"chat","limit":{"context":262144,"output":65536},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/openai/v1"}}},
		{"id":"inkling-2-small-free","type":"chat","deprecated":true,"limit":{"context":131072,"output":16384},
			"cost":{"text":{"input":{"price":0},"output":{"price":0}}},
			"api":{"chat-completions":{"base_url":"https://opencode.ai/inference/openai/v1"}}}
	]}`
	zenModels, ok := zenDiscoverModels([]byte(zenPayload), false)
	if !ok {
		t.Fatal("zenDiscoverModels failed on valid 2.4.1 catalog shape")
	}
	ids = nil
	for _, model := range zenModels {
		ids = append(ids, model.ID)
	}
	wantZen := []string{"jev-1.14", "kimi-k3.1-pro"}
	if !slices.Equal(ids, wantZen) {
		t.Fatalf("zenDiscoverModels = %v, want %v", ids, wantZen)
	}
}
