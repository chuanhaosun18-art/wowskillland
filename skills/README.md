# skills/ —— 技能内容仓库

这里存放所有技能的内容资料。**每提一个技能 = 一个文件夹 + 一次 PR**。

## 目录结构

```
skills/
├── _template/            ← 模板（不要改），复制成你技能的文件夹
│   └── skill.json.example
└── 你的技能名/
    ├── skill.json        ← 必填，把 _template/skill.json.example 复制过来改
    ├── skill.zip         ← 选填，技能包（提示词/模板/工具文件）
    └── proofs/           ← 选填，证明这个技能有用的截图
```

## 提交流程

1. fork 本仓库：https://github.com/chuanhaosun18-art/skillhub
2. clone 后，在 `skills/` 下新建 `你的技能名/` 文件夹
3. 复制 `skills/_template/skill.json.example` 为 `skill.json` 并填写
4. 可选：放入 `skill.zip` 和 `proofs/` 截图
5. 提交、push 到你的 fork，向 `main` 分支提 Pull Request
6. 合入后，维护者会拉取代码并运行导入工具把技能上架

## 填写规范

字段含义、质量分规则、常见问题，全部见 [docs/IMPORT_GUIDE.md](../docs/IMPORT_GUIDE.md)。

## 注意

- `skill.json` 必须是 UTF-8 编码，且是合法 JSON（可用 json.cn 校验）。
- `name` 不能与市场上已有技能重名（重名的会被跳过，除非维护者用 -force 覆盖）。
- 不要放超过仓库容忍大小的二进制文件；zip 建议不超过 20MB。
- `_template` 和 `README.md` 不会被导入，放心留在仓库里。
