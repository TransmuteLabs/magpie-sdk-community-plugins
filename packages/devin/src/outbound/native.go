package outbound

import (
	"bytes"
	"context"
	"errors"
	"net/http"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

func (client *Client) Native(ctx context.Context, credentials Credentials, payload []byte, model string,
	accept func(context.Context) error, emit func(codec.NativeFrame) error,
) (codec.NativeSummary, error) {
	var empty codec.NativeSummary
	if err := client.selected(credentials); err != nil {
		return empty, err
	}
	if ctx.Err() != nil {
		return empty, &Error{Operation: "native", Outcome: "not-started", cause: ctx.Err()}
	}
	requestBody, err := codec.ReplaceNativeAuthority(payload, model, credentials.Key)
	if err != nil || accept == nil || emit == nil {
		return empty, &Error{Operation: "native", Code: "invalid_params", Outcome: "not-started"}
	}
	body, compressed, err := codec.EncodeConnectRequest(requestBody)
	if err != nil {
		return empty, &Error{Operation: "native", Code: "invalid_params", Outcome: "not-started"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.server+ChatPath, bytes.NewReader(body))
	if err != nil {
		return empty, &Error{Operation: "native", Outcome: "not-started"}
	}
	request.GetBody = nil
	request.Header.Set("Content-Type", "application/connect+proto")
	request.Header.Set("Connect-Protocol-Version", "1")
	request.Header.Set("Authorization", "Basic "+credentials.Key)
	if compressed {
		request.Header.Set("Connect-Content-Encoding", "gzip")
	}
	if err := ctx.Err(); err != nil {
		return empty, &Error{Operation: "native", Outcome: "not-started", cause: err}
	}
	if err := accept(ctx); err != nil {
		return empty, &Error{Operation: "acceptance", Outcome: "uncertain", cause: err}
	}
	readCtx, cancelRead := context.WithCancelCause(ctx)
	defer cancelRead(nil)
	response, err := client.http.Do(request.WithContext(readCtx))
	if err != nil {
		return empty, &Error{Operation: "native", Outcome: "uncertain", cause: ctx.Err()}
	}
	response.Body = &quietBody{ReadCloser: response.Body, cancel: cancelRead, timer: client.quietTimer}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		value, _ := readBounded(response.Body, 64<<10)
		if failure := readFailure(ctx, readCtx, "native"); failure != nil {
			return empty, failure
		}
		return empty, remoteFailure("native", response.StatusCode, value, "uncertain", response.Header)
	}
	if !media(response.Header, "application/connect+proto") {
		return empty, &Error{Operation: "native", Status: 502, Code: "data_loss", Outcome: "uncertain"}
	}
	values := response.Header.Values("Connect-Content-Encoding")
	if len(values) > 1 || len(values) == 1 && values[0] != "" && values[0] != "gzip" && values[0] != "identity" {
		return empty, &Error{Operation: "native", Status: 502, Code: "data_loss", Outcome: "uncertain"}
	}
	seen := false
	summary, err := codec.ReadNativeRawStream(readCtx, response.Body, response.Header.Get("Connect-Content-Encoding") == "gzip", func(frame codec.NativeFrame) error {
		if client.HoldRefusals && !seen && frame.Flags&2 != 0 && codec.ParseResponseFailure(frame.Payload, true).RejectsBeforeEffect() {
			return nil
		}
		seen = true
		return emit(frame)
	})
	if failure := readFailure(ctx, readCtx, "native"); failure != nil {
		return summary, failure
	}
	if err == nil {
		return summary, nil
	}
	var native *codec.NativeError
	code := ""
	if errors.As(err, &native) {
		code = safeCode(native.Code)
	}
	if errors.Is(err, context.Canceled) {
		code = "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		code = "deadline_exceeded"
	}
	failure := &Error{Operation: "native", Status: nativeStatus(code), Code: code, Outcome: "uncertain", cause: err}
	if native != nil {
		failure.Facts = responseFacts(response.StatusCode, native.Failure, response.Header)
		failure.NoEffect = native.BeforeEffect && failure.Facts.Complete
	}
	return summary, failure
}
