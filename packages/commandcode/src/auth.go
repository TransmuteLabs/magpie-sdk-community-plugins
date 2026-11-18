package commandcode

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sdk "github.com/yetone/magpie/pkg/extension"
)

type authFlow struct {
	mu        sync.Mutex
	state     string
	expires   time.Time
	key       string
	server    *http.Server
	done      chan struct{}
	stopTimer func() bool
	closed    bool
	consumed  bool
}

func (flow *authFlow) close() {
	flow.mu.Lock()
	flow.closed = true
	flow.key = ""
	server, done, stop := flow.server, flow.done, flow.stopTimer
	flow.mu.Unlock()
	if stop != nil {
		stop()
	}
	if server != nil {
		server.Close()
		<-done
	}
}

func validKey(key string) error {
	if key == "" || len(key) > 8192 || strings.ContainsAny(key, " \t\r\n\x00") {
		return invalid("CommandCode API key is empty or malformed")
	}
	return nil
}

func opaque() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func (controller *controller) addFlow(key string) (string, *authFlow, error) {
	handle, err := opaque()
	if err != nil {
		return "", nil, err
	}
	state, err := opaque()
	if err != nil {
		return "", nil, err
	}
	flow := &authFlow{key: key, state: state, expires: time.Now().Add(10 * time.Minute)}
	controller.mu.Lock()
	var expired []*authFlow
	defer func() {
		controller.mu.Unlock()
		// Closing a browser listener joins its server; never do that under
		// the controller lock needed by finish, cancellation and disposal.
		for _, old := range expired {
			old.close()
		}
	}()
	now := time.Now()
	for id, old := range controller.flows {
		old.mu.Lock()
		stale := !now.Before(old.expires)
		old.mu.Unlock()
		if stale {
			delete(controller.flows, id)
			expired = append(expired, old)
		}
	}
	if controller.closed || len(controller.flows) >= 32 {
		return "", nil, &sdk.Error{Code: "resource_exhausted", Message: "Too many unfinished CommandCode sign-ins"}
	}
	controller.flows[handle] = flow
	return handle, flow, nil
}

