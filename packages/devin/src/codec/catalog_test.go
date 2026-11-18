package devin

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	sdk "github.com/yetone/magpie/pkg/protocolext"
)

const catalogFixture = `{"object":"list","data":[{"id":"group/ok","object":"model","type":"model","display_name":"Label","owned_by":"test","context_window":128,"context_length":128,"max_input_tokens":128,"max_output_tokens":32,"modalities":{"input":["text","image"],"output":["text"]},"description":"note","pricing":{"prompt":"1"},"reasoning":true,"supported_parameters":["tools"]}],"has_more":false,"first_id":"group/ok","last_id":"group/ok"}`

func TestCatalogNativeDeadline(t *testing.T) {
	header := catalogHeaders()
	header.Set("Connect-Timeout-Ms", "1500")
	request, err := (CatalogAdapter{Kind: CliModels}).Decode(context.Background(), header, nil)
	if err != nil || request.Timeout != 1500*time.Millisecond {
		t.Fatalf("catalog timeout lost: %+v %v", request, err)
	}
	header.Set("Connect-Timeout-Ms", "0")
	if _, err := (CatalogAdapter{Kind: CliModels}).Decode(context.Background(), header, nil); err == nil {
		t.Fatal("already expired native catalog request was admitted")
	}
}

func catalogHeaders() http.Header {
	return http.Header{"Content-Type": {"application/proto"}, "Connect-Protocol-Version": {"1"}}
}

func catalogError(t *testing.T, writer *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	if writer.Code == http.StatusOK || writer.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected non-200 unary JSON error, got status %d and headers %v", writer.Code, writer.Header())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(writer.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected unframed unary error JSON: %v", err)
	}
	if len(body) != 2 || body["error"] != nil {
		t.Fatalf("expected exactly code and message, got %s", writer.Body)
	}
	var code, message string
	if json.Unmarshal(body["code"], &code) != nil || json.Unmarshal(body["message"], &message) != nil || code == "" || message == "" {
		t.Fatal("missing Connect unary error code or message")
	}
	return code, message
}

func TestCatalogAdapterContract(t *testing.T) {
	adapters := []struct {
		kind CatalogKind
		id   string
		path string
	}{
		{CliModels, "devin.cli-models", "/exa.api_server_pb.ApiServerService/GetCliModelConfigs"},
		{CommandModels, "devin.command-models", "/exa.api_server_pb.ApiServerService/GetCommandModelConfigs"},
		{ModelStatuses, "devin.model-statuses", "/exa.api_server_pb.ApiServerService/GetModelStatuses"},
	}
	inbounds := make([]sdk.Inbound, 0, len(adapters))
	for _, expected := range adapters {
		adapter := CatalogAdapter{Kind: expected.kind}
		if adapter.ID() != expected.id || adapter.Path() != expected.path {
			t.Fatalf("wrong catalog adapter identity: %q %q", adapter.ID(), adapter.Path())
		}
		contract := adapter.Contract()
		if contract.Version.Major != 0 || contract.Version.Minor != 2 ||
			!contract.Has(sdk.InboundCodec) || !contract.Has(sdk.ModelDiscovery) || len(contract.Capabilities) != 2 {
			t.Fatalf("wrong discovery contract: %+v", contract)
		}
		if err := sdk.HostContract().Check(contract); err != nil {
			t.Fatal(err)
		}
		if sdk.InboundContract().Check(contract) == nil {
			t.Fatal("catalog adapter accepted a host without model discovery")
		}
		inbounds = append(inbounds, adapter)
	}
	registry, err := sdk.New(inbounds...)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range adapters {
		inbound, ok := registry.Lookup(expected.path)
		if !ok || inbound.ID() != expected.id {
			t.Fatalf("missing exact catalog route %s", expected.path)
		}
		if _, ok := registry.Lookup(expected.path + "/"); ok {
			t.Fatal("catalog route accepted a subpath")
		}
	}
	if (CatalogAdapter{}).ID() != "" || (CatalogAdapter{}).Path() != "" {
		t.Fatal("unknown catalog kind acquired a route")
	}
}

