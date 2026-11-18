package devin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestProviderACUAsidePreservesZeroUnknownResetAndFreeScope(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		fields  string
		used    float64
		unknown bool
	}{
		{"counted", `"acuConsumed":1.5,`, 30, false},
		{"zero", `"acuConsumed":0,`, 0, false},
		{"missing", "", 0, true},
		{"over-limit", `"acuConsumed":9,`, 100, false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case outbound.ModelsPath:
					writer.Header().Set("Content-Type", "application/proto")
					writer.Write(nativeText(1, string(nativeFields(nativeText(22, "free-fixture"), nativeNumber(24, 4)))))
				case outbound.StatusPath:
					writer.Header().Set("Content-Type", "application/json")
					io.WriteString(writer, `{"userStatus":{"planStatus":{`+fixture.fields+
						`"acuLimit":5,"planEnd":"2030-01-01T00:00:00Z","dailyQuotaRemainingPercent":75}}}`)
				default:
					t.Error("quota read reached inference")
					writer.WriteHeader(500)
				}
			}))
			defer server.Close()
			peer, _ := providerPeer(t, server.URL)
			selectAccount(peer, server.URL)
			result, _ := peer.call(t, sdk.MethodProviderUsage, sdk.UsageRequest{Revision: "9"})
			var usage sdk.UsageResult
			if result.Error != nil || json.Unmarshal(result.Result, &usage) != nil || usage.Revision != "9" ||
				len(usage.Windows) != 2 {
				t.Fatal("native ACU reading disappeared", result)
			}
			quota, acu := usage.Windows[0], usage.Windows[1]
			if quota.Used != 25 || len(quota.NotModels) != 1 || quota.NotModels[0] != "free-fixture" ||
				acu.Name != "ACUs" || !acu.Aside || acu.Used != fixture.used || acu.Unknown != fixture.unknown ||
				acu.Limit != 100 || acu.ResetsAt != 1893456000000 || len(acu.NotModels)+len(acu.Models) != 0 {
				t.Fatal("ACU display became blocking quota, guessed zero or wrong units", usage)
			}
			if strings.Contains(string(result.Result), "USD") {
				t.Fatal("native ACU reading was currency converted")
			}
		})
	}
}
