# BJTU Template Notes

## Bundled Template

The bundled template is copied from `https://github.com/anabioticsoul/BJTU-thesis-template` and includes the upstream MIT `LICENSE`.

Core files:

- `BJTU-thesis.cls`: BJTU class based on `ctexbook`.
- `GBT7714-2005NLang.bst`: GB/T 7714 numeric bibliography style.
- `titletoc.sty`, `upgreek.sty`: local style dependencies shipped by the template.
- `schoolName.pdf`, `schoolName.png`: cover assets.

## Class Options

- `AcMaster`: academic master's thesis.
- `EnMaster`: professional master's thesis.
- `Doctor`: doctoral thesis.

For professional degree master's reports, prefer `EnMaster` unless the user explicitly requests another option.

## Citation And Bibliography

The class loads:

```tex
\RequirePackage[numbers,square,comma,super,sort&compress]{natbib}
\bibliographystyle{GBT7714-2005NLang}
```

Therefore plain `\cite{...}` produces numeric superscript citations. Do not add a separate citation redefinition unless a project has a concrete conflict.

## TOC Override Used By This Skill

When the user asks for directory entries with no before/after spacing and fixed 20 pt line spacing, write after package setup:

```tex
\newcommand{\bjtuTocFont}{\songti\fontsize{12pt}{20pt}\selectfont}
\titlecontents{chapter}[0pt]{\bjtuTocFont}
{\thecontentslabel\hspace{\ccwd}}{}
{\hspace{.5em}\titlerule*{.}\contentspage}
\titlecontents{section}[2\ccwd]{\bjtuTocFont}
{\thecontentslabel\hspace{\ccwd}}{}
{\hspace{.5em}\titlerule*{.}\contentspage}
\titlecontents{subsection}[4\ccwd]{\bjtuTocFont}
{\thecontentslabel\hspace{\ccwd}}{}
{\hspace{.5em}\titlerule*{.}\contentspage}
```

## Compile Notes

Use Tectonic when system `xelatex`/`bibtex` are unavailable. A reliable sequence is:

```bash
tectonic --print --keep-intermediates --keep-logs --reruns 0 main.tex
tectonic --print --keep-intermediates --keep-logs --reruns 1 main.tex
```

Harmless warnings after final validation:

- macOS CJK font `Script "CJK"` warnings.
- `fancyhdr` warning about `E` option in one-sided mode.
- `schoolName.pdf` version 1.6 included into PDF 1.5.
- Tectonic attempting BibTeX on chapter `.aux` files and reporting missing `\bibdata`/`\bibstyle`.
- `ToUnicode CMap` warnings for some macOS fonts, if visual PDF and extracted core text are valid.

Blocking signs:

- `Package natbib Warning: There were undefined citations` in the final log.
- `Citation ... undefined` in the final log.
- `LaTeX Warning: There were undefined references` in the final log.
- Missing `main.bbl`, missing `main.pdf`, or zero `\bibitem` entries when references are expected.
- Extracted PDF text contains `??`.
