import hashlib
import json


def verify(command, request, output, home, events, provider, format_name, declaration):
    model_id = provider + "/deepseek-v4.1-flash"
    value = "magpie/" + model_id
    path = home / ".config/magpie/extensions/opencode/agent.json"
    provider_key = "provider" if format_name == "v1" else "providers"
    current_api = declaration["contract"]["version"]["minor"]

    def model():
        return json.loads(path.read_text())[provider_key]["magpie"]["models"][model_id]

    def native_options():
        records = [json.loads(line) for line in events.read_text().splitlines()]
        candidates = [record for record in records if record["method"] == "agent.plan" and not record["error"]]
        assert candidates, "ordinary agent owner never executed a native plan"
        latest = candidates[-1]
        return latest, next(item for item in latest["models"] if item["value"] == value)

    def verify_current(context, effective_output, efforts):
        actual = model()
        record, option = native_options()
        assert option["context"] == context, ("host did not publish the ordinary context override", option)
        assert actual["limit"]["context"] == context, ("module replaced effective context", actual)
        if current_api >= 9:
            assert option["maxOutput"] == effective_output and option["efforts"] == efforts, option
            variants = actual["variants"]
            if format_name == "v1":
                names = sorted(variants)
                expected = sorted(efforts)
                assert all(variants[name]["reasoningEffort"] == name for name in names)
            else:
                names = [item["id"] for item in variants]
                expected = efforts
            assert names == expected, ("module lost effective effort list", actual)
            assert actual["limit"]["output"] == effective_output, ("module lost effective output", actual)
        else:
            for field in ("maxOutput", "efforts", "defaultEffort"):
                assert field not in option, "API8 unexpectedly received API9 metadata"
            assert record.get("priorModels") is None and record.get("decisionModels") is None
            assert actual["limit"]["output"] == 384000, "API8 compatibility lost the historical static output"
        return actual

    command("model", "spec", model_id, "--context", "32768", "--output", "4096", "--reasoning", "high")
    request("/api/set", {"Agent": "opencode-sdk", "Field": "model", "Value": value})
    request("/api/agents/connect/opencode-sdk", {})
    connected = verify_current(32768, 4096, ["high"])
    (output / "connected-config.json").write_text(path.read_text())
    request("/api/agents/reapply/opencode-sdk", {})
    first_hash = hashlib.sha256(path.read_bytes()).hexdigest()
    request("/api/agents/reapply/opencode-sdk", {})
    assert hashlib.sha256(path.read_bytes()).hexdigest() == first_hash, "unchanged ordinary sync rewrote config bytes"
    command("model", "spec", model_id, "--context", "65536", "--output", "8192", "--reasoning", "low,high")
    request("/api/agents/reapply/opencode-sdk", {})
    edited = verify_current(65536, 8192, ["low", "high"])
    (output / "host-edited-config.json").write_text(path.read_text())
    if current_api >= 9:
        record, unused = native_options()
        assert record.get("priorModels") is not None, "ordinary model edit omitted the prior-model envelope"
    current = json.loads(path.read_text())
    own = current[provider_key]["magpie"]["models"][model_id]
    own["limit"]["output"] = 7000
    current["userMarker"] = 9007199254740993
    if format_name == "v1":
        own["variants"]["personal"] = {"reasoningEffort": "low", "customSetting": "keep"}
    else:
        own["variants"].append({"id": "personal", "settings": {"reasoningEffort": "low", "customSetting": "keep"}})
    path.write_text(json.dumps(current))
    command("model", "spec", model_id, "--context", "98304", "--output", "16384", "--reasoning", "off")
    request("/api/agents/reapply/opencode-sdk", {})
    preserved = model()
    record, option = native_options()
    assert preserved["limit"]["context"] == 98304 and preserved["limit"]["output"] == 7000
    assert json.loads(path.read_text())["userMarker"] == 9007199254740993
    assert "personal" in json.dumps(preserved["variants"]) and "keep" in json.dumps(preserved["variants"])
    if current_api >= 9:
        assert option["efforts"] == [], "ordinary reasonless edit did not retain an explicit empty effort list"
        variants = preserved["variants"]
        names = list(variants) if format_name == "v1" else [item["id"] for item in variants]
        assert names == ["personal"], ("stale generated variants survived an empty effective list", names)
    final_hash = hashlib.sha256(path.read_bytes()).hexdigest()
    request("/api/agents/reapply/opencode-sdk", {})
    assert hashlib.sha256(path.read_bytes()).hexdigest() == final_hash, "user-preserving sync was not idempotent"
    (output / "user-preserved-config.json").write_text(path.read_text())
    catalog = json.loads((home / ".config/magpie/extensions/catalog.json").read_text())
    assert [item["manifest"]["id"] for item in catalog] == ["opencode"]
    assert not list(home.rglob("node_modules")), "metadata proof installed a community companion"
    return {
        "actualNative": True, "moduleVersion": declaration["version"], "minimumAPI": current_api,
        "format": format_name, "ordinaryConnect": True, "ordinarySync": True, "ordinaryModelEdits": True,
        "api8MetadataOff": current_api == 8, "priorModelsObserved": current_api >= 9,
        "localOutputPreserved": True, "customVariantsPreserved": True, "idempotentSync": True,
        "connectedLimit": connected["limit"], "hostEditedLimit": edited["limit"], "finalLimit": preserved["limit"],
        "configSHA256": final_hash, "installedModules": ["opencode"],
    }
