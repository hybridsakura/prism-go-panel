/* RustDesk 管理后台 SPA（无依赖，hash 路由，GitHub Primer 风格） */
'use strict';

const state = { user: null, all: false, page: 1, keyword: '', total: 0, size: 15 };

const $app = document.getElementById('app');

/* ---------- Octicons（内嵌 SVG，16px） ---------- */

const ICONS = {
  logo: '<svg width="22" height="22" viewBox="0 0 16 16"><path d="M1.5 2.75a.25.25 0 0 1 .25-.25h12.5a.25.25 0 0 1 .25.25v7.5a.25.25 0 0 1-.25.25H1.75a.25.25 0 0 1-.25-.25v-7.5zM0 2.75C0 1.784.784 1 1.75 1h12.5c.966 0 1.75.784 1.75 1.75v7.5A1.75 1.75 0 0 1 14.25 12H9.5v1.5h1.75a.75.75 0 0 1 0 1.5h-6.5a.75.75 0 0 1 0-1.5H6.5V12H1.75A1.75 1.75 0 0 1 0 10.25v-7.5z"/></svg>',
  moon: '<svg width="16" height="16" viewBox="0 0 16 16"><path d="M9.598 1.591a.749.749 0 0 1 .785-.175 7.001 7.001 0 1 1-8.967 8.967.75.75 0 0 1 .961-.96 5.5 5.5 0 0 0 7.046-7.046.75.75 0 0 1 .175-.786zm1.616 1.945a7 7 0 0 1-7.678 7.678 5.499 5.499 0 0 0 7.678-7.678z"/></svg>',
  sun: '<svg width="16" height="16" viewBox="0 0 16 16"><path d="M8 12a4 4 0 1 1 0-8 4 4 0 0 1 0 8zM8 0a.75.75 0 0 1 .75.75v1.5a.75.75 0 0 1-1.5 0V.75A.75.75 0 0 1 8 0zm0 13a.75.75 0 0 1 .75.75v1.5a.75.75 0 0 1-1.5 0v-1.5A.75.75 0 0 1 8 13zM2.343 2.343a.75.75 0 0 1 1.061 0l1.06 1.061a.75.75 0 0 1-1.06 1.061l-1.06-1.06a.75.75 0 0 1 0-1.062zm9.193 9.193a.75.75 0 0 1 1.06 0l1.061 1.06a.75.75 0 0 1-1.06 1.061l-1.061-1.06a.75.75 0 0 1 0-1.061zM16 8a.75.75 0 0 1-.75.75h-1.5a.75.75 0 0 1 0-1.5h1.5A.75.75 0 0 1 16 8zM3 8a.75.75 0 0 1-.75.75H.75a.75.75 0 0 1 0-1.5h1.5A.75.75 0 0 1 3 8zm10.657-5.657a.75.75 0 0 1 0 1.061l-1.06 1.06a.75.75 0 1 1-1.062-1.06l1.061-1.061a.75.75 0 0 1 0 1.06zM4.464 11.536a.75.75 0 0 1 0 1.06l-1.06 1.061a.75.75 0 0 1-1.061-1.06l1.06-1.061a.75.75 0 0 1 1.06 0z"/></svg>',
  signout: '<svg width="14" height="14" viewBox="0 0 16 16"><path d="M2 2.75C2 1.784 2.784 1 3.75 1h2.5a.75.75 0 0 1 0 1.5h-2.5a.25.25 0 0 0-.25.25v10.5c0 .138.112.25.25.25h2.5a.75.75 0 0 1 0 1.5h-2.5A1.75 1.75 0 0 1 2 13.25V2.75zm10.44 4.5-1.97-1.97a.749.749 0 0 1 .326-1.275.749.749 0 0 1 .734.215l3.25 3.25a.75.75 0 0 1 0 1.06l-3.25 3.25a.749.749 0 0 1-1.275-.326.749.749 0 0 1 .215-.734l1.97-1.97H6.75a.75.75 0 0 1 0-1.5h5.69z"/></svg>',
  copy: '<svg width="14" height="14" viewBox="0 0 16 16"><path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 0 1 0 1.5h-1.5a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-1.5a.75.75 0 0 1 1.5 0v1.5A1.75 1.75 0 0 1 9.25 16h-7.5A1.75 1.75 0 0 1 0 14.25v-7.5z"/><path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0 1 14.25 11h-7.5A1.75 1.75 0 0 1 5 9.25v-7.5zm1.75-.25a.25.25 0 0 0-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 0 0 .25-.25v-7.5a.25.25 0 0 0-.25-.25h-7.5z"/></svg>',
};

function icon(name) { return ICONS[name] || ''; }

/* ---------- 基础工具 ---------- */

