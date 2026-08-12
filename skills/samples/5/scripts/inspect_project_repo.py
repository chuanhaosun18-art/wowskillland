#!/usr/bin/env python3
"""Inspect a project repository and write thesis-oriented manifests.

This script is intentionally read-only for local repositories. For Git URLs it
only clones when --clone-to is explicitly provided.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
from collections import Counter, defaultdict
from pathlib import Path
from typing import Iterable


SKIP_DIRS = {
    ".git",
    ".hg",
    ".svn",
    "__pycache__",
    ".pytest_cache",
    ".mypy_cache",
    ".ruff_cache",
    ".idea",
    ".vscode",
    "node_modules",
    "dist",
    "build",
    ".venv",
    "venv",
    "env",
}

README_NAMES = {"readme.md", "readme.txt", "readme.rst", "README.md", "README.txt", "README.rst"}
CONFIG_NAMES = {
    "pyproject.toml",
    "requirements.txt",
    "environment.yml",
    "environment.yaml",
    "setup.py",
    "setup.cfg",
    "package.json",
    "Dockerfile",
    "docker-compose.yml",
    "Makefile",
}

CODE_SUFFIXES = {
    ".py",
    ".ipynb",
    ".m",
    ".r",
    ".R",
    ".jl",
    ".cpp",
    ".cc",
    ".cxx",
    ".c",
    ".h",
    ".hpp",
    ".java",
    ".js",
    ".ts",
    ".tsx",
    ".sh",
}

DATA_SUFFIXES = {".csv", ".xlsx", ".xls", ".json", ".jsonl", ".parquet", ".pkl", ".npy", ".npz", ".mat"}
FIGURE_SUFFIXES = {".png", ".jpg", ".jpeg", ".svg", ".pdf", ".eps", ".tif", ".tiff"}
DOC_SUFFIXES = {".md", ".docx", ".doc", ".pdf", ".pptx", ".ppt", ".tex"}

KEYWORD_GROUPS = {
    "training": ("train", "fit", "epoch", "trainer"),
    "evaluation": ("eval", "test", "metric", "score", "confusion", "f1", "accuracy", "recall"),
    "model": ("model", "net", "network", "cnn", "lstm", "transformer", "attention", "resnet", "meta"),
    "data": ("data", "dataset", "loader", "preprocess", "clean", "scada", "label"),
    "augmentation": ("augment", "sample", "smote", "gan", "gpr", "dtw", "balance"),
    "deployment": ("api", "server", "app", "web", "platform", "deploy", "dashboard"),
    "results": ("result", "log", "runs", "output", "figure", "plot", "checkpoint"),
}


def is_git_url(value: str) -> bool:
    return value.startswith(("http://", "https://", "git@")) or value.endswith(".git")


def clone_if_needed(repo_arg: str, clone_to: str | None) -> Path:
    if not is_git_url(repo_arg):
        return Path(repo_arg).expanduser().resolve()
    if not clone_to:
        raise SystemExit("Git URL provided. Use --clone-to <dir> to clone before inspection.")
    target = Path(clone_to).expanduser().resolve()
    if target.exists() and any(target.iterdir()):
        repo = target
    else:
        target.parent.mkdir(parents=True, exist_ok=True)
        subprocess.run(["git", "clone", repo_arg, str(target)], check=True)
        repo = target
    return repo


def iter_files(root: Path, max_files: int) -> Iterable[Path]:
    count = 0
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS and not d.startswith(".cache")]
        base = Path(dirpath)
        for name in filenames:
            if name.startswith(".DS_Store"):
                continue
            path = base / name
            count += 1
            if count > max_files:
                return
            yield path


def rel(root: Path, path: Path) -> str:
    return str(path.relative_to(root))


def safe_size(path: Path) -> int:
    try:
        return path.stat().st_size
    except OSError:
        return -1


def read_preview(path: Path, limit: int = 3000) -> str:
    try:
        data = path.read_text(encoding="utf-8", errors="replace")
    except OSError:
        return ""
    return data[:limit].strip()


def line_count(path: Path, max_bytes: int = 2_000_000) -> int | None:
    if safe_size(path) > max_bytes:
        return None
    try:
        with path.open("rb") as f:
            return sum(1 for _ in f)
    except OSError:
        return None


def score_keywords(relative: str) -> list[str]:
    lower = relative.lower()
    hits = []
    for group, words in KEYWORD_GROUPS.items():
        if any(word in lower for word in words):
            hits.append(group)
    return hits


def inspect(root: Path, max_files: int) -> dict:
    files = list(iter_files(root, max_files=max_files))
    suffix_counts = Counter(p.suffix.lower() or "<none>" for p in files)
    top_dirs = Counter(rel(root, p).split(os.sep)[0] for p in files)

    groups: dict[str, list[dict]] = defaultdict(list)
    readmes = []
    configs = []
    candidates = []

    for path in files:
        relative = rel(root, path)
        suffix = path.suffix
        entry = {
            "path": relative,
            "size": safe_size(path),
            "suffix": suffix,
            "line_count": line_count(path) if suffix in CODE_SUFFIXES or path.name in README_NAMES else None,
            "keyword_groups": score_keywords(relative),
        }
        lname = path.name.lower()
        if path.name in README_NAMES or lname in {n.lower() for n in README_NAMES}:
            entry["preview"] = read_preview(path)
            readmes.append(entry)
        if path.name in CONFIG_NAMES:
            configs.append(entry)
        if suffix in CODE_SUFFIXES:
            groups["code"].append(entry)
        if suffix in DATA_SUFFIXES:
            groups["data"].append(entry)
        if suffix in FIGURE_SUFFIXES:
            groups["figures"].append(entry)
        if suffix in DOC_SUFFIXES:
            groups["documents"].append(entry)
        if entry["keyword_groups"]:
            candidates.append(entry)

    def sort_entries(items: list[dict]) -> list[dict]:
        return sorted(items, key=lambda x: (-(x.get("line_count") or 0), x["path"]))[:80]

    return {
        "root": str(root),
        "file_count_scanned": len(files),
        "suffix_counts": dict(suffix_counts.most_common()),
        "top_dirs": dict(top_dirs.most_common(30)),
        "readmes": readmes[:10],
        "configs": sort_entries(configs),
        "groups": {k: sort_entries(v) for k, v in groups.items()},
        "thesis_candidates": sort_entries(candidates),
    }


def write_markdown(report: dict, path: Path) -> None:
    lines = [
        "# Repository Inspection",
        "",
        f"- Root: `{report['root']}`",
        f"- Files scanned: {report['file_count_scanned']}",
        "",
        "## Top Directories",
        "",
    ]
    for name, count in report["top_dirs"].items():
        lines.append(f"- `{name}`: {count}")
    lines.extend(["", "## File Types", ""])
    for suffix, count in report["suffix_counts"].items():
        lines.append(f"- `{suffix}`: {count}")

    for readme in report["readmes"]:
        lines.extend(["", f"## README Preview: `{readme['path']}`", "", "```text"])
        lines.append(readme.get("preview", ""))
        lines.append("```")

    section_names = [
        ("configs", "Configuration Files"),
        ("code", "Code Files"),
        ("data", "Data-Like Files"),
        ("figures", "Figure-Like Files"),
        ("documents", "Document Files"),
        ("thesis_candidates", "Thesis-Relevant Candidates"),
    ]
    for key, title in section_names:
        items = report.get(key) if key in report else report["groups"].get(key, [])
        lines.extend(["", f"## {title}", ""])
        if not items:
            lines.append("- None found.")
            continue
        for item in items[:60]:
            tags = ", ".join(item.get("keyword_groups") or [])
            lc = item.get("line_count")
            extra = []
            if lc is not None:
                extra.append(f"{lc} lines")
            if tags:
                extra.append(f"tags: {tags}")
            tail = f" ({'; '.join(extra)})" if extra else ""
            lines.append(f"- `{item['path']}`{tail}")

    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("repo", help="Local repository path or Git URL")
    parser.add_argument("--clone-to", help="Clone target when repo is a Git URL")
    parser.add_argument("--out", required=True, help="JSON output path")
    parser.add_argument("--md", required=True, help="Markdown output path")
    parser.add_argument("--max-files", type=int, default=5000)
    args = parser.parse_args()

    root = clone_if_needed(args.repo, args.clone_to)
    if not root.exists() or not root.is_dir():
        raise SystemExit(f"Repository path does not exist or is not a directory: {root}")

    report = inspect(root, max_files=args.max_files)
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    write_markdown(report, Path(args.md))
    print(f"Wrote {out}")
    print(f"Wrote {args.md}")


if __name__ == "__main__":
    main()
