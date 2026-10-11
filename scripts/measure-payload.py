#!/usr/bin/env python3
"""Report release component sizes without rebuilding or changing the payload."""

import argparse
import gzip
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("build_dir", type=Path)
    parser.add_argument("artifacts_dir", type=Path, nargs="?")
    args = parser.parse_args()
    if not args.build_dir.is_dir():
        parser.error(f"build directory does not exist: {args.build_dir}")
    if args.artifacts_dir is not None and not args.artifacts_dir.is_dir():
        parser.error(f"artifacts directory does not exist: {args.artifacts_dir}")

    print("component\tbytes\tpath")
    files = sorted(
        p for p in args.build_dir.rglob("*") if p.is_file() and not p.is_symlink()
    )
    for name in ("meiro", "ffmpeg", "yt-dlp", "qjs"):
        matches = [p for p in files if p.name.lower() == name]
        if not matches:
            print(f"{name}\tmissing\t{args.build_dir}")
        for file in matches:
            print(f"{name}\t{file.stat().st_size}\t{file}")
            if name == "yt-dlp":
                internal = file.parent / "_internal"
                size = "missing"
                if internal.is_dir():
                    size = sum(
                        p.stat().st_size
                        for p in internal.rglob("*")
                        if p.is_file() and not p.is_symlink()
                    )
                print(f"yt-dlp-internal\t{size}\t{internal}")

    packages = args.artifacts_dir or args.build_dir
    for file in sorted(packages.rglob("*")):
        if file.is_file() and file.name.endswith(
            (".deb", ".rpm", ".AppImage", ".tar.gz", ".dmg")
        ):
            print(f"package\t{file.stat().st_size}\t{file}")

    font = Path(__file__).resolve().parent.parent / "fonts" / "GoogleSans.ttf"
    data = font.read_bytes()
    print(f"font\t{len(data)}\t{font}")
    print(f"font-gzip-candidate\t{len(gzip.compress(data, mtime=0))}\t{font}")


if __name__ == "__main__":
    main()
