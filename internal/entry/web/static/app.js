'use strict';

// ── State ──
const state = { snapshot: null, cc: null, grounding: { enabled: false, subject: '', sourceText: '' } };
const $ = (id) => document.getElementById(id);

function api(path, body) {
  return fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  }).then(async (res) => {
    let data = {};
    try { data = await res.json(); } catch (_) {}
    if (!res.ok) throw new Error(data.error || ('Lỗi máy chủ (' + res.status + ')'));
    return data;
  });
}

function getJSON(path) {
  return fetch(path).then((r) => r.json());
}

let toastTimer = null;
function toast(msg, kind) {
  const t = $('toast');
  t.textContent = msg;
  t.className = 'toast' + (kind ? ' ' + kind : '');
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { t.hidden = true; }, 4000);
}

function escapeHTML(s) {
  return String(s == null ? '' : s).replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));
}

function fmtTime(ms) {
  if (!ms) return '--:--:--';
  const d = new Date(ms);
  return d.toLocaleTimeString('vi-VN', { hour12: false });
}

function nearBottom(el) { return el.scrollHeight - el.scrollTop - el.clientHeight < 60; }
function scrollDown(el) { el.scrollTop = el.scrollHeight; }

// ── SSE ──
function connect() {
  const es = new EventSource('/api/stream');
  es.onmessage = (e) => {
    let msg;
    try { msg = JSON.parse(e.data); } catch (_) { return; }
    handleMessage(msg);
  };
  es.onerror = () => { /* trình duyệt tự kết nối lại */ };
}

function handleMessage(msg) {
  switch (msg.type) {
    case 'snapshot': applySnapshot(msg.data); break;
    case 'event': applyEvent(msg.data); break;
    case 'stream': applyStream(msg.data); break;
    case 'done': toast('Phiên sáng tác đã dừng.'); break;
    case 'ask_user': showAskUser(msg.data); break;
    case 'cocreate': onCoCreate(msg.data); break;
    case 'progress': onProgress(msg.data); break;
    default: break;
  }
}

// ── Snapshot render ──
const STATUS_VI = {
  COMPLETE: 'HOÀN THÀNH', REVIEW: 'ĐÁNH GIÁ', REWRITE: 'VIẾT LẠI',
  RUNNING: 'ĐANG VIẾT', READY: 'SẴN SÀNG',
};

function isFresh(s) {
  if (!s) return true;
  const p = s.Phase || '';
  return (p === '' || p === 'init') && (s.CompletedCount || 0) === 0 && !s.RecoveryLabel;
}

function applySnapshot(s) {
  state.snapshot = s;
  $('novelName').textContent = s.NovelName || 'Chưa có tác phẩm';

  const badge = $('statusBadge');
  badge.textContent = STATUS_VI[s.StatusLabel] || s.StatusLabel || 'SẴN SÀNG';
  badge.className = 'status-badge ' + statusClass(s);

  $('modelValue').textContent = (s.ModelName || '—') + (s.ThinkingLevel ? ' · ' + s.ThinkingLevel : '');
  $('modelMeta').title = 'Provider: ' + (s.Provider || '—');
  let cost = '$' + (s.TotalCostUSD || 0).toFixed(2);
  if (s.BudgetLimitUSD > 0) cost += ' / $' + s.BudgetLimitUSD.toFixed(0);
  $('costValue').textContent = cost;

  const pct = Math.round((s.ContextPercent || 0) * 100) / 100;
  const fill = $('ctxFill');
  fill.style.width = Math.min(100, pct) + '%';
  fill.style.background = pct > 85 ? 'var(--err)' : (pct > 70 ? 'var(--warn)' : 'var(--ok)');
  $('ctxValue').textContent = pct ? pct.toFixed(0) + '%' : '0%';

  $('pgChapter').textContent = s.CurrentChapter || 0;
  $('pgDone').textContent = s.CompletedCount || 0;
  $('pgTotal').textContent = s.TotalChapters || 0;
  $('pgWords').textContent = (s.TotalWordCount || 0).toLocaleString('vi-VN');

  const vol = $('volumeLine');
  if (s.CurrentVolumeArc) { vol.hidden = false; vol.textContent = '📖 ' + s.CurrentVolumeArc + (s.NextVolumeTitle ? ' → ' + s.NextVolumeTitle : ''); }
  else vol.hidden = true;
  const rec = $('recoveryLine');
  if (s.RecoveryLabel && !s.IsRunning) { rec.hidden = false; rec.textContent = '⟲ Có thể khôi phục: ' + s.RecoveryLabel; }
  else rec.hidden = true;

  renderAgents(s.Agents || []);
  renderOutline(s.Outline || []);
  renderCharacters(s);
  renderDetail(s);
  updatePrimary(s);
}

