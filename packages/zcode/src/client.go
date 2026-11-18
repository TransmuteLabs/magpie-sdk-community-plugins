package zcode

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"
)

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type HTTPFactory func(proxy string) (Doer, func(), error)

type Endpoints struct {
	Zai                 string
	BigModel            string
	Start               string
	ZaiQuota            string
	BigModelQuota       string
	LegacyBigModelQuota string
}

func DefaultEndpoints() Endpoints {
	return Endpoints{
		Zai:                 "https://api.z.ai/api/anthropic/v1/messages",
		BigModel:            "https://open.bigmodel.cn/api/anthropic/v1/messages",
		Start:               "https://zcode.z.ai/api/v1/zcode-plan/anthropic/v1/messages",
		ZaiQuota:            "https://api.z.ai/api/monitor/usage/quota/limit",
		BigModelQuota:       "https://bigmodel.cn/api/monitor/usage/quota/limit",
		LegacyBigModelQuota: "https://open.bigmodel.cn/api/monitor/usage/quota/limit",
	}
}

func (value Endpoints) validate() error {
	for _, endpoint := range []string{value.Zai, value.BigModel, value.Start, value.ZaiQuota, value.BigModelQuota, value.LegacyBigModelQuota} {
		address, err := url.Parse(endpoint)
		if err != nil || address.User != nil || address.RawQuery != "" || address.Fragment != "" ||
			address.Host == "" || address.Scheme != "https" && !(address.Scheme == "http" && net.ParseIP(address.Hostname()).IsLoopback()) {
			return errors.New("invalid ZCode endpoint")
		}
	}
	return nil
}

func (value Endpoints) quotaForAccount(account Credentials) string {
	if account.Site == "bigmodel" {
		if account.Kind == "legacy-key" && account.Organization == "" {
			return value.LegacyBigModelQuota
		}
		return value.BigModelQuota
	}
	return value.ZaiQuota
}

func (value Endpoints) forAccount(account Credentials) string {
	if account.Kind == "start" {
		return value.Start
	}
	if account.Site == "bigmodel" {
		return value.BigModel
	}
	return value.Zai
}

func defaultHTTP(proxy string) (Doer, func(), error) {
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	if proxy != "" {
		address, err := url.Parse(proxy)
		if err != nil || address.Host == "" || address.Scheme != "http" && address.Scheme != "https" &&
			address.Scheme != "socks5" && address.Scheme != "socks5h" {
			return nil, nil, errors.New("invalid host proxy resolution")
		}
		transport.Proxy = http.ProxyURL(address)
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, transport.CloseIdleConnections, nil
}

func requestHeaders(source http.Header, account Credentials, requestID string) http.Header {
	result := http.Header{}
	for key, values := range source {
		if strings.EqualFold(key, "anthropic-beta") && account.Kind != "start" {
			result["Anthropic-Beta"] = append(result["Anthropic-Beta"], values...)
		}
	}
	result.Set("Content-Type", "application/json")
	result.Set("Authorization", "Bearer "+account.token())
	if account.Kind != "start" {
		result.Set("X-Api-Key", account.token())
	}
	result.Set("Anthropic-Version", "2023-06-01")
	result.Set("User-Agent", "ZCode/"+ClientVersion+" ai-sdk/anthropic/3.0.81")
	result.Set("X-ZCode-App-Version", ClientVersion)
	result.Set("X-Request-Id", requestID)
	result.Set("HTTP-Referer", "https://zcode.z.ai")
	if account.Kind == "start" {
		result.Set("X-Title", "Z Code@electron")
		result.Set("X-Release-Channel", "production")
		result.Set("X-ZCode-Agent", "glm")
		result.Set("X-Platform", platform())
		result.Set("X-ZCode-Session-Type", "main")
		result.Set("X-ZCode-Query-Source", "main_turn")
	}
	if account.Kind == "team" || account.Kind == "legacy-key" && account.Organization != "" {
		result.Set("Bigmodel-Organization", account.Organization)
		result.Set("Bigmodel-Project", account.Project)
	}
	return result
}

func platform() string {
	osName, architecture := runtime.GOOS, runtime.GOARCH
	if osName == "windows" {
		osName = "win32"
	}
	if architecture == "amd64" {
		architecture = "x64"
	}
	return osName + "-" + architecture
}

func responseHeaders(source http.Header) http.Header {
	result := http.Header{}
	for _, name := range []string{"Content-Type", "Retry-After", "X-Request-Id", "Request-Id", "Anthropic-Ratelimit-Requests-Remaining", "Anthropic-Ratelimit-Tokens-Remaining"} {
		for _, value := range source.Values(name) {
			result.Add(name, value)
		}
	}
	return result
}

type cleanupBody struct {
	io.ReadCloser
	cleanup func()
}

func (value *cleanupBody) Close() error {
	err := value.ReadCloser.Close()
	value.cleanup()
	return err
}

func send(ctx context.Context, factory HTTPFactory, proxy string, request *http.Request) (*http.Response, error) {
	client, cleanup, err := factory(proxy)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request.WithContext(ctx))
	if err != nil {
		cleanup()
		return nil, err
	}
	if response == nil || response.Body == nil {
		cleanup()
		return nil, errors.New("ZCode transport returned no body")
	}
	response.Body = &cleanupBody{ReadCloser: response.Body, cleanup: cleanup}
	return response, nil
}
