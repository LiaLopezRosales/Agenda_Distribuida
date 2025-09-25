(() => {
  const state = { token: null, ws: null, notifications: [] };
  const api = (path, opts = {}) => {
    const headers = { 'Content-Type': 'application/json', ...(opts.headers || {}) };
    if (state.token) headers['Authorization'] = `Bearer ${state.token}`;
    return fetch(path, { ...opts, headers }).then(async (r) => {
      const text = await r.text();
      let body;
      try { body = text ? JSON.parse(text) : null; } catch { body = text; }
      if (!r.ok) { throw new Error(typeof body === 'string' ? body : JSON.stringify(body)); }
      return body;
    });
  };

  const $ = (id) => document.getElementById(id);
  const setJSON = (id, data) => { const el = $(id); if (el) el.textContent = JSON.stringify(data, null, 2); };
  const setToken = (t) => {
    state.token = t;
    const el = $('jwtPreview'); if (el) el.textContent = t ? `${t.slice(0,16)}...` : '(none)';
    connectWS();
  };

  function connectWS() {
    try { if (state.ws) state.ws.close(); } catch {}
    if (!state.token) return;
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const url = `${proto}://${location.host}/ws?token=${encodeURIComponent(state.token)}`;
    const ws = new WebSocket(url);
    state.ws = ws;
    ws.onopen = () => { console.log('ws open'); };
    ws.onmessage = (ev) => {
      const lines = ev.data.split('\n');
      for (const line of lines) {
        if (!line) continue;
        try { state.notifications.unshift(JSON.parse(line)); }
        catch { state.notifications.unshift({ type: 'raw', payload: line }); }
      }
      renderNotifications();
    };
    ws.onclose = () => { console.log('ws closed'); };
    ws.onerror = (e) => { console.log('ws error', e); };
  }

  function renderNotifications() {
    setJSON('notif_list', state.notifications.slice(0, 50));
  }

  // Auth
  $('btn_register').onclick = async () => {
    try {
      const body = {
        username: $('reg_username').value,
        email: $('reg_email').value,
        password: $('reg_password').value,
        name: $('reg_name').value,
      };
      const res = await api('/register', { method: 'POST', body: JSON.stringify(body) });
      setJSON('auth_result', res);
    } catch (e) { setJSON('auth_result', { error: String(e) }); }
  };

  $('btn_login').onclick = async () => {
    try {
      const body = { username: $('login_username').value, password: $('login_password').value };
      const res = await api('/login', { method: 'POST', body: JSON.stringify(body) });
      setToken(res.token);
      setJSON('auth_result', res);
    } catch (e) { setJSON('auth_result', { error: String(e) }); }
  };

  // Groups
  $('btn_create_group').onclick = async () => {
    try {
      const body = { name: $('grp_name').value, description: $('grp_desc').value };
      const res = await api('/api/groups', { method: 'POST', body: JSON.stringify(body) });
      setJSON('group_result', res);
    } catch (e) { setJSON('group_result', { error: String(e) }); }
  };

  $('btn_add_member').onclick = async () => {
    try {
      const gid = $('add_group_id').value;
      const body = { user_id: Number($('add_user_id').value), rank: Number($('add_rank').value) };
      const res = await api(`/api/groups/${gid}/members`, { method: 'POST', body: JSON.stringify(body) });
      setJSON('member_result', res);
    } catch (e) { setJSON('member_result', { error: String(e) }); }
  };

  // Appointments
  $('btn_create_appt').onclick = async () => {
    try {
      const gidRaw = $('appt_group_id').value.trim();
      const body = {
        title: $('appt_title').value,
        description: $('appt_desc').value,
        start: $('appt_start').value,
        end: $('appt_end').value,
        privacy: $('appt_privacy').value,
        group_id: gidRaw ? Number(gidRaw) : undefined,
      };
      const res = await api('/api/appointments', { method: 'POST', body: JSON.stringify(body) });
      setJSON('appt_result', res);
    } catch (e) { setJSON('appt_result', { error: String(e) }); }
  };

  // Agenda
  $('btn_my_agenda').onclick = async () => {
    try {
      const start = encodeURIComponent($('ag_start').value);
      const end = encodeURIComponent($('ag_end').value);
      const res = await api(`/api/agenda?start=${start}&end=${end}`);
      setJSON('my_agenda', res);
    } catch (e) { setJSON('my_agenda', { error: String(e) }); }
  };

  $('btn_group_agenda').onclick = async () => {
    try {
      const gid = $('ag_group_id').value;
      const start = encodeURIComponent($('ag_start').value);
      const end = encodeURIComponent($('ag_end').value);
      const res = await api(`/api/groups/${gid}/agenda?start=${start}&end=${end}`);
      setJSON('group_agenda', res);
    } catch (e) { setJSON('group_agenda', { error: String(e) }); }
  };

  // Notifications
  const refreshNotifications = async () => {
    try {
      const items = await api('/api/notifications');
      // avoid duplicates by id if the server sent same via WS
      const byId = new Map(state.notifications.map(n => [n.id, n]));
      for (const it of items) { if (!byId.has(it.id)) state.notifications.push(it); }
      renderNotifications();
    } catch (e) { /* ignore */ }
  };

  const btnFetchAll = $('btn_fetch_notif'); if (btnFetchAll) btnFetchAll.onclick = refreshNotifications;
  const btnFetchUnread = $('btn_fetch_unread'); if (btnFetchUnread) btnFetchUnread.onclick = async () => {
    try { const items = await api('/api/notifications/unread'); setJSON('notif_unread', items); }
    catch (e) { setJSON('notif_unread', { error: String(e) }); }
  };
  const btnMarkRead = $('btn_mark_read'); if (btnMarkRead) btnMarkRead.onclick = async () => {
    try { const id = Number($('notif_id_to_read').value); await api(`/api/notifications/${id}/read`, { method: 'POST' }); await refreshNotifications(); }
    catch (e) { /* ignore */ }
  };
})(); 