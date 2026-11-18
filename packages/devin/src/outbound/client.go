package outbound

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	sdk "github.com/yetone/magpie/pkg/extension"
)

const (
	DefaultServer = "https://server.codeium.com"
	ChatPath      = "/exa.api_server_pb.ApiServerService/GetChatMessage"
	ModelsPath    = "/exa.api_server_pb.ApiServerService/GetCliModelConfigs"
	StatusPath    = "/exa.seat_management_pb.SeatManagementService/GetUserStatus"
	ExchangePath  = "/exa.seat_management_pb.SeatManagementService/ExchangeDevinCLIPKCECode"
	maxReply      = 16 << 20
)

type Credentials struct {
	Key       string    `json:"key"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

func (credentials Credentials) Validate() error {
	_, err := codec.NativeMetadata(credentials.Key)
	return err
}

func (credentials Credentials) AccountID(server string) string {
	sum := sha256.Sum256([]byte(server + "\x00" + credentials.Key))
	return hex.EncodeToString(sum[:16])
}

func (credentials Credentials) Due(now time.Time) bool {
	return !credentials.ExpiresAt.IsZero() && credentials.ExpiresAt.Sub(now) <= 30*time.Second
}

type Error struct {
	Operation string
	Status    int
	Code      string
	Outcome   string
	Facts     *sdk.RequestFailureFacts
	NoEffect  bool
	cause     error
}

func (err *Error) Error() string {
	return "Devin " + err.Operation + " failed"
}

func (err *Error) Unwrap() error { return err.cause }

type Client struct {
	server       string
	http         *http.Client
	now          func() time.Time
	quietTimer   func(time.Duration, func()) func() bool
	HoldRefusals bool
}

func New(server string, client *http.Client, now func() time.Time) (*Client, error) {
	if server == "" {
		server = DefaultServer
	}
	address, err := url.Parse(server)
	if err != nil || address.Host == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" ||
		address.RawPath != "" || address.Path != "" && address.Path != "/" {
		return nil, errors.New("Devin API server must be an explicit origin")
	}
	if address.Scheme != "https" {
		ip := net.ParseIP(address.Hostname())
		if address.Scheme != "http" || ip == nil || !ip.IsLoopback() {
			return nil, errors.New("Devin API server requires HTTPS or an explicit loopback fixture")
		}
	}
	if client == nil {
		transport := &http.Transport{
			Proxy:             http.ProxyFromEnvironment,
			DialContext:       (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 30 * time.Second,
		}
		client = &http.Client{Transport: transport}
	}
	owned := *client
	owned.Jar = nil
	owned.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if now == nil {
		now = time.Now
	}
	return &Client{
		server: strings.TrimRight(server, "/"), http: &owned, now: now,
		quietTimer: func(wait time.Duration, expired func()) func() bool { return time.AfterFunc(wait, expired).Stop },
	}, nil
}

func (client *Client) selected(credentials Credentials) error {
	if credentials.Validate() != nil {
		return &Error{Operation: "credentials", Code: "invalid_credentials", Outcome: "not-started"}
	}
	if credentials.Due(client.now()) {
		return &Error{Operation: "credentials", Code: "auth_refresh_required", Outcome: "not-started"}
	}
	return nil
}

func safeCode(code string) string {
	switch code {
	case "invalid_argument", "unauthenticated", "permission_denied", "resource_exhausted", "unavailable",
		"deadline_exceeded", "canceled", "unimplemented", "data_loss", "internal", "failed_precondition":
		return code
	default:
		return ""
	}
}

func nativeStatus(code string) int {
	switch code {
	case "invalid_argument":
		return http.StatusBadRequest
	case "unauthenticated":
		return http.StatusUnauthorized
	case "permission_denied":
		return http.StatusForbidden
	case "resource_exhausted":
		return http.StatusTooManyRequests
	case "unimplemented":
		return http.StatusNotImplemented
	case "unavailable":
		return http.StatusServiceUnavailable
	case "deadline_exceeded":
		return http.StatusGatewayTimeout
	case "canceled":
		return 499
	default:
		return http.StatusBadGateway
	}
}

func remoteFailure(operation string, status int, body []byte, outcome string, headers ...http.Header) error {
	original := codec.ParseResponseFailure(body, false)
	facts := responseFacts(status, original, headers...)
	noEffect := facts.Complete && original.RejectsBeforeEffect() &&
		(status == 400 || status == 401 || status == 403 || status == 429) &&
		len(headers) == 1 && media(headers[0], "application/json")
	var value struct {
		Code  string `json:"code"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &value)
	code := value.Code
	if value.Error != nil {
		code = value.Error.Code
	}
	code = safeCode(code)
	// An explicit Connect verdict can arrive inside an HTTP proxy error.
	// Bare HTTP401 alone must not erase a different vendor code or assert
	// that a static credential expired; the host owns unknown401 policy.
	if code == "unauthenticated" {
		status = http.StatusUnauthorized
	}
	return &Error{Operation: operation, Status: status, Code: code, Outcome: outcome, Facts: facts, NoEffect: noEffect}
}

