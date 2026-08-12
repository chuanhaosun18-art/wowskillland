/* ============================================================
 * WowSkillLand · API 适配层
 * ------------------------------------------------------------
 * 所有与后端 / AI 的交互都收口在这个文件。
 * 前端页面只调用 WowAPI.xxx()，不直接 fetch。
 *
 * 已对接真实后端：skillhub-backend（Go + Gin + SQLite）
 *   启动：cd skillhub-backend/backend
 *         SKILLHUB_DATA=<数据目录> DEEPSEEK_API_KEY=sk-... ./backend
 *   没配 DEEPSEEK_API_KEY 也能跑：意图路由会降级为 manual_fallback，
 *   工作台推进会降级为 degraded（手动记录），其余接口不受影响。
 *
 * 真实模式：
 *   WowConfig.USE_MOCK = false  → 已登录走 DeepSeek 意图路由 / 工作台
 *   未登录仍可用本地关键词路由浏览，点「出发」会提示登录
 * ============================================================ */

var WowConfig = {
  USE_MOCK: false,                         // 接 skillhub 真实后端
  API_BASE: 'http://localhost:8080/api',
  LIVE: false,                             // /health 探测后置 true
  TOKEN: localStorage.getItem('wow_token') || '',
  USER: (function () {
    try { return JSON.parse(localStorage.getItem('wow_user') || 'null'); }
    catch (e) { return null; }
  })()
};

var AI_LEVEL_LABEL = {
  never: '从未用过 AI',
  beginner: 'AI 初级',
  intermediate: 'AI 中级',
  advanced: 'AI 高级'
};

function gradeToStage(g) {
  g = g || '';
  if (/高考|大0|志愿|择校/.test(g)) return 'y0';
  if (/大一/.test(g)) return 'y1';
  if (/大二/.test(g)) return 'y2';
  if (/大三/.test(g)) return 'y3';
  if (/大四/.test(g)) return 'y4';
  if (/研/.test(g)) return 'g1';
  return 'y1';
}

function applyUserToLocal(user) {
  if (!user || typeof DB === 'undefined' || !DB.user) return;
  DB.user.name = user.username || DB.user.name;
  DB.user.major = [user.school, user.major, user.grade].filter(Boolean).join(' · ') || DB.user.major;
  DB.user.stageId = gradeToStage(user.grade);
  DB.user.backend = user;
}

function setAuth(token, user) {
  WowConfig.TOKEN = token || '';
  WowConfig.USER = user || null;
  if (token) localStorage.setItem('wow_token', token);
  else localStorage.removeItem('wow_token');
  if (user) localStorage.setItem('wow_user', JSON.stringify(user));
  else localStorage.removeItem('wow_user');
  applyUserToLocal(user);
}

/* ---------- 请求封装 ---------- */
function wowReq(method, path, body) {
  var opt = {
    method: method,
    headers: { 'Content-Type': 'application/json' }
  };
  if (WowConfig.TOKEN) opt.headers['Authorization'] = 'Bearer ' + WowConfig.TOKEN;
  if (body != null) opt.body = JSON.stringify(body);
  return fetch(WowConfig.API_BASE + path, opt).then(function (r) {
    if (r.status === 401) {
      setAuth('', null);
      var expired = new Error('登录已过期，请重新登录');
      expired.needLogin = true;
      throw expired;
    }
    return r.json().then(function (j) {
      if (r.status === 409) { j._gate = true; return j; }
      if (!r.ok) throw new Error(j.error || ('API ' + path + ' ' + r.status));
      return j;
    }, function () {
      throw new Error('API ' + path + ' ' + r.status);
    });
  });
}
function wowGet(path) { return wowReq('GET', path); }
function wowPost(path, body) { return wowReq('POST', path, body || {}); }
function wowPut(path, body) { return wowReq('PUT', path, body || {}); }

/* growth 接口需要 JWT。未登录时抛 needLogin，由页面跳转登录，不再偷偷注册演示号。 */
function ensureLogin() {
  if (WowConfig.TOKEN) return Promise.resolve(WowConfig.TOKEN);
  var err = new Error('请先登录');
  err.needLogin = true;
  return Promise.reject(err);
}

/* mock 延迟，模拟网络/推理耗时 */
function mockDelay(data, ms) {
  return new Promise(function (resolve) {
    setTimeout(function () { resolve(data); }, ms == null ? 600 : ms);
  });
}

/* 关键词 → 阶段（mock 和真实模式共用：后端不管阶段概念，阶段是前端产品层的路由） */
function looksRomance(t) {
  t = t || '';
  if (looksFriendship(t) && !/恋爱|表白|暗恋|分手|在一起|该不该谈/.test(t)) return false;
  return /恋爱|表白|暗恋|分手|在一起|对象|该不该谈/.test(t) ||
    (/好感/.test(t) && !/交朋友|没朋友|孤独/.test(t)) ||
    (/喜欢/.test(t) && /他|她|一个人|同学/.test(t) && !/交朋友/.test(t));
}
function looksFriendship(t) {
  t = t || '';
  if (/恋爱|表白|暗恋|分手|该不该谈|在一起/.test(t) && !/交朋友|没朋友|孤独|社恐/.test(t)) return false;
  return /交朋友|没朋友|一个朋友|没有朋友|孤独|社恐|搭子|认识人|社交从零|想有朋友/.test(t);
}
function guessJunction(t) {
  if (looksFriendship(t)) return 'j-friend';
  if (looksRomance(t)) return 'j-love';
  if (/转专业|专业合不合适|适不适合这个专业/.test(t)) return 'j-major';
  if (/高考|报志愿|本省|外省|复读|择校/.test(t)) return 'j-y0';
  if (/毕业|offer|gap|延毕/.test(t)) return 'j-y4';
  return 'j-y3';
}

function looksReadyForTask(t) {
  t = t || '';
  if (/该不该|要不要选|值不值得|选哪一边|纠结要不要/.test(t) && !/怎么做|该怎么|应该怎么|怎么开始|怎么交|怎么开口/.test(t)) return false;
  return /我想做|帮我做|帮我写|帮我准备|帮我弄|开始做|怎么做|该怎么|应该怎么|我该怎么|怎么开始|怎么交|怎么开口|怎么认识|具体怎么|第一步|怎么准备|接下来怎么|有没有卡|装载|动手|试一张|匹配|教我|带我做|我想试试|我想开始|开始准备/.test(t);
}

