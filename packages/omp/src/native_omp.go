package ompsessions

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type ompTargetOptions struct {
	Cwd          string `json:"cwd"`
	CanonicalCwd string `json:"canonicalCwd"`
	Home         string `json:"home"`
	Temp         string `json:"temp"`
}

func convertNativeToOMP(ctx context.Context, request sdk.SessionRequest) (sdk.SessionResult, error) {
	var options ompTargetOptions
	if strictJSON(request.Options, &options) != nil {
		return sdk.SessionResult{}, errors.New("native OMP migration requires selected filesystem context")
	}
	for _, value := range []string{options.Cwd, options.CanonicalCwd, options.Home, options.Temp} {
		if !filepath.IsAbs(value) || strings.ContainsAny(value, "\x00\r\n") {
			return sdk.SessionResult{}, errors.New("native OMP migration requires absolute canonical filesystem context")
		}
	}
	if len(request.Files) != 1 {
		return sdk.SessionResult{}, errors.New("native migration requires one selected source journal")
	}
	body, _, err := sessionInputDocuments(ctx, request, false)
	if err != nil {
		return sdk.SessionResult{}, err
	}
	if len(body) > MaxBytes {
		return sdk.SessionResult{}, errors.New("native source journal exceeds its byte budget")
	}
	var items []Item
	switch request.From {
	case "claude":
		items, err = readClaudeNative(ctx, body)
	case "codex":
		body, err = materializeCodexLineage(ctx, request)
		if err != nil {
			return sdk.SessionResult{}, err
		}
		items, err = readCodexNative(ctx, body)
	default:
		return sdk.SessionResult{}, errors.New("unsupported native OMP source")
	}
	if err != nil {
		return sdk.SessionResult{}, err
	}
	items, err = ompNativeHistory(items)
	if err != nil {
		return sdk.SessionResult{}, err
	}
	if len(items) == 0 {
		return sdk.SessionResult{}, errors.New("native source contains no portable context")
	}
	now := time.Now()
	encoded, err := encodeWithImageMode(ctx, Document{Schema: 1, Cwd: options.Cwd, Timestamp: now, Items: items}, now, false)
	if err != nil {
		return sdk.SessionResult{}, err
	}
	if len(encoded.Assets) != 0 {
		return sdk.SessionResult{}, errors.New("ordinary native migration must not overwrite global blobs")
	}
	result := sdk.SessionResult{ID: encoded.ID, Files: []sdk.SessionDocument{{Path: ompNativeBucket(options) + "/" + encoded.Filename, Format: "omp", Body: encoded.Journal}}}
	return boundedNativeResult(result)
}

func ompNativeBucket(options ompTargetOptions) string {
	encode := strings.NewReplacer("/", "-", `\`, "-", ":", "-")
	within := func(root string) (string, bool) {
		relative, err := filepath.Rel(root, options.CanonicalCwd)
		if err != nil || !filepath.IsLocal(relative) || strings.HasPrefix(relative, "..") {
			return "", false
		}
		if relative == "." {
			return "", true
		}
		return encode.Replace(relative), true
	}
	if relative, ok := within(options.Temp); ok {
		if relative == "" {
			return "-tmp"
		}
		return "-tmp-" + relative
	}
	if relative, ok := within(options.Home); ok {
		return "-" + relative
	}
	cwd := strings.TrimPrefix(strings.TrimPrefix(options.CanonicalCwd, "/"), `\`)
	return "--" + encode.Replace(cwd) + "--"
}

func ompNativeHistory(items []Item) ([]Item, error) {
	results := map[string]int{}
	for index, item := range items {
		if item.Kind == "result" && item.ID != "" {
			if _, exists := results[item.ID]; exists {
				return nil, errors.New("duplicate native tool results")
			}
			results[item.ID] = index
		}
	}
	calls := map[string]Item{}
	native := map[string]bool{}
	var result []Item
	for index, item := range items {
		switch item.Kind {
		case "call":
			if item.ID == "" {
				return nil, errors.New("native tool call has no ID")
			}
			if _, exists := calls[item.ID]; exists {
				return nil, errors.New("duplicate native tool call ID")
			}
			calls[item.ID] = item
			at, done := results[item.ID]
			if item.Shell != "" && done && at > index {
				item.Name = "bash"
				native[item.ID] = true
				result = append(result, item)
			} else {
				note := fmt.Sprintf("Recorded tool %s call:\n%s", item.Name, item.Input)
				if !done || at <= index {
					note += "\n[no later output was recorded]"
				}
				result = append(result, Item{Kind: "assistant", Text: note, Time: item.Time})
			}
		case "result":
			if native[item.ID] {
				result = append(result, item)
				continue
			}
			call := calls[item.ID]
			content := item.Content
			if len(content) == 0 {
				content = []Content{{Type: "text", Text: item.Text}}
			}
			content = append([]Content{{Type: "text", Text: fmt.Sprintf("Recorded %s result (error=%t):", call.Name, item.IsError)}}, content...)
			result = append(result, Item{Kind: "user", Content: content, Time: item.Time})
		default:
			result = append(result, item)
		}
	}
	return result, nil
}
