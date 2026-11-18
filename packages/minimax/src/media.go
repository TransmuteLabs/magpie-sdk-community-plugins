package minimax

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/protocolext"
)

const imageModel = "MiniMax-Image"

var videoModels = []string{"MiniMax-H3", "MiniMax-H3-Max", "MiniMax-Hailuo-2.3"}

func mediaOperations() []sdk.ProviderOperation {
	return []sdk.ProviderOperation{sdk.OperationImageGenerate, sdk.OperationImageEdit, sdk.OperationVideoCreate, sdk.OperationVideoGet, sdk.OperationVideoContent}
}

func mediaModels() []sdk.Model {
	models := []sdk.Model{{ID: imageModel, Name: "MiniMax Image", Protocols: []protocolext.Protocol{protocolext.Chat}, Operations: []sdk.ProviderOperation{sdk.OperationImageGenerate, sdk.OperationImageEdit}, Vision: true, InputModalities: []string{"text", "image"}, OutputModalities: []string{"image"}, Efforts: []string{}}}
	for _, model := range videoModels {
		models = append(models, sdk.Model{ID: model, Name: model, Protocols: []protocolext.Protocol{protocolext.Chat}, Operations: []sdk.ProviderOperation{sdk.OperationVideoCreate, sdk.OperationVideoGet, sdk.OperationVideoContent}, Vision: true, InputModalities: []string{"text", "image"}, OutputModalities: []string{"video"}, Efforts: []string{}})
	}
	return models
}

type mediaEngine struct {
	matrixEngine
	now  func() time.Time
	wait func(context.Context, time.Duration) error
}

func waitMedia(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (owner *module) executeMedia(ctx context.Context, call sdk.Call, stream *sdk.Stream, request sdk.ExecuteRequest) (any, error) {
	client, err := owner.SelectedClient(ctx, call, stream, "")
	if err != nil {
		return nil, err
	}
	accepted := false
	engine := mediaEngine{
		matrixEngine: matrixEngine{client: client, accept: func(ctx context.Context) error {
			if accepted {
				return nil
			}
			if stream == nil {
				return denied("A host acceptance receipt is required")
			}
			if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
				return err
			}
			accepted = true
			return nil
		}},
		now:  owner.options.Now,
		wait: waitMedia,
	}
	response, err := engine.execute(ctx, request)
	if err != nil {
		err = client.redactedFailure(err)
		if accepted {
			return nil, keepFailureDetails(uncertain("MiniMax media request did not complete; the accepted operation must not be replayed"), err)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		message := "MiniMax media request failed"
		var failure *mediaFailure
		if errors.As(err, &failure) {
			message = failure.Message
		}
		response = mediaJSON(map[string]any{"error": map[string]any{"message": message, "type": "api_error"}})
		response.Status = toolHTTPStatus(err)
	}
	if stream == nil {
		return nil, denied("A host response stream is required")
	}
	response.Header.Set("X-Magpie-Sign-In", "kept")
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: response.Status, Headers: response.Header}); err != nil {
		return nil, err
	}
	for offset := 0; offset < len(response.Body); offset += sdk.MaxChunkBytes {
		end := min(offset+sdk.MaxChunkBytes, len(response.Body))
		if err := stream.Emit(ctx, "data", response.Body[offset:end]); err != nil {
			return nil, err
		}
	}
	return sdk.ExecuteResult{Status: response.Status, Headers: response.Header, Outcome: "completed"}, nil
}

func mediaJSON(value any) ByteResponse {
	return ByteResponse{Status: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: raw(value)}
}

func (engine mediaEngine) execute(ctx context.Context, request sdk.ExecuteRequest) (ByteResponse, error) {
	if err := ctx.Err(); err != nil {
		return ByteResponse{}, err
	}
	if request.Protocol != "chat" {
		return ByteResponse{}, mediaInvalid("Media operations require the chat protocol envelope")
	}
	var options sdk.ExecuteOptions
	if len(request.Options) != 0 && json.Unmarshal(request.Options, &options) != nil {
		return ByteResponse{}, mediaInvalid("Invalid media execute options")
	}
	expectedMethod := http.MethodPost
	if request.Operation == sdk.OperationVideoGet || request.Operation == sdk.OperationVideoContent {
		expectedMethod = http.MethodGet
	}
	if options.Method != "" && strings.ToUpper(options.Method) != expectedMethod {
		return ByteResponse{}, &mediaFailure{Status: 405, Message: "Incorrect method for the media operation"}
	}
	switch request.Operation {
	case sdk.OperationImageGenerate, sdk.OperationImageEdit:
		if request.Model != imageModel {
			return ByteResponse{}, mediaInvalid("Image operation requires MiniMax-Image")
		}
		input, err := readMediaInput(request.Body, request.Headers)
		if err != nil {
			return ByteResponse{}, err
		}
		if bodyModel := text(input.Fields["model"]); bodyModel != "" && bodyModel != request.Model {
			return ByteResponse{}, mediaInvalid("Media body model differs from the selected model")
		}
		return engine.generateImage(ctx, input, request.Operation == sdk.OperationImageEdit)
	case sdk.OperationVideoCreate:
		if !contains(videoModels, request.Model) {
			return ByteResponse{}, mediaInvalid("Video operation requires a MiniMax video model")
		}
		input, err := readMediaInput(request.Body, request.Headers)
		if err != nil {
			return ByteResponse{}, err
		}
		if bodyModel := text(input.Fields["model"]); bodyModel != "" && bodyModel != request.Model {
			return ByteResponse{}, mediaInvalid("Media body model differs from the selected model")
		}
		return engine.createVideo(ctx, request.Model, input.Fields)
	case sdk.OperationVideoGet, sdk.OperationVideoContent:
		identifier, model, task, err := videoIdentity(options, request.Operation)
		if err != nil {
			return ByteResponse{}, err
		}
		if request.Model != "" && request.Model != model {
			return ByteResponse{}, mediaInvalid("Video ID differs from the selected model")
		}
		return engine.readVideo(ctx, identifier, model, task, request.Operation == sdk.OperationVideoContent)
	default:
		return ByteResponse{}, mediaInvalid("Unsupported MiniMax media operation")
	}
}

