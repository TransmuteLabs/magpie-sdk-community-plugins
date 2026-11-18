package devin

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
	"unicode"
)

type NativeExecutionFacts struct {
	ServedModel      string
	UpstreamProvider string
	RequestID        string
	ResponseID       string
	Credits          *float64
	ACUCost          *float64
	actualModel      bool
	messageID        bool
}

func NativeStopName(stop uint64) string {
	names := [...]string{"UNSPECIFIED", "INCOMPLETE", "STOP_PATTERN", "MAX_TOKENS", "MIN_LOG_PROB",
		"MAX_NEWLINES", "EXIT_SCOPE", "NONFINITE_LOGIT_OR_PROB", "FIRST_NON_WHITESPACE_LINE",
		"PARTIAL", "FUNCTION_CALL", "CONTENT_FILTER", "NON_INSERTION", "ERROR"}
	if stop == 0 || stop >= uint64(len(names)) {
		return ""
	}
	return names[stop]
}

func nativeProviderName(provider uint64) string {
	names := map[uint64]string{
		1: "INTERNAL", 2: "OPENAI", 3: "GOOGLE_VERTEX", 4: "ANTHROPIC", 5: "VLLM",
		6: "TOGETHER_AI", 7: "HUGGING_FACE", 8: "NOMIC", 9: "TEI",
		10: "OPENAI_COMPATIBLE_EXTERNAL", 11: "ANTHROPIC_COMPATIBLE_EXTERNAL",
		12: "VERTEX_COMPATIBLE_EXTERNAL", 13: "BEDROCK_COMPATIBLE_EXTERNAL", 14: "AZURE_COMPATIBLE_EXTERNAL",
		15: "ANTHROPIC_BEDROCK", 16: "FIREWORKS", 17: "OPEN_ROUTER", 18: "XAI",
		20: "ANTHROPIC_BYOK", 21: "CEREBRAS", 22: "XAI_BYOK", 23: "GEMINI_OPENAI",
		24: "GOOGLE_GEMINI", 25: "GOOGLE_GENAI_VERTEX", 26: "ANTHROPIC_VERTEX", 27: "DATABRICKS",
		28: "OPEN_ROUTER_BYOK", 29: "ANTHROPIC_DEVIN", 30: "FIREWORKS_DEVIN", 31: "GROQ",
		32: "OPENAI_DEVIN", 33: "LLAMA_FT_DEEPWIKI", 34: "XAI_INTERNAL", 36: "FLOODGATE",
		37: "ANTHROPIC_BEDROCK_US", 38: "ANTHROPIC_BEDROCK_GLOBAL", 40: "MODAL",
		41: "GOOGLE_GEMINI_DEVIN", 42: "FIREWORKS_COGNITION", 43: "GOOGLE_GENAI_VERTEX_GLOBAL",
		44: "ANTHROPIC_VERTEX_US", 45: "ANTHROPIC_VERTEX_EU", 46: "ANTHROPIC_VERTEX_GLOBAL",
		47: "FIREWORKS_COGNITION_INTERNAL", 48: "ANTHROPIC_DATABRICKS", 49: "GEMINI_DATABRICKS",
		50: "MIMIC", 51: "ANTHROPIC_NON_ZDR", 52: "ANTHROPIC_DEVIN_NON_ZDR",
		53: "DATA_RETENTION_WORKSPACE_ANTHROPIC", 54: "TOGETHER_AI_SHADOW", 55: "OPENAI_STRICT_ZDR",
		56: "ANTHROPIC_VERTEX_US_MULTI", 57: "GOOGLE_GENAI_VERTEX_US_MULTI", 58: "COREWEAVE",
		59: "NEBIUS", 60: "SGLANG", 61: "AZURE_OPENAI_FEDERATED", 62: "DECART",
		63: "FIREWORKS_DEVIN_SERVERLESS", 64: "CLOUDFLARE", 65: "BASETEN", 66: "OPENAI_SAFETY_RETENTION",
		67: "DATABRICKS_OSS_US", 68: "BEDROCK_MANTLE_FEDERAL", 69: "APPLIED_COMPUTE",
		70: "OPENAI_INTERNAL_TESTING", 71: "MORPH", 72: "BEDROCK_MANTLE_FEDERAL_ANTHROPIC",
		73: "OPENAI_ZDR_US_IP_WHITELIST", 74: "ANTHROPIC_EFS_ZDR", 75: "ANTHROPIC_CYBER_VERIFIED",
		76: "ANTHROPIC_EAP_INTERNAL", 77: "PENGUIN_AWS_USW2", 78: "PENGUIN_CANADA",
		79: "ANTHROPIC_EFS_ZDR_US", 80: "DATA_RETENTION_WORKSPACE_ANTHROPIC_US",
		81: "OPENAI_STRICT_ZDR_US", 82: "OPENAI_SAFETY_RETENTION_US", 83: "GOOGLE_GENAI_VERTEX_SELECTOR",
		84: "ANTHROPIC_VERTEX_SELECTOR", 85: "BEDROCK_RUNTIME_FEDERAL", 86: "PENGUIN_CANADA_EVAL",
		87: "OPENAI_SIWC", 88: "COREWEAVE_US", 89: "BASETEN_US", 90: "OPENAI_DECISIONS",
		91: "OPENAI_DECISIONS_GLOBAL", 92: "OPENAI_DECISIONS_US", 93: "ANTHROPIC_US",
		94: "ANTHROPIC_GLOBAL", 95: "META", 96: "XAI_US", 97: "XAI_GLOBAL", 98: "PENGUIN_FIREWORKS",
	}
	if provider == 0 {
		return ""
	}
	name := names[provider]
	if name == "" {
		name = strconv.FormatUint(provider, 10)
	}
	return "API_PROVIDER_" + name
}

