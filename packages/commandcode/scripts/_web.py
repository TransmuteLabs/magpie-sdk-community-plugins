"""Compile browser sources against the explicitly selected public SDK types."""
import json
from pathlib import Path
import subprocess
import tempfile


def compile_web(package, sdk, output=None, env=None):
    package, sdk = Path(package).resolve(), Path(sdk).resolve()
    options = {
        "strict": True, "skipLibCheck": True, "target": "ES2022",
        "module": "ES2022", "moduleResolution": "bundler", "lib": ["ES2022", "DOM"],
        "paths": {"@magpie/extension-sdk/ui": [str(sdk / "sdk/js/ui.d.ts")]},
    }
    if output is None:
        options["noEmit"] = True
    else:
        options.update(outDir=str(Path(output).resolve()), rootDir=str(package / "web"))
    with tempfile.TemporaryDirectory(prefix="commandcode-types-") as directory:
        config = Path(directory) / "tsconfig.json"
        config.write_text(json.dumps({"compilerOptions": options, "files": [str(package / "web/app.ts")]}))
        subprocess.run(["tsc", "--project", str(config)], cwd=package, env=env, check=True)
