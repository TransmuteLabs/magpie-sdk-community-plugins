package devin

import (
	"errors"
	"slices"
	"strings"
)

func normalizeEffort(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "ultra" {
		value = "max"
	}
	if value != "" && !slices.Contains(effortOrder, value) {
		return "", errors.New("unsupported Devin reasoning effort")
	}
	return value, nil
}

func (request messagesRequest) effort() (string, error) {
	last := ""
	for _, turn := range request.Messages {
		if len(turn.OutputConfig) == 0 {
			continue
		}
		config, err := catalogObject(turn.OutputConfig)
		if err != nil || turn.Role != "system" || len(config) != 1 {
			return "", errors.New("unsupported Messages turn output config")
		}
		effort, err := catalogString(config, "effort")
		if err != nil {
			return "", err
		}
		last, err = normalizeEffort(effort)
		if err != nil || last == "" {
			return "", errors.New("unsupported Messages turn effort")
		}
	}
	if request.Thinking != nil {
		switch request.Thinking.Type {
		case "disabled":
			return "", nil
		case "enabled", "adaptive":
		default:
			return "", errors.New("unsupported Messages thinking control")
		}
	}
	if request.OutputConfig != nil && request.OutputConfig.Effort != "" {
		top, err := normalizeEffort(request.OutputConfig.Effort)
		if err != nil || last == "" {
			return top, err
		}
	}
	if last != "" {
		return last, nil
	}
	if request.Thinking == nil || request.Thinking.Budget == nil || *request.Thinking.Budget == 0 {
		return "", nil
	}
	budget := *request.Thinking.Budget
	switch {
	case budget <= 4096:
		return "low", nil
	case budget <= 12000:
		return "medium", nil
	case budget <= 24000:
		return "high", nil
	case budget <= 9007199254740991:
		return "xhigh", nil
	default:
		return "", errors.New("Messages thinking budget exceeds the exact integer range")
	}
}
