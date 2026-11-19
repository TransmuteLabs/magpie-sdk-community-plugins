package minimax

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func mediaFixture(test *testing.T, names ...string) (*toolsFakeTransport, mediaEngine, *int) {
	fixture, matrix, accepted := toolsFixture(test, names...)
	clock := time.Unix(1700000000, 0)
	engine := mediaEngine{matrixEngine: matrix, now: func() time.Time { return clock }, wait: func(ctx context.Context, duration time.Duration) error {
		clock = clock.Add(duration)
		return ctx.Err()
	}}
	return fixture, engine, accepted
}

func mediaRequest(operation sdk.ProviderOperation, model string, body any) sdk.ExecuteRequest {
	return sdk.ExecuteRequest{Protocol: "chat", Operation: operation, Model: model, Body: raw(body), Headers: http.Header{"Content-Type": {"application/json"}}}
}

func mediaResponse(test *testing.T, response ByteResponse) map[string]any {
	test.Helper()
	var result map[string]any
	if err := json.Unmarshal(response.Body, &result); err != nil {
		test.Fatal(err)
	}
	return result
}

func TestMediaImageGenerationVisibilityWorkflowNeverRegenerates(test *testing.T) {
	fixture, engine, accepted := mediaFixture(test, "generate_image", "get_asset_url")
	generated, assets := 0, 0
	fixture.respond = func(_ context.Context, name string, arguments map[string]any) (any, error) {
		switch name {
		case "generate_image":
			generated++
			item := object(arguments["requests"].([]any)[0])
			if !reflect.DeepEqual(item, map[string]any{"prompt": "fixture", "n": float64(1), "aspect_ratio": "1:1", "area": "1K"}) {
				test.Fatalf("wrong image arguments: %#v", item)
			}
			return toolsSuccess(map[string]any{"success_items": []any{map[string]any{"node_id": "image-node"}}}), nil
		case "get_asset_url":
			assets++
			if arguments["node_id"] != "image-node" {
				test.Fatal("asset lookup lost generation node")
			}
			address := "https://cdn.example/visible/image.png"
			if assets == 2 {
				address = "https://cdn.example/original/image.png"
			}
			return toolsSuccess(map[string]any{"download_url": address}), nil
		}
		test.Fatalf("unexpected tool %s", name)
		return nil, nil
	}
	response, err := engine.execute(context.Background(), mediaRequest(sdk.OperationImageGenerate, imageModel, map[string]any{"prompt": "fixture", "size": "1024x1024"}))
	if err != nil {
		test.Fatal(err)
	}
	images := mediaResponse(test, response)["data"].([]any)
	if object(images[0])["url"] != "https://cdn.example/original/image.png" || generated != 1 || assets != 2 || *accepted != 1 || len(fixture.calls) != 6 {
		test.Fatal("image lifecycle or fresh routing changed")
	}
}

func TestMediaAsset401DoesNotRepeatGeneration(test *testing.T) {
	fixture, engine, _ := mediaFixture(test, "generate_image", "get_asset_url")
	generated, assets := 0, 0
	fixture.respond = func(_ context.Context, name string, _ map[string]any) (any, error) {
		if name == "generate_image" {
			generated++
			return toolsSuccess(map[string]any{"success_items": []any{map[string]any{"node_id": "image-node"}}}), nil
		}
		assets++
		return nil, &APIError{Status: 401, Code: "http"}
	}
	_, err := engine.execute(context.Background(), mediaRequest(sdk.OperationImageGenerate, imageModel, map[string]any{"prompt": "fixture"}))
	if err == nil || generated != 1 || assets != 1 || len(fixture.calls) != 4 {
		test.Fatal("asset refusal replayed generation or hid failure")
	}
}

func TestMediaWatermarkBoundAndCancellation(test *testing.T) {
	fixture, engine, _ := mediaFixture(test, "get_asset_url")
	fixture.respond = func(context.Context, string, map[string]any) (any, error) {
		return toolsSuccess(map[string]any{"url": "https://cdn.example/visible/image.png"}), nil
	}
	address, err := engine.assetURL(context.Background(), "node")
	if err != nil || address != "https://cdn.example/visible/image.png" || len(fixture.calls) != 18 {
		test.Fatalf("semantic visibility wait not bounded: %s %v calls=%d", address, err, len(fixture.calls))
	}
	ctx, cancel := context.WithCancel(context.Background())
	engine.wait = func(context.Context, time.Duration) error { cancel(); return ctx.Err() }
	fixture.calls = nil
	if _, err := engine.assetURL(ctx, "node"); !errors.Is(err, context.Canceled) || len(fixture.calls) != 2 {
		test.Fatal("cancelled visibility workflow continued")
	}
}

