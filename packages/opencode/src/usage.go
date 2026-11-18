package opencode

import (
	"encoding/json"
	"errors"
	"time"
)

type Window struct {
	Name     string
	Used     float64
	ResetsAt *time.Time
	Span     time.Duration
}

func ParseUsage(body []byte) ([]Window, error) {
	type window struct {
		Percent  *float64 `json:"percent"`
		ResetsAt string   `json:"resetsAt"`
	}
	var reply struct {
		Usage map[string]window `json:"usage"`
	}
	if json.Unmarshal(body, &reply) != nil {
		return nil, errors.New("OpenCode usage is not valid JSON")
	}
	var out []Window
	for _, spec := range []struct {
		key  string
		name string
		span time.Duration
	}{{"rolling", "5 hours", 5 * time.Hour}, {"weekly", "7 days", 7 * 24 * time.Hour}, {"monthly", "Month", 0}} {
		value, found := reply.Usage[spec.key]
		if !found || value.Percent == nil {
			continue
		}
		result := Window{Name: spec.name, Used: *value.Percent, Span: spec.span}
		if reset, err := time.Parse(time.RFC3339, value.ResetsAt); err == nil && result.Used > 0 {
			result.ResetsAt = &reset
		}
		out = append(out, result)
	}
	if len(out) == 0 {
		return nil, errors.New("OpenCode usage has no rolling, weekly or monthly percentage window")
	}
	return out, nil
}
