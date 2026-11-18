package minimax

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"unicode/utf16"
)

// The connector supplies the schema on every call. Refuse constraints we cannot
// enforce before a paid invocation, instead of treating an unfamiliar schema as
// permission to send arbitrary arguments. Annotation keywords have no effect.
func parseToolSchema(data string) (map[string]any, error) {
	var schema map[string]any
	if json.Unmarshal([]byte(data), &schema) != nil || schema == nil {
		return nil, toolError("Malformed connector input schema")
	}
	return schema, nil
}

func supportedSchema(value any, depth int) error {
	if depth > 64 {
		return toolError("Connector schema exceeds nesting limit")
	}
	if _, ok := value.(bool); ok {
		return nil
	}
	schema, ok := value.(map[string]any)
	if !ok {
		return toolError("Malformed connector schema")
	}
	for key, value := range schema {
		switch key {
		case "$schema", "$id", "$comment", "title", "description", "default", "examples", "deprecated", "readOnly", "writeOnly":
		case "type":
			types := []any{value}
			if list, ok := value.([]any); ok {
				types = list
			}
			if len(types) == 0 {
				return toolError("Empty connector type constraint")
			}
			for _, typeConstraint := range types {
				if !contains([]string{"null", "string", "integer", "number", "boolean", "array", "object"}, text(typeConstraint)) {
					return toolError("Unsupported connector schema type")
				}
			}
		case "enum":
			values, ok := value.([]any)
			if !ok || len(values) == 0 {
				return toolError("Malformed connector enum")
			}
		case "const":
		case "minLength", "maxLength", "minItems", "maxItems":
			bound, ok := value.(float64)
			if !ok || bound < 0 || math.Trunc(bound) != bound {
				return toolError("Malformed connector size constraint")
			}
		case "minimum", "maximum":
			if _, ok := value.(float64); !ok {
				return toolError("Malformed connector number constraint")
			}
		case "required":
			keys, ok := value.([]any)
			if !ok {
				return toolError("Malformed connector required constraint")
			}
			for _, requiredKey := range keys {
				if _, ok := requiredKey.(string); !ok {
					return toolError("Malformed connector required field")
				}
			}
		case "properties":
			props, ok := value.(map[string]any)
			if !ok {
				return toolError("Malformed connector properties")
			}
			for _, subschema := range props {
				if err := supportedSchema(subschema, depth+1); err != nil {
					return err
				}
			}
		case "additionalProperties", "items", "not", "if", "then", "else":
			if err := supportedSchema(value, depth+1); err != nil {
				return err
			}
		case "allOf", "anyOf", "oneOf":
			schemas, ok := value.([]any)
			if !ok || len(schemas) == 0 {
				return toolError("Malformed connector combination constraint")
			}
			for _, subschema := range schemas {
				if err := supportedSchema(subschema, depth+1); err != nil {
					return err
				}
			}
		default:
			return toolError("Unsupported connector schema constraint: " + key)
		}
	}
	return nil
}

func validateToolArguments(schema map[string]any, arguments any) error {
	if object(arguments) == nil {
		return invalid("Tool arguments must be an object")
	}
	if err := supportedSchema(schema, 0); err != nil {
		return err
	}
	if err := checkToolSchema(schema, arguments, "arguments"); err != nil {
		return invalid(err.Error())
	}
	return nil
}

func checkToolSchema(value, arguments any, path string) error {
	if permit, ok := value.(bool); ok {
		if permit {
			return nil
		}
		return fmt.Errorf("%s is not accepted", path)
	}
	schema := object(value)
	fail := func(rule string) error {
		return fmt.Errorf("%s %s", path, rule)
	}
	if constant, ok := schema["const"]; ok && !reflect.DeepEqual(constant, arguments) {
		return fail("does not match const")
	}
	if choices, ok := schema["enum"].([]any); ok {
		found := false
		for _, choice := range choices {
			found = found || reflect.DeepEqual(choice, arguments)
		}
		if !found {
			return fail("does not match enum")
		}
	}
	if typeConstraint, ok := schema["type"]; ok {
		types := []any{typeConstraint}
		if values, ok := typeConstraint.([]any); ok {
			types = values
		}
		matched := false
		for _, typeName := range types {
			switch typeName {
			case "null":
				matched = matched || arguments == nil
			case "string":
				_, ok := arguments.(string)
				matched = matched || ok
			case "integer":
				bound, ok := arguments.(float64)
				matched = matched || ok && math.Trunc(bound) == bound
			case "number":
				_, ok := arguments.(float64)
				matched = matched || ok
			case "boolean":
				_, ok := arguments.(bool)
				matched = matched || ok
			case "array":
				_, ok := arguments.([]any)
				matched = matched || ok
			case "object":
				matched = matched || object(arguments) != nil
			}
		}
		if !matched {
			return fail("has the wrong type")
		}
	}
	limit := func(key string, actual float64, minimum bool) bool {
		bound, ok := schema[key].(float64)
		return ok && (minimum && actual < bound || !minimum && actual > bound)
	}
	switch typedValue := arguments.(type) {
	case string:
		size := float64(len(utf16.Encode([]rune(typedValue))))
		if limit("minLength", size, true) || limit("maxLength", size, false) {
			return fail("violates string length")
		}
	case float64:
		if limit("minimum", typedValue, true) || limit("maximum", typedValue, false) {
			return fail("violates numeric bounds")
		}
	case []any:
		if limit("minItems", float64(len(typedValue)), true) || limit("maxItems", float64(len(typedValue)), false) {
			return fail("violates item count")
		}
		if items, ok := schema["items"]; ok {
			for index, item := range typedValue {
				if err := checkToolSchema(items, item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		}
	case map[string]any:
		if required, ok := schema["required"].([]any); ok {
			for _, key := range required {
				if typedValue[key.(string)] == nil {
					return fail("requires " + key.(string))
				}
			}
		}
		props := object(schema["properties"])
		keys := make([]string, 0, len(typedValue))
		for key := range typedValue {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			subschema, exists := props[key]
			if !exists {
				subschema, exists = schema["additionalProperties"]
			}
			if exists {
				if err := checkToolSchema(subschema, typedValue[key], path+"."+key); err != nil {
					return err
				}
			}
		}
	}
	if schemas, ok := schema["allOf"].([]any); ok {
		for _, subschema := range schemas {
			if err := checkToolSchema(subschema, arguments, path); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		if schemas, ok := schema[key].([]any); ok {
			matches := 0
			for _, subschema := range schemas {
				if checkToolSchema(subschema, arguments, path) == nil {
					matches++
				}
			}
			if matches == 0 || key == "oneOf" && matches != 1 {
				return fail("violates " + key)
			}
		}
	}
	if subschema, exists := schema["not"]; exists && checkToolSchema(subschema, arguments, path) == nil {
		return fail("uses a forbidden combination")
	}
	if condition, exists := schema["if"]; exists {
		branch := "else"
		if checkToolSchema(condition, arguments, path) == nil {
			branch = "then"
		}
		if subschema, ok := schema[branch]; ok {
			return checkToolSchema(subschema, arguments, path)
		}
	}
	return nil
}
