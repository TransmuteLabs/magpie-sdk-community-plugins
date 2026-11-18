package outbound

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const AuthorizeURL = "https://app.devin.ai/auth/cli/continue"

type PKCE struct {
	Verifier string
	State    string
	Redirect string
	Deadline time.Time
}

func validRedirect(redirect string) (*url.URL, error) {
	address, err := url.Parse(redirect)
	if err != nil || address.Scheme != "http" || address.Hostname() != "127.0.0.1" ||
		address.Port() == "" || address.Path != "/callback" || address.RawQuery != "" || address.Fragment != "" ||
		address.User != nil || address.RawPath != "" {
		return nil, errors.New("Devin PKCE requires an explicit 127.0.0.1 callback")
	}
	if _, _, err := net.SplitHostPort(address.Host); err != nil {
		return nil, errors.New("Devin PKCE callback must name its port")
	}
	port, err := strconv.ParseUint(address.Port(), 10, 16)
	if err != nil || port == 0 {
		return nil, errors.New("Devin PKCE callback has an invalid port")
	}
	return address, nil
}

func PreparePKCE(redirect string, now time.Time) (PKCE, string, error) {
	if _, err := validRedirect(redirect); err != nil {
		return PKCE{}, "", err
	}
	verifier, state := make([]byte, 48), make([]byte, 24)
	if _, err := rand.Read(verifier); err != nil {
		return PKCE{}, "", errors.New("could not allocate Devin PKCE state")
	}
	if _, err := rand.Read(state); err != nil {
		return PKCE{}, "", errors.New("could not allocate Devin PKCE state")
	}
	flow := PKCE{Verifier: base64.RawURLEncoding.EncodeToString(verifier),
		State: base64.RawURLEncoding.EncodeToString(state), Redirect: redirect, Deadline: now.Add(10 * time.Minute)}
	challenge := sha256.Sum256([]byte(flow.Verifier))
	query := url.Values{
		"redirect_uri": {redirect}, "state": {flow.State}, "prompt": {"select_account"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"}, "cli_pkce_marker": {"1"},
	}
	return flow, AuthorizeURL + "?" + query.Encode(), nil
}

func (flow PKCE) Callback(callback string, now time.Time) (string, error) {
	got, err := url.Parse(callback)
	expected, expectedErr := validRedirect(flow.Redirect)
	if err != nil || expectedErr != nil || expected == nil || got == nil ||
		now.Before(flow.Deadline.Add(-10*time.Minute)) || !now.Before(flow.Deadline) ||
		got.Scheme != expected.Scheme || got.Host != expected.Host || got.Path != expected.Path ||
		got.User != nil || got.RawPath != "" || got.Fragment != "" {
		return "", errors.New("invalid or expired Devin PKCE callback")
	}
	query, err := url.ParseQuery(got.RawQuery)
	if err != nil || len(query["state"]) != 1 || len(query["code"]) != 1 || len(query["error"]) != 0 ||
		flow.State == "" || subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(flow.State)) != 1 {
		return "", errors.New("Devin PKCE callback state did not match")
	}
	code := query.Get("code")
	if code == "" || len(code) > 4096 || strings.TrimSpace(code) != code || strings.ContainsAny(code, "\r\n\x00") {
		return "", errors.New("invalid Devin authorization code")
	}
	return code, nil
}

type ExchangeResult struct {
	Credentials Credentials
	WebappHost  string
	APIURL      string
}

func (client *Client) Exchange(ctx context.Context, flow PKCE, callback string) (ExchangeResult, error) {
	code, err := flow.Callback(callback, client.now())
	if err != nil || len(flow.Verifier) < 43 || len(flow.Verifier) > 128 {
		return ExchangeResult{}, &Error{Operation: "exchange", Code: "invalid_params", Outcome: "not-started"}
	}
	for _, value := range []byte(flow.Verifier) {
		if !(value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '_' || value == '-') {
			return ExchangeResult{}, &Error{Operation: "exchange", Code: "invalid_params", Outcome: "not-started"}
		}
	}
	body, _ := json.Marshal(map[string]string{"code": code, "code_verifier": flow.Verifier})
	response, err := client.unary(ctx, "exchange", ExchangePath, "application/json", body)
	if err != nil {
		return ExchangeResult{}, err
	}
	var reply struct {
		Session      string `json:"sessionToken"`
		SessionSnake string `json:"session_token"`
		WebappHost   string `json:"devinWebappHost"`
		APIURL       string `json:"devinApiUrl"`
	}
	if decodeReply(response, &reply) != nil || reply.Session != "" && reply.SessionSnake != "" && reply.Session != reply.SessionSnake {
		return ExchangeResult{}, &Error{Operation: "exchange", Code: "data_loss"}
	}
	key := reply.Session
	if key == "" {
		key = reply.SessionSnake
	}
	credentials := Credentials{Key: key}
	if credentials.Validate() != nil {
		return ExchangeResult{}, &Error{Operation: "exchange", Code: "data_loss"}
	}
	return ExchangeResult{Credentials: credentials, WebappHost: reply.WebappHost, APIURL: reply.APIURL}, nil
}
