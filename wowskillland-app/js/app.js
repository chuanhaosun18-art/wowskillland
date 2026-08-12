/* ============================================================
 * WowSkillLand · 前端主程序
 * hash 路由 + 视图渲染 + 交互
 * 页面数据一律经 DB(data.js) 与 WowAPI(api.js) 获取
 * ============================================================ */

/* ---------------- 会话内状态 ---------------- */
var State = {
  coins: DB.user.coins,
  runs: [],
  purchased: {},
  published: [],
  wished: {},
  poolCount: Object.keys(DB.skills).length
};
var activeRunId = '';

function persistRuns() {
  try {
    localStorage.setItem('wow_runs', JSON.stringify({
      runs: State.runs, activeRunId: activeRunId, coins: State.coins, purchased: State.purchased
    }));
  } catch (e) { /* ignore */ }
}
function restoreRuns() {
  try {
    var raw = JSON.parse(localStorage.getItem('wow_runs') || 'null');
    if (!raw || !raw.runs) return;
    State.runs = raw.runs.filter(function (r) { return r && r.skillId && DB.skills[r.skillId]; });
    State.purchased = raw.purchased || {};
    if (typeof raw.coins === 'number') State.coins = raw.coins;
    activeRunId = raw.activeRunId || '';
  } catch (e) { /* ignore */ }
}
restoreRuns();
function persistWished() {
  try { localStorage.setItem('wow_wished', JSON.stringify(State.wished)); } catch (e) { /* ignore */ }
}
function restoreWished() {
  try {
    var raw = JSON.parse(localStorage.getItem('wow_wished') || 'null');
    if (raw && typeof raw === 'object') State.wished = raw;
  } catch (e) { /* ignore */ }
}
restoreWished();

/* ---------------- 工具 ---------------- */
function $(s, root) { return (root || document).querySelector(s); }
function $all(s, root) { return Array.prototype.slice.call((root || document).querySelectorAll(s)); }
function stageOf(id) {
  if (!id || !DB.stages) return null;
  for (var i = 0; i < DB.stages.length; i++) {
    if (DB.stages[i].id === id) return DB.stages[i];
  }
  return null;
}
function esc(str) {
  return String(str == null ? '' : str).replace(/[&<>"']/g, function (c) {
    return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
  });
}
function nl2br(s) { return esc(s).replace(/\n/g, '<br>'); }

