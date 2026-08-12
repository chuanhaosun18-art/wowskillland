# Repository To Thesis Workflow

This reference defines how to turn a user-provided code/script repository into defensible thesis content.

## Input Contract

Minimum usable input:

- repository path or Git URL;
- thesis title;
- 3-10 sentence project description, task source, or research background.

Useful optional inputs:

- target degree type and major;
- project proposal, midterm report, Word draft, slides, notes;
- dataset location and whether data may be inspected;
- experiment logs, result tables, model checkpoints, generated figures;
- required word count and output format.

## Repository Inspection

Start with fast non-destructive inspection:

```bash
rg --files <repo>
python3 <skill_dir>/scripts/inspect_project_repo.py <repo> --out <project>/09_state/repo_inspection.json --md <project>/00_project/repo_inspection.md
```

Then manually read likely high-value files:

- README, docs, project reports, notebooks;
- package/config files: `pyproject.toml`, `requirements.txt`, `environment.yml`, `package.json`, `Dockerfile`;
- training/evaluation scripts: names containing `train`, `test`, `eval`, `infer`, `predict`, `main`;
- model/method files: names containing `model`, `network`, `loss`, `dataset`, `sampler`, `augment`, `meta`, `attention`;
- experiment outputs: `results`, `logs`, `runs`, `outputs`, `figures`, `checkpoints`;
- data schemas or sample data if allowed.

Do not execute heavy training or data mutation by default. If execution is needed, prefer read-only commands, small smoke tests, or scripts that only summarize existing output.

## Evidence Map

Maintain an evidence map in `00_project/evidence_map.md` or `09_state/evidence_map.json`.

Recommended columns:

- `claim_id`: stable ID such as `C-METHOD-001`;
- `claim`: thesis statement that can appear in prose;
- `evidence_type`: code, config, data, result, figure, user-confirmed, literature;
- `source`: file path, line number if available, URL, or user message;
- `chapter`: target chapter/section;
- `confidence`: confirmed, inferred, weak, pending;
- `gap`: what still needs confirmation.

Example:

```markdown
| claim_id | claim | evidence_type | source | chapter | confidence | gap |
| --- | --- | --- | --- | --- | --- | --- |
| C-METHOD-001 | 本文模型包含多尺度卷积特征提取模块 | code | `models/ms_resmeta.py:34` | 第5章 | confirmed | 无 |
| C-RESULT-002 | 方法在山东风场数据上优于CNN基线 | result | `results/compare.csv` | 第6章 | pending | 需要确认指标含义 |
```

## Thesis Claim Policy

Use these categories:

- **Confirmed**: directly supported by code, data, result files, existing document, or user confirmation.
- **Inferred**: strongly implied by code structure or config. Phrase as “从实现结构看” or verify before final output.
- **Background**: supported by real literature or official documents.
- **Placeholder**: not yet supported. Mark as `待确认` or `待补充`.

Never turn a placeholder into a final claim.

## Chapter Mapping From Repository

Typical mappings:

- README / proposal / project background -> Chapter 1.
- literature notes and collected refs -> Chapters 1-2.
- data loaders, preprocessing, label mapping -> Chapter 3.
- model code, algorithm scripts, losses, samplers -> Chapters 4-5.
- experiment scripts, configs, result CSV/logs/figures -> Chapter 6.
- platform, API, visualization, deployment scripts -> system/application chapter.
- limitations, TODOs, failed runs, missing results -> future work.

## Drafting From Code

When turning code into prose:

- describe purpose before implementation details;
- use Chinese academic language;
- include inputs, outputs, process, assumptions, and limitations;
- use equations only when the method actually requires them;
- cite literature for general methods, but cite repo evidence for project-specific implementation.

Bad:

> 本文方法显著提升了诊断性能。

Better:

> 代码实现中包含基于类别权重的损失函数配置，但当前仓库未提供完整对比结果，因此性能提升幅度仍需结合实验日志进一步确认。

## Experiment Result Rules

Only write concrete metrics when at least one of these exists:

- result table or CSV;
- log with metric names and values;
- figure generated from real result data;
- user-provided metric values;
- reproducible script successfully executed on available data.

If metrics are missing, write experiment design and expected evaluation protocol, not invented results.

## Literature Rules

Repository-derived project content does not replace literature review. For background and related work:

- search current, real literature when the user requests a full thesis;
- record source metadata in `08_refs/` and later `reference/ref.bib`;
- distinguish “general domain research” from “this project's implementation”.

## Completion Criteria

Before generating final Word/PDF:

- `00_project/evidence_map.md` exists or equivalent evidence notes are in project manifests;
- chapter plans mention which repo assets support them;
- all concrete experiment metrics are traceable;
- unresolved items remain marked `待确认`/`待补充`;
- references are real and cited in text.
