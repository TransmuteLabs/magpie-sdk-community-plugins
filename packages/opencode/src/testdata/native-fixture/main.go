package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	opencode "github.com/TransmuteLabs/magpie-sdk-plugin-provider-opencode"
	sdk "github.com/yetone/magpie/pkg/extension"
	"github.com/yetone/magpie/pkg/extension/native"
)

func init() {
	endpoint := os.Getenv("MAGPIE_OPENCODE_FIXTURE_URL")
	if endpoint == "" {
		panic("native fixture requires its synthetic endpoint")
	}
	plugin := opencode.NewModule(opencode.ModuleOptions{
		Endpoints: opencode.Endpoints{Console: endpoint + "/console", Inference: endpoint},
	})
	switch os.Getenv("MAGPIE_OPENCODE_FIXTURE_VARIANT") {
	case "agent":
		plugin = opencode.NewAgentModule(opencode.ModuleOptions{Endpoints: opencode.Endpoints{Console: endpoint + "/console", Inference: endpoint}})
	case "window":
		plugin = opencode.NewWindowModule(opencode.ModuleOptions{Endpoints: opencode.Endpoints{Console: endpoint + "/console", Inference: endpoint}})
	case "window-api8":
		plugin = opencode.NewAPI8WindowModule(opencode.ModuleOptions{Endpoints: opencode.Endpoints{Console: endpoint + "/console", Inference: endpoint}})
	}
	if events := os.Getenv("MAGPIE_OPENCODE_FIXTURE_EVENTS"); events != "" {
		for method, handler := range plugin.Handlers {
			plugin.Handlers[method] = func(ctx context.Context, call sdk.Call, stream *sdk.Stream) (any, error) {
				result, err := handler(ctx, call, stream)
				record := map[string]any{"method": method, "scope": call.Scope, "error": err != nil, "pid": os.Getpid()}
				if method == sdk.MethodAgentPlan || method == sdk.MethodAgentInspect {
					var request struct {
						Action  string `json:"action"`
						Options struct {
							Models         json.RawMessage `json:"models"`
							PriorModels    json.RawMessage `json:"priorModels"`
							DecisionModels json.RawMessage `json:"decisionModels"`
						} `json:"options"`
					}
					if json.Unmarshal(call.Params, &request) == nil {
						record["action"], record["models"] = request.Action, request.Options.Models
						record["priorModels"], record["decisionModels"] = request.Options.PriorModels, request.Options.DecisionModels
					}
				}
				var rpc *sdk.Error
				if errors.As(err, &rpc) {
					record["code"], record["outcome"], record["message"] = rpc.Code, rpc.Outcome, rpc.Message
				}
				file, openErr := os.OpenFile(events, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if openErr == nil {
					json.NewEncoder(file).Encode(record)
					file.Close()
				}
				return result, err
			}
		}
	}
	native.Register(plugin)
}

func main() {}