function forkPack(id) {
  var j = id && typeof DB !== 'undefined' && DB.junctions[id];
  if (!j) return null;
  return {
    title: j.title, total: j.total, source: j.source, switch_note: j.switchNote,
    branches: (j.branches || []).map(function (b) {
      return {
        name: b.name,
        count: b.count,
        quotes: (b.quotes || []).map(function (q) { return (q.t || '') + (q.by ? ' —— ' + q.by : ''); })
      };
    })
  };
}

function localForkTalk(route, utterance) {
  var ctx = retrieveWowLocal(utterance, route, []);
  var jid = (route && route.junctionId) || guessJunction(utterance);
  var j = jid && DB.junctions[jid];
  var heard = (utterance || '').replace(/\s+/g, ' ').slice(0, 36);
  var lines = '听到你说「' + heard + '」。这类选择我不选边，也不把整张分叉表念一遍。';
  if (j) {
    lines += '\n\n和这件事对上的入口是「' + j.title + '」，' + j.total + ' 人走过。';
    var exp = ctx.experiences && ctx.experiences[0];
    if (exp && exp.quote) lines += '\n有人说过：' + exp.quote;
    if (j.switchNote) lines += '\n' + j.switchNote;
  }
  lines += '\n\n我先把这句话记下。你现在更卡的是哪一块，接着说就行。';
  return lines;
}

function looksLikeForkDump(text, forks) {
  text = text || '';
  if (/只把走过的人摊开/.test(text) || /你现在更卡哪一块/.test(text)) return true;
  if (/这批里共/.test(text) && /人：/.test(text)) return true;
  if (!forks || !forks.branches || forks.branches.length < 2) return false;
  var n = 0;
  forks.branches.forEach(function (b) {
    if (b.name && text.indexOf(b.name) >= 0) n++;
  });
  return n >= Math.min(3, forks.branches.length);
}

function localFollowupTalk(route, utterance, history) {
  var t = (utterance || '').trim();
  var prevUser = (history || []).filter(function (h) { return h && h.role === 'user' && h.content; })
    .map(function (h) { return h.content; });
  if (route && route.memory && route.memory.situation && prevUser.indexOf(route.memory.situation) < 0) {
    prevUser.unshift(route.memory.situation);
  }
  var jid = (route && route.junctionId) || (route && route.memory && route.memory.junction_id) || guessJunction(t);
  var j = jid && typeof DB !== 'undefined' && DB.junctions[jid];
  var head = '你前面说的是「' + ((prevUser[0] || (route && route.heard) || '').replace(/\s+/g, ' ').slice(0, 24)) +
    '」，现在问「' + t.slice(0, 24) + '」。我不选边，整张表也不再贴。';
  if (!j) return head + '\n接着说现在最卡住的那一点就行。';
  var scored = (j.branches || []).map(function (b) {
    var bl = b.name + ' ' + (b.quotes || []).map(function (q) { return (q.t || '') + ' ' + (q.by || ''); }).join(' ');
    var s = 0;
    if (/学业|课业|成绩|绩点|学习|这学期|考试|余量|影响/.test(t)) {
      if (/稳住|学期|课业/.test(b.name + bl)) s += 8;
      if (/余量/.test(bl)) s += 4;
      if (/排时间|乱/.test(bl)) s += 2;
    }
    if (/朋友|关系|弄没|开口|说出口|表白/.test(t) && /朋友|开口|说/.test(bl)) s += 5;
    if (/开始|在一起|处起来/.test(t) && /开始|处/.test(b.name)) s += 4;
    t.replace(/[，。？！、,.!?]/g, ' ').split(/\s+/).forEach(function (w) {
      if (w.length >= 2 && bl.indexOf(w) >= 0) s += 1;
    });
    return { b: b, s: s };
  }).sort(function (a, b) { return b.s - a.s; });
  var top = scored[0];
  if (!top || top.s <= 0) {
    return head + '\n完整分叉刚才已经摊过。你更想看哪一块的代价，说一块就行。';
  }
  var b = top.b;
  var q0 = b.quotes && b.quotes[0];
  var lines = head + '\n\n' + j.total + ' 人里，和这一问最相关的是「' + b.name + '」· ' + b.count + ' 人。';
  if (q0) lines += '\n有人说过：「' + q0.t + '」——' + q0.by;
  if (/学业|课业|成绩|这学期|时间|影响/.test(t) && j.switchNote) lines += '\n' + j.switchNote;
  var second = scored[1];
  if (second && second.s > 0) {
    var q1 = second.b.quotes && second.b.quotes[0];
    lines += '\n另一头「' + second.b.name + '」有 ' + second.b.count + ' 人' +
      (q1 ? '，原话是：「' + q1.t + '」' : '。');
  }
  lines += '\n\n要动手了就直接说「我该怎么做」，我去匹配能帮你做完的卡。';
  return lines;
}

function discussLocalReply(utterance, route, history, next) {
  if (next === 'match') return localTaskTalk(route, utterance);
  var follow = (history && history.length) || (route && route.memory && route.memory.situation);
  if (follow) return localFollowupTalk(route, utterance, history);
  return localForkTalk(route, utterance);
}

function retrieveWowLocal(utterance, route, history) {
  var blob = (utterance || '') + ' ' + ((route && route.heard) || '');
  (history || []).forEach(function (t) { if (t && t.content) blob += ' ' + t.content; });
  var jid = (route && route.junctionId) || guessJunction(blob);
  var j = jid && DB.junctions[jid];
  var experiences = [];
  var junctions = [];
  if (j) {
    junctions.push({ id: j.id, title: j.title, total: j.total });
    (j.branches || []).forEach(function (b) {
      (b.quotes || []).forEach(function (q) {
        var s = 0;
        if (/学业|课业|成绩|这学期|余量|时间/.test(utterance) && /学期|稳住|余量|时间|乱/.test(b.name + (q.t || ''))) s += 5;
        if (s === 0) s = 1;
        experiences.push({ score: s, junction_id: j.id, branch: b.name, quote: '「' + q.t + '」——' + q.by, count: b.count });
      });
    });
    experiences.sort(function (a, b) { return b.score - a.score; });
    experiences = experiences.slice(0, 3);
  }
  var skills = [];
  var ids = (j && j.relatedSkills) || [];
  ids.forEach(function (id) {
    var s = DB.skills[id];
    if (s) skills.push({ id: s.id, title: s.title, subtitle: s.subtitle, boundary: s.boundary || '' });
  });
  return { junctions: junctions, experiences: experiences, skills: skills };
}

