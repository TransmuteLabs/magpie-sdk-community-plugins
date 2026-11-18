package devin

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	sdk "github.com/yetone/magpie/pkg/protocolext"
)

// CatalogKind selects a native unary model discovery method.
type CatalogKind string

const (
	CliModels         CatalogKind = "cli-models"
	CommandModels     CatalogKind = "command-models"
	CascadeModels     CatalogKind = "cascade-models"
	CliModelsSite     CatalogKind = "cli-models-site"
	CommandModelsSite CatalogKind = "command-models-site"
	CascadeModelsSite CatalogKind = "cascade-models-site"
	ModelStatuses     CatalogKind = "model-statuses"

	CliModelsPath         = "/exa.api_server_pb.ApiServerService/GetCliModelConfigs"
	CommandModelsPath     = "/exa.api_server_pb.ApiServerService/GetCommandModelConfigs"
	CascadeModelsPath     = "/exa.api_server_pb.ApiServerService/GetCascadeModelConfigs"
	CliModelsSitePath     = "/exa.api_server_pb.ApiServerService/GetCliModelConfigsForSite"
	CommandModelsSitePath = "/exa.api_server_pb.ApiServerService/GetCommandModelConfigsForSite"
	CascadeModelsSitePath = "/exa.api_server_pb.ApiServerService/GetCascadeModelConfigsForSite"
	ModelStatusesPath     = "/exa.api_server_pb.ApiServerService/GetModelStatuses"
)

// CatalogAdapter translates the authenticated host's Models operation only.
type CatalogAdapter struct {
	Kind CatalogKind
}

var _ sdk.Inbound = CatalogAdapter{}

func (a CatalogAdapter) ID() string {
	if a.Path() == "" {
		return ""
	}
	return "devin." + string(a.Kind)
}

func (a CatalogAdapter) Path() string {
	switch a.Kind {
	case CliModels:
		return CliModelsPath
	case CommandModels:
		return CommandModelsPath
	case CascadeModels:
		return CascadeModelsPath
	case CliModelsSite:
		return CliModelsSitePath
	case CommandModelsSite:
		return CommandModelsSitePath
	case CascadeModelsSite:
		return CascadeModelsSitePath
	case ModelStatuses:
		return ModelStatusesPath
	default:
		return ""
	}
}

func (CatalogAdapter) Contract() sdk.Contract { return sdk.CatalogContract() }

func (a CatalogAdapter) Decode(ctx context.Context, header http.Header, body []byte) (sdk.Request, error) {
	if err := ctx.Err(); err != nil {
		return sdk.Request{}, err
	}
	if a.Path() == "" {
		return sdk.Request{}, &sdk.Error{Status: 500, Message: "unsupported Devin catalog kind"}
	}
	contentTypes := header.Values("Content-Type")
	if len(contentTypes) != 1 {
		return sdk.Request{}, &sdk.Error{Status: 415, Message: "expected application/proto"}
	}
	mediaType, _, err := mime.ParseMediaType(contentTypes[0])
	if err != nil || mediaType != "application/proto" {
		return sdk.Request{}, &sdk.Error{Status: 415, Message: "expected application/proto"}
	}
	versions := header.Values("Connect-Protocol-Version")
	if len(versions) != 1 || versions[0] != "1" {
		return sdk.Request{}, &sdk.Error{Status: 400, Message: "expected Connect-Protocol-Version 1"}
	}
	for _, name := range []string{"Content-Encoding", "Connect-Content-Encoding"} {
		values := header.Values(name)
		if len(values) > 1 || (len(values) == 1 && values[0] != "" && !strings.EqualFold(values[0], "identity")) {
			return sdk.Request{}, &sdk.Error{Status: 415, Message: "unsupported unary catalog compression"}
		}
	}
	if len(body) > maxBody {
		return sdk.Request{}, &sdk.Error{Status: 413, Message: "Devin catalog request exceeds limit"}
	}
	timeout, err := decodeTimeout(header)
	if err != nil {
		return sdk.Request{}, err
	}
	schema := map[int]int{1: 2}
	team := 0
	switch a.Kind {
	case CliModelsSite:
		schema, team = map[int]int{3: textField}, 3
	case CommandModelsSite:
		schema, team = map[int]int{2: textField}, 2
	case CascadeModelsSite:
		schema, team = map[int]int{1: textField, 2: textField, 3: textField, 4: textField, 7: textField}, 7
	}
	request, err := readMessage(body, schema)
	if err != nil {
		return sdk.Request{}, &sdk.Error{Status: 400, Message: "malformed or unsupported Devin catalog request"}
	}
	if team != 0 {
		if request.text(team) != "" {
			return sdk.Request{}, &sdk.Error{Status: 400, Message: "native team-specific model discovery is unsupported"}
		}
	} else {
		if _, err := readMessage(request.data(1), nil); err != nil {
			return sdk.Request{}, &sdk.Error{Status: 400, Message: "invalid Devin catalog metadata envelope"}
		}
	}
	return sdk.Request{Protocol: sdk.Models, Body: nil, Timeout: timeout}, nil
}