function statusClass(s) {
  if (s.Phase === 'complete') return 'complete';
  if (s.RuntimeState === 'paused') return 'paused';
  if (s.Flow === 'reviewing') return 'review';
  if (s.Flow === 'rewriting' || s.Flow === 'polishing') return 'rewrite';
  if (s.IsRunning) return 'running';
  return '';
}

const AGENT_VI = { coordinator: 'Điều phối', architect: 'Kiến trúc', writer: 'Người viết', editor: 'Biên tập' };
function renderAgents(agents) {
  const box = $('agents');
  if (!agents.length) { box.innerHTML = '<div class="agent-sub" style="color:var(--dim)">Chưa có hoạt động.</div>'; return; }
  box.innerHTML = agents.map((a) => {
    const active = a.State && a.State !== 'idle' && a.State !== '';
    const name = AGENT_VI[(a.Name || '').toLowerCase()] || a.Name || '?';
    const sub = a.Summary || a.Tool || a.TaskKind || '';
    return `<div class="agent ${active ? 'active' : ''}">
      <div class="agent-head"><span class="agent-name">${escapeHTML(name)}</span><span class="agent-state">${escapeHTML(a.State || 'idle')}</span></div>
      ${sub ? `<div class="agent-sub" title="${escapeHTML(sub)}">${escapeHTML(sub)}</div>` : ''}
    </div>`;
  }).join('');
}

function renderOutline(outline) {
  const pane = $('pane-outline');
  if (!outline.length) { pane.innerHTML = '<div style="color:var(--dim)">Chưa có đại cương.</div>'; return; }
  pane.innerHTML = outline.map((o) =>
    `<div class="ol-item"><span class="ol-ch">Chương ${o.Chapter}</span> <span class="ol-title">${escapeHTML(o.Title || '')}</span>
     ${o.CoreEvent ? `<div class="ol-core">${escapeHTML(o.CoreEvent)}</div>` : ''}</div>`
  ).join('');
}

function renderCharacters(s) {
  const pane = $('pane-characters');
  const chars = s.Characters || [];
  let html = '';
  if (chars.length) html += chars.map((c) => `<div class="char-item">${escapeHTML(c)}</div>`).join('');
  else html += '<div style="color:var(--dim)">Chưa có nhân vật.</div>';
  if (s.RecentSupporting && s.RecentSupporting.length) {
    html += `<div class="detail-row" style="margin-top:12px"><div class="dk">Phụ gần đây (${s.SupportingCount || 0})</div>
      <div class="dv">${s.RecentSupporting.map(escapeHTML).join(', ')}</div></div>`;
  }
  pane.innerHTML = html;
}

function renderDetail(s) {
  const rows = [];
  const add = (k, v) => { if (v) rows.push(`<div class="detail-row"><div class="dk">${k}</div><div class="dv">${escapeHTML(v)}</div></div>`); };
  add('Phase / Flow', (s.Phase || '-') + (s.Flow ? ' / ' + s.Flow : ''));
  add('Tiền đề', s.Premise);
  add('Định hướng kết (Compass)', s.CompassDirection);
  add('Quy mô ước tính', s.CompassScale);
  add('Commit gần nhất', s.LastCommitSummary);
  add('Đánh giá gần nhất', s.LastReviewSummary);
  add('Checkpoint', s.LastCheckpointName);
  if (s.PendingRewrites && s.PendingRewrites.length) add('Chờ viết lại', s.PendingRewrites.join(', ') + (s.RewriteReason ? ' — ' + s.RewriteReason : ''));
  if (s.PendingSteer) add('Can thiệp đang chờ', s.PendingSteer);
  if (s.RecentSummaries && s.RecentSummaries.length) add('Tóm tắt gần đây', s.RecentSummaries.join(' | '));
  $('pane-detail').innerHTML = rows.join('') || '<div style="color:var(--dim)">Chưa có chi tiết.</div>';
}

function updatePrimary(s) {
  const input = $('mainInput');
  const btn = $('btnPrimary');
  $('btnAbort').hidden = !s.IsRunning;
  if (s.IsRunning) {
    btn.textContent = 'Can thiệp';
    input.placeholder = 'Nhập ý kiến can thiệp (vd: thêm một nhân vật phản diện)…';
  } else if (s.Phase === 'complete') {
    btn.textContent = 'Đã hoàn thành';
    input.placeholder = 'Tác phẩm đã hoàn thành. Có thể nhập để tiếp tục mở rộng…';
  } else if (isFresh(s)) {
    btn.textContent = 'Bắt đầu';
    input.placeholder = 'Nhập một câu yêu cầu sáng tác rồi nhấn Bắt đầu…';
  } else {
    btn.textContent = input.value.trim() ? 'Tiếp tục' : 'Khôi phục';
    input.placeholder = 'Nhập để tiếp tục, hoặc để trống rồi nhấn Khôi phục…';
  }
}