func TestCatalogDecodeDiscardsMetadata(t *testing.T) {
	body := fixtureText(1, string(fixtureText(3, "native-credential-sentinel")))
	header := catalogHeaders()
	header.Set("Authorization", "Bearer caller-credential-sentinel")
	header.Set("X-Api-Key", "caller-api-key-sentinel")
	beforeHeader, beforeBody := header.Clone(), bytes.Clone(body)
	for _, kind := range []CatalogKind{CliModels, CommandModels, ModelStatuses} {
		for _, request := range [][]byte{nil, body} {
			decoded, err := (CatalogAdapter{Kind: kind}).Decode(context.Background(), header, request)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Protocol != sdk.Models || decoded.Body != nil || decoded.Session != "" || decoded.Timeout != 0 {
				t.Fatalf("discovery must carry only the Models operation: %+v", decoded)
			}
			if !reflect.DeepEqual(header, beforeHeader) || !bytes.Equal(body, beforeBody) {
				t.Fatal("catalog decode mutated host credentials or native metadata")
			}
		}
	}
}

func TestCatalogDecodeRefusals(t *testing.T) {
	for name, request := range map[string][]byte{
		"unknown semantic field": fixtureText(2, "native-secret"),
		"wrong metadata wire":    fixtureNumber(1, 1),
		"duplicate metadata":     join(fixtureText(1, ""), fixtureText(1, "")),
		"truncated metadata":     {0x0a, 2, 0x1a},
		"invalid metadata tag":   {0x0a, 1, 0},
		"invalid nested length":  {0x0a, 2, 0x1a, 0x7f},
		"invalid nested varint":  {0x0a, 2, 0x08, 0x80},
		"framed unary request":   fixtureFrame(0, nil),
	} {
		t.Run(name, func(t *testing.T) {
			for _, kind := range []CatalogKind{CliModels, CommandModels, ModelStatuses} {
				_, err := (CatalogAdapter{Kind: kind}).Decode(context.Background(), catalogHeaders(), request)
				var public *sdk.Error
				if !errors.As(err, &public) || public.Status != 400 || strings.Contains(public.Message, "native-secret") {
					t.Fatalf("expected safe malformed-request refusal, got %v", err)
				}
			}
		})
	}
	for _, test := range []struct {
		name   string
		header string
		values []string
		status int
	}{
		{"missing mime", "Content-Type", nil, 415},
		{"streaming mime", "Content-Type", []string{"application/connect+proto"}, 415},
		{"json mime", "Content-Type", []string{"application/json"}, 415},
		{"invalid mime", "Content-Type", []string{"application/proto; charset"}, 415},
		{"duplicate mime", "Content-Type", []string{"application/proto", "application/json"}, 415},
		{"missing version", "Connect-Protocol-Version", nil, 400},
		{"wrong version", "Connect-Protocol-Version", []string{"2"}, 400},
		{"duplicate version", "Connect-Protocol-Version", []string{"1", "1"}, 400},
		{"http gzip", "Content-Encoding", []string{"gzip"}, 415},
		{"connect gzip", "Connect-Content-Encoding", []string{"gzip"}, 415},
		{"duplicate encoding", "Content-Encoding", []string{"identity", "identity"}, 415},
	} {
		t.Run(test.name, func(t *testing.T) {
			header := catalogHeaders()
			header[test.header] = test.values
			_, err := (CatalogAdapter{Kind: CliModels}).Decode(context.Background(), header, nil)
			var public *sdk.Error
			if !errors.As(err, &public) || public.Status != test.status {
				t.Fatalf("expected status %d, got %v", test.status, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (CatalogAdapter{Kind: CliModels}).Decode(ctx, catalogHeaders(), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("catalog decode ignored cancellation: %v", err)
	}
	if _, err := (CatalogAdapter{}).Decode(context.Background(), catalogHeaders(), nil); err == nil {
		t.Fatal("unknown catalog kind decoded")
	}
}

func TestCatalogReplyRecoveredDescriptorGolden(t *testing.T) {
	expected, err := hex.DecodeString("0a2c0a054c6162656c2000280190018001b2010867726f75702f6f6bba01082080016820c80100da01046e6f7465")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []CatalogKind{CliModels, CommandModels} {
		t.Run(string(kind), func(t *testing.T) {
			writer := httptest.NewRecorder()
			reply := (CatalogAdapter{Kind: kind}).Reply(writer)
			reply.Header().Set("Content-Type", "application/json; charset=utf-8")
			if _, streaming := reply.(http.Flusher); streaming {
				t.Fatal("unary catalog advertises streaming flush")
			}
			for _, value := range []byte(catalogFixture) {
				if written, err := reply.Write([]byte{value}); written != 1 || err != nil {
					t.Fatalf("fragmented JSON write: %d %v", written, err)
				}
				if writer.Body.Len() != 0 {
					t.Fatal("catalog emitted before whole-list validation")
				}
			}
			if err := reply.Finish(nil); err != nil {
				t.Fatal(err)
			}
			if writer.Code != 200 || writer.Header().Get("Content-Type") != "application/proto" ||
				writer.Header().Get("Connect-Protocol-Version") != "1" || !bytes.Equal(writer.Body.Bytes(), expected) {
				t.Fatalf("catalog differs from recovered ClientModelConfig/ModelInfo fields: status %d, bytes %x", writer.Code, writer.Body.Bytes())
			}
			before := bytes.Clone(writer.Body.Bytes())
			if err := reply.Finish(context.Canceled); err != nil || !bytes.Equal(writer.Body.Bytes(), before) {
				t.Fatal("catalog finish was not idempotent")
			}
			if written, err := reply.Write([]byte("more")); written != 0 || !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("catalog accepted a write after finish: %d %v", written, err)
			}
		})
	}
}

func TestCatalogReplyKnownFieldsOnly(t *testing.T) {
	writer := httptest.NewRecorder()
	reply := (CatalogAdapter{Kind: CliModels}).Reply(writer)
	reply.Header().Set("Content-Type", "application/json")
	body := `{"object":"list","data":[{"id":"custom/one","object":"model"},{"id":"custom/two","object":"model","context_length":2147483647,"max_output_tokens":2147483647,"modalities":{"input":["text"],"output":["image"]}}]}`
	if _, err := io.WriteString(reply, body); err != nil {
		t.Fatal(err)
	}
	if err := reply.Finish(nil); err != nil {
		t.Fatal(err)
	}
	root, err := readMessage(writer.Body.Bytes(), map[int]int{1: 2 | repeated})
	if err != nil || len(root[1]) != 2 {
		t.Fatalf("expected two native model configs: %v", err)
	}
	for index, item := range root[1] {
		config, err := readMessage(item.b, map[int]int{1: textField, 4: 0, 5: 0, 18: 0, 22: textField, 23: 2, 27: textField})
		if err != nil {
			t.Fatal(err)
		}
		id := []string{"custom/one", "custom/two"}[index]
		if config.text(1) != id || config.text(22) != id || config.number(4) != 0 || config.number(5) != 0 {
			t.Fatal("catalog lost its explicit ID or fabricated disabled/image capabilities")
		}
		info, err := readMessage(config.data(23), map[int]int{4: 0, 13: 0, 25: 0})
		if err != nil || len(info[25]) != 1 || info.number(25) != 0 {
			t.Fatalf("catalog fabricated family, routing or vendor metadata: %v", err)
		}
		if index == 0 && (len(config[18]) != 0 || len(info[4]) != 0 || len(info[13]) != 0) {
			t.Fatal("unknown token limits became fabricated values")
		}
		if index == 1 && (config.number(18) != 2147483647 || info.number(4) != 2147483647 || info.number(13) != 2147483647) {
			t.Fatal("maximum int32 token limits did not survive encoding")
		}
	}
}

func TestCatalogReplyEmptyConfigsAndStatuses(t *testing.T) {
	for _, kind := range []CatalogKind{CliModels, CommandModels, ModelStatuses} {
		bodies := []string{`{"object":"list","data":[]}`}
		if kind == ModelStatuses {
			bodies = append(bodies, catalogFixture)
		}
		for _, body := range bodies {
			writer := httptest.NewRecorder()
			reply := (CatalogAdapter{Kind: kind}).Reply(writer)
			reply.Header().Set("Content-Type", "application/json")
			if _, err := io.WriteString(reply, body); err != nil {
				t.Fatal(err)
			}
			if err := reply.Finish(nil); err != nil {
				t.Fatal(err)
			}
			if writer.Code != 200 || writer.Header().Get("Content-Type") != "application/proto" || writer.Body.Len() != 0 {
				t.Fatal("empty catalog/statuses must be empty protobuf, with no invented health or frame")
			}
		}
	}
}

func TestCatalogReplyRejectsMalformedListsAtomically(t *testing.T) {
	const good = `{"id":"allowed/model","object":"model"}`
	badModels := map[string]string{
		"null row":                  `null`,
		"array row":                 `[]`,
		"missing model object":      `{"id":"bad"}`,
		"wrong model object":        `{"id":"bad","object":"error"}`,
		"missing id":                `{"object":"model"}`,
		"null id":                   `{"id":null,"object":"model"}`,
		"numeric id":                `{"id":2,"object":"model"}`,
		"empty id":                  `{"id":"","object":"model"}`,
		"spaced id":                 `{"id":" bad","object":"model"}`,
		"control id":                `{"id":"bad\nid","object":"model"}`,
		"duplicate id":              `{"id":"allowed/model","object":"model"}`,
		"duplicate row field":       `{"id":"bad","id":"other","object":"model"}`,
		"null label":                `{"id":"bad","object":"model","display_name":null}`,
		"numeric description":       `{"id":"bad","object":"model","description":2}`,
		"negative window":           `{"id":"bad","object":"model","context_window":-1}`,
		"overflow window":           `{"id":"bad","object":"model","context_window":2147483648}`,
		"fractional window":         `{"id":"bad","object":"model","context_window":1.5}`,
		"null window":               `{"id":"bad","object":"model","context_window":null}`,
		"string window":             `{"id":"bad","object":"model","context_window":"128"}`,
		"inconsistent windows":      `{"id":"bad","object":"model","context_window":128,"context_length":64}`,
		"inconsistent input window": `{"id":"bad","object":"model","context_window":128,"max_input_tokens":64}`,
		"negative output":           `{"id":"bad","object":"model","max_output_tokens":-1}`,
		"overflow output":           `{"id":"bad","object":"model","max_output_tokens":2147483648}`,
		"fractional output":         `{"id":"bad","object":"model","max_output_tokens":1.5}`,
		"null output":               `{"id":"bad","object":"model","max_output_tokens":null}`,
		"output over window":        `{"id":"bad","object":"model","context_window":32,"max_output_tokens":64}`,
		"null modalities":           `{"id":"bad","object":"model","modalities":null}`,
		"array modalities":          `{"id":"bad","object":"model","modalities":[]}`,
		"null input":                `{"id":"bad","object":"model","modalities":{"input":null}}`,
		"string input":              `{"id":"bad","object":"model","modalities":{"input":"image"}}`,
		"null modality":             `{"id":"bad","object":"model","modalities":{"input":[null]}}`,
		"numeric modality":          `{"id":"bad","object":"model","modalities":{"input":[2]}}`,
		"invalid output modalities": `{"id":"bad","object":"model","modalities":{"output":2}}`,
	}
	badBodies := map[string]string{
		"empty":                "",
		"null root":            `null`,
		"array root":           `[]`,
		"missing list object":  `{"data":[]}`,
		"wrong list object":    `{"object":"model","data":[]}`,
		"missing data":         `{"object":"list"}`,
		"null data":            `{"object":"list","data":null}`,
		"object data":          `{"object":"list","data":{}}`,
		"duplicate data field": `{"object":"list","data":[],"data":[]}`,
		"partial list":         `{"object":"list","data":[],"has_more":true}`,
		"null page flag":       `{"object":"list","data":[],"has_more":null}`,
		"string page flag":     `{"object":"list","data":[],"has_more":"false"}`,
		"embedded list error":  `{"object":"list","data":[],"error":{"message":"private-cause"}}`,
		"trailing data":        `{"object":"list","data":[]} {}`,
		"truncated JSON":       `{"object":"list","data":[` + good,
		"invalid UTF-8":        "{\"object\":\"list\",\"data\":[{\"id\":\"bad\xff\",\"object\":\"model\"}]}",
	}
	for name, bad := range badModels {
		badBodies[name] = `{"object":"list","data":[` + good + `,` + bad + `]}`
	}
	for name, body := range badBodies {
		t.Run(name, func(t *testing.T) {
			for _, kind := range []CatalogKind{CliModels, CommandModels, ModelStatuses} {
				writer := httptest.NewRecorder()
				reply := (CatalogAdapter{Kind: kind}).Reply(writer)
				reply.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(reply, body); err != nil {
					t.Fatal(err)
				}
				if writer.Body.Len() != 0 {
					t.Fatal("partial model catalog escaped before validation")
				}
				if err := reply.Finish(nil); err == nil {
					t.Fatalf("%s accepted malformed list", kind)
				}
				code, _ := catalogError(t, writer)
				if code != "data_loss" {
					t.Fatalf("expected data_loss, got %q", code)
				}
			}
		})
	}
}

func TestCatalogBodyLimits(t *testing.T) {
	t.Run("raw protobuf exact boundary", func(t *testing.T) {
		body := fixtureText(1, string(fixtureText(3, strings.Repeat("x", maxBody-10))))
		if len(body) != maxBody {
			t.Fatalf("wrong boundary fixture length: %d", len(body))
		}
		if _, err := (CatalogAdapter{Kind: CliModels}).Decode(context.Background(), catalogHeaders(), body); err != nil {
			t.Fatal(err)
		}
		body = append(body, 0)
		_, err := (CatalogAdapter{Kind: CliModels}).Decode(context.Background(), catalogHeaders(), body)
		var public *sdk.Error
		if !errors.As(err, &public) || public.Status != 413 {
			t.Fatalf("raw protobuf limit was not enforced: %v", err)
		}
	})
	t.Run("JSON exact boundary", func(t *testing.T) {
		writer := httptest.NewRecorder()
		reply := (CatalogAdapter{Kind: CliModels}).Reply(writer)
		reply.Header().Set("Content-Type", "application/json")
		body := catalogFixture + strings.Repeat(" ", maxBody-len(catalogFixture))
		if _, err := io.WriteString(reply, body); err != nil {
			t.Fatal(err)
		}
		if err := reply.Finish(nil); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("JSON overflow across writes", func(t *testing.T) {
		writer := httptest.NewRecorder()
		reply := (CatalogAdapter{Kind: CliModels}).Reply(writer)
		reply.Header().Set("Content-Type", "application/json")
		if _, err := reply.Write(bytes.Repeat([]byte{' '}, maxBody)); err != nil {
			t.Fatal(err)
		}
		if written, err := reply.Write([]byte{' '}); written != 0 || err == nil {
			t.Fatal("canonical JSON exceeded its bounded buffer")
		}
		if writer.Body.Len() != 0 {
			t.Fatal("overflowing catalog leaked a partial reply")
		}
		if err := reply.Finish(nil); err == nil {
			t.Fatal("overflowing catalog finished successfully")
		}
		code, _ := catalogError(t, writer)
		if code != "resource_exhausted" {
			t.Fatalf("wrong bounded-buffer error code %q", code)
		}
	})
	t.Run("native protobuf expansion", func(t *testing.T) {
		writer := httptest.NewRecorder()
		reply := (CatalogAdapter{Kind: CliModels}).Reply(writer)
		reply.Header().Set("Content-Type", "application/json")
		body := `{"object":"list","data":[{"id":"` + strings.Repeat("x", maxBody/2+32) + `","object":"model"}]}`
		if len(body) >= maxBody {
			t.Fatal("native expansion fixture exceeds JSON bound")
		}
		if _, err := io.WriteString(reply, body); err != nil {
			t.Fatal(err)
		}
		if err := reply.Finish(nil); err == nil {
			t.Fatal("expanded protobuf exceeded its limit")
		}
		if code, _ := catalogError(t, writer); code != "resource_exhausted" {
			t.Fatalf("wrong native catalog limit error code %q", code)
		}
	})
}

func TestCatalogReplyCanonicalErrors(t *testing.T) {
	for status, expected := range map[int]string{
		400: "invalid_argument", 401: "unauthenticated", 403: "permission_denied",
		404: "unimplemented", 405: "unimplemented", 408: "deadline_exceeded",
		409: "aborted", 413: "resource_exhausted", 415: "invalid_argument",
		422: "invalid_argument", 429: "resource_exhausted", 499: "canceled",
		500: "internal", 501: "unimplemented", 502: "unavailable", 503: "unavailable",
		504: "deadline_exceeded",
	} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			writer := httptest.NewRecorder()
			reply := (CatalogAdapter{Kind: CliModels}).Reply(writer)
			reply.Header().Set("Content-Type", "application/json")
			reply.WriteHeader(status)
			reply.WriteHeader(200)
			if _, err := io.WriteString(reply, `{"error":{"message":"private-vendor-diagnostic"}}`); err != nil {
				t.Fatal(err)
			}
			if err := reply.Finish(nil); err == nil {
				t.Fatal("canonical discovery error was swallowed")
			}
			code, message := catalogError(t, writer)
			if writer.Code != status || code != expected || strings.Contains(message, "private-vendor-diagnostic") {
				t.Fatalf("wrong native discovery error: status %d code %q message %q", writer.Code, code, message)
			}
			size := writer.Body.Len()
			reply.Finish(nil)
			if size != writer.Body.Len() {
				t.Fatal("failed discovery emitted a duplicate error")
			}
		})
	}
}

func TestCatalogReplyCancellationAndFormat(t *testing.T) {
	for _, test := range []struct {
		name   string
		cause  error
		mime   string
		status int
		code   string
	}{
		{"deadline", fmt.Errorf("private-cause: %w", context.DeadlineExceeded), "application/json", 200, "deadline_exceeded"},
		{"canceled", context.Canceled, "application/json", 200, "canceled"},
		{"other cause", errors.New("private-cause"), "application/json", 200, "internal"},
		{"missing format", nil, "", 200, "internal"},
		{"SSE format", nil, "text/event-stream", 200, "internal"},
		{"native format", nil, "application/proto", 200, "internal"},
		{"redirect", nil, "application/json", 302, "internal"},
		{"empty success status", nil, "application/json", 204, "internal"},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer := httptest.NewRecorder()
			reply := (CatalogAdapter{Kind: CliModels}).Reply(writer)
			reply.Header().Set("Content-Type", test.mime)
			reply.WriteHeader(test.status)
			if _, err := io.WriteString(reply, catalogFixture); err != nil {
				t.Fatal(err)
			}
			if err := reply.Finish(test.cause); err == nil {
				t.Fatal("catalog failure was ignored")
			}
			code, message := catalogError(t, writer)
			if code != test.code || strings.Contains(message, "private-cause") {
				t.Fatalf("unsafe or wrong catalog error: %q %q", code, message)
			}
		})
	}
}

func TestCatalogReplyHeaderBoundary(t *testing.T) {
	writer := httptest.NewRecorder()
	writer.Header().Set("Content-Length", "123")
	writer.Header().Set("Content-Encoding", "gzip")
	writer.Header().Set("Connect-Content-Encoding", "gzip")
	writer.Header().Set("Trailer", "Private-Trailer")
	reply := (CatalogAdapter{Kind: CliModels}).Reply(writer)
	reply.Header().Set("Content-Type", "application/json")
	reply.Header().Set("Content-Length", "999")
	reply.Header().Set("X-Private", "private-header")
	reply.Header().Set("Retry-After", "3")
	reply.Header().Set("X-Magpie-Session", "scoped-session")
	reply.WriteHeader(103)
	if _, err := io.WriteString(reply, catalogFixture); err != nil {
		t.Fatal(err)
	}
	if err := reply.Finish(nil); err != nil {
		t.Fatal(err)
	}
	if writer.Header().Get("Content-Length") != "" || writer.Header().Get("Content-Encoding") != "" ||
		writer.Header().Get("Connect-Content-Encoding") != "" || writer.Header().Get("Trailer") != "" ||
		writer.Header().Get("X-Private") != "" || writer.Header().Get("Retry-After") != "3" ||
		writer.Header().Get("X-Magpie-Session") != "scoped-session" || writer.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("catalog response crossed its header boundary: %v", writer.Header())
	}
}

