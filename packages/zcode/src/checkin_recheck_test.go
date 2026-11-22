package zcode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	sdk "github.com/yetone/magpie/pkg/extension"
)

func TestCheckinFindsLatePlanWithoutRepostingEarlierClaim(t *testing.T) {
	for _, scheduled := range []bool{false, true} {
		mode := "manual"
		if scheduled {
			mode = "scheduled"
		}
		t.Run(mode, func(t *testing.T) {
			start := time.Date(2026, 11, 22, 11, 0, 0, 0, time.UTC)
			var stage atomic.Int32
			now := func() time.Time {
				switch stage.Load() {
				case 3:
					return time.Date(2026, 11, 22, 15, 50, 0, 0, time.UTC)
				case 4:
					return time.Date(2026, 11, 22, 16, 1, 0, 0, time.UTC)
				}
				return start.Add(time.Duration(stage.Load()) * 30 * time.Minute)
			}
			var feeds, policies atomic.Int32
			var claimsMu sync.Mutex
			var claims []string
			const earlyDelivery = `{"zcode_plan":{"name":"Opening offer"},"action":{"type":"claim_zcode_plan","args":{"plan_id":"opening"}}}`
			const lateDelivery = `{"zcode_plan":{"name":"Late offer"},"action":{"type":"claim_zcode_plan","args":{"plan_id":"late"}}}`
			_, options := newCompletionServer(t, func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/v1/oauth/profile":
					profile(writer, request, "fixture-user")
				case "/api/v1/marketing/touch":
					feeds.Add(1)
					deliveries := []string{earlyDelivery, earlyDelivery + "," + lateDelivery, "", earlyDelivery + "," + lateDelivery, earlyDelivery}
					fmt.Fprintf(writer, `{"code":0,"data":{"deliveries":[%s]}}`, deliveries[stage.Load()])
				case "/api/v1/client/configs":
					policies.Add(1)
					io.WriteString(writer, `{"code":0,"data":{"configs":{"captcha":{"enabled":false}}}}`)
				case "/api/v1/zcode-plan/billing/balance":
					if stage.Load() == 0 {
						io.WriteString(writer, `{"code":0,"data":{"plans":[]}}`)
					} else {
						fmt.Fprintf(writer, `{"code":0,"data":{"plans":[{"plan_id":"opening","user_plan_id":"old-grant","status":"expired","ends_at":%d}]}}`, start.Add(15*time.Minute).Unix())
					}
				case "/api/v1/zcode-plan/billing/claim":
					var claim struct {
						Plan string `json:"plan_id"`
					}
					if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&claim) != nil {
						t.Error("claim lost its vendor method or plan")
					}
					claimsMu.Lock()
					claims = append(claims, claim.Plan)
					claimsMu.Unlock()
					if claim.Plan == "opening" {
						fmt.Fprintf(writer, `{"code":0,"data":{"plan":{"plan_id":"opening","ends_at":%d}}}`, start.Add(15*time.Minute).Unix())
					} else {
						io.WriteString(writer, `{"code":0}`)
					}
				default:
					t.Errorf("unexpected check-in request %s", request.URL.Path)
					writer.WriteHeader(http.StatusNotFound)
				}
			})

			config := moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")}
			options.Now = now
			account := nativeCredentials()
			account.SessionExpiresAt = start.Add(8 * time.Hour).UnixMilli()
			action := "billing.checkin"
			var input any = map[string]string{}
			callScope := scope(account)
			moduleFactory := NewModule
			if scheduled {
				descriptor, ok := ControlManifest().AccountCheckin(ProviderID)
				if !ok || !descriptor.Recheck {
					t.Fatal("ZCode does not opt into rechecks through the ordinary SDK scheduler")
				}
				action, input = descriptor.Action, sdk.AccountCheckinRequest{Day: "2026-11-22"}
				callScope.Session, callScope.Attempt = "", 1
				moduleFactory = NewControlModule
			}
			var outcomes []string
			var firstUntil time.Time
			for index := range 5 {
				stage.Store(int32(index))
				worker := newHarnessModule(t, options, config, moduleFactory)
				worker.next += 10 * index
				if scheduled && index == 4 {
					input = sdk.AccountCheckinRequest{Day: "2026-11-23"}
				}
				frame, _ := worker.call(sdk.MethodModuleCall, callScope, actionInput(action, input), raw(account))
				if frame.Error != nil {
					t.Fatalf("check-in %d failed: %+v", index, frame.Error)
				}
				if scheduled {
					result, err := sdk.DecodeAccountCheckinResult(frame.Result)
					if err != nil {
						t.Fatalf("check-in %d returned invalid SDK result: %v", index, err)
					}
					outcomes = append(outcomes, result.Outcome)
					if index == 0 {
						firstUntil, _ = time.Parse(time.RFC3339, result.Until)
					}
					if index == 3 {
						dayEnd := time.Date(2026, 11, 22, 16, 0, 0, 0, time.UTC)
						if until, err := time.Parse(time.RFC3339, result.Until); err != nil || !until.Equal(dayEnd) {
							t.Fatalf("scheduled result crossed the Beijing day: %s", result.Until)
						}
					}
				} else {
					var result struct {
						Outcome string `json:"outcome"`
						Until   int64  `json:"until"`
					}
					if err := json.Unmarshal(frame.Result, &result); err != nil {
						t.Fatal(err)
					}
					outcomes = append(outcomes, result.Outcome)
					if index == 0 {
						firstUntil = time.UnixMilli(result.Until)
					}
				}
			}
			claimsMu.Lock()
			defer claimsMu.Unlock()
			if !slices.Equal(claims, []string{"opening", "late", "opening"}) || feeds.Load() != 5 || policies.Load() != 3 {
				t.Fatalf("late and expired offers: claim POSTs %v, feeds %d, policy checks %d", claims, feeds.Load(), policies.Load())
			}
			if !slices.Equal(outcomes, []string{"claimed", "claimed", "done", "done", "claimed"}) || !firstUntil.Equal(start.Add(30*time.Minute)) {
				t.Fatalf("outcomes %v or first recheck %v lost the day's claim", outcomes, firstUntil)
			}
		})
	}
}

