package protocol

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestSSEBusinessFailurePreservesPartialOutputOnce(t *testing.T) {
	wire := "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"partial\"}}\n\n" +
		"data: {\"success\":false,\"code\":1302,\"msg\":\"private vendor diagnostics\"}\n\n"
	var output bytes.Buffer
	err := RelaySSE(strings.NewReader(wire), false, func(body []byte) error {
		_, err := output.Write(body)
		return err
	})
	if !errors.Is(err, ErrStreamFailure) || strings.Count(output.String(), `"text":"partial"`) != 1 ||
		!strings.Contains(output.String(), "event: error") || !strings.Contains(output.String(), `"code":"1302"`) ||
		strings.Contains(output.String(), "private vendor diagnostics") {
		t.Fatalf("output=%s, error=%v", output.String(), err)
	}
}

func TestSSETerminalDoesNotWaitForEOF(t *testing.T) {
	reader := io.MultiReader(strings.NewReader("event: message_stop\r\ndata: {\"type\":\"message_stop\"}\r\n\r\n"), failReader{})
	if err := RelaySSE(reader, false, func([]byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

type failReader struct{}

func (failReader) Read([]byte) (int, error) {
	return 0, errors.New("read after terminal")
}

func TestSSEMalformedTruncatedAndBounded(t *testing.T) {
	for _, wire := range []string{
		"data: {\"type\":\"message_start\"}\n\n",
		"data: {\"type\":\"message_stop\"}\n",
		"data: invalid\n\n",
		"data: " + strings.Repeat("x", MaxEvent+1) + "\n\n",
	} {
		if err := RelaySSE(strings.NewReader(wire), false, func([]byte) error { return nil }); err == nil {
			t.Fatal("accepted a malformed, oversized or truncated stream")
		}
	}
}

func TestSSEToolNamesOnlyAndMultiline(t *testing.T) {
	wire := "event: content_block_start\ndata: {\"type\":\"content_block_start\",\ndata: \"content_block\":{\"type\":\"tool_use\",\"name\":\"ToolSearchTool\",\"input\":{\"name\":\"ToolSearchTool\"}}}\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"
	var output bytes.Buffer
	if err := RelaySSE(strings.NewReader(wire), true, func(body []byte) error {
		_, err := output.Write(body)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"name":"ToolSearch","type":"tool_use"`) ||
		!strings.Contains(output.String(), `"input":{"name":"ToolSearchTool"}`) {
		t.Fatal(output.String())
	}
}

func TestNestedMessageStartRestoresOnlyToolAuthority(t *testing.T) {
	wire := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"type\":\"message\",\"model\":\"raw\",\"content\":[{\"type\":\"tool_use\",\"name\":\"ToolSearchTool\",\"input\":{\"name\":\"ToolSearchTool\"}},{\"type\":\"thinking\",\"thinking\":\"opaque\",\"signature\":\"signed\"}]}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	var output bytes.Buffer
	if err := RelaySSE(strings.NewReader(wire), true, func(body []byte) error {
		_, err := output.Write(body)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"name":"ToolSearch","type":"tool_use"`) ||
		!strings.Contains(output.String(), `"input":{"name":"ToolSearchTool"}`) ||
		!strings.Contains(output.String(), `"signature":"signed"`) {
		t.Fatal("nested ToolSearch lost its authority mapping or changed opaque payload:", output.String())
	}
}

func TestBusinessFailureTypes(t *testing.T) {
	for _, sample := range []struct {
		body   string
		status int
	}{
		{`{"success":false,"code":3008,"msg":"private"}`, 429},
		{`{"code":"1302"}`, 429},
		{`{"success":false,"code":2007}`, 503},
		{`{"success":false,"code":"untrusted secret"}`, 502},
		{`{"type":"error","error":{"type":"invalid_request_error"}}`, 400},
		{`{"error":{"code":"3008","message":"private"}}`, 429},
		{`{"error":{"type":"rate_limit_error","message":"private"}}`, 429},
	} {
		status, body, failed := Failure([]byte(sample.body))
		if !failed || status != sample.status || bytes.Contains(body, []byte("private")) || bytes.Contains(body, []byte("untrusted secret")) {
			t.Fatalf("status=%d body=%s failed=%v", status, body, failed)
		}
	}
	if _, _, failed := Failure([]byte(`{"code":0,"success":true,"content":[]}`)); failed {
		t.Fatal("success converted to failure")
	}
}