func TestCatalogReplyRejectsEncodedOrAmbiguousJSON(t *testing.T) {
	for _, header := range []http.Header{
		{"Content-Type": {"application/json", "application/json"}},
		{"Content-Type": {"application/json"}, "Content-Encoding": {"gzip"}},
		{"Content-Type": {"application/json"}, "Content-Encoding": {"identity", "identity"}},
	} {
		writer := httptest.NewRecorder()
		reply := (CatalogAdapter{Kind: CliModels}).Reply(writer)
		for name, values := range header {
			reply.Header()[name] = values
		}
		if _, err := io.WriteString(reply, catalogFixture); err != nil {
			t.Fatal(err)
		}
		if err := reply.Finish(nil); err == nil {
			t.Fatalf("accepted encoded or ambiguous catalog JSON: %v", header)
		}
		catalogError(t, writer)
	}
}

type catalogFailingWriter struct {
	*httptest.ResponseRecorder
	cause error
}

func (writer catalogFailingWriter) Write(body []byte) (int, error) {
	return 0, writer.cause
}

func TestCatalogReplyWriteFailure(t *testing.T) {
	for _, cause := range []error{nil, io.ErrClosedPipe} {
		writer := catalogFailingWriter{httptest.NewRecorder(), cause}
		reply := (CatalogAdapter{Kind: CliModels}).Reply(writer)
		reply.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(reply, catalogFixture); err != nil {
			t.Fatal(err)
		}
		if err := reply.Finish(nil); err == nil {
			t.Fatal("native catalog write failure was ignored")
		}
	}
}
