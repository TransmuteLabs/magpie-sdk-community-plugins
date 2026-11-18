package devin

import (
	"context"
	"sync"

	codec "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-devin/codec"
	sdk "github.com/yetone/magpie/pkg/extension"
)

type geminiThoughtKey struct {
	account      string
	generation   uint64
	caller       string
	model        string
	conversation string
	reply        string
}

type geminiThoughtStore struct {
	mu      sync.Mutex
	entries map[geminiThoughtKey]codec.GeminiThought
	order   []geminiThoughtKey
	bytes   int
	closed  bool
}

func scopedGeminiThought(call sdk.Call, options sdk.ExecuteOptions, model, conversation, reply string) geminiThoughtKey {
	return geminiThoughtKey{account: call.Scope.Account, generation: call.Scope.Generation,
		caller: options.CallerID, model: model, conversation: conversation, reply: reply}
}

func (store *geminiThoughtStore) get(key geminiThoughtKey) (codec.GeminiThought, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return codec.GeminiThought{}, false
	}
	value, found := store.entries[key]
	return value, found
}

func (store *geminiThoughtStore) put(key geminiThoughtKey, value codec.GeminiThought) {
	if key.caller == "" || key.account == "" || key.generation == 0 || key.reply == "" ||
		len(value.Signature) > 2<<20 {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return
	}
	if store.entries == nil {
		store.entries = map[geminiThoughtKey]codec.GeminiThought{}
	}
	if old, found := store.entries[key]; found {
		store.bytes -= len(old.Signature)
		for index, prior := range store.order {
			if prior == key {
				store.order = append(store.order[:index], store.order[index+1:]...)
				break
			}
		}
	}
	store.entries[key] = value
	store.order = append(store.order, key)
	store.bytes += len(value.Signature)
	for len(store.order) > 256 || store.bytes > 8<<20 {
		oldest := store.order[0]
		store.order = store.order[1:]
		store.bytes -= len(store.entries[oldest].Signature)
		delete(store.entries, oldest)
	}
}

func (store *geminiThoughtStore) close() {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.entries, store.order, store.bytes = nil, nil, 0
	store.closed = true
}

func restoreGeminiThoughts(ctx context.Context, stream *sdk.Stream, call sdk.Call, options sdk.ExecuteOptions,
	state *providerState, request sdk.ExecuteRequest, encoded codec.EncodedChat) (codec.EncodedChat, error) {
	if !codec.IsGeminiModel(encoded.Model) || options.CallerID == "" {
		return encoded, nil
	}
	conversation, err := state.cacheIndex.key(options.CallerID, request.Protocol, request.Body, request.Headers)
	if err != nil {
		return codec.EncodedChat{}, err
	}
	restored, claims, err := codec.RestoreGeminiThoughts(encoded, func(reply string) (codec.GeminiThought, bool) {
		return state.geminiThoughts.get(scopedGeminiThought(call, options, encoded.Model, conversation, reply))
	})
	if err != nil {
		return codec.EncodedChat{}, err
	}
	if err := verifyOrigins(ctx, stream, call, options, encoded.Model, claims); err != nil {
		return codec.EncodedChat{}, err
	}
	return restored, nil
}

func persistGeminiThought(ctx context.Context, stream *sdk.Stream, call sdk.Call, options sdk.ExecuteOptions,
	state *providerState, request sdk.ExecuteRequest, reply *codec.GeminiReply, collector *originCollector) error {
	key, thought, err := reply.Thought(request.Model)
	if err != nil || options.CallerID == "" {
		return err
	}
	if err := collector.persist(ctx, stream, call, options, request.Model); err != nil {
		return err
	}
	if key == "" {
		return nil
	}
	origin, err := resolveMessagesSignature(ctx, stream, call, options, request.Model, thought.Signature)
	if err != nil {
		return err
	}
	if origin.Format != thought.Format || origin.Model != thought.Model {
		return originDenied()
	}
	conversation, err := state.cacheIndex.key(options.CallerID, request.Protocol, request.Body, request.Headers)
	if err != nil {
		return err
	}
	state.geminiThoughts.put(scopedGeminiThought(call, options, request.Model, conversation, key), thought)
	return nil
}
