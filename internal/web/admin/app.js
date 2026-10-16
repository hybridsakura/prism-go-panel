/* RustDesk 管理后台 SPA（无依赖，hash 路由） */
'use strict';

const state = { user: null, all: false, page: 1, keyword: '', total: 0, size: 15 };

const $app = document.getElementById('app');

/* ---------- 基础工具 ---------- */

function esc(s) {
  return String(s == null ? '' : s).replace(/[&<>"']/g, c => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[c]));
}

function toast(msg) {
  let el = document.querySelector('.toast');
  if (!el) {
    el = document.createElement('div');
    el.className = 'toast';
    document.body.appendChild(el);
  }
  el.textContent = msg;
  el.classList.add('show');
  clearTimeout(el._timer);
  el._timer = setTimeout(() => el.classList.remove('show'), 2200);
}

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
    toast(data.error || '需要管理员权限');
    throw new Error(data.error || 'forbidden');
  }
  return res.json();
}

function fmtTime(s) { return s ? s.replace('T', ' ').replace(/\+.*$/, '') : '-'; }

/* ---------- 路由 ---------- */

const routes = {
  '/': viewDevices,
  '/conn-logs': viewConnLogs,
  '/file-logs': viewFileLogs,
  '/share': viewShare,
  '/users': viewUsers,
};

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
    render(hash === '/register' ? viewRegister() : viewLogin());
    return;
  }
  const view = routes[hash];
  if (!view) { location.hash = '#/'; return; }
  renderLayout(view);
}

function render(html) { $app.innerHTML = html; }

function renderLayout(view) {
  const items = [
    ['#/', '设备管理'],
    ['#/conn-logs', '连接日志'],
    ['#/file-logs', '文件日志'],
    ['#/share', '分享管理'],
  ];
  if (state.user.is_admin) items.push(['#/users', '用户管理']);
  const hash = location.hash || '#/';
  $app.innerHTML = `
    <div class="layout">
      <aside class="sidebar">
        <div class="brand">RustDesk 管理后台<small>rustdesk-api-go</small></div>
        ${items.map(([h, t]) => `<a class="nav-item ${h === hash ? 'active' : ''}" href="${h}">${t}</a>`).join('')}
        <div class="footer">
          ${esc(state.user.username)}${state.user.is_admin ? '（管理员）' : ''}<br>
          <a href="javascript:void 0" id="logout-link">退出登录</a>
        </div>
      </aside>
      <main class="main" id="main"></main>
    </div>`;
  document.getElementById('logout-link').onclick = async () => {
    await api('/admin/api/logout', { method: 'POST' });
    state.user = null;
    document.body.classList.remove('is-admin');
    location.hash = '#/login';
  };
  view(document.getElementById('main'));
}

/* ---------- 视图：登录 / 注册 ---------- */

function viewLogin() {
  return `
    <div class="login-box">
      <h1>登录</h1>
      <div class="sub">RustDesk API 服务器管理后台</div>
      <div class="form-row"><label>用户名</label><input type="text" id="f-user" autocomplete="username"></div>
      <div class="form-row"><label>密码</label><input type="password" id="f-pwd" autocomplete="current-password"></div>
      <button class="btn btn-primary" style="width:100%" id="f-submit">登 录</button>
      <div class="form-msg" id="f-msg"></div>
      <div class="switch">没有账号？<a href="#/register">注册一个</a></div>
    </div>`;
}

