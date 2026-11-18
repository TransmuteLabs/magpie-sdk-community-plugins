package minimax

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}
type HTTPFactory func(proxy string) (Doer, func(), error)
type ByteResponse struct {
	Status  int
	Header  http.Header
	Body    []byte
	Failure *FailureFacts
}
type SelectedClient struct {
	owner       *module
	site        Site
	credentials Credentials
	call        sdk.Call
	stream      *sdk.Stream
}

func (client *SelectedClient) SiteID() string    { return client.site.ID }
func (client *SelectedClient) AccountID() string { return client.call.Scope.Account }

type APIError struct {
	Status int
	Code   string
}

func (refusal *APIError) Error() string {
	return fmt.Sprintf("MiniMax API refusal (HTTP %d, code %s)", refusal.Status, refusal.Code)
}
func (refusal *APIError) HTTPStatus() int { return refusal.Status }

type pinnedDestination struct {
	address    string
	serverName string
}

type destinationKey struct{}

type policyClient struct {
	transport *http.Transport
}

func (client *policyClient) Do(request *http.Request) (*http.Response, error) {
	if destination, pinned := request.Context().Value(destinationKey{}).(pinnedDestination); pinned {
		request = request.Clone(request.Context())
		request.Host = request.URL.Host
		request.URL.Host = destination.address
		configuration := &tls.Config{MinVersion: tls.VersionTLS12}
		if client.transport.TLSClientConfig != nil {
			configuration = client.transport.TLSClientConfig.Clone()
		}
		configuration.ServerName = destination.serverName
		client.transport.TLSClientConfig = configuration
		if client.transport.Proxy != nil {
			proxy, err := client.transport.Proxy(request)
			if err != nil {
				return nil, err
			}
			if proxy != nil && proxy.Scheme == "https" {
				proxyTLS := configuration.Clone()
				proxyTLS.ServerName = proxy.Hostname()
				proxyTLS.NextProtos = []string{"http/1.1"}
				client.transport.DialTLSContext = verifiedProxyDialer(client.transport.DialContext, proxyTLS)
			}
		}
	}
	transportClient := &http.Client{
		Transport: client.transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return transportClient.Do(request)
}

func verifiedProxyDialer(dial func(context.Context, string, string) (net.Conn, error), configuration *tls.Config) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		connection, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		secure := tls.Client(connection, configuration)
		handshakeContext, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if err := secure.HandshakeContext(handshakeContext); err != nil {
			connection.Close()
			return nil, err
		}
		return secure, nil
	}
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
		parsedURL, err := url.Parse(proxy)
		if err != nil || parsedURL.Host == "" || !contains([]string{"http", "https", "socks5", "socks5h"}, parsedURL.Scheme) {
			return nil, nil, invalid("Invalid selected proxy")
		}
		transport.Proxy = http.ProxyURL(parsedURL)
	}
	return &policyClient{transport: transport}, transport.CloseIdleConnections, nil
}
func contains(items []string, want string) bool {
	for _, literal := range items {
		if literal == want {
			return true
		}
	}
	return false
}

type closingBody struct {
	io.ReadCloser
	close func()
}

type redirectedSiteBody struct {
	io.ReadCloser
}

func (buffer *closingBody) Close() error {
	err := buffer.ReadCloser.Close()
	buffer.close()
	return err
}

func (owner *module) request(ctx context.Context, stream *sdk.Stream, req *http.Request) (*http.Response, error) {
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
	response, err := client.Do(req.WithContext(ctx))
	if err != nil {
		cleanup()
		return nil, err
	}
	if response == nil || response.Body == nil {
		cleanup()
		return nil, errors.New("empty HTTP response")
	}
	response.Body = &closingBody{ReadCloser: response.Body, close: cleanup}
	return response, nil
}

