"""Build the offline Windows distribution; no installer or runtime download."""

import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[1]
PG_VERSION = "17.11.0"
PG_URL = (
    "https://repo.maven.apache.org/maven2/io/zonky/test/postgres/"
    "embedded-postgres-binaries-windows-amd64/"
    f"{PG_VERSION}/embedded-postgres-binaries-windows-amd64-{PG_VERSION}.jar"
)
PG_SHA256 = "98040fae18dd9633ff95932125b0cecf0a45a1a9312e216e2a33ad03a31d4251"
COPYRIGHT_URL = "https://raw.githubusercontent.com/postgres/postgres/REL_17_11/COPYRIGHT"
COPYRIGHT_SHA256 = "3d6af92ff8a4c2cdf69afb1cf44edea727922f5cd0cf8b5f72b11cdecac8fdfd"


def verified_download(url, expected):
    with urllib.request.urlopen(url, timeout=120) as response:
        data = response.read()
    actual = hashlib.sha256(data).hexdigest()
    if actual != expected:
        raise RuntimeError(f"SHA256 mismatch for {url}: {actual}")
    return data


def prepare_runtime(destination):
    destination.mkdir(parents=True, exist_ok=True)
    archive = verified_download(PG_URL, PG_SHA256)
    with zipfile.ZipFile(io.BytesIO(archive)) as jar:
        payload = jar.read("postgres-windows-x86_64.txz")
    with tarfile.open(fileobj=io.BytesIO(payload), mode="r:xz") as tar:
        tar.extractall(destination, filter="data")
    for name in ("postgres", "initdb", "pg_ctl"):
        if not (destination / "bin" / f"{name}.exe").is_file():
            raise RuntimeError(f"Incomplete PostgreSQL runtime: {name}")
    (destination / "COPYRIGHT").write_bytes(
        verified_download(COPYRIGHT_URL, COPYRIGHT_SHA256)
    )
    (destination / "SOURCE.txt").write_text(
        f"PostgreSQL {PG_VERSION}\nBinary distribution: {PG_URL}\n"
        f"SHA256: {PG_SHA256}\nSource: https://github.com/postgres/postgres/tree/REL_17_11\n"
        "Binary packaging: https://github.com/zonkyio/embedded-postgres-binaries\n",
        encoding="utf-8",
    )
    print(f"Verified PostgreSQL runtime: {destination}")


def add_visual_cpp_runtime(destination, source=None):
    if source is None:
        installer = Path(os.environ.get("ProgramFiles(x86)", "C:/Program Files (x86)"))
        vswhere = installer / "Microsoft Visual Studio/Installer/vswhere.exe"
        installations = subprocess.check_output(
            [str(vswhere), "-products", "*", "-property", "installationPath"],
            text=True,
        ).splitlines()
        candidates = [
            path
            for installation in installations
            for path in Path(installation).glob("VC/Redist/MSVC/*/x64/Microsoft.VC*.CRT")
        ]
        if not candidates:
            raise RuntimeError("Visual Studio x64 redistributable CRT directory not found")
        source = sorted(candidates)[-1]
    for name in ("vcruntime140.dll", "msvcp140.dll"):
        if not (source / name).is_file():
            raise RuntimeError(f"Missing redistributable runtime: {source / name}")
    for path in source.glob("*.dll"):
        shutil.copy2(path, destination / "bin" / path.name)
    (destination / "MICROSOFT_RUNTIME.txt").write_text(
        "Microsoft Visual C++ Redistributable (x64), app-local CRT DLLs.\n"
        "Provided from the build environment's Visual Studio redistributable directory.\n"
        "Microsoft copyright and license terms apply.\n"
        "https://learn.microsoft.com/cpp/windows/redistributing-visual-cpp-files\n"
        "https://visualstudio.microsoft.com/license-terms/\n",
        encoding="utf-8",
    )
    print(f"App-local Visual C++ runtime included from: {source}")


def package(binary, runtime, output):
    for name in ("vcruntime140.dll", "msvcp140.dll"):
        if not (runtime / "bin" / name).is_file():
            raise RuntimeError(f"Portable package is missing app-local {name}")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="artex-portable-") as temp:
        root = Path(temp) / "artex-windows-amd64-portable"
        root.mkdir()
        shutil.copy2(binary, root / "artex.exe")
        shutil.copy2(ROOT / "start.bat", root)
        shutil.copy2(ROOT / "README_PORTABLE.md", root)
        shutil.copytree(runtime, root / "postgres")
        shutil.copytree(ROOT / "skills", root / "skills")
        config = {"database": {"driver": "embedded"}, "skill_dir": ""}
        (root / "config.json").write_text(
            json.dumps(config, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )
        # No user data, passwords, logs, or local configuration enter the archive.
        with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED) as archive:
            for path in sorted(root.rglob("*")):
                if path.is_file():
                    archive.write(path, path.relative_to(root.parent))
    digest = hashlib.sha256(output.read_bytes()).hexdigest()
    output.with_suffix(output.suffix + ".sha256").write_text(
        f"{digest}  {output.name}\n", encoding="ascii"
    )
    print(f"Portable package: {output}\nSHA256: {digest}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    runtime = sub.add_parser("runtime")
    runtime.add_argument("--output", type=Path, required=True)
    crt = sub.add_parser("crt")
    crt.add_argument("--runtime", type=Path, required=True)
    crt.add_argument("--source", type=Path)
    bundle = sub.add_parser("package")
    bundle.add_argument("--binary", type=Path, required=True)
    bundle.add_argument("--runtime", type=Path, required=True)
    bundle.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if args.command == "runtime":
        prepare_runtime(args.output)
    elif args.command == "crt":
        add_visual_cpp_runtime(args.runtime, args.source)
    else:
        package(args.binary, args.runtime, args.output)


if __name__ == "__main__":
    main()
