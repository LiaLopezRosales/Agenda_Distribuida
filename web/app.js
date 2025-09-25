(() => {
  const state = { 
    token: null, 
    ws: null, 
    notifications: [],
    currentDate: new Date(),
    events: [],
    groups: [],
    user: null
  };

  // API helper
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

  // DOM helpers
  const $ = (id) => document.getElementById(id);
  const $$ = (selector) => document.querySelectorAll(selector);

  // Initialize app
  function init() {
    setupEventListeners();
    loadStoredAuth();
    renderCalendar();
    loadGroups();
  }

  // Event listeners
  function setupEventListeners() {
    // Auth
    $('loginBtn').onclick = () => showAuthModal('login');
    $('registerBtn').onclick = () => showAuthModal('register');
    $('logoutBtn').onclick = logout;
    
    // Auth modal
    $('closeAuthModal').onclick = hideAuthModal;
    $$('.auth-tab').forEach(tab => {
      tab.onclick = () => switchAuthTab(tab.dataset.tab);
    });
    
    // Password toggles
    $('toggleLoginPassword').onclick = () => togglePassword('loginPassword', 'toggleLoginPassword');
    $('toggleRegPassword').onclick = () => togglePassword('regPassword', 'toggleRegPassword');
    
    $('loginForm').onsubmit = handleLogin;
    $('registerForm').onsubmit = handleRegister;
    
    // Calendar navigation
    $('prevMonth').onclick = () => changeMonth(-1);
    $('nextMonth').onclick = () => changeMonth(1);
    $('todayBtn').onclick = goToToday;
    
    // View switcher
    $$('.view-btn').forEach(btn => {
      btn.onclick = () => switchView(btn.dataset.view);
    });
    
    // Event creation
    $('createEventBtn').onclick = () => showEventModal();
    $('closeEventModal').onclick = hideEventModal;
    $('cancelEvent').onclick = hideEventModal;
    $('saveEvent').onclick = saveEvent;
    
    // Group creation
  const addGroupBtn = $('addGroupBtn');
  if (addGroupBtn) addGroupBtn.addEventListener('click', () => {
    showGroupModal();
  });
  const closeGroupModalBtn = $('closeGroupModal');
  if (closeGroupModalBtn) closeGroupModalBtn.addEventListener('click', hideGroupModal);
  const cancelGroupBtn = $('cancelGroup');
  if (cancelGroupBtn) cancelGroupBtn.addEventListener('click', hideGroupModal);
  const saveGroupBtn = $('saveGroup');
  if (saveGroupBtn) saveGroupBtn.addEventListener('click', saveGroup);
    
    // Calendar cell clicks
    document.addEventListener('click', (e) => {
      if (e.target.classList.contains('day-cell')) {
        const date = e.target.dataset.date;
        if (date) {
          showEventModal(new Date(date));
        }
      }
    });
  }

  // Auth functions
  function showAuthModal(tab) {
    $('authModal').style.display = 'flex';
    switchAuthTab(tab);
  }

  function hideAuthModal() {
    $('authModal').style.display = 'none';
    
    // Reset password fields to hidden
    const loginPassword = $('loginPassword');
    const regPassword = $('regPassword');
    const toggleLogin = $('toggleLoginPassword');
    const toggleReg = $('toggleRegPassword');
    
    if (loginPassword) {
      loginPassword.type = 'password';
      toggleLogin.querySelector('.eye-icon').innerHTML = `
        <path d="M12 4.5C7 4.5 2.73 7.61 1 12c1.73 4.39 6 7.5 11 7.5s9.27-3.11 11-7.5c-1.73-4.39-6-7.5-11-7.5zM12 17c-2.76 0-5-2.24-5-5s2.24-5 5-5 5 2.24 5 5-2.24 5-5 5zm0-8c-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3-1.34-3-3-3z"/>
      `;
    }
    
    if (regPassword) {
      regPassword.type = 'password';
      toggleReg.querySelector('.eye-icon').innerHTML = `
        <path d="M12 4.5C7 4.5 2.73 7.61 1 12c1.73 4.39 6 7.5 11 7.5s9.27-3.11 11-7.5c-1.73-4.39-6-7.5-11-7.5zM12 17c-2.76 0-5-2.24-5-5s2.24-5 5-5 5 2.24 5 5-2.24 5-5 5zm0-8c-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3-1.34-3-3-3z"/>
      `;
    }
    
    // Reset forms
    $('loginForm').reset();
    $('registerForm').reset();
  }

  function switchAuthTab(tab) {
    $$('.auth-tab').forEach(t => t.classList.remove('active'));
    $$('.auth-form').forEach(f => f.classList.remove('active'));
    
    $(`${tab}Tab`).classList.add('active');
    $(`${tab}Form`).classList.add('active');
  }

  async function handleLogin(e) {
    e.preventDefault();
    try {
      const res = await api('/login', {
        method: 'POST',
        body: JSON.stringify({
          username: $('loginUsername').value,
          password: $('loginPassword').value
        })
      });
      
      setToken(res.token);
      state.user = res.user;
      hideAuthModal();
      updateUI();
      loadEvents();
      loadGroups();
    } catch (error) {
      alert('Login failed: ' + error.message);
    }
  }

  async function handleRegister(e) {
    e.preventDefault();
    try {
      const res = await api('/register', {
        method: 'POST',
        body: JSON.stringify({
          username: $('regUsername').value,
          email: $('regEmail').value,
          password: $('regPassword').value,
          name: $('regName').value
        })
      });
      
      setToken(res.token);
      state.user = res.user;
      hideAuthModal();
      updateUI();
      loadEvents();
      loadGroups();
    } catch (error) {
      alert('Registration failed: ' + error.message);
    }
  }

  function logout() {
    state.token = null;
    state.user = null;
    state.events = [];
    state.groups = [];
    localStorage.removeItem('token');
    updateUI();
    renderCalendar();
    updateGroupList();
    updateEventGroupSelect();
  }

  function setToken(token) {
    state.token = token;
    if (token) {
      localStorage.setItem('token', token);
      connectWS();
    }
  }

  function loadStoredAuth() {
    const token = localStorage.getItem('token');
    if (token) {
      state.token = token;
      connectWS();
      loadUserInfo();
    }
  }

  async function loadUserInfo() {
    try {
      // Assuming you have a /me endpoint
      const user = await api('/api/me');
      state.user = user;
      updateUI();
      loadEvents();
      loadGroups();
    } catch (error) {
      console.error('Failed to load user info:', error);
      logout();
    }
  }

  function updateUI() {
    if (state.token && state.user) {
      $('authSection').style.display = 'none';
      $('userInfo').style.display = 'flex';
      $('userName').textContent = state.user.display_name || state.user.username;
    } else {
      $('authSection').style.display = 'flex';
      $('userInfo').style.display = 'none';
    }
  }

  // WebSocket
  function connectWS() {
    try { if (state.ws) state.ws.close(); } catch {}
    if (!state.token) return;
    
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const url = `${proto}://${location.host}/ws?token=${encodeURIComponent(state.token)}`;
    const ws = new WebSocket(url);
    state.ws = ws;
    
    ws.onopen = () => console.log('WebSocket connected');
    ws.onmessage = (ev) => {
      const lines = ev.data.split('\n');
      for (const line of lines) {
        if (!line) continue;
        try {
          const notification = JSON.parse(line);
          state.notifications.unshift(notification);
          // Refresh events when we get notifications
          loadEvents();
        } catch (e) {
          console.error('Failed to parse notification:', line);
        }
      }
    };
    ws.onclose = () => console.log('WebSocket disconnected');
    ws.onerror = (e) => console.log('WebSocket error:', e);
  }

  // Calendar functions
  function renderCalendar() {
    const calendarGrid = $('calendarGrid');
    calendarGrid.innerHTML = '';
    
    // Day headers
    const dayHeaders = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
    dayHeaders.forEach(day => {
      const header = document.createElement('div');
      header.className = 'day-header';
      header.textContent = day;
      calendarGrid.appendChild(header);
    });
    
    // Get first day of month and calculate starting date
    const year = state.currentDate.getFullYear();
    const month = state.currentDate.getMonth();
    const firstDay = new Date(year, month, 1);
    const startDate = new Date(firstDay);
    startDate.setDate(startDate.getDate() - firstDay.getDay());
    
    // Generate 42 days (6 weeks)
    for (let i = 0; i < 42; i++) {
      const cellDate = new Date(startDate);
      cellDate.setDate(startDate.getDate() + i);
      
      const cell = document.createElement('div');
      cell.className = 'day-cell';
      cell.dataset.date = cellDate.toISOString().split('T')[0];
      
      if (cellDate.getMonth() !== month) {
        cell.classList.add('other-month');
      }
      
      if (isToday(cellDate)) {
        cell.classList.add('today');
      }
      
      const dayNumber = document.createElement('div');
      dayNumber.className = 'day-number';
      dayNumber.textContent = cellDate.getDate();
      cell.appendChild(dayNumber);
      
      // Add events for this day
      const dayEvents = getEventsForDate(cellDate);
      dayEvents.forEach(event => {
        const eventEl = document.createElement('div');
        eventEl.className = `event ${event.group_id ? 'group-event' : ''} ${event.status}`;
        eventEl.textContent = event.title;
        eventEl.title = `${event.title}\n${formatTime(event.start)} - ${formatTime(event.end)}`;
        cell.appendChild(eventEl);
      });
      
      calendarGrid.appendChild(cell);
    }
    
    // Update current date display
    $('currentDate').textContent = state.currentDate.toLocaleDateString('en-US', { 
      month: 'long', 
      year: 'numeric' 
    });
  }

  function getEventsForDate(date) {
    const dateStr = date.toISOString().split('T')[0];
    return state.events.filter(event => {
      const eventDate = new Date(event.start).toISOString().split('T')[0];
      return eventDate === dateStr;
    });
  }

  function isToday(date) {
    const today = new Date();
    return date.toDateString() === today.toDateString();
  }

  function formatTime(dateString) {
    return new Date(dateString).toLocaleTimeString('en-US', { 
      hour: 'numeric', 
      minute: '2-digit',
      hour12: true 
    });
  }

  function changeMonth(direction) {
    state.currentDate.setMonth(state.currentDate.getMonth() + direction);
    renderCalendar();
    loadEvents();
  }

  function goToToday() {
    state.currentDate = new Date();
    renderCalendar();
    loadEvents();
  }

  function switchView(view) {
    $$('.view-btn').forEach(btn => btn.classList.remove('active'));
    $(`[data-view="${view}"]`).classList.add('active');
    // TODO: Implement different views in Phase 2
  }

  // Event management
  function showEventModal(date = null) {
    if (!state.token) {
      showAuthModal('login');
      return;
    }
    
    $('eventModal').classList.add('show');
    
    if (date) {
      const startTime = new Date(date);
      startTime.setHours(9, 0, 0, 0);
      const endTime = new Date(startTime);
      endTime.setHours(10, 0, 0, 0);
      
      $('eventStart').value = formatDateTimeLocal(startTime);
      $('eventEnd').value = formatDateTimeLocal(endTime);
    }
  }

  function hideEventModal() {
    $('eventModal').classList.remove('show');
    $('eventForm').reset();
  }

  function formatDateTimeLocal(date) {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    const hours = String(date.getHours()).padStart(2, '0');
    const minutes = String(date.getMinutes()).padStart(2, '0');
    return `${year}-${month}-${day}T${hours}:${minutes}`;
  }

  async function saveEvent() {
    try {
      const formData = {
        title: $('eventTitle').value,
        description: $('eventDescription').value,
        start: new Date($('eventStart').value).toISOString(),
        end: new Date($('eventEnd').value).toISOString(),
        privacy: $('eventPrivacy').value,
        group_id: $('eventGroup').value ? Number($('eventGroup').value) : undefined
      };
      
      const res = await api('/api/appointments', {
        method: 'POST',
        body: JSON.stringify(formData)
      });
      
      hideEventModal();
      loadEvents();
    } catch (error) {
      alert('Failed to create event: ' + error.message);
    }
  }

  async function loadEvents() {
    if (!state.token) return;
    
    try {
      const start = new Date(state.currentDate.getFullYear(), state.currentDate.getMonth(), 1);
      const end = new Date(state.currentDate.getFullYear(), state.currentDate.getMonth() + 1, 0);
      
      const res = await api(`/api/agenda?start=${start.toISOString()}&end=${end.toISOString()}`);
      state.events = res || [];
      renderCalendar();
      updateEventCounts();
    } catch (error) {
      console.error('Failed to load events:', error);
    }
  }

  async function loadGroups() {
    if (!state.token) return;
    
    try {
      const res = await api('/api/groups'); // GET now lists user's groups
      state.groups = Array.isArray(res) ? res : [];
      updateGroupList();
      updateEventGroupSelect();
    } catch (error) {
      console.error('Failed to load groups:', error);
    }
  }

  function updateGroupList() {
    const container = $('groupCalendars');
    container.innerHTML = '';
    
    state.groups.forEach(group => {
      const item = document.createElement('div');
      item.className = 'calendar-item';
      item.dataset.groupId = group.id;
      
      item.innerHTML = `
        <div class="calendar-color" style="background: #34a853;"></div>
        <span class="calendar-name">${group.name}</span>
        <span class="calendar-count" id="groupCount${group.id}">0</span>
      `;
      
      container.appendChild(item);
    });
  }

  function updateEventGroupSelect() {
    const select = $('eventGroup');
    select.innerHTML = '<option value="">Personal Event</option>';
    
    state.groups.forEach(group => {
      const option = document.createElement('option');
      option.value = group.id;
      option.textContent = group.name;
      select.appendChild(option);
    });
  }

  function showGroupModal() {
    const form = $('groupForm');
    if (form) form.reset();
    const modal = $('groupModal');
    if (!modal) return;
    modal.classList.add('show');
    modal.style.display = 'flex';
  }
  
  function hideGroupModal() {
    const modal = $('groupModal');
    if (!modal) return;
    modal.classList.remove('show');
    modal.style.display = 'none';
  }
  
  async function saveGroup() {
    try {
      if (!state.token) {
        hideGroupModal();
        showAuthModal('login');
        return;
      }

      const name = $('groupName').value.trim();
      const description = $('groupDesc').value.trim();
      if (!name) { alert('Group name is required'); return; }

      await api('/api/groups', {
        method: 'POST',
        body: JSON.stringify({ name, description })
      });

      hideGroupModal();
      await loadGroups();
      updateEventGroupSelect(); // keep the event modal's group list in sync
    } catch (e) {
      alert('Failed to create group: ' + e.message);
    }
  }

  function updateEventCounts() {
    // Update personal count
    const personalCount = state.events.filter(e => !e.group_id).length;
    $('personalCount').textContent = personalCount;
    
    // Update group counts
    state.groups.forEach(group => {
      const groupCount = state.events.filter(e => e.group_id === group.id).length;
      const countEl = $(`groupCount${group.id}`);
      if (countEl) countEl.textContent = groupCount;
    });
  }

  function togglePassword(inputId, buttonId) {
    const input = document.getElementById(inputId);
    const button = document.getElementById(buttonId);
    const eyeIcon = button.querySelector('.eye-icon');
    
    if (input.type === 'password') {
      input.type = 'text';
      // Eye open (slash through it)
      eyeIcon.innerHTML = `
        <path d="M12 7c2.76 0 5 2.24 5 5 0 .65-.13 1.26-.36 1.83l2.92 2.92c1.51-1.26 2.7-2.89 3.43-4.75-1.73-4.39-6-7.5-11-7.5-1.4 0-2.74.25-3.98.7l2.16 2.16C10.74 7.13 11.35 7 12 7zM2 4.27l2.28 2.28.46.46C3.08 8.3 1.78 10.02 1 12c1.73 4.39 6 7.5 11 7.5 1.55 0 3.03-.3 4.38-.84l.42.42L19.73 22 21 20.73 3.27 3 2 4.27zM7.53 9.8l1.55 1.55c-.05.21-.08.43-.08.65 0 1.66 1.34 3 3 3 .22 0 .44-.03.65-.08l1.55 1.55c-.67.33-1.41.53-2.2.53-2.76 0-5-2.24-5-5 0-.79.2-1.53.53-2.2zm4.31-.78l3.15 3.15.02-.16c0-1.66-1.34-3-3-3l-.17.01z"/>
      `;
    } else {
      input.type = 'password';
      // Eye closed (normal eye)
      eyeIcon.innerHTML = `
        <path d="M12 4.5C7 4.5 2.73 7.61 1 12c1.73 4.39 6 7.5 11 7.5s9.27-3.11 11-7.5c-1.73-4.39-6-7.5-11-7.5zM12 17c-2.76 0-5-2.24-5-5s2.24-5 5-5 5 2.24 5 5-2.24 5-5 5zm0-8c-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3-1.34-3-3-3z"/>
      `;
    }
  }

  // Initialize the app
  init();
})();