package policycontract_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type permitHost struct {
	result sdk.RequestRecoveryResult
}

func (host permitHost) HostCall(_ context.Context, _ string, _ any, out any) error {
	*out.(*sdk.RequestRecoveryResult) = host.result
	return nil
}

type permitTransport struct {
	requests int
}

func (transport *permitTransport) RoundTrip(*http.Request) (*http.Response, error) {
	transport.requests++
	status := 503
	if transport.requests > 1 {
		status = 200
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("{}"))}, nil
}

func TestMalformedNamedPermitCannotAuthorizeRepeat(test *testing.T) {
	for _, permit := range []sdk.RequestRecoveryResult{
		{Step: "named-read", Action: "terminal", Retry: true},
		{Step: "foreign-read", Action: "wait", Retry: true},
		{Step: "named-read", Retry: true},
	} {
		test.Run(permit.Step+"/"+permit.Action, func(test *testing.T) {
			request, err := http.NewRequest(http.MethodGet, "https://fixture.invalid/read", nil)
			if err != nil {
				test.Fatal(err)
			}
			transport := &permitTransport{}
			response, _, _ := sdk.RequestWithPolicy(context.Background(), permitHost{permit}, "named-read",
				request, transport, func(response *http.Response) (sdk.RequestRecoveryRequest, error) {
					if response.StatusCode == 200 {
						return sdk.RequestRecoveryRequest{}, nil
					}
					return sdk.RequestRecoveryRequest{
						Evidence: "read-only", Facts: sdk.RequestFailureFacts{
							Status: 503, Operation: sdk.MethodModuleCall, Phase: "response", Complete: true,
						},
					}, nil
				})
			if response != nil {
				response.Body.Close()
			}
			if transport.requests != 1 {
				test.Errorf("malformed public permit authorized %d dispatches: %+v", transport.requests, permit)
			}
		})
	}
}
