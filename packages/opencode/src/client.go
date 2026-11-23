package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxReplyBytes = 1 << 20

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type Endpoints struct {
	Console   string
	Inference string
}

func DefaultEndpoints() Endpoints {
	return Endpoints{Console: "https://opencode.ai/console", Inference: "https://opencode.ai"}
}

func (value Endpoints) validate() error {
	for _, raw := range []string{value.Console, value.Inference} {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("invalid OpenCode endpoint root")
		}
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
			return errors.New("OpenCode endpoints require HTTPS or an injected loopback fixture")
		}
	}
	return nil
}

type Client struct {
	HTTP      Doer
	Endpoints Endpoints
	Now       func() time.Time
	Version   string
}

func NewClient(httpClient Doer, endpoints Endpoints) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("OpenCode requires an HTTP client")
	}
	endpoints.Console = strings.TrimRight(endpoints.Console, "/")
	endpoints.Inference = strings.TrimRight(endpoints.Inference, "/")
	if err := endpoints.validate(); err != nil {
		return nil, err
	}
	return &Client{HTTP: httpClient, Endpoints: endpoints, Now: time.Now, Version: ClientVersion}, nil
}

func (client *Client) request(ctx context.Context, method, target, access string, input any) (int, []byte, error) {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "opencode/latest/"+client.Version+"/cli")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
	}
	res, err := client.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxReplyBytes+1))
	if err != nil {
		return 0, nil, err
	}
	if len(data) > maxReplyBytes {
		return 0, nil, errors.New("OpenCode reply exceeds its byte limit")
	}
	return res.StatusCode, data, nil
}

func (client *Client) StartDevice(ctx context.Context) (DeviceCode, error) {
	status, body, err := client.request(ctx, http.MethodPost, client.Endpoints.Console+"/auth/device/code", "",
		map[string]any{"client_id": ClientID, "supports_org_scope": true})
	if err != nil {
		return DeviceCode{}, err
	}
	if status < 200 || status >= 300 {
		return DeviceCode{}, &RemoteError{Operation: "device authorization", Status: status}
	}
	var result DeviceCode
	if json.Unmarshal(body, &result) != nil || result.Device == "" || result.User == "" ||
		result.ExpiresIn <= 0 || result.ExpiresIn > int64((1<<63-1)/time.Second) {
		return DeviceCode{}, errors.New("OpenCode returned invalid device authorization")
	}
	base, _ := url.Parse(client.Endpoints.Console + "/")
	address, err := url.Parse(result.URL)
	if err != nil || result.URL == "" {
		return DeviceCode{}, errors.New("OpenCode returned an invalid verification URL")
	}
	address = base.ResolveReference(address)
	if (address.Scheme != "https" && address.Scheme != "http") || address.Host == "" || address.User != nil {
		return DeviceCode{}, errors.New("OpenCode returned an invalid verification URL")
	}
	result.URL = address.String()
	if result.Interval < 1 {
		result.Interval = 1 // donor devicePollFloor, not a new retry policy
	}
	return result, nil
}

func (client *Client) PollDevice(ctx context.Context, device string) (PollResult, error) {
	if device == "" {
		return PollResult{}, errors.New("OpenCode device code is missing")
	}
	status, body, err := client.request(ctx, http.MethodPost, client.Endpoints.Console+"/auth/device/token", "",
		map[string]any{"grant_type": "urn:ietf:params:oauth:grant-type:device_code", "device_code": device, "client_id": ClientID})
	if err != nil {
		return PollResult{}, err
	}
	receivedAt := client.Now()
	var token TokenReply
	if json.Unmarshal(body, &token) != nil {
		return PollResult{}, errors.New("OpenCode returned an invalid device token reply")
	}
	switch token.Error {
	case "authorization_pending":
		return PollResult{Pending: true}, nil
	case "slow_down":
		return PollResult{Pending: true, SlowDown: true}, nil
	case "":
	default:
		return PollResult{}, &RemoteError{Operation: "device authorization", Status: status, Code: publicCode(token.Error)}
	}
	if status < 200 || status >= 300 || !token.valid() {
		return PollResult{}, &RemoteError{Operation: "device token", Status: status}
	}
	token.expiresAt = receivedAt.Add(time.Duration(token.ExpiresIn) * time.Second)
	return PollResult{Token: &token}, nil
}

func (client *Client) Identity(ctx context.Context, token TokenReply) (Credentials, error) {
	if !token.valid() {
		return Credentials{}, errors.New("OpenCode token is incomplete")
	}
	expiresAt := token.expiresAt
	if expiresAt.IsZero() {
		// Direct callers without a captured response still anchor before I/O.
		expiresAt = client.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}
	get := func(path string, out any) error {
		status, body, err := client.request(ctx, http.MethodGet, client.Endpoints.Console+path, token.Access, nil)
		if err != nil {
			return err
		}
		if status < 200 || status >= 300 {
			return &RemoteError{Operation: "identity", Status: status}
		}
		if json.Unmarshal(body, out) != nil {
			return errors.New("OpenCode returned invalid identity JSON")
		}
		return nil
	}
	var user struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	var orgs []Organization
	if err := get("/api/user", &user); err != nil {
		return Credentials{}, err
	}
	if err := get("/api/orgs", &orgs); err != nil {
		return Credentials{}, err
	}
	var org Organization
	if token.OrgID != nil {
		for _, candidate := range orgs {
			if candidate.ID == *token.OrgID {
				org = candidate
				break
			}
		}
		if org.ID == "" {
			return Credentials{}, errors.New("OpenCode token-selected organization is unavailable")
		}
	} else if len(orgs) == 1 {
		org = orgs[0]
	} else if len(orgs) > 1 {
		return Credentials{}, ErrAmbiguousOrganization
	}
	result := Credentials{Access: token.Access, Refresh: token.Refresh,
		Expires: expiresAt,
		UserID:  user.ID, Email: user.Email, InitialOrgID: org.ID, OrgID: org.ID, OrgName: org.Name}
	return result, result.Validate()
}

