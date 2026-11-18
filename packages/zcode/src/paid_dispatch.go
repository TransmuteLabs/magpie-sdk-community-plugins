package zcode

import (
	"context"
	"net/http"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func (owner *module) paidDispatch(ctx context.Context, stream *sdk.Stream, proxy string, outbound *http.Request) (*http.Response, error) {
	return owner.policyDispatch(ctx, stream, proxy, outbound, "paid-messages", nil, nil)
}
