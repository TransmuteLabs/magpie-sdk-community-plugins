package devin

import (
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/outbound"
)

type browserCallbacks struct {
	mu     sync.Mutex
	closed bool
	flows  map[string]*browserCallback
}

type browserCallback struct {
	id       string
	mu       sync.Mutex
	pkce     outbound.PKCE
	callback string
	closed   bool
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
}

func newBrowserCallbacks() *browserCallbacks {
	return &browserCallbacks{flows: make(map[string]*browserCallback)}
}

func (callbacks *browserCallbacks) prepare(now func() time.Time) (*browserCallback, string, error) {
	identity, err := randomIdentity()
	if err != nil {
		return nil, "", err
	}
	flow := &browserCallback{id: identity, stop: make(chan struct{}), done: make(chan struct{})}
	callbacks.mu.Lock()
	if callbacks.closed || len(callbacks.flows) >= 16 {
		callbacks.mu.Unlock()
		return nil, "", errors.New("Devin browser callback capacity is unavailable")
	}
	callbacks.flows[identity] = flow
	callbacks.mu.Unlock()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		callbacks.finished(flow)
		return nil, "", err
	}
	pkce, address, err := outbound.PreparePKCE("http://"+listener.Addr().String()+"/callback", now())
	if err != nil {
		listener.Close()
		callbacks.finished(flow)
		return nil, "", err
	}
	flow.pkce = pkce
	var connections sync.WaitGroup
	server := &http.Server{
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			flow.receive(writer, request, now)
		}),
		ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second,
		WriteTimeout: 3 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 8192,
		ErrorLog: log.New(io.Discard, "", 0),
		ConnState: func(_ net.Conn, state http.ConnState) {
			if state == http.StateNew {
				connections.Add(1)
			} else if state == http.StateClosed || state == http.StateHijacked {
				connections.Done()
			}
		},
	}
	server.SetKeepAlivesEnabled(false)
	served := make(chan struct{})
	go func() {
		defer close(served)
		server.Serve(listener)
	}()
	go func() {
		timer := time.NewTimer(pkce.Deadline.Sub(now()))
		defer timer.Stop()
		select {
		case <-flow.stop:
		case <-timer.C:
		case <-served:
		}
		flow.mu.Lock()
		flow.closed = true
		flow.callback = ""
		flow.pkce = outbound.PKCE{}
		flow.mu.Unlock()
		server.Close()
		<-served
		connections.Wait()
		callbacks.finished(flow)
	}()
	return flow, address, nil
}

func (callbacks *browserCallbacks) finished(flow *browserCallback) {
	callbacks.mu.Lock()
	defer callbacks.mu.Unlock()
	close(flow.done)
	delete(callbacks.flows, flow.id)
}

func (flow *browserCallback) close() {
	flow.once.Do(func() { close(flow.stop) })
	<-flow.done
}

func (callbacks *browserCallbacks) remove(identity string) {
	callbacks.mu.Lock()
	flow := callbacks.flows[identity]
	callbacks.mu.Unlock()
	if flow != nil {
		flow.close()
	}
}

func (callbacks *browserCallbacks) close() {
	callbacks.mu.Lock()
	callbacks.closed = true
	flows := make([]*browserCallback, 0, len(callbacks.flows))
	for _, flow := range callbacks.flows {
		flows = append(flows, flow)
	}
	callbacks.mu.Unlock()
	for _, flow := range flows {
		flow.close()
	}
}

func (callbacks *browserCallbacks) pending(identity string, now time.Time) (string, error) {
	callbacks.mu.Lock()
	flow := callbacks.flows[identity]
	callbacks.mu.Unlock()
	if flow == nil {
		return "", errors.New("Devin browser callback instance ended")
	}
	flow.mu.Lock()
	defer flow.mu.Unlock()
	if flow.closed || !now.Before(flow.pkce.Deadline) || now.Before(flow.pkce.Deadline.Add(-10*time.Minute)) {
		return "", errors.New("Devin browser callback expired")
	}
	return flow.callback, nil
}

func (flow *browserCallback) receive(writer http.ResponseWriter, request *http.Request, now func() time.Time) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	flow.mu.Lock()
	if flow.closed {
		flow.mu.Unlock()
		writer.WriteHeader(http.StatusGone)
		return
	}
	status := http.StatusBadRequest
	if request.Method == http.MethodGet && request.Host == redirectHost(flow.pkce.Redirect) &&
		request.URL.Path == "/callback" && request.URL.RawPath == "" && !request.URL.IsAbs() &&
		len(request.RequestURI) <= 8192 && request.ContentLength == 0 && len(request.TransferEncoding) == 0 {
		callback := flow.pkce.Redirect + "?" + request.URL.RawQuery
		if _, err := flow.pkce.Callback(callback, now()); err == nil {
			if flow.callback != "" {
				status = http.StatusConflict
			} else {
				flow.callback = callback
				status = http.StatusOK
			}
		}
	}
	flow.mu.Unlock()
	writer.WriteHeader(status)
	if status == http.StatusOK {
		io.WriteString(writer, "Authorization received. Return to Magpie and check sign-in.")
	} else {
		io.WriteString(writer, "Authorization callback refused.")
	}
}

func redirectHost(redirect string) string {
	return redirect[len("http://") : len(redirect)-len("/callback")]
}
