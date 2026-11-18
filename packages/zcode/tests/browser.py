#!/usr/bin/env python3
"""Compile active browser sources and run fixture-only Node/Bun regressions."""
import argparse
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile


PACKAGE = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(PACKAGE.parents[1] / "scripts"))
from _workspace import build_environment


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--sdk", type=Path)
    args = parser.parse_args()
    with build_environment(PACKAGE, args.sdk) as (env, sdk), tempfile.TemporaryDirectory(prefix="zcode-browser-") as scratch:
        output = Path(scratch)
        config = output / "tsconfig.json"
        config.write_text(json.dumps({
            "compilerOptions": {
                "strict": True, "target": "ES2022", "module": "NodeNext",
                "moduleResolution": "NodeNext", "lib": ["ES2022", "DOM"],
                "rootDir": str(PACKAGE / "web"), "outDir": str(output),
                "paths": {"@magpie/extension-sdk/ui": [str(sdk / "sdk/js/ui.d.ts")]},
            },
            "include": [str(PACKAGE / "web/*.ts")],
        }))
        subprocess.run(["tsc", "--project", str(config)], cwd=PACKAGE, env=env, check=True)
        subprocess.run(["bun", "build", str(sdk / "sdk/js/ui.js"), "--target", "browser",
                        "--format", "esm", "--outfile", str(output / "sdk-ui.js")],
                       cwd=PACKAGE, env=env, check=True)
        subprocess.run(["bun", "build", str(output / "app.js"), "--target", "browser",
                        "--format", "iife", "--outfile", str(output / "app.bundle.js")],
                       cwd=PACKAGE, env=env, check=True)
        (output / "package.json").write_text('{"type":"module"}\n')
        shutil.copyfile(PACKAGE / "web/index.html", output / "index.html")
        env.update(ZCODE_PAGE_BUILD_DIR=str(output), ZCODE_UI15_BUILD_DIR=str(output))
        tests = [str(path) for path in sorted((PACKAGE / "web").glob("*.test.mjs"))]
        tests.append(str(PACKAGE / "docs/history/prototype/ui-sdk15/control.test.mjs"))
        for command in (["node", "--test"], ["bun", "test"]):
            subprocess.run([*command, *tests], cwd=PACKAGE, env=env, check=True, timeout=60)


if __name__ == "__main__":
    main()
