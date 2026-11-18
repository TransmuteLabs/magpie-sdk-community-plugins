import contextlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import socket
import struct
import subprocess
import threading
import time


def process_identity(pid):
    directory = Path("/proc") / str(pid)
    fields = (directory / "stat").read_text().rsplit(")", 1)[1].split()
    return {"pid": pid, "ppid": int(fields[1]), "startTicks": int(fields[19])}


def tcp_address(address):
    return f"{struct.unpack('<I', socket.inet_aton(address[0]))[0]:08X}:{address[1]:04X}"


class OwnedIngress:
    def __init__(self, vendor, root):
        self.vendor = vendor
        self.root = process_identity(os.getpid())
        self.ss = shutil.which("ss")
        if not self.ss or not Path("/proc/self/net/tcp").is_file():
            raise RuntimeError("owned agent fixture requires Linux /proc and the installed ss command")
        self.log = Path(root) / "ingress.jsonl"
        self.output = self.log.open("x", encoding="utf-8")
        self.log.chmod(0o600)
        self.lock = threading.Lock()
        self.owners = {}
        self.rejected = 0
        vendor.server.verify_request = self.verify_request
        audit = self
        base = vendor.server.RequestHandlerClass

        class Handler(base):
            def parse_request(self):
                result = super().parse_request()
                if result:
                    peer = self.connection.getpeername()
                    with audit.lock:
                        owner = audit.owners[peer]
                    audit.emit("request", method=self.command, uri=self.path,
                               headers=list(self.headers.raw_items()), remote=peer,
                               local=self.connection.getsockname(), owner=owner)
                return result

            def finish(self):
                try:
                    super().finish()
                finally:
                    with audit.lock:
                        audit.owners.pop(self.client_address, None)

        vendor.server.RequestHandlerClass = Handler

    def emit(self, kind, **fields):
        record = {"kind": kind, "wallNs": time.time_ns(),
                  "monotonicNs": time.monotonic_ns(), **fields}
        with self.lock:
            self.output.write(json.dumps(record, sort_keys=True) + "\n")
            self.output.flush()

    def owner(self, connection):
        peer, local = connection.getpeername(), connection.getsockname()
        expected = (tcp_address(peer), tcp_address(local))
        inodes = {
            fields[9] for line in Path("/proc/self/net/tcp").read_text().splitlines()[1:]
            if len(fields := line.split()) >= 10 and tuple(fields[1:3]) == expected
        }
        if len(inodes) != 1:
            raise RuntimeError("client socket inode is unknown or ambiguous")
        inode = next(iter(inodes))
        query = f"src {peer[0]}:{peer[1]} and dst {local[0]}:{local[1]}"
        result = subprocess.run([self.ss, "-ntpHe", query], text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                timeout=5, check=True)
        owners = set(re.findall(r"pid=(\d+),fd=(\d+)", result.stdout))
        if not owners:
            raise RuntimeError("client socket process is unknown")
        matches = []
        for pid_text, descriptor in owners:
            pid = int(pid_text)
            process = process_identity(pid)
            directory = Path("/proc") / pid_text
            if os.readlink(directory / "fd" / descriptor) != f"socket:[{inode}]":
                raise RuntimeError("client socket process changed")
            executable = os.readlink(directory / "exe")
            lineage = [process]
            current = process
            while current["pid"] > 1 and current != self.root:
                if len(lineage) >= 64:
                    raise RuntimeError("client process lineage exceeds fixture bound")
                current = process_identity(current["ppid"])
                lineage.append(current)
            if process_identity(pid) != process:
                raise RuntimeError("client process identity changed")
            matches.append({**process, "fd": int(descriptor), "inode": inode,
                            "executable": executable, "owned": current == self.root,
                            "lineage": lineage})
        if len({match["owned"] for match in matches}) != 1:
            raise RuntimeError("client socket has mixed process ownership")
        return {"processes": matches, "owned": matches[0]["owned"]}

    def verify_request(self, connection, address):
        try:
            owner = self.owner(connection)
        except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as error:
            with self.vendor.lock:
                self.vendor.errors.append("unattributed fixture ingress: " + type(error).__name__)
            self.emit("unattributed", remote=address, local=connection.getsockname(),
                      error=type(error).__name__)
            return False
        if not owner["owned"]:
            with self.lock:
                self.rejected += 1
            self.emit("outside-fixture", remote=address,
                      local=connection.getsockname(), owner=owner)
            return False
        with self.lock:
            self.owners[address] = owner
        return True

    def close(self):
        self.output.close()


@contextlib.contextmanager
def owned_vendor(source, root):
    spec = importlib.util.spec_from_file_location("zcode_owned_vendor", source / "testdata/window-auth/vendor.py")
    vendor_module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(vendor_module)
    vendor = vendor_module.Vendor()
    try:
        audit = OwnedIngress(vendor, root)
    except BaseException:
        vendor.server.server_close()
        raise
    vendor.ingress = audit
    try:
        with vendor:
            yield vendor
    finally:
        audit.close()