function mergeWowMemoryLocal(prev, utterance, route, next) {
  prev = prev || {};
  var facts = (prev.facts || []).slice();
  var bit = (utterance || '').replace(/\s+/g, ' ').slice(0, 40);
  if (bit && facts.indexOf(bit) < 0) facts.push(bit);
  if (facts.length > 8) facts = facts.slice(-8);
  return {
    situation: prev.situation || ((route && route.heard) || bit),
    facts: facts,
    constraints: prev.constraints || [],
    junction_id: prev.junction_id || (route && route.junctionId) || '',
    task: next === 'match' ? bit : (prev.task || ''),
    open_question: prev.open_question || ''
  };
}

function packDiscussResult(utterance, route, history, next, live, extra) {
  extra = extra || {};
  var ctx = extra.context || retrieveWowLocal(utterance, route, history);
  var memory = extra.memory || mergeWowMemoryLocal(route && route.memory, utterance, route, next);
  return {
    reply: extra.reply || discussLocalReply(utterance, route, history, next),
    type: extra.type || (route && route.type) || 'explore',
    next: next,
    live: !!live,
    sessionId: extra.sessionId || 0,
    memory: memory,
    context: ctx,
    junctionId: (memory && memory.junction_id) || (route && route.junctionId) || extra.junctionId,
    topic: extra.topic || '',
    degraded: !!extra.degraded
  };
}

function localTaskTalk(route, utterance) {
  var heard = utterance || (route && route.heard) || '';
  return '听到你要动手了：「' + heard.replace(/\s+/g, ' ').slice(0, 36) + '」。不讨论该不该了，去匹配一张能帮你把这件事做完的卡——没有就不编。';
}

function localRouteIntent(text) {
  var t = text || '';
  var heard = t.replace(/\s+/g, '').slice(0, 20);
  var res;
  if (/撑不住|崩溃|活不下去|想不开|被伤害|想逃|自杀|重度焦虑/.test(t) ||
      ((/好累|难受|emo/.test(t)) && /撑|崩|逃|一直/.test(t))) {
    res = { type: 'emotion', stageId: null, junctionId: null,
      heard: heard,
      reply: '听到了。你说的是「' + heard + '」——这句话不需要被解决，也不会变成任何数据。\n如果只是累，歇一会儿再来。如果这种感觉持续了一段时间，校心理支持中心（工作日 8:30–17:30）比任何流程都合适。' };
  } else if (/该不该|要不要|值不值得|选哪|纠结|不知道该/.test(t) ||
      (/还是/.test(t) && /保研|考研|就业|出国|转|考公|工作|实习|本省|外省/.test(t))) {
    res = { type: 'decide', stageId: null, junctionId: guessJunction(t), heard: heard,
      reply: '你卡在「' + heard + '」。我不会替你选边——这类选择没有「做对了」的标准。\n我能给的是走过这个路口的人去了哪、付出了什么。' };
  } else if (/我决定|已经决定|已经想好|定了|接下来怎么准备|怎么排/.test(t) && !/该不该|要不要/.test(t)) {
    res = { type: 'action', stageId: null, junctionId: null, orchIntent: guessOrch(t), heard: heard,
      reply: '听到你已经定了方向（「' + heard + '」）。那就不试了，按别人真走完的路排接下来几周——不承诺结果。' };
  } else {
    res = { type: 'explore', stageId: guessStage(t),
      junctionId: (looksFriendship(t) || looksRomance(t)) ? guessJunction(t) : null, heard: heard,
      reply: '听到的是：「' + heard + '」。这还不是「该不该」的题，也先不用选边。我们从一件能试的小事开始。' };
  }
  return res;
}

function mapInterpret(text, r) {
  var exit = r.route_exit;
  if (exit !== 'explore' && exit !== 'decide' && exit !== 'action' && exit !== 'emotion') {
    if (r.mode === 'rejected' && r.task_intent === 'emotional_support') exit = 'emotion';
    else if (r.mode === 'rejected') exit = 'decide';
    else if (r.mode === 'orchestration') exit = 'action';
    else exit = 'explore';
  }
  var crisis = /撑不住|崩溃|活不下去|想不开|被伤害|想逃|自杀|重度焦虑/.test(text);
  var asking = /该不该|要不要|值不值得|选哪|纠结|不知道该/.test(text) ||
    (/恋爱|好感|表白|在一起/.test(text) && !/撑不住|崩溃/.test(text) && /该不该|要不要|到底/.test(text));
  if (exit === 'emotion' && !crisis && asking) {
    exit = 'decide';
    if (!r.junction_id) r.junction_id = guessJunction(text);
    if (!r.reply || /心理支持|挺不好过|帮不上|该不该谈/.test(r.reply || r.response || '')) {
      r.reply = looksFriendship(text)
        ? '你卡在「怎么开始有朋友」这一类处境上。这不是谈恋爱，我也不会把两件事混在一起。'
        : '你卡在「该不该谈」这一类选择上。有好感不是危机，我也不会替你选边——没有做对了的标准。我能给的是走过这个路口的人后来怎么选的。';
    }
  }
  var reply = r.reply || r.response || r.message || r.clarify_question || '';
  if (!reply && r.task_card) {
    reply = (r.heard ? '听到的是：「' + r.heard + '」。\n' : '') +
      (r.task_card.next_step ? '可以先做：' + r.task_card.next_step : '先从一件能试的小事开始。');
  }
  var out = {
    type: exit,
    stageId: r.stage_id || guessStage(text),
    junctionId: r.junction_id || ((exit === 'decide' || looksFriendship(text) || looksRomance(text)) ? guessJunction(text) : null),
    orchIntent: r.orchestration_intent || (exit === 'action' ? guessOrch(text) : ''),
    heard: r.heard || '',
    reply: reply,
    live: true,
    raw: r
  };
  if (exit === 'emotion' && r.resources && r.resources.length && reply.indexOf('心理') < 0) {
    var extra = r.resources.map(function (x) {
      if (typeof x === 'string') return x;
      return [x.label, x.hint].filter(Boolean).join(' · ');
    }).join('\n');
    if (extra) out.reply = (out.reply ? out.reply + '\n' : '') + extra;
  }
  return out;
}