// ── Events ──
const evMap = new Map();
let evCounter = 0;
function makeEventNode(dto) {
  const node = document.createElement('div');
  node.className = eventClass(dto);
  node.innerHTML = `<span class="ev-time">${fmtTime(dto.time)}</span><span class="ev-cat">${escapeHTML(dto.category || '')}</span><span class="ev-sum">${escapeHTML(dto.summary || '')}</span>`;
  return node;
}
function eventClass(dto) {
  return 'event' + (dto.level ? ' level-' + dto.level : '') + (dto.running ? ' running' : '');
}
function applyEvent(dto) {
  if (!dto.summary) return;
  const box = $('events');
  const stick = nearBottom(box);
  if (dto.id && evMap.has(dto.id)) {
    const entry = evMap.get(dto.id);
    entry.node.className = eventClass(dto);
    entry.node.querySelector('.ev-sum').textContent = dto.summary;
  } else {
    const node = makeEventNode(dto);
    if (dto.id) evMap.set(dto.id, { node });
    box.appendChild(node);
    while (box.children.length > 400) { const first = box.firstChild; box.removeChild(first); }
  }
  if (stick) scrollDown(box);
}

function applyStream(d) {
  const box = $('stream');
  const stick = nearBottom(box);
  if (d.clear) { box.textContent = ''; return; }
  box.textContent += d.text || '';
  if (stick) scrollDown(box);
}

// ── Modal helpers ──
function openModal(html, opts) {
  const m = $('modal');
  m.className = 'modal' + (opts && opts.wide ? ' wide' : '');
  m.innerHTML = html;
  $('overlay').hidden = false;
}
function closeModal() { $('overlay').hidden = true; $('modal').innerHTML = ''; }
$('overlay').addEventListener('click', (e) => {
  if (e.target === $('overlay') && !$('overlay').dataset.locked) closeModal();
});

// ── AskUser ──
function showAskUser(d) {
  $('overlay').dataset.locked = '1';
  const qs = d.questions || [];
  const body = qs.map((q, qi) => {
    const opts = (q.options || []).map((o, oi) => {
      const type = q.multiSelect ? 'checkbox' : 'radio';
      return `<label class="opt"><input type="${type}" name="q${qi}" value="${escapeHTML(o.label)}" />
        <span><span class="opt-label">${escapeHTML(o.label)}</span><span class="opt-desc">${escapeHTML(o.description)}</span></span></label>`;
    }).join('');
    return `<div data-q="${qi}" style="margin-bottom:16px">
      <label style="color:var(--accent);font-weight:600">${escapeHTML(q.header)}</label>
      <div style="margin:4px 0 8px">${escapeHTML(q.question)}</div>
      ${opts}
      <label>Khác (tự nhập)</label>
      <input type="text" class="ask-custom" placeholder="Tùy chọn khác…" />
    </div>`;
  }).join('');
  openModal(`<h2>AI cần bạn bổ sung thông tin</h2><div class="sub">Chọn phương án phù hợp để định hướng sáng tác.</div>
    <form id="askForm">${body}
    <div class="modal-actions">
      <button type="button" class="btn" id="askSkip">Bỏ qua (AI tự quyết)</button>
      <button type="submit" class="btn primary">Gửi câu trả lời</button>
    </div></form>`);
  const finish = (answers, notes) => {
    api('/api/answer', { id: d.id, answers, notes }).then(() => { delete $('overlay').dataset.locked; closeModal(); })
      .catch((e) => toast(e.message, 'err'));
  };
  $('askSkip').onclick = () => finish({}, {});
  $('askForm').onsubmit = (e) => {
    e.preventDefault();
    const answers = {}, notes = {};
    qs.forEach((q, qi) => {
      const block = e.target.querySelector(`[data-q="${qi}"]`);
      const checked = Array.from(block.querySelectorAll(`input[name="q${qi}"]:checked`)).map((i) => i.value);
      const custom = block.querySelector('.ask-custom').value.trim();
      const vals = checked.slice();
      if (custom) { vals.push(custom); notes[q.question] = custom; }
      if (vals.length) answers[q.question] = vals.join('、');
    });
    finish(answers, notes);
  };
}