func TestScheduledCheckinSkipsAPIKeyAndRejectsAnotherDay(t *testing.T) {
	descriptor, ok := Manifest().AccountCheckin(ProviderID)
	if !ok {
		t.Fatal("missing ordinary check-in")
	}
	worker := newHarness(t, ModuleOptions{})
	account := fixtureCredentials("legacy-key")
	callScope := scope(account)
	callScope.Session, callScope.Attempt = "", 1
	frame, _ := worker.call(sdk.MethodModuleCall, callScope,
		actionInput(descriptor.Action, sdk.AccountCheckinRequest{Day: zcodeCheckinDay(fixtureNow)}), raw(account))
	result, err := sdk.DecodeAccountCheckinResult(frame.Result)
	if frame.Error != nil || err != nil || result.Outcome != "inactive" || worker.proxies != 0 || worker.accepted.Load() {
		t.Fatalf("API-key check-in acquired effects or required a session journal: %+v %s", frame.Error, frame.Result)
	}
	frame, _ = worker.call(sdk.MethodModuleCall, callScope,
		actionInput(descriptor.Action, sdk.AccountCheckinRequest{Day: zcodeCheckinDay(fixtureNow.Add(-24 * time.Hour))}), raw(account))
	if frame.Error == nil || frame.Error.Outcome != "not-started" || worker.proxies != 0 || worker.accepted.Load() {
		t.Fatalf("different day reached check-in: %+v %s", frame.Error, frame.Result)
	}
}

