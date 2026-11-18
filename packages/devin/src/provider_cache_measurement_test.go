package devin

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	sdk "github.com/yetone/magpie/pkg/extension"
)

// Promote the independent SDK-peer probe: a completed request may have no
// measured usage, but that absence must not authorize more paid work.
func TestCacheTaskNeedsMeasuredUsageForSuccessor(t *testing.T) {
	dimension := func(key string, value float32) []byte {
		number := binary.LittleEndian.AppendUint32([]byte{2<<3 | 5}, math.Float32bits(value))
		metric := nativeFields(nativeText(4, string(number)), nativeText(5, key))
		group := nativeFields(nativeText(1, "Token Usage"), nativeText(2, string(metric)))
		return nativeText(28, string(group))
	}
	for _, scenario := range []struct {
		name          string
		usage         []byte
		wantSuccessor bool
	}{
		{name: "absent usage"},
		{name: "only unknown metric", usage: dimension("vendor_future_metric", 7)},
		{name: "only output zero", usage: dimension("output_tokens", 0)},
		{name: "complete measured hit", usage: nativeText(7, string(nativeFields(nativeNumber(2, 7),
			nativeNumber(3, 1), nativeNumber(4, 5), nativeNumber(5, 17)))), wantSuccessor: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if requests.Add(1) == 1 {
					cacheResponse(writer, true)
					return
				}
				writer.Header().Set("Content-Type", "application/connect+proto")
				writer.Write(nativeFrame(0, nativeFields(nativeText(3, "answer"), nativeNumber(5, 2), scenario.usage)))
				writer.Write(nativeFrame(2, []byte(`{}`)))
			}))
			defer server.Close()
			peer, host := cachePeer(t, server.URL, `{"keepAlive":true,"cacheKeepaliveInterval":30,"cacheKeepaliveMaxPings":6}`, ModuleOptions{})
			result, _ := peer.call(t, sdk.MethodProviderExecute, cacheExecute("measurement-boundary"))
			if result.Error != nil || len(host.scheduled) != 1 {
				t.Fatalf("foreground setup failed: result=%+v scheduled=%d", result, len(host.scheduled))
			}
			peer.scope.Operation = "paid-measurement-operation"
			result, _ = peer.call(t, sdk.MethodModuleCall, cacheTaskInput(host.scheduled[0]))
			var completed sdk.AccountTaskResult
			if result.Error != nil || json.Unmarshal(result.Result, &completed) != nil || completed.Outcome != "completed" || requests.Load() != 2 {
				t.Fatalf("paid response did not complete exactly once: result=%+v requests=%d", result, requests.Load())
			}
			wantSchedules := 1
			if scenario.wantSuccessor {
				wantSchedules = 2
			}
			t.Logf("requests=%d schedules=%d", requests.Load(), len(host.scheduled))
			if len(host.scheduled) != wantSchedules {
				t.Fatalf("unmeasured successful reply authorized another paid task: schedules=%d want=%d", len(host.scheduled), wantSchedules)
			}
		})
	}
}
