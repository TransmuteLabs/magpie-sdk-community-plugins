package minimax

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

const maxMediaUpload = 64 << 20

type mediaFile struct {
	Field    string
	Filename string
	MIME     string
	Bytes    []byte
}

type mediaReference struct {
	URL  string
	MIME string
	File *mediaFile
}

type mediaInput struct {
	Fields map[string]any
	Files  []mediaFile
}

type mediaFailure struct {
	Status  int
	Message string
}

func (failure *mediaFailure) Error() string {
	return failure.Message
}
func (failure *mediaFailure) HTTPStatus() int {
	return failure.Status
}

func mediaInvalid(message string) error {
	return &mediaFailure{Status: http.StatusBadRequest, Message: message}
}

func readMediaInput(body []byte, headers http.Header) (mediaInput, error) {
	input := mediaInput{Fields: map[string]any{}}
	if len(body) > sdk.MaxNativeBodyBytes {
		return input, mediaInvalid("Media request exceeds the SDK body limit")
	}
	contentType := headers.Get("Content-Type")
	mediaType, parameters, parseError := mime.ParseMediaType(contentType)
	if strings.HasPrefix(strings.ToLower(contentType), "multipart/") {
		if parseError != nil || mediaType != "multipart/form-data" || parameters["boundary"] == "" {
			return input, mediaInvalid("Invalid multipart media request")
		}
		reader := multipart.NewReader(bytes.NewReader(body), parameters["boundary"])
		for count := 0; ; count++ {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil || count >= 128 {
				return input, mediaInvalid("Invalid or excessive multipart fields")
			}
			data, err := io.ReadAll(io.LimitReader(part, maxMediaUpload+1))
			part.Close()
			if err != nil || len(data) > maxMediaUpload {
				return input, mediaInvalid("Invalid multipart media bytes")
			}
			name := part.FormName()
			if name == "mask" {
				return input, mediaInvalid("MiniMax Image does not support mask edits")
			}
			if part.FileName() == "" {
				if _, exists := input.Fields[name]; !exists {
					input.Fields[name] = string(data)
				}
				continue
			}
			input.Files = append(input.Files, mediaFile{Field: name, Filename: part.FileName(), MIME: imageMIME(part.Header.Get("Content-Type"), data), Bytes: data})
		}
		return input, nil
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return input, nil
	}
	if json.Unmarshal(body, &input.Fields) != nil || input.Fields == nil {
		return input, mediaInvalid("Media request body must be a JSON object")
	}
	if input.Fields["mask"] != nil {
		return input, mediaInvalid("MiniMax Image does not support mask edits")
	}
	return input, nil
}

func imageMIME(hint string, data []byte) string {
	normalized := strings.ToLower(strings.TrimSpace(strings.Split(hint, ";")[0]))
	if normalized == "image/jpg" {
		normalized = "image/jpeg"
	}
	if normalized != "" && normalized != "application/octet-stream" {
		return normalized
	}
	if bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G'}) {
		return "image/png"
	}
	if bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}) {
		return "image/jpeg"
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	return "application/octet-stream"
}

func allowedImageMIME(value string) bool {
	return contains([]string{"image/jpeg", "image/png", "image/webp", "image/heic", "image/heif"}, value)
}

func imageFilename(contentType string) string {
	extension := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/heic": ".heic", "image/heif": ".heif"}[contentType]
	return "image" + extension
}

func mediaHTTPS(address string) bool {
	parsed, err := url.Parse(address)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && parsed.Fragment == ""
}

func parseMediaReference(value any) (*mediaReference, error) {
	if value == nil || value == "" {
		return nil, nil
	}
	address, isString := value.(string)
	mimeHint := ""
	if !isString {
		entry := object(value)
		address = firstMediaText(entry, "url", "image_url")
		mimeHint = firstMediaText(entry, "mime_type", "mimeType")
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, mediaInvalid("Reference images must be HTTPS or data: URLs")
	}
	reference := &mediaReference{URL: address}
	if normalized := imageMIME(mimeHint, nil); allowedImageMIME(normalized) {
		reference.MIME = normalized
	}
	if !strings.HasPrefix(address, "data:") {
		if !mediaHTTPS(address) {
			return nil, mediaInvalid("Reference images require a credentialless HTTPS URL or data: URL")
		}
		return reference, nil
	}
	metadata, payload, found := strings.Cut(strings.TrimPrefix(address, "data:"), ",")
	if !found {
		return nil, mediaInvalid("Malformed media data: URL")
	}
	var data []byte
	var err error
	if strings.Contains(strings.ToLower(";"+metadata)+";", ";base64;") {
		data, err = base64.StdEncoding.DecodeString(payload)
		if err != nil && !strings.Contains(payload, "=") {
			data, err = base64.RawStdEncoding.DecodeString(payload)
		}
	} else {
		var decoded string
		decoded, err = url.PathUnescape(payload)
		data = []byte(decoded)
	}
	if err != nil || len(data) == 0 || len(data) > maxMediaUpload {
		return nil, mediaInvalid("Invalid or oversized media data: URL")
	}
	contentType := imageMIME(metadata, data)
	if reference.MIME != "" {
		contentType = reference.MIME
	}
	reference.File = &mediaFile{Field: "image", Filename: imageFilename(contentType), MIME: contentType, Bytes: data}
	return reference, nil
}

func parseMediaList(value any) ([]*mediaReference, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok || len(items) > 16 {
		return nil, mediaInvalid("reference_images must be an array of at most 16 images")
	}
	references := make([]*mediaReference, 0, len(items))
	for _, item := range items {
		reference, err := parseMediaReference(item)
		if err != nil {
			return nil, err
		}
		if reference == nil {
			return nil, mediaInvalid("Reference image is empty")
		}
		references = append(references, reference)
	}
	return references, nil
}

func imageReferences(input mediaInput, edits bool) ([]*mediaReference, error) {
	references := []*mediaReference{}
	for _, file := range input.Files {
		if file.Field == "image" || file.Field == "image[]" {
			if len(file.Bytes) == 0 {
				return nil, mediaInvalid("Reference image is empty")
			}
			file.Filename = path.Base(strings.ReplaceAll(file.Filename, "\\", "/"))
			references = append(references, &mediaReference{File: &file})
		}
	}
	if len(references) == 0 {
		var err error
		references, err = parseMediaList(input.Fields["reference_images"])
		if err != nil {
			return nil, err
		}
	}
	if len(references) == 0 {
		value := input.Fields["image"]
		if value == nil {
			value = input.Fields["image_url"]
		}
		reference, err := parseMediaReference(value)
		if err != nil {
			return nil, err
		}
		if reference != nil {
			references = append(references, reference)
		}
	}
	if len(references) > 16 || edits && len(references) == 0 {
		return nil, mediaInvalid("Image edits need 1 to 16 reference images")
	}
	return references, nil
}
