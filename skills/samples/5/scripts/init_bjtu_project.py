#!/usr/bin/env python3
from __future__ import annotations

import argparse
import shutil
from pathlib import Path

from migrate_to_bjtu_template import DEFAULT_TEMPLATE, Metadata, build_main_tex, copy_template_core, safe_output_dir, write_text


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Create a blank BJTU template thesis project.")
    parser.add_argument("--output", required=True, help="Output BJTU project directory.")
    parser.add_argument("--template", default=str(DEFAULT_TEMPLATE), help="BJTU template directory.")
    parser.add_argument("--force", action="store_true", help="Replace a non-empty output directory.")
    parser.add_argument("--class-option", default="EnMaster", choices=["AcMaster", "EnMaster", "Doctor"])
    parser.add_argument("--author", default="")
    parser.add_argument("--student-number", default="")
    parser.add_argument("--advisor", default="")
    parser.add_argument("--advisor-title", default="")
    parser.add_argument("--engineering-master-field", default="")
    parser.add_argument("--degree-type", default="")
    parser.add_argument("--major", default="")
    parser.add_argument("--research-area", default="")
    parser.add_argument("--title", default="中文题名")
    parser.add_argument("--english-title", default="English Title")
    parser.add_argument("--date", default="")
    parser.add_argument("--toc-line-pt", type=int, default=20)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    output = Path(args.output).expanduser().resolve()
    template = Path(args.template).expanduser().resolve()
    if not template.exists():
        raise SystemExit(f"Template directory does not exist: {template}")
    safe_output_dir(output, args.force)
    copy_template_core(template, output)
    if (template / "reference" / "ref.bib").exists():
        shutil.copy2(template / "reference" / "ref.bib", output / "reference" / "ref.bib")
    else:
        write_text(output / "reference" / "ref.bib", "")

    write_text(
        output / "chapters" / "abstract.tex",
        "\\begin{abstract}\n请在此填写中文摘要。\n\n\\noindent\\keywords{关键词一；关键词二；关键词三}\n\\end{abstract}\n",
    )
    write_text(
        output / "chapters" / "englishabstract.tex",
        "\\begin{englishabstract}\nPlease write the English abstract here.\n\n\\noindent\\englishkeywords{keyword1; keyword2; keyword3}\n\\end{englishabstract}\n",
    )
    write_text(output / "chapters" / "chapter01.tex", "\\chapter{绪论}\n请在此填写正文。\n")

    metadata = Metadata(
        class_option=args.class_option,
        author=args.author,
        student_number=args.student_number,
        advisor=args.advisor,
        advisor_title=args.advisor_title,
        engineering_master_field=args.engineering_master_field,
        degree_type=args.degree_type,
        major=args.major,
        research_area=args.research_area,
        title=args.title,
        english_title=args.english_title,
        date=args.date,
        toc_line_pt=args.toc_line_pt,
    )
    write_text(output / "main.tex", build_main_tex(metadata, [r"\include{chapters/chapter01}"]))
    print(output)


if __name__ == "__main__":
    main()
