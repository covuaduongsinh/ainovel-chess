// Logic màn chọn dự án. Trang này độc lập hoàn toàn với workbench (app.js) — chỉ dùng chung
// styles.css. Mọi thao tác lọc / sắp xếp chạy thuần phía trình duyệt trên dữ liệu của một lần
// gọi /api/projects; chỉ tạo / mở / đổi tên / lưu trữ / xóa mới gọi máy chủ.

const $ = (id) => document.getElementById(id);

// ── Trạng thái ────────────────────────────────────────────────────────────────
let allProjects = [];  // dự án đang dùng
let allArchived = [];  // dự án trong kho lưu trữ
let tab = 'active';    // 'active' | 'archived'
let query = '';
let sortBy = 'recent';
let openMenuDir = null; // thư mục của thẻ đang mở menu ⋯

// ── Tiện ích ──────────────────────────────────────────────────────────────────
function escapeHTML(s) {
  return String(s == null ? '' : s).replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));
}

function fmtDate(s) {
  if (!s) return '';
  const d = new Date(s);
  if (isNaN(d.getTime())) return '';
  return d.toLocaleString('vi-VN', { hour12: false });
}

function fmtNum(n) { return (n || 0).toLocaleString('vi-VN'); }

// fold bỏ dấu tiếng Việt và hạ chữ thường, để gõ không dấu vào ô lọc vẫn khớp tên có dấu.
// NFD tách dấu thành ký tự tổ hợp riêng (Unicode category Mn), xóa đi là còn chữ trần;
// riêng đ/Đ không có dạng tổ hợp nên phải thay tay.
function fold(s) {
  return String(s == null ? '' : s)
    .normalize('NFD').replace(/\p{Mn}/gu, '')
    .replace(/đ/g, 'd').replace(/Đ/g, 'D')
    .toLowerCase();
}

// slugify là bản sao phía trình duyệt của store.Slugify (internal/store/projects.go), chỉ dùng để
// xem trước tên thư mục trong hộp thoại đổi tên. Máy chủ vẫn là nơi quyết định tên thật.
const RESERVED = new Set(['con', 'prn', 'aux', 'nul',
  'com1', 'com2', 'com3', 'com4', 'com5', 'com6', 'com7', 'com8', 'com9',
  'lpt1', 'lpt2', 'lpt3', 'lpt4', 'lpt5', 'lpt6', 'lpt7', 'lpt8', 'lpt9']);

function slugify(name) {
  let out = '';
  for (const ch of String(name || '')) {
    if (ch.codePointAt(0) < 0x20) continue;
    if ('\\/:*?"<>|'.includes(ch)) continue;
    out += (ch === ' ' || ch === '\t') ? '-' : ch;
  }
  while (out.includes('--')) out = out.replace(/--/g, '-');
  out = out.replace(/^[ .-]+/, '').replace(/[ .-]+$/, '');
  if (!out) return 'novel';
  if (RESERVED.has(out.toLowerCase())) out += '-novel';
  return out;
}

const PHASE_LABELS = {
  init: ['Khởi tạo', 'ph-init'],
  premise: ['Ý tưởng', 'ph-premise'],
  outline: ['Dàn ý', 'ph-outline'],
  writing: ['Đang viết', 'ph-writing'],
  complete: ['Hoàn thành', 'ph-complete'],
};

// phaseBadge trả về [nhãn tiếng Việt, lớp CSS]. Phase lạ giữ nguyên chuỗi gốc thay vì nuốt đi.
function phaseBadge(p) {
  if (!p.hasProgress) return ['Chưa bắt đầu', 'ph-none'];
  return PHASE_LABELS[p.phase] || [p.phase || 'Không rõ', 'ph-init'];
}

function toast(msg, kind) {
  const el = document.createElement('div');
  el.className = 'toast' + (kind ? ' ' + kind : '');
  el.textContent = msg;
  document.body.appendChild(el);
  setTimeout(() => el.remove(), 3200);
}

function showErr(msg) {
  $('err').textContent = msg;
  $('err').hidden = false;
}

// post gửi JSON và ném Error mang thông báo tiếng Việt của máy chủ khi thất bại.
async function post(url, body) {
  const r = await fetch(url, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body || {}),
  });
  const d = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(d.error || 'Thao tác thất bại');
  return d;
}