func nativeMetrics(fields message) (NativeSummary, error) {
	result := NativeSummary{Stop: fields.number(5), Facts: NativeExecutionFacts{
		ServedModel: fields.text(23), RequestID: fields.text(17), ResponseID: fields.text(1),
		actualModel: fields.text(23) != "", messageID: fields.text(1) != "",
	}}
	if len(fields[7]) != 0 {
		usage, err := nativeUsage(fields.data(7))
		if err != nil {
			return NativeSummary{}, err
		}
		result.Usage = usage
		stats, _ := readMessage(fields.data(7), nil)
		if stats.number(6) > math.MaxInt32 {
			return NativeSummary{}, &NativeError{Code: "data_loss"}
		}
		result.Facts.UpstreamProvider = nativeProviderName(stats.number(6))
		if result.Facts.ServedModel == "" {
			result.Facts.ServedModel = stats.text(9)
		}
		if result.Facts.ResponseID == "" {
			result.Facts.ResponseID = stats.text(7)
		}
	} else {
		var err error
		result.Usage, err = dimensionUsage(fields[28])
		if err != nil {
			return NativeSummary{}, err
		}
	}
	if len(fields[18]) != 0 {
		if fields.number(18) > math.MaxInt32 {
			return NativeSummary{}, &NativeError{Code: "data_loss"}
		}
		value := float64(fields.number(18))
		result.Facts.Credits = &value
	}
	if len(fields[22]) != 0 {
		value := math.Float64frombits(binary.LittleEndian.Uint64(fields.data(22)))
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return NativeSummary{}, &NativeError{Code: "data_loss"}
		}
		result.Facts.ACUCost = &value
	}
	for _, value := range []string{result.Facts.ServedModel, result.Facts.RequestID, result.Facts.ResponseID} {
		if len(value) > 300 || strings.ContainsFunc(value, unicode.IsControl) {
			return NativeSummary{}, &NativeError{Code: "data_loss"}
		}
	}
	return result, nil
}

func DecodeNativeMetrics(payload []byte) (NativeSummary, error) {
	fields, err := nativeResponseFields(payload)
	if err != nil {
		return NativeSummary{}, err
	}
	return nativeMetrics(fields)
}

func (summary *NativeSummary) Merge(update NativeSummary) {
	if update.Stop != 0 {
		summary.Stop = update.Stop
	}
	summary.Usage = mergeUsage(summary.Usage, update.Usage)
	if update.Facts.ServedModel != "" && (update.Facts.actualModel || !summary.Facts.actualModel) {
		summary.Facts.ServedModel, summary.Facts.actualModel = update.Facts.ServedModel, update.Facts.actualModel
	}
	if update.Facts.ResponseID != "" && (update.Facts.messageID || !summary.Facts.messageID) {
		summary.Facts.ResponseID, summary.Facts.messageID = update.Facts.ResponseID, update.Facts.messageID
	}
	if update.Facts.UpstreamProvider != "" {
		summary.Facts.UpstreamProvider = update.Facts.UpstreamProvider
	}
	if update.Facts.RequestID != "" {
		summary.Facts.RequestID = update.Facts.RequestID
	}
	if update.Facts.Credits != nil {
		copy := *update.Facts.Credits
		summary.Facts.Credits = &copy
	}
	if update.Facts.ACUCost != nil {
		copy := *update.Facts.ACUCost
		summary.Facts.ACUCost = &copy
	}
}