function esc(s) {
  return String(s == null ? '' : s).replace(/[&<>"']/g, c => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[c]));
}

function flash(msg, kind) {
  const area = document.getElementById('flash-area');
  const el = document.createElement('div');
  el.className = 'flash' + (kind ? ' ' + kind : '');
  el.textContent = msg;
  area.appendChild(el);
  setTimeout(() => el.remove(), 3000);
}
function toast(msg) { flash(msg, ''); }

async function api(path, opts = {}) {
  const res = await fetch(path, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    ...opts,
  });
  if (res.status === 401) {
    state.user = null;
    location.hash = '#/login';
    throw new Error('未登录');
  }
  if (res.status === 403) {
    const data = await res.json().catch(() => ({}));
    flash(data.error || '需要管理员权限', 'error');
    throw new Error(data.error || 'forbidden');
  }
  return res.json();
}

function fmtTime(s) { return s ? s.replace('T', ' ').replace(/\+.*$/, '').slice(0, 19) : '-'; }
function truncate(s, n) { s = String(s || ''); return s.length > n ? s.slice(0, n) + '…' : s; }

function copyText(text, doneMsg) {
  navigator.clipboard.writeText(text).then(() => toast(doneMsg || '已复制'));
}

// 可复制的单行代码块（多处复用）
function copyblockHTML(id, text) {
  return `<div class="copyblock"><code id="${id}">${esc(text)}</code><button class="btn" data-cb="${id}">${icon('copy')}复制</button></div>`;
}
function bindCopyblocks() {
  document.querySelectorAll('[data-cb]').forEach(b => {
    b.onclick = () => copyText(document.getElementById(b.dataset.cb).textContent, '已复制');
  });
}

/* ---------- 主题 ---------- */

function currentTheme() { return document.documentElement.getAttribute('data-color-mode') || 'light'; }
function toggleTheme() {
  const next = currentTheme() === 'dark' ? 'light' : 'dark';
  document.documentElement.setAttribute('data-color-mode', next);
  try { localStorage.setItem('rd-color-mode', next); } catch (e) {}
  updateThemeIcon();
}
function updateThemeIcon() {
  const btn = document.getElementById('theme-toggle');
  if (btn) btn.innerHTML = currentTheme() === 'dark' ? icon('sun') : icon('moon');
}

// 主题切换用事件委托：视图重渲染不影响，且保证 click 一定被捕获
document.addEventListener('click', e => {
  const t = e.target.closest && e.target.closest('#theme-toggle');
  if (t) toggleTheme();
});

/* ---------- 路由 ---------- */

const routes = {
  '/': viewDevices,
  '/conn-logs': viewConnLogs,
  '/file-logs': viewFileLogs,
  '/share': viewShare,
  '/conflicts': viewConflicts,
  '/server': viewServer,
  '/users': viewUsers,
};

const NAV_ITEMS = [
  ['#/', '设备'],
  ['#/conn-logs', '连接日志'],
  ['#/file-logs', '文件日志'],
  ['#/share', '分享'],
  ['#/conflicts', 'ID 冲突', true],
  ['#/server', '服务端配置', true],
  ['#/users', '用户', true],
];

function router() {
  const hash = location.hash.slice(1) || '/';
  if (!state.user && hash !== '/login' && hash !== '/register') {
    location.hash = '#/login';
    return;
  }
  if (state.user && (hash === '/login' || hash === '/register')) {
    location.hash = '#/';
    return;
  }
  if (!state.user) {
    $app.innerHTML = hash === '/register' ? viewRegister() : viewLogin();
    bindAuthForm(hash !== '/register');
    return;
  }
  const view = routes[hash];
  if (!view) { location.hash = '#/'; return; }
  renderLayout(view);
}

function renderLayout(view) {
  const hash = location.hash || '#/';
  const items = NAV_ITEMS.filter(([, , admin]) => !admin || state.user.is_admin);
  $app.innerHTML = `
    <header class="topnav">
      <a class="logo" href="#/">${icon('logo')}<span>RustDesk 管理后台</span></a>
      <nav>${items.map(([h, t]) => `<a class="${h === hash ? 'active' : ''}" href="${h}">${t}</a>`).join('')}</nav>
      <div class="right">
        <button class="icon-btn" id="theme-toggle" title="切换浅色/深色">${icon('moon')}</button>
        <span class="who">${esc(state.user.username)}${state.user.is_admin ? ' · 管理员' : ''}</span>
        <a class="signout" href="javascript:void 0" id="logout-link">${icon('signout')}退出</a>
      </div>
    </header>
    <div class="container" id="main"></div>`;
  updateThemeIcon();
  document.getElementById('logout-link').onclick = async () => {
    await api('/admin/api/logout', { method: 'POST' });
    state.user = null;
    location.hash = '#/login';
  };
  view(document.getElementById('main'));
}

/* ---------- 视图：登录 / 注册 ---------- */

function viewLogin() {
  return `
    <div class="auth-wrap">
      <div class="auth-logo">${icon('logo').replace('width="22" height="22"', 'width="40" height="40"')}</div>
      <h1>登录 RustDesk 管理后台</h1>
      <div class="auth-box">
        <div class="form-row"><label>用户名</label><input type="text" id="f-user" autocomplete="username"></div>
        <div class="form-row"><label>密码</label><input type="password" id="f-pwd" autocomplete="current-password"></div>
        <button class="btn btn-primary btn-block" id="f-submit">Sign in</button>
        <div class="form-msg" id="f-msg"></div>
      </div>
      <div class="auth-links">没有账号？<a href="#/register">注册一个</a></div>
      <div class="auth-tip">rustdesk-api-go · Go rewrite of kingmo888/rustdesk-api-server</div>
    </div>`;
}

function viewRegister() {
  return `
    <div class="auth-wrap">
      <div class="auth-logo">${icon('logo').replace('width="22" height="22"', 'width="40" height="40"')}</div>
      <h1>注册账号</h1>
      <div class="auth-box">
        <div class="form-row"><label>用户名（至少 4 个字符）</label><input type="text" id="f-user"></div>
        <div class="form-row"><label>密码（8~20 位）</label><input type="password" id="f-pwd"></div>
        <button class="btn btn-primary btn-block" id="f-submit">注 册</button>
        <div class="form-msg" id="f-msg"></div>
      </div>
      <div class="auth-links">首个注册的用户将成为管理员 · <a href="#/login">去登录</a></div>
    </div>`;
}

function bindAuthForm(isLogin) {
  const submit = document.getElementById('f-submit');
  const msg = document.getElementById('f-msg');
  const show = (text, ok) => { msg.textContent = text; msg.className = 'form-msg ' + (ok ? 'ok' : 'error'); };
  submit.onclick = async () => {
    const user = document.getElementById('f-user').value.trim();
    const pwd = document.getElementById('f-pwd').value;
    if (!user || !pwd) return show('请输入用户名和密码', false);
    try {
      const data = isLogin
        ? await api('/admin/api/login', { method: 'POST', body: JSON.stringify({ username: user, password: pwd }) })
        : await api('/admin/api/register', { method: 'POST', body: JSON.stringify({ user, pwd }) });
      if (isLogin) {
        if (data.code === 1 && data.user) {
          state.user = data.user;
          state.all = !!data.user.is_admin; // 管理员默认查看全部终端
          location.hash = '#/';
        } else show(data.msg || data.error || '登录失败', false);
      } else {
        if (data.code === 1) { show('注册成功，请登录', true); setTimeout(() => location.hash = '#/login', 800); }
        else show(data.msg || '注册失败', false);
      }
    } catch (e) { show(e.message === 'Failed to fetch' ? '网络错误' : '请求失败', false); }
  };
  ['f-user', 'f-pwd'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.addEventListener('keydown', e => { if (e.key === 'Enter') submit.click(); });
  });
}