// ── Tải & vẽ danh sách ────────────────────────────────────────────────────────
function load() {
  return fetch('/api/projects')
    .then((r) => r.json())
    .then((data) => {
      allProjects = data.projects || [];
      allArchived = data.archived || [];
      $('root').textContent = 'Thư mục: ' + (data.root || '');
      $('rootHint').textContent = data.root || 'output';
      render();
    })
    .catch((e) => {
      $('grid').innerHTML = '';
      $('empty').hidden = false;
      $('empty').textContent = 'Lỗi tải danh sách: ' + e.message;
    });
}

function currentList() { return tab === 'archived' ? allArchived : allProjects; }

function ratio(p) {
  if (!p.totalChaps) return -1;
  return (p.chapters || 0) / p.totalChaps;
}

function sortList(list) {
  const out = list.slice();
  if (sortBy === 'name') {
    out.sort((a, b) => String(a.name).localeCompare(String(b.name), 'vi'));
  } else if (sortBy === 'words') {
    out.sort((a, b) => (b.words || 0) - (a.words || 0));
  } else if (sortBy === 'progress') {
    out.sort((a, b) => ratio(b) - ratio(a));
  } else {
    out.sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt));
  }
  return out;
}

function cardHTML(p) {
  const [label, cls] = phaseBadge(p);
  const stats = p.hasProgress
    ? `${p.chapters}/${p.totalChaps || '?'} chương · ${fmtNum(p.words)} chữ`
    : 'Chưa có nội dung';
  const when = fmtDate(p.updatedAt);
  const pct = p.totalChaps ? Math.min(100, Math.round((p.chapters || 0) / p.totalChaps * 100)) : null;
  const bar = pct === null ? '' : `<div class="pk-progress">
        <div class="pk-bar"><i style="width:${pct}%"></i></div>
        <span class="pk-pct">${pct}%</span>
      </div>`;

  return `<div class="pk-card${p.phase === 'complete' ? ' is-complete' : ''}" data-dir="${escapeHTML(p.dir)}" tabindex="0" role="button">
      <div class="pk-card-top">
        <div class="pk-card-name">${escapeHTML(p.name)}</div>
        <button type="button" class="pk-more" data-act="menu" title="Thao tác khác">⋯</button>
      </div>
      <div class="pk-card-meta">
        <span class="pk-badge ${cls}">${escapeHTML(label)}</span>
        <span class="pk-slug" title="${escapeHTML(p.slug)}">${escapeHTML(p.slug)}</span>
      </div>
      <div class="pk-stats">${escapeHTML(stats)}</div>
      ${bar}
      <div class="pk-card-foot">
        <span class="pk-when">${when ? 'Sửa cuối ' + escapeHTML(when) : ''}</span>
        <button type="button" class="btn primary" data-act="open">Mở</button>
      </div>
    </div>`;
}

function render() {
  $('tabActive').classList.toggle('on', tab === 'active');
  $('tabArchived').classList.toggle('on', tab === 'archived');
  $('tabArchived').textContent = allArchived.length ? `Lưu trữ (${allArchived.length})` : 'Lưu trữ';
  $('queryClear').hidden = !query;

  const all = currentList();
  const q = fold(query.trim());
  const shown = sortList(q ? all.filter((p) => fold(p.name).includes(q) || fold(p.slug).includes(q)) : all);

  $('count').textContent = q
    ? `${shown.length}/${all.length} dự án`
    : (all.length ? `${all.length} dự án` : '');

  $('grid').innerHTML = shown.map(cardHTML).join('');
  closeMenu();

  if (shown.length) {
    $('empty').hidden = true;
    return;
  }
  $('empty').hidden = false;
  if (q) {
    $('empty').innerHTML = `Không có dự án nào khớp “${escapeHTML(query.trim())}”.
      <div><button type="button" class="btn" id="clearFilter">Xóa bộ lọc</button></div>`;
    $('clearFilter').onclick = () => { query = ''; $('query').value = ''; render(); };
  } else if (tab === 'archived') {
    $('empty').textContent = 'Kho lưu trữ trống. Dùng menu ⋯ trên thẻ dự án để cất bớt dự án chưa dùng tới.';
  } else {
    $('empty').textContent = 'Chưa có dự án nào. Hãy nhập tên sách ở ô phía trên để tạo dự án đầu tiên.';
  }
}

function findProject(dir) {
  return currentList().find((p) => p.dir === dir);
}

