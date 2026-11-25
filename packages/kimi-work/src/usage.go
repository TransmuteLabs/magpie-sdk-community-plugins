package kimiwork

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	sdk "github.com/yetone/magpie/pkg/extension"
)

func clampPercent(value any) (float64, bool) {
	ratio, ok := asFloat(value)
	if !ok {
		return 0, false
	}
	pct := ratio * 100
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return pct, true
}

func (owner *module) usage(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request sdk.UsageRequest
	if len(call.Params) > 0 && decodeObject(call.Params, &request) != nil {
		return nil, invalid("Invalid Kimi Work usage request")
	}
	rawCreds := call.Auth
	if !nonempty(rawCreds) {
		rawCreds = request.Credentials
	}
	var creds Credentials
	if decodeObject(rawCreds, &creds) != nil || !creds.Valid() {
		return nil, unauthenticated(expiredError("", false).Message)
	}
	paired := creds.HasPair()
	var token string
	if paired {
		token = creds.Access
	} else {
		session, err := owner.currentSession(creds)
		if err != nil {
			var deskErr *DesktopError
			if errors.As(err, &deskErr) && deskErr.SignIn == "kept" {
				return sdk.UsageResult{Error: deskErr.Message}, nil
			}
			return nil, err
		}
		if session.Away {
			return sdk.UsageResult{
				Error: "这个账号不是 Kimi 桌面版当前登录的账号，切换到它后才能读取会员额度（对话不受影响）",
			}, nil
		}
		token = session.WebToken
	}
	statsURL := strings.TrimRight(owner.options.Endpoints.Member, "/") + "/GetSubscriptionStats"
	payload, err := owner.jsonRequest(ctx, stream, statsURL, token, map[string]any{}, "")
	if err != nil {
		why := err.Error()
		var stErr *statusError
		if errors.As(err, &stErr) && stErr.Status == http.StatusUnauthorized {
			if paired {
				why = "Kimi 会员登录已过期，请重新连接 Kimi Work 以恢复额度读取（对话不受影响）"
			} else {
				why = "Kimi 网页登录已过期，打开一次 Kimi 桌面版即可刷新额度（对话不受影响）"
			}
		} else if errors.Is(err, context.DeadlineExceeded) {
			why = "读取 Kimi 会员额度超时"
		}
		return sdk.UsageResult{Error: why}, nil
	}
	balance := pickObject(payload, "subscriptionBalance", "subscription_balance")
	if balance == nil {
		return sdk.UsageResult{Error: "Kimi 会员额度回复里没有共享额度（subscriptionBalance）"}, nil
	}
	rawRatio := pickAny(balance, "amountUsedRatio", "amount_used_ratio")
	if rawRatio == nil {
		rawRatio = float64(0)
	}
	used, ok := clampPercent(rawRatio)
	if !ok {
		return sdk.UsageResult{Error: "Kimi 会员额度回复里没有共享额度（subscriptionBalance）"}, nil
	}
	window := sdk.UsageWindow{
		Name: "本月",
		Used: used,
	}
	if codeUsed, hasCode := clampPercent(pickAny(balance, "kimiCodeUsedRatio", "kimi_code_used_ratio")); hasCode {
		rounded := math.Round(codeUsed*100) / 100
		window.Display = fmt.Sprintf("Code 已用 %s%%（占总额）", strconv.FormatFloat(rounded, 'f', -1, 64))
	}
	return sdk.UsageResult{
		Windows: []sdk.UsageWindow{window},
	}, nil
}
