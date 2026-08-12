# BJTU Thesis Autowriter

以北京交通大学学校 LaTeX 学位论文模板为核心的大论文自动写作 Skill。

本仓库是一个 **基于北京交通大学学校 LaTeX 模板的 thesis skill**：内置并围绕 `https://github.com/anabioticsoul/BJTU-thesis-template` 组织工作流，目标是把“项目脚本仓库、论文题目、简要项目介绍”转化为可持续迭代的北京交通大学硕士论文工程。它不仅能辅助完成项目材料盘点、证据映射、章节规划和正文初稿，还会优先生成符合 BJTU LaTeX 模板结构的工程、PDF 和 BibTeX 参考文献；Word/DOCX 则作为过程稿、反解析和辅助交付格式。

换句话说，这个 Skill 的主线不是“普通 Word 论文生成器”，而是 **BJTU LaTeX 模板驱动的论文写作、排版、编译和校验工作台**。

## 适用场景

- 根据代码仓库、实验脚本、数据处理流程和结果文件撰写北京交通大学硕士论文。
- 使用北京交通大学 LaTeX 模板生成规范 PDF。
- 使用模板内置 `BJTU-thesis.cls`、`GBT7714-2005NLang.bst` 和校名图等资源构建论文工程。
- 将已有 Word 草稿反解析为章节 Markdown、图片、表格和公式资产。
- 生成或续写 30000/35000 字以上中文硕士论文主体内容。
- 管理论文图、表、公式、参考文献、代码和数据资产。
- 生成北京交通大学风格 Word 过程稿或辅助交付版本。
- 使用 GB/T 7714 风格组织参考文献，并通过 BibTeX 进入 BJTU LaTeX PDF。

## 理想输入

最少只需要：

1. 项目脚本仓库路径或 Git URL；
2. 论文题目；
3. 简要项目介绍或课题来源。

更完整的输入包括：

- 开题报告、中期报告、已有 Word 草稿或 PDF；
- 数据集说明、实验日志、结果表、模型配置；
- 已有图片、表格、公式和参考文献；
- 学校模板、学院格式要求或导师修改意见；
- 目标输出：优先 BJTU LaTeX/PDF，也可以同时输出 Word。

## 核心能力

### 1. BJTU LaTeX 模板驱动的 PDF 输出

仓库内置北京交通大学 LaTeX 模板资源：

```text
assets/BJTU-thesis-template/
```

来源：

```text
https://github.com/anabioticsoul/BJTU-thesis-template
```

关键文件：

- `BJTU-thesis.cls`
- `GBT7714-2005NLang.bst`
- `schoolName.pdf`
- `titletoc.sty`
- `upgreek.sty`

LaTeX/PDF 相关脚本：

```bash
python3 scripts/init_bjtu_project.py ...
python3 scripts/migrate_to_bjtu_template.py ...
python3 scripts/compile_pdf.py --project <latex_project> --copy-to <final.pdf>
python3 scripts/validate_pdf.py --project <latex_project> --must-contain "参考文献"
```

参考文献应使用 BibTeX：

```latex
\bibliography{reference/ref}
```

模板类文件已经指定：

```latex
\bibliographystyle{GBT7714-2005NLang}
```

因此最终 PDF 的参考文献页应由模板 `.bst` 自动生成，而不是手写参考文献列表。

### 2. 项目仓库到论文证据

Skill 会先扫描仓库，而不是直接写正文。

内置脚本：

```bash
python3 scripts/inspect_project_repo.py <repo_path> \
  --out <project_dir>/09_state/repo_inspection.json \
  --md <project_dir>/00_project/repo_inspection.md
```

它会生成：

- 文件类型统计；
- README 预览；
- 训练、评估、模型、数据、增强、部署和结果相关候选文件；
- 可用于论文写作的代码、数据、图片和文档资产清单。

后续写作会基于证据表展开，避免把没有依据的功能、实验结果或指标写进论文。

### 3. 中文硕士论文写作组织

项目工作区包含：

