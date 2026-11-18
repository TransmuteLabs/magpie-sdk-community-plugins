package zcode

import (
	"bytes"
	"context"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/protocol"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type startWaiter struct {
	ready   chan struct{}
	granted bool
}

type startQueue struct {
	active      int
	address     bool
	limit       int
	narrowUntil time.Time
	waiters     []*startWaiter
}

type startRouteHold struct {
	until time.Time
	hits  []time.Time
}

type startGuard struct {
	mu            sync.Mutex
	lanes         map[string]*startQueue
	routes        map[string]*startRouteHold
	overflowUntil time.Time
	now           func() time.Time
	wait          time.Duration
}

func (guard *startGuard) laneLimit(lane *startQueue) int {
	if lane.address {
		return 6
	}
	if lane.limit > 0 && guard.now().Before(lane.narrowUntil) {
		return lane.limit
	}
	return 3
}

func startAddressKey(route string) string {
	return "\x00address:" + route
}

func (guard *startGuard) full(key string) bool {
	lane := guard.lanes[key]
	return lane != nil && (lane.active >= guard.laneLimit(lane) || len(lane.waiters) > 0)
}

func (guard *startGuard) next(lane *startQueue) {
	for lane.active < guard.laneLimit(lane) && len(lane.waiters) > 0 {
		waiter := lane.waiters[0]
		lane.waiters = lane.waiters[1:]
		lane.active++
		waiter.granted = true
		close(waiter.ready)
	}
}

func (guard *startGuard) held(route string) bool {
	hold := guard.routes[route]
	return guard.now().Before(guard.overflowUntil) || hold != nil && guard.now().Before(hold.until)
}

func (guard *startGuard) routeHold(route string) *startRouteHold {
	if hold := guard.routes[route]; hold != nil {
		return hold
	}
	now := guard.now()
	for name, hold := range guard.routes {
		recent := len(hold.hits) > 0 && now.Sub(hold.hits[len(hold.hits)-1]) <= time.Minute
		if !now.Before(hold.until) && !recent {
			delete(guard.routes, name)
		}
	}
	if len(guard.routes) >= 4096 {
		guard.overflowUntil = now.Add(3 * time.Minute)
		return nil
	}
	hold := &startRouteHold{}
	guard.routes[route] = hold
	return hold
}

func startHeldError() error {
	return &sdk.Error{Code: "rate_limit_error",
		Message: "ZCode Start requests are held on this outbound route to avoid repeated concurrency or firewall refusals", Outcome: "not-started"}
}

func (guard *startGuard) queueWait() time.Duration {
	if guard.wait > 0 {
		return guard.wait
	}
	return 2 * time.Minute
}

func (guard *startGuard) acquire(ctx context.Context, key, route string) (func(), error) {
	return guard.waitFor(ctx, key, route, false, time.Now().Add(guard.queueWait()))
}

func (guard *startGuard) waitFor(ctx context.Context, key, route string, front bool, until time.Time) (func(), error) {
	account, err := guard.waitLane(ctx, key, route, front, until)
	if err != nil {
		return nil, err
	}
	address, err := guard.waitLane(ctx, startAddressKey(route), route, front, until)
	if err != nil {
		account()
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			address()
			account()
		})
	}, nil
}

func (guard *startGuard) waitLane(ctx context.Context, key, route string, front bool, until time.Time) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	timeout := &sdk.Error{Code: "rate_limit_error", Message: "ZCode Start queue timed out before dispatch", Outcome: "not-started"}
	if key == startAddressKey(route) {
		timeout.Message = "ZCode Start outbound route queue timed out before dispatch"
	}
	if !time.Now().Before(until) {
		return nil, timeout
	}
	guard.mu.Lock()
	if guard.held(route) {
		guard.mu.Unlock()
		return nil, startHeldError()
	}
	lane := guard.lanes[key]
	if lane == nil {
		for name, previous := range guard.lanes {
			if previous.active == 0 && len(previous.waiters) == 0 && !guard.now().Before(previous.narrowUntil) {
				delete(guard.lanes, name)
			}
		}
		if len(guard.lanes) >= 4096 {
			guard.mu.Unlock()
			return nil, &sdk.Error{Code: "rate_limit_error", Message: "Start account gate capacity is reached before dispatch", Outcome: "not-started"}
		}
		lane = &startQueue{address: key == startAddressKey(route)}
		guard.lanes[key] = lane
	}
	if len(lane.waiters) >= 4096 {
		guard.mu.Unlock()
		return nil, &sdk.Error{Code: "rate_limit_error", Message: "Start account queue is full before dispatch", Outcome: "not-started"}
	}
	waiter := &startWaiter{ready: make(chan struct{})}
	if front {
		lane.waiters = append([]*startWaiter{waiter}, lane.waiters...)
	} else {
		lane.waiters = append(lane.waiters, waiter)
	}
	guard.next(lane)
	guard.mu.Unlock()
	timer := time.NewTimer(max(0, time.Until(until)))
	defer timer.Stop()
	var waitErr error
	select {
	case <-waiter.ready:
	case <-ctx.Done():
		waitErr = ctx.Err()
	case <-timer.C:
		waitErr = timeout
	}
	guard.mu.Lock()
	if !time.Now().Before(until) {
		waitErr = timeout
	}
	if ctx.Err() != nil {
		waitErr = ctx.Err()
	}
	if waitErr == nil && guard.held(route) {
		waitErr = startHeldError()
	}
	if waitErr != nil {
		if waiter.granted {
			lane.active--
		} else {
			for index, current := range lane.waiters {
				if current == waiter {
					lane.waiters = append(lane.waiters[:index], lane.waiters[index+1:]...)
					break
				}
			}
		}
		guard.next(lane)
		guard.mu.Unlock()
		return nil, waitErr
	}
	guard.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			guard.mu.Lock()
			defer guard.mu.Unlock()
			lane.active--
			guard.next(lane)
			if lane.active == 0 && len(lane.waiters) == 0 && !guard.now().Before(lane.narrowUntil) {
				delete(guard.lanes, key)
			}
		})
	}, nil
}

