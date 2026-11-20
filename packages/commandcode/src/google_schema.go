package commandcode

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

func isGoogle(model string) bool {
	model = strings.ToLower(model)
	return strings.HasPrefix(model, "google/") || strings.HasPrefix(model, "gemini") || strings.Contains(model, "/gemini")
}

func googleSchema(root map[string]any, geminiNullable bool) (map[string]any, error) {
	remaining := 65536
	open := make(map[string]bool)
	var walk func(any, int, int, bool) (any, error)
	walk = func(value any, depth, references int, nullable bool) (any, error) {
		remaining--
		if remaining < 0 || depth > 64 || references > 32 {
			return nil, errors.New("Google tool schema expansion exceeds its bounded limit")
		}
		if list, ok := value.([]any); ok {
			result := make([]any, len(list))
			for index, child := range list {
				_, schema := child.(map[string]any)
				next, err := walk(child, depth+1, references, nullable && schema)
				if err != nil {
					return nil, err
				}
				result[index] = next
			}
			return result, nil
		}
		object, ok := value.(map[string]any)
		if !ok {
			return value, nil
		}
		result := make(map[string]any, len(object))
		for name, child := range object {
			result[name] = child
		}
		if ref, ok := object["$ref"].(string); ok {
			target, found := schemaPointer(root, ref)
			tokens := strings.Split(ref, "/")
			direct := len(tokens) == 3 && tokens[0] == "#" && (tokens[1] == "$defs" || tokens[1] == "definitions")
			if target, objectTarget := target.(map[string]any); found && objectTarget && !direct {
				if open[ref] {
					return nil, errors.New("Google tool schema has a cyclic reference")
				}
				open[ref] = true
				defer delete(open, ref)
				merged := make(map[string]any, len(target)+len(object))
				for name, child := range target {
					merged[name] = child
				}
				for name, child := range object {
					if name != "$ref" {
						merged[name] = child
					}
				}
				return walk(merged, depth+1, references+1, nullable)
			}
		}
		if nullable {
			collapseGeminiNullable(result)
		}
		for _, name := range []string{"properties", "patternProperties", "$defs", "definitions", "dependentSchemas"} {
			children, ok := result[name].(map[string]any)
			if !ok {
				continue
			}
			updated := make(map[string]any, len(children))
			for key, child := range children {
				_, schema := child.(map[string]any)
				next, err := walk(child, depth+1, references, nullable && schema)
				if err != nil {
					return nil, err
				}
				updated[key] = next
			}
			result[name] = updated
		}
		for _, name := range []string{"items", "additionalProperties", "contains", "propertyNames", "not", "if", "then", "else", "unevaluatedProperties", "unevaluatedItems", "contentSchema", "additionalItems", "anyOf", "oneOf", "allOf", "prefixItems"} {
			if child, found := result[name]; found {
				_, list := child.([]any)
				validShape := !list
				switch name {
				case "anyOf", "oneOf", "allOf", "prefixItems":
					validShape = list
				case "items":
					validShape = true
				}
				next, err := walk(child, depth+1, references, nullable && validShape)
				if err != nil {
					return nil, err
				}
				result[name] = next
			}
		}
		return result, nil
	}
	result, err := walk(root, 0, 0, geminiNullable)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > 1<<20 {
		return nil, errors.New("Google tool schema exceeds its serialized limit")
	}
	return result.(map[string]any), nil
}

func collapseGeminiNullable(schema map[string]any) {
	types, ok := schema["type"].([]any)
	if !ok || len(types) != 2 {
		return
	}
	if declared, found := schema["nullable"]; found && declared != true {
		return
	}
	base, nullType := types[0], types[1]
	if base == "null" {
		base, nullType = nullType, base
	}
	if nullType != "null" {
		return
	}
	switch base {
	case "array", "boolean", "integer", "number", "object", "string":
		schema["type"] = base
		schema["nullable"] = true
		if schema["default"] == nil {
			delete(schema, "default")
		}
	}
}

func schemaPointer(root map[string]any, ref string) (any, bool) {
	if ref != "#" && !strings.HasPrefix(ref, "#/") {
		return nil, false
	}
	var current any = root
	for _, token := range strings.Split(ref, "/")[1:] {
		if decoded, err := url.PathUnescape(token); err == nil {
			token = decoded
		}
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		switch value := current.(type) {
		case map[string]any:
			var found bool
			current, found = value[token]
			if !found {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(value) || strconv.Itoa(index) != token {
				return nil, false
			}
			current = value[index]
		default:
			return nil, false
		}
	}
	return current, true
}
