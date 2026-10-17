"""Exercise the shipped batch launcher in hidden, real Windows consoles."""

import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile


def check(launcher, codepage, scenario):
    with tempfile.TemporaryDirectory(prefix="artex launcher-") as temp:
        root = Path(temp)
        (root / "start.bat").write_bytes(launcher)
        outside = root / "different cwd"
        outside.mkdir()
        cmd = Path(os.environ["SystemRoot"]) / "System32/cmd.exe"
        if scenario != "missing":
            # A copy of cmd.exe is a harmless stand-in for artex.exe: its /c
            # helper controls exit codes without starting a real application.
            shutil.copy2(cmd, root / "artex.exe")
        if scenario == "arguments":
            helper = (
                '@echo off\n'
                # %* preserves quotes and the trailing '='; individual batch
                # parameters treat '=' as a delimiter, unlike Go's argv parser.
                'echo ARGS=%*\nexit /b 0\n'
            )
        else:
            first_exit = 75 if scenario == "update" else 5
            helper = (
                '@echo off\nif not exist attempted.flag (\n'
                'type nul > attempted.flag\necho FIRST\n'
                f'exit /b {first_exit}\n)\necho SECOND\nexit /b 0\n'
            )
        (root / "scenario.bat").write_bytes(helper.replace("\n", "\r\n").encode("ascii"))
        command = (
            f'chcp {codepage} >nul & call "..\\start.bat" '
            '/d /c call scenario.bat -addr :9000 "data path with spaces" -proxy='
        )
        startup = subprocess.STARTUPINFO()
        startup.dwFlags |= subprocess.STARTF_USESHOWWINDOW
        startup.wShowWindow = subprocess.SW_HIDE
        result = subprocess.run(
            # cmd uses its own quoting rules, rather than the C argv rules
            # Python applies to a list of arguments. /s removes these outer quotes.
            f'"{cmd}" /d /s /c "{command}"', cwd=outside,
            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=15,
            creationflags=subprocess.CREATE_NEW_CONSOLE, startupinfo=startup,
        )
        output = result.stdout.decode("utf-8", errors="replace")
        expected_code = 1 if scenario == "missing" else 0
        if result.returncode != expected_code:
            raise RuntimeError(f"{codepage}/{scenario}: exit {result.returncode}\n{output}")
        expected = {
            "arguments": 'ARGS=-addr :9000 "data path with spaces" -proxy=\r\n[artex] Stopped normally.\r\n',
            "update": "FIRST\r\n[artex] Restart requested. Applying staged version.\r\nSECOND\r\n[artex] Stopped normally.\r\n",
            "crash": "FIRST\r\n[artex] Unexpected exit (code=5). Restarting in 1s. \r\nSECOND\r\n[artex] Stopped normally.\r\n",
            "missing": "[artex] Executable not found: artex.exe \r\n",
        }[scenario]
        # Ignore echo's trailing space before stderr redirection.
        if [line.rstrip() for line in output.splitlines()] != [line.rstrip() for line in expected.splitlines()]:
            raise RuntimeError(f"{codepage}/{scenario}: unexpected console output\n{output}")
        print(f"PASS: code page {codepage}, {scenario}")


def main():
    with zipfile.ZipFile(sys.argv[1]) as archive:
        launcher = archive.read("artex-windows-amd64-portable/start.bat")
    launcher.decode("ascii")
    if b"\n" in launcher.replace(b"\r\n", b""):
        raise RuntimeError("Packaged start.bat must use CRLF line endings")
    for codepage in (936, 65001):
        for scenario in ("arguments", "update", "crash", "missing"):
            check(launcher, codepage, scenario)


if __name__ == "__main__":
    main()