func (guard *startGuard) sustained(key, route string) bool {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	now := guard.now()
	lane := guard.lanes[key]
	if lane != nil && lane.active > 1 {
		lane.limit, lane.narrowUntil = max(1, min(guard.laneLimit(lane), lane.active-1)), now.Add(5*time.Minute)
		return true
	}
	hold := guard.routeHold(route)
	if hold == nil {
		return false
	}
	if now.Before(hold.until) {
		return false
	}
	hits := []time.Time{}
	for _, at := range hold.hits {
		if now.Sub(at) <= time.Minute {
			hits = append(hits, at)
		}
	}
	hold.hits = append(hits, now)
	if len(hold.hits) >= 5 {
		hold.until, hold.hits = now.Add(3*time.Minute), nil
	}
	return false
}

type startBody struct {
	*cleanupBody
}

func (body *startBody) Read(destination []byte) (int, error) {
	count, err := body.ReadCloser.Read(destination)
	if err != nil {
		body.cleanup()
	}
	return count, err
}

func (owner *module) startDispatch(ctx context.Context, stream *sdk.Stream, account Credentials, outbound *http.Request, proxy string) (*http.Response, func(), error) {
	state, err := currentInstance(ctx)
	if err != nil {
		return nil, nil, err
	}
	key := controller.Principal(owner.zcodeOrigin(), account.UserID)
	route := controller.Principal(owner.zcodeOrigin(), proxy)
	release, err := state.startGuard.acquire(ctx, key, route)
	if err != nil {
		return nil, nil, err
	}
	var until time.Time
	requeue, busy := false, 0
	before := func() error {
		if requeue {
			release()
			release, err = state.startGuard.waitFor(ctx, key, route, true, until)
			if err != nil {
				return err
			}
			requeue, busy = false, 0
		}
		state.startGuard.mu.Lock()
		held := state.startGuard.held(route)
		state.startGuard.mu.Unlock()
		if held {
			return startHeldError()
		}
		return nil
	}
	observe := func(response *http.Response, failure sdk.RequestRecoveryRequest) error {
		media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if response.StatusCode >= 400 && media != "application/json" {
			data, err := io.ReadAll(io.LimitReader(response.Body, protocol.MaxBody+1))
			response.Body.Close()
			if err != nil || len(data) > protocol.MaxBody {
				return uncertain("Start refusal was incomplete; no retry was made")
			}
			response.Body = io.NopCloser(bytes.NewReader(data))
			if strings.Contains(strings.ToLower(string(data)), "<html") ||
				strings.Contains(strings.ToLower(string(data)), "errors.aliyun.com") {
				state.startGuard.mu.Lock()
				if hold := state.startGuard.routeHold(route); hold != nil {
					hold.until, hold.hits = state.startGuard.now().Add(3*time.Minute), nil
				}
				state.startGuard.mu.Unlock()
			}
		}
		if failure.Evidence == "rejected-before-effect" && response.StatusCode == 429 &&
			(failure.Facts.Code == "3008" || failure.Facts.Code == "3009" || failure.Facts.Code == "3010") {
			busy++
			if busy >= 2 && state.startGuard.sustained(key, route) {
				requeue = true
				if until.IsZero() {
					until = time.Now().Add(state.startGuard.queueWait())
				}
			}
		}
		return nil
	}
	response, err := owner.policyDispatch(ctx, stream, proxy, outbound, "start-messages", before, observe)
	if err != nil {
		if release != nil {
			release()
		}
		return nil, nil, err
	}
	response.Body = &startBody{&cleanupBody{ReadCloser: response.Body, cleanup: func() { release() }}}
	return response, func() { release() }, nil
}
