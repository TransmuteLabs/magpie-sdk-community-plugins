package zcode

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type vendorResponse struct {
	Status  int
	Headers http.Header
	Data    map[string]any
}

func (owner *module) vendorRequest(ctx context.Context, stream *sdk.Stream, method, endpoint string, headers http.Header, body any) (vendorResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	address, err := url.Parse(endpoint)
	if err != nil || address.Host == "" || address.User != nil || address.Fragment != "" ||
		address.Scheme != "https" && address.Scheme != "http" {
		return vendorResponse{}, invalid("Invalid vendor endpoint")
	}
	var proxy sdk.NetworkProxyResult
	if err := stream.HostCall(ctx, sdk.MethodNetworkProxy, sdk.NetworkProxyRequest{URL: endpoint}, &proxy); err != nil {
		return vendorResponse{}, err
	}
	var encoded []byte
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return vendorResponse{}, err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return vendorResponse{}, err
	}
	request.Header = headers.Clone()
	if request.Header == nil {
		request.Header = http.Header{}
	}
	request.Header.Set("User-Agent", "ZCode/"+ClientVersion)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := send(ctx, owner.options.HTTPFactory, proxy.URL, request)
	if err != nil {
		return vendorResponse{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return vendorResponse{}, &sdk.Error{Code: "upstream", Message: "Vendor discovery response is incomplete or too large"}
	}
	value, err := protocol.ReadObject(data)
	if err != nil {
		return vendorResponse{Status: response.StatusCode, Headers: response.Header.Clone()}, err
	}
	return vendorResponse{Status: response.StatusCode, Headers: response.Header.Clone(), Data: value}, nil
}

func responseValue(response vendorResponse) (any, error) {
	if response.Status < 200 || response.Status >= 300 || response.Data["success"] == false {
		return nil, &sdk.Error{Code: "upstream", Message: "Vendor request was refused"}
	}
	code, valid := finiteNumber(response.Data["code"])
	if !valid || code != 0 && code != 200 {
		return nil, &sdk.Error{Code: "upstream", Message: "Vendor response did not confirm success"}
	}
	value, present := response.Data["data"]
	if !present || value == nil {
		return nil, protocol.ErrInvalid
	}
	return value, nil
}

func responseData(response vendorResponse) (map[string]any, error) {
	value, err := responseValue(response)
	result, valid := value.(map[string]any)
	if err != nil || !valid {
		return nil, protocol.ErrInvalid
	}
	return result, nil
}