/* ---------- 视图：设备 ---------- */

async function viewDevices(main) {
  main.innerHTML = `
    <div class="pagehead"><h1>设备</h1><span class="hint">心跳驱动的在线状态与资产清单</span></div>
    <div id="conflict-alert"></div>
    <div class="stat-grid" id="stat-cards"></div>
    <div class="box">
      <div class="box-body" style="padding-bottom:8px">
        <div class="toolbar">
          <input type="text" id="q" placeholder="搜索 RID / 主机名 / IP" value="${esc(state.keyword)}">
          <button class="btn" id="btn-search">搜索</button>
          ${state.user.is_admin
            ? `<label class="checkbox-row"><input type="checkbox" id="all" ${state.all ? 'checked' : ''}> 全部用户设备</label>`
            : ''}
          <div class="spacer"></div>
          ${state.user.is_admin ? '<a class="btn" href="/admin/api/devices/export">导出 Excel</a>' : ''}
        </div>
      </div>
      <div class="table-wrap"><table>
        <thead><tr>
          <th>RID</th><th>别名</th><th>主机名</th><th>系统用户</th><th>平台</th>
          <th>CPU</th><th>内存</th><th>版本</th><th>IP</th>${state.all ? '<th>所属用户</th>' : ''}
          <th>状态</th><th>最近更新</th>
        </tr></thead>
        <tbody id="rows"><tr><td colspan="12" class="empty">加载中…</td></tr></tbody>
      </table></div>
      <div class="pagination" id="pager"></div>
    </div>`;

  const reload = async () => {
    const q = encodeURIComponent(state.keyword);
    const data = await api(`/admin/api/devices?page=${state.page}&size=${state.size}&q=${q}${state.all ? '&all=1' : ''}`);
    state.total = data.total;
    document.getElementById('rows').innerHTML = data.items.length ? data.items.map(d => `
      <tr>
        <td class="mono">${esc(d.rid)}${d.conflict ? ' <span class="label-badge danger" title="该 ID 被多台机器重复使用"><span class="dot"></span>重复</span>' : ''}</td>
        <td>${esc(d.alias) || '<span style="color:var(--fg-muted)">-</span>'}</td>
        <td>${esc(d.hostname) || '-'}</td>
        <td>${esc(d.username) || '-'}</td>
        <td>${esc(d.platform) || '-'}</td>
        <td title="${esc(d.cpu)}">${esc(truncate(d.cpu, 28))}</td>
        <td>${esc(d.memory) || '-'}</td>
        <td class="mono">${esc(d.version) || '-'}</td>
        <td class="mono">${esc(d.ip_address) || '-'}</td>
        ${state.all ? `<td>${esc(d.owner)}</td>` : ''}
        <td><span class="label-badge ${d.status === 'online' ? 'online' : ''}"><span class="dot"></span>${d.status === 'online' ? '在线' : '离线'}</span></td>
        <td>${fmtTime(d.update_time)}</td>
      </tr>`).join('') : `<tr><td colspan="12" class="empty">${state.user.is_admin && !state.all
        ? '管理视图已切换为「我的设备」，但你名下没有绑定设备 — 勾选上方「全部用户设备」查看全部终端'
        : '暂无设备 — 客户端配置好 API 地址并登录后，设备会出现在这里'}</td></tr>`;
    renderPager(document.getElementById('pager'), data, reload);
  };

  document.getElementById('btn-search').onclick = () => {
    state.keyword = document.getElementById('q').value.trim();
    state.page = 1; reload();
  };
  document.getElementById('q').addEventListener('keydown', e => { if (e.key === 'Enter') document.getElementById('btn-search').click(); });
  const allBox = document.getElementById('all');
  if (allBox) allBox.onchange = () => { state.all = allBox.checked; state.page = 1; reload(); };

  loadStats(document.getElementById('stat-cards'));
  reload();
}