func (engine mediaEngine) generateImage(ctx context.Context, input mediaInput, edits bool) (ByteResponse, error) {
	prompt := text(input.Fields["prompt"])
	if prompt == "" {
		return ByteResponse{}, mediaInvalid("prompt is required")
	}
	count := mediaInteger(input.Fields["n"], 1)
	if count < 1 || count > 10 {
		return ByteResponse{}, mediaInvalid("n must be an integer from 1 to 10")
	}
	references, err := imageReferences(input, edits)
	if err != nil {
		return ByteResponse{}, err
	}
	item := map[string]any{"prompt": prompt, "n": count}
	aspect := text(input.Fields["aspect_ratio"])
	if aspect == "" {
		aspect = mediaAspect(text(input.Fields["size"]))
	}
	if aspect != "" {
		item["aspect_ratio"] = aspect
	}
	if area := mediaArea(input.Fields); area != "" {
		item["area"] = area
	}
	if err := engine.accept(ctx); err != nil {
		return ByteResponse{}, err
	}
	if len(references) > 0 {
		item["reference_images"], err = engine.resolveReferences(ctx, references)
		if err != nil {
			return ByteResponse{}, err
		}
	}
	value, err := engine.matrix(ctx, "generate_image", map[string]any{"requests": []any{item}}, 10*time.Minute)
	if err != nil {
		return ByteResponse{}, err
	}
	items, ok := object(value)["success_items"].([]any)
	if !ok || len(items) == 0 {
		return ByteResponse{}, toolError("MiniMax Image returned no images")
	}
	images := []map[string]string{}
	for _, item := range items {
		node := text(object(item)["node_id"])
		if node == "" {
			return ByteResponse{}, toolError("MiniMax Image returned no node_id")
		}
		address, err := engine.assetURL(ctx, node)
		if err != nil {
			return ByteResponse{}, err
		}
		images = append(images, map[string]string{"url": address})
	}
	return mediaJSON(map[string]any{"created": engine.now().Unix(), "data": images}), nil
}

func (engine mediaEngine) assetURL(ctx context.Context, node string) (string, error) {
	deadline := engine.now().Add(8 * time.Second)
	for attempt := 0; ; attempt++ {
		result, err := engine.matrix(ctx, "get_asset_url", map[string]any{"node_id": node}, 55*time.Second)
		if err != nil {
			return "", err
		}
		address := firstMediaText(object(result), "download_url", "url", "temp_url")
		if !mediaHTTPS(address) {
			return "", toolError("get_asset_url returned no valid HTTPS URL")
		}
		remaining := deadline.Sub(engine.now())
		if !strings.Contains(address, "/visible/") || remaining <= 0 || attempt >= 8 {
			return address, nil
		}
		if err := engine.wait(ctx, min(time.Second, remaining)); err != nil {
			return "", err
		}
	}
}