/* 极简 markdown 渲染：## 小标题 / - 列表 / 数字列表 / **加粗** / `行内代码` / ```代码块``` */
function inlineMd(s) {
  return esc(s)
    .replace(/`([^`]+)`/g, '<code class="md-inline">$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
}
function mdRender(md) {
  if (!md) return '';
  var lines = String(md).replace(/\r\n/g, '\n').split('\n');
  var html = '', inCode = false, codeBuf = [];
  function flushCode() {
    if (codeBuf.length) { html += '<pre class="md-code">' + esc(codeBuf.join('\n')) + '</pre>'; codeBuf = []; }
  }
  lines.forEach(function (line) {
    var t = line.trim();
    if (inCode) {
      if (/^```/.test(t)) { flushCode(); inCode = false; }
      else codeBuf.push(line);
      return;
    }
    if (/^```/.test(t)) { inCode = true; return; }
    if (/^#{1,4}\s/.test(line)) html += '<div class="md-h">' + inlineMd(line.replace(/^#+\s*/, '')) + '</div>';
    else if (/^\s*[-*+]\s+/.test(line)) html += '<div class="md-li">· ' + inlineMd(line.replace(/^\s*[-*+]\s+/, '')) + '</div>';
    else if (/^\s*\d+[.)]\s+/.test(line)) html += '<div class="md-li">' + inlineMd(t) + '</div>';
    else if (t === '') html += '<div class="md-gap"></div>';
    else html += '<p class="md-p">' + inlineMd(line) + '</p>';
  });
  flushCode();
  return html;
}
function toast(txt, gold) {
  var w = $('#toast-wrap'); var t = document.createElement('div');
  t.className = 'toast' + (gold ? ' gold' : ''); t.textContent = txt; w.appendChild(t);
  setTimeout(function () { t.style.opacity = '0'; t.style.transition = 'opacity .4s'; setTimeout(function () { t.remove(); }, 400); }, 3200);
}
function confetti() {
  var e = ['🎉', '✨', '🃏', '⭐', '🎊'];
  for (var i = 0; i < 26; i++) {
    var s = document.createElement('div'); s.className = 'confetti';
    s.textContent = e[Math.floor(Math.random() * e.length)];
    s.style.left = (Math.random() * 100) + 'vw';
    s.style.animationDuration = (1.6 + Math.random() * 1.6) + 's';
    s.style.fontSize = (14 + Math.random() * 14) + 'px';
    document.body.appendChild(s);
    (function (node) { setTimeout(function () { node.remove(); }, 3400); })(s);
  }
}
function syncTopbar() {
  $('#pool-count').textContent = State.poolCount;
  $('#coin-count').textContent = State.coins;
  var live = $('#live-chip');
  if (live) {
    if (WowConfig.LIVE) live.removeAttribute('hidden');
    else live.setAttribute('hidden', '');
  }
  var slot = $('#auth-slot');
  if (!slot) return;
  if (WowAPI.auth.isLoggedIn()) {
    var u = WowConfig.USER;
    var initial = (u.username || '?').slice(0, 1);
    slot.innerHTML = '<a class="auth-chip" href="#/me"><span class="auth-ava">' + esc(initial) + '</span>' +
      esc(u.username) + '</a><button class="auth-out" id="btn-logout">退出</button>';
    var out = $('#btn-logout');
    if (out) out.onclick = function () {
      WowAPI.auth.logout();
      toast('已退出');
      location.hash = '#/home';
      render();
    };
  } else {
    slot.innerHTML = '<a class="btn-ghost" href="#/login">登录 / 注册</a>';
  }
}
function openModal(html) {
  $('#modal-box').innerHTML = '<button class="close" data-close>✕</button>' + html;
  $('#modal-overlay').classList.add('active');
  $('[data-close]', $('#modal-box')).addEventListener('click', closeModal);
}
function closeModal() { $('#modal-overlay').classList.remove('active'); }
$('#modal-overlay').addEventListener('click', function (e) { if (e.target === this) closeModal(); });

function findTask(id) {
  var found = null;
  DB.stages.forEach(function (st) {
    st.scenes.forEach(function (sc) {
      sc.tasks.forEach(function (t) { if (t.id === id) found = t; });
    });
  });
  return found;
}
function gapTasks() {
  var out = [];
  DB.stages.forEach(function (st) {
    st.scenes.forEach(function (sc) {
      sc.tasks.forEach(function (t) {
        if (t.wish) out.push({ id: t.id, title: t.title, desc: t.desc, stage: st.short });
      });
    });
  });
  return out;
}
function skillCountOfStage(st) {
  var n = 0;
  st.scenes.forEach(function (sc) { sc.tasks.forEach(function (t) { n += (t.skillIds || []).length; }); });
  return n;
}
function isLoaded(skillId) {
  return State.runs.some(function (r) { return r.skillId === skillId && !r.finished; });
}

/* ---------------- 公共渲染片段 ---------------- */
function skillChipHtml(id) {
  var s = DB.skills[id]; if (!s) return '';
  var tag = s.verify
    ? '<span class="sc-v">✓' + s.verify.pass + '/' + s.verify.total + '</span>'
    : (s.real ? '<span class="sc-v" style="color:#9a6700">⭐ 开源</span>' : '');
  return '<span class="skill-chip" data-goskill="' + s.id + '">🃏 ' + esc(s.title) + ' ' + tag + '</span>';
}
function skillCardHtml(s) {
  if (!s) return '';
  var creator = s.creator || {};
  var price = s.price > 0
    ? '<span class="price-tag coin">🪙 ' + s.price + '</span>'
    : '<span class="price-tag free">免费</span>';
  var st = stageOf(s.stageId);
  var proofBadge = s.verify
    ? '<span class="badge green">✓ ' + s.verify.pass + '/' + s.verify.total + ' 验证</span>'
    : (s.real ? '<span class="badge ink">⭐ GitHub 开源</span>' : '');
  var initial = creator.initial || String(creator.name || '?').slice(0, 1);
  return '<div class="skill-card" data-goskill="' + s.id + '">' +
    '<div class="k-top"><span class="badge violet">' + esc(s.type || 'Skill') + '</span>' +
    '<span class="badge amber">' + (st ? st.short : '') + '</span>' +
    proofBadge + '</div>' +
    '<h3>' + esc(s.title) + '</h3>' +
    '<div class="k-sub">' + esc(s.subtitle) + '</div>' +
    '<div class="k-meta"><span class="k-creator"><span class="k-ava" style="background:' + (creator.color || '#5b5bd6') + '">' +
    esc(initial) + '</span>' + esc(creator.name || '匿名') + ' · ' + esc(creator.meta || creator.tag || '') + '</span>' + price + '</div></div>';
}
function bindSkillLinks(root) {
  $all('[data-goskill]', root).forEach(function (el) {
    el.addEventListener('click', function (e) {
      e.stopPropagation();
      location.hash = '#/skill/' + el.getAttribute('data-goskill');
    });
  });
}

/* ============================================================
 * 视图：首页
 * ============================================================ */
var lastQuery = '';
var lastRoute = null;
var homeChat = [];
var homeBusy = false;
var wowSessionId = 0;
var exploreState = { step: 0, answers: {}, query: '', stageId: 'y1' };
var EXPLORE_Q = [
  { key: 'hours', q: '你每周大概有多少能自己支配的时间？', opts: ['几乎没有', '5–10 小时', '10–15 小时', '比较多'] },
  { key: 'hard', q: '现在最硬的约束是哪一条？', opts: ['课业 / 绩点', '钱或家里期望', '社交从零开始', '方向完全没谱'] }
];

function junctionList() {
  return Object.keys(DB.junctions).map(function (k) { return DB.junctions[k]; });
}
function skillsOfStage(stageId) {
  return Object.keys(DB.skills).map(function (k) { return DB.skills[k]; })
    .filter(function (s) { return s.stageId === stageId; });
}
function queryTokens(q) {
  var t = q || '';
  var extra = [];
  if (/交朋友|没朋友|孤独|社恐|搭子/.test(t)) extra = extra.concat(['破冰', '搭子', '朋友', '社交', '开口', '食堂']);
  if (/恋爱|表白|分手|暗恋|对象|在一起/.test(t) || (/好感/.test(t) && !/交朋友|孤独/.test(t))) extra = extra.concat(['好感', '表白']);
  if (/转专业/.test(t)) extra = extra.concat(['转专业', '旁听', '专业']);
  if (/保研|推免|夏令营/.test(t)) extra = extra.concat(['保研', '夏令营', '材料']);
  if (/就业|求职|实习|秋招|简历/.test(t)) extra = extra.concat(['简历', '实习', '秋招']);
  if (/考研/.test(t)) extra = extra.concat(['考研']);
  if (/宿舍|室友/.test(t)) extra = extra.concat(['宿舍', '边界']);
  if (/论文|选题/.test(t)) extra = extra.concat(['论文', '选题']);
  var raw = t.replace(/[，。？！、,.!?]/g, ' ').split(/\s+/).filter(function (x) { return x.length >= 2; });
  return raw.concat(extra);
}
function matchTaskLabel() {
  var mem = lastRoute && lastRoute.memory;
  if (mem && mem.task) return mem.task;
  if (mem && mem.situation) return mem.situation;
  return lastQuery || (lastRoute && lastRoute.heard) || '';
}
function matchSearchBlob() {
  var mem = lastRoute && lastRoute.memory;
  return [
    lastQuery,
    lastRoute && lastRoute.heard,
    lastRoute && lastRoute.topic,
    mem && mem.task,
    mem && mem.situation,
    mem && mem.junction_id,
    ((mem && mem.facts) || []).join(' ')
  ].filter(Boolean).join(' ');
}
function matchSkills(query) {
  var tokens = queryTokens(matchSearchBlob() || query);
  var seen = {};
  var out = [];
  var jid = lastRoute && lastRoute.junctionId;
  if (jid && DB.junctions[jid] && DB.junctions[jid].relatedSkills) {
    DB.junctions[jid].relatedSkills.forEach(function (id) {
      if (DB.skills[id] && !seen[id]) { seen[id] = true; out.push(DB.skills[id]); }
    });
  }
  var scored = Object.keys(DB.skills).map(function (k) { return DB.skills[k]; })
    .filter(function (s) { return s && s.title && !seen[s.id]; })
    .map(function (s) {
      var blob = [s.title, s.subtitle, s.match, s.trigger, s.story, s.boundary, s.judge].join(' ');
      var score = 0;
      tokens.forEach(function (tok) { if (tok && blob.indexOf(tok) >= 0) score++; });
      return { s: s, score: score };
    })
    .filter(function (x) { return x.score > 0; })
    .sort(function (a, b) { return b.score - a.score; });
  scored.forEach(function (x) {
    if (out.length >= 6) return;
    seen[x.s.id] = true;
    out.push(x.s);
  });
  return out;
}
function rerouteHtml() {
  return '<div class="reroute">判错了？一键改道：' +
    '<button class="chip-btn" data-reroute="explore">我想先试小事</button>' +
    '<button class="chip-btn" data-reroute="decide">我在纠结该不该</button>' +
    '<button class="chip-btn" data-reroute="action">我已经决定了</button></div>';
}
function bindReroute(root) {
  $all('[data-reroute]', root).forEach(function (b) {
    b.addEventListener('click', function () {
      var t = b.getAttribute('data-reroute');
      lastRoute = lastRoute || {};
      lastRoute.type = t;
      if (t === 'explore') {
        exploreState = { step: 0, answers: {}, query: lastQuery, stageId: (lastRoute && lastRoute.stageId) || guessStage(lastQuery || '') };
        location.hash = '#/explore';
      } else if (t === 'decide') {
        location.hash = '#/paths';
      } else {
        location.hash = '#/orch/' + (lastRoute.orchIntent || guessOrch(lastQuery || ''));
      }
    });
  });
}
function goAfterRoute(res) {
  lastRoute = res;
  if (res.type === 'explore') {
    exploreState = { step: 0, answers: {}, query: lastQuery, stageId: res.stageId || 'y1' };
    location.hash = '#/explore';
  } else if (res.type === 'decide' && res.junctionId) {
    location.hash = '#/junction/' + res.junctionId;
  } else if (res.type === 'action') {
    location.hash = '#/orch/' + (res.orchIntent || guessOrch(lastQuery || ''));
  }
}

function viewHome() {
  return '<div class="view home-wrap">' +
    '<div class="home-chat">' +
    '<div class="home-intro"><h1>说说你现在的<span class="hl">不知道</span></h1>' +
    '<p>不测评 · 不建议 · 不承诺。先把「不知道」聊清楚；真要动手了，再匹配能帮你做完的 Skill。</p>' +
    '<div class="home-chips" id="home-chips">' +
    '<button class="chip-btn" data-q="我大一，一个朋友都没有，很孤独">大一很孤独</button>' +
    '<button class="chip-btn" data-q="我大二，不知道该不该转专业">该不该转专业</button>' +
    '<button class="chip-btn" data-q="我大三，不知道该保研还是就业">保研还是就业</button>' +
    '<button class="chip-btn" data-q="我决定保研了，接下来怎么准备">我决定保研了</button>' +
    '<button class="chip-btn alt" data-q="最近好累，感觉撑不住了">最近好累</button></div></div>' +
    '<div class="home-thread" id="home-thread"></div>' +
    '<div class="home-composer">' +
    '<input id="home-input" placeholder="直接说就行……" enterkeyhint="send" autocomplete="off">' +
    '<button class="btn-main" id="home-send">发送</button></div></div></div>';
}
function homeRouteMeta(res) {
  if (res.next === 'match') return ['explore', '要动手了 → 匹配能做完的 Skill'];
  if (res.topic === 'friendship') return ['explore', '交朋友 · 不是恋爱'];
  if (res.topic === 'romance') return ['reject', '抉择型 · 不回答该不该'];
  return { explore: ['explore', '先聊清楚'], decide: ['reject', '抉择型 · 不回答该不该'],
    action: ['explore', '要动手了 → 匹配能做完的 Skill'], emotion: ['stop', '已拦截 · 不进任何流'] }[res.type] || ['explore', '已路由'];
}
function ctxRefsHtml(res) {
  var ctx = res && res.context;
  if (!ctx) return '';
  var bits = [];
  (ctx.experiences || []).slice(0, 2).forEach(function (e) {
    var q = (e.quote || '').replace(/^「/, '').replace(/」.*$/, '');
    if (q) bits.push('<span class="ctx-chip"><i>经验</i>' + esc(q.slice(0, 28)) + (q.length > 28 ? '…' : '') + '</span>');
  });
  (ctx.junctions || []).slice(0, 1).forEach(function (j) {
    if (!j || !j.title) return;
    var href = j.id ? '#/junction/' + j.id : '#/paths';
    bits.push('<a class="ctx-chip" href="' + href + '"><i>入口</i>' + esc(j.title) + (j.total ? ' · ' + j.total + ' 人' : '') + '</a>');
  });
  // Skill 卡片：只要 context.skills 有内容就渲染（不限于 next==='match'），
  // 渲染成可点击的迷你卡片，点击直接跳到 Skill 详情页
  (ctx.skills || []).slice(0, 4).forEach(function (s) {
    if (!s || !s.title) return;
    var sub = s.subtitle || '';
    bits.push('<a class="skill-mini-card" href="#/skill/' + s.id + '">' +
      '<span class="smc-type">' + esc(s.type || s.category || 'Skill') + '</span>' +
      '<span class="smc-title">' + esc(s.title) + '</span>' +
      (sub ? '<span class="smc-sub">' + esc(sub.slice(0, 30)) + (sub.length > 30 ? '…' : '') + '</span>' : '') +
      '</a>');
  });
  if (!bits.length) return '';
  return '<div class="ctx-refs">' + bits.join('') + '</div>';
}

function applyDiscuss(res, d) {
  res.reply = d.reply || res.reply;
  res.next = d.next || res.next;
  res.live = d.live || res.live;
  res.sessionId = d.sessionId;
  res.memory = d.memory;
  res.context = d.context;
  if (d.junctionId) res.junctionId = d.junctionId;
  if (d.type) res.type = d.type;
  if (d.topic) res.topic = d.topic;
  if (d.degraded) res.degraded = true;
  if (d.sessionId) wowSessionId = d.sessionId;
  return res;
}
function homeRouteBtns(res) {
  var btns = '';
  if (!res) return btns;
  if (res.needLogin) btns += '<button class="btn-main btn-sm" data-nav-to="#/login">登录后继续聊 →</button>';
  if (res.type === 'emotion') return btns;
  if (res.next === 'match' || res.type === 'action') {
    btns += '<button class="btn-main btn-sm" data-nav-to="#/match">去匹配能帮你做完这件事的 Skill →</button>';
  }
  return btns;
}
function paintHomeThread(root) {
  var thread = $('#home-thread', root);
  if (!thread) return;
  thread.innerHTML = homeChat.map(function (m) {
    if (m.role === 'me') {
      return '<div class="msg me"><div class="bubble">' + nl2br(m.text) + '</div></div>';
    }
    if (m.thinking) {
      return '<div class="msg bot"><div class="bubble thinking">在听…</div></div>';
    }
    var res = m.route || {};
    var badge = homeRouteMeta(res);
    var liveTag = res.live
      ? '<span class="route-badge explore">多轮对话 · 有记忆</span>'
      : (res.needLogin ? '<span class="route-badge reject">本地预览 · 登录后走真实 AI</span>' : '<span class="route-badge explore">本地多轮</span>');
    var btns = homeRouteBtns(res);
    return '<div class="msg bot">' + liveTag +
      (res.type ? '<span class="route-badge ' + badge[0] + '">' + badge[1] + '</span>' : '') +
      (res.memory && res.memory.situation ? '<div class="heard">还记得：' + esc(res.memory.situation) + '</div>' : (res.heard ? '<div class="heard">听到的是：' + esc(res.heard) + '</div>' : '')) +
      '<div class="bubble">' + nl2br(m.text) + '</div>' +
      ctxRefsHtml(res) +
      (btns ? '<div class="actions">' + btns + '</div>' : '') + '</div>';
  }).join('');
  thread.scrollTop = thread.scrollHeight;
  var chips = $('#home-chips', root);
  if (chips) chips.hidden = homeChat.length > 0;
  $all('[data-nav-to]', thread).forEach(function (b) {
    b.addEventListener('click', function () { location.hash = b.getAttribute('data-nav-to'); });
  });
  $all('[data-go]', thread).forEach(function (b) {
    b.addEventListener('click', function () { if (lastRoute) goAfterRoute(lastRoute); });
  });
  bindReroute(thread);
}
function bindHome(root) {
  function setBusy(on) {
    homeBusy = on;
    var inp = $('#home-input', root);
    var btn = $('#home-send', root);
    if (inp) inp.disabled = on;
    if (btn) btn.disabled = on;
  }
  function go(q) {
    if (homeBusy || !q) return;
    var prev = lastRoute;
    var history = homeChat.filter(function (m) { return m && !m.thinking && m.text; }).map(function (m) {
      return { role: m.role === 'me' ? 'user' : 'assistant', content: m.text };
    });
    lastQuery = q;
    homeChat.push({ role: 'me', text: q });
    homeChat.push({ role: 'bot', thinking: true });
    setBusy(true);
    paintHomeThread(root);
    $('#home-input', root).value = '';

    function finish(res) {
      lastRoute = res;
      homeChat.pop();
      homeChat.push({ role: 'bot', text: res.reply, route: res });
      setBusy(false);
      paintHomeThread(root);
      if (res.next === 'match' && res.type !== 'emotion') {
        setTimeout(function () { location.hash = '#/match'; }, 350);
        return;
      }
      $('#home-input', root).focus();
    }
    function fail(err) {
      homeChat.pop();
      if (err && err.needLogin) {
        homeChat.push({
          role: 'bot',
          text: '这句话会进真实对话，先登录一下。',
          route: { needLogin: true, type: '' }
        });
      } else {
        homeChat.push({ role: 'bot', text: '暂时走不通：' + (err.message || err) });
      }
      setBusy(false);
      paintHomeThread(root);
      $('#home-input', root).focus();
    }

    var seed = Object.assign({}, prev || {}, { heard: q });
    if (!seed.type) seed.type = 'explore';
    WowAPI.discuss(q, seed, history, null, wowSessionId).then(function (d) {
      var res = applyDiscuss(seed, d);
      if (res.type === 'emotion') res.next = 'stop';
      return res;
    }).then(finish).catch(fail);
  }
  paintHomeThread(root);
  $('#home-send', root).addEventListener('click', function () {
    go($('#home-input', root).value.trim());
  });
  $('#home-input', root).addEventListener('keydown', function (e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      go(this.value.trim());
    }
  });
  $all('#home-chips .chip-btn', root).forEach(function (b) {
    b.addEventListener('click', function () {
      var q = b.getAttribute('data-q');
      $('#home-input', root).value = q;
      go(q);
    });
  });
  setTimeout(function () { var i = $('#home-input', root); if (i) i.focus(); }, 0);
}

/* ============================================================
 * 视图：阶段页
 * ============================================================ */
function viewStage(stageId) {
  var st = stageOf(stageId);
  if (!st) return '<div class="empty-box">阶段不存在</div>';

  var junctionHtml = '';
  if (st.junctionId) {
    var j = DB.junctions[st.junctionId];
    junctionHtml = '<div class="junction-banner" data-nav-to="#/junction/' + j.id + '">' +
      '<div><div class="jb-t">🛤️ ' + esc(j.title) + '</div>' +
      '<div class="jb-s">' + j.total + ' 位学长走过这个路口 · 只给绝对人数与代价原话，不给建议</div></div>' +
      '<div class="jb-go">看分叉 →</div></div>';
  }

  var scenesHtml = st.scenes.map(function (sc) {
    var tasksHtml = sc.tasks.map(function (t) {
      var right = '';
      if (t.junction) {
        right = '<button class="btn-ghost btn-sm" data-nav-to="#/junction/' + t.junction + '">🛤️ 看真实分叉 →</button>';
      } else if (t.emotion) {
        right = '<span class="badge coral">💛 情绪输入 · 拦截保护，不进流程</span>';
      } else if (t.skillIds && t.skillIds.length) {
        right = t.skillIds.map(skillChipHtml).join('');
      }
      if (t.wish) {
        var wished = State.wished[t.id];
        right += '<span class="wish-chip" data-wish="' + t.id + '">' +
          (wished ? '✅ 已挂许愿池（' + wished + ' 人在等）' : '🙋 还没有 Skill · 挂许愿池') + '</span>';
      }
      return '<div class="task-row"><div class="t-main">' +
        '<div class="t-title">' + esc(t.title) + '</div>' +
        '<div class="t-desc">' + esc(t.desc) + '</div></div>' +
        '<div class="t-skills">' + right + '</div></div>';
    }).join('');
    return '<div class="scene-block">' +
      '<div class="scene-head-row"><h2>📌 ' + esc(sc.title) + '</h2>' +
      '<span class="sc-desc">' + esc(sc.desc) + '</span></div>' + tasksHtml + '</div>';
  }).join('');

  return '<div class="view">' +
    '<div class="crumb"><a href="#/home">首页</a> / ' + esc(st.name) + '</div>' +
    '<div class="page-head"><h1>' + st.emoji + ' ' + esc(st.name) + '</h1><p>' + esc(st.desc) + '</p></div>' +
    junctionHtml + scenesHtml +
    '<div class="n-note">※ 任务没有对应 Skill 时我们直接说"还没有"，并挂到许愿池——AI 编的第一步是同类产品的通用坟场，我们不编。</div>' +
    '</div>';
}
function bindStage(root) {
  bindSkillLinks(root);
  $all('[data-nav-to]', root).forEach(function (b) {
    b.addEventListener('click', function () { location.hash = b.getAttribute('data-nav-to'); });
  });
  $all('[data-wish]', root).forEach(function (w) {
    w.addEventListener('click', function () {
      var id = w.getAttribute('data-wish');
      if (State.wished[id]) { location.hash = '#/wishes'; return; }
      if (!WowAPI.auth.isLoggedIn()) {
        toast('登录后才能把缺口挂到许愿池');
        location.hash = '#/login';
        return;
      }
      var task = findTask(id);
      if (!task) return;
      w.textContent = '挂上…';
      WowAPI.hangWish(task.title, (task.desc || '') + '\n来源阶段任务：' + id).then(function (res) {
        State.wished[id] = res.waiting || 1;
        persistWished();
        w.textContent = '✅ 已挂许愿池（' + State.wished[id] + ' 人在等）';
        toast(res.created ? '缺口已挂上。有人走通会来认领。' : '你也在等这张卡了', true);
      }).catch(function (err) {
        w.textContent = '🙋 还没有 Skill · 挂许愿池';
        toast(err.message || '没挂上');
      });
    });
  });
}

/* ============================================================
 * 视图：探索流（访谈 → 匹配卡）
 * ============================================================ */
function viewExplore() {
  var st = stageOf(exploreState.stageId) || stageOf('y1');
  var q = EXPLORE_Q[exploreState.step];
  if (exploreState.step < EXPLORE_Q.length) {
    var opts = q.opts.map(function (o) {
      return '<button class="chip-btn" data-ans="' + esc(o) + '">' + esc(o) + '</button>';
    }).join('');
    return '<div class="view">' +
      '<div class="crumb"><a href="#/home">首页</a> / 探索</div>' +
      '<div class="hero">' +
      '<span class="route-badge explore">轻访谈 ' + (exploreState.step + 1) + '/' + EXPLORE_Q.length + ' · 不测评</span>' +
      '<h1>' + esc(q.q) + '</h1>' +
      '<div class="sub">你刚才说：' + esc(exploreState.query || '（从首页进来）') + '</div>' +
      '<div class="hero-chips">' + opts + '</div>' +
      rerouteHtml() + '</div></div>';
  }
  var cards = skillsOfStage(st.id).slice(0, 3);
  var quote = exploreState.query ? '你说「' + exploreState.query + '」' : '按你的阶段';
  var cardsHtml = cards.map(function (s) {
    return '<div class="match-wrap">' + skillCardHtml(s) +
      '<div class="match-why">推荐理由：' + quote + ' → 处境接近「' + esc(s.match || s.subtitle) + '」</div></div>';
  }).join('');
  return '<div class="view">' +
    '<div class="crumb"><a href="#/home">首页</a> / 探索 / ' + esc(st.short) + '</div>' +
    '<div class="page-head"><h1>先试这几件小事</h1>' +
    '<p>依据：' + esc(quote) + ' · 每周 ' + esc(exploreState.answers.hours || '—') + ' · 硬约束 ' + esc(exploreState.answers.hard || '—') + '</p></div>' +
    '<div class="grid-3">' + (cardsHtml || '<div class="empty-box">这个阶段还没有卡</div>') + '</div>' +
    '<div class="actions"><a class="btn-ghost" href="#/stage/' + st.id + '">看完整 ' + esc(st.short) + '</a>' +
    '<a class="btn-ghost" href="#/paths">去路口</a></div>' + rerouteHtml() + '</div>';
}
function bindExplore(root) {
  bindSkillLinks(root);
  bindReroute(root);
  $all('[data-ans]', root).forEach(function (b) {
    b.addEventListener('click', function () {
      var q = EXPLORE_Q[exploreState.step];
      exploreState.answers[q.key] = b.getAttribute('data-ans');
      exploreState.step++;
      render();
    });
  });
}

/* ============================================================
 * 视图：按原话找 Skill（抉择/探索的下一步）
 * ============================================================ */
function viewMatch() {
  var q = matchTaskLabel();
  return '<div class="view">' +
    '<div class="crumb"><a href="#/home">首页</a> / 匹配 Skill</div>' +
    '<div class="page-head"><h1>匹配能帮你做完这件事的 Skill</h1>' +
    '<p>你要做的是「' + esc(q || '（从对话进来）') + '」。对得上就装载去做，对不上就去许愿池——不编一张假卡。</p></div>' +
    '<div id="match-grid"><div class="n-note">正在按你要做的事找卡…</div></div></div>';
}
function bindMatch(root) {
  var q = matchTaskLabel();
  var local = matchSkills(q);
  ((lastRoute && lastRoute.context && lastRoute.context.skills) || []).forEach(function (ref) {
    var s = ref && DB.skills[ref.id];
    if (s && local.indexOf(s) < 0) local.unshift(s);
  });
  function paint(list) {
    var grid = (root && document.body.contains(root) && $('#match-grid', root)) || document.querySelector('#match-grid');
    if (!grid) return;
    var seen = {};
    var merged = [];
    local.concat(list || []).forEach(function (s) {
      if (!s || !s.id || seen[s.id]) return;
      seen[s.id] = true;
      merged.push(s);
    });
    var html;
    try {
      html = merged.length
        ? '<div class="grid-3">' + merged.slice(0, 6).map(function (s) {
            return '<div class="match-wrap">' + skillCardHtml(s) +
              '<div class="match-why">这张卡是别人把这件事做完的方法。装载后按剧本走，不承诺结果。</div></div>';
          }).join('') + '</div>'
        : '<div class="empty-box">还没有能帮你做完这件事的卡。<br>我们不编一张假的。<br><br>' +
          '<button class="btn-main" data-nav-to="#/wishes">去许愿池挂这个任务 →</button></div>';
    } catch (err) {
      html = '<div class="empty-box">还没有能帮你做完这件事的卡。<br>我们不编一张假的。<br><br>' +
        '<button class="btn-main" data-nav-to="#/wishes">去许愿池挂这个任务 →</button></div>';
    }
    var extra = '<div class="actions" style="margin-top:16px">' +
      (lastRoute && lastRoute.junctionId ? '<a class="btn-ghost" href="#/junction/' + lastRoute.junctionId + '">回看真实分叉</a>' : '') +
      '<a class="btn-ghost" href="#/market">去市场自己搜</a>' +
      '<a class="btn-ghost" href="#/wishes">没有就挂许愿池</a></div>' +
      '<div class="n-note">匹配不是给建议。能装载的是别人做过这件事的方法；对不上就说还没有，去许愿池。</div>';
    grid.innerHTML = html + extra;
    bindSkillLinks(grid);
    $all('[data-nav-to]', grid).forEach(function (b) {
      b.addEventListener('click', function () { location.hash = b.getAttribute('data-nav-to'); });
    });
  }
  paint([]);
  var tokens = queryTokens(matchSearchBlob() || q).filter(function (t) {
    return t && t.length >= 2 && t.length <= 6;
  });
  var prefer = tokens.filter(function (t) {
    return /朋友|社交|搭子|破冰|恋爱|保研|转专业|实习|考研|就业|论文/.test(t);
  });
  var kw = prefer[0] || tokens[0] || '';
  if (!kw) return;
  WowAPI.listSkills({ q: kw }).then(function (list) {
    paint((list || []).filter(function (s) { return s.fromBackend; }));
  }).catch(function () { paint([]); });
}

/* ============================================================
 * 视图：路口索引
 * ============================================================ */
function viewPaths() {
  var html = junctionList().map(function (j) {
    var max = Math.max.apply(null, j.branches.map(function (b) { return b.count; }));
    var bars = j.branches.map(function (b) {
      return '<div class="mini-bar"><span>' + esc(b.name) + '</span>' +
        '<i style="width:' + (b.count / max * 100) + '%;background:' + b.color + '"></i>' +
        '<b>' + b.count + '</b></div>';
    }).join('');
    return '<a class="path-full" href="#/junction/' + j.id + '">' +
      '<div class="pf-top"><h3>' + esc(j.title) + '</h3><span>' + j.total + ' 人</span></div>' +
      bars + '<div class="pf-go">看代价原话 →</div></a>';
  }).join('');
  return '<div class="view">' +
    '<div class="page-head"><h1>🛤️ 路口</h1>' +
    '<p>我们不替你选。四个路口，只给走过的人数和一句代价。</p></div>' +
    '<div class="path-list">' + html + '</div>' + rerouteHtml() + '</div>';
}
function bindPaths(root) { bindReroute(root); }

/* ============================================================
 * 视图：编排态
 * ============================================================ */
function viewOrch(intent) {
  intent = intent || 'postgrad_recommend';
  return '<div class="view" id="orch-view">' +
    '<div class="crumb"><a href="#/home">首页</a> / 编排</div>' +
    '<div class="page-head"><h1>按别人走过的路来排</h1>' +
    '<p>不承诺结果。没人走过的方向，我们拒绝生成。</p></div>' +
    '<div class="card" id="orch-body">正在查看有没有人走过…</div>' +
    rerouteHtml() + '</div>';
}
function bindOrch(root, intent) {
  bindReroute(root);
  var body = $('#orch-body', root);
  WowAPI.probeOrchestration(intent || 'postgrad_recommend', lastQuery).then(function (r) {
    if (!r || r.available === false) {
      body.innerHTML = '<span class="route-badge stop">拒绝生成</span>' +
        '<h2 style="font-size:18px;font-weight:900;margin:8px 0">' + esc((r && r.message) || '这条路还没人走完过。') + '</h2>' +
        '<p class="n-note">这一幕比生成成功更能说明判断力：没有来源 Path，就不编时间表。</p>' +
        '<div class="actions"><a class="btn-main" href="#/paths">看相邻路口</a>' +
        '<a class="btn-ghost" href="#/explore">我先去做第一件事</a></div>';
      return;
    }
    var path = (r.source_paths && r.source_paths[0]) || {};
    var nodes = path.nodes || path.Nodes || [];
    var nodeHtml = nodes.map(function (n, i) {
      var label = n.label || n.Label || ('节点 ' + (i + 1));
      var week = n.week_offset != null ? n.week_offset : n.WeekOffset;
      var ctrl = n.controllable != null ? n.controllable : n.Controllable;
      return '<div class="orch-node' + (ctrl === false ? ' wait' : '') + '">' +
        '<div class="on-w">第 ' + (week != null ? week : i) + ' 周</div>' +
        '<div class="on-t">' + esc(label) + '</div>' +
        '<div class="on-c">' + (ctrl === false ? '不可控 · 只看见，不勾选' : '你能推进') + '</div></div>';
    }).join('');
    body.innerHTML = '<span class="route-badge explore">有人走过 · ' + (r.walked_total || 0) + ' 人</span>' +
      '<h2 style="font-size:18px;font-weight:900;margin:8px 0">' + esc(r.label || path.goal_label || '编排') + '</h2>' +
      '<p class="n-note">' + esc(r.provenance_note || '来源为回忆整理，不给成功率。') + '</p>' +
      '<div class="orch-nodes">' + (nodeHtml || '<div class="n-note">节点加载中</div>') + '</div>' +
      '<div class="actions"><a class="btn-main" href="#/market">装载对应 Skill</a></div>';
  }).catch(function (err) {
    if (err && err.needLogin) {
      body.innerHTML = '编排需要登录。<div class="actions"><a class="btn-main" href="#/login">去登录</a></div>';
      return;
    }
    body.innerHTML = '暂时看不了：' + esc(err.message || err);
  });
}

/* ============================================================
 * 视图：Skill 市场
 * ============================================================ */
var marketFilter = { stage: '', type: '', freeOnly: false, q: '' };
function viewMarket() {
  var stageOpts = '<option value="">全部阶段</option>' + DB.stages.map(function (s) {
    return '<option value="' + s.id + '"' + (marketFilter.stage === s.id ? ' selected' : '') + '>' + esc(s.short) + '</option>';
  }).join('');
  var types = ['剧本', '清单', '模板', '提示词包', 'Agent Skill'];
  var typeOpts = '<option value="">全部类型</option>' + types.map(function (t) {
    return '<option value="' + t + '"' + (marketFilter.type === t ? ' selected' : '') + '>' + t + '</option>';
  }).join('');
  return '<div class="view">' +
    '<div class="page-head"><h1>🃏 Skill 市场</h1>' +
    '<p>每个 Skill 都有验证数、边界与去向分布 —— 没有评分，没有销量排行，信任来自证据</p></div>' +
    '<div class="filterbar">' +
    '<select id="f-stage">' + stageOpts + '</select>' +
    '<select id="f-type">' + typeOpts + '</select>' +
    '<label><input type="checkbox" id="f-free"' + (marketFilter.freeOnly ? ' checked' : '') + '> 只看免费</label>' +
    '<input type="text" id="f-q" placeholder="搜标题 / 创建者…" value="' + esc(marketFilter.q) + '">' +
    '<span class="n-note" style="margin:0" id="f-count"></span></div>' +
    '<div class="grid-3" id="market-grid"></div></div>';
}
function bindMarket(root) {
  function refresh() {
    WowAPI.listSkills(marketFilter).then(function (list) {
      $('#market-grid', root).innerHTML = list.length
        ? list.map(skillCardHtml).join('')
        : '<div class="empty-box">没有匹配的 Skill<br><br><button class="btn-main" data-nav-to="#/wishes">去许愿池挂一个 →</button></div>';
      $('#f-count', root).textContent = '共 ' + list.length + ' 个';
      bindSkillLinks(root);
      $all('[data-nav-to]', root).forEach(function (b) {
        b.addEventListener('click', function () { location.hash = b.getAttribute('data-nav-to'); });
      });
    });
  }
  $('#f-stage', root).addEventListener('change', function () { marketFilter.stage = this.value; refresh(); });
  $('#f-type', root).addEventListener('change', function () { marketFilter.type = this.value; refresh(); });
  $('#f-free', root).addEventListener('change', function () { marketFilter.freeOnly = this.checked; refresh(); });
  $('#f-q', root).addEventListener('input', function () { marketFilter.q = this.value.trim(); refresh(); });
  refresh();
}

/* ============================================================
 * 视图：许愿池（复用论坛 looking_for）
 * ============================================================ */
function fmtShort(t) {
  if (!t) return '';
  var d = new Date(t);
  if (isNaN(d.getTime())) return '';
  return (d.getMonth() + 1) + '/' + d.getDate();
}
function wishCardHtml(t) {
  return '<a class="wish-card" href="#/wish/' + t.id + '">' +
    '<h3>' + esc(t.title) + '</h3>' +
    '<div class="wc-meta">' + esc(t.username || '匿名') + ' · ' +
    (t.like_count || 0) + ' 人在等 · ' + (t.reply_count || 0) + ' 条回应' +
    (t.created_at ? ' · ' + fmtShort(t.created_at) : '') + '</div></a>';
}
function viewWishes() {
  var gaps = gapTasks().map(function (g) {
    var on = State.wished[g.id];
    return '<button class="wish-gap' + (on ? ' on' : '') + '" data-gap="' + g.id + '">' +
      '<span class="wg-st">' + esc(g.stage) + '</span>' +
      '<span class="wg-t">' + esc(g.title) + '</span>' +
      '<span class="wg-a">' + (on ? '已在等' : '挂上') + '</span></button>';
  }).join('');
  return '<div class="view">' +
    '<div class="page-head"><h1>许愿池</h1>' +
    '<p>市场里没有的卡，挂在这里。点「我也在等」是排队，走过的人可以回一句——我们不编一张假 Skill。</p></div>' +
    '<div class="card wish-form">' +
    '<div class="sec-t">挂一个缺口</div>' +
    '<input id="wish-title" placeholder="还没有哪张卡？一句话说清（至少 3 个字）">' +
    '<textarea id="wish-body" rows="3" placeholder="可选：你卡在哪、试过什么。不需要写成方法。"></textarea>' +
    '<button class="btn-main" id="wish-hang">挂上许愿池</button></div>' +
    '<div class="section-title">阶段里还缺的卡</div>' +
    '<div class="wish-gaps">' + gaps + '</div>' +
    '<div class="section-title">正在等的人</div>' +
    '<div id="wish-list"><div class="n-note">加载中…</div></div></div>';
}
function bindWishes(root) {
  var titleEl = $('#wish-title', root);
  if (titleEl && lastQuery && !titleEl.value) {
    titleEl.value = lastQuery.replace(/\s+/g, ' ').slice(0, 40);
  }
  function needLogin(err) {
    if (err && err.needLogin) {
      toast('登录后才能挂愿望');
      location.hash = '#/login';
      return true;
    }
    return false;
  }
  function refresh() {
    WowAPI.listWishes().then(function (list) {
      $('#wish-list', root).innerHTML = list.length
        ? list.map(wishCardHtml).join('')
        : '<div class="empty-box">还没有人挂缺口。市场搜不到的东西，值得先挂在这里。</div>';
    });
  }
  function hang(title, content, taskId) {
    if (!title || title.length < 3) { toast('标题太短了，多说几个字'); return; }
    WowAPI.hangWish(title, content || '').then(function (res) {
      if (taskId) { State.wished[taskId] = res.waiting || 1; persistWished(); }
      toast(res.created ? '缺口已挂上' : '你也在等这张卡了', true);
      if (res.id) location.hash = '#/wish/' + res.id;
      else refresh();
    }).catch(function (err) {
      if (needLogin(err)) return;
      toast(err.message || '没挂上');
    });
  }
  $('#wish-hang', root).addEventListener('click', function () {
    hang($('#wish-title', root).value.trim(), $('#wish-body', root).value.trim());
  });
  $all('[data-gap]', root).forEach(function (b) {
    b.addEventListener('click', function () {
      var id = b.getAttribute('data-gap');
      var task = findTask(id);
      if (!task) return;
      if (State.wished[id]) { hang(task.title, task.desc, id); return; }
      hang(task.title, (task.desc || '') + '\n来源阶段任务：' + id, id);
    });
  });
  refresh();
}
function viewWish(id) {
  return '<div class="view" id="wish-detail"><div class="n-note">加载中…</div></div>';
}
function bindWish(root, id) {
  function paint(pack) {
    var t = pack.wish || {};
    var replies = pack.replies || [];
    var replyHtml = replies.length
      ? replies.map(function (r) {
          return '<div class="wish-reply"><div class="wr-h">' + esc(r.username || '匿名') +
            ' · ' + fmtShort(r.created_at) + '</div><div class="wr-b">' + nl2br(r.content) + '</div></div>';
        }).join('')
      : '<div class="n-note">还没有人走过。走过的人可以回一句，不需要写成方法。</div>';
    root.innerHTML = '<div class="view"><div class="crumb"><a href="#/wishes">许愿池</a> / ' + esc(t.title || '') + '</div>' +
      '<div class="page-head"><h1>' + esc(t.title || '愿望') + '</h1>' +
      '<p>' + esc(t.username || '匿名') + ' 挂出 · ' + (t.like_count || 0) + ' 人在等 · ' +
      (t.reply_count || 0) + ' 条回应</p></div>' +
      (t.content ? '<div class="card"><div class="wish-body">' + nl2br(t.content) + '</div></div>' : '') +
      '<div class="actions">' +
      '<button class="btn-main" id="wish-wait">' + (t.liked ? '✓ 你已在等（' + (t.like_count || 0) + '）' : '🙋 我也在等') + '</button>' +
      '<a class="btn-ghost" href="#/publish">我走过，去沉淀 →</a></div>' +
      '<div class="section-title">走过的人</div>' + replyHtml +
      '<div class="card wish-form"><div class="sec-t">回一句</div>' +
      '<textarea id="wish-reply" rows="3" placeholder="你走过的话，说一句就够。不承诺、不建议。"></textarea>' +
      '<button class="btn-main" id="wish-send">送出回应</button></div></div>';
    $('#wish-wait', root).addEventListener('click', function () {
      if (t.liked) { toast('你已经在等了'); return; }
      WowAPI.waitWish(id).then(function () {
        toast('你也在等这张卡了', true);
        WowAPI.getWish(id).then(paint);
      }).catch(function (err) {
        if (err && err.needLogin) { toast('登录后才能排队'); location.hash = '#/login'; return; }
        toast(err.message || '没加上');
      });
    });
    $('#wish-send', root).addEventListener('click', function () {
      var txt = $('#wish-reply', root).value.trim();
      if (!txt) { toast('写一句再送'); return; }
      WowAPI.replyWish(id, txt).then(function () {
        toast('回应已送出', true);
        WowAPI.getWish(id).then(paint);
      }).catch(function (err) {
        if (err && err.needLogin) { toast('登录后才能回应'); location.hash = '#/login'; return; }
        toast(err.message || '没送出');
      });
    });
  }
  WowAPI.getWish(id).then(paint).catch(function (err) {
    root.innerHTML = '<div class="empty-box">这条愿望找不到了<br><br><a class="btn-main" href="#/wishes">回许愿池</a></div>';
    if (err) toast(err.message || '加载失败');
  });
}

/* ============================================================
 * 视图：Skill 详情
 * ============================================================ */
function viewSkill(id) {
  var s = DB.skills[id];
  if (!s) return '<div class="empty-box">Skill 不存在</div>';
  var st = stageOf(s.stageId);

  var scriptHtml = (Array.isArray(s.script) ? s.script : []).map(function (row) {
    return '<div class="script-item"><span class="day">' + esc(row[0]) + '</span><span>' + esc(row[1]) + '</span></div>';
  }).join('');

  var distSection = '';
  if (s.dist && s.dist.length) {
    var max = Math.max.apply(null, s.dist.map(function (d) { return d[1]; }));
    var distHtml = s.dist.map(function (d) {
      return '<div class="dist-row"><span class="lbl">' + esc(d[0]) + '</span>' +
        '<span class="bar" style="width:' + (d[1] / max * 180) + 'px;background:' + d[2] + '"></span>' +
        '<span class="num">' + d[1] + ' 人</span></div>';
    }).join('');
    distSection = '<div class="sec"><div class="sec-t">🧭 用过的人后来去了哪（绝对人数 · 无百分比）</div>' + distHtml + '</div>';
  }

  var reviewsHtml = (s.reviews || []).map(function (r) {
    return '<div class="review"><div class="r-head">' +
      '<span class="badge ' + (r.verdict ? 'green' : 'coral') + '">' + (r.verdict ? '成立 ✓' : '不成立 ✗') + '</span>' +
      '<span>' + esc(r.by) + '</span></div>' +
      (r.flow ? '<div class="r-q">🕐 忘了时间的时刻："' + esc(r.flow) + '"</div>' : '') +
      (r.escape ? '<div class="r-q">🏃 想逃的时刻："' + esc(r.escape) + '"</div>' : '') +
      '</div>';
  }).join('') || '<div class="n-note">还没有 verdict 反馈</div>';

  var versionsSection = '';
  if (s.versions && s.versions.length) {
    versionsSection = '<div class="sec"><div class="sec-t">🕰️ 版本历史（反馈驱动迭代）</div>' +
      s.versions.map(function (v) {
        return '<div class="version"><span class="v-tag">' + esc(v.v) + '</span><span>' + esc(v.date) + ' · ' + esc(v.note) + '</span></div>';
      }).join('') + '</div>';
  }

  /* 真实开源 Skill：来源与安装 */
  var realSection = '';
  if (s.real) {
    realSection = '<div class="sec"><div class="sec-t">🔗 来源与安装（真实开源 Skill）</div>' +
      '<div class="combo-row" style="flex-direction:column;align-items:flex-start;gap:6px">' +
      '<span>📦 仓库：<a href="' + esc(s.real.url) + '" target="_blank" rel="noopener" style="color:var(--violet);font-weight:800">' + esc(s.real.repo) + '</a></span>' +
      '<span style="font-family:ui-monospace,Menlo,monospace;font-size:12px;background:var(--ink);color:var(--cream);border-radius:8px;padding:6px 12px;word-break:break-all">$ ' + esc(s.real.install) + '</span>' +
      '</div></div>';
  }

  /* AI 使用指南：零基础也能照做的「在哪用 / 装什么 / 用什么 prompt / 怎么下载」 */
  var explainSection = '<div class="sec"><div class="sec-t">🤖 怎么用（AI 读 skill 包内容生成 · 零基础也能照做）</div>' +
    '<button class="btn-main btn-sm" id="explain-btn">生成使用指南 →</button>' +
    '<div id="explain-box" class="md-body" style="display:none"></div></div>';

  var combosHtml = '';
  if (s.combos && s.combos.length) {
    var chain = [s.id].concat(s.combos).map(function (cid) {
      var cs = DB.skills[cid];
      return cid === s.id
        ? '<b>' + esc(cs.title) + '</b>'
        : '<span class="skill-chip" data-goskill="' + cid + '">' + esc(cs.title) + '</span>';
    }).join('<span class="arrow">→</span>');
    combosHtml = '<div class="sec"><div class="sec-t">🧩 常见组合（Path）· 可叠加装载，边界约束同时生效</div>' +
      '<div class="combo-row">' + chain + '</div></div>';
  }

  var variantsHtml = '';
  if (s.variants && s.variants.length) {
    variantsHtml = '<div class="sec"><div class="sec-t">🌱 社区变体（谱系）</div>' +
      s.variants.map(function (v) {
        return '<div class="combo-row"><span class="badge amber">变体</span><b>' + esc(v.title) + '</b>' +
          '<span class="lineage">by ' + esc(v.by) + ' · ' + esc(v.note) + '</span></div>';
      }).join('') + '</div>';
  }

  var loaded = isLoaded(s.id);
  var owned = s.price === 0 || State.purchased[s.id];
  var actionLabel = loaded ? '✅ 已装载 · 去运行时看看' :
    owned ? '⚡ 装载这个 Skill · 开始运行' : '🪙 用 ' + s.price + ' 积分兑换并装载';

  return '<div class="view">' +
    '<div class="crumb"><a href="#/home">首页</a> / <a href="#/market">Skill 市场</a> / ' + esc(s.title) + '</div>' +
    '<div class="card">' +
    '<div class="detail-head"><div class="dh-main">' +
    '<span class="card-kind">' + esc(s.type) + ' · ' + esc(s.duration) + '</span>' +
    (st ? ' <span class="badge violet">' + esc(st.name) + '</span>' : '') +
    '<h1>' + esc(s.title) + '</h1>' +
    '<div class="k-sub">' + esc(s.subtitle) + '</div>' +
    '<div class="creator"><div class="c-ava" style="background:' + s.creator.color + '">' + esc(s.creator.initial) + '</div>' +
    '<div><div class="c-name">' + esc(s.creator.name) + '</div><div class="c-meta">' + esc(s.creator.meta) + '</div></div></div>' +
    '<div class="stat-row">' +
    (s.verify
      ? '<span class="stat-pill v">✓ ' + s.verify.pass + '/' + s.verify.total + ' 同处境验证成立</span>'
      : (s.real ? '<span class="stat-pill v">🌐 开源 Skill · 社区维护 · 平台 verdict 从 0 开始积累</span>' : '')) +
    '<span class="stat-pill c">🎯 ' + esc(s.match) + '</span>' +
    (s.price > 0 ? '<span class="stat-pill a">🪙 ' + s.price + ' 积分 · 创建者可获得回报</span>' : '<span class="stat-pill a">免费</span>') +
    '</div></div></div>' +

    '<div class="sec"><div class="sec-t">📋 剧本 / 用法（完成标准都在你自己的行动上）</div>' + scriptHtml + '</div>' +
    '<div class="sec dp"><div class="dp-t">⏰ 判断点 · 装载后会在对的时间自动出现</div>' + esc(s.judge) + '</div>' +
    '<div class="sec boundary"><div class="b-t">🚫 不适合谁（这个 Skill 自己知道什么时候失效）</div>' + esc(s.boundary) + '</div>' +
    distSection +
    (s.story
      ? '<button class="story-toggle" id="story-t">📖 TA 的完整故事（口述原文）▾</button>' +
        '<div class="story-body" id="story-b">' + nl2br(s.story) + '</div>'
      : '') +
    '<div class="sec choose-if"><b>选它取决于什么：</b><br>' + esc(s.chooseIf) + '</div>' +
    combosHtml + variantsHtml + realSection + explainSection +
    '<div class="sec"><div class="sec-t">🗣️ verdict 反馈（两问原话，不打分）</div>' + reviewsHtml + '</div>' +
    versionsSection +
    '<div class="actions"><button class="btn-main" id="load-btn">' + actionLabel + '</button>' +
    '<button class="btn-ghost" data-nav-to="#/stage/' + s.stageId + '">回到 ' + (st ? esc(st.short) : '') + ' 阶段</button></div>' +
    '</div></div>';
}
function bindSkill(root, id) {
  var s = DB.skills[id]; if (!s) return;
  bindSkillLinks(root);
  $all('[data-nav-to]', root).forEach(function (b) {
    b.addEventListener('click', function () { location.hash = b.getAttribute('data-nav-to'); });
  });
  var storyT = $('#story-t', root);
  if (storyT) storyT.addEventListener('click', function () { $('#story-b', root).classList.toggle('open'); });
  /* AI 使用指南：调后端 explain API（读取 skill 包内 SKILL.md 真实内容，按用户水平生成四要素） */
  var explainBtn = $('#explain-btn', root);
  if (explainBtn) explainBtn.addEventListener('click', function () {
    var box = $('#explain-box', root);
    box.style.display = 'block';
    box.innerHTML = '<div class="n-note">🤖 AI 正在读取 skill 包内文件（SKILL.md / README / references）并生成使用指南…（约 10-30 秒，零基础用户会更详细）</div>';
    WowAPI.explainSkill(s.id).then(function (r) {
      box.innerHTML = mdRender(r.data || '');
      if (r.live && r.levelLabel) {
        var tag = document.createElement('div');
        tag.className = 'n-note';
        tag.innerHTML = '依据你的 AI 水平（' + esc(r.levelLabel) + '）生成，内容取自该 skill 包内真实文件。';
        box.appendChild(tag);
      }
    }).catch(function (err) {
      if (err && err.needLogin) {
        box.innerHTML = '<div class="n-note">需要登录后才能生成个性化指南。</div>' +
          '<div class="actions"><a class="btn-main btn-sm" href="#/login">去登录</a></div>';
      } else {
        box.innerHTML = '<div class="n-note">生成失败：' + esc(err && err.message ? err.message : String(err)) + '</div>';
      }
    });
  });
  $('#load-btn', root).addEventListener('click', function () {
    if (isLoaded(s.id)) {
      var exist = State.runs.filter(function (r) { return r.skillId === s.id && !r.finished; })[0];
      if (exist) activeRunId = exist.id;
      persistRuns();
      location.hash = '#/run';
      return;
    }
    var owned = s.price === 0 || State.purchased[s.id];
    if (!owned) {
      if (State.coins < s.price) { toast('积分不足（当前 ' + State.coins + '）——完成微尝试提交 verdict 可以赚积分'); return; }
      State.coins -= s.price;
      State.purchased[s.id] = true;
      DB.user.ledger.unshift({ date: '刚刚', item: '兑换「' + s.title + '」', delta: -s.price });
      syncTopbar();
      toast('🪙 已兑换 · ' + s.price + ' 积分 → 创建者 ' + s.creator.name + ' 获得回报');
    }
    var script = s.script && s.script.length
      ? s.script.map(function (row) { return { label: row[0] + ' ' + row[1], done: false, sub: '' }; })
      : [{ label: '按这张卡的步骤开始做', done: false, sub: '' }];
    var dur = s.duration || '';
    var newRun = {
      id: 'r' + Date.now(), skillId: s.id, day: 1,
      total: dur.indexOf('天') > -1 ? parseInt(dur, 10) || 14 : 0,
      startDate: '今天',
      checks: script,
      feed: [{ t: '⚡ 已装载', c: 'AI 现在带着「' + s.title + '」（' + s.creator.name + '）的经验包陪你跑。判断点会在对的时间出现；触碰边界时它会喊停。' }],
      finished: false
    };
    State.runs.unshift(newRun);
    activeRunId = newRun.id;
    persistRuns();
    toast('🃏 已装载「' + s.title + '」', true);
    setTimeout(function () { location.hash = '#/run'; }, 400);
  });
}

/* ============================================================
 * 视图：运行时（我的进行中）
 * ============================================================ */
function liveRuns() {
  return State.runs.filter(function (r) { return !r.finished && DB.skills[r.skillId]; });
}
function viewRun() {
  var runs = liveRuns();
  if (!runs.length) {
    return '<div class="view"><div class="page-head"><h1>进行中</h1><p>装载一张卡之后，陪跑会出现在这里。</p></div>' +
      '<div class="empty-box">还没有正在跑的 Skill<br><br>' +
      '<button class="btn-main" data-nav-to="#/market">去市场选一张 →</button></div></div>';
  }
  if (!runs.some(function (r) { return r.id === activeRunId; })) activeRunId = runs[0].id;
  var run = runs.filter(function (r) { return r.id === activeRunId; })[0];
  var s = DB.skills[run.skillId];
  if (!s) {
    return '<div class="view"><div class="empty-box">这张卡已经不在了</div></div>';
  }

  var tabsHtml = runs.map(function (r) {
    var rs = DB.skills[r.skillId];
    return '<button class="run-tab' + (r.id === activeRunId ? ' active' : '') + '" data-run="' + r.id + '">' +
      (r.persistent ? '🔁 ' : '🃏 ') + esc(rs ? rs.title.slice(0, 14) : r.skillId) + '…</button>';
  }).join('');

  var checksHtml = (run.checks || []).map(function (c, i) {
    return '<div class="check' + (c.done ? ' done' : '') + (c.diff ? ' diff' : '') + '" data-check="' + i + '">' +
      '<div class="box">' + (c.diff ? '≠' : (c.done ? '✓' : '')) + '</div>' +
      '<div class="txt">' + (c.diff ? c.label.replace(/实际：/, '<b>实际：</b>') : esc(c.label)) +
      (c.sub ? '<div class="sub">' + esc(c.sub) + '</div>' : '') + '</div></div>';
  }).join('');

  var feedHtml = run.feed.map(function (f) {
    var opts = (f.pendingDecision && f.options && f.options.length)
      ? '<div class="hero-chips dec-opts">' +
        f.options.map(function (o) {
          var v = typeof o === 'string' ? o : (o.label || o.text || o.title || '');
          return '<button class="chip-btn" data-dec-opt="' + esc(v) + '">' + esc(v) + '</button>';
        }).join('') + '</div>'
      : '';
    return '<div class="agent-msg' + (f.warn ? ' warn' : '') + (f.user ? ' user' : '') + '">' +
      '<div class="am-t">' + esc(f.t) + '</div>' + nl2br(f.c) + opts + '</div>';
  }).join('');

  var dayPill = run.persistent
    ? '<span class="day-pill">🔁 持续型 · 第 ' + run.day + ' 天</span>'
    : (run.total > 0
      ? '<span class="day-pill">Day ' + run.day + ' / ' + run.total + '</span>'
      : '<span class="day-pill">进行中</span>');

  var tl = DB.user.timeline.map(function (t, i) {
    return '<div class="tl-item' + (t.isNew ? ' new' : '') + '">' +
      '<div class="tl-date">' + esc(t.date) + '</div>' +
      '<div class="tl-txt">' + t.txt + '</div>' +
      (t.quote ? '<div class="tl-quote">' + esc(t.quote) + '</div>' : '') + '</div>';
  }).join('');

  return '<div class="view">' +
    '<div class="page-head"><h1>⚡ 进行中 · Skill 运行时</h1>' +
    '<p>Skill 不是被读的，是被执行的——判断点自动触发，边界持续监控，产出物流着学长经验的血</p></div>' +
    '<div class="run-select">' + tabsHtml + '</div>' +
    '<div class="grid-2">' +
    '<div class="card">' +
    '<div style="display:flex;justify-content:space-between;gap:10px;flex-wrap:wrap;align-items:flex-start">' +
    '<h2 style="font-size:17px;font-weight:900">🃏 ' + esc(s.title) + '</h2>' + dayPill + '</div>' +
    '<div class="n-note" style="margin:4px 0 10px">创建者：' + esc(s.creator && s.creator.name ? s.creator.name : '学长') +
    (s.creator && s.creator.meta ? ' · ' + esc(s.creator.meta) : '') +
    ' · <span data-goskill="' + s.id + '" style="color:var(--violet);cursor:pointer;font-weight:800">查看原卡 →</span></div>' +
    '<div>' + checksHtml + '</div>' +
    '<div class="agent-feed" id="agent-feed">' + feedHtml + '</div>' +
    '<div class="chat-row"><input id="run-input" placeholder="和陪跑 Agent 说点什么…（试试：帮我写话术 / 想放弃了 / 我这周只剩2小时）">' +
    '<button class="btn-main btn-sm" id="run-send">发送</button></div>' +
    (run.finished
      ? '<div class="actions"><span class="badge green">✅ 已完成 · verdict 已回流</span></div>'
      : (run.persistent ? '' :
        '<div class="actions"><button class="btn-main" id="finish-btn">✅ 完成收尾（两问 + verdict）</button></div>')) +
    '</div>' +
    '<div class="card">' +
    '<h2 style="font-size:17px;font-weight:900;display:flex;gap:10px;align-items:center;flex-wrap:wrap">🗺️ 我的探索地图 ' +
    '<span class="noscore">本页没有分数——这是设计，不是缺陷</span></h2>' +
    '<div class="tl">' + tl + '</div>' +
    '<div class="narrative"><div class="n-t">AI 叙事（只描述行为，只引用本人原话）</div>' + esc(DB.user.narrative) + '</div>' +
    '</div></div></div>';
}
function bindRun(root) {
  bindSkillLinks(root);
  $all('[data-nav-to]', root).forEach(function (b) {
    b.addEventListener('click', function () { location.hash = b.getAttribute('data-nav-to'); });
  });
  $all('[data-run]', root).forEach(function (b) {
    b.addEventListener('click', function () {
      activeRunId = b.getAttribute('data-run');
      persistRuns();
      render();
    });
  });
  var run = State.runs.filter(function (r) { return r.id === activeRunId; })[0];
  if (!run) return;
  var s = DB.skills[run.skillId];

  /* 手动勾选（新装载的 run 可以打勾） */
  $all('[data-check]', root).forEach(function (el) {
    el.addEventListener('click', function () {
      var i = +el.getAttribute('data-check');
      var c = run.checks[i];
      if (c.diff) return;
      c.done = !c.done;
      persistRuns();
      render();
    });
  });

  /* Agent 对话（AI 接入点：WowAPI.runChat） */
  function sendChat(text) {
    var inp = $('#run-input', root);
    var v = (text != null ? String(text) : (inp ? inp.value.trim() : ''));
    if (!v) return;
    if (inp) inp.value = '';
    run.feed.push({ t: '🙋 ' + DB.user.name, c: v, user: true });
    render();
    WowAPI.runChat(v, { skillId: run.skillId, day: run.day, runId: run.id, execId: run.execId }).then(function (res) {
      if (res.execId) run.execId = res.execId;
      persistRuns();
      run.feed.push({
        t: res.boundaryHit ? '🚫 关键判断点（等你的答案）' : '🤖 陪跑 Agent（已注入 ' + s.creator.name + ' 的经验包）',
        c: res.reply, warn: res.boundaryHit,
        pendingDecision: res.pendingDecision || false,
        options: res.options || []
      });
      render();
      var feed = $('#agent-feed'); if (feed) feed.scrollTop = feed.scrollHeight;
      if (res.boundaryHit) toast('判断点接住你了 · 你的选择会被记录为可溯源的判断');
    }).catch(function (err) {
      if (err && err.needLogin) {
        toast('请先登录，才能走真实 AI 对话');
        setTimeout(function () { location.hash = '#/login'; }, 800);
      } else {
        toast((err && err.message) ? ('对话失败：' + err.message) : '对话失败，请重试');
      }
    });
  }

  /* 判断点选项按钮：点击即作为用户选择发送，走同一条对话通道 */
  $all('[data-dec-opt]', root).forEach(function (b) {
    b.addEventListener('click', function () { sendChat(b.getAttribute('data-dec-opt')); });
  });
  var sendBtn = $('#run-send', root);
  if (sendBtn) {
    sendBtn.addEventListener('click', function () { sendChat(); });
    $('#run-input', root).addEventListener('keydown', function (e) { if (e.key === 'Enter') sendChat(); });
  }

  /* 完成打卡 → 两问 + verdict → 变体检测 */
  var fin = $('#finish-btn', root);
  if (fin) fin.addEventListener('click', function () { openFinishFlow(run); });
}

function openFinishFlow(run) {
  var s = DB.skills[run.skillId];
  openModal(
    '<span class="card-kind">收尾只有两个问题 + 一句 verdict</span>' +
    '<h2 style="font-size:19px;font-weight:900">「' + esc(s.title) + '」结束了，不打分</h2>' +
    '<div id="f-body"><div class="n-note">正在按这个 Skill 的原始文档生成你的专属问题…（AI 接入点：WowAPI.genClosingQuestions）</div></div>' +
    '<div id="f-variant"></div>'
  );
  WowAPI.genClosingQuestions(run.execId, s).then(function (q) {
    if (q && q.execId) run.execId = q.execId;
    var body = $('#f-body');
    if (!body) return; // 用户已关掉弹窗
    var q1 = (q && q.q1) ? q.q1 : { question: '哪个时刻你忘了时间？', options: ['做这件事的某个瞬间', '和人交流的某个瞬间'] };
    var q2 = (q && q.q2) ? q.q2 : { question: '哪个时刻你想逃？', options: ['开始前的犹豫', '没有想逃的时刻'] };
    var q3 = (q && q.q3) ? q.q3 : { question: '这个 Skill 在你的情况下成立吗？', options: ['成立 ✓', '不成立（补一句反例）'] };
    body.innerHTML =
      '<div class="sec"><div class="sec-t">① ' + esc(q1.question) + '</div>' + finishChipRow('fq1', q1.options) + '</div>' +
      '<div class="sec"><div class="sec-t">② ' + esc(q2.question) + '</div>' + finishChipRow('fq2', q2.options) + '</div>' +
      '<div class="sec"><div class="sec-t">③ ' + esc(q3.question) + '</div>' + finishChipRow('fq3', q3.options) + '</div>' +
      '<div id="f-reason" style="display:none"><div class="n-note" style="margin:8px 0 6px">说说你的反例（这个 Skill 在哪一步对你不成立）</div>' +
      '<input id="f-reason-txt" style="width:100%;border:2px solid var(--ink);border-radius:12px;background:var(--cream);font-family:inherit;font-size:13px;padding:10px 12px;box-sizing:border-box;outline:none" placeholder="比如：我根本没有 D3-5 的旁听场景…">' +
      '</div>' +
      '<div id="f-improve" style="display:none"><div class="sec" style="border-top:2px dashed var(--line);margin-top:14px">' +
      '<div class="sec-t">④ 对「' + esc(s.title) + '」的改进建议</div>' +
      '<div class="n-note" style="margin:4px 0 8px">想告诉这个 Skill 的创建者什么？（手动输入，可选）——单独一条不会改版本，同类建议重复出现才会触发版本候选。</div>' +
      '<textarea id="f-improve-txt" style="width:100%;min-height:96px;border:2px solid var(--ink);border-radius:12px;background:var(--cream);font-family:inherit;font-size:13px;line-height:1.8;padding:10px 12px;resize:vertical;outline:none" placeholder="比如：某一步的说法很模糊 / 缺一个模板 / 边界可以更严…"></textarea>' +
      '<div class="actions"><button class="btn-main" id="f-submit">📮 提交收尾</button></div></div></div>';
    var picked = 0;
    ['fq1', 'fq2', 'fq3'].forEach(function (qid) {
      bindFinishChips(qid, function () {
        if (!$('#' + qid).dataset.done) {
          $('#' + qid).dataset.done = '1';
          picked++;
          if (picked === 3) {
            var imp = $('#f-improve');
            if (imp) imp.style.display = '';
          }
        }
      });
    });
    // ③ 选中「不成立（补一句反例）」时展开反例输入框
    bindFinishChips('fq3', function () {
      var v = $('#fq3') ? $('#fq3').dataset.v : '';
      var reason = $('#f-reason');
      if (reason) reason.style.display = (v.indexOf('不成立') >= 0) ? '' : 'none';
    });
    var sb = $('#f-submit');
    if (sb) sb.addEventListener('click', function () {
      sb.disabled = true; sb.textContent = '正在记录…';
      WowAPI.submitClosing(run.execId, {
        q1: $('#fq1') ? $('#fq1').dataset.v : '',
        q2: $('#fq2') ? $('#fq2').dataset.v : '',
        verdict: $('#fq3') ? $('#fq3').dataset.v : '',
        verdict_reason: $('#f-reason-txt') ? $('#f-reason-txt').value : '',
        improvement: $('#f-improve-txt') ? $('#f-improve-txt').value : ''
      }).then(function () {
        afterVerdict(run);
      }).catch(function (err) {
        sb.disabled = false; sb.textContent = '📮 提交收尾';
        toast((err && err.message) ? ('提交失败：' + err.message) : '提交失败，请重试');
      });
    });
  });
}
function finishChipRow(id, opts) {
  return '<div class="hero-chips" id="' + id + '" style="margin:6px 0 0">' +
    opts.map(function (o) { return '<button class="chip-btn" data-v="' + esc(o) + '">' + esc(o) + '</button>'; }).join('') + '</div>';
}
function bindFinishChips(qid, onPick) {
  var box = $('#' + qid);
  if (!box) return;
  $all('.chip-btn', box).forEach(function (b) {
    b.addEventListener('click', function () {
      $all('.chip-btn', box).forEach(function (x) { x.style.background = ''; x.style.opacity = '.5'; });
      b.style.background = 'var(--violet-soft)'; b.style.opacity = '1';
      box.dataset.v = b.getAttribute('data-v');
      if (onPick) onPick();
    });
  });
}
function afterVerdict(run) {
  var v = $('#f-variant');
  v.innerHTML = '<div class="n-note">正在对比你的实际轨迹与原剧本…（AI 接入点：WowAPI.detectVariant）</div>';
  WowAPI.detectVariant(run.id).then(function (res) {
    if (!res.hasVariant) {
      v.innerHTML = '<div class="sec" style="border-top:2px dashed var(--line);padding-top:14px">' +
        '<div class="n-note">你的轨迹与原剧本一致。verdict 与两问已回流到原卡的验证数。</div>' +
        '<div class="actions"><button class="btn-main" id="v-done">完成</button></div></div>';
      $('#v-done').addEventListener('click', function () {
        finishRun(run, false); closeModal();
      });
      return;
    }
    v.innerHTML = '<div class="sec" style="border-top:2px dashed var(--line);padding-top:14px">' +
      '<span class="card-kind" style="background:var(--green-soft)">🌱 被动供给 · AI 检测到你走出了一条变体</span>' +
      '<h2 style="font-size:16px;font-weight:900">你的走法和原卡不一样，效果不错——要不要留给下一个人？</h2>' +
      '<div class="diff-view">' +
      '<div class="diff-line old"><span class="m">−</span>' + esc(res.diff.old) + '</div>' +
      '<div class="diff-line new"><span class="m">+</span>' + esc(res.diff.new) + '</div></div>' +
      '<div class="n-note">' + esc(res.note) + '</div>' +
      '<div class="actions"><button class="btn-main" id="v-pub">📮 确认发布变体「' + esc(res.draftTitle) + '」</button>' +
      '<button class="btn-ghost" id="v-skip">这次不了</button></div></div>';
    $('#v-pub').addEventListener('click', function () {
      finishRun(run, true); closeModal();
      State.poolCount++; State.coins += 10;
      DB.user.ledger.unshift({ date: '刚刚', item: '发布变体「' + res.draftTitle + '」· 首次装载奖励', delta: 10 });
      syncTopbar(); confetti();
      toast('🎉 你的走法已经可以被下一个人装载 · 供给池 ' + (State.poolCount - 1) + ' → ' + State.poolCount, true);
    });
    $('#v-skip').addEventListener('click', function () {
      finishRun(run, false); closeModal();
      toast('已记录。verdict 与两问已回流到原卡（9/11 → 10/12）');
    });
  });
}
function finishRun(run, published) {
  run.finished = true;
  var s = DB.skills[run.skillId];
  if (s && s.verify) { s.verify.pass++; s.verify.total++; }
  if (s) {
    DB.user.timeline.push({
      date: '刚刚 · Day ' + (run.total || run.day), isNew: true,
      txt: '完成「' + s.title + '」打卡 · verdict：成立' + (published ? ' · <b>从消费者变成了供给者</b>' : ''),
      quote: ''
    });
  }
  var next = liveRuns()[0];
  activeRunId = next ? next.id : '';
  persistRuns();
  render();
}

/* ============================================================
 * 视图：路口页
 * ============================================================ */
function viewJunction(id) {
  var j = DB.junctions[id];
  if (!j) return '<div class="empty-box">路口不存在</div>';
  var max = Math.max.apply(null, j.branches.map(function (b) { return b.count; }));
  var branchesHtml = j.branches.map(function (b) {
    var quotes = b.quotes.map(function (q) {
      return '<div class="quote">"' + esc(q.t) + '"<div class="q-by">—— ' + esc(q.by) + '</div></div>';
    }).join('');
    return '<div class="branch"><div class="b-head"><h3>' + esc(b.name) + '</h3>' +
      '<div class="count">' + b.count + ' <small>人</small></div></div>' +
      '<div class="bbar" style="width:' + (b.count / max * 100) + '%;background:' + b.color + '"></div>' + quotes + '</div>';
  }).join('');
  var relHtml = (j.relatedSkills || []).map(skillChipHtml).join('');
  var canOrch = /保研|考研|就业|毕业|志愿|高考|出国/.test(j.title);
  var skillSection = '<div class="section-title">决定之前，可以先看清处境</div>' +
    '<p class="n-note" style="margin-top:0">不是建议你选哪边。这些卡是别人用来看清自己的方法；没有卡就去许愿池。</p>' +
    (relHtml ? '<div class="card flat"><div class="t-skills">' + relHtml + '</div></div>' : '') +
    '<div class="actions">' +
    '<a class="btn-main" href="#/match">按刚才那句话找 Skill →</a>' +
    '<a class="btn-ghost" href="#/wishes">没有卡，挂许愿池</a></div>';
  return '<div class="view">' +
    '<div class="crumb"><a href="#/home">首页</a> / 路口</div>' +
    '<div class="junction-head">' +
    '<span class="no-advice">我们不替你选，只让你看见走过的人</span>' +
    '<h2>🛤️ ' + esc(j.title) + '</h2>' +
    '<div class="src">' + esc(j.source) + '</div></div>' +
    '<div class="branches">' + branchesHtml + '</div>' +
    '<div class="switch-note">⚠️ <b>' + esc(j.switchNote) + '</b> 我们把它标出来，不是劝你等，是让你知道大家在哪里重新做了决定。</div>' +
    skillSection +
    '<div class="actions">' +
    (canOrch ? '<a class="btn-ghost" href="#/orch/' + guessOrch(j.title) + '">我已经决定了，帮我排 →</a>' : '') +
    '<a class="btn-ghost" href="#/paths">全部路口</a></div>' +
    rerouteHtml() +
    '<div class="n-note">本页无任何成功率。名额与结果由分配规则决定，给数字就是骗人。</div>' +
    '</div>';
}

/* ============================================================
 * 视图：发布端（学长录入）· 双通道沉淀
 *   通道 A 多轮访谈：AI 问、你说 → sedimentChat 逐轮推进，
 *           收尾 sedimentFinish 提取四槽 backfill，无来源即丢弃
 *   通道 B 上传 Skill 包：sedimentUpload，每次上传跑 LLM 四维评测
 *           （可检索性 / 文件完备性 / 格式完整性 / 边界控制，边界为硬门槛）
 * ============================================================ */
var pubState = {
  mode: 'chat',          // chat | upload
  msgs: [],              // [{role:'user'|'bot', text}]
  progress: 0,
  extracted: [],         // 逐轮累计的决策槽 [{key,title,content,source}]
  versionId: null,
  skillId: null,
  chatBusy: false
};
function viewPublish() {
  return '<div class="view">' +
    '<div class="page-head"><h1>🎙️ 我要沉淀 · 10 分钟，只确认，不撰写</h1>' +
    '<p>两种方式把经验变成卡：<b>多轮访谈</b>（AI 逐题问、你讲，关键判断必须对得上原话，对不上就丢弃）或 <b>上传 Skill 包</b>（每次上传都跑 LLM 四维评测，边界是硬门槛）</p></div>' +
    '<div class="pub-tabs">' +
    '<button class="pub-tab active" data-pubmode="chat">🎙️ 多轮访谈</button>' +
    '<button class="pub-tab" data-pubmode="upload">📦 上传 Skill 包</button></div>' +
    '<div id="pub-pane-chat" class="pub-pane">' + publishChatPane() + '</div>' +
    '<div id="pub-pane-upload" class="pub-pane" hidden>' + publishUploadPane() + '</div>' +
    '</div>';
}

/* 通道 A：访谈面板（消息流 + 进度 + 已抽槽位 + 完成按钮） */
function publishChatPane() {
  var progress = pubState.progress;
  return '<div class="card speech">' +
    '<div class="sed-progress"><div class="sed-progress-label">沉淀进度 <b>' + progress + '%</b> · 每次回复末尾自动抽取槽位</div>' +
    '<div class="sed-bar"><div class="sed-bar-fill" id="sed-bar" style="width:' + progress + '%"></div></div></div>' +
    '<div class="sed-thread" id="sed-thread"></div>' +
    '<div class="chat-row"><input id="sed-input" placeholder="讲讲你做成过的那件事…（例：大三保研，我差点放弃但扛下来了）"><button class="btn-main btn-sm" id="btn-sed-send">发送</button></div>' +
    '<div class="n-note">AI 一次只问 1–2 题，你只管说。说完点「完成沉淀」，关键判断必须能在你的原话里找到依据，找不到就丢弃。</div>' +
    '<div class="slots" id="sed-slots"></div>' +
    '<div class="actions"><button class="btn-main" id="btn-sed-finish">🤖 完成沉淀（AI 提取四槽成草稿）</button></div>' +
    '<p class="n-note" id="sed-done" style="display:none">✅ 草稿已生成，口述全文保留为「TA 的完整故事」。可去工作台继续编辑定价、走发布门禁。</p>' +
    '</div>';
}

/* 通道 B：上传面板（zip + 元数据 + 四维评测卡） */
function publishUploadPane() {
  return '<div class="grid-2">' +
    '<div class="card speech">' +
    '<h3 style="font-size:15px;font-weight:900;margin-bottom:8px">📦 Skill 包（.zip，内含 SKILL.md）</h3>' +
    '<label>Skill 名称（要独特、可检索）<input id="sed-name" placeholder="例：给实验室发旁听邮件（附原信）"></label>' +
    '<label>一句话描述（帮谁解决什么问题、产出什么）<textarea id="sed-desc" rows="2" placeholder="例：帮大二学生用一封邮件进组旁听，产出可直接改写的邮件模板"></textarea></label>' +
    '<label>标签（逗号分隔，覆盖用户会搜的词）<input id="sed-tags" placeholder="例：实验室,旁听,邮件,保研"></label>' +
    '<input type="file" id="sed-file" accept=".zip" class="sed-file">' +
    '<div class="actions"><button class="btn-main" id="btn-sed-upload">📦 上传并评测（LLM 四维）</button></div>' +
    '<div class="n-note">每次上传都会跑一遍 LLM 评测：<b>可检索性 / 文件完备性 / 格式完整性 / 边界控制</b>。边界（不适用条件 + 交回给人触发点）是硬门槛，不过整体 fail。</div>' +
    '</div>' +
    '<div id="sed-eval"><div class="card flat"><div class="sec-t">评测结果（上传后显示）</div>' +
    '<div class="n-note">四维各自打分 pass/fail；边界 fail 则整体 fail，无论其他维度多好。</div></div></div>' +
    '</div>';
}

/* 渲染四维评测卡（labels 由后端返回，边界维度标硬门槛） */
function renderEvalCard(root, ev) {
  ev = ev || {};
  var dims = ev.dimensions || [];
  var labels = ev.labels || ['可检索性', '文件完备性', '格式完整性', '边界控制'];
  var hard = ev.hard_gate || 'boundary';
  var rows = dims.map(function (d, i) {
    var pass = d.verdict === 'pass';
    var hardTag = d.key === hard ? ' <span class="eval-hard">硬门槛</span>' : '';
    var issues = (d.issues || []).length
      ? '<ul class="eval-issues">' + d.issues.map(function (x) { return '<li>' + esc(x) + '</li>'; }).join('') + '</ul>'
      : '<div class="eval-none">未发现问题</div>';
    return '<div class="eval-dim ' + (pass ? 'ok' : 'bad') + '">' +
      '<div class="eval-dim-h"><b>' + esc(labels[i] || d.key) + '</b>' + hardTag +
      '<span class="eval-v">' + (pass ? '✅ 通过' : '❌ 未过') + '</span>' +
      '<span class="eval-score">' + Math.round((d.score || 0) * 100) + '/100</span></div>' +
      issues +
      (d.suggestion && d.suggestion !== '无' ? '<div class="eval-sug">💡 ' + esc(d.suggestion) + '</div>' : '') +
      '</div>';
  }).join('');
  var pass = ev.overall_verdict === 'pass';
  var box = '<div class="eval-summary ' + (pass ? 'ok' : 'bad') + '">' +
    '<b>' + (pass ? '✅ 总体通过 · 进入发布门禁' : '❌ 总体未过 · 按 issues 修复后重新上传') + '</b>' +
    '<span>（' + Math.round((ev.overall_score || 0) * 100) + '/100' + (ev.degraded ? ' · 降级评测' : '') + '）</span></div>';
  $('#sed-eval', root).innerHTML = '<div class="eval-card">' + box + rows +
    (ev.summary ? '<div class="eval-note">' + esc(ev.summary) + '</div>' : '') + '</div>';
}

/* 消息流渲染 */
function paintSedThread(root, thinking) {
  var thread = $('#sed-thread', root);
  var msgs = pubState.msgs.slice();
  if (thinking) msgs.push({ role: 'bot', thinking: true });
  thread.innerHTML = msgs.map(function (m) {
    if (m.thinking) return '<div class="msg bot"><div class="bubble thinking">在听…</div></div>';
    if (m.role === 'user') return '<div class="msg me"><div class="bubble">' + nl2br(m.text) + '</div></div>';
    return '<div class="msg bot"><div class="bubble">' + nl2br(m.text) + '</div></div>';
  }).join('');
  thread.scrollTop = thread.scrollHeight;
  var bar = $('#sed-bar', root);
  if (bar) bar.style.width = pubState.progress + '%';
  var label = $('.sed-progress-label b', root);
  if (label) label.textContent = pubState.progress + '%';
}

/* 槽位渲染：pubState.extracted（逐轮累计）或收尾返回的 slots */
function paintSedSlots(root, list, note) {
  var wrap = $('#sed-slots', root);
  if (!list || !list.length) {
    wrap.innerHTML = '<div class="slot" style="opacity:1"><div class="s-c">' +
      (note ? esc(note) : '正在逐轮抽取…AI 会从你的话里挖关键判断（触发信号 → 判断 → 适用范围）') + '</div></div>';
    return;
  }
  wrap.innerHTML = list.map(function (s) {
    return '<div class="slot filled"><div class="s-t">' + esc(s.title || s.key || '') + '</div>' +
      '<div class="s-c">' + nl2br(s.content || '') +
      (s.source ? '<br><span style="font-size:11px;color:var(--ink-3)">来源：<span class="src-mark">' + esc(s.source) + '</span></span>' : '') +
      '</div></div>';
  }).join('');
}

/* 逐轮累计：把后端每轮【抽取】的 decisions 并入已有槽位（按 触发信号+判断 去重） */
function mergeExtracted(existing, ext) {
  if (!ext) return existing;
  var list = existing.slice();
  var seen = {};
  list.forEach(function (s) { if (s.key) seen[s.key] = true; });
  var decisions = ext.decisions || [];
  decisions.forEach(function (d) {
    var key = d.slot || d.key || '';
    if (seen[key]) return;
    if (!d.trigger_signal && !d.judgment) return;
    seen[key] = true;
    list.push({
      key: key,
      title: d.slot || key,
      content: (d.trigger_signal || '') + ' → ' + (d.judgment || '') + '（' + (d.scope || '') + '）',
      source: '口述原话'
    });
  });
  return list;
}

function bindPublish(root) {
  pubState = { mode: 'chat', msgs: [], progress: 0, extracted: [], versionId: null, skillId: null, chatBusy: false };
  paintSedThread(root, false);
  paintSedSlots(root, null, 'AI 将逐题引导：先讲那件事 → 做成什么样 → 哪一步差点放弃 → 适合谁 / 不适用条件。');

  /* 双通道 Tab */
  $all('.pub-tab', root).forEach(function (t) {
    t.addEventListener('click', function () {
      $all('.pub-tab', root).forEach(function (x) { x.classList.remove('active'); });
      t.classList.add('active');
      pubState.mode = t.getAttribute('data-pubmode');
      $('#pub-pane-chat', root).hidden = pubState.mode !== 'chat';
      $('#pub-pane-upload', root).hidden = pubState.mode !== 'upload';
    });
  });

  /* 通道 A：发送 */
  function send() {
    var input = $('#sed-input', root);
    var text = (input.value || '').trim();
    if (!text || pubState.chatBusy) return;
    input.value = '';
    pubState.msgs.push({ role: 'user', text: text });
    pubState.chatBusy = true;
    $('#btn-sed-send', root).disabled = true;
    paintSedThread(root, true);
    WowAPI.sedimentChat(pubState.msgs).then(function (res) {
      pubState.msgs.push({ role: 'bot', text: res.reply || '' });
      pubState.progress = Math.max(pubState.progress, res.progress || 0);
      pubState.extracted = mergeExtracted(pubState.extracted, res.extracted);
      paintSedThread(root, false);
      paintSedSlots(root, pubState.extracted);
      if (res.degraded) toast('AI 暂不可用，当前为本地兜底引导');
    }).catch(function (err) {
      if (err && err.needLogin) { location.hash = '#/login'; return; }
      toast(err.message || '发送失败');
      paintSedThread(root, false);
    }).then(function () {
      pubState.chatBusy = false;
      var b = $('#btn-sed-send', root);
      if (b) b.disabled = false;
    });
  }
  $('#btn-sed-send', root).addEventListener('click', send);
  $('#sed-input', root).addEventListener('keydown', function (e) {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send(); }
  });

  /* 通道 A：完成沉淀 → sedimentFinish（LLM 提取 + 无来源即丢弃 + runBackfill 落库） */
  $('#btn-sed-finish', root).addEventListener('click', function () {
    var btn = this;
    if (pubState.chatBusy) return toast('等 AI 回完这一轮再收尾');
    btn.disabled = true; btn.textContent = '🤖 正在整理成草稿…（每个判断都在比对你的原话）';
    WowAPI.sedimentFinish(pubState.msgs).then(function (res) {
      pubState.versionId = res.versionId;
      pubState.skillId = res.skillId;
      paintSedSlots(root, res.slots || []);
      btn.textContent = '✅ 草稿已生成';
      var note = $('#sed-done', root);
      if (note) {
        note.style.display = 'block';
        note.textContent = '✅ 草稿已生成（' + (res.versionId || '') + '），关键判断全部可溯源。可去工作台继续编辑定价、走发布门禁。';
      }
      if (res.note) toast(res.note);
      toast('🎉 草稿已生成 · 口述全文保留为「TA 的完整故事」', true);
    }).catch(function (err) {
      if (err && err.needLogin) { location.hash = '#/login'; return; }
      btn.disabled = false; btn.textContent = '🤖 完成沉淀（AI 提取四槽成草稿）';
      toast(err.message || '整理失败');
    });
  });

  /* 通道 B：上传 + LLM 四维评测 */
  $('#btn-sed-upload', root).addEventListener('click', function () {
    var btn = this;
    var file = $('#sed-file', root).files[0];
    var name = $('#sed-name', root).value.trim();
    var desc = $('#sed-desc', root).value.trim();
    var tags = $('#sed-tags', root).value.trim();
    if (!file) return toast('请先选择 .zip 的 Skill 包');
    if (!name) return toast('请填写 Skill 名称');
    if (!/\.zip$/i.test(file.name)) return toast('只接受 .zip 格式的 Skill 包');
    var fd = new FormData();
    fd.append('archive', file);
    fd.append('name', name);
    if (desc) fd.append('description', desc);
    if (tags) fd.append('tags', JSON.stringify(tags.split(/[,，、\s]+/).filter(Boolean)));
    btn.disabled = true; btn.textContent = '🔍 上传并评测中（LLM 四维）…';
    $('#sed-eval', root).innerHTML = '<div class="card flat"><div class="sec-t">评测中</div>' +
      '<div class="n-note">正在扫描包内文件 + 跑 LLM 四维评测：可检索性 / 文件完备性 / 格式完整性 / 边界控制…</div></div>';
    WowAPI.sedimentUpload(fd).then(function (r) {
      pubState.skillId = r.skill_id;
      renderEvalCard(root, r.eval || {});
      btn.disabled = false; btn.textContent = '📦 上传并评测（LLM 四维）';
      toast('评测完成', true);
    }).catch(function (err) {
      if (err && err.needLogin) { location.hash = '#/login'; return; }
      btn.disabled = false; btn.textContent = '📦 上传并评测（LLM 四维）';
      $('#sed-eval', root).innerHTML = '<div class="card flat"><div class="sec-t">上传失败</div>' +
        '<div class="n-note">' + esc(err.message || '请检查 zip 是否包含 SKILL.md') + '</div></div>';
      toast(err.message || '上传失败');
    });
  });
}

/* ============================================================
 * 视图：登录 / 注册（复用 skillhub JWT + AI 问卷）
 * ============================================================ */
var authMode = 'login';
var quizState = {
  heard_of_llm: null, used_llm: null, used_agent: null,
  has_agent_installed: null, ran_full_project: null
};
var QUIZ_ITEMS = [
  { key: 'heard_of_llm', text: '你是否听说过 ChatGPT 这类大语言模型？' },
  { key: 'used_llm', text: '你是否实际用过 ChatGPT / DeepSeek / Claude？' },
  { key: 'used_agent', text: '你是否用过 Cursor / Codex 这类 AI 编码助手？' },
  { key: 'has_agent_installed', text: '你的电脑上是否装过上述 AI 助手？' },
  { key: 'ran_full_project', text: '你是否用 AI 助手完整跑通过一个项目（不止问答）？' }
];

function viewAuth() {
  var loginPane = authMode === 'login';
  var tabs = '<div class="auth-tabs">' +
    '<button class="auth-tab' + (loginPane ? ' active' : '') + '" data-amode="login">登录</button>' +
    '<button class="auth-tab' + (!loginPane ? ' active' : '') + '" data-amode="register">注册</button></div>';

  var loginHtml = '<form id="login-form" class="auth-form">' +
    '<label>用户名或邮箱<input name="account" required placeholder="demo_user" autocomplete="username"></label>' +
    '<label>密码<input name="password" type="password" required minlength="6" placeholder="至少 6 位" autocomplete="current-password"></label>' +
    '<button class="btn-main" type="submit">进入 WowSkillLand</button>' +
    '<p class="auth-hint">这是 skillhub 上一版的登录系统，JWT 直接复用，账号打通成长路由和工作台。</p></form>';

  var quizHtml = QUIZ_ITEMS.map(function (q, i) {
    var v = quizState[q.key];
    return '<div class="quiz-item"><div class="q-t">' + (i + 1) + '. ' + esc(q.text) + '</div>' +
      '<div class="q-btns">' +
      '<button type="button" class="q-btn' + (v === true ? ' on' : '') + '" data-qk="' + q.key + '" data-qv="1">是</button>' +
      '<button type="button" class="q-btn' + (v === false ? ' on' : '') + '" data-qk="' + q.key + '" data-qv="0">否</button>' +
      '</div></div>';
  }).join('');

  var regHtml = '<form id="reg-form" class="auth-form">' +
    '<div class="auth-grid">' +
    '<label>用户名<input name="username" required minlength="2" placeholder="至少 2 个字"></label>' +
    '<label>密码<input name="password" type="password" required minlength="6" placeholder="至少 6 位"></label>' +
    '<label>学校<input name="school" placeholder="例如 某大学"></label>' +
    '<label>年级<select name="grade">' +
      '<option>大一</option><option>大二</option><option selected>大三</option>' +
      '<option>大四</option><option>研一</option><option>高考 / 大0</option></select></label>' +
    '<label>专业<input name="major" placeholder="例如 计算机"></label>' +
    '<label>邮箱（可选）<input name="email" type="email" placeholder="name@school.edu"></label>' +
    '</div>' +
    '<div class="quiz-box"><div class="sec-t">AI 使用经验（5 题，用来个性化解读，不是测评）</div>' + quizHtml + '</div>' +
    '<button class="btn-main" type="submit">注册并进入</button></form>';

  return '<div class="view auth-view">' +
    '<div class="auth-card">' +
    '<div class="card-kind">复用 skillhub 登录</div>' +
    '<h1>先告诉我你是谁</h1>' +
    '<p class="auth-lead">处境字段（年级 / 专业 / 学校）会用来匹配第一步卡；AI 问卷只决定解读深度，不给人打分。</p>' +
    tabs + (loginPane ? loginHtml : regHtml) +
    '<div class="auth-err" id="auth-err" hidden></div></div></div>';
}
function bindAuth(root) {
  $all('[data-amode]', root).forEach(function (b) {
    b.addEventListener('click', function () { authMode = b.getAttribute('data-amode'); render(); });
  });
  $all('[data-qk]', root).forEach(function (b) {
    b.addEventListener('click', function () {
      var key = b.getAttribute('data-qk');
      var val = b.getAttribute('data-qv') === '1';
      quizState[key] = val;
      $all('[data-qk="' + key + '"]', root).forEach(function (x) {
        x.classList.toggle('on', (x.getAttribute('data-qv') === '1') === val);
      });
    });
  });
  function showErr(msg) {
    var el = $('#auth-err', root); el.hidden = false; el.textContent = msg;
  }
  var lf = $('#login-form', root);
  if (lf) lf.addEventListener('submit', function (e) {
    e.preventDefault();
    var fd = new FormData(lf);
    lf.querySelector('button[type=submit]').disabled = true;
    WowAPI.auth.login(fd.get('account').trim(), fd.get('password')).then(function (u) {
      toast('欢迎回来，' + u.username, true);
      location.hash = '#/home';
      render();
    }).catch(function (err) { showErr(err.message || '登录失败'); lf.querySelector('button[type=submit]').disabled = false; });
  });
  var rf = $('#reg-form', root);
  if (rf) rf.addEventListener('submit', function (e) {
    e.preventDefault();
    var unanswered = QUIZ_ITEMS.filter(function (q) { return quizState[q.key] === null; });
    if (unanswered.length) { showErr('请先答完 5 道 AI 经验题（这不是测评，只决定介绍深度）'); return; }
    var fd = new FormData(rf);
    rf.querySelector('button[type=submit]').disabled = true;
    WowAPI.auth.register({
      username: fd.get('username').trim(),
      password: fd.get('password'),
      school: (fd.get('school') || '').trim(),
      grade: fd.get('grade'),
      major: (fd.get('major') || '').trim(),
      email: (fd.get('email') || '').trim(),
      ai_quiz: {
        heard_of_llm: !!quizState.heard_of_llm,
        used_llm: !!quizState.used_llm,
        used_agent: !!quizState.used_agent,
        has_agent_installed: !!quizState.has_agent_installed,
        ran_full_project: !!quizState.ran_full_project
      }
    }).then(function (u) {
      var lv = AI_LEVEL_LABEL[u.ai_level] || u.ai_level || '';
      toast('注册成功' + (lv ? ' · ' + lv : ''), true);
      location.hash = '#/home';
      render();
    }).catch(function (err) { showErr(err.message || '注册失败'); rf.querySelector('button[type=submit]').disabled = false; });
  });
}

/* ============================================================
 * 视图：我的
 * ============================================================ */
var meTab = 'map';
function viewMe() {
  if (!WowAPI.auth.isLoggedIn()) {
    return '<div class="view"><div class="auth-card">' +
      '<h1>探索地图是私人的</h1><p class="auth-lead">登录后，这里只描述你做过的事和你自己的原话——没有分数、没有雷达图。</p>' +
      '<a class="btn-main" href="#/login">登录 / 注册</a></div></div>';
  }
  var bu = WowConfig.USER;
  var u = DB.user;
  var level = AI_LEVEL_LABEL[bu.ai_level] || '';
  var tabs = [['map', '🗺️ 探索地图'], ['runs', '⚡ 我装载的'], ['pub', '🎙️ 我发布的'], ['ledger', '🪙 积分明细']];
  var tabsHtml = tabs.map(function (t) {
    return '<button class="run-tab' + (meTab === t[0] ? ' active' : '') + '" data-metab="' + t[0] + '">' + t[1] + '</button>';
  }).join('');

  var body = '';
  if (meTab === 'map') {
    var tl = u.timeline.map(function (t) {
      return '<div class="tl-item' + (t.isNew ? ' new' : '') + '"><div class="tl-date">' + esc(t.date) + '</div>' +
        '<div class="tl-txt">' + t.txt + '</div>' +
        (t.quote ? '<div class="tl-quote">' + esc(t.quote) + '</div>' : '') + '</div>';
    }).join('');
    body = '<div class="card"><h2 style="font-size:16px;font-weight:900;display:flex;gap:10px;align-items:center;flex-wrap:wrap">我的探索地图 ' +
      '<span class="noscore">没有分数、没有雷达图——这是设计，不是缺陷</span></h2>' +
      '<div class="tl">' + tl + '</div>' +
      '<div class="narrative"><div class="n-t">AI 叙事（只描述行为，只引用本人原话）</div>' + esc(u.narrative) + '</div></div>';
  } else if (meTab === 'runs') {
    body = State.runs.length
      ? '<div class="grid-3">' + State.runs.map(function (r) {
          var s = DB.skills[r.skillId];
          return skillCardHtml(s);
        }).join('') + '</div>'
      : '<div class="empty-box">还没装载过 Skill</div>';
  } else if (meTab === 'pub') {
    var items = State.published.map(function (p) {
      return '<div class="task-row"><div class="t-main"><div class="t-title">🃏 ' + esc(p.title) + '</div>' +
        '<div class="t-desc">' + (p.price ? '定价 🪙' + p.price + ' · 每次兑换你获得 80%' : '免费 · 积累声望与 verdict') + '</div></div>' +
        '<span class="badge green">已上架</span></div>';
    }).join('');
    var variantNote = '<div class="task-row"><div class="t-main"><div class="t-title">🌱 变体「先当记分员再上手」</div>' +
      '<div class="t-desc">谱系：老周 v1.0 → 小林变体 · 已被装载 2 次</div></div><span class="badge amber">变体卡</span></div>';
    body = (items || '') + variantNote +
      '<div class="n-note">供给的三级阶梯：交 verdict（最低成本）→ 确认变体（AI 代写，你只点确认）→ 主动录入（10 分钟口述）。</div>';
  } else {
    var rows = u.ledger.map(function (l) {
      var cls = l.delta > 0 ? 'pos' : (l.delta < 0 ? 'neg' : '');
      var sign = l.delta > 0 ? '+' : '';
      return '<div class="ledger-row"><span>' + esc(l.date) + ' · ' + esc(l.item) + '</span>' +
        '<span class="delta ' + cls + '">' + (l.delta === 0 ? '—' : sign + l.delta) + '</span></div>';
    }).join('');
    body = '<div class="card">' + rows +
      '<div class="n-note">积分是流通凭证：verdict 回流、变体被装载、Skill 被兑换都会产生收入——优秀经验成为可持续的能力资产。</div></div>';
  }

  return '<div class="view">' +
    '<div class="me-head"><div class="me-ava">' + esc((bu.username || '我').slice(0, 1)) + '</div>' +
    '<div><h1 style="font-size:20px;font-weight:900">' + esc(bu.username) + '</h1>' +
    '<p style="font-size:12.5px;color:var(--ink-3);font-weight:700">' +
    esc([bu.school, bu.major, bu.grade].filter(Boolean).join(' · ') || u.major) +
    (level ? ' · ' + esc(level) : '') +
    ' · 当前阶段：' + esc((stageOf(u.stageId) || {}).name || '') + ' · 🪙 ' + State.coins + ' 积分</p>' +
    '<p><a class="btn-ghost btn-sm" href="#/publish">沉淀经验</a></p></div></div>' +
    '<div class="me-tabs">' + tabsHtml + '</div>' + body + '</div>';
}
function bindMe(root) {
  $all('[data-metab]', root).forEach(function (b) {
    b.addEventListener('click', function () { meTab = b.getAttribute('data-metab'); render(); });
  });
  bindSkillLinks(root);
}

/* ============================================================
 * 路由
 * ============================================================ */
function parseHash() {
  var h = location.hash.replace(/^#\/?/, '') || 'home';
  var parts = h.split('/');
  return { page: parts[0] || 'home', param: parts[1] || '' };
}
function render() {
  var r = parseHash();
  var view = $('#view');
  var html = '', bind = null, navKey = r.page;

  switch (r.page) {
    case 'home': html = viewHome(); bind = bindHome; break;
    case 'explore': html = viewExplore(); bind = bindExplore; navKey = 'home'; break;
    case 'match': html = viewMatch(); bind = bindMatch; navKey = 'market'; break;
    case 'paths': html = viewPaths(); bind = bindPaths; navKey = 'paths'; break;
    case 'orch': html = viewOrch(r.param); bind = function (root) { bindOrch(root, r.param); }; navKey = 'paths'; break;
    case 'stage': html = viewStage(r.param); bind = bindStage; break;
    case 'market': html = viewMarket(); bind = bindMarket; break;
    case 'wishes': html = viewWishes(); bind = bindWishes; break;
    case 'wish': html = viewWish(r.param); bind = function (root) { bindWish(root, r.param); }; navKey = 'wishes'; break;
    case 'skill': html = viewSkill(r.param); bind = function (root) { bindSkill(root, r.param); }; navKey = 'market'; break;
    case 'run': html = viewRun(); bind = bindRun; break;
    case 'junction': html = viewJunction(r.param); bind = function (root) { bindSkillLinks(root); bindReroute(root); }; navKey = 'paths'; break;
    case 'publish': html = viewPublish(); bind = bindPublish; break;
    case 'login': html = viewAuth(); bind = bindAuth; navKey = ''; break;
    case 'me': html = viewMe(); bind = bindMe; break;
    default: html = viewHome(); bind = bindHome; navKey = 'home';
  }
  view.innerHTML = html;
  if (bind) bind(view);
  if (r.page === 'stage') navKey = 'home';

  $all('#mainnav a, #tabbar a').forEach(function (a) {
    a.classList.toggle('active', a.getAttribute('data-nav') === navKey);
  });
  syncTopbar();
}

window.addEventListener('hashchange', render);
window.addEventListener('load', function () {
  try {
    if (!location.hash) location.hash = '#/home';
    applyUserToLocal(WowConfig.USER);
    Promise.all([
      WowAPI.auth.ping(),
      WowAPI.auth.restore()
    ]).then(function () {
      render();
    }).catch(function () { render(); });
  } catch (err) {
    console.error(err);
    var f = document.createElement('div');
    f.style.cssText = 'position:fixed;inset:0;display:flex;align-items:center;justify-content:center;background:#faf6ee;color:#22201c;font-size:16px;z-index:999;text-align:center;padding:24px';
    f.textContent = '页面加载出了点小问题，刷新一下试试～';
    document.body.appendChild(f);
  }
});
