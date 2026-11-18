import ctypes
import importlib.util
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock


module_path = Path(os.environ.get("ZCODE_CLEANUP_SOURCE", Path(__file__).with_name("owned_run.py")))
spec = importlib.util.spec_from_file_location("zcode_owned_run", module_path)
owned_run = importlib.util.module_from_spec(spec)
spec.loader.exec_module(owned_run)


guard_source = """
import json,os,signal,sys,time
from pathlib import Path
signal.signal(signal.SIGTERM,signal.SIG_IGN)
child=os.fork()
if child==0:
 while True: signal.pause()
def identity(pid):
 fields=Path('/proc',str(pid),'stat').read_text().rsplit(')',1)[1].split()
 return {'pid':pid,'startTicks':int(fields[19])}
Path(sys.argv[1]).write_text(json.dumps([identity(os.getpid()),identity(child)]))
if sys.argv[2]=='exit': raise SystemExit(0)
while True: signal.pause()
"""


def running(process):
    try:
        fields = Path("/proc", str(process["pid"]), "stat").read_text().rsplit(")", 1)[1].split()
        return int(fields[19]) == process["startTicks"] and fields[0] != "Z"
    except FileNotFoundError:
        return False


@unittest.skipUnless(sys.platform == "linux", "Linux process identity and prctl subreaper")
class CleanupTests(unittest.TestCase):
    def setUp(self):
        self.libc = ctypes.CDLL(None, use_errno=True)
        self.previous = ctypes.c_int()
        self.assertEqual(self.libc.prctl(37, ctypes.byref(self.previous), 0, 0, 0), 0)
        self.assertEqual(self.libc.prctl(36, 1, 0, 0, 0), 0)

    def tearDown(self):
        self.assertEqual(self.libc.prctl(36, self.previous.value, 0, 0, 0), 0)

    def exercise(self, mode):
        processes = []
        with tempfile.TemporaryDirectory() as root:
            path = Path(root) / "owned.json"
            unrelated = subprocess.Popen(
                [sys.executable, "-I", "-c", "import time; time.sleep(10)"],
                start_new_session=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            try:
                result = owned_run.run_owned(
                    [sys.executable, "-I", "-c", guard_source, str(path), mode],
                    timeout=0.8, grace=0.1, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                processes = json.loads(path.read_text())
                deadline = time.monotonic() + 1
                while any(running(process) for process in processes) and time.monotonic() < deadline:
                    time.sleep(0.01)
                self.assertTrue(all(not running(process) for process in processes), processes)
                self.assertEqual(result["timedOut"], mode == "wait")
                self.assertEqual(result["exit"], 124 if mode == "wait" else 0)
                self.assertEqual(result["childExit"], -signal.SIGKILL if mode == "wait" else 0)
                self.assertTrue(result["ownedGroupStopped"])
                self.assertIsNone(unrelated.poll())
            finally:
                if path.exists():
                    processes = json.loads(path.read_text())
                for process in processes:
                    if running(process):
                        os.kill(process["pid"], signal.SIGKILL)
                for process in processes:
                    deadline = time.monotonic() + 3
                    while True:
                        try:
                            finished, _ = os.waitpid(process["pid"], os.WNOHANG)
                        except ChildProcessError:
                            break
                        if finished:
                            break
                        if time.monotonic() >= deadline:
                            self.fail("owned synthetic child was not joined")
                        time.sleep(0.01)
                unrelated.terminate()
                unrelated.wait(timeout=3)

    def test_timeout_stops_guard_and_child_without_signalling_unrelated(self):
        self.exercise("wait")

    def test_success_stops_lingering_owned_child(self):
        self.exercise("exit")

    def test_command_error_remains_red(self):
        result = owned_run.run_owned(
            [sys.executable, "-I", "-c", "raise SystemExit(7)"],
            timeout=1, grace=0, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        self.assertEqual(result["exit"], 7)
        self.assertEqual(result["childExit"], 7)
        self.assertFalse(result["timedOut"])

    def test_reused_or_changed_leader_identity_never_signals(self):
        process = mock.Mock(pid=os.getpid())
        expected = owned_run.identity(process.pid)
        expected["startTicks"] += 1
        with mock.patch.object(owned_run.os, "killpg") as kill_group:
            with self.assertRaisesRegex(RuntimeError, "identity"):
                owned_run.stop_group(process, expected, 0)
            kill_group.assert_not_called()

    def test_shared_process_group_never_signals(self):
        process = mock.Mock(pid=os.getpid())
        with mock.patch.object(owned_run.os, "killpg") as kill_group:
            with self.assertRaisesRegex(RuntimeError, "session"):
                owned_run.stop_group(process, owned_run.identity(process.pid), 0)
            kill_group.assert_not_called()


if __name__ == "__main__":
    unittest.main()
