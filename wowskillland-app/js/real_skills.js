/* ============================================================
 * WowSkillLand · 真实开源 Skill 接入
 * ------------------------------------------------------------
 * 以下 Skill 提取自真实 GitHub 仓库 / SkillsMP 的 SKILL.md，
 * 内容（工作流、判断、边界）均来自源仓库原文，非虚构。
 * 每个条目带 real: { repo, url, install }，
 * 详情页会展示来源与安装命令。
 * ============================================================ */

var REAL_SKILLS = [
  {
    id: 'rs01', stageId: 'y3', type: 'Agent Skill', duration: '按面试周期',
    title: '面试应答军师 interview-response-coach',
    subtitle: '面试前 / 面试中 / 面试后全流程：JD 匹配、W-STAR、7 主线+3 支线、模拟面试、谈薪与 Offer 分析',
    creator: { name: 'entropy66', meta: 'GitHub 开源 · interview-response-coach', color: '#3a86ff', initial: 'E' },
    price: 0,
    match: '适用：准备/模拟面试、改自我介绍、写 STAR、谈薪、分析 Offer、处理离职原因/同事冲突/价值观题',
    script: [
      ['面试前', 'JD 匹配抽 4–6 个硬需求映射你的长板（长板优先，不补短板）· 公司调研卡（6 了解格 + 3 风控格，研究起点=投递日）· 灵魂三问 · W-STAR 逐字稿 ×2 · 口述计时（自我介绍 60–90 秒）'],
      ['面试中', '7 主线 + 3 支线，每题固定输出：映射到哪个"为什么"→ 考察底层逻辑 → 错误答法 → 正确结构 → 按你经历改写版 → 口述版'],
      ['模拟面试', 'mock 模式：按轮次（HR/业务主管/总经理）一问一答，每题 5 维评分（1–5）+ 1 个追问 + 1 版更好口述稿；结束输出薄弱环节 Top3 + 明天只练 3 件事'],
      ['面试后', '谈薪清单（CN/HK 市场适配）· Offer 5 维：公司前景/直属领导/职责/职业加持/通勤，禁止只看钱 · 输出可接受/谨慎接受/建议拒 + 1 条下一步']
    ],
    judge: '"公司招最合适的人，不是最优秀的人。所有问题最终映射到 3 个为什么——为什么这个行业、为什么这家公司、为什么我能胜任。杜绝纯题海。"（源仓库核心判断）',
    boundary: '不伪造经历或量化结果；不替你做最终接受/拒绝决策；只想"背题"会被拉回框架再进模拟逼练；价值观冲突可以一票否决业务亮点。',
    chooseIf: '要"能说出来"直接进模拟面试模式；只有 JD 还没开始准备的，从面试前流程走全套。',
    real: {
      repo: 'entropy66/interview-response-coach',
      url: 'https://github.com/entropy66/interview-response-coach',
      install: 'npx skills add entropy66/interview-response-coach'
    }
  },
  {
    id: 'rs02', stageId: 'g1', type: 'Agent Skill', duration: '每日运行',
    title: '科研军师 research-junshi · 每日文献雷达与选题参谋',
    subtitle: '读你的论文建研究画像，每天扫 arXiv + 顶会，生成排序过的大胆选题（含第一周最小实验）',
    creator: { name: 'junshi-research', meta: 'GitHub 开源 · research-junshi', color: '#5b5bd6', initial: 'J' },
    price: 0,
    match: '适用：想每天跟上文献、不知道下一个课题做什么、想把初步实验观察变成可发表方向的研究生',
    script: [
      ['首次配置', '对话式收集研究领域/问题/论文文件夹路径，逐篇读你的 PDF，抽取贡献、方法、开放问题，建立研究画像 profile.md'],
      ['每日 Step 1–2', '按你的 arXiv 分类扫近 24 小时 + 顶会检索（强制步骤，不可跳过），从约 100 篇候选选出 10 篇最相关'],
      ['每日 Step 3–4', '逐篇提炼：核心想法 / 关键洞察 / 留下的开放问题 / 与你工作的关联；军师模式生成 8–10 个具体可执行的选题（不是"探索 X"，而是"用洞察 W 做 Y 达到 Z"）'],
      ['每日 Step 5–7', '按 新颖×0.4 + 可行×0.3 + 影响×0.3 排序取 Top3–5，存为每日 digest，每个想法附"第一周最小实验"和"最可能失败的方式"']
    ],
    judge: '"一个反直觉的初步观察，比一篇写完的论文更能解锁好想法——先记录你的 preliminary results，基于它的选题对审稿人更有辩护力。"（源仓库原文）',
    boundary: '不替你写论文；画像为空时只能给领域通用建议；自动化 cron 脚本使用 --dangerously-skip-permissions，源仓库明确要求先审查脚本再在可信环境运行。',
    chooseIf: '刚进组没方向的先做首次配置喂论文；已有课题的把它当每日文献雷达用。',
    real: {
      repo: 'junshi-research/research-junshi',
      url: 'https://github.com/junshi-research/research-junshi',
      install: 'npx skills add junshi-research/research-junshi'
    }
  },
  {
    id: 'rs03', stageId: 'y1', type: 'Agent Skill', duration: '即时',
    title: '情圣 qingsheng-skill · 聊天回复与关系节奏军师',
    subtitle: '"在吗起手，必是小丑" —— 聊天记录逐条判读、朋友圈评论、邀约收口、已读不回应对',
    creator: { name: 'tomwong001', meta: 'GitHub 开源 · 基于 1TB+ 中文情感课程语料蒸馏', color: '#ff6b5e', initial: '情' },
    price: 0,
    match: '适用：盯着屏幕半小时不知道怎么回、被已读不回、朋友圈不知道怎么评论、邀约被"下次吧"挡住的时刻',
    script: [
      ['贴记录', '粘贴微信/探探/Soul/Bumble 聊天记录，逐条判读双方信号密度与投入比（IOI/IOD，基于亲代投资理论与 Gottman 互动研究）'],
      ['定阶段', '判断关系卡在哪个阶段、问题出在谁——常见诊断："两轮追时间，需求感直接拉满，她只能用万能挡箭牌收尾"'],
      ['给成品', '直接给可发送的回复/评论/收口话术，附发送时机与积极/含糊/不回应三种后续分支'],
      ['控节奏', '该冷的时候直接说"这条别回，冷两天"——变强化节奏不是玄学，是 Skinner 的实验结论']
    ],
    judge: '"点名 + 具体观察，不要写\'好可爱\'——\'好可爱\'是所有人都会写的，点名说明你真的看了。评论完别点赞：两个都做等于稀释了评论的力度。"（源仓库示例原文）',
    boundary: '源仓库明确定位：不是 PUA 教学、不教装高价值；本地运行不上传。平台补充边界：对方明确拒绝即停，未成年人相关一律拒绝。',
    chooseIf: '要"这句怎么回"的即时救场选这个；要长期关系策略与情绪支持，看狗头军师。',
    real: {
      repo: 'tomwong001/qingsheng-skill',
      url: 'https://github.com/tomwong001/qingsheng-skill',
      install: 'npx skills add tomwong001/qingsheng-skill'
    }
  },
  {
    id: 'rs04', stageId: 'y1', type: 'Agent Skill', duration: '持续',
    title: '恋爱教练 dating-coach（HowToGetAlongWithGirls）',
    subtitle: '吸引→平淡→暧昧→恋爱→长期的全周期路线图，带分级止损系统与自我诊断',
    creator: { name: 'Mayuqi-crypto', meta: 'GitHub 开源 · dating-coach 技能包（中英双语）', color: '#2e9e64', initial: 'M' },
    price: 0,
    match: '适用：想知道现在走到哪一步、分不清兴趣/礼貌/废测/拒绝、纠结该不该继续投入的阶段',
    script: [
      ['阶段诊断', '定位当前处于 吸引→平淡→暧昧→恋爱→长期 的哪一段，给出下一步动作'],
      ['聊天分析', '逐条解读信号，精准区分兴趣/礼貌/废测/拒绝——不把礼貌当兴趣，也不把废测当拒绝'],
      ['止损评估', '分级止损模型 + 量化评估表——知道什么时候该放弃，比知道怎么追更重要'],
      ['自我建设', '形象自诊（0–100）/ 聊天习惯测试 / 硬价值坦诚评估——话术不能逆天改命，自我建设才是根本']
    ],
    judge: '"真诚 > 技巧、尊重对方意愿、明显没戏就果断止损、自我建设才是根本。"（源仓库内置硬性底线原文）',
    boundary: '源仓库硬门槛：坚决拒绝欺骗、操控、贬低羞辱、未成年人、骚扰跟踪。对象档案用代号管理，个人数据与技能分离。',
    chooseIf: '要全周期方法论和"该不该止损"的判断选这个；要即时回复救场选情圣。',
    real: {
      repo: 'Mayuqi-crypto/HowToGetAlongWithGirls',
      url: 'https://github.com/Mayuqi-crypto/HowToGetAlongWithGirls',
      install: 'npx skills add Mayuqi-crypto/HowToGetAlongWithGirls'
    }
  },
  {
    id: 'rs05', stageId: 'y1', type: 'Agent Skill', duration: '即时',
    title: '狗头军师 goutoujunshi · 恋爱决策与情绪支持',
    subtitle: '先接住情绪，再分清事实，最后给能执行的选择——含冲突、退出与危机转介',
    creator: { name: 'powerycy', meta: 'GitHub 开源 · goutoujunshi', color: '#9a6700', initial: '狗' },
    price: 0,
    match: '适用：心动/暧昧/冲突/冷淡/投入失衡/分手复合等场景，尤其是情绪先崩了的时刻',
    script: [
      ['情绪落地', '2–4 句指出感受、触发点和冲突；高情绪时先缩小到"这一小时"或"发送前的动作"'],
      ['事实拆分', '已知事实 / 合理推测 / 关键未知分开列——只认可见原文，不补线下动作，不读心'],
      ['利益判断', '互惠、可靠、安全、可逆性、机会成本综合评估——不把"得到某个人"当唯一胜利'],
      ['行动收束', '一句首选 + 2–4 个理由，最多三版（稳健/策略/强势），并给一个现在能做的小动作与停止条件']
    ],
    judge: '"把\'对用户最有利\'理解为情绪稳定、安全、自尊、边界、互惠、时间精力、机会成本与未来选择权的综合利益。"（源仓库核心原则原文）',
    boundary: '源仓库安全边界：不诊断心理疾病；不保证话术让特定的人爱上你；家暴/跟踪/胁迫/自伤风险时先确认当下安全并转介可信支持或紧急服务。',
    chooseIf: '情绪需要先被接住的选这个；纯话术需求选情圣；要止损框架选恋爱教练。',
    real: {
      repo: 'powerycy/goutoujunshi',
      url: 'https://github.com/powerycy/goutoujunshi',
      install: 'npx skills add powerycy/goutoujunshi'
    }
  },
  {
    id: 'rs06', stageId: 'y1', type: 'Agent Skill', duration: '按次练习',
    title: '社交技能 AI 教练 social-skill-ai-coach',
    subtitle: '四段循环陪练：Analyze → Coach → Role-Play → Reflect，每段一个专职 Agent',
    creator: { name: 'john-data-chen', meta: 'GitHub 开源 · Kaggle AI Agents 参赛作品（Agents for Good）', color: '#5b5bd6', initial: 'S' },
    price: 0,
    match: '适用：想在真实开口前先排练一遍的社交场景——搭话、邀约、道歉、拒绝、小组破冰',
    script: [
      ['Analyze', '把模糊焦虑的处境结构化：谁/什么/哪里/渠道/场景类型/目标——这一步不给任何建议'],
      ['Coach', '只基于课程库（PEERS 式教程，RAG 检索选片）给具体到场景的建议和可直接说出口的开场白'],
      ['Role-Play', 'AI 扮演对方陪你实练，按你的社交水平做出真实反应——真实对话是一次性的，排练不是'],
      ['Reflect', '对照 rubric 逐维复盘 Role-Play 记录，给结构化的分维评估']
    ],
    judge: '"社交技能主要靠练习习得，但结构化练习稀缺且昂贵（PEERS 课程 $2,800–3,600）；真实对话高风险、机会一次性、几乎没人当场给你诚实反馈。"（源仓库问题定义原文）',
    boundary: '源仓库声明：概念性 MVP，不能替代执业心理师/治疗师；为高功能自闭/阿斯伯格人群的练习场景设计，重度社交焦虑请先寻求专业支持。',
    chooseIf: '与「破冰剧本」互补：剧本告诉你做什么动作，教练陪你把动作先练一遍。',
    real: {
      repo: 'john-data-chen/social-skill-ai-coach',
      url: 'https://github.com/john-data-chen/social-skill-ai-coach',
      install: 'npm install social-skills-coach-mcp'
    }
  },
  {
    id: 'rs07', stageId: 'y1', type: 'Agent Skill', duration: '按学习周期',
    title: '学习规划助手 learning-planner',
    subtitle: '学习路径设计 + 周/月计划 + 自测题生成 + 间隔重复，从入门到精通的里程碑拆解',
    creator: { name: 'chendongqi', meta: 'SkillsMP · OPB-Skills 合集（⭐107）', color: '#2e9e64', initial: 'C' },
    price: 0,
    match: '适用：学新技能没路径、备考没计划、碎片时间不知道怎么用、学了记不住的场景',
    script: [
      ['目标澄清', '明确学习目的（求职/兴趣/认证）+ 现状评估（现有基础与可用时间）'],
      ['路径规划', '技能树与前置依赖梳理，入门→进阶→精通分阶段，每阶段设里程碑与检查点'],
      ['计划输出', '周次 × 学习主题 × 每日时长 × 产出物 的可执行计划表 + 检验标准'],
      ['练习评估', '自测题库生成 · 间隔重复（1→3→7→14 天复习周期）· 费曼技巧：能教会别人才算真正掌握']
    ],
    judge: '"主动回忆优于重复阅读，交叉学习优于单一练习——测试自己，而不是重读笔记。"（源 SKILL.md 学习效率原则）',
    boundary: '规划不替代执行；资源推荐需自行甄别；备考类规划请以官方考纲为准。',
    chooseIf: '和「一周时间模板」互补：模板管节奏，规划助手管路径和内容。',
    real: {
      repo: 'chendongqi/OPB-Skills',
      url: 'https://skillsmp.com/zh/creators/chendongqi/opb-skills/skills-learning-development-learning-planner',
      install: 'npx skills add https://github.com/chendongqi/OPB-Skills --skill learning-development-learning-planner'
    }
  },
  {
    id: 'rs08', stageId: 'y3', type: 'Agent Skill', duration: '1–2 次会话',
    title: '职业方向诊断 career-direction-skill',
    subtitle: '拆清"想做 / 适合长期承担 / 当前能否进入"，形成有证据、有边界、可继续验证的方向判断',
    creator: { name: 'career-direction-lab', meta: 'GitHub 开源 · career-direction-skill', color: '#3a86ff', initial: '职' },
    price: 0,
    match: '适用：不知道自己想做什么/适合什么、想判断某个岗位值不值得继续、因一段实习考虑换方向',
    script: [
      ['拆三层', '把"想做、适合长期承担、当前能否进入"三件事分开诊断，不许一锅炖'],
      ['四项条件', '正式方向结论必须满足：工作事实、知情选择、真实经历、职业结果与代价边界'],
      ['方向比较', '多方向排序：优先级 + 主要代价 + 仍需验证的部分——模糊条件（高薪/稳定/爱与人打交道）不能直接映射到岗位'],
      ['第二层', '只有你明确同意后，才检查企业端的当前进入条件；新增/排除/收窄方向前都会先确认']
    ],
    judge: '"第一层不主动索取学校、学历、专业和实习背景——先看你怎么描述工作本身，再看门槛。"（源仓库诊断边界）',
    boundary: '源仓库边界：不输出录用概率；不编造团队氛围、公司情况和岗位事实；不负责简历修改、职位搜索、投递策略和面试诊断（面试的事交给面试军师）。',
    chooseIf: '和路口页互补：路口给你看走过的人的分叉，这个 Skill 帮你拆自己的判断依据。',
    real: {
      repo: 'career-direction-lab/career-direction-skill',
      url: 'https://github.com/career-direction-lab/career-direction-skill',
      install: 'npx skills add career-direction-lab/career-direction-skill'
    }
  },
  {
    id: 'rs09', stageId: 'y4', type: 'Agent Skill', duration: '按论文周期',
    title: '学位论文写作 paper-write（best-skills 合集）',
    subtitle: '大纲审核（理工/文科自动区分）、结构仿写、润色去 AI 化、BibTeX 参考文献、答辩信息提取',
    creator: { name: 'xstongxue', meta: 'GitHub 开源 · best-skills 合集', color: '#5b5bd6', initial: 'X' },
    price: 0,
    match: '适用：毕设/学位论文从大纲到统稿的全流程——尤其是"这段读起来像 AI 写的"时刻',
    script: [
      ['大纲审核', '「帮我审核一下这个论文大纲」——理工科/文科自动区分'],
      ['结构仿写', '按范文仿写绪论/摘要/实验章节（理工）或文献综述/案例分析/对策建议（文科）'],
      ['润色去 AI 化', '「这段读起来像 AI 写的，帮我润色」——防 AIGC 检测、扩写/缩写、中英互译'],
      ['参考文献与答辩', '「帮我找 RLHF 代表作并给 BibTeX」· 从论文提取结构化信息直接喂给答辩 PPT Skill']
    ],
    judge: '同一合集内配套 codegen-doc（根据项目代码生成系统设计章节）与 pptgen-drawio（答辩 PPT），三者常组合使用。',
    boundary: '润色和仿写不等于代写——学术诚信红线由你自己守；防 AIGC 功能用于让你自己写的内容更自然，不是用于绕过检测的代写。',
    chooseIf: '论文本体用这个；答辩 PPT 用同合集的 pptgen-drawio（平台内已单独收录）。',
    real: {
      repo: 'xstongxue/best-skills',
      url: 'https://github.com/xstongxue/best-skills',
      install: 'npx skills add xstongxue/best-skills --skill paper-write'
    }
  },
  {
    id: 'rs10', stageId: 'y4', type: 'Agent Skill', duration: '1–2 天',
    title: '答辩 PPT 生成 pptgen-drawio（best-skills 合集）',
    subtitle: '「帮我做答辩 PPT，论文在 xxx」→ 输出 .drawio，一键导出 .pptx，四种风格',
    creator: { name: 'xstongxue', meta: 'GitHub 开源 · best-skills 合集', color: '#ffb003', initial: 'X' },
    price: 0,
    match: '适用：毕业答辩、组会汇报、通用汇报——从论文/大纲直接生成结构化 PPT',
    script: [
      ['输入', '给论文文件或大纲：「帮我做答辩 PPT，论文在 xxx」「根据这个大纲生成汇报 PPT」'],
      ['风格', '经典学术 / 科技明快 等多种预置风格，输出 .drawio 源文件可继续手改'],
      ['导出', '用同仓库 drawio2pptx 导出 .pptx；配图可调用 drawio-diagram 画架构图/流程图'],
      ['配套', '先用 paper-write 的「结构化信息提取」把论文要点抽出来，效果最好']
    ],
    judge: '答辩 PPT 的常见死法是把论文每章复制进去——这个 Skill 按"答辩叙事"重组：问题→方法→实验→结论，每页一个主张。',
    boundary: '生成的是初稿骨架，数据图表准确性需自查；院系有固定模板的以院系模板为准。',
    chooseIf: '毕业答辩、组会汇报都能用；和「第一次组会汇报模板」互补——模板管内容结构，这个管出片。',
    real: {
      repo: 'xstongxue/best-skills',
      url: 'https://github.com/xstongxue/best-skills',
      install: 'npx skills add xstongxue/best-skills --skill pptgen-drawio'
    }
  },
  {
    id: 'rs11', stageId: 'g1', type: 'Agent Skill', duration: '按专利周期',
    title: '中文发明专利撰写 patent-write（best-skills 合集）',
    subtitle: '题目、摘要、背景技术、发明内容、权利要求、附图、具体实施方式——全流程或单章节',
    creator: { name: 'xstongxue', meta: 'GitHub 开源 · best-skills 合集', color: '#5b5bd6', initial: 'X' },
    price: 0,
    match: '适用：课题组要报专利、比赛/项目成果想落袋、导师说"把这个写成专利"但你从没写过的时刻',
    script: [
      ['写全文顺序', '先提炼创新点与保护对象 → 先搭权利要求骨架 → 再写摘要/发明内容 → 展开具体实施方式 → 最后统稿与术语一致性检查'],
      ['单章节', '题目优化/摘要/背景技术/权利要求/附图说明/实施例——说哪章补哪章'],
      ['附图绘制', '「帮我画专利附图」——Draw.io 黑白中文图，与图号、模块名称闭环'],
      ['参考蒸馏', '给一篇参考专利，蒸馏它的写法、结构和表达套路（不直接照抄）']
    ],
    judge: '"权利要求 1 负责总流程或核心模块，从属权利要求负责细化子步骤、参数、约束——先宽后窄；背景缺陷、发明目的、技术方案、有益效果要一一对应。"（源 SKILL.md 蒸馏出的高频规律）',
    boundary: '源 SKILL.md 硬约束：不编造——材料里没有的模块、步骤、实验数据、性能指标不写；信息不足以支撑正式文本时先列缺失项。正式申报请让代理人/导师终审。',
    chooseIf: '第一次写专利从"写全文顺序"走全套；已有草稿的直接进统稿和一致性检查。',
    real: {
      repo: 'xstongxue/best-skills',
      url: 'https://github.com/xstongxue/best-skills',
      install: 'npx skills add xstongxue/best-skills --skill patent-write'
    }
  },
  {
    id: 'rs12', stageId: 'y1', type: 'Agent Skill', duration: '按篇',
    title: '公众号/自媒体创作 wechat-article-writer（best-skills 合集）',
    subtitle: '9 种写作风格 + 封面图 + 正文插图 + 文风克隆——社团推文到个人号都能用',
    creator: { name: 'xstongxue', meta: 'GitHub 开源 · best-skills 合集', color: '#ff6b5e', initial: 'X' },
    price: 0,
    match: '适用：接了社团/校媒的推文活、想开个人号、要给活动写宣传稿但没写过公开内容的时刻',
    script: [
      ['选风格', '9 种风格按需切换：高流量爆款/清单体方法论/资源盘点/个人实测/认知颠覆/身份共鸣/故事化/深度随笔/默认'],
      ['搜资料', '并行搜多来源（官方文档/X/Reddit/技术论坛），优先当月最新资料，深度总结后再写'],
      ['出成品', '故事化开头 + 5 个备选爆款标题（痛点/数字/结果/情绪/悬念）+ 排版与配图位置建议'],
      ['配图', '封面图（1283×383 合并封面）与正文插图直接生成 .drawio，可一键导出 PNG'],
      ['文风克隆', '「分析这篇的写作风格」——把范文提炼成风格指南，之后照着写']
    ],
    judge: '写作之前先搜当月资料再动笔——过时素材是自媒体文的第一死因（源 SKILL.md 流程约束：Step 2 搜索先行）。',
    boundary: '生成的是初稿与标题备选，事实性内容需自查；蹭热点与标题党的度由你把握——工具不替你负责公众号的人设。',
    chooseIf: '和站内「社团试听周」剧本互补：剧本帮你选社团，这个帮你在社团把活干漂亮。',
    real: {
      repo: 'xstongxue/best-skills',
      url: 'https://github.com/xstongxue/best-skills',
      install: 'npx skills add xstongxue/best-skills --skill wechat-article-writer'
    }
  },
  {
    id: 'rs13', stageId: 'y4', type: 'Agent Skill', duration: '按图',
    title: 'Draw.io 图表生成 drawio-diagram（best-skills 合集）',
    subtitle: '模型架构图 / 算法流程图 / UML 时序图 / 考试示意图——从零生成或按参考图风格迁移',
    creator: { name: 'xstongxue', meta: 'GitHub 开源 · best-skills 合集', color: '#3a86ff', initial: 'X' },
    price: 0,
    match: '适用：毕设/论文要画 CNN、Transformer 架构图，课程报告要流程图、时序图，复习要函数图像/电路图的时刻',
    script: [
      ['从零生成', '「画一个 CNN 架构图」「画用户登录的时序图」「画 y=x² 的图像」——直出可编辑的 .drawio'],
      ['风格迁移', '上传参考图 + 描述内容 → 按参考图的排版/配色画新图'],
      ['质量约束', 'XML 标签严格闭合、唯一 id、特殊字符转义——保证 Draw.io 能直接打开编辑'],
      ['交付', '附使用指南：导出 PNG/SVG/PDF + 图题与论文引用示例']
    ],
    judge: '3D cube 节点画神经网络层、虚线画残差连接——论文图的"专业感"多半来自这两个约定（源 SKILL.md 样式规范）。',
    boundary: '生成的是可编辑初稿，专业图的模块命名与数据流向需自查；超复杂大图建议拆成多张。',
    chooseIf: '画"项目代码里已有的东西"用 codegen-diagram（自动读代码）；画"脑子里的概念"用这个。',
    real: {
      repo: 'xstongxue/best-skills',
      url: 'https://github.com/xstongxue/best-skills',
      install: 'npx skills add xstongxue/best-skills --skill drawio-diagram'
    }
  },
  {
    id: 'rs14', stageId: 'y4', type: 'Agent Skill', duration: '按图',
    title: '代码生成项目图表 codegen-diagram（best-skills 合集）',
    subtitle: '读你的项目代码，自动画技术栈图 / 系统架构图 / 数据结构图 / E-R 图',
    creator: { name: 'xstongxue', meta: 'GitHub 开源 · best-skills 合集', color: '#2e9e64', initial: 'X' },
    price: 0,
    match: '适用：毕设论文的"系统设计"章节要架构图和 E-R 图、答辩要技术栈图，而你不想手画的时刻',
    script: [
      ['技术栈图', '「根据当前项目画技术栈结构图」——自动识别组件选型'],
      ['系统架构图', '「画我们系统的四层架构图」——分层结构与调用流程'],
      ['数据结构 / E-R', '「根据数据库表结构画 E-R 图」——实体字段关系直出'],
      ['配套', '与 codegen-doc（生成系统设计章节文字）+ pptgen-drawio（答辩 PPT）同合集串联使用']
    ],
    judge: '图从代码里长出来，而不是凭记忆画——答辩被问"这个模块在哪"时，图和代码是对得上的。',
    boundary: '需要把项目代码给到 Agent 运行环境；对超大仓库先指定要画的模块范围。',
    chooseIf: '毕设系统章节三件套：codegen-diagram 画图 → codegen-doc 写章节 → pptgen-drawio 出答辩 PPT。',
    real: {
      repo: 'xstongxue/best-skills',
      url: 'https://github.com/xstongxue/best-skills',
      install: 'npx skills add xstongxue/best-skills --skill codegen-diagram'
    }
  },
  {
    id: 'rs15', stageId: 'g1', type: 'Agent Skill', duration: '按图',
    title: 'Excalidraw 手绘图 excalidraw-diagram（best-skills 合集）',
    subtitle: '手绘风架构图 / 流程图 / 研究框架白板草图——组会讨论和非正式分享的最佳画风',
    creator: { name: 'xstongxue', meta: 'GitHub 开源 · best-skills 合集', color: '#9a6700', initial: 'X' },
    price: 0,
    match: '适用：组会上要讲研究框架、任务分解，或要发给非技术同学看的示意图——正式图太重，手绘刚好',
    script: [
      ['描述输入', '按模板给：图类型 / 节点列表 / 关系（谁连谁、方向含义）/ 布局偏好'],
      ['自动建模', 'Agent 先整理"节点列表 + 关系列表"，再分层布局（同层水平对齐，层间递增）'],
      ['直出文件', '标准 .excalidraw JSON，浏览器 / Obsidian / VSCode 里直接打开继续编辑'],
      ['适用场景', '论文结构、研究框架、任务分解的发散式白板草图尤其合适']
    ],
    judge: '手绘风的价值是"看起来还没定稿"——组会上大家更敢对一张草图提意见，而不是一张精修图。',
    boundary: '不追求复杂自动排版，只给合理可读的初始位置——大图需要你在 Excalidraw 里手动微调。',
    chooseIf: '要进论文的图用 drawio-diagram；要上白板讨论的图用这个。',
    real: {
      repo: 'xstongxue/best-skills',
      url: 'https://github.com/xstongxue/best-skills',
      install: 'npx skills add xstongxue/best-skills --skill excalidraw-diagram'
    }
  }
];

