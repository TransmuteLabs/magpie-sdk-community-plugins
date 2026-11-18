import http.client
import importlib.util
import json
import multiprocessing
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import urllib.request


spec = importlib.util.spec_from_file_location("zcode_fixture_ingress", Path(__file__).with_name("ingress.py"))
ingress = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ingress)
source = Path(__file__).resolve().parents[2]


def request(origin, method, path, headers=None):
    value = urllib.request.Request(origin + path, method=method,
                                   data=b"{}" if method == "POST" else None,
                                   headers=headers or {})
    try:
        with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(value, timeout=10):
            pass
    except urllib.error.HTTPError as error:
        error.close()


def foreign_server(pipe, root):
    with ingress.owned_vendor(source, root) as vendor:
        pipe.send(vendor.origin)
        pipe.recv()
        pipe.send(list(vendor.errors))
    pipe.close()


@unittest.skipUnless(sys.platform == "linux", "Linux /proc and ss socket ownership")
class IngressTests(unittest.TestCase):
    def test_owned_unknown_methods_and_marked_probe_remain_errors(self):
        with tempfile.TemporaryDirectory() as root, ingress.owned_vendor(source, root) as vendor:
            request(vendor.origin, "GET", "/", {"X-Fake-Probe": "not-authority"})
            request(vendor.origin, "POST", "/")
            self.assertEqual(vendor.errors, ["unexpected vendor GET /", "unexpected vendor POST /"])
            self.assertEqual(vendor.ingress.rejected, 0)
            records = [json.loads(line) for line in (Path(root) / "ingress.jsonl").read_text().splitlines()]
            self.assertEqual([record["method"] for record in records], ["GET", "POST"])
            self.assertTrue(all(record["owner"]["owned"] for record in records))

    def test_actual_owned_child_reaches_strict_vendor(self):
        with tempfile.TemporaryDirectory() as root, ingress.owned_vendor(source, root) as vendor:
            result = subprocess.run([
                sys.executable, "-I", "-c",
                "import urllib.request,urllib.error,sys\n"
                "try: urllib.request.build_opener(urllib.request.ProxyHandler({})).open(sys.argv[1],timeout=10)\n"
                "except urllib.error.HTTPError as error: error.close()\n",
                vendor.origin + "/",
            ], stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=15)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(vendor.errors, ["unexpected vendor GET /"])
            records = [json.loads(line) for line in (Path(root) / "ingress.jsonl").read_text().splitlines()]
            self.assertEqual(len(records), 1)
            self.assertTrue(records[0]["owner"]["owned"])
            self.assertNotEqual(records[0]["owner"]["processes"][0]["pid"], vendor.ingress.root["pid"])

    def test_actual_foreign_process_is_rejected_before_http(self):
        with tempfile.TemporaryDirectory() as root:
            context = multiprocessing.get_context("fork")
            parent, child = context.Pipe()
            process = context.Process(target=foreign_server, args=(child, root))
            process.start()
            try:
                self.assertTrue(parent.poll(10))
                origin = parent.recv()
                for method in ("GET", "POST"):
                    with self.assertRaises(http.client.RemoteDisconnected):
                        request(origin, method, "/")
                parent.send("stop")
                self.assertTrue(parent.poll(10))
                self.assertEqual(parent.recv(), [])
                process.join(10)
                self.assertEqual(process.exitcode, 0)
                records = [json.loads(line) for line in (Path(root) / "ingress.jsonl").read_text().splitlines()]
                self.assertEqual(len(records), 2)
                self.assertTrue(all(record["kind"] == "outside-fixture" for record in records))
                self.assertTrue(all(not record["owner"]["owned"] for record in records))
            finally:
                if process.is_alive():
                    process.terminate()
                    process.join(5)
                parent.close()
                child.close()

    def test_unknown_owner_fails_closed(self):
        with tempfile.TemporaryDirectory() as root, ingress.owned_vendor(source, root) as vendor:
            def missing_owner(connection):
                raise RuntimeError("synthetic missing owner")

            vendor.ingress.owner = missing_owner
            with self.assertRaises(http.client.RemoteDisconnected):
                request(vendor.origin, "GET", "/")
            self.assertEqual(vendor.errors, ["unattributed fixture ingress: RuntimeError"])
            records = [json.loads(line) for line in (Path(root) / "ingress.jsonl").read_text().splitlines()]
            self.assertEqual([record["kind"] for record in records], ["unattributed"])


if __name__ == "__main__":
    unittest.main()