function guessOrch(t) {
  if (/保研/.test(t)) return 'postgrad_recommend';
  if (/考研/.test(t)) return 'postgrad_exam';
  if (/出国|留学/.test(t)) return 'study_abroad';
  if (/就业|秋招|春招|求职|实习/.test(t)) return 'job_season';
  if (/进组|科研/.test(t)) return 'research_entry';
  if (/竞赛/.test(t)) return 'competition_season';
  return 'postgrad_recommend';
}

function skillPackText(skill) {
  if (!skill) return '';
  var script = (skill.script || []).map(function (row) {
    return (row[0] || '') + ' ' + (row[1] || '');
  }).join('\n');
  return [
    '【Skill】' + (skill.title || ''),
    '【判断点】' + (skill.judge || ''),
    '【边界 / 不适合谁】' + (skill.boundary || ''),
    '【两周剧本】\n' + script,
    '【学长原话】' + (skill.story || '')
  ].join('\n');
}

function guessStage(t) {
  if (/高考|报志愿|选专业|择校/.test(t)) return 'y0';
  if (/大一|孤独|朋友|社团|时间/.test(t)) return 'y1';
  if (/大二|竞赛|专业|转/.test(t)) return 'y2';
  if (/大三|考研|考公|实习|保研|秋招/.test(t)) return 'y3';
  if (/大四|毕业|offer|毕设/.test(t)) return 'y4';
  if (/研一|科研|导师|论文|文献/.test(t)) return 'g1';
  return 'y1';
}

function localExtractSlots(transcript, note) {
  return mockDelay({
    note: note || '',
    slots: [
      { key: 'trigger', title: '触发处境（适合谁）',
        content: '大一下 · 开始因"别人都进实验室了"而慌 · 想看科研但不想承诺进组',
        source: '"我大一下的时候特别迷茫，看别人都进实验室了就慌"' },
      { key: 'script', title: '两周剧本',
        content: 'D1–3 选老师并读 TA 最近一篇论文的摘要引言\nD4 发邮件（第二段必须有一句对论文的理解）\nD5–7 三天没回是常态，第 4 天发简短跟进\nD8–14 旁听一次组会，记下他们在争什么',
        source: '"在乎的是你有没有读过他最近的论文……第四天我又发了一封很短的跟进，当天就回了"' },
      { key: 'judge', title: '判断点（当年在哪一步差点放弃）',
        content: '发完三天没回信最容易放弃——三天没回是常态，第四天的跟进才是关键动作',
        source: '"发完三天没回信，我差点就放弃了"' },
      { key: 'boundary', title: '不适合谁（发布硬门槛）',
        content: '大四上不适合（实验室一般不收短期）；只想混推荐信的不适合',
        source: '"如果是大四上就别试这个了……如果只是想混封推荐信，也别来"' },
      { key: 'story', title: '当时的感受（故事层整体保留）',
        content: '"改了十一遍邮件" · "前两次完全听不懂" · "我从此知道科研长什么样了，一点都不后悔"',
        source: '口述全文将脱敏保留为「TA 的完整故事」' }
    ],
    missing: []
  }, 700);
}

/* 后端 skill（含 /growth/wow/discuss 返回的意图识别卡片）→ 前端 DB.skills['be-'+id] 的统一映射。
 * listSkills / getSkill / discuss 共用，避免三处各自写一遍。 */
function mapBackendSkill(s) {
  var id = 'be-' + s.id;
  var mapped = {
    id: id, backendId: s.id,
    title: s.name, subtitle: s.description || '',
    type: s.category || 'Skill', stageId: null,
    price: 0, days: 0, duration: '—',
    creator: {
      name: s.owner_name || s.username || '平台用户',
      tag: 'v' + (s.version || '1.0'),
      meta: 'v' + (s.version || '1.0'),
      color: '#5b5bd6',
      initial: String(s.owner_name || s.username || '平').slice(0, 1)
    },
    trigger: '', script: [], judge: '', boundary: s.description || '',
    fromBackend: true
  };
  DB.skills[id] = Object.assign(DB.skills[id] || {}, mapped);
  return DB.skills[id];
}

/* 本地兜底：未登录 / 后端不可用时，把 utterance 分词（补常见近义意图词）与本地 mock skill
 * （不含 fromBackend 的后端卡）的 title+subtitle+type 做包含匹配，返回最多 3 张卡。
 * 卡片字段与 retrieveWowLocal 的 skills 一致：{id,title,subtitle,boundary}。 */
function wowLocalMatchSkills(utterance) {
  var t = (utterance || '').trim();
  if (!t) return [];
  var toks = t.replace(/[，。？！、,.!?：:；;]/g, ' ').split(/\s+/).filter(function (x) { return x.length >= 2; });
  var ext = [];
  toks.forEach(function (w) {
    if (w.indexOf('论文') >= 0 || w.indexOf('写作') >= 0) ext.push('论文', '学术', 'LaTeX');
    if (w.indexOf('画') >= 0 || w.indexOf('图') >= 0) ext.push('图', '架构图', 'Draw.io', '图表');
    if (w.indexOf('保研') >= 0 || w.indexOf('推免') >= 0) ext.push('保研', '推免', '夏令营', '材料');
    if (w.indexOf('简历') >= 0 || w.indexOf('求职') >= 0 || w.indexOf('面试') >= 0) ext.push('简历', '面试');
  });
  var tokens = toks.concat(ext);
  var scored = Object.keys(DB.skills).map(function (k) { return DB.skills[k]; })
    .filter(function (s) { return s && s.title && !s.fromBackend; })
    .map(function (s) {
      var blob = [s.title, s.subtitle, s.type].join(' ');
      var n = 0;
      tokens.forEach(function (tok) { if (tok && blob.indexOf(tok) >= 0) n++; });
      return { s: s, n: n };
    })
    .filter(function (x) { return x.n > 0; })
    .sort(function (a, b) { return b.n - a.n; })
    .slice(0, 3)
    .map(function (x) {
      return { id: x.s.id, title: x.s.title, subtitle: x.s.subtitle || '', boundary: x.s.boundary || '' };
    });
  return scored;
}

