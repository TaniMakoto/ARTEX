"""Verify the shipped executable starts with no external database configured."""

import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
import zipfile


def main():
    with tempfile.TemporaryDirectory(prefix="artex-中文 portable-") as temp:
        workspace = Path(temp)
        with zipfile.ZipFile(sys.argv[1]) as archive:
            archive.extractall(workspace)
        root = workspace / "artex-windows-amd64-portable"
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        env = os.environ.copy()
        env.pop("ARTEX_PG_DSN", None)
        env.pop("ARTEX_POSTGRES_DIR", None)
        env["ARTEX_CONFIG"] = str(root / "config.json")
        env["ARTEX_SKILL_DIR"] = str(root / "skills")
        data = workspace / "持久数据 data"
        log_path = workspace / "smoke.log"
        with log_path.open("wb") as log:
            process = subprocess.Popen(
                [str(root / "artex.exe"), "-addr", f"127.0.0.1:{port}",
                 "-proxy=", "-data", str(data)],
                # Run outside the package directory to catch CWD dependencies.
                cwd=workspace, env=env, stdout=log, stderr=log,
                creationflags=subprocess.CREATE_NEW_PROCESS_GROUP,
            )
            try:
                deadline = time.monotonic() + 120
                while True:
                    if process.poll() is not None:
                        raise RuntimeError(f"Application exited: {process.returncode}")
                    try:
                        with urllib.request.urlopen(
                            f"http://127.0.0.1:{port}/api/health", timeout=2
                        ) as response:
                            health = json.load(response)
                        if health.get("status") == "ok" or health.get("ok") is True:
                            break
                        # Health schemas may also report version/worker details.
                        if health.get("version"):
                            break
                        raise RuntimeError(f"Unexpected health response: {health}")
                    except (urllib.error.URLError, TimeoutError):
                        if time.monotonic() >= deadline:
                            raise RuntimeError("Packaged application did not become ready")
                        time.sleep(0.25)
                with urllib.request.urlopen(f"http://127.0.0.1:{port}/", timeout=10) as response:
                    html = response.read().decode("utf-8")
                    if response.status != 200 or "<html" not in html.lower():
                        raise RuntimeError("Packaged web interface is missing")
                if not (data / "postgres" / "cluster" / "PG_VERSION").is_file():
                    raise RuntimeError("Persistent local database was not initialized")
                pid_file = data / "postgres" / "cluster" / "postmaster.pid"
                database_port = int(pid_file.read_text(encoding="utf-8").splitlines()[3])
                graceful = True
                try:
                    process.send_signal(signal.CTRL_BREAK_EVENT)
                except OSError:
                    # Windows CI may run without a console. Its job object must
                    # still clean up the database when ARTEX is terminated.
                    graceful = False
                    process.kill()
                code = process.wait(timeout=30)
                if graceful and code != 0:
                    raise RuntimeError("Application failed to shut down cleanly")
                deadline = time.monotonic() + 15
                while True:
                    try:
                        with socket.create_connection(("127.0.0.1", database_port), timeout=1):
                            pass
                    except OSError:
                        break
                    if time.monotonic() >= deadline:
                        raise RuntimeError("Owned database still listening after application exit")
                    time.sleep(0.1)
                if graceful and pid_file.exists():
                    raise RuntimeError("Local database was not stopped")
                print("PASS: offline package, embedded web UI, persistent database, process cleanup")
            except BaseException:
                log.flush()
                print(log_path.read_text(encoding="utf-8", errors="replace"), file=sys.stderr)
                raise
            finally:
                if process.poll() is None:
                    process.kill()
                    process.wait(timeout=15)


if __name__ == "__main__":
    main()