type catalogResponse struct {
	writer   http.ResponseWriter
	header   http.Header
	kind     CatalogKind
	status   int
	body     []byte
	finished bool
	err      error
	code     string
	failure  int
}

var _ sdk.Response = (*catalogResponse)(nil)

func (a CatalogAdapter) Reply(writer http.ResponseWriter) sdk.Response {
	return &catalogResponse{writer: writer, header: make(http.Header), kind: a.Kind}
}

func (r *catalogResponse) Header() http.Header { return r.header }

func (r *catalogResponse) WriteHeader(status int) {
	if !r.finished && r.status == 0 && status >= 200 {
		r.status = status
	}
}

func (r *catalogResponse) fail(status int, code, message string) error {
	if r.err == nil {
		r.err, r.failure, r.code = errors.New(message), status, code
	}
	return r.err
}

func (r *catalogResponse) Write(body []byte) (int, error) {
	if r.finished {
		return 0, io.ErrClosedPipe
	}
	if r.err != nil {
		return 0, r.err
	}
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if len(body) > maxBody-len(r.body) {
		return 0, r.fail(413, "resource_exhausted", "canonical model catalog exceeds limit")
	}
	r.body = append(r.body, body...)
	return len(body), nil
}

func (r *catalogResponse) Finish(cause error) error {
	if r.finished {
		return r.err
	}
	r.finished = true
	if r.err == nil && r.status >= 400 && r.status <= 599 {
		r.fail(r.status, catalogErrorCode(r.status), "gateway model discovery failed")
	}
	if r.err == nil && cause != nil {
		switch {
		case errors.Is(cause, context.DeadlineExceeded):
			r.fail(504, "deadline_exceeded", "model discovery deadline exceeded")
		case errors.Is(cause, context.Canceled):
			r.fail(499, "canceled", "model discovery canceled")
		default:
			r.fail(500, "internal", "model discovery failed")
		}
	}
	var native []byte
	if r.err == nil {
		contentTypes := r.header.Values("Content-Type")
		mediaType, _, err := mime.ParseMediaType(r.header.Get("Content-Type"))
		encoding := r.header.Values("Content-Encoding")
		if r.status != http.StatusOK || len(contentTypes) != 1 || err != nil || mediaType != "application/json" ||
			len(encoding) > 1 || (len(encoding) == 1 && encoding[0] != "" && !strings.EqualFold(encoding[0], "identity")) {
			r.fail(502, "internal", "expected canonical model catalog JSON")
		} else if (CatalogAdapter{Kind: r.kind}).Path() == "" {
			r.fail(500, "internal", "unsupported Devin catalog kind")
		} else {
			native, err = encodeCatalog(r.body, r.kind)
			if err != nil {
				var public *sdk.Error
				if errors.As(err, &public) {
					r.fail(public.Status, catalogErrorCode(public.Status), public.Message)
				} else {
					r.fail(502, "data_loss", err.Error())
				}
			}
		}
	}
	r.body = nil
	header := r.writer.Header()
	for _, name := range []string{"Content-Length", "Content-Encoding", "Connect-Content-Encoding", "Transfer-Encoding", "Trailer"} {
		header.Del(name)
	}
	header.Set("Connect-Protocol-Version", "1")
	header.Set("Cache-Control", "no-store")
	for _, name := range []string{"Retry-After", "X-Magpie-Session"} {
		if values := r.header.Values(name); len(values) > 0 {
			header[name] = append([]string(nil), values...)
		}
	}
	status := http.StatusOK
	header.Set("Content-Type", "application/proto")
	if r.err != nil {
		status = r.failure
		header.Set("Content-Type", "application/json")
		native, _ = json.Marshal(struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{r.code, r.err.Error()})
	}
	r.writer.WriteHeader(status)
	written, err := r.writer.Write(native)
	if err != nil || written != len(native) {
		r.err = errors.New("native catalog response write failed")
	}
	return r.err
}

func catalogErrorCode(status int) string {
	switch status {
	case 400, 415, 422:
		return "invalid_argument"
	case 401:
		return "unauthenticated"
	case 403:
		return "permission_denied"
	case 404, 405, 501:
		return "unimplemented"
	case 408, 504:
		return "deadline_exceeded"
	case 409:
		return "aborted"
	case 413, 429:
		return "resource_exhausted"
	case 499:
		return "canceled"
	case 502, 503:
		return "unavailable"
	default:
		return "internal"
	}
}

func catalogObject(body []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("invalid canonical model catalog object")
	}
	object := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, errors.New("invalid canonical model catalog object")
		}
		name, ok := token.(string)
		if !ok || object[name] != nil {
			return nil, errors.New("duplicate or invalid canonical model catalog field")
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, errors.New("invalid canonical model catalog field")
		}
		object[name] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, errors.New("invalid canonical model catalog object")
	}
	if decoder.Decode(new(json.RawMessage)) != io.EOF {
		return nil, errors.New("trailing canonical model catalog data")
	}
	return object, nil
}

