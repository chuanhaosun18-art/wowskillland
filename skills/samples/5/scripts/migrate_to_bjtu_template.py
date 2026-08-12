#!/usr/bin/env python3
from __future__ import annotations

import argparse
import re
import shutil
from dataclasses import dataclass
from pathlib import Path


SKILL_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_TEMPLATE = SKILL_ROOT / "assets" / "BJTU-thesis-template"

ROOT_FILES = [
    "BJTU-thesis.cls",
    "GBT7714-2005NLang.bst",
    "titletoc.sty",
    "upgreek.sty",
    "schoolName.pdf",
    "schoolName.png",
    "LICENSE",
]


@dataclass
class Metadata:
    class_option: str
    author: str
    student_number: str
    advisor: str
    advisor_title: str
    engineering_master_field: str
    degree_type: str
    major: str
    research_area: str
    title: str
    english_title: str
    date: str
    toc_line_pt: int


def read_text(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def write_text(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


def strip_comments(text: str) -> str:
    lines: list[str] = []
    for line in text.splitlines():
        escaped = False
        out: list[str] = []
        for ch in line:
            if ch == "%" and not escaped:
                break
            out.append(ch)
            escaped = ch == "\\" and not escaped
            if ch != "\\":
                escaped = False
        lines.append("".join(out))
    return "\n".join(lines)


def safe_output_dir(output: Path, force: bool) -> None:
    if output.exists() and any(output.iterdir()):
        if not force:
            raise SystemExit(f"Output directory is not empty: {output}. Use --force to replace it.")
        shutil.rmtree(output)
    output.mkdir(parents=True, exist_ok=True)


def copy_template_core(template: Path, output: Path) -> None:
    for name in ROOT_FILES:
        src = template / name
        if src.exists():
            shutil.copy2(src, output / name)
    (output / "chapters").mkdir(parents=True, exist_ok=True)
    (output / "figures").mkdir(parents=True, exist_ok=True)
    (output / "reference").mkdir(parents=True, exist_ok=True)


def copy_optional_tree(src: Path, dst: Path) -> None:
    if src.exists():
        shutil.copytree(src, dst, dirs_exist_ok=True)


def find_source_tex(source: Path, explicit: str | None) -> Path:
    if explicit:
        tex = Path(explicit).expanduser().resolve()
    else:
        tex = source / "main.tex"
    if not tex.exists():
        raise SystemExit(f"Cannot find source TeX file: {tex}")
    return tex


def find_bib(source: Path, explicit: str | None) -> Path | None:
    if explicit:
        bib = Path(explicit).expanduser().resolve()
        if not bib.exists():
            raise SystemExit(f"Cannot find bibliography file: {bib}")
        return bib
    candidates = [
        source / "refs.bib",
        source / "ref.bib",
        source / "reference" / "ref.bib",
    ]
    return next((p for p in candidates if p.exists()), None)


def extract_between(text: str, start: str, end: str) -> str:
    if start not in text:
        return ""
    part = text.split(start, 1)[1]
    if end in part:
        part = part.split(end, 1)[0]
    return part.strip()


def clean_abstract_body(block: str, english: bool = False) -> tuple[str, str]:
    block = re.sub(r"\\addcontentsline\{toc\}\{chapter\}\{[^{}]+\}", "", block)
    block = block.replace(r"\begin{abstract}", "").replace(r"\end{abstract}", "")
    block = block.replace(r"\begin{englishabstract}", "").replace(r"\end{englishabstract}", "")
    lines = [line.rstrip() for line in block.splitlines()]
    body_lines: list[str] = []
    keywords = ""
    for line in lines:
        if any(marker in line for marker in ["关键词：", "关键词:", "Keywords:", "KEYWORDS:"]):
            cleaned = re.sub(r"\\noindent\s*", "", line)
            cleaned = cleaned.replace(r"\songti", "").replace(r"\heiti", "")
            cleaned = re.sub(r"\\textbf\{([^{}]*)\}", r"\1", cleaned)
            cleaned = cleaned.replace(r"\\", " ")
            if "：" in cleaned:
                keywords = cleaned.split("：", 1)[-1].strip()
            elif ":" in cleaned:
                keywords = cleaned.split(":", 1)[-1].strip()
        else:
            body_lines.append(line)
    body = "\n".join(body_lines).strip()
    if not keywords:
        keywords = "keyword1; keyword2; keyword3" if english else "关键词一；关键词二；关键词三"
    return body, keywords


def extract_abstracts(text: str) -> tuple[tuple[str, str], tuple[str, str]]:
    zh_block = ""
    en_block = ""
    if r"\chapter*{摘要}" in text:
        zh_block = extract_between(text, r"\chapter*{摘要}", r"\chapter*{ABSTRACT}")
        en_block = extract_between(text, r"\chapter*{ABSTRACT}", r"\tableofcontents")
    if not zh_block:
        m = re.search(r"\\begin\{abstract\}(.*?)\\end\{abstract\}", text, flags=re.S)
        zh_block = m.group(1).strip() if m else ""
    if not en_block:
        m = re.search(r"\\begin\{englishabstract\}(.*?)\\end\{englishabstract\}", text, flags=re.S)
        en_block = m.group(1).strip() if m else ""
    return clean_abstract_body(zh_block), clean_abstract_body(en_block, english=True)


def split_inline_chapters(text: str) -> list[str]:
    if r"\mainmatter" in text:
        body = text.split(r"\mainmatter", 1)[1]
    elif r"\tableofcontents" in text:
        body = text.split(r"\tableofcontents", 1)[1]
    else:
        body = text
    for marker in [r"\backmatter", r"\bibliography", r"\begin{thebibliography}"]:
        if marker in body:
            body = body.split(marker, 1)[0]
    matches = list(re.finditer(r"\\chapter\{[^{}]+\}", body))
    chapters: list[str] = []
    for idx, match in enumerate(matches):
        start = match.start()
        end = matches[idx + 1].start() if idx + 1 < len(matches) else len(body)
        chapters.append(body[start:end].strip())
    return chapters


def convert_chapter(block: str) -> str:
    block = block.replace(r"\begin{figure}[H]", r"\begin{figure}[!htbp]")
    block = block.replace(r"\begin{table}[H]", r"\begin{table}[!htbp]")
    block = block.replace(r"\bibliography{refs}", "")
    block = block.replace(r"\bibliography{reference/ref}", "")
    return block.strip() + "\n"


def collect_chapters(source: Path, tex_text: str) -> list[str]:
    chapters = split_inline_chapters(tex_text)
    if chapters:
        return [convert_chapter(ch) for ch in chapters]
    chapter_files = sorted((source / "chapters").glob("chapter*.tex"))
    return [convert_chapter(read_text(path)) for path in chapter_files]


def build_main_tex(metadata: Metadata, include_lines: list[str]) -> str:
    return rf"""\documentclass[{metadata.class_option}]{{BJTU-thesis}}

\setmainfont{{Times New Roman}}
\IfFontExistsTF{{SimSun}}{{
  \setCJKmainfont{{SimSun}}
  \setCJKfamilyfont{{zhsong}}[AutoFakeBold={{2.17}}]{{SimSun}}
}}{{
  \setCJKmainfont{{Songti SC}}
  \setCJKfamilyfont{{zhsong}}[AutoFakeBold={{2.17}}]{{Songti SC}}
}}
\renewcommand*{{\songti}}{{\CJKfamily{{zhsong}}}}

\usepackage{{longtable}}
\usepackage{{float}}
\usepackage{{tikz}}
\usetikzlibrary{{arrows.meta,positioning,fit,calc,shapes.geometric}}
\graphicspath{{{{figures/}}}}
\setlength{{\parskip}}{{0pt}}
\setlength{{\emergencystretch}}{{2em}}

% TOC entries: no before/after spacing, fixed {metadata.toc_line_pt} pt baseline.
\newcommand{{\bjtuTocFont}}{{\songti\fontsize{{12pt}}{{{metadata.toc_line_pt}pt}}\selectfont}}
\titlecontents{{chapter}}[0pt]{{\bjtuTocFont}}
{{\thecontentslabel\hspace{{\ccwd}}}}{{}}
{{\hspace{{.5em}}\titlerule*{{.}}\contentspage}}
\titlecontents{{section}}[2\ccwd]{{\bjtuTocFont}}
{{\thecontentslabel\hspace{{\ccwd}}}}{{}}
{{\hspace{{.5em}}\titlerule*{{.}}\contentspage}}
\titlecontents{{subsection}}[4\ccwd]{{\bjtuTocFont}}
{{\thecontentslabel\hspace{{\ccwd}}}}{{}}
{{\hspace{{.5em}}\titlerule*{{.}}\contentspage}}

\author{{{metadata.author}}}
\studentNumber{{{metadata.student_number}}}
\advisor{{{metadata.advisor}}}
\advisorTitle{{{metadata.advisor_title}}}
\engineeringMasterField{{{metadata.engineering_master_field}}}
\degreeType{{{metadata.degree_type}}}
\major{{{metadata.major}}}
\researchArea{{{metadata.research_area}}}
\title{{{metadata.title}}}
\englishtitle{{{metadata.english_title}}}
\datetime{{{metadata.date}}}

\begin{{document}}
\makecover
\makeInfo
\include{{chapters/abstract}}
\include{{chapters/englishabstract}}
\tableofcontents
\newpage\pagenumbering{{arabic}}
{chr(10).join(include_lines)}

\bibliography{{reference/ref}}

\end{{document}}
"""


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Migrate a LaTeX thesis project into the bundled BJTU template.")
    parser.add_argument("--source", required=True, help="Source LaTeX project directory.")
    parser.add_argument("--output", required=True, help="Output BJTU project directory.")
    parser.add_argument("--template", default=str(DEFAULT_TEMPLATE), help="BJTU template directory.")
    parser.add_argument("--tex", help="Source main .tex file. Defaults to SOURCE/main.tex.")
    parser.add_argument("--bib", help="Source .bib file. Defaults to refs.bib/ref.bib/reference/ref.bib.")
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
    source = Path(args.source).expanduser().resolve()
    output = Path(args.output).expanduser().resolve()
    template = Path(args.template).expanduser().resolve()
    if not source.exists():
        raise SystemExit(f"Source directory does not exist: {source}")
    if not template.exists():
        raise SystemExit(f"Template directory does not exist: {template}")

    safe_output_dir(output, args.force)
    copy_template_core(template, output)

    source_tex = find_source_tex(source, args.tex)
    tex_text = strip_comments(read_text(source_tex))
    copy_optional_tree(source / "figures", output / "figures")

    bib = find_bib(source, args.bib)
    if bib:
        shutil.copy2(bib, output / "reference" / "ref.bib")
    elif (template / "reference" / "ref.bib").exists():
        shutil.copy2(template / "reference" / "ref.bib", output / "reference" / "ref.bib")
    else:
        write_text(output / "reference" / "ref.bib", "")

    (zh_body, zh_keywords), (en_body, en_keywords) = extract_abstracts(tex_text)
    write_text(
        output / "chapters" / "abstract.tex",
        "\\begin{abstract}\n"
        + (zh_body or "请在此填写中文摘要。")
        + "\n\n\\noindent\\keywords{"
        + zh_keywords
        + "}\n\\end{abstract}\n",
    )
    write_text(
        output / "chapters" / "englishabstract.tex",
        "\\begin{englishabstract}\n"
        + (en_body or "Please write the English abstract here.")
        + "\n\n\\noindent\\englishkeywords{"
        + en_keywords
        + "}\n\\end{englishabstract}\n",
    )

    chapters = collect_chapters(source, tex_text)
    include_lines: list[str] = []
    for idx, chapter in enumerate(chapters, 1):
        name = f"chapter{idx:02d}"
        write_text(output / "chapters" / f"{name}.tex", chapter)
        include_lines.append(rf"\include{{chapters/{name}}}")
    if not include_lines:
        write_text(output / "chapters" / "chapter01.tex", "\\chapter{绪论}\n请在此填写正文。\n")
        include_lines.append(r"\include{chapters/chapter01}")

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
    write_text(output / "main.tex", build_main_tex(metadata, include_lines))
    write_text(
        output / "README.md",
        f"Generated BJTU project from {source_tex}.\n\n"
        f"- Template: {template}\n"
        f"- Class option: {args.class_option}\n"
        f"- Chapters: {len(include_lines)}\n"
        f"- Bibliography: reference/ref.bib\n",
    )
    print(output)


if __name__ == "__main__":
    main()
