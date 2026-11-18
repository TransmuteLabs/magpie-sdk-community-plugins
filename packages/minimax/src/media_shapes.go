package minimax

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var mediaSizePattern = regexp.MustCompile(`(?i)^(\d+)\s*[x×]\s*(\d+)$`)
var mediaRatioPattern = regexp.MustCompile(`^\d+:\d+$`)

func mediaDimensions(value string) (int64, int64) {
	match := mediaSizePattern.FindStringSubmatch(value)
	if match == nil {
		return 0, 0
	}
	width, widthError := strconv.ParseInt(match[1], 10, 32)
	height, heightError := strconv.ParseInt(match[2], 10, 32)
	if widthError != nil || heightError != nil {
		return 0, 0
	}
	return width, height
}

func mediaAspect(size string) string {
	if mediaRatioPattern.MatchString(size) || size == "auto" || size == "adaptive" {
		return size
	}
	width, height := mediaDimensions(size)
	if width == 0 || height == 0 {
		return ""
	}
	divisor, remainder := width, height
	for remainder != 0 {
		divisor, remainder = remainder, divisor%remainder
	}
	return strconv.FormatInt(width/divisor, 10) + ":" + strconv.FormatInt(height/divisor, 10)
}

func mediaArea(fields map[string]any) string {
	if area := text(fields["area"]); contains([]string{"1K", "2K", "4K"}, area) {
		return area
	}
	switch strings.ToLower(text(fields["quality"])) {
	case "low":
		return "1K"
	case "medium":
		return "2K"
	case "high", "xhigh", "max":
		return "4K"
	}
	size := text(fields["size"])
	if contains([]string{"1K", "2K", "4K"}, size) {
		return size
	}
	width, height := mediaDimensions(size)
	longest := max(width, height)
	switch {
	case longest == 0:
		return ""
	case longest <= 1280:
		return "1K"
	case longest <= 2560:
		return "2K"
	default:
		return "4K"
	}
}

func mediaDuration(value any, model string) (int, error) {
	duration := 6.0
	if value != nil && value != "" {
		parsed, ok := number(value)
		if !ok || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return 0, mediaInvalid("seconds must be a number")
		}
		duration = math.Floor(parsed + 0.5)
	}
	if model == "MiniMax-Hailuo-2.3" {
		if duration >= 10 {
			return 10, nil
		}
		return 6, nil
	}
	minimum := 4.0
	if model == "MiniMax-H3-Max" {
		minimum = 5
	}
	return int(math.Min(15, math.Max(minimum, duration))), nil
}

func mediaResolution(size, model string) string {
	fallback := "768P"
	if model == "MiniMax-H3" {
		fallback = "2K"
	}
	upper := strings.ToUpper(size)
	resolution := fallback
	switch {
	case contains([]string{"480P", "768P", "1080P", "2K"}, upper):
		resolution = upper
	case strings.Contains(upper, "480") || strings.Contains(upper, "640"):
		resolution = "480P"
	case strings.Contains(upper, "1080") || strings.Contains(upper, "1920"):
		resolution = "2K"
		if model == "MiniMax-Hailuo-2.3" {
			resolution = "1080P"
		}
	case strings.Contains(upper, "2K") || strings.Contains(upper, "1440") || strings.Contains(upper, "2560"):
		resolution = "2K"
	case strings.Contains(upper, "720") || strings.Contains(upper, "768") || strings.Contains(upper, "1280"):
		resolution = "768P"
	}
	allowed := map[string][]string{"MiniMax-H3": {"768P", "2K"}, "MiniMax-H3-Max": {"480P", "768P"}, "MiniMax-Hailuo-2.3": {"768P", "1080P"}}[model]
	if !contains(allowed, resolution) {
		return allowed[len(allowed)-1]
	}
	return resolution
}

func mediaInteger(value any, fallback int64) int64 {
	parsed, ok := number(value)
	if !ok || math.Trunc(parsed) != parsed || parsed < -1<<53 || parsed > 1<<53 {
		return fallback
	}
	return int64(parsed)
}

func videoState(result map[string]any) (string, int64) {
	status := strings.ToLower(firstMediaText(result, "status", "task_status", "state", "taskStatus"))
	progress := mediaInteger(result["progress"], 0)
	switch status {
	case "succeeded", "success", "completed", "done":
		return "completed", 100
	case "failed", "error", "cancelled", "canceled", "expired":
		return "failed", progress
	case "running", "processing", "in_progress", "generating":
		if progress <= 0 || progress >= 100 {
			progress = 50
		}
		return "in_progress", progress
	default:
		return "queued", progress
	}
}
