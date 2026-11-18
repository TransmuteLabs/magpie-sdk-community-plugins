import argparse
import hashlib
import json
import os
from pathlib import Path
import runpy
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    parser.add_argument("label")
    parser.add_argument("--red", action="store_true")
    parser.add_argument("--without-confirmation", action="store_true")
    args = parser.parse_args()
    source = Path(__file__).resolve().parents[2]
    root = args.root.resolve()
    isolation = runpy.run_path(str(source / "testdata/agent/verify.py"))["isolate"]()
    home = Path(tempfile.mkdtemp(prefix=args.label + "-", dir=root))
    output = home / "build"
    env = {"PATH": os.environ["PATH"], "HOME": str(home), "USERPROFILE": str(home),
           "XDG_CONFIG_HOME": str(home / "config"), "XDG_CACHE_HOME": str(home / "cache"),
           "XDG_DATA_HOME": str(home / "data"), "XDG_STATE_HOME": str(home / "state"),
           "ZCODE_PAGE_BUILD_DIR": str(output), "LANG": "C.UTF-8", "TZ": "UTC"}
    config = home / "tsconfig.json"
    config.write_text(json.dumps({
        "compilerOptions": {
            "strict": True, "target": "ES2022", "module": "NodeNext",
            "moduleResolution": "NodeNext", "lib": ["ES2022", "DOM"],
            "rootDir": str(source.parent / "web"), "outDir": str(output),
            "paths": {"@magpie/extension-sdk/ui": [str(root / "magpie/sdk/js/ui.d.ts")]},
        },
        "include": [str(source.parent / "web/*.ts")],
    }))
    subprocess.run(["tsc", "--project", str(config)], env=env, cwd=source,
                   check=True, timeout=60)
    if args.without_confirmation:
        page = output / "page.js"
        before = "if (!await confirmWrite(`Проверить предложения"
        original = page.read_text()
        assert original.count(before) == 1
        page.write_text(original.replace(before, "if (false && !await confirmWrite(`Проверить предложения"))
    (output / "package.json").write_text('{"type":"module"}\n')
    log = root / (args.label + ".log")
    with log.open("wb") as stream:
        process = subprocess.run(["node", "--test", "../web/page.test.mjs"], env=env,
                                 cwd=source, stdout=stream, stderr=subprocess.STDOUT, timeout=60)
    report = {"isolation": isolation, "exit": process.returncode,
              "withoutConfirmation": args.without_confirmation,
              "compiledPageSHA256": hashlib.sha256((output / "page.js").read_bytes()).hexdigest(),
              "logSHA256": hashlib.sha256(log.read_bytes()).hexdigest()}
    (root / (args.label + "-results.json")).write_text(json.dumps(report, indent=2) + "\n")
    print(args.label, "exit", process.returncode, flush=True)
    if args.red:
        assert process.returncode == 1 and "ERR_ASSERTION" in log.read_text(), log.read_text()
    else:
        assert process.returncode == 0, log.read_text()


if __name__ == "__main__":
    main()
