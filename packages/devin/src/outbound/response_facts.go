package outbound

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	sdk "github.com/yetone/magpie/pkg/extension"
)

var freeReset = regexp.MustCompile(`(?i)(?:^|[. ]+)your limit will reset in ([0-9]+) (seconds?|minutes?|hours?)[.]?$`)

func responseFacts(status int, failure *codec.ResponseFailure, headers ...http.Header) *sdk.RequestFailureFacts {
	facts := &sdk.RequestFailureFacts{Status: status}
	if failure != nil {
		facts.Code, facts.Type, facts.Message, facts.Complete = failure.Code, failure.Type, failure.Message, failure.Complete
		if failure.Complete && failure.Code == "resource_exhausted" && strings.HasPrefix(strings.ToLower(failure.Message), "reached free model rate limit") {
			match := freeReset.FindStringSubmatch(failure.Message)
			if strings.Contains(strings.ToLower(failure.Message), "your limit will reset in") && match == nil {
				facts.Complete = false
			}
			if match != nil {
				count, err := strconv.ParseInt(match[1], 10, 64)
				unit := int64(1000)
				switch strings.ToLower(match[2]) {
				case "minute", "minutes":
					unit *= 60
				case "hour", "hours":
					unit *= 3600
				}
				if err == nil && count <= 30*24*3600000/unit {
					delay := count * unit
					facts.RetryAfterMS = &delay
				} else {
					facts.Complete = false
				}
			}
		}
	}
	if len(headers) == 1 {
		for _, name := range []string{"Retry-After", "X-Ratelimit-Reset", "Ratelimit-Reset", "X-Ratelimit-Reset-Requests", "X-Ratelimit-Reset-Tokens"} {
			values := headers[0].Values(name)
			if len(values) == 0 {
				continue
			}
			if len(values) != 1 || len(values[0]) > 128 || !utf8.ValidString(values[0]) || strings.IndexFunc(values[0], unicode.IsControl) >= 0 {
				facts.Complete = false
				continue
			}
			if facts.Headers == nil {
				facts.Headers = map[string]string{}
			}
			facts.Headers[name] = values[0]
		}
	}
	return facts
}