func (owner *module) siteRequest(ctx context.Context, stream *sdk.Stream, method, target string, body []byte, headers http.Header) (*http.Response, error) {
	at, err := url.Parse(target)
	if err != nil || at.Host == "" || at.User != nil || at.Fragment != "" {
		return nil, invalid("Invalid MiniMax endpoint")
	}
	for hop := 0; ; hop++ {
		req, err := http.NewRequestWithContext(ctx, method, at.String(), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header = headers.Clone()
		response, err := owner.request(ctx, stream, req)
		if err != nil {
			return nil, err
		}
		if !contains([]string{"301", "302", "303", "307", "308"}, strconv.Itoa(response.StatusCode)) || response.Header.Get("Location") == "" {
			if hop > 0 {
				response.Body = &redirectedSiteBody{ReadCloser: response.Body}
			}
			return response, nil
		}
		next, err := at.Parse(response.Header.Get("Location"))
		if err != nil {
			response.Body.Close()
			return nil, invalid("MiniMax returned an invalid redirect")
		}
		if next.Host != at.Host || next.Scheme != at.Scheme || next.User != nil {
			response.Body.Close()
			return nil, &sdk.Error{Code: "region_redirect", Message: "MiniMax redirected to another site; use the selected provider's regional proxy. Credentials were kept."}
		}
		if hop >= 5 {
			response.Body.Close()
			return nil, &sdk.Error{Code: "upstream", Message: "MiniMax redirect limit exceeded"}
		}
		response.Body.Close()
		if response.StatusCode != 307 && response.StatusCode != 308 && method != "GET" && method != "HEAD" {
			method = "GET"
			body = nil
		}
		at = next
	}
}

func bounded(response *http.Response, maxBytes int64) (ByteResponse, error) {
	defer response.Body.Close()
	if maxBytes <= 0 || maxBytes > 64<<20 {
		return ByteResponse{}, invalid("Invalid response byte limit")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		facts := refusalFacts(response.StatusCode, response.Header, body, false)
		facts.BodyComplete = false
		return ByteResponse{}, withFailureDetails(err, facts)
	}
	if int64(len(body)) > maxBytes {
		facts := refusalFacts(response.StatusCode, response.Header, nil, false)
		facts.BodyComplete = false
		return ByteResponse{}, withFailureDetails(&sdk.Error{Code: "resource_exhausted", Message: "MiniMax response exceeds its byte limit"}, facts)
	}
	return ByteResponse{Status: response.StatusCode, Header: response.Header.Clone(), Body: body}, nil
}

func endpoint(origin, path string) (string, error) {
	parsedURL, err := url.Parse(path)
	if err != nil || parsedURL.IsAbs() || parsedURL.Host != "" || parsedURL.Fragment != "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return "", invalid("MiniMax API path must be relative to its selected site")
	}
	return strings.TrimRight(origin, "/") + path, nil
}
func md5hex(literal string) string {
	sum := md5.Sum([]byte(literal))
	return hex.EncodeToString(sum[:])
}
func encodeURIComponent(literal string) string {
	var buffer strings.Builder
	const digits = "0123456789ABCDEF"
	for offset := 0; offset < len(literal); offset++ {
		octet := literal[offset]
		if octet >= 'a' && octet <= 'z' || octet >= 'A' && octet <= 'Z' || octet >= '0' && octet <= '9' || strings.ContainsRune("-_.!~*'()", rune(octet)) {
			buffer.WriteByte(octet)
		} else {
			buffer.WriteByte('%')
			buffer.WriteByte(digits[octet>>4])
			buffer.WriteByte(digits[octet&15])
		}
	}
	return buffer.String()
}
func jsonBytes(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func signed(site Site, origin, path, access, userID string, body any, query url.Values, now int64, tz int, platform string) (string, http.Header, []byte, error) {
	target, err := endpoint(origin, path)
	if err != nil {
		return "", nil, nil, err
	}
	parsedURL, err := url.Parse(target)
	if err != nil {
		return "", nil, nil, err
	}
	keys := []string{"device_platform", "biz_id", "app_id", "version_code"}
	values := map[string]string{"device_platform": "mcode", "biz_id": "3", "app_id": "3001", "version_code": "22201"}

	for _, key := range []string{"is_desktop", "desktop_version", "page_number", "page_size", "include_expired"} {
		if query.Has(key) {
			keys = append(keys, key)
			values[key] = query.Get(key)
		}
	}
	for _, key := range keys {
		if query.Has(key) {
			values[key] = query.Get(key)
		}
	}
	if userID == "" {
		userID = "0"
	}
	tail := []struct{ key, value string }{{"unix", strconv.FormatInt(now, 10)}, {"timezone_offset", strconv.Itoa(tz)}, {"sys_language", site.Lang}, {"lang", site.Lang}, {"device_id", "0"}, {"os_name", platform}, {"browser_name", "mcode"}, {"user_id", userID}, {"client", "mcode"}}
	for _, pair := range tail {
		keys = append(keys, pair.key)
		values[pair.key] = pair.value
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, url.QueryEscape(key)+"="+url.QueryEscape(values[key]))
	}
	parsedURL.RawQuery = strings.Join(parts, "&")
	var data []byte
	if body != nil {
		data, err = jsonBytes(body)
		if err != nil {
			return "", nil, nil, err
		}
	}
	if len(data) > sdk.MaxNativeBodyBytes {
		return "", nil, nil, invalid("MiniMax request exceeds its byte limit")
	}
	payload := string(data)
	if body == nil {
		payload = "{}"
	}
	stamp := strconv.FormatInt(now/1000, 10)
	signature := stamp + "I*7Cf%WZ#S&%1RlZJ&C2"
	if body != nil {
		signature += string(data)
	}
	headers := http.Header{"Accept": {"application/json"}, "Content-Type": {"application/json"}, "User-Agent": {"MiniMaxCode"}, "Authorization": {"Bearer " + access}, "Yy": {md5hex(encodeURIComponent(parsedURL.EscapedPath()+"?"+parsedURL.RawQuery) + "_" + payload + md5hex(strconv.FormatInt(now, 10)) + "ooui")}, "X-Timestamp": {stamp}, "X-Signature": {md5hex(signature)}}
	return parsedURL.String(), headers, data, nil
}