func catalogString(object map[string]json.RawMessage, name string) (string, error) {
	value, ok := object[name]
	if !ok {
		return "", nil
	}
	var text string
	if bytes.Equal(value, []byte("null")) || json.Unmarshal(value, &text) != nil {
		return "", errors.New("invalid canonical model catalog string")
	}
	return text, nil
}

func catalogTokens(object map[string]json.RawMessage, names ...string) (int32, error) {
	var tokens int32
	found := false
	for _, name := range names {
		value, ok := object[name]
		if !ok {
			continue
		}
		var count int32
		if bytes.Equal(value, []byte("null")) || json.Unmarshal(value, &count) != nil || count < 0 || (found && tokens != count) {
			return 0, errors.New("invalid or inconsistent canonical model token limit")
		}
		tokens, found = count, true
	}
	return tokens, nil
}

func catalogImages(value json.RawMessage) (bool, error) {
	if value == nil {
		return false, nil
	}
	modalities, err := catalogObject(value)
	if err != nil {
		return false, errors.New("invalid canonical model modalities")
	}
	images := false
	for _, name := range []string{"input", "output"} {
		value, ok := modalities[name]
		if !ok {
			continue
		}
		var kinds []json.RawMessage
		if json.Unmarshal(value, &kinds) != nil || kinds == nil {
			return false, errors.New("invalid canonical model modalities")
		}
		for _, kind := range kinds {
			var text string
			if bytes.Equal(kind, []byte("null")) || json.Unmarshal(kind, &text) != nil || text == "" {
				return false, errors.New("invalid canonical model modality")
			}
			images = images || (name == "input" && text == "image")
		}
	}
	return images, nil
}

func encodeCatalog(body []byte, kind CatalogKind) ([]byte, error) {
	if !utf8.Valid(body) {
		return nil, errors.New("invalid canonical model catalog UTF-8")
	}
	root, err := catalogObject(body)
	if err != nil {
		return nil, err
	}
	object, err := catalogString(root, "object")
	if err != nil || object != "list" {
		return nil, errors.New("expected canonical model list")
	}
	if value, ok := root["error"]; ok && !bytes.Equal(value, []byte("null")) {
		return nil, errors.New("canonical model list contains an error")
	}
	if value, ok := root["has_more"]; ok {
		var more bool
		if bytes.Equal(value, []byte("null")) || json.Unmarshal(value, &more) != nil || more {
			return nil, errors.New("expected a complete canonical model list")
		}
	}
	var rows []json.RawMessage
	if json.Unmarshal(root["data"], &rows) != nil || rows == nil {
		return nil, errors.New("expected canonical model list data array")
	}
	var native []byte
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		model, err := catalogObject(row)
		if err != nil {
			return nil, err
		}
		object, err := catalogString(model, "object")
		if err != nil || object != "model" {
			return nil, errors.New("expected canonical model object")
		}
		id, err := catalogString(model, "id")
		if err != nil || id == "" || strings.TrimSpace(id) != id || strings.ContainsFunc(id, unicode.IsControl) || seen[id] {
			return nil, errors.New("invalid or duplicate canonical model id")
		}
		seen[id] = true
		label, err := catalogString(model, "display_name")
		if err != nil {
			return nil, err
		}
		if label == "" {
			label = id
		}
		description, err := catalogString(model, "description")
		if err != nil {
			return nil, err
		}
		window, err := catalogTokens(model, "context_window", "context_length", "max_input_tokens")
		if err != nil {
			return nil, err
		}
		output, err := catalogTokens(model, "max_output_tokens")
		if err != nil || (window > 0 && output > window) {
			return nil, errors.New("invalid canonical model output limit")
		}
		images, err := catalogImages(model["modalities"])
		if err != nil {
			return nil, err
		}
		if kind == ModelStatuses {
			continue
		}
		config := addText(nil, 1, label)
		config = addNumber(config, 4, 0)
		supportsImages := uint64(0)
		if images {
			supportsImages = 1
		}
		config = addNumber(config, 5, supportsImages)
		if window > 0 {
			config = addNumber(config, 18, uint64(window))
		}
		config = addText(config, 22, id)
		var info []byte
		if window > 0 {
			info = addNumber(info, 4, uint64(window))
		}
		if output > 0 {
			info = addNumber(info, 13, uint64(output))
		}
		info = addNumber(info, 25, 0)
		config = addBytes(config, 23, info)
		config = addText(config, 27, description)
		size := 1 + len(binary.AppendUvarint(nil, uint64(len(config)))) + len(config)
		if size > maxBody-len(native) {
			return nil, &sdk.Error{Status: 413, Message: "native model catalog exceeds limit"}
		}
		native = addBytes(native, 1, config)
	}
	return native, nil
}
