package zcode

import (
	"context"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode/controller"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type moduleConfig struct {
	DataDir         string           `json:"dataDir,omitempty"`
	FreeFirst       *bool            `json:"freeFirst,omitempty"`
	NativeWorkspace *nativeWorkspace `json:"nativeWorkspace,omitempty"`
}

type instance struct {
	store           *controller.Store
	lifetime        context.Context
	dataDir         string
	mu              sync.Mutex
	closed          bool
	freeFirst       bool
	startGuard      *startGuard
	giftRest        map[string]time.Time
	giftHints       map[string]giftHint
	nativeWorkspace *nativeWorkspace
	nativeRuns      map[string]*nativeActive
}

func (owner *module) start(ctx context.Context, state sdk.ModuleContext) (sdk.ModuleReady, func(), error) {
	if err := owner.options.Endpoints.validate(); err != nil {
		return sdk.ModuleReady{}, nil, invalid("Invalid ZCode transport endpoints")
	}
	var config moduleConfig
	if err := decode(state.Config.Values, &config); err != nil {
		return sdk.ModuleReady{}, nil, err
	}
	if config.NativeWorkspace != nil && !config.NativeWorkspace.valid() {
		return sdk.ModuleReady{}, nil, invalid("nativeWorkspace requires an explicit absolute clean path and workspace key")
	}
	value := &instance{lifetime: ctx, dataDir: config.DataDir, freeFirst: config.FreeFirst == nil || *config.FreeFirst}
	value.nativeWorkspace = config.NativeWorkspace
	value.nativeRuns = make(map[string]*nativeActive)
	value.startGuard = &startGuard{lanes: map[string]*startQueue{}, routes: map[string]*startRouteHold{}, now: owner.options.Now}
	value.giftRest = map[string]time.Time{}
	value.giftHints = map[string]giftHint{}
	if config.DataDir != "" {
		if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
			return sdk.ModuleReady{}, nil, unsupported("Durable ZCode controller locking is implemented for Linux and macOS")
		}
		if !filepath.IsAbs(config.DataDir) || filepath.Clean(config.DataDir) != config.DataDir {
			return sdk.ModuleReady{}, nil, invalid("dataDir must be an absolute clean directory path")
		}
	}
	return sdk.ModuleReady{State: value}, func() {
		value.mu.Lock()
		defer value.mu.Unlock()
		value.closed = true
		if value.store != nil {
			value.store.Close()
		}
	}, nil
}

func (value *instance) controller() (*controller.Store, error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	if value.closed || value.lifetime.Err() != nil || value.dataDir == "" {
		return nil, &sdk.Error{Code: "not_ready", Message: "Configure an explicit private dataDir before using native controls", Outcome: "not-started"}
	}
	if value.store == nil {
		store, err := controller.Open(value.dataDir)
		if err != nil {
			return nil, &sdk.Error{Code: "not_ready", Message: "The configured controller dataDir is not private or available", Outcome: "not-started"}
		}
		value.store = store
	}
	return value.store, nil
}

func currentInstance(ctx context.Context) (*instance, error) {
	state, found := sdk.ModuleContextFrom(ctx)
	value, valid := state.State.(*instance)
	if !found || !valid || value.lifetime.Err() != nil {
		return nil, &sdk.Error{Code: "not_ready", Message: "ZCode module instance is not active", Outcome: "not-started"}
	}
	return value, nil
}
