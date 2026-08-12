---
name: bjtu-thesis-autowriter
description: Beijing Jiaotong University school LaTeX-template-based Chinese master's thesis autowriter. It uses the bundled BJTU-thesis-template, BJTU-thesis.cls, GBT7714-2005NLang.bst, BibTeX, and PDF validation as the primary output path, while also supporting Word/DOCX drafts. Use when the user asks to write, plan, expand, revise, or generate a BJTU/北交大 master's thesis or long report from project repositories, experiment scripts, datasets, notes, Word drafts, or LaTeX templates, especially for 30000+ or 35000+ Chinese theses with citations and BJTU formatting.
---

# BJTU Thesis Autowriter

This skill is the end-to-end thesis layer built around the Beijing Jiaotong University LaTeX thesis template. It combines:

- project/repository understanding and evidence mapping;
- Chinese master's thesis planning and drafting;
- Word/DOCX reverse parsing and generation;
- BJTU LaTeX/PDF generation using the bundled `BJTU-thesis-template`;
- GB/T 7714 reference handling through the BJTU template's `GBT7714-2005NLang.bst`.

Use this skill when the user wants a large BJTU-style thesis/report from a code repository, thesis title, project background, existing Word/PDF, figures, tables, formulas, or literature material.

## Core Promise

The ideal user input can be small:

1. project repository path or Git URL;
2. thesis title;
3. short project description, research background, or task source.

From that, build a thesis workspace, inspect the repo, extract defensible claims from code/data/results, plan chapters, draft most reusable text, and generate Word and/or BJTU LaTeX/PDF outputs.

## Non-Negotiable Rules

- Do not fabricate real literature, experiment results, sample sizes, metrics, datasets, company facts, code behavior, or platform functions.
- Every technical claim about the user's project should be traceable to a repository file, user-provided material, generated figure/table, or explicit user confirmation.
- If evidence is missing, write `待补充` / `待确认` and record it in project manifests.
- Do not overwrite original Word, LaTeX, data, code, or template files.
- New DOCX files go under `10_output/`. Generated LaTeX/PDF projects go under a separate output directory.
- For final BJTU PDF, use the bundled `assets/BJTU-thesis-template/` and BibTeX style `GBT7714-2005NLang`.

## First Response Workflow

If the user provides enough to begin, start work. If key inputs are missing, ask only the minimum questions needed, usually:

- repository path or Git URL;
- thesis title;
- brief project description or task source;
- desired output: Word, PDF, LaTeX project, or all.

If a repository path is provided, inspect it before proposing chapter content.

## Standard Pipeline

1. **Create Workspace**
   - Create an independent thesis project folder.
   - Run Word workspace initialization when DOCX output is needed:
     `python3 <skill_dir>/scripts/init_thesis_workspace.py . --template <docx_path>`
   - If no Word template is provided, use `examples/Template.docx`.

2. **Ingest Existing Drafts**
   - If the user provides an existing Word draft and wants reuse, run:
     `python3 <skill_dir>/scripts/reverse_parse_docx.py <project_dir> --docx <user.docx>`
   - If the Word file is only a format template, initialize from it without reverse parsing.

3. **Inspect Project Repository**
   - Read `references/repo_to_thesis_workflow.md`.
   - Run:
     `python3 <skill_dir>/scripts/inspect_project_repo.py <repo_path> --out <project_dir>/09_state/repo_inspection.json --md <project_dir>/00_project/repo_inspection.md`
   - Manually inspect key files found by the script: README, configs, training/evaluation scripts, datasets, notebooks, result files, figures, logs.

4. **Build Evidence Map**
   - Record what each file supports: method, model, data, experiment, platform, figure, table, or limitation.
   - Keep speculative or unsupported claims out of final prose.

5. **Plan Thesis**
   - Read `references/writing_workflow.md` and `references/repo_to_thesis_workflow.md`.
   - Generate/update:
     - `00_project/project_manifest.md`
     - `00_project/thesis_master_index.md`
     - `00_project/decisions_log.md`
     - `03_chapters/chXX_plan.md`
   - For a full Chinese engineering master's thesis, a common structure is:
     `1 绪论`, `2 理论基础与研究现状`, `3 数据与问题建模`, `4 方法设计`, `5 实验与结果分析`, `6 系统实现或工程应用`, `7 总结与展望`.

6. **Literature and References**
   - Use real references only. Search the web when current papers, standards, policy documents, or official statistics are needed.
   - Prefer official pages, publisher pages, DOI pages, CNKI/万方/维普 metadata, arXiv, IEEE, Elsevier, Springer, ACM, MDPI, Nature, official government pages.
   - Store references in `08_refs/` for Word workflow and `reference/ref.bib` for LaTeX workflow.
   - For BJTU PDF, use BibTeX and `\bibliography{reference/ref}` rather than hand-written reference lists.

7. **Draft Chapters**
   - Write chapter prose in `03_chapters/chXX_draft.md`.
   - Use Chinese academic style, grounded in the evidence map.
   - Use placeholders from `references/placeholders.md` for figures, tables, formulas, symbols, and unresolved citations.
   - Do not hard-code final figure/table/formula/reference numbers in Markdown.

8. **Generate Word**
   - Read `references/xml_mapping_spec.md` before DOCX writeback.
   - Run:
     - `python3 <skill_dir>/scripts/flat_opc_converter.py toxml 01_template/original_template.docx 01_template/template.flat.xml`
     - `python3 <skill_dir>/scripts/parse_template_xml.py 01_template/template.flat.xml 09_state/parsed_structure.json`
     - `python3 <skill_dir>/scripts/apply_markdown_to_xml.py . --out 09_state/current_working.xml`
     - `python3 <skill_dir>/scripts/build_new_docx.py . --name thesis_draft_v1.docx`
     - `python3 <skill_dir>/scripts/validate_xml_docx.py .`

9. **Generate BJTU LaTeX/PDF**
   - Read `references/bjtu_latex_workflow.md` and `references/bjtu-template-notes.md`.
   - Use scripts copied from `bjtu-thesis-template`:
     - `python3 <skill_dir>/scripts/init_bjtu_project.py ...`
     - `python3 <skill_dir>/scripts/migrate_to_bjtu_template.py ...`
     - `python3 <skill_dir>/scripts/compile_pdf.py --project <project> --copy-to <final.pdf>`
     - `python3 <skill_dir>/scripts/validate_pdf.py --project <project> --must-contain "参考文献"`

10. **Validate and Report**
    - Check output paths, page count, table of contents, reference generation, missing citations, `??`, missing figures, formulas, and obvious placeholder residue.
    - Report what was generated, what evidence was used, and what remains `待确认`.

## When To Read Extra References

- Project repository ingestion: `references/repo_to_thesis_workflow.md`
- Chinese thesis planning/writing: `references/writing_workflow.md`
- Placeholders for figures/tables/formulas/references: `references/placeholders.md`
- Reference and GB/T 7714 handling: `references/reference_rules.md`
- Word/XML generation: `references/xml_mapping_spec.md`
- BJTU LaTeX/PDF generation: `references/bjtu_latex_workflow.md`
- Template warning triage: `references/bjtu-template-notes.md`

## Output Discipline

For large theses, keep a Markdown source of truth first. Word and LaTeX/PDF are generated artifacts. If the user edits Word manually later, reverse parse or explicitly reconcile the edits before regenerating outputs.

For a 35000+ word thesis, draft incrementally by chapter and validate often. Do not wait until the end to discover missing evidence, broken figures, or invalid references.