// ── Co-create ──
function openCoCreate(stage) {
  state.cc = { stage, deltas: { thinking: '', reply: '' }, round: null, started: false };
  if (stage) {
    renderCoCreate();
    api('/api/cocreate/open', { stage: true }).catch((e) => { toast(e.message, 'err'); closeModal(); });
  } else {
    // cold: hỏi ý tưởng ban đầu trước
    openModal(`<h2>Cộng tác lập kế hoạch</h2><div class="sub">Trò chuyện với AI để làm rõ ý tưởng, rồi bắt đầu sáng tác.</div>
      <label>Ý tưởng ban đầu của bạn</label>
      <textarea id="ccInitial" placeholder="vd: Một câu chuyện trinh thám lấy bối cảnh Hà Nội thập niên 1990…"></textarea>
      <div class="modal-actions"><button class="btn" onclick="closeModal()">Hủy</button>
      <button class="btn primary" id="ccBegin">Bắt đầu trò chuyện</button></div>`);
    $('ccBegin').onclick = () => {
      const initial = $('ccInitial').value.trim();
      if (!initial) return toast('Nhập ý tưởng ban đầu', 'err');
      state.cc.started = true;
      renderCoCreate();
      api('/api/cocreate/open', { stage: false, initial }).catch((e) => { toast(e.message, 'err'); closeModal(); });
    };
  }
}

function renderCoCreate() {
  const cc = state.cc;
  if (!cc) return;
  const title = cc.stage ? 'Cộng tác giai đoạn' : 'Cộng tác lập kế hoạch';
  const sub = cc.stage ? 'Lập kế hoạch hướng đi tiếp theo, rồi tiếp tục viết.' : 'Làm rõ yêu cầu cùng AI, rồi bắt đầu sáng tác.';
  const startLabel = cc.stage ? 'Áp dụng & tiếp tục' : 'Bắt đầu sáng tác';
  const conv = (cc.round && cc.round.history || []).map((m) => {
    if (m.role === 'system') return `<div class="cc-msg system">${escapeHTML(m.content)}</div>`;
    const role = m.role === 'assistant' ? 'AI' : 'Bạn';
    return `<div class="cc-msg ${m.role}"><div class="role">${role}</div><div class="body">${escapeHTML(m.content)}</div></div>`;
  }).join('');
  const live = [];
  if (cc.deltas.thinking) live.push(`<div class="cc-thinking">${escapeHTML(cc.deltas.thinking)}</div>`);
  if (cc.deltas.reply) live.push(`<div class="cc-msg assistant"><div class="role">AI</div><div class="body">${escapeHTML(cc.deltas.reply)}</div></div>`);
  const suggestions = (cc.round && cc.round.suggestions || []).map((s, i) =>
    `<button class="btn" data-sug="${i}">${escapeHTML((i + 1) + '. ' + s)}</button>`).join('');
  const draft = cc.round && cc.round.draft ? escapeHTML(cc.round.draft) : '<span style="color:var(--dim)">AI sẽ tổng hợp chỉ thị sáng tác tại đây.</span>';
  const canStart = cc.round && cc.round.canStart;

  openModal(`<h2>${title}</h2><div class="sub">${sub}</div>
    <div class="cocreate-grid">
      <div class="cc-col">
        <div class="cc-conv" id="ccConv">${conv}${live.join('')}</div>
        <div class="cc-suggest" id="ccSug">${suggestions}</div>
        <div style="display:flex;gap:8px;margin-top:8px">
          <textarea id="ccInput" rows="1" style="flex:1" placeholder="Nhập trả lời cho AI…"></textarea>
          <button class="btn" id="ccSend">Gửi</button>
        </div>
      </div>
      <div class="cc-col">
        <div style="font-size:12px;color:var(--accent);text-transform:uppercase;margin-bottom:6px">${cc.stage ? 'Hướng đi tiếp theo' : 'Chỉ thị sáng tác'}</div>
        <div class="cc-draft">${draft}</div>
      </div>
    </div>
    <div class="modal-actions">
      <button class="btn" id="ccCancel">Hủy</button>
      <button class="btn primary" id="ccStart" ${canStart ? '' : 'disabled'}>${startLabel}</button>
    </div>`, { wide: true });

  const convBox = $('ccConv'); if (convBox) scrollDown(convBox);
  $('ccSend').onclick = ccSend;
  $('ccInput').addEventListener('keydown', (e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); ccSend(); } });
  $('ccCancel').onclick = () => { api('/api/cocreate/cancel').catch(() => {}); state.cc = null; closeModal(); };
  $('ccStart').onclick = () => {
    api('/api/cocreate/start').then(() => { state.cc = null; closeModal(); toast('Đã áp dụng. Bắt đầu sáng tác.', 'ok'); })
      .catch((e) => toast(e.message, 'err'));
  };
  document.querySelectorAll('#ccSug button').forEach((b) => {
    b.onclick = () => { $('ccInput').value = cc.round.suggestions[+b.dataset.sug]; $('ccInput').focus(); };
  });
}

function ccSend() {
  const input = $('ccInput');
  const text = input.value.trim();
  if (!text) return;
  input.value = '';
  state.cc.deltas = { thinking: '', reply: '' };
  if (state.cc.round) state.cc.round.history = (state.cc.round.history || []).concat([{ role: 'user', content: text }]);
  renderCoCreate();
  api('/api/cocreate/send', { text }).catch((e) => toast(e.message, 'err'));
}

