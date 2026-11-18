import json
from pathlib import Path
import re
import unittest

PACKAGE = Path(__file__).resolve().parents[1]


class ProvenanceTests(unittest.TestCase):
    def test_distribution_identity_matches_source(self):
        lock = json.loads((PACKAGE / "docs/source-lock.json").read_text())
        source = (PACKAGE / "src/module.go").read_text()
        self.assertEqual(lock["schema"], 1)
        for constant, value in (("ModuleID", lock["plugin"]["id"]),
                                ("Version", lock["plugin"]["version"]),
                                ("AgentID", lock["agent"]["id"]),
                                ("TargetID", lock["agent"]["target"]),
                                ("ProfileID", lock["agent"]["profileUUID"])):
            self.assertRegex(source, re.escape(constant) + r'\s*=\s*"' + re.escape(value) + '"')
        self.assertEqual(lock["plugin"]["sdkMinimum"], "1.5")
        self.assertEqual(lock["plugin"]["nativeABI"], 1)
        self.assertIn(lock["agent"]["publicGoModule"], (PACKAGE / "src/go.mod").read_text())
        self.assertFalse(lock["verification"]["liveDesktopAcceptance"])

    def test_each_reference_and_watch_point_is_identified(self):
        lock = json.loads((PACKAGE / "docs/source-lock.json").read_text())
        sources = {source["id"]: source for source in lock["sources"]}
        self.assertEqual(set(sources), {"claude-desktop-recovered", "magpie-desktop-builtin"})
        for source in sources.values():
            self.assertRegex(source["commit"], r"^[0-9a-f]{40}$")
            self.assertTrue(source["files"])
            for digest in source["files"].values():
                self.assertRegex(digest, r"^[0-9a-f]{64}$")
        self.assertEqual(sources["claude-desktop-recovered"]["version"], "5.11492.5")
        for watch in lock["watch"]:
            self.assertIn(watch["source"], sources)
            self.assertTrue(watch["paths"])
            self.assertTrue(watch["behaviors"])
            for path in watch["tests"]:
                self.assertTrue((PACKAGE / path).is_file(), path)


if __name__ == "__main__":
    unittest.main()
