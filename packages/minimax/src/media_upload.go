package minimax

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"sort"
	"strings"
	"time"
)

func (engine mediaEngine) resolveReference(ctx context.Context, reference *mediaReference) (map[string]any, error) {
	if reference == nil {
		return nil, nil
	}
	if reference.File != nil {
		return engine.uploadImage(ctx, *reference.File)
	}
	result := map[string]any{"url": reference.URL}
	if reference.MIME != "" {
		result["mime_type"] = reference.MIME
	}
	return result, nil
}

func (engine mediaEngine) resolveReferences(ctx context.Context, references []*mediaReference) ([]any, error) {
	results := make([]any, 0, len(references))
	for _, reference := range references {
		resolved, err := engine.resolveReference(ctx, reference)
		if err != nil {
			return nil, err
		}
		results = append(results, resolved)
	}
	return results, nil
}

func uploadArguments(schema map[string]any, file mediaFile) (map[string]any, bool, error) {
	properties := object(schema["properties"])
	if len(properties) == 0 {
		return map[string]any{"filename": file.Filename, "mime_type": file.MIME}, false, nil
	}
	arguments := map[string]any{}
	selectField := func(aliases []string, value any) bool {
		for _, alias := range aliases {
			if _, found := properties[alias]; found {
				arguments[alias] = value
				return true
			}
		}
		return false
	}
	selectField([]string{"filename", "file_name", "name", "fileName"}, file.Filename)
	selectField([]string{"mime_type", "mimeType", "content_type", "contentType", "mime"}, file.MIME)
	selectField([]string{"size_bytes", "size", "bytes", "content_length"}, float64(len(file.Bytes)))
	sentBytes := selectField([]string{"data", "content", "content_base64", "bytes_base64", "file_base64", "base64"}, base64.StdEncoding.EncodeToString(file.Bytes))
	for _, field := range []string{"file_path", "path", "local_path"} {
		if _, found := properties[field]; found && len(arguments) == 0 {
			return nil, false, toolError("This upload tool requires a local path, which cannot be supplied")
		}
	}
	if len(arguments) == 0 {
		arguments = map[string]any{"filename": file.Filename, "mime_type": file.MIME}
	}
	if err := validateToolArguments(schema, arguments); err != nil {
		return nil, false, err
	}
	return arguments, sentBytes, nil
}

func signedUploadResult(value any) (map[string]any, error) {
	if encoded, ok := value.(string); ok {
		if json.Unmarshal([]byte(encoded), &value) != nil {
			return nil, responseFailure(toolError("Malformed upload response"), ByteResponse{Body: []byte(encoded)}, "upload-envelope")
		}
	}
	result := object(value)
	if encoded, ok := result["result_json"].(string); ok {
		if json.Unmarshal([]byte(encoded), &result) != nil {
			return nil, responseFailure(toolError("Malformed nested upload response"), ByteResponse{Body: []byte(encoded)}, "upload-envelope")
		}
	}
	if result == nil {
		return nil, toolError("Upload tool returned no form")
	}
	if value, exists := result["code"]; exists {
		code, valid := number(value)
		if !valid || code != 0 {
			return nil, responseFailure(toolError("Upload tool reported a business failure"), ByteResponse{Body: raw(result)}, "upload-envelope")
		}
	}
	return result, nil
}

func (engine mediaEngine) uploadImage(ctx context.Context, file mediaFile) (map[string]any, error) {
	if len(file.Bytes) == 0 || len(file.Bytes) > maxMediaUpload {
		return nil, mediaInvalid("Reference image is empty or larger than 64 MiB")
	}
	catalog, err := engine.catalog(ctx)
	if err != nil {
		return nil, err
	}
	var entry matrixEntry
	for _, candidate := range catalog.Tools {
		if strings.HasSuffix(candidate.RuntimeTool, "__upload_temp_url") || candidate.ProviderTool == "upload_temp_url" || strings.HasSuffix(candidate.ProviderTool, "_upload_temp_url") {
			entry = candidate
			break
		}
	}
	if entry.RuntimeTool == "" {
		return nil, toolError("No upload_temp_url tool is connected on this account")
	}
	schema := map[string]any{"type": "object"}
	if entry.InputSchema != "" {
		schema, err = parseToolSchema(entry.InputSchema)
		if err != nil {
			return nil, err
		}
	}
	arguments, sentBytes, err := uploadArguments(schema, file)
	if err != nil {
		return nil, err
	}
	value, err := engine.invoke(ctx, entry, arguments, 55*time.Second)
	if err != nil {
		return nil, err
	}
	form, err := signedUploadResult(value)
	if err != nil {
		return nil, err
	}
	address := firstMediaText(form, "temp_url", "tempUrl", "url")
	if !mediaHTTPS(address) {
		return nil, toolError("Upload tool returned no valid HTTPS temp_url")
	}
	uploadHost := firstMediaText(form, "upload_host", "uploadHost", "upload_url", "uploadUrl")
	maximum := form["max_bytes"]
	if maximum == nil {
		maximum = form["maxBytes"]
	}
	if maximum != nil {
		limit, valid := number(maximum)
		if !valid || limit < float64(len(file.Bytes)) {
			return nil, toolError("Reference image exceeds the signed upload limit")
		}
	}
	if uploadHost != "" {
		if !mediaHTTPS(uploadHost) {
			return nil, toolError("Upload tool returned an upload host that is not HTTPS")
		}
		if err := engine.postImage(ctx, uploadHost, form, file); err != nil {
			return nil, err
		}
	} else if !sentBytes {
		return nil, toolError("Upload tool returned no upload_host without receiving image bytes")
	}
	result := map[string]any{"url": address}
	if allowedImageMIME(file.MIME) {
		result["mime_type"] = file.MIME
	}
	return result, nil
}

func (engine mediaEngine) postImage(ctx context.Context, address string, form map[string]any, file mediaFile) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := object(form["fields"])
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if value, ok := fields[key].(string); ok {
			if err := writer.WriteField(key, value); err != nil {
				return err
			}
		}
	}
	field := firstMediaText(form, "file_field_name", "fileFieldName")
	if field == "" {
		field = "file"
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": field, "filename": file.Filename}))
	header.Set("Content-Type", file.MIME)
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	if _, err := part.Write(file.Bytes); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 55*time.Second)
	defer cancel()
	response, err := engine.client.Transfer(ctx, http.MethodPost, address, body.Bytes(), http.Header{"Content-Type": {writer.FormDataContentType()}}, 1<<20)
	if err != nil {
		return err
	}
	if response.Status < 200 || response.Status >= 300 {
		return transferFailure(toolError(fmt.Sprintf("Media upload failed (HTTP %d)", response.Status)), response)
	}
	return nil
}