function onCoCreate(d) {
  if (!state.cc) return;
  if (d.phase === 'delta') {
    state.cc.deltas[d.kind] = d.text;
    const box = $('ccConv');
    if (box) {
      // cập nhật tại chỗ phần live mà không dựng lại toàn modal (mượt hơn)
      renderCoCreateLive();
    }
    return;
  }
  if (d.phase === 'round') {
    state.cc.round = d;
    state.cc.deltas = { thinking: '', reply: '' };
    renderCoCreate();
    return;
  }
  if (d.phase === 'error') { toast('Cộng tác lỗi: ' + d.text, 'err'); return; }
  if (d.phase === 'closed') { /* server đã đóng phiên */ return; }
}

function renderCoCreateLive() {
  const box = $('ccConv');
  if (!box) return;
  // bỏ phần live cũ rồi vẽ lại
  box.querySelectorAll('.cc-live').forEach((n) => n.remove());
  const frag = document.createDocumentFragment();
  if (state.cc.deltas.thinking) {
    const t = document.createElement('div'); t.className = 'cc-thinking cc-live'; t.textContent = state.cc.deltas.thinking; frag.appendChild(t);
  }
  if (state.cc.deltas.reply) {
    const r = document.createElement('div'); r.className = 'cc-msg assistant cc-live';
    r.innerHTML = `<div class="role">AI</div><div class="body">${escapeHTML(state.cc.deltas.reply)}</div>`; frag.appendChild(r);
  }
  box.appendChild(frag);
  scrollDown(box);
}

// ── Progress (import / simulate) ──
let progressLog = [];
function onProgress(p) {
  progressLog.push(p);
  const log = $('progLog');
  if (log) {
    const line = document.createElement('div');
    line.className = 'pl' + (p.error ? ' err' : (p.done && !p.error ? ' done' : ''));
    line.textContent = `[${p.stage}] ${p.message}` + (p.total ? ` (${p.current}/${p.total})` : '') + (p.error ? ' — ' + p.error : '');
    log.appendChild(line);
    scrollDown(log);
  }
  if (p.done) {
    const title = $('progTitle');
    if (title) title.textContent = p.error ? 'Đã dừng (lỗi)' : 'Hoàn tất';
    const cancel = $('progCancel');
    if (cancel) cancel.textContent = 'Đóng';
  }
}

function openProgressModal(heading) {
  progressLog = [];
  openModal(`<h2 id="progTitle">${escapeHTML(heading)}</h2><div class="sub">Tiến trình sẽ cập nhật theo thời gian thực.</div>
    <div class="proglog" id="progLog"></div>
    <div class="modal-actions"><button class="btn" id="progCancel">Dừng</button></div>`);
}

// ── Command modals ──
function openExport() {
  openModal(`<h2>Xuất file</h2><div class="sub">Hợp nhất các chương đã hoàn thành thành TXT hoặc EPUB.</div>
    <label>Đường dẫn (để trống = mặc định trong thư mục tác phẩm; hậu tố .txt / .epub quyết định định dạng)</label>
    <input type="text" id="exPath" placeholder="vd: D:\\truyen\\tac-pham.epub" />
    <div class="field-row">
      <div><label>Từ chương</label><input type="number" id="exFrom" min="0" placeholder="đầu" /></div>
      <div><label>Đến chương</label><input type="number" id="exTo" min="0" placeholder="cuối" /></div>
    </div>
    <label class="opt" style="margin-top:12px"><input type="checkbox" id="exOver" /><span class="opt-label">Ghi đè nếu file đã tồn tại</span></label>
    <div class="modal-actions"><button class="btn" onclick="closeModal()">Hủy</button>
    <button class="btn primary" id="exGo">Xuất</button></div>`);
  $('exGo').onclick = () => {
    const body = {
      path: $('exPath').value.trim(),
      from: parseInt($('exFrom').value || '0', 10) || 0,
      to: parseInt($('exTo').value || '0', 10) || 0,
      overwrite: $('exOver').checked,
    };
    $('exGo').disabled = true;
    api('/api/export', body).then((r) => {
      closeModal();
      toast(`✓ Đã xuất ${r.chapters} chương → ${r.path}`, 'ok');
    }).catch((e) => { $('exGo').disabled = false; toast(e.message, 'err'); });
  };
}

