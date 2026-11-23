#!/usr/bin/env bash
set -euo pipefail

kit="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
[[ "$(uname -s -m)" == 'Linux x86_64' && "$(id -u)" == 1000 ]]

if [[ "${1:-}" != --isolated ]]; then
    [[ "$#" == 3 && "$3" == /* && ! -e "$3" ]]
    host="$(cd "$1" && pwd -P)"
    sdk="$(cd "$2" && pwd -P)"
    mkdir "$3"
    receipt="$(cd "$3" && pwd -P)"
    toolchain=/tmp/magpie-sdk-v11-usbox-niq306rt
    goroot="$toolchain/go-toolchain-1.26.8"
    [[ -x "$goroot/bin/go" && -x "$goroot/bin/gofmt" ]]
    mapfile -t caches < <(python3 -IB - "$toolchain/go-env-external-toolchain.json" "$goroot" <<'PY'
import json
from pathlib import Path
import sys

metadata = json.loads(Path(sys.argv[1]).read_text())
assert metadata["GOROOT"] == sys.argv[2]
for key in ("GOPATH", "GOMODCACHE", "GOCACHE"):
    path = Path(metadata[key])
    assert path.is_absolute() and path.is_dir() and "\n" not in str(path)
    print(path)
PY
    )
    [[ "${#caches[@]}" == 3 ]]
    ps -eo stat=,comm= | awk '$1 !~ /^Z/ && $2 ~ /(^|\/)(go|gofmt|compile|link|asm|[[:alnum:]_-]+\.test)$/ {print}' > "$receipt/go-census.log"
    [[ ! -s "$receipt/go-census.log" ]]
    date -u +%FT%T.%NZ > "$receipt/started.utc"
    status=0
    env -i PATH="$goroot/bin:/usr/local/bin:/usr/bin:/usr/sbin:/bin:/sbin" \
        GOROOT="$goroot" GOPATH="${caches[0]}" GOMODCACHE="${caches[1]}" GOCACHE="${caches[2]}" LANG=C LC_ALL=C \
        timeout --kill-after=5s 300s bwrap --unshare-net --unshare-pid --die-with-parent \
        --cap-drop ALL --ro-bind / / --bind "$receipt" "$receipt" --bind "${caches[2]}" "${caches[2]}" \
        --dev /dev --proc /proc /bin/bash "$kit/run-linux.sh" --isolated "$host" "$sdk" "$receipt" \
        > "$receipt/runner.log" 2>&1 || status=$?
    printf '%s\n' "$status" > "$receipt/suite.exit"
    date -u +%FT%T.%NZ > "$receipt/finished.utc"
    exit "$status"
fi

[[ "$#" == 4 && "$(ip -o link show | wc -l)" -eq 1 ]]
ip -o link show dev lo up | grep -q 'LOOPBACK'
[[ -z "$(ip route show default)" && -z "$(ip -6 route show default)" ]]
awk '/^Cap(Inh|Prm|Eff|Bnd|Amb):/ { count++; if ($2 !~ /^0+$/) exit 1 } END { if (count != 5) exit 1 }' /proc/self/status
host="$2"
sdk="$3"
receipt="$4"
scratch="$(mktemp -d "$receipt/.scratch.XXXXXXXX")"
trap 'rm -rf -- "$scratch"' EXIT
mkdir "$scratch/home" "$scratch/tmp" "$scratch/magpie" "$scratch/sdk"
export HOME="$scratch/home" USERPROFILE="$scratch/home" TMPDIR="$scratch/tmp"
export XDG_CONFIG_HOME="$HOME/config" XDG_CACHE_HOME="$HOME/cache" XDG_DATA_HOME="$HOME/data" XDG_STATE_HOME="$HOME/state"
export GOENV=off GOTELEMETRY=off GOTOOLCHAIN=local GOWORK=off GOPROXY=off GOSUMDB=off
export GOOS=linux GOARCH=amd64 CGO_ENABLED=1 GOMAXPROCS=2 MAGPIE_NO_STATS=1
go version > "$receipt/toolchain.log"
grep -Fxq 'go version go1.26.8 linux/amd64' "$receipt/toolchain.log"
printf 'GOROOT=%s\nGOPATH=%s\nGOMODCACHE=%s\nGOCACHE=%s\n' "$GOROOT" "$GOPATH" "$GOMODCACHE" "$GOCACHE" > "$receipt/cache-inputs.log"
ip -o link show > "$receipt/network.log"
awk '/^Cap(Inh|Prm|Eff|Bnd|Amb):/' /proc/self/status >> "$receipt/network.log"

host_base=947d03d554ba0855001fa4ca315df1c08d3f584f
sdk_base=afe1cd6a4c5b8ee65e596c9ec7a8bdb33bad7206
[[ "$(git -C "$host" rev-parse "$host_base^{tree}")" == 86ea14ea48e579646eb589659eba0ce0f8582542 ]]
[[ "$(git -C "$sdk" rev-parse "$sdk_base^{tree}")" == 494960fb82c42f8b4e830cfa19b40d278d6a68e8 ]]
[[ "$(git -C "$sdk" rev-parse "$sdk_base:packages/zcode/src")" == 2e77c4a20526d7abd1e3f4a354b7a6039484186f ]]
git -C "$host" show -s --format='%H %T' "$host_base" > "$receipt/host.git"
git -C "$sdk" show -s --format='%H %T' "$sdk_base" > "$receipt/sdk.git"
(
    cd "$kit"
    sha256sum worker.go gateway/zcode_trial_group_accounts_test.go run-linux.sh
) > "$receipt/inputs.sha256"
git -C "$host" archive "$host_base" | tar -x -C "$scratch/magpie"
git -C "$sdk" archive "$sdk_base" packages/zcode/src | tar -x -C "$scratch/sdk"

host_test="$scratch/magpie/internal/gateway/zcode_trial_group_accounts_test.go"
worker_source="$scratch/sdk/packages/zcode/src/testdata/trial-group-accounts"
[[ ! -e "$host_test" && ! -e "$worker_source" ]]
mkdir "$worker_source"
cp "$kit/gateway/zcode_trial_group_accounts_test.go" "$host_test"
cp "$kit/worker.go" "$worker_source/main.go"
gofmt -l "$host_test" "$worker_source/main.go" > "$receipt/gofmt.log"
[[ ! -s "$receipt/gofmt.log" ]]

run() {
    local label="$1" status=0
    shift
    "$@" > "$receipt/$label.log" 2> "$receipt/$label.stderr" || status=$?
    printf '%s\n' "$status" > "$receipt/$label.exit"
    return "$status"
}

(
    cd "$scratch/sdk/packages/zcode/src"
    run worker-build go build -mod=readonly -p=2 -o "$scratch/worker" ./testdata/trial-group-accounts
    sha256sum "$scratch/worker" > "$receipt/worker.sha256"
)
(
    cd "$scratch/magpie"
    run gateway-build go test -c -mod=readonly -tags=nogui -p=2 -race -o "$scratch/gateway.test" ./internal/gateway
)
(
    cd "$scratch/magpie/internal/gateway"
    selector='^TestZCodeInstalledTrialGroupAccountIsolation$'
    printf '%s\n' "$selector" > "$receipt/selector"
    run gateway-race env MAGPIE_ZCODE_GROUP_FIXTURE_WORKER="$scratch/worker" go tool test2json \
        -p github.com/yetone/magpie/internal/gateway "$scratch/gateway.test" -test.v=test2json \
        -test.run="$selector" -test.count=3 -test.timeout=125s
)
python3 -IB - "$receipt/gateway-race.log" > "$receipt/gateway-race.verified" <<'PY'
from collections import Counter
import json
from pathlib import Path
import sys

parent = "TestZCodeInstalledTrialGroupAccountIsolation"
cases = {
    "a-trial-held": "429 vendor sends=0",
    "trial-falls-through-to-b": "200 vendor sends=1 path=/start wire model=GLM-5.3 credentials matched",
    "a-preview-beside-held-trial": "200 vendor sends=1 path=/start wire model=glm-limited-preview credentials matched",
    "paid-falls-through-to-a": "200 vendor sends=1 path=/zai wire model=GLM-5.3 credentials matched",
}
events = [json.loads(line) for line in Path(sys.argv[1]).read_text().splitlines()]
expected = {"": 1, parent: 3, **{f"{parent}/{case}": 3 for case in cases}}
passes = Counter(row.get("Test", "") for row in events if row["Action"] == "pass")
assert passes == expected, passes
wire = Counter()
for row in events:
    assert row["Action"] not in ("fail", "skip"), row
    line = row.get("Output", "")
    assert "WARNING: DATA RACE" not in line, row
    if row["Action"] == "output" and "group status=" in line:
        wire[(row["Test"], line.split("group status=", 1)[1].strip())] += 1
assert wire == {(f"{parent}/{case}", text): 3 for case, text in cases.items()}, wire
print(json.dumps({"passes": passes, "verifiedWireEvents": sum(wire.values())}, sort_keys=True))
PY
(
    cd "$scratch/magpie"
    run gateway-vet go vet -mod=readonly -tags=nogui -p=2 ./internal/gateway
)
(
    cd "$kit"
    sha256sum -c "$receipt/inputs.sha256"
    cmp gateway/zcode_trial_group_accounts_test.go "$host_test"
    cmp worker.go "$worker_source/main.go"
) > "$receipt/final-source-check.log"