async function loadStats(el) {
  const s = await api('/admin/api/stats');
  const alert = document.getElementById('conflict-alert');
  if (alert) {
    alert.innerHTML = s.id_conflicts > 0 ? `
      <div class="box" style="border-color:var(--danger);margin-bottom:16px">
        <div class="box-body" style="display:flex;align-items:center;gap:12px">
          <span class="label-badge danger" style="flex:none"><span class="dot"></span>需要处理</span>
          <div style="flex:1">
            检测到 <b>${s.id_conflicts}</b> 个远程 ID 被多台机器重复使用。这会直接导致远程连接提示
            「无法验证端到端加密」，并可能连到错误的机器。
          </div>
          <a class="btn btn-sm" href="#/conflicts">查看详情</a>
        </div>
      </div>` : '';
  }
  el.innerHTML = `
    <div class="stat-card"><div class="label">设备总数</div><div class="value">${s.devices_total}</div></div>
    <div class="stat-card"><div class="label">在线设备</div><div class="value green">${s.devices_online}</div></div>
    <div class="stat-card"><div class="label">离线设备</div><div class="value">${s.devices_offline}</div></div>
    <div class="stat-card"><div class="label">在线率</div><div class="value blue">${esc(s.online_rate)}%</div></div>
    <div class="stat-card"><div class="label">用户数</div><div class="value">${s.users_total}</div></div>
    <div class="stat-card"><div class="label">今日连接</div><div class="value">${s.conn_today}</div></div>
    <div class="stat-card"><div class="label">今日文件传输</div><div class="value">${s.files_today}</div></div>`;
}

/* ---------- 视图：日志 ---------- */

async function viewConnLogs(main) {
  main.innerHTML = `
    <div class="pagehead"><h1>连接日志</h1><span class="hint">客户端 audit 上报</span></div>
    <div class="box">
      <div class="table-wrap"><table>
        <thead><tr><th>ID</th><th>动作</th><th>连接 ID</th><th>来源 IP</th><th>来源设备</th><th>目标设备</th><th>开始时间</th><th>结束时间</th><th>Session</th></tr></thead>
        <tbody id="rows"><tr><td colspan="9" class="empty">加载中…</td></tr></tbody>
      </table></div>
      <div class="pagination" id="pager"></div>
    </div>`;
  const reload = async () => {
    const data = await api(`/admin/api/conn_logs?page=${state.page}&size=${state.size}`);
    state.total = data.total;
    document.getElementById('rows').innerHTML = data.items.length ? data.items.map(l => `
      <tr>
        <td>${l.id}</td><td>${esc(l.action)}</td><td class="mono">${esc(l.conn_id)}</td>
        <td class="mono">${esc(l.from_ip)}</td><td class="mono">${esc(l.from_id) || '-'}</td>
        <td class="mono">${esc(l.rid)}</td><td>${fmtTime(l.conn_start)}</td><td>${fmtTime(l.conn_end)}</td>
        <td class="mono">${esc(l.session_id) || '-'}</td>
      </tr>`).join('') : '<tr><td colspan="9" class="empty">暂无日志</td></tr>';
    renderPager(document.getElementById('pager'), data, reload);
  };
  reload();
}