func TestScheduledCheckinRespectsClaimAnswersAcrossRestart(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		status    int
		vendor    string
		first     string
		second    string
		boundary  time.Duration
		wantPosts int32
		wantFeeds int32
	}{
		{"already-claimed", 200, `{"code":1003}`, "done", "done", checkinInterval, 1, 2},
		{"not-yet-eligible", 200, `{"code":1004}`, "ineligible", "claimed", checkinInterval, 2, 2},
		{"verification", 400, `{"code":3007}`, "captcha", "captcha", 0, 1, 1},
		{"rate-limit", 429, `{"code":3002}`, "rate-limited", "claimed", 45 * time.Minute, 2, 2},
		{"rate-limit-after-claim", 429, `{"code":3002}`, "rate-limited", "claimed", 45 * time.Minute, 3, 2},
		{"unknown-after-post", 503, `{}`, "uncertain", "uncertain", 0, 1, 2},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var posts, feeds, openingPosts atomic.Int32
			partial := scenario.name == "rate-limit-after-claim"
			_, options := newCompletionServer(t, func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/v1/oauth/profile":
					profile(writer, request, "fixture-user")
				case "/api/v1/marketing/touch":
					feeds.Add(1)
					opening := ""
					if partial {
						opening = `{"action":{"type":"claim_zcode_plan","args":{"plan_id":"opening"}}},`
					}
					fmt.Fprintf(writer, `{"code":0,"data":{"deliveries":[%s{"action":{"type":"claim_zcode_plan","args":{"plan_id":"gift"}}}]}}`, opening)
				case "/api/v1/client/configs":
					io.WriteString(writer, `{"code":0,"data":{"configs":{"captcha":{"enabled":false}}}}`)
				case "/api/v1/zcode-plan/billing/balance":
					io.WriteString(writer, `{"code":0,"data":{"plans":[]}}`)
				case "/api/v1/zcode-plan/billing/claim":
					var claim struct {
						Plan string `json:"plan_id"`
					}
					if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&claim) != nil {
						t.Error("check-in changed the claim method")
					}
					post := posts.Add(1)
					if partial && claim.Plan == "opening" {
						openingPosts.Add(1)
						io.WriteString(writer, `{"code":0,"data":{"plan":{"plan_id":"opening"}}}`)
						return
					}
					if claim.Plan != "gift" {
						t.Errorf("unexpected plan %q", claim.Plan)
					}
					if post > 1+openingPosts.Load() {
						io.WriteString(writer, `{"code":0}`)
						return
					}
					if scenario.status == 429 {
						writer.Header().Set("Retry-After", "2700")
					}
					writer.WriteHeader(scenario.status)
					io.WriteString(writer, scenario.vendor)
				default:
					t.Errorf("unexpected check-in request %s", request.URL.Path)
					writer.WriteHeader(http.StatusNotFound)
				}
			})
			start := time.Date(2026, 11, 22, 11, 0, 0, 0, time.UTC)
			if scenario.name == "rate-limit" {
				start = time.Date(2026, 11, 22, 15, 30, 0, 0, time.UTC)
			}
			var clock atomic.Int64
			options.Now = func() time.Time { return time.UnixMilli(clock.Load()) }
			config := moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")}
			account := nativeCredentials()
			account.SessionExpiresAt = start.Add(4 * time.Hour).UnixMilli()
			selected := scope(account)
			selected.Session, selected.Attempt = "", 1
			call := func(index int, at time.Time, want string) sdk.AccountCheckinResult {
				t.Helper()
				clock.Store(at.UnixMilli())
				worker := newHarnessModule(t, options, config, NewControlModule)
				worker.next += 10 * index
				frame, _ := worker.call(sdk.MethodModuleCall, selected,
					actionInput(accountCheckinAction, sdk.AccountCheckinRequest{Day: zcodeCheckinDay(at)}), raw(account))
				if want == "uncertain" {
					if frame.Error == nil || frame.Error.Outcome != want {
						t.Fatalf("claim uncertainty lost after %d calls: %+v %s", index, frame.Error, frame.Result)
					}
					return sdk.AccountCheckinResult{}
				}
				result, err := sdk.DecodeAccountCheckinResult(frame.Result)
				if frame.Error != nil || err != nil || result.Outcome != want {
					t.Fatalf("check-in %d wanted %s: %+v %s (%v)", index, want, frame.Error, frame.Result, err)
				}
				return result
			}
			first := call(0, start, scenario.first)
			delay := max(scenario.boundary, checkinInterval)
			if scenario.boundary > 0 {
				until, err := time.Parse(time.RFC3339, first.Until)
				if err != nil || !until.Equal(start.Add(scenario.boundary)) {
					t.Fatalf("lost confirmed retry deadline: %s", first.Until)
				}
				call(1, until.Add(-time.Second), scenario.first)
			} else if first.Until != "" {
				t.Fatalf("unresolved or captcha check-in acquired a retry deadline: %s", first.Until)
			}
			call(2, start.Add(delay), scenario.second)
			if posts.Load() != scenario.wantPosts || feeds.Load() != scenario.wantFeeds {
				t.Fatalf("claim POSTs %d, feeds %d; wanted %d/%d", posts.Load(), feeds.Load(), scenario.wantPosts, scenario.wantFeeds)
			}
			if partial && openingPosts.Load() != 1 {
				t.Fatalf("confirmed opening claim appeared in %d POSTs after the rate limit", openingPosts.Load())
			}
		})
	}
}