function openImport() {
  openModal(`<h2>Nhập truyện có sẵn</h2><div class="sub">Phản suy tiền đề/nhân vật/đại cương từ một file .txt/.md, rồi viết tiếp.</div>
    <label>Đường dẫn file truyện</label>
    <input type="text" id="imPath" placeholder="vd: D:\\truyen\\tieu-thuyet.txt" />
    <label>Bắt đầu từ chương (tùy chọn, 0 = từ đầu)</label>
    <input type="number" id="imFrom" min="0" value="0" />
    <div class="modal-actions"><button class="btn" onclick="closeModal()">Hủy</button>
    <button class="btn primary" id="imGo">Nhập</button></div>`);
  $('imGo').onclick = () => {
    const path = $('imPath').value.trim();
    if (!path) return toast('Nhập đường dẫn file', 'err');
    const from = parseInt($('imFrom').value || '0', 10) || 0;
    api('/api/import', { path, from }).then(() => openProgressModal('Đang nhập truyện'))
      .catch((e) => toast(e.message, 'err'));
  };
}

function openSimulate() {
  openModal(`<h2>Phỏng tác (mô phỏng văn phong)</h2><div class="sub">Đọc thư mục <code>./simulate</code> và tạo/ cập nhật hồ sơ phỏng tác.</div>
    <p style="font-size:13px;color:var(--muted)">Đặt các bài tham khảo (.txt/.md) vào thư mục <code>simulate/</code> trong thư mục khởi chạy, rồi bấm Chạy.</p>
    <label>Hoặc nhập sẵn hồ sơ đã có (đường dẫn .json, tùy chọn)</label>
    <input type="text" id="simImport" placeholder="vd: D:\\profile.json (để trống nếu phân tích thư mục simulate)" />
    <div class="modal-actions"><button class="btn" onclick="closeModal()">Hủy</button>
    <button class="btn primary" id="simGo">Chạy</button></div>`);
  $('simGo').onclick = () => {
    const imp = $('simImport').value.trim();
    const call = imp ? api('/api/importsim', { text: imp }) : api('/api/simulate');
    call.then(() => openProgressModal(imp ? 'Đang nhập hồ sơ phỏng tác' : 'Đang tạo hồ sơ phỏng tác'))
      .catch((e) => toast(e.message, 'err'));
  };
}

function openGrounding() {
  const g = state.grounding;
  openModal(`<h2>Viết bám sát nhân vật có thật</h2><div class="sub">Neo truyện vào một nhân vật/chủ thể CÓ THẬT: giữ đúng tên, mốc thời gian, thành tựu, tính cách thật làm mỏ neo; phần phiêu lưu vẫn hư cấu tự do. Áp dụng cho lần "Bắt đầu" kế tiếp.</div>
    <label class="opt"><input type="checkbox" id="grEnabled" ${g.enabled ? 'checked' : ''} /><span class="opt-label">Bật chế độ bám sát nhân vật có thật</span></label>
    <label style="margin-top:12px">Tên nhân vật có thật</label>
    <input type="text" id="grSubject" placeholder="vd: Wilhelm Steinitz" value="${escapeHTML(g.subject)}" />
    <label style="margin-top:12px">Tư liệu tham khảo (dán tiểu sử/dữ kiện thật — để trống rồi bấm “Soạn hồ sơ” để AI soạn nháp)</label>
    <textarea id="grSource" rows="8" placeholder="Dán tiểu sử, mốc thời gian, thành tựu, tính cách… của nhân vật.">${escapeHTML(g.sourceText)}</textarea>
    <div class="sub" id="grNote" style="margin-top:6px"></div>
    <div class="modal-actions">
      <button class="btn" id="grDraft">✦ Soạn hồ sơ (AI)</button>
      <button class="btn" onclick="closeModal()">Hủy</button>
      <button class="btn primary" id="grSave">Lưu</button></div>`);
  $('grDraft').onclick = () => {
    const subject = $('grSubject').value.trim();
    if (!subject) return toast('Nhập tên nhân vật có thật trước', 'err');
    $('grDraft').disabled = true;
    $('grNote').textContent = '⏳ AI đang soạn nháp… (dựa vào trí nhớ mô hình, cần bạn kiểm chứng)';
    api('/api/dossier/draft', { subject }).then((r) => {
      $('grSource').value = r.markdown || '';
      $('grNote').textContent = '⚠ ' + (r.disclaimer || 'Bản nháp AI — hãy kiểm chứng và chỉnh sửa trước khi dùng.');
    }).catch((e) => toast(e.message, 'err')).finally(() => { $('grDraft').disabled = false; });
  };
  $('grSave').onclick = () => {
    state.grounding = {
      enabled: $('grEnabled').checked,
      subject: $('grSubject').value.trim(),
      sourceText: $('grSource').value.trim(),
    };
    closeModal();
    toast(state.grounding.enabled ? '✓ Đã bật bám sát nhân vật thật cho lần Bắt đầu kế tiếp' : 'Đã tắt bám sát nhân vật thật', 'ok');
  };
}

