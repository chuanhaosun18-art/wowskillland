#!/usr/bin/env python3
from __future__ import annotations

import argparse
import shutil
import subprocess
from pathlib import Path


GENERATED_SUFFIXES = [
    ".aux",
    ".bbl",
    ".blg",
    ".log",
    ".out",
    ".toc",
    ".xdv",
    ".synctex.gz",
]


def find_tectonic(explicit: str | None) -> str:
    candidates = []
    if explicit:
        candidates.append(Path(explicit).expanduser())
    found = shutil.which("tectonic")
    if found:
        candidates.append(Path(found))
    candidates.append(Path.home() / "Downloads" / "thesis_midterm_latex" / "build" / "tectonic")
    for candidate in candidates:
        if candidate and candidate.exists():
            return str(candidate)
    raise SystemExit("Cannot find tectonic. Install it or pass --tectonic /path/to/tectonic.")


def clean_project(project: Path) -> None:
    for suffix in GENERATED_SUFFIXES:
        for path in project.glob(f"*{suffix}"):
            path.unlink(missing_ok=True)
        for path in (project / "chapters").glob(f"*{suffix}"):
            path.unlink(missing_ok=True)


def run_tectonic(tectonic: str, project: Path, reruns: int) -> None:
    cmd = [
        tectonic,
        "--print",
        "--keep-intermediates",
        "--keep-logs",
        "--reruns",
        str(reruns),
        "main.tex",
    ]
    subprocess.run(cmd, cwd=project, check=True)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Compile a BJTU thesis project with Tectonic.")
    parser.add_argument("--project", required=True, help="BJTU project directory containing main.tex.")
    parser.add_argument("--tectonic", help="Path to tectonic executable.")
    parser.add_argument("--copy-to", help="Optional final PDF path to copy main.pdf to.")
    parser.add_argument("--clean", action="store_true", help="Remove generated LaTeX files before compiling.")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    project = Path(args.project).expanduser().resolve()
    if not (project / "main.tex").exists():
        raise SystemExit(f"Cannot find main.tex in {project}")
    tectonic = find_tectonic(args.tectonic)
    if args.clean:
        clean_project(project)

    run_tectonic(tectonic, project, 0)
    run_tectonic(tectonic, project, 1)

    pdf = project / "main.pdf"
    if not pdf.exists():
        raise SystemExit(f"Compilation finished but main.pdf was not produced: {pdf}")
    if args.copy_to:
        target = Path(args.copy_to).expanduser().resolve()
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(pdf, target)
        print(target)
    else:
        print(pdf)


if __name__ == "__main__":
    main()