function viewRegister() {
  return `
    <div class="login-box">
      <h1>注册</h1>
      <div class="sub">首个注册的用户将成为管理员</div>
      <div class="form-row"><label>用户名（至少 4 个字符）</label><input type="text" id="f-user"></div>
      <div class="form-row"><label>密码（8~20 位）</label><input type="password" id="f-pwd"></div>
      <button class="btn btn-primary" style="width:100%" id="f-submit">注 册</button>
      <div class="form-msg" id="f-msg"></div>
      <div class="switch">已有账号？<a href="#/login">去登录</a></div>
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
        state.user = data.user;
        document.body.classList.toggle('is-admin', !!data.user.is_admin);
        location.hash = '#/';
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

/* ---------- 视图：设备管理 ---------- */

async function viewDevices(main) {
  main.innerHTML = `
    <div class="page-title">设备管理</div>
    <div class="cards" id="stat-cards"><div class="card"><div class="label">加载中…</div></div></div>
    <div class="panel">
      <div class="toolbar">
        <input type="text" id="q" placeholder="搜索 RID / 主机名 / IP" value="${esc(state.keyword)}">
        <button class="btn" id="btn-search">搜索</button>
        ${state.user.is_admin
          ? `<label style="font-size:13px;color:var(--text-2)"><input type="checkbox" id="all" ${state.all ? 'checked' : ''}> 查看全部设备</label>`
          : ''}
        <div class="spacer"></div>
        ${state.user.is_admin ? '<a class="btn" href="/admin/api/devices/export">导出 Excel</a>' : ''}
      </div>
      <div class="table-wrap"><table>
        <thead><tr>
          <th>RID</th><th>别名</th><th>主机名</th><th>系统用户</th><th>平台</th>
          <th>CPU</th><th>内存</th><th>版本</th><th>IP</th>${state.all ? '<th>所属用户</th>' : ''}
          <th>状态</th><th>最近更新</th>
        </tr></thead>
        <tbody id="rows"><tr><td colspan="11" class="empty">加载中…</td></tr></tbody>
      </table></div>
      <div class="pagination" id="pager"></div>
    </div>`;

  const reload = async () => {
    const q = encodeURIComponent(state.keyword);
    const data = await api(`/admin/api/devices?page=${state.page}&size=${state.size}&q=${q}${state.all ? '&all=1' : ''}`);
    state.total = data.total;
    document.getElementById('rows').innerHTML = data.items.length ? data.items.map(d => `
      <tr>
        <td class="mono">${esc(d.rid)}</td>
        <td>${esc(d.alias) || '<span style="color:var(--text-2)">-</span>'}</td>
        <td>${esc(d.hostname) || '-'}</td>
        <td>${esc(d.username) || '-'}</td>
        <td>${esc(d.platform) || '-'}</td>
        <td title="${esc(d.cpu)}">${esc(truncate(d.cpu, 28))}</td>
        <td>${esc(d.memory) || '-'}</td>
        <td class="mono">${esc(d.version) || '-'}</td>
        <td class="mono">${esc(d.ip_address) || '-'}</td>
        ${state.all ? `<td>${esc(d.owner)}</td>` : ''}
        <td><span class="badge badge-${d.status}">${d.status === 'online' ? '在线' : '离线'}</span></td>
        <td>${fmtTime(d.update_time)}</td>
      </tr>`).join('') : '<tr><td colspan="11" class="empty">暂无设备</td></tr>';
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

function truncate(s, n) { s = String(s || ''); return s.length > n ? s.slice(0, n) + '…' : s; }

async function loadStats(el) {
  const s = await api('/admin/api/stats');
  el.innerHTML = `
    <div class="card"><div class="label">设备总数</div><div class="value">${s.devices_total}</div></div>
    <div class="card"><div class="label">在线设备</div><div class="value green">${s.devices_online}</div></div>
    <div class="card"><div class="label">离线设备</div><div class="value">${s.devices_offline}</div></div>
    <div class="card"><div class="label">在线率</div><div class="value">${esc(s.online_rate)}%</div></div>
    <div class="card"><div class="label">用户数</div><div class="value">${s.users_total}</div></div>
    <div class="card"><div class="label">今日连接</div><div class="value">${s.conn_today}</div></div>
    <div class="card"><div class="label">今日文件传输</div><div class="value">${s.files_today}</div></div>`;
}

/* ---------- 视图：日志 ---------- */

async function viewConnLogs(main) {
  main.innerHTML = `
    <div class="page-title">连接日志</div>
    <div class="panel">
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
    <div class="page-title">文件日志</div>
    <div class="panel">
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

/* ---------- 视图：分享管理 ---------- */

async function viewShare(main) {
  main.innerHTML = `
    <div class="page-title">分享管理</div>
    <div class="panel">
      <h3>创建分享链接（15 分钟内有效，仅可领取一次）</h3>
      <div class="toolbar">
        <select id="share-peers" multiple size="6" style="min-width:320px;flex:1"><option value="">加载中…</option></select>
      </div>
      <button class="btn btn-primary" id="btn-share">生成链接</button>
      <span style="font-size:12px;color:var(--text-2)">按住 Ctrl / Cmd 多选</span>
    </div>
    <div class="panel">
      <h3>我的分享链接</h3>
      <div class="table-wrap"><table>
        <thead><tr><th>链接 Key</th><th>设备</th><th>状态</th><th>创建时间</th><th>操作</th></tr></thead>
        <tbody id="share-rows"><tr><td colspan="5" class="empty">加载中…</td></tr></tbody>
      </table></div>
    </div>
    <div class="panel">
      <h3>领取他人的分享</h3>
      <div class="toolbar">
        <input type="text" id="claim-hash" placeholder="输入分享 Key">
        <button class="btn" id="btn-claim">领 取</button>
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
        <td>${s.is_used ? '<span class="badge badge-used">已领取</span>'
          : s.is_expired ? '<span class="badge badge-expired">已过期</span>'
          : '<span class="badge badge-active">有效</span>'}</td>
        <td>${fmtTime(s.create_time)}</td>
        <td><button class="btn btn-sm" data-copy="${esc(s.shash)}">复制 Key</button></td>
      </tr>`).join('') : '<tr><td colspan="5" class="empty">暂无分享链接</td></tr>';
    document.querySelectorAll('[data-copy]').forEach(b => {
      b.onclick = () => { navigator.clipboard.writeText(b.dataset.copy).then(() => toast('已复制')); };
    });
  };
  loadShares();

  document.getElementById('btn-share').onclick = async () => {
    const peers = Array.from(sel.selectedOptions).map(o => o.value);
    if (!peers.length) return toast('请先选择设备');
    const data = await api('/admin/api/shares', { method: 'POST', body: JSON.stringify({ peers }) });
    if (data.code === 1) { toast('已生成：' + data.shash); loadShares(); }
    else toast(data.msg || '生成失败');
  };
  document.getElementById('btn-claim').onclick = async () => {
    const shash = document.getElementById('claim-hash').value.trim();
    if (!shash) return toast('请输入 Key');
    const data = await api('/admin/api/shares/' + encodeURIComponent(shash) + '/claim', { method: 'POST' });
    toast(data.msg || (data.code === 1 ? '领取成功' : '领取失败'));
    document.getElementById('claim-hash').value = '';
  };
}