async function viewFileLogs(main) {
  main.innerHTML = `
    <div class="pagehead"><h1>文件日志</h1><span class="hint">文件传输审计</span></div>
    <div class="box">
      <div class="table-wrap"><table>
        <thead><tr><th>ID</th><th>文件</th><th>远端设备</th><th>用户设备</th><th>用户 IP</th><th>大小</th><th>方向</th><th>时间</th></tr></thead>
        <tbody id="rows"><tr><td colspan="8" class="empty">加载中…</td></tr></tbody>
      </table></div>
      <div class="pagination" id="pager"></div>
    </div>`;
  const reload = async () => {
    const data = await api(`/admin/api/file_logs?page=${state.page}&size=${state.size}`);
    state.total = data.total;
    document.getElementById('rows').innerHTML = data.items.length ? data.items.map(l => `
      <tr>
        <td>${l.id}</td><td title="${esc(l.file)}">${esc(truncate(l.file, 50))}</td>
        <td class="mono">${esc(l.remote_id)}</td><td class="mono">${esc(l.user_id)}</td>
        <td class="mono">${esc(l.user_ip)}</td><td>${esc(l.filesize)}</td>
        <td>${l.direction === 1 ? '发送' : '接收'}</td><td>${fmtTime(l.logged_at)}</td>
      </tr>`).join('') : '<tr><td colspan="8" class="empty">暂无日志</td></tr>';
    renderPager(document.getElementById('pager'), data, reload);
  };
  reload();
}

function renderPager(el, data, reload) {
  const pages = Math.max(1, Math.ceil(data.total / data.size));
  el.innerHTML = `
    <span>共 ${data.total} 条 · 第 ${data.page} / ${pages} 页</span>
    <div class="spacer" style="flex:1"></div>
    <button class="btn btn-sm" ${data.page <= 1 ? 'disabled' : ''} id="pg-prev">上一页</button>
    <button class="btn btn-sm" ${data.page >= pages ? 'disabled' : ''} id="pg-next">下一页</button>`;
  const prev = document.getElementById('pg-prev');
  const next = document.getElementById('pg-next');
  if (prev) prev.onclick = () => { state.page = Math.max(1, state.page - 1); reload(); };
  if (next) next.onclick = () => { state.page = Math.min(pages, state.page + 1); reload(); };
}

/* ---------- 视图：分享 ---------- */

async function viewShare(main) {
  main.innerHTML = `
    <div class="pagehead"><h1>分享</h1><span class="hint">把设备地址簿条目分享给其他用户</span></div>
    <div class="box">
      <div class="box-header">创建分享链接<span class="sub">15 分钟内有效，仅可领取一次</span></div>
      <div class="box-body">
        <div class="toolbar">
          <select id="share-peers" multiple size="6" style="min-width:320px;flex:1"><option value="">加载中…</option></select>
        </div>
        <button class="btn btn-primary" id="btn-share">生成链接</button>
        <span style="font-size:12.5px;color:var(--fg-muted)">按住 Ctrl / Cmd 多选</span>
      </div>
    </div>
    <div class="box">
      <div class="box-header">我的分享链接</div>
      <div class="table-wrap"><table>
        <thead><tr><th>链接 Key</th><th>设备</th><th>状态</th><th>创建时间</th><th>操作</th></tr></thead>
        <tbody id="share-rows"><tr><td colspan="5" class="empty">加载中…</td></tr></tbody>
      </table></div>
    </div>
    <div class="box">
      <div class="box-header">领取他人的分享</div>
      <div class="box-body">
        <div class="toolbar">
          <input type="text" id="claim-hash" placeholder="输入分享 Key">
          <button class="btn" id="btn-claim">领 取</button>
        </div>
      </div>
    </div>`;

  const peersData = await api('/admin/api/devices?page=1&size=200');
  const sel = document.getElementById('share-peers');
  sel.innerHTML = peersData.items.map(d =>
    `<option value="${esc(d.rid)}">${esc(d.rid)}${d.alias ? ' · ' + esc(d.alias) : ''}</option>`).join('');

  const loadShares = async () => {
    const data = await api('/admin/api/shares');
    document.getElementById('share-rows').innerHTML = data.items.length ? data.items.map(s => `
      <tr>
        <td class="mono">${esc(s.shash)}</td>
        <td>${s.peers.map(p => `<span class="tag mono">${esc(p)}</span>`).join('')}</td>
        <td>${s.is_used ? '<span class="label-badge attention"><span class="dot"></span>已领取</span>'
          : s.is_expired ? '<span class="label-badge"><span class="dot"></span>已过期</span>'
          : '<span class="label-badge online"><span class="dot"></span>有效</span>'}</td>
        <td>${fmtTime(s.create_time)}</td>
        <td><button class="btn btn-sm" data-copy="${esc(s.shash)}">${icon('copy')}复制</button></td>
      </tr>`).join('') : '<tr><td colspan="5" class="empty">暂无分享链接</td></tr>';
    document.querySelectorAll('[data-copy]').forEach(b => {
      b.onclick = () => copyText(b.dataset.copy, '已复制 Key');
    });
  };
  loadShares();

  document.getElementById('btn-share').onclick = async () => {
    const peers = Array.from(sel.selectedOptions).map(o => o.value);
    if (!peers.length) return toast('请先选择设备');
    const data = await api('/admin/api/shares', { method: 'POST', body: JSON.stringify({ peers }) });
    if (data.code === 1) { toast('已生成：' + data.shash); loadShares(); }
    else flash(data.msg || '生成失败', 'error');
  };
  document.getElementById('btn-claim').onclick = async () => {
    const shash = document.getElementById('claim-hash').value.trim();
    if (!shash) return toast('请输入 Key');
    const data = await api('/admin/api/shares/' + encodeURIComponent(shash) + '/claim', { method: 'POST' });
    if (data.code === 1) flash('领取成功', 'success');
    else flash(data.msg || '领取失败', 'error');
    document.getElementById('claim-hash').value = '';
  };
}