func readBounded(reader io.Reader, maximum int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil || int64(len(body)) > maximum {
		return nil, errors.New("Devin reply is unreadable or exceeds its limit")
	}
	return body, nil
}

func media(header http.Header, expected string) bool {
	values := header.Values("Content-Type")
	kind, _, err := mime.ParseMediaType(header.Get("Content-Type"))
	return len(values) == 1 && err == nil && kind == expected
}

func (client *Client) unary(ctx context.Context, operation, path, contentType string, body []byte) ([]byte, error) {
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(bounded, http.MethodPost, client.server+path, bytes.NewReader(body))
	if err != nil {
		return nil, &Error{Operation: operation, Code: "invalid_params", Outcome: "not-started"}
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Accept", contentType)
	request.Header.Set("Connect-Protocol-Version", "1")
	response, err := client.http.Do(request)
	if err != nil {
		return nil, &Error{Operation: operation, cause: bounded.Err()}
	}
	defer response.Body.Close()
	payload, err := readBounded(response.Body, maxReply)
	if err != nil {
		return nil, &Error{Operation: operation, Status: response.StatusCode, Code: "data_loss", cause: bounded.Err()}
	}
	if response.StatusCode != http.StatusOK {
		return nil, remoteFailure(operation, response.StatusCode, payload, "", response.Header)
	}
	if !media(response.Header, contentType) {
		return nil, &Error{Operation: operation, Status: response.StatusCode, Code: "data_loss"}
	}
	return payload, nil
}

func (client *Client) Models(ctx context.Context, credentials Credentials) ([]codec.NativeModel, error) {
	if err := client.selected(credentials); err != nil {
		return nil, err
	}
	body, err := codec.NativeCatalogRequest(credentials.Key)
	if err != nil {
		return nil, err
	}
	payload, err := client.unary(ctx, "models", ModelsPath, "application/proto", body)
	if err != nil {
		return nil, err
	}
	models, err := codec.DecodeNativeModels(payload)
	if err != nil {
		return nil, &Error{Operation: "models", Code: "data_loss"}
	}
	return models, nil
}

func (client *Client) Chat(ctx context.Context, credentials Credentials, body []byte,
	accept func(context.Context) error, emit func(codec.NativeDelta) error,
) (codec.NativeSummary, error) {
	var empty codec.NativeSummary
	if err := client.selected(credentials); err != nil {
		return empty, err
	}
	if err := ctx.Err(); err != nil {
		return empty, &Error{Operation: "chat", Outcome: "not-started", cause: err}
	}
	encoded, err := codec.EncodeChatRequest(body, credentials.Key)
	if err != nil || accept == nil {
		return empty, &Error{Operation: "chat", Status: http.StatusBadRequest, Code: "invalid_params", Outcome: "not-started"}
	}
	if codec.IsNativeModelSelection(encoded.Model) || encoded.Effort != "" {
		models, err := client.Models(ctx, credentials)
		if err != nil {
			return empty, &Error{Operation: "models", Code: "data_loss", Outcome: "not-started", cause: err}
		}
		model, err := codec.ResolveNativeModel(models, encoded.Model, encoded.Effort)
		if err != nil {
			return empty, &Error{Operation: "models", Code: "invalid_params", Outcome: "not-started"}
		}
		encoded.Payload, err = codec.ReplaceNativeAuthority(encoded.Payload, model, credentials.Key)
		if err != nil {
			return empty, &Error{Operation: "chat", Code: "invalid_params", Outcome: "not-started"}
		}
	}
	return client.complete(ctx, credentials, encoded, accept, emit)
}

func (client *Client) Messages(ctx context.Context, credentials Credentials, encoded codec.EncodedChat,
	accept func(context.Context) error, emit func(codec.NativeDelta) error,
) (codec.NativeSummary, error) {
	return client.Complete(ctx, credentials, encoded, accept, emit)
}

func (client *Client) Complete(ctx context.Context, credentials Credentials, encoded codec.EncodedChat,
	accept func(context.Context) error, emit func(codec.NativeDelta) error,
) (codec.NativeSummary, error) {
	var empty codec.NativeSummary
	if err := client.selected(credentials); err != nil {
		return empty, err
	}
	if accept == nil || emit == nil || len(encoded.Payload) == 0 {
		return empty, &Error{Operation: "messages", Status: http.StatusBadRequest, Code: "invalid_params", Outcome: "not-started"}
	}
	return client.complete(ctx, credentials, encoded, accept, emit)
}

func (client *Client) complete(ctx context.Context, credentials Credentials, encoded codec.EncodedChat,
	accept func(context.Context) error, emit func(codec.NativeDelta) error,
) (codec.NativeSummary, error) {
	var empty codec.NativeSummary
	wire, compressed, err := codec.EncodeConnectRequest(encoded.Payload)
	if err != nil {
		return empty, &Error{Operation: "chat", Status: http.StatusBadRequest, Code: "invalid_params", Outcome: "not-started"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.server+ChatPath, bytes.NewReader(wire))
	if err != nil {
		return empty, &Error{Operation: "chat", Outcome: "not-started"}
	}
	request.Header.Set("Content-Type", "application/connect+proto")
	request.Header.Set("Connect-Protocol-Version", "1")
	request.Header.Set("Authorization", "Basic "+credentials.Key)
	if compressed {
		request.Header.Set("Connect-Content-Encoding", "gzip")
	}
	if err := ctx.Err(); err != nil {
		return empty, &Error{Operation: "chat", Outcome: "not-started", cause: err}
	}
	if err := accept(ctx); err != nil {
		return empty, &Error{Operation: "acceptance", Outcome: "uncertain", cause: err}
	}
	readCtx, cancelRead := context.WithCancelCause(ctx)
	defer cancelRead(nil)
	response, err := client.http.Do(request.WithContext(readCtx))
	if err != nil {
		return empty, &Error{Operation: "chat", Outcome: "uncertain", cause: ctx.Err()}
	}
	response.Body = &quietBody{ReadCloser: response.Body, cancel: cancelRead, timer: client.quietTimer}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := readBounded(response.Body, 64<<10)
		if failure := readFailure(ctx, readCtx, "chat"); failure != nil {
			return empty, failure
		}
		return empty, remoteFailure("chat", response.StatusCode, payload, "uncertain", response.Header)
	}
	if !media(response.Header, "application/connect+proto") {
		return empty, &Error{Operation: "chat", Status: http.StatusBadGateway, Code: "data_loss", Outcome: "uncertain"}
	}
	encodings := response.Header.Values("Connect-Content-Encoding")
	if len(encodings) > 1 || len(encodings) == 1 && encodings[0] != "" && encodings[0] != "gzip" && encodings[0] != "identity" {
		return empty, &Error{Operation: "chat", Code: "unimplemented", Outcome: "uncertain"}
	}
	summary, err := codec.ReadNativeStream(readCtx, response.Body, response.Header.Get("Connect-Content-Encoding") == "gzip", emit)
	if failure := readFailure(ctx, readCtx, "chat"); failure != nil {
		return summary, failure
	}
	if err != nil {
		var native *codec.NativeError
		code := ""
		if errors.As(err, &native) {
			code = safeCode(native.Code)
		} else if errors.Is(err, context.DeadlineExceeded) {
			code = "deadline_exceeded"
		} else if errors.Is(err, context.Canceled) {
			code = "canceled"
		}
		failure := &Error{Operation: "chat", Status: nativeStatus(code), Code: code, Outcome: "uncertain", cause: err}
		if native != nil {
			failure.Facts = responseFacts(response.StatusCode, native.Failure, response.Header)
			failure.NoEffect = native.BeforeEffect && failure.Facts.Complete
		}
		return summary, failure
	}
	return summary, nil
}