/* ---------------- 注册进平台 ---------------- */
(function () {
  REAL_SKILLS.forEach(function (s) { DB.skills[s.id] = s; });

  function findTask(taskId) {
    for (var i = 0; i < DB.stages.length; i++) {
      var scenes = DB.stages[i].scenes;
      for (var j = 0; j < scenes.length; j++) {
        var tasks = scenes[j].tasks;
        for (var k = 0; k < tasks.length; k++) {
          if (tasks[k].id === taskId) return tasks[k];
        }
      }
    }
    return null;
  }
  function findScene(sceneId) {
    for (var i = 0; i < DB.stages.length; i++) {
      var scenes = DB.stages[i].scenes;
      for (var j = 0; j < scenes.length; j++) {
        if (scenes[j].id === sceneId) return scenes[j];
      }
    }
    return null;
  }

  /* 挂到已有任务 */
  [
    ['t-y3-4', 'rs01'],   // 第一份实习·简历 → 面试军师
    ['t-y3-5', 'rs01'],   // 面试复盘 → 面试军师
    ['t-g1-1', 'rs02'],   // 读文献 → 科研军师
    ['t-y1-1', 'rs06'],   // 破冰 → 社交教练陪练
    ['t-y1-5', 'rs07'],   // 时间节奏 → 学习规划
    ['t-y3-2', 'rs07'],   // 考研 14 天 → 学习规划
    ['t-y4-1', 'rs08'],   // offer 对比 → 职业方向诊断
    ['t-y4-4', 'rs09'],   // 论文查重避坑 → paper-write
    ['t-g1-2', 'rs10'],   // 组会汇报 → pptgen-drawio
    ['t-g1-2', 'rs15']    // 组会汇报 → excalidraw 手绘白板
  ].forEach(function (pair) {
    var t = findTask(pair[0]);
    if (t && t.skillIds.indexOf(pair[1]) < 0) {
      t.skillIds.push(pair[1]);
      if (t.wish) t.wish = false;  // 有真实 Skill 了，摘掉许愿池标记
    }
  });

  /* 新增任务 */
  var y1a = findScene('y1-a');
  if (y1a) y1a.tasks.push({
    id: 't-y1-7', title: '暧昧期不知道怎么聊',
    desc: '聊天记录分析 · 已读不回 · 邀约与止损——三个开源军师，风格任选',
    skillIds: ['rs03', 'rs04', 'rs05']
  });
  var y3a = findScene('y3-a');
  if (y3a) y3a.tasks.push({
    id: 't-y3-6', title: '不知道自己想做什么岗位',
    desc: '把"想做 / 适合长期承担 / 当前能否进入"拆清楚，再去投简历',
    skillIds: ['rs08']
  });
  var y4b = findScene('y4-b');
  if (y4b) {
    y4b.tasks.push({
      id: 't-y4-5', title: '毕业答辩 PPT',
      desc: '从论文自动生成答辩 PPT（.drawio → .pptx，四种风格）',
      skillIds: ['rs10', 'rs09']
    });
    y4b.tasks.push({
      id: 't-y4-6', title: '毕设的架构图 / 流程图 / E-R 图',
      desc: '从零画、按参考图画、或直接从项目代码里长出来',
      skillIds: ['rs13', 'rs14', 'rs15']
    });
  }
  var g1a = findScene('g1-a');
  if (g1a) g1a.tasks.push({
    id: 't-g1-6', title: '把成果写成发明专利',
    desc: '导师说"把这个报个专利"，而你从没写过——权利要求骨架先行',
    skillIds: ['rs11']
  });
  var y1aExtra = findScene('y1-a');
  if (y1aExtra) y1aExtra.tasks.push({
    id: 't-y1-8', title: '在社团接一个真活：写一期推文',
    desc: '一起干过活的交情比一起开过会的深，产出还能进简历',
    skillIds: ['rs12']
  });
})();