// ── Menu ⋯ trên thẻ ───────────────────────────────────────────────────────────
function closeMenu() {
  const m = document.querySelector('.pk-menu');
  if (m) m.remove();
  document.querySelectorAll('.pk-more.on').forEach((b) => b.classList.remove('on'));
  openMenuDir = null;
}

function toggleMenu(card, btn) {
  const dir = card.dataset.dir;
  if (openMenuDir === dir) { closeMenu(); return; }
  closeMenu();
  openMenuDir = dir;
  btn.classList.add('on');

  const archived = tab === 'archived';
  const menu = document.createElement('div');
  menu.className = 'pk-menu';
  menu.innerHTML = `<button type="button" data-act="rename">Đổi tên</button>
    <button type="button" data-act="${archived ? 'unarchive' : 'archive'}">${archived ? 'Bỏ lưu trữ' : 'Lưu trữ'}</button>
    <div class="pk-menu-sep"></div>
    <button type="button" class="danger" data-act="delete">Xóa…</button>`;
  card.appendChild(menu);

  // Thẻ ở cuối vùng cuộn: mở menu lên trên để không bị khuất dưới mép.
  const body = document.querySelector('.pk-body');
  if (body && menu.getBoundingClientRect().bottom > body.getBoundingClientRect().bottom - 8) {
    menu.classList.add('up');
  }
}

// ── Thao tác trên dự án ───────────────────────────────────────────────────────
function openProject(dir) {
  // Máy chủ swap sang workbench ngay trong lần gọi này, trình duyệt chỉ cần tải lại.
  post('/api/projects/open', { dir })
    .then(() => location.reload())
    .catch((e) => toast(e.message, 'err'));
}

function archiveProject(dir, undo) {
  const url = undo ? '/api/projects/unarchive' : '/api/projects/archive';
  post(url, { dir })
    .then(() => load())
    .then(() => toast(undo ? 'Đã đưa dự án trở lại danh sách' : 'Đã cất dự án vào kho lưu trữ', 'ok'))
    .catch((e) => toast(e.message, 'err'));
}

// ── Hộp thoại đổi tên ─────────────────────────────────────────────────────────
let renameDir = null;

function openRename(p) {
  renameDir = p.dir;
  $('renameSub').textContent = 'Đổi tên sách và cả tên thư mục trên đĩa.';
  $('renameInput').value = p.name;
  $('renameErr').hidden = true;
  $('renameOverlay').hidden = false;
  updateRenamePreview();
  $('renameInput').focus();
  $('renameInput').select();
}

function updateRenamePreview() {
  const v = $('renameInput').value.trim();
  $('renamePreview').innerHTML = v ? `Thư mục sẽ là <b>${escapeHTML(slugify(v))}</b>` : '';
}

function submitRename() {
  const name = $('renameInput').value.trim();
  if (!name) {
    $('renameErr').textContent = 'Vui lòng nhập tên dự án';
    $('renameErr').hidden = false;
    return;
  }
  $('renameOk').disabled = true;
  post('/api/projects/rename', { dir: renameDir, name })
    .then(() => { $('renameOverlay').hidden = true; return load(); })
    .then(() => toast('Đã đổi tên dự án', 'ok'))
    .catch((e) => { $('renameErr').textContent = e.message; $('renameErr').hidden = false; })
    .finally(() => { $('renameOk').disabled = false; });
}

// ── Hộp thoại xóa ─────────────────────────────────────────────────────────────
let deleteTarget = null;

function openDelete(p) {
  deleteTarget = p;
  $('deleteSub').textContent = p.slug;
  const lost = p.hasProgress
    ? `${p.chapters} chương và ${fmtNum(p.words)} chữ`
    : 'toàn bộ dữ liệu';
  $('deleteWarn').textContent =
    `Thao tác này xóa vĩnh viễn thư mục dự án — mất ${lost}. Không thể hoàn tác. `
    + 'Nếu chỉ muốn tạm cất đi, hãy dùng "Lưu trữ".';
  $('deleteInput').value = '';
  $('deleteErr').hidden = true;
  $('deleteOk').disabled = true;
  $('deleteOverlay').hidden = false;
  $('deleteInput').focus();
}