func authStart(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	controller, err := owner(ctx)
	if err != nil {
		return nil, err
	}
	var input sdk.AuthStartRequest
	if decode(call.Params, &input) != nil {
		return nil, invalid("invalid CommandCode sign-in request")
	}
	var key string
	switch input.Method {
	case "api-key":
		var fields struct {
			Key string `json:"apiKey"`
		}
		if decode(input.Input, &fields) != nil {
			return nil, invalid("API-key sign-in requires apiKey")
		}
		key = fields.Key
		if err := validKey(key); err != nil {
			return nil, err
		}
	case "cli-import":
		if len(input.Input) > 0 && string(input.Input) != "{}" && string(input.Input) != "null" {
			return nil, invalid("CLI import uses only the fixed CommandCode credential file")
		}
		key, err = importCLIKey()
		if err != nil {
			return nil, err
		}
	case "browser":
	default:
		return nil, invalid("Choose api-key, cli-import or browser sign-in")
	}
	handle, flow, err := controller.addFlow(key)
	if err != nil {
		return nil, err
	}
	start := sdk.AuthStartResult{Continuation: json.RawMessage(`{"flow":"` + handle + `"}`), PollAfterMs: 1000}
	if input.Method != "browser" {
		return start, nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		controller.removeFlow(handle, flow)
		return nil, &sdk.Error{Code: "unavailable", Message: "CommandCode browser callback could not start"}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", flow.callback)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 16 << 10}
	flow.mu.Lock()
	flow.server = server
	flow.done = make(chan struct{})
	done := flow.done
	flow.mu.Unlock()
	go func() { defer close(done); server.Serve(listener) }()
	stop := context.AfterFunc(controller.ctx, func() { flow.close() })
	timer := time.AfterFunc(time.Until(flow.expires), func() { controller.removeFlow(handle, flow) })
	flow.mu.Lock()
	flow.stopTimer = func() bool {
		stoppedParent, stoppedTimer := stop(), timer.Stop()
		return stoppedParent || stoppedTimer
	}
	flow.mu.Unlock()
	studio, _ := url.Parse(strings.TrimRight(controller.config.StudioURL, "/") + "/studio/auth/cli")
	query := studio.Query()
	query.Set("callback", "http://"+listener.Addr().String()+"/callback")
	query.Set("state", flow.state)
	query.Set("mode", "redirect")
	studio.RawQuery = query.Encode()
	start.URL = studio.String()
	return start, nil
}

func (controller *controller) removeFlow(handle string, flow *authFlow) {
	controller.mu.Lock()
	if controller.flows[handle] == flow {
		delete(controller.flows, handle)
	}
	controller.mu.Unlock()
	flow.close()
}

func (flow *authFlow) callback(writer http.ResponseWriter, request *http.Request) {
	origin := request.Header.Get("Origin")
	if origin != "" && origin != "https://commandcode.ai" && origin != "https://staging.commandcode.ai" {
		http.Error(writer, "Origin is not allowed", http.StatusForbidden)
		return
	}
	if origin != "" {
		writer.Header().Set("Access-Control-Allow-Origin", origin)
		writer.Header().Set("Vary", "Origin")
		writer.Header().Set("Access-Control-Allow-Private-Network", "true")
	}
	if request.Method == http.MethodOptions {
		writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		writer.Header().Set("Access-Control-Allow-Headers", "content-type")
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodPost {
		http.Error(writer, "POST is required", 405)
		return
	}
	body, err := bounded(request.Body, 10<<10)
	if err != nil {
		http.Error(writer, "Callback is oversized", 413)
		return
	}
	var fields struct {
		State string `json:"state"`
		Key   string `json:"apiKey"`
	}
	if strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
		err = decode(body, &fields)
	} else {
		form, formErr := url.ParseQuery(string(body))
		err = formErr
		fields.State, fields.Key = form.Get("state"), form.Get("apiKey")
	}
	if err != nil || validKey(fields.Key) != nil || fields.State == "" || subtle.ConstantTimeCompare([]byte(fields.State), []byte(flow.state)) != 1 {
		http.Error(writer, "Callback state or credential is invalid", 403)
		return
	}
	flow.mu.Lock()
	defer flow.mu.Unlock()
	if flow.closed || flow.consumed || flow.key != "" || time.Now().After(flow.expires) {
		http.Error(writer, "Sign-in is expired or already completed", 409)
		return
	}
	flow.key = fields.Key
	writer.Header().Set("Content-Type", "application/json")
	io.WriteString(writer, `{"success":true}`)
}

func authFinish(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	controller, err := owner(ctx)
	if err != nil {
		return nil, err
	}
	var input sdk.AuthFinishRequest
	if decode(call.Params, &input) != nil {
		return nil, invalid("invalid CommandCode sign-in completion")
	}
	var continuation struct {
		Flow string `json:"flow"`
	}
	if decode(input.Continuation, &continuation) != nil || continuation.Flow == "" {
		return nil, invalid("CommandCode sign-in continuation is required")
	}
	controller.mu.Lock()
	flow := controller.flows[continuation.Flow]
	controller.mu.Unlock()
	if flow == nil {
		return nil, &sdk.Error{Code: "permission_denied", Message: "CommandCode sign-in belongs to another or expired component"}
	}
	flow.mu.Lock()
	if flow.closed || flow.consumed || time.Now().After(flow.expires) {
		flow.mu.Unlock()
		controller.removeFlow(continuation.Flow, flow)
		return nil, &sdk.Error{Code: "permission_denied", Message: "CommandCode sign-in is expired or consumed"}
	}
	key := flow.key
	if key == "" {
		flow.mu.Unlock()
		return sdk.AuthResult{Pending: true}, nil
	}
	flow.consumed = true
	flow.mu.Unlock()
	result, err := controller.validateKey(ctx, stream, key)
	if err != nil {
		flow.mu.Lock()
		if !flow.closed {
			flow.consumed = false
		}
		flow.mu.Unlock()
		return nil, err
	}
	controller.removeFlow(continuation.Flow, flow)
	return result, nil
}

func (controller *controller) validateKey(ctx context.Context, stream *sdk.Stream, key string) (sdk.AuthResult, error) {
	body, err := controller.read(ctx, stream, "/alpha/whoami", key)
	if err != nil {
		return sdk.AuthResult{}, &sdk.Error{Code: "unavailable", Message: "CommandCode could not verify this key; sign-in has not been completed"}
	}
	var identity struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
		User  struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			UserName string `json:"userName"`
			Email    string `json:"email"`
		} `json:"user"`
	}
	if decode(body, &identity) != nil {
		return sdk.AuthResult{}, &sdk.Error{Code: "upstream_error", Message: "CommandCode returned an unreadable account identity"}
	}
	if identity.ID == "" {
		identity.ID = identity.User.ID
	}
	if identity.Name == "" {
		identity.Name = identity.User.Name
	}
	if identity.Name == "" {
		identity.Name = identity.User.UserName
	}
	if identity.Name == "" {
		identity.Name = identity.User.Email
	}
	if identity.Name == "" {
		identity.Name = identity.Email
	}
	if identity.ID == "" {
		return sdk.AuthResult{}, &sdk.Error{Code: "upstream_error", Message: "CommandCode did not return an account identity"}
	}
	hash := sha256.Sum256([]byte(key))
	accountID := "key-" + hex.EncodeToString(hash[:16])
	encoded, _ := json.Marshal(credentials{Key: key, UserID: identity.ID, Name: identity.Name})
	return sdk.AuthResult{ID: accountID, Name: identity.Name, Credentials: encoded}, nil
}