/* ---------- 视图：用户管理（管理员） ---------- */

async function viewUsers(main) {
  main.innerHTML = `
    <div class="page-title">用户管理</div>
    <div class="panel">
      <h3>新建用户</h3>
      <div class="toolbar">
        <input type="text" id="n-user" placeholder="用户名（至少 4 个字符）">
        <input type="password" id="n-pwd" placeholder="密码（8~20 位）">
        <label style="font-size:13px"><input type="checkbox" id="n-admin"> 管理员</label>
        <button class="btn btn-primary" id="btn-create">创建</button>
      </div>
    </div>
    <div class="panel">
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
        <td>${u.is_admin ? '<span class="badge badge-admin">管理员</span>' : '<span class="badge badge-offline">普通用户</span>'}</td>
        <td>${u.is_active ? '<span class="badge badge-online">正常</span>' : '<span class="badge badge-expired">禁用</span>'}</td>
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
        toast(r.code === 1 ? '已重置' : (r.msg || '失败'));
      };
    });
    document.querySelectorAll('[data-del]').forEach(b => {
      b.onclick = async () => {
        if (!confirm('确认删除该用户？')) return;
        const r = await api('/admin/api/users/' + b.dataset.del, { method: 'DELETE' });
        if (r.code === 1) { toast('已删除'); load(); } else toast(r.msg || '失败');
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
    if (r.code === 1) { toast('创建成功'); document.getElementById('n-user').value = ''; document.getElementById('n-pwd').value = ''; load(); }
    else toast(r.msg || '创建失败');
  };
}

/* ---------- 启动 ---------- */

window.addEventListener('hashchange', () => {
  state.page = 1;
  router();
  bindForms();
});

function bindForms() {
  const hash = location.hash.slice(1) || '/';
  if (hash === '/login') bindAuthForm(true);
  if (hash === '/register') bindAuthForm(false);
}

(async function boot() {
  try { state.user = await api('/admin/api/me'); } catch { state.user = null; }
  document.body.classList.toggle('is-admin', !!(state.user && state.user.is_admin));
  router();
  bindForms();
})();