func TestScheduledCheckinHonorsFeedRetryAfterFromResponse(t *testing.T) {
	start := time.Date(2026, 11, 22, 11, 0, 0, 0, time.UTC)
	received := start.Add(4 * time.Second)
	boundary := received.Add(45 * time.Minute)
	for _, scenario := range []struct {
		name   string
		header string
	}{
		{"seconds", "2700"},
		{"date", boundary.Format(http.TimeFormat)},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var clock atomic.Int64
			var feeds atomic.Int32
			_, options := newCompletionServer(t, func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/v1/oauth/profile":
					profile(writer, request, "fixture-user")
				case "/api/v1/marketing/touch":
					if feeds.Add(1) == 1 {
						clock.Store(received.UnixMilli())
						writer.Header().Set("Retry-After", scenario.header)
						writer.WriteHeader(http.StatusTooManyRequests)
						io.WriteString(writer, `{"code":3002}`)
						return
					}
					io.WriteString(writer, `{"code":0,"data":{"deliveries":[]}}`)
				default:
					t.Errorf("unexpected check-in request %s", request.URL.Path)
					writer.WriteHeader(http.StatusNotFound)
				}
			})
			options.Now = func() time.Time { return time.UnixMilli(clock.Load()) }
			config := moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")}
			account := nativeCredentials()
			account.SessionExpiresAt = start.Add(2 * time.Hour).UnixMilli()
			selected := scope(account)
			selected.Session, selected.Attempt = "", 1
			call := func(index int, now time.Time, want string) sdk.AccountCheckinResult {
				t.Helper()
				clock.Store(now.UnixMilli())
				worker := newHarnessModule(t, options, config, NewControlModule)
				worker.next += 10 * index
				frame, _ := worker.call(sdk.MethodModuleCall, selected,
					actionInput(accountCheckinAction, sdk.AccountCheckinRequest{Day: zcodeCheckinDay(now)}), raw(account))
				result, err := sdk.DecodeAccountCheckinResult(frame.Result)
				if frame.Error != nil || err != nil || result.Outcome != want {
					t.Fatalf("check-in %d wanted %s: %+v %s (%v)", index, want, frame.Error, frame.Result, err)
				}
				return result
			}
			wantUntil := boundary.Format(time.RFC3339)
			if first := call(0, start, "rate-limited"); first.Until != wantUntil {
				t.Fatalf("Retry-After %s, want %s", first.Until, wantUntil)
			}
			if held := call(1, boundary.Add(-time.Millisecond), "rate-limited"); held.Until != wantUntil || feeds.Load() != 1 {
				t.Fatalf("hold changed before %s: %+v, %d feed reads", wantUntil, held, feeds.Load())
			}
			call(2, boundary, "inactive")
			if feeds.Load() != 2 {
				t.Fatalf("feed reads %d, want two", feeds.Load())
			}
		})
	}
}

