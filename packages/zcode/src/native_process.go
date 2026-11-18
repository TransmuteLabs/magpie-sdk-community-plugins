package zcode

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type nativeWorkspace struct {
	Path string `json:"workspacePath"`
	Key  string `json:"workspaceKey"`
}

func (workspace nativeWorkspace) valid() bool {
	return filepath.IsAbs(workspace.Path) && filepath.Clean(workspace.Path) == workspace.Path &&
		len(workspace.Path) <= 4096 && !strings.ContainsFunc(workspace.Path, unicode.IsControl) &&
		nativeToken(workspace.Key, 256)
}

func nativeToken(value string, limit int) bool {
	return value != "" && len(value) <= limit && strings.TrimSpace(value) == value &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

type nativeRunRequest struct {
	SessionID string `json:"sessionId"`
	CommandID string `json:"commandId"`
	Model     string `json:"model"`
	Text      string `json:"text"`
}

type nativeStopRequest struct {
	SessionID string `json:"sessionId"`
	CommandID string `json:"commandId"`
}

type nativeActive struct {
	account    string
	generation uint64
	command    string
	stop       chan struct{}
	stopping   bool
}

type nativeRunResult struct {
	Outcome   string `json:"outcome"`
	SessionID string `json:"sessionId"`
	CommandID string `json:"commandId"`
	State     string `json:"state"`
	Joined    bool   `json:"joined"`
}

func (owner *module) nativeRun(ctx context.Context, call sdk.Call, stream *sdk.Stream) (response any, runErr error) {
	var request nativeRunRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	if !nativeToken(request.SessionID, 256) || strings.HasPrefix(request.SessionID, "-") ||
		!nativeToken(request.CommandID, 256) || !nativeToken(request.Model, 1024) ||
		strings.TrimSpace(request.Text) == "" || len(request.Text) > 64<<10 {
		return nil, invalid("Native run requires a bounded session, command, gateway model and text")
	}
	if _, err := owner.identity(call); err != nil {
		return nil, err
	}
	state, err := currentInstance(ctx)
	if err != nil {
		return nil, err
	}
	if stream == nil || state.nativeWorkspace == nil || call.Scope.Operation == "" || call.Scope.Attempt == 0 {
		return nil, invalid("Native run requires its foreground operation and configured workspace")
	}
	store, err := state.controller()
	if err != nil {
		return nil, err
	}
	workspace := *state.nativeWorkspace
	lane := historyDigest([]string{workspace.Path, workspace.Key, request.SessionID})
	active := &nativeActive{account: call.Scope.Account, generation: call.Scope.Generation,
		command: request.CommandID, stop: make(chan struct{})}
	state.mu.Lock()
	if state.nativeRuns[lane] != nil {
		state.mu.Unlock()
		return nil, uncertain("A native command for this workspace and session is already active")
	}
	state.nativeRuns[lane] = active
	state.mu.Unlock()
	defer func() {
		state.mu.Lock()
		if state.nativeRuns[lane] == active {
			delete(state.nativeRuns, lane)
		}
		state.mu.Unlock()
	}()
	workspaceHash := historyDigest(workspace)
	key := controller.NativeKey(workspaceHash, request.SessionID, request.CommandID)
	principal := historyDigest("zcode-native-control-v1")
	entry := controller.NativeCommand{Account: call.Scope.Account, Generation: call.Scope.Generation,
		Operation: call.Scope.Operation, Workspace: workspaceHash, Session: request.SessionID,
		Command: request.CommandID, RequestHash: historyDigest(request), State: "pending"}
	if err := store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
		if transaction.Journal.Native == nil {
			transaction.Journal.Native = make(map[string]controller.NativeCommand)
		}
		if _, exists := transaction.Journal.Native[key]; exists {
			return uncertain("This native command already has a durable intent and will not be sent again")
		}
		for _, previous := range transaction.Journal.Native {
			if previous.Workspace == workspaceHash && previous.Session == request.SessionID && previous.State != "completed" {
				return uncertain("A previous native command remains unconfirmed; no new input was sent")
			}
		}
		transaction.Journal.Native[key] = entry
		return transaction.Save()
	}); err != nil {
		return nil, err
	}
	defer func() {
		if runErr != nil {
			response, runErr = nil, uncertain("Native command remains unconfirmed; no input was replayed")
		}
	}()
	if err := stream.Emit(ctx, "accepted", sdk.AcceptedEvent{OperationID: call.Scope.Operation}); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var opened sdk.ProcessOpenResult
	if err := stream.HostCall(ctx, sdk.MethodProcessOpen, sdk.ProcessOpenRequest{RecipeID: "native.run"}, &opened); err != nil {
		return nil, err
	}
	wire := newNativeWire(ctx, stream, opened, workspace, request)
	defer wire.close(ctx)
	if err := wire.subscribe(ctx); err != nil {
		return nil, err
	}
	ack, err := wire.command(ctx, request.CommandID, "sendText", map[string]any{
		"text": request.Text, "modelSelection": map[string]string{"providerId": "magpie", "modelId": request.Model},
	})
	if err != nil || !nativeAck(ack, request.CommandID) {
		return nil, errors.New("native input acknowledgment is unconfirmed")
	}
	stop := active.stop
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-stop:
			stop = nil
			if wire.execution == "" {
				if _, err := wire.query(ctx); err != nil {
					return nil, err
				}
			}
			if wire.execution == "" {
				return nil, errors.New("native stop has no current execution identity")
			}
			stopID := seededUUID("native-stop\x00" + request.CommandID)
			stopped, err := wire.command(ctx, stopID, "stop", map[string]string{
				"expectedForegroundExecutionId": wire.execution,
			})
			if err != nil || !nativeAck(stopped, stopID) {
				return nil, errors.New("native stop acknowledgment is unconfirmed")
			}
		default:
		}
		terminal, err := wire.query(ctx)
		if err != nil {
			return nil, err
		}
		if terminal != "" {
			if err := wire.close(ctx); err != nil {
				return nil, err
			}
			entry.State, entry.Terminal = "completed", terminal
			if err := store.Locked(ctx, principal, func(transaction *controller.Transaction) error {
				current, exists := transaction.Journal.Native[key]
				if !exists || current.Operation != call.Scope.Operation || current.Generation != call.Scope.Generation ||
					current.RequestHash != entry.RequestHash || current.State != "pending" {
					return errors.New("native command intent changed")
				}
				transaction.Journal.Native[key] = entry
				return transaction.Save()
			}); err != nil {
				return nil, err
			}
			return nativeRunResult{Outcome: "completed", SessionID: request.SessionID,
				CommandID: request.CommandID, State: terminal, Joined: true}, nil
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-stop:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (owner *module) nativeStop(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
	var request nativeStopRequest
	if err := decode(call.Params, &request); err != nil {
		return nil, err
	}
	if !nativeToken(request.SessionID, 256) || !nativeToken(request.CommandID, 256) {
		return nil, invalid("Native stop requires its exact active session and command")
	}
	if _, err := owner.identity(call); err != nil {
		return nil, err
	}
	state, err := currentInstance(ctx)
	if err != nil {
		return nil, err
	}
	if stream == nil || state.nativeWorkspace == nil {
		return nil, invalid("Native stop has no configured foreground owner")
	}
	workspace := *state.nativeWorkspace
	lane := historyDigest([]string{workspace.Path, workspace.Key, request.SessionID})
	state.mu.Lock()
	defer state.mu.Unlock()
	active := state.nativeRuns[lane]
	if active == nil || active.command != request.CommandID || active.account != call.Scope.Account ||
		active.generation != call.Scope.Generation {
		return nil, invalid("Native stop does not belong to this active account, session and command")
	}
	if !active.stopping {
		active.stopping = true
		close(active.stop)
	}
	return map[string]any{"outcome": "completed", "requested": true}, nil
}