/* ---------- 视图：ID 冲突（管理员） ---------- */

async function viewConflicts(main) {
  const data = await api('/admin/api/id-conflicts');
  main.innerHTML = `
    <div class="pagehead"><h1>ID 冲突</h1><span class="hint">同一远程 ID 被多台机器占用（硬盘克隆后遗症）</span></div>
    <div class="box">
      <div class="box-body">
        <p style="color:var(--fg-muted);font-size:13px;margin:0">
          同一个远程 ID 被两台机器共用时，hbbs 只能保存其中一台的公钥，另一台连接时会提示
          「无法验证端到端加密」。下表是系统检测到的冲突；冲突机器登录时会收到更换 ID 的提示。
        </p>
      </div>
      <div class="table-wrap"><table>
        <thead><tr><th>远程 ID</th><th>原机</th><th>涉及主机名</th><th>观察次数</th><th>已拦截登录</th><th>最近发现</th><th>操作</th></tr></thead>
        <tbody>${data.items.length ? data.items.map(c => `
          <tr>
            <td class="mono">${esc(c.rid)}</td>
            <td>${esc(c.primary_hostname) || '-'}</td>
            <td>${c.hostnames.map(h => `<span class="tag">${esc(h)}</span>`).join('')}</td>
            <td>${c.observations}</td>
            <td>${c.blocked}</td>
            <td>${fmtTime(c.last_seen)}</td>
            <td><button class="btn btn-sm" data-clear="${esc(c.rid)}">清除记录</button>
                <button class="btn btn-sm" data-cmd="${esc(c.primary_hostname)}">复制修复命令</button></td>
          </tr>`).join('') : '<tr><td colspan="7" class="empty">暂无冲突 — 目前没有发现重复的远程 ID</td></tr>'}</tbody>
      </table></div>
    </div>
    <div class="box">
      <div class="box-header">怎么改 ID<span class="sub">新版客户端已隐藏「更改 ID」按钮，用命令行即可</span></div>
      <div class="box-body">
        <p style="margin:0 0 8px">在<strong>要更换 ID 的那台机器</strong>上，以<strong>管理员身份</strong>打开命令提示符，执行：</p>
        ${copyblockHTML('cb-getid', 'rustdesk.exe --get-id')}
        ${copyblockHTML('cb-setid', 'rustdesk.exe --set-id 1234567890')}
        <ul class="note-list">
          <li><code>--get-id</code> 先确认当前 ID；<code>--set-id</code> 改成新的 10 位随机数字（上表「复制修复命令」会生成一个）。</li>
          <li>该命令走的是与界面相同的改 ID 逻辑，无需登录账号；需已安装客户端且有管理员权限。</li>
          <li>若提示 <code>Changing ID is disabled!</code>，说明该客户端是定制版（内置禁用了改 ID），只能重装标准版。</li>
          <li>改完约 10 秒内会以新 ID 注册；确认无误后点「清除记录」。</li>
        </ul>
      </div>
    </div>`;

  // 复制该行的修复命令（随机 10 位 ID）
  bindCopyblocks();
  document.querySelectorAll('[data-cmd]').forEach(b => {
    b.onclick = () => {
      const newId = String(Math.floor(1_000_000_000 + Math.random() * 900_000_000));
      copyText(`rustdesk.exe --set-id ${newId}`, `已复制，新 ID：${newId}`);
    };
  });

  document.querySelectorAll('[data-clear]').forEach(b => {
    b.onclick = async () => {
      if (!confirm('确认清除该远程 ID 的冲突记录？')) return;
      const r = await api('/admin/api/id-conflicts/' + encodeURIComponent(b.dataset.clear), { method: 'DELETE' });
      if (r.code === 1) { flash('已清除', 'success'); viewConflicts(main); }
      else flash(r.msg || '清除失败', 'error');
    };
  });
}

/* ---------- 视图：用户管理（管理员） ---------- */