func (engine mediaEngine) createVideo(ctx context.Context, model string, fields map[string]any) (ByteResponse, error) {
	prompt := text(fields["prompt"])
	if prompt == "" {
		return ByteResponse{}, mediaInvalid("prompt is required")
	}
	duration, err := mediaDuration(fields["seconds"], model)
	if err != nil {
		return ByteResponse{}, err
	}
	startValue := fields["input_reference"]
	if startValue == nil {
		startValue = fields["input_image"]
	}
	start, err := parseMediaReference(startValue)
	if err != nil {
		return ByteResponse{}, err
	}
	endValue := fields["last_frame"]
	if endValue == nil {
		endValue = fields["last_frame_image"]
	}
	end, err := parseMediaReference(endValue)
	if err != nil {
		return ByteResponse{}, err
	}
	references, err := parseMediaList(fields["reference_images"])
	if err != nil {
		return ByteResponse{}, err
	}
	arguments := map[string]any{"model": model, "prompt": prompt, "duration": duration, "resolution": mediaResolution(text(fields["size"]), model)}
	ratio := text(fields["ratio"])
	if ratio == "" {
		ratio = mediaAspect(text(fields["size"]))
	}
	if ratio != "" && ratio != "auto" {
		arguments["ratio"] = ratio
	}
	if err := engine.accept(ctx); err != nil {
		return ByteResponse{}, err
	}
	for _, item := range []struct {
		name      string
		reference *mediaReference
	}{{"input_image", start}, {"last_frame_image", end}} {
		if item.reference != nil {
			arguments[item.name], err = engine.resolveReference(ctx, item.reference)
			if err != nil {
				return ByteResponse{}, err
			}
		}
	}
	if start != nil {
		arguments["reference_type"] = "first_frame"
	}
	if len(references) > 0 {
		arguments["reference_images"], err = engine.resolveReferences(ctx, references)
		if err != nil {
			return ByteResponse{}, err
		}
	}
	result, err := engine.matrix(ctx, "submit_video_generation", arguments, 55*time.Second)
	if err != nil {
		return ByteResponse{}, err
	}
	task := firstMediaText(object(result), "task_id", "taskId", "id")
	if task == "" {
		return ByteResponse{}, toolError("MiniMax video returned no task_id")
	}
	return mediaJSON(map[string]any{"id": model + "::" + task, "object": "video", "created_at": engine.now().Unix(), "status": "queued", "progress": 0, "model": model, "seconds": strconv.Itoa(duration)}), nil
}

func videoIdentity(options sdk.ExecuteOptions, operation sdk.ProviderOperation) (string, string, string, error) {
	endpoint := options.EscapedPath
	if endpoint == "" {
		endpoint = options.Path
	}
	if operation == sdk.OperationVideoContent {
		endpoint = strings.TrimSuffix(endpoint, "/content")
	}
	_, encoded, found := strings.Cut(endpoint, "/videos/")
	identifier, err := url.PathUnescape(encoded)
	model, task, split := strings.Cut(identifier, "::")
	if !found || err != nil || !split || task == "" || !contains(videoModels, model) || strings.Contains(encoded, "/") {
		return "", "", "", &mediaFailure{Status: 404, Message: "Not a MiniMax video ID"}
	}
	return identifier, model, task, nil
}

func (engine mediaEngine) readVideo(ctx context.Context, identifier, model, task string, content bool) (ByteResponse, error) {
	value, err := engine.matrix(ctx, "query_video_generation", map[string]any{"model": model, "task_id": task}, 55*time.Second)
	if err != nil {
		return ByteResponse{}, err
	}
	result := object(value)
	if result == nil {
		return ByteResponse{}, toolError("Malformed video status")
	}
	status, progress := videoState(result)
	if !content {
		response := map[string]any{"id": identifier, "object": "video", "created_at": mediaInteger(result["created_at"], engine.now().Unix()), "status": status, "progress": progress, "model": model}
		if duration := result["duration"]; duration != nil {
			response["seconds"] = fmt.Sprint(duration)
		}
		if status == "failed" {
			response["error"] = map[string]string{"code": "failed", "message": "MiniMax video generation failed"}
		}
		return mediaJSON(response), nil
	}
	if status != "completed" {
		return ByteResponse{}, &mediaFailure{Status: 409, Message: "The video is not ready yet (" + status + ")"}
	}
	node := text(result["node_id"])
	if items, ok := result["success_items"].([]any); node == "" && ok && len(items) > 0 {
		node = text(object(items[0])["node_id"])
	}
	if node == "" {
		return ByteResponse{}, toolError("MiniMax video returned no node_id")
	}
	address, err := engine.assetURL(ctx, node)
	if err != nil {
		return ByteResponse{}, err
	}
	response, err := engine.client.Transfer(ctx, http.MethodGet, address, nil, nil, 64<<20)
	if err != nil {
		return ByteResponse{}, err
	}
	if response.Status < 200 || response.Status >= 300 {
		return ByteResponse{}, transferFailure(toolError("MiniMax video download failed"), response)
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		if encoding != "gzip" {
			return ByteResponse{}, toolError("Unsupported video content encoding")
		}
		reader, err := gzip.NewReader(bytes.NewReader(response.Body))
		if err != nil {
			return ByteResponse{}, toolError("Invalid compressed video")
		}
		decoded, readError := io.ReadAll(io.LimitReader(reader, (64<<20)+1))
		reader.Close()
		if readError != nil || len(decoded) > 64<<20 {
			return ByteResponse{}, toolError("Video content exceeds its byte limit or is incomplete")
		}
		response.Body = decoded
	}
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "video/mp4"
	}
	response.Header = http.Header{"Content-Type": {contentType}}
	response.Status = http.StatusOK
	return response, nil
}
