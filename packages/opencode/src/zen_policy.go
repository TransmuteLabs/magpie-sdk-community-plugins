package opencode

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

type publicEnvironment struct {
	Directory string `json:"directory"`
	Workspace string `json:"workspace"`
	Git       *bool  `json:"git"`
	Platform  string `json:"platform"`
	Temporary string `json:"temporary"`
}

var nativeEnvironment = regexp.MustCompile(`(?s)<env>\r?\n {2}Working directory: [^\n]*\n.*?</env>`)

func environmentPath(value string) bool {
	absolute := strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\\`) ||
		len(value) >= 3 && (value[0] >= 'A' && value[0] <= 'Z' || value[0] >= 'a' && value[0] <= 'z') &&
			value[1] == ':' && (value[2] == '/' || value[2] == '\\')
	return absolute && len(value) <= 4096 && !strings.ContainsAny(value, "<>") && !strings.ContainsFunc(value, unicode.IsControl)
}

func (facts *publicEnvironment) valid() bool {
	return facts.Git != nil && environmentPath(facts.Directory) && environmentPath(facts.Workspace) &&
		environmentPath(facts.Temporary) && slices.Contains([]string{"linux", "darwin", "win32", "freebsd", "openbsd", "netbsd"}, facts.Platform)
}

func (facts *publicEnvironment) block() string {
	repository := "no"
	if *facts.Git {
		repository = "yes"
	}
	return strings.Join([]string{
		"Here is some useful information about the environment you are running in:",
		"<env>",
		"  Working directory: " + facts.Directory,
		"  Workspace root folder: " + facts.Workspace,
		"  Is directory a git repo: " + repository,
		"  Platform: " + facts.Platform,
		"  Prefer " + facts.Temporary + " for temporary files.",
		"</env>",
	}, "\n")
}

func environmentText(content any) string {
	if text, ok := content.(string); ok {
		return text
	}
	var texts []string
	if parts, ok := content.([]any); ok {
		for _, part := range parts {
			if text, ok := zenToolObject(part)["text"].(string); ok {
				texts = append(texts, text)
			}
		}
	}
	return strings.Join(texts, "\n")
}

func withPublicEnvironment(content any, block string) (any, error) {
	if content == nil || content == "" {
		return block, nil
	}
	switch content := content.(type) {
	case string:
		return content + "\n\n" + block, nil
	case []any:
		return append(content, map[string]any{"type": "text", "text": block}), nil
	default:
		return nil, errors.New("public environment requires string or text-part system content")
	}
}

func addPublicEnvironment(value map[string]any, protocol string, facts *publicEnvironment) error {
	if facts == nil {
		return nil
	}
	if protocol == "messages" {
		if nativeEnvironment.MatchString(environmentText(value["system"])) {
			return nil
		}
		content, err := withPublicEnvironment(value["system"], facts.block())
		value["system"] = content
		return err
	}
	key := "messages"
	if protocol == "responses" {
		key = "input"
	}
	var first map[string]any
	if messages, ok := value[key].([]any); ok {
		for _, message := range messages {
			object := zenToolObject(message)
			if object["role"] != "system" && object["role"] != "developer" {
				continue
			}
			if nativeEnvironment.MatchString(environmentText(object["content"])) {
				return nil
			}
			if first == nil && object["role"] == "system" {
				first = object
			}
		}
	}
	if protocol == "responses" {
		if nativeEnvironment.MatchString(environmentText(value["instructions"])) {
			return nil
		}
		content, err := withPublicEnvironment(value["instructions"], facts.block())
		value["instructions"] = content
		return err
	}
	if first != nil {
		content, err := withPublicEnvironment(first["content"], facts.block())
		first["content"] = content
		return err
	}
	messages, err := zenToolList(value["messages"])
	if err != nil {
		return err
	}
	value["messages"] = append([]any{map[string]any{"role": "system", "content": facts.block()}}, messages...)
	return nil
}

func prepareZenPublic(protocol string, body []byte, facts *publicEnvironment) ([]byte, error) {
	value, err := zenToolJSON(body)
	if err != nil {
		return nil, err
	}
	value["stream"] = true
	if protocol == "chat" {
		options := zenToolObject(value["stream_options"])
		if value["stream_options"] != nil && options == nil {
			return nil, errors.New("public Chat stream_options must be an object")
		}
		if options == nil {
			options = map[string]any{}
		}
		options["include_usage"] = true
		value["stream_options"] = options
	}
	if err := addPublicEnvironment(value, protocol, facts); err != nil {
		return nil, err
	}
	body, err = json.Marshal(value)
	if len(body) > 16<<20 {
		return nil, errors.New("public request exceeds its byte limit")
	}
	return body, err
}