// Refresh exchanges one explicitly selected token. It does not persist, retry,
// mark an account revoked, or claim to serialize publication across hosts.
func (client *Client) Refresh(ctx context.Context, old Credentials) (Credentials, error) {
	if err := old.Validate(); err != nil {
		return Credentials{}, err
	}
	status, body, err := client.request(ctx, http.MethodPost, client.Endpoints.Console+"/auth/device/token", "",
		map[string]any{"grant_type": "refresh_token", "refresh_token": old.Refresh, "client_id": ClientID})
	if err != nil {
		return Credentials{}, err
	}
	var token TokenReply
	if json.Unmarshal(body, &token) != nil {
		return Credentials{}, errors.New("OpenCode returned invalid refresh JSON")
	}
	if status == http.StatusBadRequest && token.Error == "invalid_grant" {
		return Credentials{}, &RemoteError{Operation: "refresh", Status: status, Code: "invalid_grant"}
	}
	if status < 200 || status >= 300 || token.Error != "" || !token.valid() {
		return Credentials{}, &RemoteError{Operation: "refresh", Status: status, Code: publicCode(token.Error)}
	}
	result := old
	result.Access, result.Refresh = token.Access, token.Refresh
	result.Expires = client.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	if token.OrgID != nil && *token.OrgID != old.OrgID {
		result.OrgID, result.OrgName = *token.OrgID, *token.OrgID
	}
	if err := result.Validate(); err != nil {
		return Credentials{}, err
	}
	return result, nil
}

func ClearAuth(headers http.Header) {
	for key := range headers {
		switch strings.ToLower(key) {
		case "authorization", "x-api-key", "x-goog-api-key", "api-key", "cookie",
			"x-org-id", "x-opencode-org-id", "proxy-authorization":
			delete(headers, key)
		}
	}
}

func (client *Client) Chat(ctx context.Context, credentials Credentials, headers http.Header, body []byte) (*http.Response, error) {
	return client.generate(ctx, credentials, headers, body, goOpenAIBase+"/chat/completions", false)
}

func (client *Client) GenerateContent(ctx context.Context, credentials Credentials, headers http.Header, body []byte, model string, streaming bool) (*http.Response, error) {
	if !modelSupports(model, "gemini") {
		return nil, errors.New("OpenCode model does not use the native Gemini endpoint")
	}
	return client.generateGemini(ctx, credentials, headers, body, model, streaming)
}

func (client *Client) generateGemini(ctx context.Context, credentials Credentials, headers http.Header, body []byte, model string, streaming bool) (*http.Response, error) {
	endpoint := goGeminiBase + "/models/" + url.PathEscape(model) + ":generateContent"
	if streaming {
		endpoint = goGeminiBase + "/models/" + url.PathEscape(model) + ":streamGenerateContent?alt=sse"
	}
	return client.generate(ctx, credentials, headers, body, endpoint, true)
}

func (client *Client) modelList(ctx context.Context, target, access, orgHeader, org string) []byte {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+access)
	request.Header.Set("User-Agent", "opencode/latest/"+client.Version+"/cli")
	request.Header.Set("x-opencode-client", "cli")
	if org != "" {
		request.Header.Set(orgHeader, org)
	}
	response, err := client.HTTP.Do(request)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil
	}
	body, err := readBounded(response.Body, maxReplyBytes)
	if err != nil {
		return nil
	}
	return body
}

func (client *Client) generate(ctx context.Context, credentials Credentials, headers http.Header, body []byte, endpoint string, gemini bool) (*http.Response, error) {
	if err := credentials.Validate(); err != nil {
		return nil, err
	}
	if credentials.Due(client.Now()) {
		return nil, ErrRefreshRequired
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.Endpoints.Inference+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = headers.Clone()
	if req.Header == nil {
		req.Header = http.Header{}
	}
	ClearAuth(req.Header)
	// The transport must negotiate gzip itself to enable automatic decoding.
	for key := range req.Header {
		if strings.EqualFold(key, "Accept-Encoding") {
			delete(req.Header, key)
		}
	}
	if gemini {
		req.Header.Set("x-goog-api-key", credentials.Access)
	} else {
		req.Header.Set("Authorization", "Bearer "+credentials.Access)
	}
	if credentials.OrgID != "" {
		req.Header.Set("x-opencode-org-id", credentials.OrgID)
	}
	req.Header.Set("Content-Type", "application/json")
	return client.HTTP.Do(req)
}

func (client *Client) Usage(ctx context.Context, credentials Credentials, headers http.Header) ([]Window, error) {
	if err := credentials.Validate(); err != nil {
		return nil, err
	}
	if credentials.Due(client.Now()) {
		return nil, ErrRefreshRequired
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.Endpoints.Inference+"/zen/go/v1/usage", nil)
	if err != nil {
		return nil, err
	}
	req.Header = headers.Clone()
	if req.Header == nil {
		req.Header = http.Header{}
	}
	ClearAuth(req.Header)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "opencode/latest/"+client.Version+"/cli")
	req.Header.Set("Authorization", "Bearer "+credentials.Access)
	if credentials.OrgID != "" {
		req.Header.Set("x-opencode-org-id", credentials.OrgID)
	}
	res, err := client.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxReplyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxReplyBytes {
		return nil, errors.New("OpenCode usage exceeds its byte limit")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, &RemoteError{Operation: "usage", Status: res.StatusCode}
	}
	return ParseUsage(body)
}