func checkedJSON(response ByteResponse) (json.RawMessage, error) {
	result, err := decodeResponseJSON(response)
	if err != nil {
		return nil, responseFailure(err, response, "http-body")
	}
	return result, nil
}

func decodeResponseJSON(response ByteResponse) (json.RawMessage, error) {
	if response.Status < 200 || response.Status >= 300 {
		return nil, &APIError{Status: response.Status, Code: "http"}
	}
	var value map[string]any
	if json.Unmarshal(response.Body, &value) != nil || value == nil {
		return nil, &sdk.Error{Code: "upstream", Message: "MiniMax returned invalid JSON"}
	}
	for _, entry := range []struct {
		value any
		key   string
	}{{value["statusInfo"], "code"}, {value["base_resp"], "status_code"}} {
		if entry.value == nil {
			continue
		}
		node := object(entry.value)
		code, ok := apiCode(node[entry.key])
		if !ok {
			return nil, &sdk.Error{Code: "upstream", Message: "MiniMax returned an invalid status envelope"}
		}
		if code != 0 {
			return nil, envelopeError(code)
		}
	}
	if value, exists := value["code"]; exists {
		code, ok := apiCode(value)
		if !ok {
			return nil, &sdk.Error{Code: "upstream", Message: "MiniMax returned an invalid tool status"}
		}
		if code != 0 {
			return nil, envelopeError(code)
		}
	}
	return append(json.RawMessage{}, response.Body...), nil
}

func apiCode(value any) (int, bool) {
	if literal, stringCode := value.(string); stringCode {
		parsed, err := strconv.ParseInt(literal, 10, 32)
		if err != nil || parsed < 0 || strconv.FormatInt(parsed, 10) != literal {
			return 0, false
		}
		return int(parsed), true
	}
	numeric, valid := value.(float64)
	if !valid || numeric < 0 || numeric > math.MaxInt32 || math.Trunc(numeric) != numeric || math.IsNaN(numeric) {
		return 0, false
	}
	return int(numeric), true
}
func envelopeError(code int) error {
	status := 0
	if code >= 400 && code <= 599 {
		status = code
	}
	switch code {
	case 1002, 1039, 1041, 1045, 2045, 2056, 2062:
		status = 429
	case 1008, 2053:
		status = 402
	case 1000048:
		status = 401
	}
	return &APIError{Status: status, Code: strconv.Itoa(code)}
}

func (client *SelectedClient) JSON(ctx context.Context, path string, body any, maxBytes int64) (json.RawMessage, error) {
	target, err := endpoint(client.site.LLM, path)
	if err != nil {
		return nil, err
	}
	if body == nil {
		body = map[string]any{}
	}
	data, err := jsonBytes(body)
	if err != nil || len(data) > sdk.MaxNativeBodyBytes {
		return nil, invalid("Invalid bounded MiniMax tool request")
	}
	headers := http.Header{"Content-Type": {"application/json"}, "User-Agent": {"MiniMaxAgent"}, "Authorization": {"Bearer " + client.credentials.Access}}
	if path == webSearchPath || path == matrixListPath {
		response, err := client.readWithPolicy(ctx, path, target, data, headers, maxBytes)
		if err != nil {
			return nil, safeError(client.selectedFailure(err))
		}
		result, err := checkedJSON(response)
		return result, client.selectedFailure(err)
	}
	response, err := client.owner.siteRequest(ctx, client.stream, "POST", target, data, headers)
	if err != nil {
		return nil, safeError(err)
	}
	boundedResponse, err := bounded(response, maxBytes)
	if err != nil {
		return nil, safeError(client.selectedFailure(err))
	}
	result, err := checkedJSON(boundedResponse)
	if err == nil && path == matrixCallPath {
		_, err = connectorResult(boundedResponse)
	}
	if err != nil {
		return nil, client.selectedFailure(err)
	}
	return result, nil
}