// Nút xóa chỉ bật khi gõ đúng tên sách hoặc tên thư mục — máy chủ kiểm tra lại lần nữa.
function deleteConfirmMatches() {
  if (!deleteTarget) return false;
  const v = $('deleteInput').value.trim().toLowerCase();
  return v !== '' && (v === String(deleteTarget.name).trim().toLowerCase() || v === String(deleteTarget.slug).toLowerCase());
}

function submitDelete() {
  if (!deleteConfirmMatches()) return;
  $('deleteOk').disabled = true;
  post('/api/projects/delete', { dir: deleteTarget.dir, confirm: $('deleteInput').value.trim() })
    .then(() => { $('deleteOverlay').hidden = true; return load(); })
    .then(() => toast('Đã xóa dự án', 'ok'))
    .catch((e) => {
      $('deleteErr').textContent = e.message;
      $('deleteErr').hidden = false;
      $('deleteOk').disabled = false;
    });
}

// ── Gắn sự kiện ───────────────────────────────────────────────────────────────
$('createForm').addEventListener('submit', (e) => {
  e.preventDefault();
  $('err').hidden = true;
  const name = $('name').value.trim();
  if (!name) { showErr('Vui lòng nhập tên dự án'); return; }
  $('createBtn').disabled = true;
  $('createBtn').textContent = 'Đang tạo…';
  post('/api/projects', { name })
    .then(() => location.reload())
    .catch((err) => {
      showErr(err.message);
      $('createBtn').disabled = false;
      $('createBtn').textContent = '+ Tạo & mở';
    });
});

$('query').addEventListener('input', (e) => { query = e.target.value; render(); });
$('queryClear').addEventListener('click', () => { query = ''; $('query').value = ''; render(); $('query').focus(); });
$('sort').addEventListener('change', (e) => { sortBy = e.target.value; render(); });
$('tabActive').addEventListener('click', () => { tab = 'active'; render(); });
$('tabArchived').addEventListener('click', () => { tab = 'archived'; render(); });

// Ủy quyền sự kiện cho cả lưới: thẻ được vẽ lại sau mỗi lần lọc nên không gắn handler từng thẻ.
$('grid').addEventListener('click', (e) => {
  const card = e.target.closest('.pk-card');
  if (!card) return;
  const act = e.target.closest('[data-act]');
  const dir = card.dataset.dir;

  if (act && act.dataset.act === 'menu') { toggleMenu(card, act); return; }
  // Bấm vào khoảng trống của menu (viền, đường ngăn) không được coi là bấm vào thẻ.
  if (e.target.closest('.pk-menu') && !act) return;
  if (act && act.closest('.pk-menu')) {
    closeMenu();
    const p = findProject(dir);
    if (!p) return;
    if (act.dataset.act === 'rename') openRename(p);
    else if (act.dataset.act === 'archive') archiveProject(dir, false);
    else if (act.dataset.act === 'unarchive') archiveProject(dir, true);
    else if (act.dataset.act === 'delete') openDelete(p);
    return;
  }
  openProject(dir);
});

$('grid').addEventListener('keydown', (e) => {
  if (e.key !== 'Enter' && e.key !== ' ') return;
  const card = e.target.closest('.pk-card');
  if (!card || e.target !== card) return;
  e.preventDefault();
  openProject(card.dataset.dir);
});

// Bấm ra ngoài thì đóng menu ⋯.
document.addEventListener('click', (e) => {
  if (openMenuDir && !e.target.closest('.pk-menu') && !e.target.closest('.pk-more')) closeMenu();
});

$('renameInput').addEventListener('input', updateRenamePreview);
$('renameInput').addEventListener('keydown', (e) => { if (e.key === 'Enter') submitRename(); });
$('renameOk').addEventListener('click', submitRename);
$('renameCancel').addEventListener('click', () => { $('renameOverlay').hidden = true; });

$('deleteInput').addEventListener('input', () => { $('deleteOk').disabled = !deleteConfirmMatches(); });
$('deleteInput').addEventListener('keydown', (e) => { if (e.key === 'Enter') submitDelete(); });
$('deleteOk').addEventListener('click', submitDelete);
$('deleteCancel').addEventListener('click', () => { $('deleteOverlay').hidden = true; });

document.addEventListener('keydown', (e) => {
  if (e.key !== 'Escape') return;
  if (!$('renameOverlay').hidden) { $('renameOverlay').hidden = true; return; }
  if (!$('deleteOverlay').hidden) { $('deleteOverlay').hidden = true; return; }
  if (openMenuDir) closeMenu();
});

load();
