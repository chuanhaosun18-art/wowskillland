#!/usr/bin/env python3
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


CRITICAL_LOG_PATTERNS = [
    r"Citation `[^']+' .*undefined",
    r"There were undefined citations",
    r"There were undefined references",
    r"Rerun to get citations correct",
    r"Label\(s\) may have changed",
    r"! LaTeX Error:",
    r"! Undefined control sequence",
]


def read_text(path: Path) -> str:
    return path.read_text(encoding="utf-8", errors="ignore") if path.exists() else ""


def count_bibitems(path: Path) -> int:
    return len(re.findall(r"\\bibitem", read_text(path)))


def extract_pdf_text(pdf: Path) -> tuple[int | None, str, str | None]:
    try:
        import fitz  # type: ignore
    except Exception as exc:
        return None, "", f"PyMuPDF unavailable: {exc}"
    try:
        doc = fitz.open(pdf)
        text = "\n".join(page.get_text() for page in doc)
        return doc.page_count, text, None
    except Exception as exc:
        return None, "", f"PDF text extraction failed: {exc}"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Validate a BJTU thesis PDF build.")
    parser.add_argument("--project", required=True, help="BJTU project directory containing main.pdf/main.log.")
    parser.add_argument("--pdf", help="PDF path. Defaults to PROJECT/main.pdf.")
    parser.add_argument("--must-contain", action="append", default=[], help="Text that must appear in extracted PDF text.")
    parser.add_argument("--min-pages", type=int, default=1)
    parser.add_argument("--min-refs", type=int, default=0)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    project = Path(args.project).expanduser().resolve()
    pdf = Path(args.pdf).expanduser().resolve() if args.pdf else project / "main.pdf"
    log = project / "main.log"
    bbl = project / "main.bbl"
    failures: list[str] = []
    warnings: list[str] = []

    if not pdf.exists() or pdf.stat().st_size == 0:
        failures.append(f"Missing or empty PDF: {pdf}")
    log_text = read_text(log)
    for pattern in CRITICAL_LOG_PATTERNS:
        if re.search(pattern, log_text):
            failures.append(f"Critical log pattern matched: {pattern}")

    refs = count_bibitems(bbl)
    if refs < args.min_refs:
        failures.append(f"Reference count {refs} is below --min-refs {args.min_refs}")

    pages = None
    text = ""
    if pdf.exists():
        pages, text, extraction_warning = extract_pdf_text(pdf)
        if extraction_warning:
            warnings.append(extraction_warning)
        if pages is not None and pages < args.min_pages:
            failures.append(f"PDF page count {pages} is below --min-pages {args.min_pages}")
        if text.count("??") > 0:
            failures.append("Extracted PDF text contains ??")
        for needle in args.must_contain:
            if text and needle not in text:
                failures.append(f"PDF text does not contain required string: {needle}")

    print(f"pdf={pdf}")
    print(f"pages={pages if pages is not None else 'unknown'}")
    print(f"chars={len(text)}")
    print(f"refs={refs}")
    for warning in warnings:
        print(f"warning={warning}")
    if failures:
        for failure in failures:
            print(f"failure={failure}", file=sys.stderr)
        raise SystemExit(1)
    print("validation=ok")


if __name__ == "__main__":
    main()