func unreachable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return false
	}
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "dial" {
		return true
	}
	var unknown x509.UnknownAuthorityError
	var invalidCertificate x509.CertificateInvalidError
	return errors.As(err, &unknown) || errors.As(err, &invalidCertificate)
}
func (client *SelectedClient) accountJSON(ctx context.Context, path string, body any, query url.Values, origin string) (json.RawMessage, error) {
	if origin == "" {
		origin = client.site.Agent
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	send := func(origin string) (*http.Response, error) {
		target, headers, data, err := signed(client.site, origin, path, client.credentials.Access, client.credentials.UserID, body, query, client.owner.options.Now().UnixMilli(), client.owner.options.TimezoneOffset(), client.owner.options.Platform)
		if err != nil {
			return nil, err
		}
		method := "GET"
		if body != nil {
			method = "POST"
		}
		return client.owner.siteRequest(ctx, client.stream, method, target, data, headers)
	}
	var wrote atomic.Bool
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) { wrote.Store(true) }})
	response, err := send(origin)
	if err != nil && !wrote.Load() && unreachable(err) && origin == client.site.Agent && client.site.AgentFallback != "" && client.site.AgentFallback != origin {
		response, err = send(client.site.AgentFallback)
	}
	if err != nil {
		return nil, safeError(err)
	}
	buffer, err := bounded(response, 4<<20)
	if err != nil {
		return nil, safeError(client.selectedFailure(err))
	}
	result, err := checkedJSON(buffer)
	if err != nil {
		return nil, client.selectedFailure(err)
	}
	return result, nil
}

func publicIP(ip netip.Addr) bool {
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	for _, reserved := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/32", "2002::/16", "64:ff9b::/96"} {
		if netip.MustParsePrefix(reserved).Contains(ip.Unmap()) {
			return false
		}
	}
	return true
}
func publicURL(ctx context.Context, rawURL string) (pinnedDestination, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Hostname() == "" || parsedURL.User != nil || parsedURL.Fragment != "" {
		return pinnedDestination{}, invalid("Media transfers require a public HTTPS URL")
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", parsedURL.Hostname())
	if err != nil || len(ips) == 0 {
		return pinnedDestination{}, invalid("Media host could not be resolved")
	}
	for _, ip := range ips {
		if !publicIP(ip.Unmap()) {
			return pinnedDestination{}, denied("Private media destinations are not permitted")
		}
	}
	port := parsedURL.Port()
	if port == "" {
		port = "443"
	}
	return pinnedDestination{address: net.JoinHostPort(ips[0].Unmap().String(), port), serverName: parsedURL.Hostname()}, nil
}

func (client *SelectedClient) Transfer(ctx context.Context, method, rawURL string, body []byte, headers http.Header, maxBytes int64) (ByteResponse, error) {
	if method != "GET" && method != "PUT" && method != "POST" {
		return ByteResponse{}, invalid("Unsupported media transfer method")
	}
	if int64(len(body)) > 64<<20 {
		return ByteResponse{}, invalid("Media transfer exceeds its byte limit")
	}
	destination, err := publicURL(ctx, rawURL)
	if err != nil {
		return ByteResponse{}, err
	}
	ctx = context.WithValue(ctx, destinationKey{}, destination)
	clean := http.Header{"Accept-Encoding": {"identity"}}
	for key, values := range headers {
		if !contains([]string{"content-type", "content-md5", "x-amz-content-sha256", "x-amz-date", "x-amz-security-token", "x-oss-security-token"}, strings.ToLower(key)) {
			return ByteResponse{}, invalid("Unexpected media transfer header")
		}
		clean[key] = append([]string{}, values...)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return ByteResponse{}, invalid("Invalid media URL")
	}
	req.Header = clean
	response, err := client.owner.request(ctx, client.stream, req)
	if err != nil {
		return ByteResponse{}, safeError(err)
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		response.Body.Close()
		facts := refusalFacts(response.StatusCode, response.Header, nil, false)
		facts.Origin, facts.BodyComplete = "media-transfer", false
		return ByteResponse{}, client.redactedFailure(withFailureDetails(denied("Media transfer redirects are not permitted"), facts))
	}
	result, err := bounded(response, maxBytes)
	if err != nil {
		return ByteResponse{}, safeError(client.redactedFailure(err))
	}
	if result.Status < 200 || result.Status >= 300 {
		facts := redactFailureFacts(refusalFacts(result.Status, result.Header, result.Body, false), client.credentials)
		facts.Origin = "media-transfer"
		result.Failure = &facts
	}
	return result, nil
}
