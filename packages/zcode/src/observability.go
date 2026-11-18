package zcode

import (
	"context"
	"errors"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func diagnosticHandler(phase string, handler sdk.Handler) sdk.Handler {
	return func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
		started := time.Now()
		result, err := handler(ctx, call, stream)
		if ctx.Err() != nil {
			return result, err
		}
		diagnostic := sdk.Diagnostic{Level: "info", Code: phase + ".completed", Phase: phase,
			Millis: min(time.Since(started).Milliseconds(), int64(24*time.Hour/time.Millisecond))}
		if phase == "execute" {
			diagnostic.Code = "execution.completed"
		}
		if execution, valid := result.(sdk.ExecuteResult); valid {
			diagnostic.Status = execution.Status
			if execution.Status >= 400 {
				diagnostic.Level, diagnostic.Code = "warn", "execution.rejected"
			}
		}
		if err != nil {
			diagnostic.Level, diagnostic.Code = "warn", phase+".failed"
			var failure *sdk.Error
			if errors.As(err, &failure) && failure.Outcome == "uncertain" {
				diagnostic.Code = phase + ".uncertain"
			}
		}
		if usage, valid := result.(sdk.UsageResult); valid && usage.ReadDeferred {
			diagnostic.Level, diagnostic.Code = "debug", "usage.deferred"
		}
		observeCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		defer cancel()
		var receipt sdk.DiagnosticResult
		_ = stream.HostCall(observeCtx, sdk.MethodDiagnosticEmit, diagnostic, &receipt)
		return result, err
	}
}