```text
00_project/   项目事实、总索引、决策记录、证据表
01_template/  Word 原始模板
03_chapters/  章节计划和章节正文 Markdown
04_figures/   图片资产
05_tables/    表格资产
06_code/      代码资产说明
07_data/      数据和实验结果说明
08_refs/      参考文献和检索记录
09_state/     XML、反解析、结构化状态
10_output/    生成的 Word 输出
```

章节正文推荐写入：

```text
03_chapters/ch01_draft.md
03_chapters/ch02_draft.md
...
```

支持常见硕士论文结构：

```text
1 绪论
2 理论基础与研究现状
3 数据与问题建模
4 方法设计
5 实验与结果分析
6 系统实现或工程应用
7 总结与展望
```

### 4. Word/DOCX 辅助工作流

可以从 Word 模板初始化项目：

```bash
python3 scripts/init_thesis_workspace.py . --template <docx_path>
```

可以反解析已有 Word：

```bash
python3 scripts/reverse_parse_docx.py <project_dir> --docx <user_thesis.docx>
```

可以从 Markdown 写回 Word：

```bash
python3 scripts/flat_opc_converter.py toxml 01_template/original_template.docx 01_template/template.flat.xml
python3 scripts/parse_template_xml.py 01_template/template.flat.xml 09_state/parsed_structure.json
python3 scripts/apply_markdown_to_xml.py . --out 09_state/current_working.xml
python3 scripts/build_new_docx.py . --name thesis_draft_v1.docx
python3 scripts/validate_xml_docx.py .
```

生成文件只写入 `10_output/`，不会覆盖原始模板。Word 工作流主要用于已有草稿反解析、过程稿输出和与导师交流；正式 PDF 输出优先走 BJTU LaTeX 模板。

## 重要原则

- 不编造真实文献。
- 不编造实验结果、样本量、指标或数据来源。
- 不覆盖用户原始 Word、LaTeX、代码或数据文件。
- 对没有依据的内容标记为 `待确认` 或 `待补充`。
- 技术结论应能追溯到代码、数据、实验日志、图表、文献或用户确认。
- 正式 PDF 优先使用 BJTU LaTeX 模板生成，Word 是辅助输出。
- Word 和 PDF 都是生成物，章节 Markdown、BibTeX 和资产清单应作为主要内容来源。

## Skill 触发方式

在 Codex 中可以这样使用：

```text
使用 bjtu-thesis-autowriter，根据这个项目仓库、论文题目和项目简介，帮我生成北交大硕士论文初稿。
```

示例：

```text
使用 bjtu-thesis-autowriter。
项目仓库：/path/to/project
论文题目：基于多尺度残差注意力元学习的风机变桨系统故障诊断方法研究
项目简介：课题来源于校企合作项目，研究对象为风电机组变桨系统 SCADA 数据和故障日志，目标是实现跨风场小样本故障诊断。
输出：BJTU LaTeX 工程和 PDF，同时生成 Word 过程稿。
```

## 目录说明

```text
SKILL.md                         Skill 总入口和端到端流程
agents/openai.yaml               UI 展示元数据
references/repo_to_thesis_workflow.md
                                  项目仓库到论文证据的流程
references/writing_workflow.md   中文硕士论文写作规则
references/placeholders.md       图表公式引用占位符规则
references/reference_rules.md    参考文献规则
references/xml_mapping_spec.md   Word/XML 写回规则
references/bjtu_latex_workflow.md
                                  BJTU LaTeX/PDF 输出规则
scripts/                         Word、LaTeX、仓库扫描和校验脚本
templates/                       项目 manifest 模板
examples/Template.docx           默认 Word 模板
assets/BJTU-thesis-template/     BJTU LaTeX 模板资源
```

## 当前定位

这个仓库不是单纯的 Word 工具，也不是单纯的论文写作提示词集合，而是以 **北京交通大学 LaTeX 模板** 为最终排版目标的大论文自动写作工作流。它把项目仓库、实验资产、论文写作、BibTeX 参考文献、Word 过程稿和 BJTU LaTeX/PDF 输出串成一个可持续迭代的流程。