func TestCheckinAndManualClaimKeepTheLatestPrincipalHold(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		feedHold   string
		claimHold  string
		firstUntil time.Duration
	}{
		{"late-short-feed", "300", "7200", 2 * time.Hour},
		{"late-long-feed", "7200", "300", 5 * time.Minute},
		{"late-successful-feed", "", "7200", 2 * time.Hour},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			start := time.Date(2026, 11, 22, 11, 0, 0, 0, time.UTC)
			boundary := start.Add(2 * time.Hour)
			var clock atomic.Int64
			clock.Store(start.UnixMilli())
			var posts, feeds atomic.Int32
			feedStarted, resumeFeed := make(chan struct{}), make(chan struct{})
			resume := sync.OnceFunc(func() { close(resumeFeed) })
			_, options := newCompletionServer(t, func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/api/v1/oauth/profile":
					profile(writer, request, "fixture-user")
				case "/api/v1/marketing/touch":
					if feeds.Add(1) == 1 {
						close(feedStarted)
						select {
						case <-resumeFeed:
						case <-request.Context().Done():
							return
						}
						if scenario.feedHold != "" {
							writer.Header().Set("Retry-After", scenario.feedHold)
							writer.WriteHeader(http.StatusTooManyRequests)
							io.WriteString(writer, `{"code":3002}`)
							return
						}
						io.WriteString(writer, `{"code":0,"data":{"deliveries":[]}}`)
						return
					}
					io.WriteString(writer, `{"code":0,"data":{"deliveries":[
						{"action":{"type":"claim_zcode_plan","args":{"plan_id":"confirmed"}}},
						{"action":{"type":"claim_zcode_plan","args":{"plan_id":"uncertain"}}}]}}`)
				case "/api/v1/client/configs":
					io.WriteString(writer, `{"code":0,"data":{"configs":{"captcha":{"enabled":false}}}}`)
				case "/api/v1/zcode-plan/billing/balance":
					io.WriteString(writer, `{"code":0,"data":{"plans":[]}}`)
				case "/api/v1/zcode-plan/billing/claim":
					var claim struct {
						Plan string `json:"plan_id"`
					}
					if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&claim) != nil {
						t.Error("expected a vendor claim POST")
					}
					posts.Add(1)
					switch claim.Plan {
					case "confirmed":
						io.WriteString(writer, `{"code":0,"data":{"plan":{"plan_id":"confirmed"}}}`)
					case "uncertain":
						writer.WriteHeader(http.StatusServiceUnavailable)
						io.WriteString(writer, `{}`)
					case "manual":
						writer.Header().Set("Retry-After", scenario.claimHold)
						writer.WriteHeader(http.StatusTooManyRequests)
						io.WriteString(writer, `{"code":3002}`)
					default:
						t.Errorf("unexpected claim plan %q", claim.Plan)
						writer.WriteHeader(http.StatusNotFound)
					}
				default:
					t.Errorf("unexpected check-in request %s", request.URL.Path)
					writer.WriteHeader(http.StatusNotFound)
				}
			})
			options.Now = func() time.Time { return time.UnixMilli(clock.Load()) }
			config := moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")}
			account := nativeCredentials()
			account.SessionExpiresAt = start.Add(4 * time.Hour).UnixMilli()
			selected := scope(account)
			selected.Session, selected.Attempt = "", 1
			manual := newHarnessConfigured(t, options, config)
			claim := func(worker *harness, plan string) sdk.Frame {
				frame, _ := worker.call(sdk.MethodModuleCall, selected,
					actionInput("billing.claim", map[string]string{"planId": plan}), raw(account))
				return frame
			}
			if frame := claim(manual, "confirmed"); frame.Error != nil || claimAnswer(frame.Result).Outcome != "confirmed" {
				t.Fatalf("initial confirmation failed: %+v %s", frame.Error, frame.Result)
			}
			if frame := claim(manual, "uncertain"); frame.Error == nil || frame.Error.Outcome != "uncertain" {
				t.Fatalf("initial write missed uncertainty: %+v %s", frame.Error, frame.Result)
			}
			checkin := func(t *testing.T, index int) sdk.Frame {
				t.Helper()
				worker := newHarnessModule(t, options, config, NewControlModule)
				worker.next += 10 * index
				frame, _ := worker.call(sdk.MethodModuleCall, selected,
					actionInput(accountCheckinAction, sdk.AccountCheckinRequest{Day: zcodeCheckinDay(options.Now())}), raw(account))
				return frame
			}
			var first sdk.Frame
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				t.Run("feed-worker", func(t *testing.T) { first = checkin(t, 1) })
			}()
			defer func() {
				resume()
				<-finished
			}()
			select {
			case <-feedStarted:
			case <-finished:
				t.Fatalf("check-in never reached the delayed feed: %+v %s", first.Error, first.Result)
			case <-time.After(8 * time.Second):
				t.Fatal("check-in did not reach the delayed feed")
			}
			if frame := claim(manual, "manual"); frame.Error != nil || claimAnswer(frame.Result).Until != start.Add(scenario.firstUntil).UnixMilli() {
				t.Fatalf("manual claim did not publish its hold: %+v %s", frame.Error, frame.Result)
			}
			resume()
			<-finished
			assertHold := func(frame sdk.Frame) {
				t.Helper()
				result, err := sdk.DecodeAccountCheckinResult(frame.Result)
				if frame.Error != nil || err != nil || result.Outcome != sdk.AccountCheckinRateLimited ||
					result.Until != boundary.Format(time.RFC3339) {
					t.Fatalf("expected shared pause through %s: %+v %s (%v)", boundary, frame.Error, frame.Result, err)
				}
			}
			assertHold(first)
			clock.Store(boundary.Add(-time.Second).UnixMilli())
			assertHold(checkin(t, 2))
			restarted := newHarnessConfigured(t, options, config)
			restarted.next += 30
			if frame := claim(restarted, "other"); frame.Error != nil || claimAnswer(frame.Result).Until != boundary.UnixMilli() ||
				claimAnswer(frame.Result).Outcome != "rate_limited" || feeds.Load() != 1 || posts.Load() != 3 {
				t.Fatalf("persisted principal hold disagreed with scheduler: %+v %s, %d feeds, %d claims", frame.Error, frame.Result, feeds.Load(), posts.Load())
			}
			clock.Store(boundary.UnixMilli())
			if frame := checkin(t, 4); frame.Error == nil || frame.Error.Outcome != "uncertain" || feeds.Load() != 2 || posts.Load() != 3 {
				t.Fatalf("expiry lost a claim fence: %+v %s, %d feeds, %d claims", frame.Error, frame.Result, feeds.Load(), posts.Load())
			}
		})
	}
}