function openDiag() {
  openModal(`<h2>Chẩn đoán</h2><div class="sub">Đang phân tích sức khỏe sáng tác… có thể mất vài giây.</div><div id="diagBody">⏳ Đang chạy…</div>`);
  api('/api/diag').then((r) => renderDiag(r)).catch((e) => { $('diagBody').innerHTML = `<div class="errline">${escapeHTML(e.message)}</div>`; });
}
function renderDiag(r) {
  const rep = r.report || {};
  const st = rep.Stats || {};
  const findings = rep.Findings || [];
  let html = `<div class="diag-stats">
    <span>Chương: <b>${st.CompletedChapters || 0}/${st.TotalChapters || 0}</b></span>
    <span>Chữ: <b>${(st.TotalWords || 0).toLocaleString('vi-VN')}</b></span>
    <span>Đánh giá: <b>${st.ReviewCount || 0}</b></span>
    ${st.RewriteCount ? `<span>Viết lại: <b>${st.RewriteCount}</b></span>` : ''}
    ${st.AvgReviewScore ? `<span>Điểm TB: <b>${st.AvgReviewScore.toFixed(1)}</b></span>` : ''}
  </div>`;
  if (!findings.length) html += '<div class="checkline">✓ Không phát hiện vấn đề.</div>';
  else html += findings.map((f) => `<div class="finding ${f.Severity}">
    <div class="ft">[${f.Severity}] ${escapeHTML(f.Title)}</div>
    ${f.Evidence ? `<div class="fe">${escapeHTML(f.Evidence)}</div>` : ''}
    ${f.Suggestion ? `<div class="fs">→ ${escapeHTML(f.Suggestion)}</div>` : ''}
  </div>`).join('');
  if (r.exportPath) html += `<div class="sub" style="margin-top:12px">Báo cáo ẩn danh đã lưu: ${escapeHTML(r.exportPath)}</div>`;
  $('diagBody').innerHTML = html;
}

// ── Models ──
function openModels() {
  Promise.all([getJSON('/api/models'), getJSON('/api/thinking')]).then(([m, t]) => {
    const provOpts = (prov) => m.providers.map((p) => `<option value="${escapeHTML(p.name)}" ${p.name === prov ? 'selected' : ''}>${escapeHTML(p.name)}</option>`).join('');
    const thinkMap = {}; (t.roles || []).forEach((r) => thinkMap[r.role] = r);
    const rows = (m.roles || []).map((r) => {
      const tr = thinkMap[r.role] || { available: [], current: '' };
      const models = (m.providers.find((p) => p.name === r.provider) || { models: [] }).models;
      const modelOpts = models.map((mm) => `<option ${mm === r.model ? 'selected' : ''}>${escapeHTML(mm)}</option>`).join('') ||
        `<option selected>${escapeHTML(r.model || '')}</option>`;
      const thinkOpts = (tr.available || []).map((lv) => `<option value="${escapeHTML(lv)}" ${lv === tr.current ? 'selected' : ''}>${escapeHTML(lv)}</option>`).join('');
      return `<tr data-role="${r.role}" style="border-bottom:1px solid var(--border)">
        <td style="padding:6px 8px;font-weight:600">${escapeHTML(roleVi(r.role))}</td>
        <td style="padding:6px 8px"><select class="mProv">${provOpts(r.provider)}</select></td>
        <td style="padding:6px 8px"><input type="text" class="mModel" value="${escapeHTML(r.model || '')}" list="ml-${r.role}" style="width:200px"/>
          <datalist id="ml-${r.role}">${modelOpts}</datalist></td>
        <td style="padding:6px 8px"><select class="mThink"><option value="">(mặc định)</option>${thinkOpts}</select></td>
        <td style="padding:6px 8px"><button class="btn mApply">Áp dụng</button></td>
      </tr>`;
    }).join('');
    openModal(`<h2>Mô hình & mức suy luận</h2><div class="sub">Đổi mô hình theo vai trò. Thay đổi được ghi vào cấu hình.</div>
      <table style="width:100%;border-collapse:collapse;font-size:13px">
        <thead><tr style="color:var(--dim);text-align:left"><th style="padding:6px 8px">Vai trò</th><th>Provider</th><th>Mô hình</th><th>Suy luận</th><th></th></tr></thead>
        <tbody>${rows}</tbody></table>
      <div class="modal-actions">
        <button class="btn mAuto" data-preset="standard">Tự chọn: Chuẩn</button>
        <button class="btn mAuto" data-preset="economy">Tự chọn: Tiết kiệm</button>
        <button class="btn" onclick="closeModal()">Đóng</button>
      </div>`, { wide: true });
    const autoBtns = document.querySelectorAll('.mAuto');
    autoBtns.forEach((auto) => {
      auto.onclick = () => {
        autoBtns.forEach((b) => (b.disabled = true));
        api('/api/model/auto', { preset: auto.dataset.preset })
          .then((r) => { toast('Đã áp preset: ' + ((r && r.label) || auto.dataset.preset), 'ok'); openModels(); })
          .catch((e) => { autoBtns.forEach((b) => (b.disabled = false)); toast(e.message, 'err'); });
      };
    });
    document.querySelectorAll('.mApply').forEach((b) => {
      b.onclick = () => {
        const tr = b.closest('tr');
        const role = tr.dataset.role;
        const provider = tr.querySelector('.mProv').value;
        const model = tr.querySelector('.mModel').value.trim();
        const level = tr.querySelector('.mThink').value;
        b.disabled = true;
        api('/api/model', { role, provider, model }).then(() => api('/api/thinking', { role, level }))
          .then(() => { b.disabled = false; toast('Đã đổi mô hình cho ' + roleVi(role), 'ok'); })
          .catch((e) => { b.disabled = false; toast(e.message, 'err'); });
      };
    });
  }).catch((e) => toast(e.message, 'err'));
}
function roleVi(r) { return { default: 'Mặc định', coordinator: 'Điều phối', architect: 'Kiến trúc', writer: 'Người viết', editor: 'Biên tập' }[r] || r; }

