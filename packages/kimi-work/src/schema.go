package kimiwork

import (
	"bytes"
	"encoding/json"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

var (
	schemaMapKeys = []string{"properties", "$defs", "definitions", "dependentSchemas", "patternProperties"}
	schemaSubKeys = []string{
		"items", "additionalProperties", "contains", "propertyNames", "not", "if", "then", "else",
		"allOf", "anyOf", "oneOf", "prefixItems", "unevaluatedProperties", "unevaluatedItems", "contentSchema",
	}
	allJSONTypes = []string{"string", "number", "integer", "boolean", "object", "array", "null"}
)

func eachSchema(node any, visit func(map[string]any)) {
	switch typed := node.(type) {
	case []any:
		for _, item := range typed {
			eachSchema(item, visit)
		}
	case map[string]any:
		visit(typed)
		for _, key := range schemaMapKeys {
			if childMap, ok := typed[key].(map[string]any); ok {
				keys := make([]string, 0, len(childMap))
				for childKey := range childMap {
					keys = append(keys, childKey)
				}
				slices.Sort(keys)
				for _, childKey := range keys {
					eachSchema(childMap[childKey], visit)
				}
			}
		}
		for _, key := range schemaSubKeys {
			if sub, exists := typed[key]; exists {
				eachSchema(sub, visit)
			}
		}
	}
}

func refTarget(root map[string]any, ref string) (any, bool) {
	if ref != "#" && !strings.HasPrefix(ref, "#/") {
		return nil, false
	}
	if ref == "#" {
		return root, true
	}
	var current any = root
	for _, rawToken := range strings.Split(ref[2:], "/") {
		token := strings.ReplaceAll(strings.ReplaceAll(rawToken, "~1", "/"), "~0", "~")
		if decoded, err := url.PathUnescape(token); err == nil {
			token = decoded
		}
		switch container := current.(type) {
		case map[string]any:
			next, exists := container[token]
			if !exists {
				return nil, false
			}
			current = next
		case []any:
			idx, err := strconv.Atoi(token)
			if err != nil || idx < 0 || idx >= len(container) {
				return nil, false
			}
			current = container[idx]
		default:
			return nil, false
		}
	}
	return current, true
}

func hasRefInside(node any) bool {
	encoded, err := json.Marshal(node)
	if err != nil {
		return false
	}
	return bytes.Contains(encoded, []byte(`"$ref"`))
}

func cloneValue(node any) any {
	switch typed := node.(type) {
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for key, val := range typed {
			copied[key] = cloneValue(val)
		}
		return copied
	case []any:
		copied := make([]any, len(typed))
		for idx, val := range typed {
			copied[idx] = cloneValue(val)
		}
		return copied
	default:
		return node
	}
}

func jsonValueType(value any) string {
	if value == nil {
		return "null"
	}
	switch typed := value.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		if !math.IsNaN(typed) && !math.IsInf(typed, 0) && math.Trunc(typed) == typed {
			return "integer"
		}
		return "number"
	case json.Number:
		if _, err := typed.Int64(); err == nil {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	default:
		return "object"
	}
}

func enumType(values []any) (any, bool) {
	primary := ""
	hasNull := false
	for _, val := range values {
		kind := jsonValueType(val)
		if kind == "null" {
			hasNull = true
			continue
		}
		if kind == "array" || kind == "object" {
			return nil, false
		}
		if primary == "" || primary == kind {
			primary = kind
		} else if (primary == "integer" && kind == "number") || (primary == "number" && kind == "integer") {
			primary = "number"
		} else {
			return nil, false
		}
	}
	if primary == "" {
		return nil, false
	}
	if hasNull {
		return []any{primary, "null"}, true
	}
	return primary, true
}

func schemaTypes(root map[string]any, node map[string]any, depth int) map[string]bool {
	if depth > 16 || node == nil {
		return nil
	}
	if rawType, exists := node["type"]; exists {
		out := map[string]bool{}
		switch typed := rawType.(type) {
		case string:
			out[typed] = true
		case []any:
			for _, item := range typed {
				if name, ok := item.(string); ok {
					out[name] = true
				}
			}
		}
		return out
	}
	if rawRef, exists := node["$ref"].(string); exists {
		target, ok := refTarget(root, rawRef)
		if !ok {
			return nil
		}
		targetObj, ok := target.(map[string]any)
		if !ok {
			return nil
		}
		return schemaTypes(root, targetObj, depth+1)
	}
	if enumVals, ok := node["enum"].([]any); ok {
		out := map[string]bool{}
		for _, val := range enumVals {
			out[jsonValueType(val)] = true
		}
		return out
	}
	if constVal, exists := node["const"]; exists {
		return map[string]bool{jsonValueType(constVal): true}
	}
	for _, branchKey := range []string{"oneOf", "anyOf", "allOf"} {
		branches, ok := node[branchKey].([]any)
		if !ok {
			continue
		}
		out := map[string]bool{}
		for _, branch := range branches {
			branchObj, isObj := branch.(map[string]any)
			var branchTypes map[string]bool
			if isObj {
				branchTypes = schemaTypes(root, branchObj, depth+1)
			}
			if len(branchTypes) == 0 {
				if branchKey != "allOf" {
					return nil
				}
				continue
			}
			for kind := range branchTypes {
				out[kind] = true
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	hasAny := func(keys ...string) bool {
		for _, key := range keys {
			if _, exists := node[key]; exists {
				return true
			}
		}
		return false
	}
	if hasAny("properties", "required", "additionalProperties", "patternProperties", "minProperties", "maxProperties", "propertyNames", "dependentRequired", "dependentSchemas", "unevaluatedProperties") {
		return map[string]bool{"object": true}
	}
	if hasAny("items", "prefixItems", "minItems", "maxItems", "uniqueItems", "contains", "unevaluatedItems") {
		return map[string]bool{"array": true}
	}
	if hasAny("minLength", "maxLength", "pattern", "format", "contentEncoding", "contentMediaType") {
		return map[string]bool{"string": true}
	}
	if hasAny("minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf") {
		return map[string]bool{"number": true}
	}
	return nil
}

func typeFor(root map[string]any, node map[string]any) (any, bool) {
	if enumVals, ok := node["enum"].([]any); ok {
		return enumType(enumVals)
	}
	if constVal, exists := node["const"]; exists {
		return enumType([]any{constVal})
	}
	found := schemaTypes(root, node, 0)
	if len(found) == 0 {
		out := make([]any, len(allJSONTypes))
		for idx, kind := range allJSONTypes {
			out[idx] = kind
		}
		return out, true
	}
	if found["integer"] && found["number"] {
		delete(found, "integer")
	}
	names := make([]any, 0, len(found))
	for _, kind := range allJSONTypes {
		if found[kind] {
			names = append(names, kind)
		}
	}
	if len(names) == 1 {
		return names[0], true
	}
	return names, true
}

func MoonshotSchema(root map[string]any) bool {
	changed := false
	eachSchema(root, func(node map[string]any) {
		_, hasRef := node["$ref"]
		_, hasType := node["type"]
		if hasRef && hasType {
			refStr, _ := node["$ref"].(string)
			target, ok := refTarget(root, refStr)
			targetObj, isObj := target.(map[string]any)
			if ok && isObj && !hasRefInside(targetObj) {
				own := make(map[string]any, len(node))
				for key, val := range node {
					if key != "$ref" {
						own[key] = val
					}
				}
				for key := range node {
					delete(node, key)
				}
				clonedTarget := cloneValue(targetObj).(map[string]any)
				for key, val := range clonedTarget {
					node[key] = val
				}
				for key, val := range own {
					node[key] = val
				}
			} else {
				delete(node, "type")
			}
			changed = true
		}
		if _, hasAnyOf := node["anyOf"].([]any); hasAnyOf {
			if _, stillHasType := node["type"]; stillHasType {
				delete(node, "type")
				changed = true
			}
		}
	})
	eachSchema(root, func(node map[string]any) {
		if _, hasType := node["type"]; hasType {
			return
		}
		if enumVals, ok := node["enum"].([]any); ok {
			if inferred, ok := enumType(enumVals); ok {
				node["type"] = inferred
				changed = true
			}
			return
		}
		if constVal, exists := node["const"]; exists {
			if inferred, ok := enumType([]any{constVal}); ok {
				node["type"] = inferred
				changed = true
			}
		}
	})
	eachSchema(root, func(node map[string]any) {
		refStr, ok := node["$ref"].(string)
		if !ok {
			return
		}
		target, found := refTarget(root, refStr)
		targetObj, isObj := target.(map[string]any)
		if !found || !isObj || len(targetObj) == 0 {
			return
		}
		if _, hasType := targetObj["type"]; hasType {
			return
		}
		if _, hasAnyOf := targetObj["anyOf"]; hasAnyOf {
			return
		}
		if _, hasRef := targetObj["$ref"]; hasRef {
			return
		}
		if inferred, ok := typeFor(root, targetObj); ok {
			targetObj["type"] = inferred
			changed = true
		}
	})
	return changed
}

func MoonshotTools(body []byte) []byte {
	if !bytes.Contains(body, []byte(`"tools"`)) {
		return body
	}
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil || doc == nil {
		return body
	}
	tools, ok := doc["tools"].([]any)
	if !ok {
		return body
	}
	changed := false
	for _, rawTool := range tools {
		toolObj, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		fnObj, ok := toolObj["function"].(map[string]any)
		if !ok {
			continue
		}
		params, ok := fnObj["parameters"].(map[string]any)
		if ok && MoonshotSchema(params) {
			changed = true
		}
	}
	if !changed {
		return body
	}
	encoded, err := encodeJSONNoEscape(doc)
	if err != nil {
		return body
	}
	return encoded
}

func asksFast(body []byte) bool {
	return bytes.Contains(body, []byte(`"service_tier"`)) || bytes.Contains(body, []byte(FastSuffix+`"`))
}

func FastSpeed(body []byte, offered map[string]string) ([]byte, bool) {
	if !asksFast(body) || len(offered) == 0 {
		return body, false
	}
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil || doc == nil {
		return body, false
	}
	modelID, _ := doc["model"].(string)
	targetID, ok := offered[modelID]
	if !ok || targetID == "" {
		return body, false
	}
	named := targetID != modelID
	tierVal, _ := doc["service_tier"].(string)
	tier := tierVal == "priority" || tierVal == "fast"
	if !named && !tier {
		return body, false
	}
	if tier {
		delete(doc, "service_tier")
	}
	doc["model"] = targetID
	encoded, err := encodeJSONNoEscape(doc)
	if err != nil {
		return body, false
	}
	return encoded, true
}

func encodeJSONNoEscape(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}