func authRefresh(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	controller, err := owner(ctx)
	if err != nil {
		return nil, err
	}
	auth, err := account(call)
	if err != nil {
		return nil, err
	}
	return controller.validateKey(ctx, stream, auth.Key)
}

func authLogout(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	if _, err := owner(ctx); err != nil {
		return nil, err
	}
	return sdk.AuthResult{}, nil
}

func (controller *controller) keyRefused(ctx context.Context, stream *sdk.Stream, key string) bool {
	target := strings.TrimRight(controller.config.APIURL, "/") + "/alpha/whoami"
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	proxy, err := controller.proxy(ctx, stream, target)
	if err != nil {
		return false
	}
	httpClient, closeClient, err := client(proxy)
	if err != nil {
		return false
	}
	defer closeClient()
	request, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return false
	}
	request.Header.Set("Authorization", "Bearer "+key)
	response, err := httpClient.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == 401 || response.StatusCode == 403
}

func importCLIKey() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", invalid("CommandCode CLI credential home is unavailable")
	}
	root, err := os.OpenRoot(filepath.Join(home, ".commandcode"))
	if err != nil {
		return "", invalid("CommandCode CLI sign-in was not found")
	}
	defer root.Close()
	file, err := root.Open("auth.json")
	if err != nil {
		return "", invalid("CommandCode CLI sign-in could not be read")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", invalid("CommandCode CLI sign-in must be a regular file")
	}
	body, err := bounded(file, 64<<10)
	if err != nil {
		return "", invalid("CommandCode CLI sign-in is oversized")
	}
	key, err := decodeCLIKey(body)
	if err != nil {
		return "", invalid("CommandCode CLI sign-in has no supported key")
	}
	return key, nil
}

func decodeCLIKey(body []byte) (string, error) {
	var fields struct {
		APIKey string `json:"apiKey"`
	}
	if decode(body, &fields) != nil || validKey(fields.APIKey) != nil {
		return "", errors.New("CLI auth has no valid apiKey")
	}
	return fields.APIKey, nil
}