func mediaMultipart(test *testing.T, mask bool) ([]byte, http.Header, []byte) {
	test.Helper()
	imageBytes := []byte{0x89, 'P', 'N', 'G', 0, 1, 2, 3}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("prompt", "fixture"); err != nil {
		test.Fatal(err)
	}
	fields := []string{"image"}
	if mask {
		fields = append(fields, "mask")
	}
	for _, field := range fields {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": field, "filename": "fixture.png"}))
		header.Set("Content-Type", "application/octet-stream")
		part, err := writer.CreatePart(header)
		if err != nil {
			test.Fatal(err)
		}
		if _, err := part.Write(imageBytes); err != nil {
			test.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		test.Fatal(err)
	}
	return body.Bytes(), http.Header{"Content-Type": {writer.FormDataContentType()}}, imageBytes
}

func TestMediaMultipartUploadPreservesBytesAndUsesFreshCatalog(test *testing.T) {
	fixture, engine, accepted := mediaFixture(test, "upload_temp_url", "generate_image", "get_asset_url")
	fixture.catalog = toolsCatalog(toolsEntry("upload_temp_url", `{"type":"object","properties":{"filename":{"type":"string"},"mime_type":{"type":"string"}},"required":["filename","mime_type"],"additionalProperties":false}`), toolsEntry("generate_image", "{}"), toolsEntry("get_asset_url", "{}"))
	body, headers, expected := mediaMultipart(test, false)
	uploads := 0
	fixture.transfer = func(_ context.Context, method, address string, body []byte, headers http.Header) (ByteResponse, error) {
		uploads++
		if method != "POST" || address != "https://upload.example/put" || len(headers) != 1 || headers.Get("Authorization") != "" {
			test.Fatalf("upload authority or credentials changed: %s %s %#v", method, address, headers)
		}
		_, parameters, err := mime.ParseMediaType(headers.Get("Content-Type"))
		if err != nil {
			test.Fatal(err)
		}
		form, err := multipart.NewReader(bytes.NewReader(body), parameters["boundary"]).ReadForm(1 << 20)
		if err != nil {
			test.Fatal(err)
		}
		defer form.RemoveAll()
		file, err := form.File["file"][0].Open()
		if err != nil {
			test.Fatal(err)
		}
		defer file.Close()
		actual, err := io.ReadAll(file)
		if err != nil || !bytes.Equal(actual, expected) || form.Value["policy"][0] != "signed" {
			test.Fatal("upload bytes/fields changed")
		}
		return ByteResponse{Status: 204}, nil
	}
	fixture.respond = func(_ context.Context, name string, arguments map[string]any) (any, error) {
		switch name {
		case "upload_temp_url":
			if arguments["filename"] != "fixture.png" || arguments["mime_type"] != "image/png" {
				test.Fatal("MIME sniff or filename lost")
			}
			return toolsSuccess(map[string]any{"upload_host": "https://upload.example/put", "temp_url": "https://temp.example/frame.png", "fields": map[string]any{"policy": "signed"}}), nil
		case "generate_image":
			item := object(arguments["requests"].([]any)[0])
			reference := object(item["reference_images"].([]any)[0])
			if reference["url"] != "https://temp.example/frame.png" || reference["mime_type"] != "image/png" {
				test.Fatal("signed upload reference not used")
			}
			return toolsSuccess(map[string]any{"success_items": []any{map[string]any{"node_id": "node"}}}), nil
		default:
			return toolsSuccess(map[string]any{"download_url": "https://cdn.example/result.png"}), nil
		}
	}
	request := mediaRequest(sdk.OperationImageEdit, imageModel, nil)
	request.Body, request.Headers = body, headers
	if _, err := engine.execute(context.Background(), request); err != nil {
		test.Fatal(err)
	}
	if uploads != 1 || *accepted != 1 || len(fixture.calls) != 6 {
		test.Fatal("upload/generation lifecycle changed")
	}
}

func TestMediaMaskInvalidReferenceAndMissingImageFailBeforeAcceptance(test *testing.T) {
	fixture, engine, accepted := mediaFixture(test)
	body, headers, _ := mediaMultipart(test, true)
	requests := []sdk.ExecuteRequest{{Protocol: "chat", Operation: sdk.OperationImageEdit, Model: imageModel, Body: body, Headers: headers}, mediaRequest(sdk.OperationImageEdit, imageModel, map[string]any{"prompt": "fixture"})}
	for _, address := range []string{"http://cdn.example/x", "file:///tmp/private", "/tmp/private", "https://user:secret@cdn.example/x", "data:image/png;base64,!!!", "data:image/png,%GG"} {
		requests = append(requests, mediaRequest(sdk.OperationImageGenerate, imageModel, map[string]any{"prompt": "fixture", "image": address}))
	}
	for _, request := range requests {
		if _, err := engine.execute(context.Background(), request); err == nil || toolHTTPStatus(err) != 400 {
			test.Fatalf("invalid input not refused: %v", err)
		}
	}
	if *accepted != 0 || len(fixture.calls) != 0 {
		test.Fatal("invalid media uploaded or generated")
	}
}

func TestMediaVideoDataReferenceAndInlineUploadAliases(test *testing.T) {
	fixture, engine, _ := mediaFixture(test, "upload_temp_url", "submit_video_generation")
	fixture.catalog = toolsCatalog(toolsEntry("upload_temp_url", `{"type":"object","properties":{"fileName":{"type":"string"},"mimeType":{"type":"string"},"size_bytes":{"type":"integer"},"content_base64":{"type":"string"}},"required":["content_base64"],"additionalProperties":false}`), toolsEntry("submit_video_generation", "{}"))
	imageBytes := []byte("fixture bytes")
	fixture.respond = func(_ context.Context, name string, arguments map[string]any) (any, error) {
		if name == "upload_temp_url" {
			decoded, err := base64.StdEncoding.DecodeString(text(arguments["content_base64"]))
			if err != nil || !bytes.Equal(decoded, imageBytes) || arguments["size_bytes"] != float64(len(imageBytes)) {
				test.Fatal("inline bytes/size alias lost")
			}
			return toolsSuccess(map[string]any{"temp_url": "https://temp.example/frame.png"}), nil
		}
		if arguments["duration"] != float64(8) || arguments["resolution"] != "2K" || arguments["ratio"] != "16:9" || arguments["reference_type"] != "first_frame" || object(arguments["input_image"])["url"] != "https://temp.example/frame.png" {
			test.Fatalf("wrong video create args: %#v", arguments)
		}
		return toolsSuccess(map[string]any{"task_id": "task-9"}), nil
	}
	request := mediaRequest(sdk.OperationVideoCreate, "MiniMax-H3", map[string]any{"prompt": "fixture", "seconds": "8", "size": "1920x1080", "input_reference": "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)})
	response, err := engine.execute(context.Background(), request)
	if err != nil {
		test.Fatal(err)
	}
	result := mediaResponse(test, response)
	if result["id"] != "MiniMax-H3::task-9" || result["status"] != "queued" || result["seconds"] != "8" || len(fixture.calls) != 4 {
		test.Fatalf("wrong submitted video: %#v", result)
	}
}

func TestMediaVideoTextH3SendsConcreteRatio(test *testing.T) {
	for _, entry := range []struct {
		name   string
		model  string
		fields map[string]any
		ratio  string
	}{
		{"h3-default", "MiniMax-H3", nil, "16:9"},
		{"max-auto", "MiniMax-H3-Max", map[string]any{"ratio": "auto"}, "16:9"},
		{"h3-adaptive", "MiniMax-H3", map[string]any{"ratio": "adaptive"}, "16:9"},
		{"explicit-portrait", "MiniMax-H3", map[string]any{"ratio": "9:16"}, "9:16"},
		{"size-derived", "MiniMax-H3", map[string]any{"size": "1080x1920"}, "9:16"},
		{"auto-size-derived", "MiniMax-H3-Max", map[string]any{"ratio": "auto", "size": "1080x1920"}, "9:16"},
		{"auto-size-outside-enum", "MiniMax-H3-Max", map[string]any{"ratio": "auto", "size": "1024x1792"}, "16:9"},
		{"default-size-outside-enum", "MiniMax-H3", map[string]any{"size": "1024x1792"}, "16:9"},
		{"adaptive-size", "MiniMax-H3", map[string]any{"size": "adaptive"}, "16:9"},
		{"keyframe-default", "MiniMax-H3", map[string]any{"input_reference": "https://example.com/frame.png"}, ""},
		{"keyframe-adaptive", "MiniMax-H3-Max", map[string]any{"input_reference": "https://example.com/frame.png", "ratio": "adaptive"}, "adaptive"},
		{"hailuo-default", "MiniMax-Hailuo-2.3", nil, ""},
	} {
		test.Run(entry.name, func(test *testing.T) {
			fixture, engine, accepted := mediaFixture(test, "submit_video_generation")
			schema := matrixDefinitions()["submit_video_generation"].Schema
			fixture.catalog = toolsCatalog(toolsEntry("submit_video_generation", string(raw(schema))))
			fixture.respond = func(_ context.Context, _ string, arguments map[string]any) (any, error) {
				if ratio := text(arguments["ratio"]); ratio != entry.ratio {
					test.Errorf("submitted ratio = %q, want %q", ratio, entry.ratio)
				}
				return toolsSuccess(map[string]any{"task_id": "fixture"}), nil
			}
			fields := map[string]any{"prompt": "fixture"}
			for name, value := range entry.fields {
				fields[name] = value
			}
			request := mediaRequest(sdk.OperationVideoCreate, entry.model, fields)
			response, err := engine.execute(test.Context(), request)
			if err != nil {
				test.Fatal(err)
			}
			if mediaResponse(test, response)["id"] != entry.model+"::fixture" || *accepted != 1 {
				test.Fatal("video request was not submitted once")
			}
		})
	}
}

func TestMediaVideoQueryAndContentDecodeAndStripTransportHeaders(test *testing.T) {
	fixture, engine, _ := mediaFixture(test, "query_video_generation", "get_asset_url")
	fixture.respond = func(_ context.Context, name string, arguments map[string]any) (any, error) {
		if name == "query_video_generation" {
			if arguments["task_id"] != "task/9" || arguments["model"] != "MiniMax-H3" {
				test.Fatal("encoded task identity changed")
			}
			return toolsSuccess(map[string]any{"status": "succeeded", "node_id": "video-node", "duration": 8}), nil
		}
		return toolsSuccess(map[string]any{"download_url": "https://cdn.example/video.mp4"}), nil
	}
	fixture.transfer = func(_ context.Context, method, address string, _ []byte, headers http.Header) (ByteResponse, error) {
		if method != "GET" || address != "https://cdn.example/video.mp4" || len(headers) != 0 {
			test.Fatal("download received account headers or wrong URL")
		}
		var body bytes.Buffer
		compressed := gzip.NewWriter(&body)
		if _, err := compressed.Write([]byte("MP4")); err != nil {
			test.Fatal(err)
		}
		if err := compressed.Close(); err != nil {
			test.Fatal(err)
		}
		return ByteResponse{Status: 200, Header: http.Header{"Content-Encoding": {"gzip"}, "Content-Length": {"99"}, "Set-Cookie": {"secret"}}, Body: body.Bytes()}, nil
	}
	identifier := url.PathEscape("MiniMax-H3::task/9")
	request := sdk.ExecuteRequest{Protocol: "chat", Operation: sdk.OperationVideoGet, Options: raw(sdk.ExecuteOptions{Method: "GET", EscapedPath: "/v1/videos/" + identifier})}
	response, err := engine.execute(context.Background(), request)
	if err != nil || mediaResponse(test, response)["status"] != "completed" {
		test.Fatalf("query failed: %v", err)
	}
	request.Operation = sdk.OperationVideoContent
	request.Options = raw(sdk.ExecuteOptions{Method: "GET", EscapedPath: "/v1/videos/" + identifier + "/content"})
	response, err = engine.execute(context.Background(), request)
	if err != nil || string(response.Body) != "MP4" || len(response.Header) != 1 || response.Header.Get("Content-Type") != "video/mp4" {
		test.Fatalf("download corrupted: %#v %v", response, err)
	}
}

func TestMediaVideoNotReadyAndMalformedIdentityDoNotDownload(test *testing.T) {
	fixture, engine, _ := mediaFixture(test, "query_video_generation")
	fixture.respond = func(context.Context, string, map[string]any) (any, error) {
		return toolsSuccess(map[string]any{"status": "running"}), nil
	}
	request := sdk.ExecuteRequest{Protocol: "chat", Operation: sdk.OperationVideoContent, Options: raw(sdk.ExecuteOptions{Method: "GET", Path: "/v1/videos/MiniMax-H3::task/content"})}
	if _, err := engine.execute(context.Background(), request); err == nil || toolHTTPStatus(err) != 409 || len(fixture.calls) != 2 {
		test.Fatal("unfinished video downloaded or wrong refusal")
	}
	request.Options = raw(sdk.ExecuteOptions{Method: "GET", Path: "/v1/videos/not-a-minimax-id/content"})
	if _, err := engine.execute(context.Background(), request); err == nil || toolHTTPStatus(err) != 404 || len(fixture.calls) != 2 {
		test.Fatal("malformed ID reached vendor")
	}
}

func TestMediaUploadAuthorityLimitsAndLocalPathSchema(test *testing.T) {
	for _, upload := range []map[string]any{
		{"upload_host": "http://127.0.0.1/", "temp_url": "https://temp.example/x"},
		{"upload_host": "https://user:secret@upload.example/", "temp_url": "https://temp.example/x"},
		{"upload_host": "https://upload.example/", "temp_url": "http://temp.example/x"},
		{"upload_host": "https://upload.example/", "temp_url": "https://temp.example/x", "max_bytes": 1},
		{"temp_url": "https://temp.example/x"},
	} {
		fixture, engine, _ := mediaFixture(test, "upload_temp_url")
		fixture.respond = func(context.Context, string, map[string]any) (any, error) { return toolsSuccess(upload), nil }
		if _, err := engine.uploadImage(context.Background(), mediaFile{Filename: "x.png", MIME: "image/png", Bytes: []byte("xx")}); err == nil {
			test.Fatalf("invalid upload proceeded: %#v", upload)
		}
	}
	fixture, engine, _ := mediaFixture(test, "upload_temp_url")
	fixture.catalog = toolsCatalog(toolsEntry("upload_temp_url", `{"type":"object","properties":{"file_path":{"type":"string"}}}`))
	if _, err := engine.uploadImage(context.Background(), mediaFile{Filename: "x.png", MIME: "image/png", Bytes: []byte("xx")}); err == nil || len(fixture.calls) != 1 {
		test.Fatal("local-path uploader was invoked")
	}
}

func TestMediaShapeMappingsAndURLParser(test *testing.T) {
	for _, entry := range []struct {
		model, seconds string
		expected       int
	}{{"MiniMax-H3", "2", 4}, {"MiniMax-H3", "8", 8}, {"MiniMax-H3-Max", "3", 5}, {"MiniMax-Hailuo-2.3", "7", 6}, {"MiniMax-Hailuo-2.3", "11", 10}} {
		actual, err := mediaDuration(entry.seconds, entry.model)
		if err != nil || actual != entry.expected {
			test.Fatalf("duration %+v: %d %v", entry, actual, err)
		}
	}
	if mediaAspect("1280x720") != "16:9" || mediaArea(map[string]any{"size": "1024x1024"}) != "1K" || mediaArea(map[string]any{"size": "1024x1024", "quality": "high"}) != "4K" || mediaResolution("1920x1080", "MiniMax-H3-Max") != "768P" {
		test.Fatal("aspect/area/model clamp changed")
	}
	for _, status := range []string{"succeeded", "success", "completed", "done"} {
		actual, progress := videoState(map[string]any{"status": status})
		if actual != "completed" || progress != 100 {
			test.Fatal("completed status alias lost")
		}
	}
	reference, err := parseMediaReference("data:image/png,%89PNG%00")
	if err != nil || !strings.HasPrefix(string(reference.File.Bytes), "\x89PNG") {
		test.Fatal("valid percent-encoded data reference lost")
	}
}

func TestMediaUnsupportedLiveSchemaDoesNotSubmitGeneration(test *testing.T) {
	fixture, engine, _ := mediaFixture(test, "generate_image")
	fixture.catalog = toolsCatalog(toolsEntry("generate_image", `{"type":"object","$ref":"#/definitions/input"}`))
	_, err := engine.execute(context.Background(), mediaRequest(sdk.OperationImageGenerate, imageModel, map[string]any{"prompt": "fixture"}))
	if err == nil || len(fixture.calls) != 1 || fixture.calls[0].Path != matrixListPath {
		test.Fatal("native media submitted under an unenforceable schema")
	}
}
