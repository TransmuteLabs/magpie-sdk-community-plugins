import json
from pathlib import Path


def create_wrapper(target, sdk, module, kind, origin):
    target = Path(target)
    target.mkdir(parents=True, exist_ok=False)
    if kind == "zcode":
        import_path = "github.com/TransmuteLabs/magpie-sdk-plugin-protocol-zcode"
        provider, method = "zcode", "browser"
        endpoints = {
            "Zai": origin + "/zai", "BigModel": origin + "/bigmodel", "Start": origin + "/start",
            "ZaiQuota": origin + "/quota", "BigModelQuota": origin + "/quota",
            "LegacyBigModelQuota": origin + "/quota",
        }
    elif kind == "opencode":
        import_path = "github.com/TransmuteLabs/magpie-sdk-plugin-provider-opencode"
        provider, method = "opencode-go", "device"
        endpoints = {"Console": origin + "/console", "Inference": origin}
    else:
        raise ValueError("unsupported native consumer fixture")
    target.joinpath("go.mod").write_text(
        "module example.invalid/magpie-window-auth-consumer\n\ngo 1.26.8\n\n"
        f"require (\n github.com/yetone/magpie v0.0.0\n {import_path} v0.0.0\n)\n"
        f"replace github.com/yetone/magpie => {json.dumps(str(Path(sdk).resolve()))}\n"
        f"replace {import_path} => {json.dumps(str(Path(module).resolve()))}\n"
    )
    fields = ",".join(key + ":" + json.dumps(value) for key, value in endpoints.items())
    source = '''
package main

import (
 "context"
 "encoding/json"
 "os"
 "strconv"
 OPTIONAL_IMPORTS
 domain "DOMAIN_IMPORT"
 sdk "github.com/yetone/magpie/pkg/extension"
 "github.com/yetone/magpie/pkg/extension/native"
)

func fixture() sdk.Module {
 value := domain.NewModule(domain.ModuleOptions{Endpoints:domain.Endpoints{ENDPOINTS}})
 value.Manifest.Version = "0.0.0-auth-consumer"
 value.Manifest.Contract.Version = sdk.APIVersion{Major:1,Minor:5}
 value.Manifest.Compatibility = &sdk.Compatibility{HostAPI:"^1.5",NativeABI:1}
 value.Manifest.Window = &sdk.WindowDescriptor{Metadata:"package.json"}
 value.Manifest.AuthBindings = []sdk.WindowAuthBindingDescriptor{{
  ID:"signin",From:"window:page",Provider:"PROVIDER",
  Methods:[]sdk.WindowAuthMethod{{ID:"METHOD",Mode:"poll"}},Reauthorize:true,
 }}
 found := false
 for _, capability := range value.Manifest.Contract.Capabilities {
  if capability == sdk.ModuleRPC {found=true}
 }
 if !found {value.Manifest.Contract.Capabilities=append(value.Manifest.Contract.Capabilities,sdk.ModuleRPC)}
 value.Manifest.Actions=append(value.Manifest.Actions,sdk.ActionDescriptor{
  ID:"fixture.selected",InputSchema:json.RawMessage(`{"type":"object","additionalProperties":false}`),Effect:"read",
 })
 value.Manifest.Bindings=append(value.Manifest.Bindings,sdk.BindingDescriptor{
  ID:"selected",From:"window:page",Action:"fixture.selected",Provider:"PROVIDER",
 })
 if value.Actions == nil {value.Actions=map[string]sdk.Handler{}}
 usage:=value.Handlers[sdk.MethodProviderUsage]
 USAGE_GATE
 value.Actions["fixture.selected"]=func(ctx context.Context,call sdk.Call,stream *sdk.Stream)(any,error){
  var selected domain.Credentials
  if call.Scope.Account=="" || call.Scope.Generation==0 || call.Scope.Provider!="PROVIDER" ||
   json.Unmarshal(call.Auth,&selected)!=nil || SELECTED_ID!=call.Scope.Account {
   return nil,&sdk.Error{Code:"permission_denied",Message:"selected host account required"}
  }
  if _,err:=usage(ctx,call,stream);err!=nil{return nil,err}
  return map[string]any{"id":call.Scope.Account,"generation":strconv.FormatUint(call.Scope.Generation,10),"selectedAuthPresent":true},nil
 }
 return value
}

func init(){if err:=native.Register(fixture());err!=nil{panic(err)}}
func main(){if len(os.Args)==2 && os.Args[1]=="manifest"{if err:=json.NewEncoder(os.Stdout).Encode(fixture().Manifest);err!=nil{panic(err)}}}
'''
    source = source.replace("DOMAIN_IMPORT", import_path).replace("ENDPOINTS", fields)
    selected_id = "selected.AccountID()" if kind == "zcode" else "selected.AccountID(domain.ProviderGo)"
    source = source.replace("SELECTED_ID", selected_id)
    gate = '''
 var gates sync.Map
 value.Handlers[sdk.MethodProviderUsage]=func(ctx context.Context,call sdk.Call,stream *sdk.Stream)(any,error){
  if _,held:=gates.Load(call.Scope.Account);held{
   var request sdk.UsageRequest
   if err:=json.Unmarshal(call.Params,&request);err!=nil{return nil,err}
   return sdk.UsageResult{Revision:request.Revision,ReadDeferred:true},nil
  }
  return usage(ctx,call,stream)
 }
 for _, action:=range []string{"fixture.invalidate","fixture.allow-usage"} {
  value.Manifest.Actions=append(value.Manifest.Actions,sdk.ActionDescriptor{
   ID:action,InputSchema:json.RawMessage(`{"type":"object","additionalProperties":false}`),Effect:"write",
  })
  value.Manifest.Bindings=append(value.Manifest.Bindings,sdk.BindingDescriptor{
   ID:action,From:"window:page",Action:action,Provider:"zcode",
  })
 }
 value.Actions["fixture.invalidate"]=func(ctx context.Context,call sdk.Call,stream *sdk.Stream)(any,error){
  if err:=stream.Emit(ctx,"accepted",sdk.AcceptedEvent{OperationID:call.Scope.Operation});err!=nil{return nil,err}
  gates.Store(call.Scope.Account,true)
  var result sdk.UsageInvalidateResult
  if err:=stream.HostCall(ctx,sdk.MethodUsageInvalidate,sdk.UsageInvalidateRequest{},&result);err!=nil{return nil,err}
  return result,nil
 }
 value.Actions["fixture.allow-usage"]=func(ctx context.Context,call sdk.Call,stream *sdk.Stream)(any,error){
  if err:=stream.Emit(ctx,"accepted",sdk.AcceptedEvent{OperationID:call.Scope.Operation});err!=nil{return nil,err}
  gates.Delete(call.Scope.Account)
  return map[string]any{"released":true},nil
 }
''' if kind == "zcode" else ""
    source = source.replace("OPTIONAL_IMPORTS", '"sync"' if kind == "zcode" else "")
    source = source.replace("USAGE_GATE", gate)
    source = source.replace("PROVIDER", provider).replace("METHOD", method)
    target.joinpath("main.go").write_text(source)
    return {"provider": provider, "method": method}
