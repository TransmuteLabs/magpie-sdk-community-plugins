import json
import os
from pathlib import Path
import selectors
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parent
WIRE = json.loads((ROOT / "wire.json").read_bytes())


class PeerTests(unittest.TestCase):
    def start(self, mode="duplex"):
        env = os.environ.copy()
        env["ZCODE_CONTRACT_CASE"] = mode
        process = subprocess.Popen([sys.executable, "-I", str(ROOT / "peer.py"), "app-server", "--stdio"],
                                   env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE, bufsize=0)
        self.addCleanup(self.close, process)
        return process

    @staticmethod
    def close(process):
        if process.poll() is None:
            process.kill()
        process.wait(timeout=5)
        for stream in (process.stdin, process.stdout, process.stderr):
            if not stream.closed:
                stream.close()

    def write(self, process, name):
        process.stdin.write(json.dumps(WIRE[name]).encode() + b"\n")
        process.stdin.flush()

    def read(self, process):
        with selectors.DefaultSelector() as selector:
            selector.register(process.stdout, selectors.EVENT_READ)
            self.assertTrue(selector.select(timeout=5), "peer blocked behind pending execution")
        body = process.stdout.readline()
        self.assertTrue(body, "peer lost expected frame")
        return json.loads(body)

    def test_resume_preserves_literal_argv_and_cwd(self):
        with tempfile.TemporaryDirectory(prefix="literal cwd ") as directory:
            session = "session $(touch unexpected) `touch unexpected`; 'quoted' --continue"
            result = subprocess.run([sys.executable, "-I", str(ROOT / "peer.py"), "--resume", session],
                                    cwd=directory, capture_output=True, timeout=10, check=True)
            proof = json.loads(result.stdout)
            self.assertEqual(proof["argv"], ["--resume", session])
            self.assertEqual(proof["cwd"], str(Path(directory).resolve()))
            self.assertGreater(proof["pid"], 0)
            self.assertNotEqual(proof["pid"], os.getpid())
            self.assertEqual(list(Path(directory).iterdir()), [])

    def test_reverse_and_stop_complete_while_send_is_pending(self):
        process = self.start()
        self.write(process, "subscribe")
        self.assertEqual(self.read(process), WIRE["subscribeAck"])
        self.write(process, "send")
        self.assertEqual(self.read(process), WIRE["sendAck"])
        self.assertEqual(self.read(process), WIRE["reverse"])
        self.write(process, "query")
        state = self.read(process)["result"]["results"][0]["turn"]["state"]
        self.assertEqual(state, "running", "admission ACK became terminal success")
        self.write(process, "reverseReply")
        self.write(process, "stop")
        self.assertEqual(self.read(process), WIRE["reverseCancelled"])
        self.assertEqual(self.read(process), WIRE["stopAck"])
        self.write(process, "query")
        self.assertEqual(self.read(process), WIRE["queryResult"])
        process.stdin.close()
        self.assertEqual(process.wait(timeout=5), 0)

    def test_loss_after_ack_has_no_terminal_or_reverse(self):
        process = self.start("loss-after-ack")
        self.write(process, "send")
        self.assertEqual(self.read(process), WIRE["sendAck"])
        process.stdin.close()
        self.assertEqual(process.stdout.read(), b"")
        self.assertEqual(process.wait(timeout=5), 0)

    def test_replayed_admission_is_rejected(self):
        process = self.start()
        self.write(process, "send")
        self.read(process)
        self.read(process)
        self.write(process, "send")
        process.stdin.close()
        self.assertEqual(process.wait(timeout=5), 3)
        self.assertIn(b"admitted input was replayed", process.stderr.read())

    def test_foreign_reverse_reply_is_rejected(self):
        process = self.start()
        self.write(process, "send")
        self.read(process)
        self.read(process)
        process.stdin.write(b'{"id":"foreign","result":{"headersApplied":false}}\n')
        process.stdin.close()
        self.assertEqual(process.wait(timeout=5), 3)
        self.assertIn(b"foreign correlation", process.stderr.read())

    def test_large_split_utf8_blank_lines_and_eof_tail(self):
        process = self.start("framing")
        output, diagnostic = process.communicate(timeout=15)
        self.assertEqual(process.returncode, 0)
        self.assertEqual(diagnostic, b"synthetic diagnostic is not a protocol frame\n")
        frames = [line for line in output.split(b"\n") if line.strip()]
        self.assertEqual(len(frames), 1)
        self.assertFalse(output.endswith(b"\n"))
        value = json.loads(frames[0])
        self.assertEqual(value["id"], "framing-1")
        self.assertEqual(value["result"]["text"], "中\u2028\u2029" + "x" * (15 * 1024 * 1024))

    def test_malformed_frame_is_available_for_host_rejection(self):
        process = self.start("malformed")
        output, _ = process.communicate(timeout=5)
        self.assertEqual(process.returncode, 0)
        with self.assertRaises(json.JSONDecodeError):
            json.loads(output)


if __name__ == "__main__":
    unittest.main(verbosity=2)
