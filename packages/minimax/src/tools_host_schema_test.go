package minimax

import (
	"encoding/json"
	"testing"
)

func TestToolsPublishedHostSelectionSchema(test *testing.T) {
	configured := NewModule(ModuleOptions{})
	if len(configured.Manifest.Tools) == 0 {
		test.Fatal("MiniMax published no tools")
	}
	for _, descriptor := range configured.Manifest.Tools {
		test.Run(descriptor.Name, func(test *testing.T) {
			var schema map[string]any
			if err := json.Unmarshal(descriptor.InputSchema, &schema); err != nil {
				test.Fatal(err)
			}
			selection := object(object(schema["properties"])["_magpie"])
			if selection == nil {
				test.Fatal("published tool schema does not admit host account selection")
			}
			if err := supportedSchema(schema, 0); err != nil {
				test.Fatal(err)
			}
			for _, provider := range []string{China, Global} {
				if err := checkToolSchema(selection, map[string]any{"provider": provider, "account": "selected-a"}, "_magpie"); err != nil {
					test.Fatal("explicit host selection rejected:", err)
				}
			}
			for _, invalid := range []any{
				nil, "selected-a", map[string]any{},
				map[string]any{"provider": Global},
				map[string]any{"account": "selected-a"},
				map[string]any{"provider": "", "account": "selected-a"},
				map[string]any{"provider": Global, "account": ""},
				map[string]any{"provider": Global, "account": "selected-a", "generation": float64(1)},
				map[string]any{"provider": Global, "account": "selected-a", "credentials": "forged"},
			} {
				if err := checkToolSchema(selection, invalid, "_magpie"); err == nil {
					test.Fatalf("invalid host selection accepted: %#v", invalid)
				}
			}
			requiredFields, _ := schema["required"].([]any)
			for _, required := range requiredFields {
				if required == "_magpie" {
					test.Fatal("host selection must remain optional for preselected SDK calls")
				}
			}
			switch descriptor.Name {
			case "list_tools", "web_search":
				arguments := map[string]any{}
				if descriptor.Name == "web_search" {
					arguments["query"] = "synthetic schema control"
				}
				if err := validateToolArguments(schema, arguments); err != nil {
					test.Fatal("preselected call rejected:", err)
				}
				arguments["_magpie"] = map[string]any{"provider": Global, "account": "selected-a"}
				if err := validateToolArguments(schema, arguments); err != nil {
					test.Fatal("ordinary MCP selected call rejected:", err)
				}
			}
		})
	}
}