async function viewUsers(main) {
  main.innerHTML = `
    <div class="pagehead"><h1>用户</h1><span class="hint">平台账号与权限</span></div>
    <div class="box">
      <div class="box-header">新建用户</div>
      <div class="box-body">
        <div class="toolbar">
          <input type="text" id="n-user" placeholder="用户名（至少 4 个字符）">
          <input type="password" id="n-pwd" placeholder="密码（8~20 位）">
          <label class="checkbox-row"><input type="checkbox" id="n-admin"> 管理员</label>
          <button class="btn btn-primary" id="btn-create">创建</button>
        </div>
      </div>
      <div class="table-wrap"><table>
        <thead><tr><th>ID</th><th>用户名</th><th>角色</th><th>状态</th><th>注册时间</th><th>操作</th></tr></thead>
        <tbody id="user-rows"><tr><td colspan="6" class="empty">加载中…</td></tr></tbody>
      </table></div>
    </div>`;

  const load = async () => {
    const data = await api('/admin/api/users');
    document.getElementById('user-rows').innerHTML = data.items.map(u => `
      <tr>
        <td>${u.id}</td><td>${esc(u.username)}</td>
        <td>${u.is_admin ? '<span class="label-badge accent"><span class="dot"></span>管理员</span>' : '<span class="label-badge"><span class="dot"></span>普通用户</span>'}</td>
        <td>${u.is_active ? '<span class="label-badge online"><span class="dot"></span>正常</span>' : '<span class="label-badge danger"><span class="dot"></span>禁用</span>'}</td>
        <td>${fmtTime(u.created_at)}</td>
        <td>
          <button class="btn btn-sm" data-reset="${u.id}">重置密码</button>
          <button class="btn btn-sm btn-danger" data-del="${u.id}">删除</button>
        </td>
      </tr>`).join('');
    document.querySelectorAll('[data-reset]').forEach(b => {
      b.onclick = async () => {
        const pwd = prompt('为 ' + b.dataset.reset + ' 设置新密码（8~20 位）：');
        if (!pwd) return;
        const r = await api('/admin/api/users/' + b.dataset.reset + '/password', { method: 'POST', body: JSON.stringify({ password: pwd }) });
        if (r.code === 1) flash('已重置', 'success'); else flash(r.msg || '失败', 'error');
      };
    });
    document.querySelectorAll('[data-del]').forEach(b => {
      b.onclick = async () => {
        if (!confirm('确认删除该用户？')) return;
        const r = await api('/admin/api/users/' + b.dataset.del, { method: 'DELETE' });
        if (r.code === 1) { flash('已删除', 'success'); load(); } else flash(r.msg || '失败', 'error');
      };
    });
  };
  load();

  document.getElementById('btn-create').onclick = async () => {
    const username = document.getElementById('n-user').value.trim();
    const password = document.getElementById('n-pwd').value;
    const is_admin = document.getElementById('n-admin').checked;
    if (!username || !password) return toast('请输入用户名和密码');
    const r = await api('/admin/api/users', { method: 'POST', body: JSON.stringify({ username, password, is_admin }) });
    if (r.code === 1) { flash('创建成功', 'success'); document.getElementById('n-user').value = ''; document.getElementById('n-pwd').value = ''; load(); }
    else flash(r.msg || '创建失败', 'error');
  };
}

/* ---------- 视图：服务端配置（管理员） ---------- */

