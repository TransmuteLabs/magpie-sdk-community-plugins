package devin

import (
	"context"
	"errors"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func emitDiagnostic(ctx context.Context, stream *sdk.Stream, value sdk.Diagnostic) {
	if stream == nil || ctx.Err() != nil {
		return
	}
	bounded, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	var result sdk.DiagnosticResult
	_ = stream.HostCall(bounded, sdk.MethodDiagnosticEmit, value, &result)
}

func finishDiagnostic(ctx context.Context, stream *sdk.Stream, started time.Time, code, phase string, status int, failure error) {
	value := sdk.Diagnostic{Level: "info", Code: code + ".completed", Phase: phase, Status: status,
		Millis: min(time.Since(started).Milliseconds(), 24*60*60*1000)}
	if failure != nil {
		value.Level, value.Code = "warn", code+".failed"
		if errors.Is(failure, context.Canceled) || ctx.Err() != nil {
			value.Code = code + ".cancelled"
		}
	}
	emitDiagnostic(ctx, stream, value)
}
