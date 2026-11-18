package opencode

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

func skillScalar(value string) (valid, text bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return true, false
	}
	if value[0] == '\'' {
		for index := 1; index < len(value); index++ {
			if value[index] != '\'' {
				continue
			}
			if index+1 < len(value) && value[index+1] == '\'' {
				index++
				continue
			}
			tail := strings.TrimSpace(value[index+1:])
			return tail == "" || strings.HasPrefix(tail, "#"), true
		}
		return false, false
	}
	if value[0] == '"' {
		for index := 1; index < len(value); index++ {
			if value[index] == '\\' {
				index++
				continue
			}
			if value[index] == '"' {
				var text string
				tail := strings.TrimSpace(value[index+1:])
				return json.Unmarshal([]byte(value[:index+1]), &text) == nil &&
					(tail == "" || strings.HasPrefix(tail, "#")), true
			}
		}
		return false, false
	}
	if strings.ContainsAny(value[:1], "[{") {
		return json.Valid([]byte(value)), false
	}
	if strings.ContainsAny(value[:1], "&*!|>@`%") || strings.HasPrefix(value, "- ") ||
		strings.HasPrefix(value, "? ") || strings.ContainsAny(value, "\t\r\x00") {
		return false, false
	}
	for index := 0; index < len(value); index++ {
		if value[index] == '#' && (index == 0 || value[index-1] == ' ') {
			value = strings.TrimSpace(value[:index])
			break
		}
	}
	if value == "" {
		return true, false
	}
	_, numeric := strconv.ParseFloat(value, 64)
	typed := numeric == nil || slices.Contains([]string{"true", "false", "null", "~", ".inf", "+.inf", "-.inf", ".nan"}, strings.ToLower(value)) ||
		strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0o") || strings.HasPrefix(value, "0b")
	return true, !typed
}

func skillKey(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character != '_' && character != '-' && character != '/' &&
			(character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func validSkillFrontmatter(content string) bool {
	if !utf8.ValidString(content) || len(content) > maxReplyBytes {
		return false
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return true
	}
	lines := strings.Split(content[4:], "\n")
	closing := -1
	for index, line := range lines {
		if strings.TrimRight(line, " \t") == "---" {
			closing = index
			break
		}
	}
	if closing < 0 || closing > 10000 {
		return false
	}
	seen := map[string]bool{}
	for index := 0; index < closing; index++ {
		line := lines[index]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			return false
		}
		key, value, found := strings.Cut(line, ":")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !found || !skillKey(key) || seen[key] {
			return false
		}
		seen[key] = true
		if slices.Contains([]string{"|", "|-", "|+", ">", ">-", ">+"}, strings.TrimSpace(strings.SplitN(value, " #", 2)[0])) {
			for index+1 < closing && (strings.TrimSpace(lines[index+1]) == "" || strings.HasPrefix(lines[index+1], " ")) {
				index++
				if strings.ContainsRune(lines[index], '\t') {
					return false
				}
			}
			continue
		}
		if key == "metadata" && value == "" {
			nested := map[string]bool{}
			indentation := 0
			for index+1 < closing && (strings.TrimSpace(lines[index+1]) == "" || strings.HasPrefix(lines[index+1], " ")) {
				index++
				line := lines[index]
				if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
					continue
				}
				indent := len(line) - len(strings.TrimLeft(line, " "))
				if indentation == 0 {
					indentation = indent
				}
				name, value, found := strings.Cut(strings.TrimSpace(line), ":")
				if !found || indent != indentation || !skillKey(name) || nested[name] {
					return false
				}
				nested[name] = true
				if valid, _ := skillScalar(value); !valid {
					return false
				}
			}
			continue
		}
		valid, text := skillScalar(value)
		if !valid || (key == "name" || key == "description") && !text {
			return false
		}
	}
	return true
}