async function viewServer(main) {
  main.innerHTML = `
    <div class="pagehead"><h1>服务端配置</h1><span class="hint">对接你的 hbbs / hbbr 部署，生成客户端配置</span></div>

    <div class="box">
      <div class="box-header">服务端档案<span class="sub">记录 RustDesk 服务端实际部署信息（hbbs / hbbr 的地址与端口由它们的启动参数决定，此处用于对接）</span></div>
      <div class="box-body">
        <div class="form-row">
          <label>ID 服务器（hbbs）</label>
          <input type="text" id="p-id" placeholder="如 rustdesk.example.com 或 1.2.3.4:31000">
          <div class="note">host 或 host:port；端口为 hbbs -p 基准，默认 21116</div>
        </div>
        <div class="form-row">
          <label>中继服务器（hbbr，可选）</label>
          <input type="text" id="p-relay" placeholder="默认同 ID 服务器，端口 21117">
        </div>
        <div class="form-row">
          <label>API 服务器（本服务，可选）</label>
          <input type="text" id="p-api" placeholder="如 rustdesk.example.com:21114">
        </div>
        <div class="form-row">
          <label>服务端公钥（Key，可选）</label>
          <input type="text" id="p-key" class="mono" placeholder="id_ed25519.pub 的内容">
          <div class="note" id="p-key-note">填写后客户端连接方可加密；一体部署时可设置 HBBS_KEY_FILE 自动探测</div>
        </div>
        <div class="toolbar">
          <button class="btn btn-primary" id="btn-save">保存档案</button>
          <button class="btn" id="btn-check">连通性自检</button>
        </div>
        <div id="check-results"></div>
      </div>
    </div>

    <div id="client-config-area"></div>`;

  const profile = await api('/admin/api/server-profile');
  document.getElementById('p-id').value = profile.id_server || '';
  document.getElementById('p-api').value = profile.api_server || '';
  document.getElementById('p-key').value = profile.key || '';
  if (!profile.id_server && profile.suggested_id_server) {
    document.getElementById('p-id').placeholder = '如 ' + profile.suggested_id_server;
  }
  if (profile.detected_key) {
    const note = document.getElementById('p-key-note');
    note.innerHTML = `已探测到公钥（HBBS_KEY_FILE）：<a href="javascript:void 0" id="use-detected">使用它</a>`;
    document.getElementById('use-detected').onclick = () => {
      document.getElementById('p-key').value = profile.detected_key;
      toast('已填入探测到的公钥');
    };
  }

  const refreshConfig = async () => {
    const area = document.getElementById('client-config-area');
    const cc = await api('/admin/api/server-profile/client-config');
    if (cc.code !== 1) {
      area.innerHTML = `
        <div class="box"><div class="box-body" style="color:var(--fg-muted)">
          保存 ID 服务器地址后，这里会生成客户端配置（重命名安装包 / 手动填写 / 端口清单）。
        </div></div>`;
      return;
    }
    area.innerHTML = `
      <div class="box">
        <div class="box-header">客户端配置生成<span class="sub">发给最终用户即可免手敲</span></div>
        <div class="box-body">
          <p style="font-weight:600;margin-bottom:4px">① Windows 重命名法（推荐）</p>
          <p style="color:var(--fg-muted);font-size:13px;margin:0">把下载的 <span class="mono">rustdesk.exe</span> 重命名为：</p>
          ${cc.filename ? copyblockHTML('cb-fn', cc.filename) : '<p style="color:var(--attention)">填写公钥后生成（host 与 key 缺一不可）</p>'}
          ${cc.filename_elevate ? `<p style="color:var(--fg-muted);font-size:13px">自动提权变体（免去 UAC 问题）：</p>${copyblockHTML('cb-qs', cc.filename_elevate)}` : ''}

          <p style="font-weight:600;margin:16px 0 4px">② 客户端界面手动填写</p>
          <div class="table-wrap"><table class="kv-table">
            <tbody>
              <tr><td>ID 服务器</td><td class="mono">${esc(cc.manual.id_server) || '-'}</td></tr>
              <tr><td>中继服务器</td><td class="mono">${esc(cc.manual.relay_server) || '（留空，自动推导）'}</td></tr>
              <tr><td>API 服务器</td><td class="mono">${esc(cc.manual.api_server) || '（留空）'}</td></tr>
              <tr><td>Key</td><td class="mono">${esc(cc.manual.key) || '（留空则连接不加密）'}</td></tr>
            </tbody>
          </table></div>

          <p style="font-weight:600;margin:16px 0 4px">③ 防火墙 / 端口放行清单</p>
          <div class="table-wrap"><table>
            <thead><tr><th>服务</th><th>协议</th><th>端口</th><th>用途</th></tr></thead>
            <tbody>${cc.ports.map(p => `<tr><td>${esc(p.service)}</td><td>${esc(p.proto)}</td><td class="mono">${p.port}</td><td>${esc(p.usage)}</td></tr>`).join('')}</tbody>
          </table></div>

          <p style="font-weight:600;margin:16px 0 4px">④ 编译专属客户端（GitHub Actions，可选）</p>
          <p style="color:var(--fg-muted);font-size:13px;margin:0">Fork rustdesk 主仓库后配置以下 Secrets，即可编译出内置服务器的安装包：</p>
          ${copyblockHTML('cb-rs', 'RENDEZVOUS_SERVER=' + cc.custom_build.RENDEZVOUS_SERVER)}
          ${cc.custom_build.RS_PUB_KEY ? copyblockHTML('cb-pk', 'RS_PUB_KEY=' + cc.custom_build.RS_PUB_KEY) : ''}

          <ul class="note-list">${cc.notes.map(n => `<li>${esc(n)}</li>`).join('')}</ul>
        </div>
      </div>`;
    bindCopyblocks();
  };

  document.getElementById('btn-save').onclick = async () => {
    const body = {
      id_server: document.getElementById('p-id').value.trim(),
      relay_server: document.getElementById('p-relay').value.trim(),
      api_server: document.getElementById('p-api').value.trim(),
      key: document.getElementById('p-key').value.trim(),
    };
    const r = await api('/admin/api/server-profile', { method: 'PUT', body: JSON.stringify(body) });
    if (r.code === 1) { flash('档案已保存', 'success'); refreshConfig(); }
    else flash(r.msg || '保存失败', 'error');
  };

  document.getElementById('btn-check').onclick = async () => {
    document.getElementById('btn-save').click();
    const box = document.getElementById('check-results');
    box.innerHTML = '<div style="color:var(--fg-muted);padding:8px 0">检测中…</div>';
    try {
      const r = await api('/admin/api/server-profile/check', { method: 'POST' });
      if (r.code !== 1) { box.innerHTML = ''; return flash(r.msg || '无法检测', 'error'); }
      box.innerHTML = r.results.map(it => `
        <div class="check-item">
          <span class="label-badge ${it.ok ? 'online' : 'danger'}"><span class="dot"></span>${it.ok ? '可达' : '不可达'}</span>
          <span class="addr mono">${esc(it.addr)}</span>
          <span>${esc(it.name)}</span>
          <span class="latency">${it.ok ? esc(it.latency) : ''}</span>
        </div>`).join('');
    } catch (e) { box.innerHTML = ''; }
  };

  refreshConfig();
}

/* ---------- 启动 ---------- */

window.addEventListener('hashchange', () => {
  state.page = 1;
  router();
});

(async function boot() {
  try { state.user = await api('/admin/api/me'); } catch { state.user = null; }
  // 管理员默认查看全部终端，避免"登录后一片空白"的误解
  state.all = !!(state.user && state.user.is_admin);
  router();
})();
