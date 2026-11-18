package commandcode

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func systemOneSession() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&15 | 64
	value[8] = value[8]&63 | 128
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

func systemOneModel(model string) string {
	switch strings.ToLower(model) {
	case Jev, "jev", "jev-latest", "typesafe-ai/jev":
		return Jev
	}
	return ""
}

func systemOneProject(directory string) string {
	if directory == "" {
		directory, _ = os.UserHomeDir()
	}
	name := path.Base(strings.ReplaceAll(directory, "\\", "/"))
	if name == "." || name == "/" {
		name = "root"
	}
	for _, character := range name {
		if character < 0x20 || character > 0x7e && character < 0xa0 || character > 0xff {
			return strings.ReplaceAll(url.QueryEscape(name), "+", "%20")
		}
	}
	return name
}

func (controller *controller) systemOne(ctx context.Context, call sdk.Call, stream *sdk.Stream, auth credentials, input sdk.ExecuteRequest) (any, error) {
	if input.Protocol != "" || systemOneModel(input.Model) == "" {
		return nil, invalid("SystemOne requires its decision model and an empty protocol")
	}
	var options sdk.ExecuteOptions
	if len(input.Options) > 0 && decode(input.Options, &options) != nil ||
		options.Method != "" && options.Method != "POST" ||
		options.Path != "" && options.Path != "/v1/systemone" || options.Query != "" {
		return nil, invalid("SystemOne requires POST /v1/systemone without a query")
	}
	var fields map[string]json.RawMessage
	var model string
	if decode(input.Body, &fields) != nil || fields == nil || json.Unmarshal(fields["model"], &model) != nil ||
		systemOneModel(model) != systemOneModel(input.Model) {
		return nil, invalid("SystemOne requires a JSON object naming the selected decision model")
	}
	if value, present := fields["stream"]; present && string(value) != "false" {
		return nil, invalid("SystemOne does not declare streaming")
	}
	fields["model"] = json.RawMessage(`"typesafe/jev"`)
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, invalid("SystemOne request cannot be encoded")
	}
	target := strings.TrimRight(controller.config.APIURL, "/") + "/provider/v1/systemone"
	proxy := options.Proxy
	if proxy == "" {
		proxy, err = controller.proxy(ctx, stream, target)
		if err != nil {
			return nil, err
		}
	}
	httpClient, closeClient, err := client(proxy)
	if err != nil {
		return nil, err
	}
	defer closeClient()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, invalid("invalid SystemOne URL")
	}
	directory := controller.config.WorkingDirectory
	if encoded := input.Headers.Get("x-magpie-work-dir"); encoded != "" && len(encoded) <= 16384 {
		if decoded, err := url.PathUnescape(encoded); err == nil && decoded != "" {
			directory = decoded
		}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+auth.Key)
	request.Header.Set("User-Agent", "cli")
	request.Header.Set("x-command-code-version", cliVersion)
	request.Header.Set("x-cli-environment", "production")
	request.Header.Set("x-project-slug", systemOneProject(directory))
	request.Header.Set("x-taste-learning", "false")
	request.Header.Set("x-session-id", controller.session)
	if controller.config.ZDR {
		request.Header.Set("x-cmd-zdr", "1")
	}
	response, err := httpClient.Do(request)
	if err != nil {
		if emitErr := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); emitErr != nil {
			return nil, emitErr
		}
		return nil, &sdk.Error{Code: "upstream_error", Message: "SystemOne request outcome is uncertain; it was not replayed", Outcome: "uncertain"}
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
			response.Body.Close()
			return nil, err
		}
	}
	answer, readErr := bounded(response.Body, 4<<20)
	closeErr := response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if readErr == nil && closeErr == nil {
			if refused, proved := commandRefusal(response.StatusCode, response.Header, answer, auth.Key, "commandcode.systemone"); proved {
				return refused, nil
			}
		}
		if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
			return nil, err
		}
	}
	if readErr != nil || closeErr != nil {
		return nil, &sdk.Error{Code: "upstream_error", Message: "SystemOne returned an incomplete or oversized answer", Outcome: "uncertain"}
	}
	status := response.StatusCode
	headers := responseHeaders(response.Header)
	headers.Set("X-Magpie-Sign-In", "kept")
	outcome := "completed"
	var totals *sdk.Usage
	if status < 200 || status >= 300 {
		status, outcome = refusalStatus(status, answer), "not-started"
	} else {
		totals, err = systemOneUsage(answer)
		if err != nil {
			return nil, &sdk.Error{Code: "upstream_error", Message: "SystemOne returned invalid usage or answer JSON", Outcome: "uncertain"}
		}
	}
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: status, Headers: headers}); err != nil {
		return nil, err
	}
	for len(answer) > 0 {
		count := min(len(answer), sdk.MaxChunkBytes)
		if err := stream.Emit(ctx, "data", answer[:count]); err != nil {
			return nil, err
		}
		answer = answer[count:]
	}
	if err := stream.Emit(ctx, "flush", nil); err != nil {
		return nil, err
	}
	return sdk.ExecuteResult{Status: status, Headers: headers, Usage: totals, Outcome: outcome}, nil
}

func systemOneUsage(answer []byte) (*sdk.Usage, error) {
	var value struct {
		Usage *struct {
			Input  *int64 `json:"input_tokens"`
			Output *int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := decode(answer, &value); err != nil {
		return nil, err
	}
	if value.Usage == nil || value.Usage.Input == nil && value.Usage.Output == nil {
		return nil, nil
	}
	totals := &sdk.Usage{}
	if value.Usage.Input != nil && value.Usage.Output != nil {
		totals.InputTokens, totals.OutputTokens = *value.Usage.Input, *value.Usage.Output
	} else {
		totals.Facts = &sdk.ExecutionUsageFacts{ExecutionCounts: sdk.ExecutionCounts{
			InputTokens: value.Usage.Input, OutputTokens: value.Usage.Output,
		}}
	}
	return totals, sdk.ValidateExecutionUsage(Manifest(), totals)
}