func TestScheduledCheckinRetainsPendingLeaseAcrossBeijingDay(t *testing.T) {
	server, options := newCompletionServer(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/oauth/profile" {
			profile(writer, request, "fixture-user")
			return
		}
		t.Errorf("pending check-in reached %s", request.URL.Path)
		writer.WriteHeader(http.StatusNotFound)
	})
	now := time.Date(2026, 11, 22, 16, 0, 30, 0, time.UTC)
	options.Now = func() time.Time { return now }
	account := nativeCredentials()
	account.SessionExpiresAt = now.Add(time.Hour).UnixMilli()
	config := moduleConfig{DataDir: filepath.Join(t.TempDir(), "state")}
	store, err := controller.Open(config.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	principal := controller.Principal(server.URL, account.UserID)
	if err := store.Locked(t.Context(), principal, func(transaction *controller.Transaction) error {
		transaction.Journal.Checkin = &controller.Checkin{
			Operation: "previous-checkin", Pending: true, Until: now.Add(time.Minute).UnixMilli(),
		}
		return transaction.Save()
	}); err != nil {
		t.Fatal(err)
	}
	worker := newHarnessModule(t, options, config, NewControlModule)
	selected := scope(account)
	selected.Session, selected.Attempt = "", 1
	frame, _ := worker.call(sdk.MethodModuleCall, selected,
		actionInput(accountCheckinAction, sdk.AccountCheckinRequest{Day: zcodeCheckinDay(now)}), raw(account))
	if frame.Error == nil || frame.Error.Outcome != "uncertain" || worker.proxies != 1 || worker.accepted.Load() {
		t.Fatalf("new Beijing day bypassed an active check-in: %+v", frame.Error)
	}
}