var WowAPI = {

  /* ----------------------------------------------------------
   * 0. 认证（真实模式用；页面可选调用，不调则走演示账号）
   * ---------------------------------------------------------- */
  auth: {
    isLoggedIn: function () { return !!WowConfig.TOKEN && !!WowConfig.USER; },
    ping: function () {
      return fetch(WowConfig.API_BASE + '/health').then(function (r) {
        WowConfig.LIVE = r.ok;
        return r.ok;
      }).catch(function () { WowConfig.LIVE = false; return false; });
    },
    restore: function () {
      if (!WowConfig.TOKEN) return Promise.resolve(null);
      return wowGet('/auth/me').then(function (r) {
        var u = r.data || r.user || r;
        setAuth(WowConfig.TOKEN, u);
        return u;
      }).catch(function () {
        setAuth('', null);
        return null;
      });
    },
    register: function (profile) {
      return wowPost('/auth/register', profile).then(function (res) {
        setAuth(res.token, res.user);
        return res.user;
      });
    },
    login: function (account, password) {
      return wowPost('/auth/login', { account: account, password: password }).then(function (res) {
        setAuth(res.token, res.user);
        return res.user;
      });
    },
    logout: function () { setAuth('', null); },
    updateProfile: function (form) {
      var id = WowConfig.USER && WowConfig.USER.id;
      if (!id) return Promise.reject(new Error('未登录'));
      return wowPut('/users/' + id, form).then(function (r) {
        var u = r.data || r;
        setAuth(WowConfig.TOKEN, u);
        return u;
      });
    },
    mySkills: function () { return wowGet('/users/me/skills').then(function (r) { return r.data || []; }); },
    myGrowth: function () { return wowGet('/growth/my-profile'); }
  },

  /* ----------------------------------------------------------
   * 1. 意图路由：用户一句话 -> 四态之一
   *    返回 { type: 'explore'|'decide'|'action'|'emotion',
   *           stageId, junctionId, reply, raw }
   *    [REAL] POST /growth/goals/interpret { utterance }
   *      后端 mode → 前端 type 映射：
   *        rejected + emotional_support → emotion（全拦，带心理资源）
   *        rejected + life_decision     → decide（带 branches 分支人数）
   *        orchestration                → action（进编排态，next=probe）
   *        task                         → explore（带 task_card 四筛结果）
   *        clarify / manual_fallback / not_skillable → explore（带提示语）
   * ---------------------------------------------------------- */
  routeIntent: function (text, userStage) {
    var useLive = !WowConfig.USE_MOCK && WowConfig.TOKEN;
    if (useLive) {
      return wowPost('/growth/goals/interpret', { utterance: text }).then(function (r) {
        return mapInterpret(text, r);
      });
    }
    var res = localRouteIntent(text);
    res.live = false;
    res.needLogin = !WowConfig.TOKEN && !WowConfig.USE_MOCK;
    return mockDelay(res, 500);
  },

  /* ----------------------------------------------------------
   * 2. 运行时对话：装载 Skill 后与陪跑 Agent 聊天
   *    ctx = { skillId, day, runId, execId?, taskIntent? }
   *    返回 { reply, boundaryHit, execId }
   *    [REAL] 后端对应「任务工作台」：
   *      首次：POST /growth/executions { task_intent, task_title, goal, material }
   *      推进：POST /growth/executions/:id/advance
   *      出参 mode 映射：
   *        action   → 普通推进，reply = title + content（后端已停用判断点拦截，连续推进）
   *        handoff  → 交回人处理（≈ 边界命中），boundaryHit=true
   *        degraded → LLM 不可用降级，提示手动记录
   *      每一步都会落 execution_steps，供给侧飞轮从这里开始。
   * ---------------------------------------------------------- */
  /* 确保 execution 存在：已有 execId 直接用，没有则按装载的 skill 创建一条。
   * 返回值 Promise<execId>。收尾链路（genClosingQuestions / submitClosing）也复用它。 */
  ensureExecId: function (opt) {
    var skill = opt.skill || null;
    if (opt.execId) return Promise.resolve(opt.execId);
    return ensureLogin().then(function () {
      return wowPost('/growth/executions', {
        task_intent: opt.taskIntent || 'project_convergence',
        task_title: opt.taskTitle || (skill ? skill.title : '陪跑任务'),
        goal: opt.text || '',
        material: skillPackText(skill),
        skill_id: skill && skill.backendId ? skill.backendId : 0
      }).then(function (r) {
        return r.data && r.data.ID ? r.data.ID : (r.data && r.data.id);
      });
    });
  },
  runChat: function (text, ctx) {
    if (!WowConfig.USE_MOCK) {
      var skill = (typeof DB !== 'undefined' && DB.skills[ctx.skillId]) || null;
      return WowAPI.ensureExecId({
        execId: ctx.execId, skill: skill, text: text,
        taskIntent: ctx.taskIntent, taskTitle: skill ? skill.title : '陪跑任务'
      }).then(function (execId) {
        ctx.execId = execId;
        return wowPost('/growth/executions/' + execId + '/advance', { user_input: text });
      }).then(function (r) {
        if (r.mode === 'handoff') {
          return { boundaryHit: true, execId: ctx.execId,
            reply: '这一步需要你亲自来：' + r.title + '\n' + (r.content || '') };
        }
        if (r.mode === 'degraded') {
          return { boundaryHit: false, execId: ctx.execId,
            reply: r.message || 'AI 暂时不可用，你可以手动记录这一步做了什么。' };
        }
        /* mode === 'action'（后端已停用 decision 拦截，一律连续推进） */
        return { boundaryHit: false, execId: ctx.execId,
          reply: (r.title ? r.title + '\n' : '') + (r.signal || r.content || '') +
            (r.done ? '\n✅ ' + (r.done_reason || '这个任务可以收尾了') : '') };
      });
    }
    var skill = DB.skills[ctx.skillId];
    var res;
    if (/只剩.*小时|没时间|太忙|忙不过来/.test(text)) {
      res = { boundaryHit: true,
        reply: '这触碰了本卡的边界（' + (skill ? skill.boundary.split('；')[0] : '时间下限') + '）。这张卡的前提不成立了。建议：① 暂停，下周恢复 ② 换一张低时间密度的卡 ③ 只保留最后的"决定"环节。帖子不会喊停，Skill 会——它知道自己什么时候失效。' };
    } else if (/怎么写|帮我写|话术|模板/.test(text)) {
      res = { boundaryHit: false,
        reply: '按 ' + (skill ? skill.creator.name : '创建者') + ' 卡里的原话术风格，给你生成了一版草稿（真实接入后由 LLM 基于 skill 包的 prompts 生成）：\n"学长你好，我是上周来旁听的大一新生，想申请下次队训帮忙记分，不会打扰训练。可以吗？"\n—— 产出物里流着学长经验的血，这是装载 Skill 和裸问 AI 的区别。' };
    } else if (/想放弃|想走|坚持不下去/.test(text)) {
      res = { boundaryHit: false,
        reply: (skill ? '「' + skill.title + '」的判断点正好说到这个时刻：\n' + skill.judge : '这个时刻卡里有预案，翻一下判断点。') + '\n\n你现在的答案是什么？两个答案都算完成。' };
    } else {
      res = { boundaryHit: false,
        reply: '收到。我是带着' + (skill ? '「' + skill.title + '」（' + skill.creator.name + '）' : '当前装载卡') + '经验包的陪跑 Agent。你可以问我：某一步怎么做、帮你写话术、或者直接说"想放弃"——判断点会接住你。\n（真实接入后此处透传 LLM，system prompt 注入 skill 包全文）' };
    }
    return mockDelay(res, 800);
  },

  /* ----------------------------------------------------------
   * 收尾闭环：两问 + 一句 verdict
   *    [REAL] 问题按该 Skill 的 .md 文档由 LLM 生成：
   *      POST /growth/executions/:id/closing/questions  → {q1,q2}
   *      提交（含用户手动输入的改进建议）：
   *      POST /growth/executions/:id/closing/submit {q1,q2,verdict,improvement}
   *    execId 为空（未走真实对话的演示 run）→ 返回 null / no-op，前端用通用问题兜底
   * ---------------------------------------------------------- */
  genClosingQuestions: function (execId, skill) {
    if (!WowConfig.USE_MOCK) {
      return WowAPI.ensureExecId({ execId: execId, skill: skill || null, text: '' })
        .then(function (id) {
          // 透传该 skill 的文档内容：真实 skill 后端读 zip 包，mock skill 用这份摘要走 LLM
          return wowPost('/growth/executions/' + id + '/closing/questions', { skill_content: skillPackText(skill) })
            .then(function (r) {
              if (r && r.q1 && r.q2) { r.execId = id; return r; }
              return null;
            });
        })
        .catch(function () { return null; });
    }
    return mockDelay(null, 400);
  },
  submitClosing: function (execId, data) {
    if (!WowConfig.USE_MOCK) {
      if (!execId) return Promise.resolve({ ok: true });
      return ensureLogin().then(function () {
        return wowPost('/growth/executions/' + execId + '/closing/submit', data);
      });
    }
    return mockDelay({ ok: true }, 400);
  },

  /* ----------------------------------------------------------
   * 3. 学长口述 -> 四槽抽取
   *    返回 { slots: [{key,title,content,source}], missing: [],
   *           versionId?, skillId? }
   *    [REAL] POST /growth/backfill（轨迹补录通道：
   *      承认经验发生在平台外，蒸馏度封顶 0.85，Trust Card 标注补录来源）
   *      入参 { task_intent, task_title, before, after, decisions[] }
   *      出参为草稿全貌：slots 四槽分组（含 prompt / filled），
   *      versionId 用于后续 PATCH /growth/drafts/:versionID 编辑、
   *      generate-folder 生成六 slot skill 包、publish 走发布门禁。
   * ---------------------------------------------------------- */
  extractSlots: function (transcript) {
    if (!WowConfig.USE_MOCK) {
      return ensureLogin().then(function () {
        return wowPost('/growth/backfill', {
          task_intent: 'project_convergence',
          task_title: '学长口述经验补录',
          before: '',
          after: transcript,
          decisions: []
        });
      }).then(function (r) {
        var slots = (r.slots || []).map(function (s) {
          var d = (s.decisions && s.decisions[0]) || null;
          return {
            key: s.slot,
            title: s.prompt || s.slot,
            content: d ? (d.trigger_signal + ' → ' + d.judgment + '（' + d.scope + '）') : '',
            source: d && d.source_step_index != null ? '轨迹第 ' + d.source_step_index + ' 步' : '补录'
          };
        }).filter(function (s) { return s.content; });
        if (slots.length) {
          return { slots: slots, missing: r.missing || [], versionId: r.version_id, skillId: r.skill_id, live: true };
        }
        /* 补录通道要求关键判断，口述尚未结构化时：记下笔记，界面仍用可溯源抽取展示 */
        return localExtractSlots(transcript, r.message || r.still_missing);
      }).catch(function () { return localExtractSlots(transcript); });
    }
    return localExtractSlots(transcript);
  },

  /* ----------------------------------------------------------
   * 4. 变体检测：run 结束时对比实际轨迹与原剧本
   *    返回 { hasVariant, diff: {old,new}, draftTitle, note }
   *    [REAL] GET /growth/skills/:id/version-candidates
   *      后端 F12 反馈闭环：调用量 ≥20 用 3 次/3 人触发，
   *      <20 用 2 次/2 人（冷启动门槛），候选带溯源。
   *      接受候选：POST /growth/version-candidates/:id/accept
   * ---------------------------------------------------------- */
  detectVariant: function (runId, skillBackendId) {
    if (!WowConfig.USE_MOCK) {
      if (!skillBackendId) return Promise.resolve({ hasVariant: false });
      return wowGet('/growth/skills/' + skillBackendId + '/version-candidates').then(function (r) {
        var list = r.data || r.candidates || [];
        if (!list.length) return { hasVariant: false };
        var c0 = list[0];
        return {
          hasVariant: true,
          candidateId: c0.id,
          diff: { old: c0.old_text || '（原版本）', new: c0.new_text || c0.suggestion || '' },
          draftTitle: c0.title || '版本候选',
          note: '由 ' + (c0.feedback_count || '多') + ' 条真实反馈触发 · 接受后自动升版本并保留谱系'
        };
      });
    }
    if (runId !== 'r1') return mockDelay({ hasVariant: false }, 500);
    return mockDelay({
      hasVariant: true,
      diff: {
        old: 'D6–10 把当场最简单的题带回去自己试',
        new: 'D6–10 报名当一次队训记分员——不用会打也有正当位置，还能看清全场'
      },
      draftTitle: '先当记分员再上手',
      note: '变体卡草稿已生成 · 署名：小林（计算机 2029 级）· 原卡谱系：老周 v1.0 → 你只需要确认，不需要写一个字'
    }, 1000);
  },

  /* ----------------------------------------------------------
   * 5. Skill 检索/匹配
   *    [REAL] GET /skills?keyword=&category=（游客可用，无需登录）
   *      后端排序已按 PRD 改造：quality_score（任务证据）优先，
   *      不提供按评分/下载量排序。
   *      语义路由（两段式，带 choose_if 解释）：POST /growth/route
   *    真实模式下把后端技能合并进本地 DB（本地 mock 卡保留，
   *      后端技能 id 加 'be-' 前缀避免冲突）。
   * ---------------------------------------------------------- */
  listSkills: function (filter) {
    if (!WowConfig.USE_MOCK) {
      var q = [];
      if (filter && filter.q) q.push('keyword=' + encodeURIComponent(filter.q));
      if (filter && filter.type) q.push('category=' + encodeURIComponent(filter.type));
      return wowGet('/skills' + (q.length ? '?' + q.join('&') : '')).then(function (r) {
        var remote = (r.data || []).map(mapBackendSkill);
        var local = Object.keys(DB.skills).map(function (k) { return DB.skills[k]; })
          .filter(function (s) { return !s.fromBackend; });
        var all = local.concat(remote);
        if (!filter) return all;
        return all.filter(function (s) {
          if (filter.stage && s.stageId !== filter.stage) return false;
          if (filter.freeOnly && s.price > 0) return false;
          return true;
        });
      });
    }
    var all = Object.keys(DB.skills).map(function (k) { return DB.skills[k]; });
    if (!filter) return Promise.resolve(all);
    return Promise.resolve(all.filter(function (s) {
      if (filter.stage && s.stageId !== filter.stage) return false;
      if (filter.type && s.type !== filter.type) return false;
      if (filter.freeOnly && s.price > 0) return false;
      if (filter.q && (s.title + s.subtitle + s.creator.name).indexOf(filter.q) < 0) return false;
      return true;
    }));
  },
  getSkill: function (id) {
    if (!WowConfig.USE_MOCK && /^be-/.test(id) && !(DB.skills[id] && DB.skills[id].title)) {
      return wowGet('/skills/' + id.slice(3)).then(function (r) {
        var s = r.data || r;
        var mapped = mapBackendSkill(s);
        mapped.files = s.files || [];
        return mapped;
      });
    }
    return Promise.resolve(DB.skills[id] || null);
  },

  /* ----------------------------------------------------------
   * 5.5 Skill 使用指南（零基础版）
   *    [REAL] GET /skills/:id/explain（需登录；后端读取 zip 包内 SKILL.md 等
   *      真实内容，按用户 AI 水平生成：在哪用 / 装什么依赖 / 用什么 prompt / 怎么下载）
   *    返回 { data: markdown, aiLevel, levelLabel }
   * ---------------------------------------------------------- */
  explainSkill: function (skillId) {
    var s = (typeof DB !== 'undefined' && DB.skills[skillId]) || null;
    var live = !WowConfig.USE_MOCK;
    var id = String(skillId);
    if (live) {
      if (/^be-/.test(id)) id = id.slice(3);
      else if (s && s.backendId) id = String(s.backendId);
      else live = false; // 本地演示 skill（无 zip 包）：降级返回演示版，避免请求不存在的后端 id
    }
    if (live) {
      return ensureLogin().then(function () {
        return wowGet('/skills/' + id + '/explain');
      }).then(function (r) {
        return { data: r.data || '', aiLevel: r.ai_level, levelLabel: r.level_label, live: true };
      });
    }
    var demoGuide = '## 怎么使用这个 Skill（演示版）\n\n' +
      '- 在什么软件上运行：配合 AI 编码助手（如 Trae / Cursor）使用，这是开源 Skill 的常见用法。\n' +
      '- 需要什么依赖：Node.js（运行 npx 命令）\n' +
      '- 用什么 prompt 驱动：复制「请按这个 Skill 的流程指导我」到对话框。\n' +
      '- 从哪里下载：' + (s && s.real ? (s.real.repo || '仓库见下方') : '仓库见下方') + ' → ' + (s && s.real ? (s.real.url || '') : '') + '\n' +
      '- 安装命令：' + (s && s.real ? (s.real.install || '') : '') + '\n\n' +
      '> 登录后由 AI 基于你的水平生成完整版。';
    return mockDelay({
      data: s && s.real ? demoGuide : '',
      aiLevel: 'beginner', levelLabel: 'AI 初级', live: false
    }, 500);
  },

  /* ----------------------------------------------------------
   * 6. Trust Card（真实模式新增能力，mock 页面暂用本地数据）
   *    [REAL] GET /growth/skills/:id/trust-card（游客可看）
   *      七分区、判断级溯源、无任何综合评分。
   * ---------------------------------------------------------- */
  getTrustCard: function (skillBackendId) {
    if (!WowConfig.USE_MOCK) return wowGet('/growth/skills/' + skillBackendId + '/trust-card');
    return Promise.resolve(null);
  },

  /* ----------------------------------------------------------
   * 7. 编排态（真实模式新增能力）
   *    [REAL] POST /growth/orch-probe { orchestration_intent }
   *           没人走过的方向会拒绝生成（这一幕值得放进路演）
   *           POST /growth/orch-interview → POST /growth/orchestrations
   * ---------------------------------------------------------- */
  probeOrchestration: function (intent, utterance) {
    if (!WowConfig.USE_MOCK) {
      return ensureLogin().then(function () {
        return wowPost('/growth/orch-probe', {
          orchestration_intent: intent || 'postgrad_recommend',
          utterance: utterance || ''
        });
      });
    }
    return Promise.resolve({
      available: intent === 'postgrad_recommend',
      orchestration_intent: intent,
      label: intent === 'postgrad_recommend' ? '保研准备' : '考研准备',
      walked_total: intent === 'postgrad_recommend' ? 10 : 0,
      message: intent === 'postgrad_recommend'
        ? '有人走过。下面是按真实 Path 排的节点，不承诺结果。'
        : '这条路目前还没有人在这里走完过。我不会凭空给你排一份。',
      source_paths: intent === 'postgrad_recommend' ? [{
        goal_label: '保研准备（回忆整理）',
        walked_count: 10,
        provenance: 'retrospective',
        nodes: [
          { label: '确认排名与名额口径', week_offset: 0, controllable: true },
          { label: '联系导师 / 准备材料', week_offset: 2, controllable: true },
          { label: '夏令营投递', week_offset: 4, controllable: true },
          { label: '等待名额与面试', week_offset: 6, controllable: false }
        ]
      }] : []
    });
  },

  /* ----------------------------------------------------------
   * 8. 许愿池（复用论坛：无 Skill 时的供给缺口）
   *    GET  /forum/topics?category=looking_for
   *    POST /forum/topics          挂愿望
   *    POST /forum/topics/:id/like 我也在等
   *    GET  /forum/topics/:id      详情 + 学长回应
   *    POST /forum/topics/:id/replies
   * ---------------------------------------------------------- */
  listWishes: function (keyword) {
    var q = ['category=looking_for'];
    if (keyword) q.push('keyword=' + encodeURIComponent(keyword));
    return wowGet('/forum/topics?' + q.join('&')).then(function (r) {
      return r.data || [];
    }).catch(function () { return []; });
  },
  hangWish: function (title, content) {
    return ensureLogin().then(function () {
      return wowGet('/forum/topics?category=looking_for&keyword=' + encodeURIComponent(title));
    }).then(function (r) {
      var list = r.data || [];
      var hit = list.filter(function (t) { return t.title === title; })[0];
      if (hit) {
        if (hit.liked) {
          return { id: hit.id, waiting: hit.like_count || 1, joined: true };
        }
        return wowPost('/forum/topics/' + hit.id + '/like').then(function (d) {
          var body = d.data || d;
          return { id: hit.id, waiting: body.like_count || (hit.like_count || 0) + 1, joined: true };
        });
      }
      return wowPost('/forum/topics', {
        title: title,
        content: content || '',
        category: 'looking_for'
      }).then(function (res) {
        var id = (res.data && res.data.id) || res.id;
        return wowPost('/forum/topics/' + id + '/like').then(function (d) {
          var body = d.data || d;
          return { id: id, waiting: body.like_count || 1, created: true };
        }).catch(function () {
          return { id: id, waiting: 1, created: true };
        });
      });
    });
  },
  getWish: function (id) {
    return wowGet('/forum/topics/' + id).then(function (r) {
      return { wish: r.data || r, replies: r.replies || [] };
    });
  },
  replyWish: function (id, content) {
    return ensureLogin().then(function () {
      return wowPost('/forum/topics/' + id + '/replies', { content: content });
    });
  },
  waitWish: function (id) {
    return ensureLogin().then(function () {
      return wowPost('/forum/topics/' + id + '/like').then(function (d) { return d.data || d; });
    });
  },

  /* 多轮对话：服务端会话记忆 + 检索经验/入口/Skill */
  discuss: function (utterance, route, history, forks, sessionId) {
    var local = function () {
      var next = looksReadyForTask(utterance) ? 'match' : 'talk';
      // 本地兜底：本地 mock 卡（不含后端 be- 卡）做关键词匹配，未登录也有可点的卡片
      var localCtx = retrieveWowLocal(utterance, route, history || []);
      var matched = wowLocalMatchSkills(utterance);
      if (matched.length) localCtx.skills = matched;
      return packDiscussResult(utterance, route, history, next, false, { context: localCtx });
    };
    if (!WowConfig.TOKEN) return Promise.resolve(local());
    return wowPost('/growth/wow/discuss', {
      session_id: sessionId || 0,
      utterance: utterance,
      history: history || [],
      forks: forks || null
    }).then(function (r) {
      var next = r.next || 'talk';
      if (r.degraded && looksReadyForTask(utterance)) next = 'match';
      var reply = r.reply || discussLocalReply(utterance, route, history, next);
      if (next !== 'match' && looksLikeForkDump(reply, forks)) {
        reply = discussLocalReply(utterance, route, history, next);
      }
      // 后端意图识别返回的 skill 卡片：逐个映射成前端对象并缓存进 DB.skills（id 用 'be-'+id）
      var ctx = { skills: [] };
      if (r.context && Array.isArray(r.context.skills)) {
        ctx.skills = r.context.skills.map(mapBackendSkill);
      }
      return packDiscussResult(utterance, route, history, next, !r.degraded, {
        reply: reply,
        type: r.route_exit || (route && route.type) || 'explore',
        sessionId: r.session_id,
        memory: r.memory,
        context: ctx,
        junctionId: r.memory && r.memory.junction_id,
        topic: r.topic,
        degraded: r.degraded
      });
    }).catch(function () { return local(); });
  }
};
