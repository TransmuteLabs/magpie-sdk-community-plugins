package zcode

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func streamResponse(ctx context.Context, response *http.Response, stream *sdk.Stream, renamed, observability bool, responseModel ...string) (any, error) {
	headers := responseHeaders(response.Header)
	usage := &executionUsage{rich: observability}
	usage.observeHeaders(response.Header)
	if usage.value() != nil {
		if err := stream.Emit(ctx, "usage", usage.value()); err != nil {
			return nil, err
		}
	}
	media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	emit := func(body []byte) error {
		for len(body) > 0 {
			count := min(len(body), sdk.MaxChunkBytes)
			if err := stream.Emit(ctx, "data", body[:count]); err != nil {
				return err
			}
			body = body[count:]
		}
		return nil
	}
	if media == "text/event-stream" && response.StatusCode >= 200 && response.StatusCode < 300 {
		if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: response.StatusCode, Headers: headers}); err != nil {
			return nil, err
		}
		err := protocol.RelaySSEObserved(response.Body, renamed, emit, func(body []byte) error {
			if usage.observe(body) {
				if err := stream.Emit(ctx, "usage", usage.value()); err != nil {
					return err
				}
			}
			return nil
		}, responseModel...)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, &sdk.Error{Code: "stream_failed", Message: "ZCode stream failed or ended before message_stop; no retry was made", Outcome: "uncertain"}
		}
		return sdk.ExecuteResult{Status: response.StatusCode, Outcome: "completed", Usage: usage.value()}, nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, protocol.MaxBody+1))
	if err != nil || len(body) > protocol.MaxBody {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &sdk.Error{Code: "upstream", Message: "ZCode response is incomplete or too large", Outcome: "uncertain"}
	}
	status := response.StatusCode
	usage.observe(body)
	if observability && usage.value() != nil {
		if err := stream.Emit(ctx, "usage", usage.value()); err != nil {
			return nil, err
		}
	}
	if failureStatus, failure, failed := protocol.Failure(body); failed {
		if status >= 200 && status < 300 {
			status = failureStatus
		}
		body = failure
		headers.Set("Content-Type", "application/json")
	} else if status >= 200 && status < 300 {
		value, err := protocol.ReadObject(body)
		if err != nil || value["type"] != "message" {
			return nil, &sdk.Error{Code: "upstream", Message: "ZCode returned an invalid Messages response", Outcome: "uncertain"}
		}
		var message struct {
			Content []json.RawMessage `json:"content"`
		}
		if json.Unmarshal(body, &message) != nil || message.Content == nil {
			return nil, &sdk.Error{Code: "upstream", Message: "ZCode returned an incomplete Messages response", Outcome: "uncertain"}
		}
		body, err = protocol.Restore(body, renamed, responseModel...)
		if err != nil {
			return nil, &sdk.Error{Code: "upstream", Message: "ZCode returned invalid tool content", Outcome: "uncertain"}
		}
		headers.Set("Content-Type", "application/json")
	}
	if err := stream.Emit(ctx, "headers", sdk.HeadersEvent{Status: status, Headers: headers}); err != nil {
		return nil, err
	}
	if err := emit(body); err != nil {
		return nil, err
	}
	return sdk.ExecuteResult{Status: status, Outcome: "completed", Usage: usage.value()}, nil
}
