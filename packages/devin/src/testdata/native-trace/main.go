package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	devin "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin"
	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/extension/native"
)

func init() {
	module, err := devin.New()
	if err != nil {
		panic(err)
	}
	execute := module.Handlers[sdk.MethodProviderExecute]
	module.Handlers[sdk.MethodProviderExecute] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
		var request sdk.ExecuteRequest
		if err := json.Unmarshal(call.Params, &request); err != nil {
			return nil, err
		}
		var options sdk.ExecuteOptions
		_ = json.Unmarshal(request.Options, &options)
		fmt.Fprintln(os.Stderr, "SYNTHETIC TRACE execute", request.Protocol, options.Method, options.Path,
			"caller-present", options.CallerID != "", string(request.Body))
		sampler := time.AfterFunc(time.Second, func() {
			buffer := make([]byte, 2<<20)
			size := runtime.Stack(buffer, true)
			_, _ = os.Stderr.Write(buffer[:size])
		})
		defer sampler.Stop()
		result, err := execute(ctx, call, stream)
		fmt.Fprintln(os.Stderr, "SYNTHETIC TRACE returned", err)
		return result, err
	}
	if err := native.Register(module); err != nil {
		panic(err)
	}
}

func main() {}