// ── Actions ──
function onPrimary() {
  const s = state.snapshot;
  const input = $('mainInput');
  const text = input.value.trim();
  if (s && s.IsRunning) {
    if (!text) return toast('Nhập nội dung can thiệp', 'err');
    api('/api/steer', { text }).then(() => { input.value = ''; updatePrimary(s); }).catch((e) => toast(e.message, 'err'));
    return;
  }
  if (isFresh(s)) {
    if (!text) return toast('Nhập yêu cầu sáng tác', 'err');
    $('btnPrimary').disabled = true;
    const g = state.grounding || {};
    api('/api/start', { prompt: text, grounding: !!g.enabled, subject: g.subject || '', sourceText: g.sourceText || '' })
      .then(() => { input.value = ''; }).catch((e) => toast(e.message, 'err'))
      .finally(() => { $('btnPrimary').disabled = false; });
    return;
  }
  if (text) {
    api('/api/continue', { text }).then(() => { input.value = ''; }).catch((e) => toast(e.message, 'err'));
  } else {
    api('/api/resume').then((r) => { if (!r.label) toast('Không có gì để khôi phục', 'err'); }).catch((e) => toast(e.message, 'err'));
  }
}

function bindUI() {
  $('btnPrimary').onclick = onPrimary;
  $('btnAbort').onclick = () => api('/api/abort').catch((e) => toast(e.message, 'err'));
  $('btnModels').onclick = openModels;
  const input = $('mainInput');
  input.addEventListener('input', () => {
    input.style.height = 'auto'; input.style.height = Math.min(140, input.scrollHeight) + 'px';
    if (state.snapshot) updatePrimary(state.snapshot);
  });
  input.addEventListener('keydown', (e) => { if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); onPrimary(); } });

  document.querySelectorAll('[data-cmd]').forEach((b) => {
    b.onclick = () => {
      const cmd = b.dataset.cmd;
      if (cmd === 'abort') return api('/api/abort').catch((e) => toast(e.message, 'err'));
      if (cmd === 'export') return openExport();
      if (cmd === 'import') return openImport();
      if (cmd === 'simulate') return openSimulate();
      if (cmd === 'grounding') return openGrounding();
      if (cmd === 'diag') return openDiag();
      if (cmd === 'cocreate') return openCoCreate(!isFresh(state.snapshot));
    };
  });

  document.querySelectorAll('.tab').forEach((t) => {
    t.onclick = () => {
      document.querySelectorAll('.tab').forEach((x) => x.classList.remove('active'));
      document.querySelectorAll('.tab-pane').forEach((x) => x.classList.remove('active'));
      t.classList.add('active');
      $('pane-' + t.dataset.tab).classList.add('active');
    };
  });

  document.addEventListener('keydown', (e) => { if (e.key === 'Escape' && !$('overlay').dataset.locked) closeModal(); });
}

// progress cancel button uses delegation since modal is dynamic
$('modal').addEventListener('click', (e) => {
  if (e.target && e.target.id === 'progCancel') {
    const ids = progressLog.map((p) => p.id).filter(Boolean);
    const id = ids[ids.length - 1];
    const allDone = progressLog.length && progressLog[progressLog.length - 1].done;
    if (id && !allDone) api('/api/job/cancel', { text: id }).catch(() => {});
    closeModal();
  }
});

// ── Boot ──
window.closeModal = closeModal;
getJSON('/api/meta').then((m) => { document.title = `ainovel ${m.version || ''} — Xưởng sáng tác AI`; }).catch(() => {});
bindUI();
connect();
