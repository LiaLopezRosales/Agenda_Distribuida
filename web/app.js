(() => {
  const state = { token: null };
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
  const setJSON = (id, data) => { $(id).textContent = JSON.stringify(data, null, 2); };
  const setToken = (t) => {
    state.token = t;
    $('jwtPreview').textContent = t ? `${t.slice(0,16)}...` : '(none)';
  };

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
})(); 