package kimiwork

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type HTTPFactory func(proxy string) (Doer, func(), error)

type statusError struct {
	Status  int
	Message string
	Code    string
	SignIn  string
}

func (err *statusError) Error() string { return err.Message }

type policyClient struct {
	transport *http.Transport
}

func (client *policyClient) Do(request *http.Request) (*http.Response, error) {
	httpClient := &http.Client{
		Transport: client.transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("redirects are not allowed")
		},
	}
	return httpClient.Do(request)
}

func defaultHTTP(proxy string) (Doer, func(), error) {
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
	}
	if proxy != "" {
		parsed, err := url.Parse(proxy)
		if err != nil || parsed.Host == "" ||
			(parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5" && parsed.Scheme != "socks5h") {
			return nil, nil, invalid("Invalid selected proxy")
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	return &policyClient{transport: transport}, transport.CloseIdleConnections, nil
}

type closingBody struct {
	io.ReadCloser
	cleanup func()
}

func (body *closingBody) Close() error {
	err := body.ReadCloser.Close()
	body.cleanup()
	return err
}

func sanitizeASCII(value, fallback string) string {
	var builder strings.Builder
	for _, char := range value {
		if char >= 0x20 && char <= 0x7e {
			builder.WriteRune(char)
		}
	}
	trimmed := strings.TrimSpace(builder.String())
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func (owner *module) deviceModel() string {
	osType := owner.options.OSType
	if osType == "" {
		switch runtime.GOOS {
		case "darwin":
			osType = "Darwin"
		case "windows":
			osType = "Windows_NT"
		default:
			osType = "Linux"
		}
	}
	osRelease := owner.options.OSRelease
	if osRelease == "" {
		osRelease = "0"
	}
	osArch := owner.options.OSArch
	if osArch == "" {
		osArch = runtime.GOARCH
	}
	if osType == "Darwin" {
		return "macOS " + osRelease + " " + osArch
	}
	if osType == "Windows_NT" {
		return "Windows " + osRelease + " " + osArch
	}
	return strings.TrimSpace(osType + " " + osRelease + " " + osArch)
}

func (owner *module) webHeaders() http.Header {
	plat := "web"
	switch owner.options.Platform {
	case "darwin":
		plat = "mac"
	case "win32", "windows":
		plat = "windows"
	}
	return http.Header{
		"User-Agent":     {UserAgent},
		"X-Msh-Platform": {plat},
		"X-Msh-Version":  {DesktopVersion},
		"R-Timezone":     {owner.options.Timezone},
	}
}

func (owner *module) refreshHeaders() http.Header {
	system := "X11; Linux x86_64"
	switch owner.options.Platform {
	case "darwin":
		system = "Macintosh; Intel Mac OS X 10_15_7"
	case "win32", "windows":
		system = "Windows NT 10.0; Win64; x64"
	}
	headers := owner.webHeaders()
	headers.Set("X-Msh-Version", AuthVersion)
	headers.Set("User-Agent", fmt.Sprintf("Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) kimi-desktop/%s Chrome/150.0.7871.250 Electron/43.7.3 Safari/537.36", system, AuthVersion))
	headers.Set("X-Language", owner.options.Locale)
	return headers
}

func (owner *module) workHeaders(deviceID string) http.Header {
	hostname := owner.options.Hostname
	if hostname == "" {
		hostname, _ = os.Hostname()
	}
	osRelease := owner.options.OSRelease
	if osRelease == "" {
		osRelease = "0"
	}
	headers := http.Header{
		"User-Agent":           {UserAgent},
		"X-Msh-Device-Model":   {sanitizeASCII(owner.deviceModel(), "unknown")},
		"X-Msh-Device-Name":    {sanitizeASCII(hostname, "unknown")},
		"X-Msh-Os-Version":     {sanitizeASCII(osRelease, "unknown")},
		"X-Msh-Platform":       {"kimi-code-cli"},
		"X-Msh-Version":        {KimiCodeVersion},
		"X-Msh-Daimon-Version": {DaimonVersion},
	}
	if deviceID != "" {
		headers.Set("X-Msh-Device-Id", deviceID)
	}
	return headers
}

func (owner *module) doHTTP(ctx context.Context, stream *sdk.Stream, req *http.Request) (*http.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if stream == nil {
		return nil, denied("Selected host network policy is required")
	}
	var proxy sdk.NetworkProxyResult
	if err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: req.URL.String()}, &proxy); err != nil {
		return nil, err
	}
	client, cleanup, err := owner.options.HTTPFactory(proxy.URL)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		cleanup()
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		cleanup()
		return nil, errors.New("empty HTTP response")
	}
	resp.Body = &closingBody{ReadCloser: resp.Body, cleanup: cleanup}
	return resp, nil
}

func (owner *module) jsonRequest(ctx context.Context, stream *sdk.Stream, targetURL, token string, body any, deviceID string) (map[string]any, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	method := http.MethodGet
	var bodyReader io.Reader
	if body != nil {
		method = http.MethodPost
		encoded, err := encodeJSONNoEscape(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(reqCtx, method, targetURL, bodyReader)
	if err != nil {
		return nil, err
	}
	var headers http.Header
	switch {
	case targetURL == owner.options.Endpoints.Refresh:
		headers = owner.refreshHeaders()
	case strings.HasPrefix(targetURL, strings.TrimRight(owner.options.Endpoints.Base, "/")):
		headers = owner.workHeaders(deviceID)
	default:
		headers = owner.webHeaders()
	}
	headers.Set("Accept", "application/json")
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		headers.Set("Content-Type", "application/json")
		headers.Set("Connect-Protocol-Version", "1")
	}
	req.Header = headers
	resp, err := owner.doHTTP(reqCtx, stream, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, &statusError{
			Status:  http.StatusUnauthorized,
			Code:    "kimi_work_login_expired",
			SignIn:  "expired",
			Message: expiredError("", false).Message,
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &statusError{
			Status:  resp.StatusCode,
			Message: fmt.Sprintf("Kimi Work 服务暂不可用（HTTP %d）。", resp.StatusCode),
		}
	}
	const maxJSONBytes = 2 * 1024 * 1024
	if clStr := resp.Header.Get("Content-Length"); clStr != "" {
		if cl, err := strconv.ParseInt(clStr, 10, 64); err == nil && cl > maxJSONBytes {
			return nil, errors.New("Kimi Work 响应过大。")
		}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxJSONBytes {
		return nil, errors.New("Kimi Work 响应过大。")
	}
	var parsed map[string]any
	if json.Unmarshal(raw, &parsed) != nil || parsed == nil {
		return nil, errors.New("Kimi Work 响应格式无效。")
	}
	return parsed, nil
}

func (owner *module) currentSession(creds Credentials) (DesktopSession, error) {
	if !creds.Valid() {
		return DesktopSession{}, expiredError("", false)
	}
	session, err := owner.options.ReadSession()
	if err != nil {
		return DesktopSession{}, err
	}
	if creds.UID == session.UID {
		return session, nil
	}
	if key := session.Keys[creds.UID]; key != "" {
		return DesktopSession{
			WorkToken: key,
			DeviceID:  session.DeviceID,
			UID:       creds.UID,
			Away:      true,
		}, nil
	}
	return DesktopSession{}, notHereError(false)
}
