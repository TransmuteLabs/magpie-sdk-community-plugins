package outbound

import (
	"bytes"
	"context"
	"net/http"
	"time"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
)

func (client *Client) Account(ctx context.Context, credentials Credentials, kind codec.AccountKind,
	body []byte, model string, accept func(context.Context) error,
) ([]byte, error) {
	if err := client.selected(credentials); err != nil {
		return nil, err
	}
	payload, err := codec.AccountPayload(kind, body, model, credentials.Key)
	if err != nil {
		return nil, &Error{Operation: "account", Code: "invalid_params", Outcome: "not-started"}
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(bounded, http.MethodPost, client.server+kind.Path(), bytes.NewReader(payload))
	if err != nil || bounded.Err() != nil {
		return nil, &Error{Operation: "account", Code: "invalid_params", Outcome: "not-started", cause: bounded.Err()}
	}
	request.GetBody = nil
	request.Header.Set("Content-Type", "application/proto")
	request.Header.Set("Accept", "application/proto")
	request.Header.Set("Connect-Protocol-Version", "1")
	request.Header.Set("Authorization", "Basic "+credentials.Key+"-"+credentials.Key)
	outcome := "not-started"
	if kind == codec.ModelAssignment {
		if accept == nil {
			return nil, &Error{Operation: "account", Code: "invalid_params", Outcome: "not-started"}
		}
		outcome = "uncertain"
		if err := accept(bounded); err != nil {
			return nil, &Error{Operation: "account", Outcome: outcome, cause: err}
		}
	}
	response, err := client.http.Do(request)
	if err != nil {
		return nil, &Error{Operation: "account", Outcome: outcome, cause: bounded.Err()}
	}
	defer response.Body.Close()
	value, err := readBounded(response.Body, 1<<20)
	if err != nil {
		return nil, &Error{Operation: "account", Code: "data_loss", Outcome: outcome}
	}
	if response.StatusCode != http.StatusOK {
		return nil, remoteFailure("account", response.StatusCode, value, outcome)
	}
	if !media(response.Header, "application/proto") || bytes.Contains(value, []byte(credentials.Key)) ||
		codec.ValidateAccountResponse(kind, value) != nil {
		return nil, &Error{Operation: "account", Code: "data_loss", Outcome: outcome}
	}
	return value, nil
}
