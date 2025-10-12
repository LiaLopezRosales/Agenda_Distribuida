(() => {
  const state = {
    token: null,
    ws: null,
    notifications: [],
    currentView: 'month',
    currentDate: new Date(),
    selectedDate: null,
    events: [],
    groups: [],
    user: null,
    personalEvents: [],
    viewingFromPersonal: false,
    currentAppointment: null,
    editingAppointment: null,
    currentGroup: null,
    editingGroupField: null,
    currentNotification: null
  };

  const api = (path, opts = {}) => {
    const headers = { 'Content-Type': 'application/json', ...(opts.headers || {}) };
    if (state.token) headers['Authorization'] = `Bearer ${state.token}`;
    // Log request
    console.log('[API] Request:', path, opts);
    return fetch(path, { ...opts, headers })
      .then(async (r) => {
      const text = await r.text();
      let body;
      try { body = text ? JSON.parse(text) : null; } catch { body = text; }
        // Log response
        console.log('[API] Response:', path, 'Status:', r.status, 'Body:', body);
      if (!r.ok) { throw new Error(typeof body === 'string' ? body : JSON.stringify(body)); }
      return body;
      })
      .catch((err) => {
        // Log network/fetch errors
        console.error('[API] Fetch/network error:', path, err, err.stack);
        throw err;
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
    loadNotifications();
    updateUnreadNotificationsCount();
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
    
    // Make current date clickable
    $('currentDate').onclick = showCalendarPicker;
    
    // View switcher - Enhanced with visual feedback
    $$('.view-btn').forEach(btn => {
      console.log('Setting up listener for button:', btn.dataset.view);
      
      // Add hover effects
      btn.addEventListener('mouseenter', function() {
        if (!this.classList.contains('active')) {
          this.style.background = 'rgba(26, 115, 232, 0.08)';
          this.style.color = '#1a73e8';
        }
      });
      
      btn.addEventListener('mouseleave', function() {
        if (!this.classList.contains('active')) {
          this.style.background = 'transparent';
          this.style.color = '#5f6368';
        }
      });
      
      btn.addEventListener('click', function(e) {
        e.preventDefault();
        e.stopPropagation();
        console.log('Button clicked:', this.dataset.view);
        
        // Add click animation
        this.style.transform = 'scale(0.95)';
        setTimeout(() => {
          this.style.transform = '';
        }, 150);
        
        switchView(this.dataset.view);
      });
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
  
  // Group management
  const editGroupNameBtn = $('editGroupNameBtn');
  if (editGroupNameBtn) editGroupNameBtn.addEventListener('click', () => showEditGroupModal('name'));
  const editGroupDescBtn = $('editGroupDescBtn');
  if (editGroupDescBtn) editGroupDescBtn.addEventListener('click', () => showEditGroupModal('description'));
  const deleteGroupBtn = $('deleteGroupBtn');
  if (deleteGroupBtn) deleteGroupBtn.addEventListener('click', deleteGroup);
  const closeEditGroupModalBtn = $('closeEditGroupModal');
  if (closeEditGroupModalBtn) closeEditGroupModalBtn.addEventListener('click', hideEditGroupModal);
  const cancelEditGroupBtn = $('cancelEditGroup');
  if (cancelEditGroupBtn) cancelEditGroupBtn.addEventListener('click', hideEditGroupModal);
  const saveEditGroupBtn = $('saveEditGroup');
  if (saveEditGroupBtn) saveEditGroupBtn.addEventListener('click', saveEditGroup);
  
  // Notification details
  const closeNotificationDetailsModalBtn = $('closeNotificationDetailsModal');
  if (closeNotificationDetailsModalBtn) closeNotificationDetailsModalBtn.addEventListener('click', hideNotificationDetailsModal);
  const closeNotificationDetailsBtn = $('closeNotificationDetails');
  if (closeNotificationDetailsBtn) closeNotificationDetailsBtn.addEventListener('click', hideNotificationDetailsModal);
  const acceptInvitationBtn = $('acceptInvitationBtn');
  if (acceptInvitationBtn) acceptInvitationBtn.addEventListener('click', acceptInvitation);
  const rejectInvitationBtn = $('rejectInvitationBtn');
  if (rejectInvitationBtn) rejectInvitationBtn.addEventListener('click', rejectInvitation);
  
  // Profile settings
  const userNameDisplay = $('userName');
  if (userNameDisplay) userNameDisplay.addEventListener('click', showProfileSettings);
  const closeProfileSettingsModalBtn = $('closeProfileSettingsModal');
  if (closeProfileSettingsModalBtn) closeProfileSettingsModalBtn.addEventListener('click', hideProfileSettings);
  const cancelProfileSettingsBtn = $('cancelProfileSettings');
  if (cancelProfileSettingsBtn) cancelProfileSettingsBtn.addEventListener('click', hideProfileSettings);
  const saveProfileSettingsBtn = $('saveProfileSettings');
  if (saveProfileSettingsBtn) saveProfileSettingsBtn.addEventListener('click', saveProfileSettings);
    
    // Event details
    $('closeEventDetailsModal').onclick = hideEventDetailsModal;
    $('closeEventDetails').onclick = hideEventDetailsModal;
    $('editEventBtn').onclick = editEvent;
    $('deleteEventBtn').onclick = deleteEvent;
    
    // Override the event details close behavior when viewing from Personal
    const originalCloseEventDetails = $('closeEventDetailsModal');
    const originalCloseEventDetails2 = $('closeEventDetails');

    if (originalCloseEventDetails) {
      originalCloseEventDetails.onclick = () => {
        if (state.viewingFromPersonal) {
          // Just hide the event details modal, keep Personal modal open
          hideEventDetailsModal();
        } else {
          // Normal behavior - hide event details modal
          hideEventDetailsModal();
        }
      };
    }

    if (originalCloseEventDetails2) {
      originalCloseEventDetails2.onclick = () => {
        if (state.viewingFromPersonal) {
          // Just hide the event details modal, keep Personal modal open
          hideEventDetailsModal();
        } else {
          // Normal behavior - hide event details modal
          hideEventDetailsModal();
        }
      };
    }
    
    // Calendar cell clicks - Updated to handle all view types
    document.addEventListener('click', (e) => {
      // Handle event clicks for all view types
      if (e.target.classList.contains('event') || 
          e.target.classList.contains('week-event') || 
          e.target.classList.contains('day-event')) {
        e.stopPropagation();
        const appointmentId = e.target.dataset.appointmentId;
        console.log('Event clicked:', e.target.textContent, 'ID:', appointmentId);
        if (appointmentId) {
          showEventDetailsModal(appointmentId);
        }
        return;
      }
      
      // Handle day cell clicks for month view
      if (e.target.classList.contains('day-cell')) {
        const date = e.target.dataset.date;
        if (date) {
          showEventModal(new Date(date));
        }
      }
      
      // Handle time slot clicks for week and day views
      if (e.target.classList.contains('week-time-slot') || 
          e.target.classList.contains('day-time-slot')) {
        const date = e.target.dataset.date;
        if (date) {
          showEventModal(new Date(date));
        }
      }
    });

    // Calendar picker event listeners
    $('closeCalendarPicker').onclick = hideCalendarPicker;
    $('calendarPickerToday').onclick = goToTodayInPicker;
    $('calendarPickerSelect').onclick = applyCalendarSelection;
    $('calendarPrevYear').onclick = () => navigateCalendarPicker('prev-year');
    $('calendarPrevMonth').onclick = () => navigateCalendarPicker('prev-month');
    $('calendarNextMonth').onclick = () => navigateCalendarPicker('next-month');
    $('calendarNextYear').onclick = () => navigateCalendarPicker('next-year');

    // Personal calendar click handler
    const personalCalendar = document.querySelector('[data-calendar="personal"]');
    if (personalCalendar) {
      personalCalendar.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        showPersonalEventsModal();
      });
    }

    // Personal Events Modal
    $('closePersonalEventsModal').onclick = hidePersonalEventsModal;

    // Notifications: expand/collapse and refresh
    const notificationsHeader = $('notificationsHeader');
    if (notificationsHeader) {
      notificationsHeader.addEventListener('click', () => {
        const list = $('notificationsList');
        if (!list) return;
        list.style.display = list.style.display === 'none' ? 'block' : 'none';
      });
    }

    const refreshNotificationsBtn = $('refreshNotificationsBtn');
    if (refreshNotificationsBtn) {
      refreshNotificationsBtn.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        loadNotifications();
        updateUnreadNotificationsCount();
      });
    }

    // Groups: expand/collapse
    const groupsHeader = $('groupsHeader');
    if (groupsHeader) {
      groupsHeader.addEventListener('click', () => {
        const list = $('groupCalendars');
        if (!list) return;
        list.style.display = list.style.display === 'none' ? 'block' : 'none';
      });
    }
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
      loadNotifications();
      updateUnreadNotificationsCount();
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
      loadNotifications();
      updateUnreadNotificationsCount();
    } catch (error) {
      alert('Registration failed: ' + error.message);
    }
  }

  function logout() {
    state.token = null;
    state.user = null;
    state.events = [];
    state.groups = [];
    state.notifications = [];
    localStorage.removeItem('token');
    updateUI();
    renderCalendar();
    updateGroupList();
    updateEventGroupSelect();
    const list = $('notificationsList');
    if (list) list.innerHTML = '';
    const badge = $('unreadNotificationsCount');
    if (badge) badge.textContent = '0';
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
      loadNotifications();
      updateUnreadNotificationsCount();
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
      loadNotifications();
      updateUnreadNotificationsCount();
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
          updateUnreadNotificationsCount();
          renderNotificationsList(); // keep list and badge in sync when live notifications arrive
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
    console.log('=== RENDERING CALENDAR ===');
    console.log('Current view:', state.currentView);
    
    const calendarGrid = $('calendarGrid');
    if (!calendarGrid) {
      console.error('Calendar grid element not found!');
      return;
    }
    
    console.log('Found calendar grid:', calendarGrid);
    
    // Clear the grid
    calendarGrid.innerHTML = '';
    console.log('Grid cleared');
    
    // Set the appropriate class
    calendarGrid.className = `calendar-grid ${state.currentView}-view`;
    console.log('Grid class set to:', calendarGrid.className);
    
    // Render based on view
    if (state.currentView === 'month') {
      console.log('Calling renderMonthView');
      renderMonthView(calendarGrid);
    } else if (state.currentView === 'week') {
      console.log('Calling renderWeekView');
      renderWeekView(calendarGrid);
    } else if (state.currentView === 'day') {
      console.log('Calling renderDayView');
      renderDayView(calendarGrid);
    } else {
      console.error('Unknown view:', state.currentView);
    }
    
    // Update current date display
    updateCurrentDateDisplay();
    console.log('=== RENDERING COMPLETE ===');
  }

  function renderMonthView(calendarGrid) {
    calendarGrid.className = 'calendar-grid month-view';
    
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
        eventEl.dataset.appointmentId = event.id;
        cell.appendChild(eventEl);
      });
      
      calendarGrid.appendChild(cell);
    }
  }

  function renderWeekView(calendarGrid) {
    calendarGrid.className = 'calendar-grid week-view';
    
    // Add time column header
    const timeHeader = document.createElement('div');
    timeHeader.className = 'time-column';
    timeHeader.textContent = '';
    calendarGrid.appendChild(timeHeader);
    
    // Get the start of the week (Sunday)
    const startOfWeek = new Date(state.currentDate);
    startOfWeek.setDate(state.currentDate.getDate() - state.currentDate.getDay());
    
    // Add day headers
    for (let i = 0; i < 7; i++) {
      const dayDate = new Date(startOfWeek);
      dayDate.setDate(startOfWeek.getDate() + i);
      
      const dayHeader = document.createElement('div');
      dayHeader.className = 'week-day-header';
      if (isToday(dayDate)) {
        dayHeader.classList.add('today');
      }
      
      const dayName = document.createElement('div');
      dayName.className = 'week-day-name';
      dayName.textContent = dayDate.toLocaleDateString('en-US', { weekday: 'short' });
      
      const dayNumber = document.createElement('div');
      dayNumber.className = 'week-day-number';
      dayNumber.textContent = dayDate.getDate();
      
      dayHeader.appendChild(dayName);
      dayHeader.appendChild(dayNumber);
      dayHeader.dataset.date = dayDate.toISOString().split('T')[0];
      calendarGrid.appendChild(dayHeader);
    }
    
    // Add time slots (24 hours)
    for (let hour = 0; hour < 24; hour++) {
      // Time label
      const timeSlot = document.createElement('div');
      timeSlot.className = 'time-column';
      timeSlot.textContent = formatHour(hour);
      calendarGrid.appendChild(timeSlot);
      
      // Day slots for this hour
      for (let day = 0; day < 7; day++) {
        const dayDate = new Date(startOfWeek);
        dayDate.setDate(startOfWeek.getDate() + day);
        dayDate.setHours(hour, 0, 0, 0);
        
        const slot = document.createElement('div');
        slot.className = 'week-time-slot';
        slot.dataset.date = dayDate.toISOString();
        
        // Highlight current hour
        const now = new Date();
        if (now.getDate() === dayDate.getDate() && 
            now.getMonth() === dayDate.getMonth() && 
            now.getFullYear() === dayDate.getFullYear() && 
            now.getHours() === hour) {
          slot.classList.add('current-hour');
        }
        
        // Add events for this time slot
        const slotEvents = getEventsForTimeSlot(dayDate, hour);
        slotEvents.forEach(event => {
          const eventEl = document.createElement('div');
          eventEl.className = `week-event ${event.group_id ? 'group-event' : ''} ${event.status}`;
          eventEl.textContent = event.title;
          eventEl.title = `${event.title}\n${formatTime(event.start)} - ${formatTime(event.end)}`;
          eventEl.dataset.appointmentId = event.id;
          
          // Calculate position and height based on event duration
          const startTime = new Date(event.start);
          const endTime = new Date(event.end);
          const startMinutes = startTime.getHours() * 60 + startTime.getMinutes();
          const endMinutes = endTime.getHours() * 60 + endTime.getMinutes();
          const duration = endMinutes - startMinutes;
          
          const topOffset = (startMinutes % 60) / 60 * 100;
          const height = Math.max((duration / 60) * 100, 20);
          
          eventEl.style.top = `${topOffset}%`;
          eventEl.style.height = `${height}%`;
          eventEl.style.position = 'absolute';
          
          slot.appendChild(eventEl);
        });
        
        calendarGrid.appendChild(slot);
      }
    }
  }

  function renderDayView(calendarGrid) {
    calendarGrid.className = 'calendar-grid day-view';
    
    // Add time column header
    const timeHeader = document.createElement('div');
    timeHeader.className = 'time-column';
    timeHeader.textContent = '';
    calendarGrid.appendChild(timeHeader);
    
    // Add day header
    const dayHeader = document.createElement('div');
    dayHeader.className = 'day-header';
    
    const dayName = document.createElement('div');
    dayName.className = 'day-name';
    dayName.textContent = state.currentDate.toLocaleDateString('en-US', { 
      weekday: 'long',
      month: 'long', 
      day: 'numeric',
      year: 'numeric'
    });
    
    dayHeader.appendChild(dayName);
    dayHeader.dataset.date = state.currentDate.toISOString().split('T')[0];
    calendarGrid.appendChild(dayHeader);
    
    // Add time slots (24 hours)
    for (let hour = 0; hour < 24; hour++) {
      // Time label
      const timeSlot = document.createElement('div');
      timeSlot.className = 'time-column';
      timeSlot.textContent = formatHour(hour);
      calendarGrid.appendChild(timeSlot);
      
      // Day slot for this hour
      const dayDate = new Date(state.currentDate);
      dayDate.setHours(hour, 0, 0, 0);
      
      const slot = document.createElement('div');
      slot.className = 'day-time-slot';
      slot.dataset.date = dayDate.toISOString();
      
      // Highlight current hour
      const now = new Date();
      if (now.getDate() === dayDate.getDate() && 
          now.getMonth() === dayDate.getMonth() && 
          now.getFullYear() === dayDate.getFullYear() && 
          now.getHours() === hour) {
        slot.classList.add('current-hour');
      }
      
      // Add events for this time slot
      const slotEvents = getEventsForTimeSlot(dayDate, hour);
      slotEvents.forEach(event => {
        const eventEl = document.createElement('div');
        eventEl.className = `day-event ${event.group_id ? 'group-event' : ''} ${event.status}`;
        eventEl.textContent = event.title;
        eventEl.title = `${event.title}\n${formatTime(event.start)} - ${formatTime(event.end)}`;
        eventEl.dataset.appointmentId = event.id;
        
        // Calculate position and height based on event duration
        const startTime = new Date(event.start);
        const endTime = new Date(event.end);
        const startMinutes = startTime.getHours() * 60 + startTime.getMinutes();
        const endMinutes = endTime.getHours() * 60 + endTime.getMinutes();
        const duration = endMinutes - startMinutes;
        
        const topOffset = (startMinutes % 60) / 60 * 100;
        const height = Math.max((duration / 60) * 100, 20);
        
        eventEl.style.top = `${topOffset}%`;
        eventEl.style.height = `${height}%`;
        eventEl.style.position = 'absolute';
        
        slot.appendChild(eventEl);
      });
      
      calendarGrid.appendChild(slot);
    }
  }

  function getEventsForTimeSlot(date, hour) {
    const startOfHour = new Date(date);
    startOfHour.setHours(hour, 0, 0, 0);
    const endOfHour = new Date(date);
    endOfHour.setHours(hour + 1, 0, 0, 0);
    
    return state.events.filter(event => {
      const eventStart = new Date(event.start);
      const eventEnd = new Date(event.end);
      
      // Check if event overlaps with this hour
      return eventStart < endOfHour && eventEnd > startOfHour;
    });
  }

  function formatHour(hour) {
    if (hour === 0) return '12 AM';
    if (hour < 12) return `${hour} AM`;
    if (hour === 12) return '12 PM';
    return `${hour - 12} PM`;
  }

  function updateCurrentDateDisplay() {
    const currentDateEl = $('currentDate');
    
    if (state.currentView === 'month') {
      currentDateEl.textContent = state.currentDate.toLocaleDateString('en-US', { 
        month: 'long', 
        year: 'numeric' 
      });
    } else if (state.currentView === 'week') {
      const startOfWeek = new Date(state.currentDate);
      startOfWeek.setDate(state.currentDate.getDate() - state.currentDate.getDay());
      const endOfWeek = new Date(startOfWeek);
      endOfWeek.setDate(startOfWeek.getDate() + 6);
      
      const startStr = startOfWeek.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
      const endStr = endOfWeek.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
      
      currentDateEl.textContent = `${startStr} - ${endStr}`;
    } else if (state.currentView === 'day') {
      currentDateEl.textContent = state.currentDate.toLocaleDateString('en-US', { 
        weekday: 'long',
        month: 'long', 
        day: 'numeric',
        year: 'numeric'
      });
    }
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
    if (state.currentView === 'month') {
      state.currentDate.setMonth(state.currentDate.getMonth() + direction);
    } else if (state.currentView === 'week') {
      state.currentDate.setDate(state.currentDate.getDate() + (direction * 7));
    } else if (state.currentView === 'day') {
      state.currentDate.setDate(state.currentDate.getDate() + direction);
    }
    renderCalendar();
    loadEvents();
  }

  function goToToday() {
    state.currentDate = new Date();
    renderCalendar();
    loadEvents();
  }

  function switchView(view) {
    console.log('=== SWITCHING VIEW ===');
    console.log('Requested view:', view);
    console.log('Current state view:', state.currentView);
    
    // Update state
    state.currentView = view;
    console.log('State updated to:', state.currentView);
    
    // Remove active class from all buttons with animation
    $$('.view-btn').forEach(btn => {
      btn.classList.remove('active');
      btn.style.transform = 'translateY(0)';
      btn.style.boxShadow = 'none';
      console.log('Removed active from:', btn.dataset.view);
    });
    
    // Add active class to target button with animation
    const targetBtn = $(`[data-view="${view}"]`);
    if (targetBtn) {
      // Small delay for smooth transition
      setTimeout(() => {
        targetBtn.classList.add('active');
        targetBtn.style.transform = 'translateY(-1px)';
        targetBtn.style.boxShadow = '0 2px 4px rgba(0,0,0,0.15)';
        console.log('Added active to:', targetBtn.dataset.view);
      }, 50);
    } else {
      console.error('Target button not found for view:', view);
    }
    
    // Update navigation buttons based on view
    const prevBtn = $('prevMonth');
    const nextBtn = $('nextMonth');
    
    if (view === 'week') {
      prevBtn.textContent = '‹';
      nextBtn.textContent = '›';
    } else if (view === 'day') {
      prevBtn.textContent = '‹';
      nextBtn.textContent = '›';
    } else {
      prevBtn.textContent = '‹';
      nextBtn.textContent = '›';
    }
    
    // Force re-render
    console.log('About to call renderCalendar');
    renderCalendar();
    
    console.log('=== VIEW SWITCH COMPLETE ===');
  }

  // Event management
  function showEventModal(date = null) {
    if (!state.token) {
      showAuthModal('login');
      return;
    }
    
    $('eventModal').classList.add('show');
    
    let startTime, endTime;
    if (date) {
      startTime = new Date(date);
      startTime.setHours(9, 0, 0, 0);
      endTime = new Date(startTime);
      endTime.setHours(10, 0, 0, 0);
    } else {
      startTime = new Date();
      startTime.setMinutes(0, 0, 0); // round to the hour
      endTime = new Date(startTime);
      endTime.setHours(startTime.getHours() + 1);
    }
    $('eventStart').value = formatDateTimeLocal(startTime);
    $('eventEnd').value = formatDateTimeLocal(endTime);
  }

  function hideEventModal() {
    $('eventModal').classList.remove('show');
    $('eventForm').reset();
    state.editingAppointment = null; // Reset editing state
    $('eventModalTitle').textContent = 'Create Event'; // Reset modal title
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
      const startRaw = $('eventStart').value;
      const endRaw = $('eventEnd').value;
      // Log values before parsing
      console.log('[saveEvent] Raw start:', startRaw, 'Raw end:', endRaw);

      if (!startRaw || !endRaw) {
        alert('Debes completar las fechas de inicio y fin.');
        return;
      }

      const startDate = new Date(startRaw);
      const endDate = new Date(endRaw);

      if (isNaN(startDate.getTime()) || isNaN(endDate.getTime())) {
        alert('Formato de fecha inválido.');
        return;
      }

      const formData = {
        title: $('eventTitle').value,
        description: $('eventDescription').value,
        start: startDate.toISOString(),
        end: endDate.toISOString(),
        privacy: $('eventPrivacy').value,
        group_id: $('eventGroup').value ? Number($('eventGroup').value) : undefined
      };
      // Log form data antes de enviar
      console.log('[saveEvent] Form data to send:', formData);
      
      let response;
      if (state.editingAppointment) {
        // Update existing appointment
        console.log('[saveEvent] Updating appointment:', state.editingAppointment);
        response = await api(`/api/appointments/${state.editingAppointment}`, {
          method: 'PUT',
          body: JSON.stringify(formData)
        });
        console.log('[saveEvent] Event updated successfully:', response);
      } else {
        // Create new appointment
        console.log('[saveEvent] Creating new appointment');
        response = await api('/api/appointments', {
          method: 'POST',
          body: JSON.stringify(formData)
        });
        console.log('[saveEvent] Event created successfully:', response);
      }
      
      hideEventModal();
      loadEvents();
    } catch (error) {
      // Log error completo
      console.error('[saveEvent] Error creating event:', error, error.stack);
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
      
      const groupTypeIcon = group.group_type === 'hierarchical' ? '👑' : '👥';
      const groupTypeText = group.group_type === 'hierarchical' ? 'H' : 'NH';
      
      item.innerHTML = `
        <div class="calendar-color" style="background: #34a853;"></div>
        <span class="calendar-name">${group.name}</span>
        <span class="calendar-count" id="groupCount${group.id}" title="${groupTypeIcon} ${group.group_type}">${groupTypeText}</span>
      `;
      
      item.addEventListener('click', () => openGroupDetails(group.id));
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
      const groupType = $('groupType').value;
      if (!name) { alert('Group name is required'); return; }
      if (!groupType) { alert('Group type is required'); return; }
      console.log('Sending group type:', groupType);
      const response = await api('/api/groups', {
        method: 'POST',
        body: JSON.stringify({ name, description, group_type: groupType })
      });

      console.log('Group created successfully:', response);
      hideGroupModal();
      await loadGroups();
      updateEventGroupSelect(); // keep the event modal's group list in sync
    } catch (e) {
      console.error('Failed to create group:', e);
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

  async function openGroupDetails(groupId) {
    try {
      if (!state.token) { showAuthModal('login'); return; }
      const res = await api(`/api/groups/${groupId}`);
      const { group, members } = res || {};
      if (!group) return;

      // Store current group for editing
      state.currentGroup = group;

      $('groupDetailsTitle').textContent = group.name;
      $('groupDetailsName').textContent = group.name;
      $('groupDetailsDesc').textContent = group.description || '';
      
      // Show group type
      const groupTypeElement = document.getElementById('groupDetailsType');
      if (groupTypeElement) {
        groupTypeElement.textContent = group.group_type === 'hierarchical' ? 'Hierarchical' : 'Non-Hierarchical';
      }
      
      // Show/hide rank field based on group type
      const rankField = $('newMemberRank');
      if (group.group_type === 'hierarchical') {
        rankField.style.display = 'block';
        rankField.placeholder = 'Rank';
      } else {
        rankField.style.display = 'none';
        rankField.value = '0'; // Set default rank for non-hierarchical groups
      }

      const list = $('groupMembersList');
      list.innerHTML = '';

      // Render members with user_id, username, rank and action buttons
      for (const m of members) {
        const row = document.createElement('div');
        row.style.display = 'flex';
        row.style.justifyContent = 'space-between';
        row.style.alignItems = 'center';
        row.style.padding = '6px 8px';
        row.style.borderBottom = '1px solid #eee';
        
        const memberInfo = document.createElement('div');
        memberInfo.style.display = 'flex';
        memberInfo.style.alignItems = 'center';
        memberInfo.style.gap = '8px';
        memberInfo.innerHTML = `<span>@${m.username || m.user_id}</span><span style="color: #5f6368;">Rank: ${m.rank}</span>`;
        
        const actions = document.createElement('div');
        actions.style.display = 'flex';
        actions.style.gap = '4px';
        actions.innerHTML = `
          <button class="btn btn-sm" onclick="editMemberRank(${m.user_id}, ${m.rank})" style="padding: 2px 6px; font-size: 11px;">Edit Rank</button>
          <button class="btn btn-sm btn-danger" onclick="removeMember(${m.user_id}, '${m.username || m.user_id}')" style="padding: 2px 6px; font-size: 11px; background: #ea4335; color: white;">Remove</button>
        `;
        
        row.appendChild(memberInfo);
        row.appendChild(actions);
        list.appendChild(row);
      }

      // Bind add member action
      const addBtn = $('addMemberBtn');
      addBtn.onclick = async () => {
        try {
          const username = ($('newMemberUsername').value || '').trim();
          const rank = Number(($('newMemberRank').value || '').trim());

          console.log('DEBUG username:', username, 'rank:', rank);
          if (!username) { alert('Username is required'); return; }
          
          // For non-hierarchical groups, rank is automatically set to 0
          // For hierarchical groups, validate the rank
          if (state.currentGroup && state.currentGroup.group_type === 'hierarchical') {
            if (Number.isNaN(rank) || rank < 0) { alert('Rank must be a positive number'); return; }
          }

          const response = await api(`/api/groups/${groupId}/members`, {
            method: 'POST',
            body: JSON.stringify({ username, rank })
          });

          console.log('Member added successfully:', response);
          $('newMemberUsername').value = '';
          $('newMemberRank').value = '';
          
          // Reset rank field visibility based on group type
          if (state.currentGroup && state.currentGroup.group_type === 'non_hierarchical') {
            $('newMemberRank').style.display = 'none';
          }

          await openGroupDetails(groupId);
        } catch (e) {
          console.error('Failed to add member:', e);
          alert('Failed to add member: ' + e.message);
        }
      };


      // Show modal
      const modal = $('groupDetailsModal');
      modal.classList.add('show');
      modal.style.display = 'flex';

      const closeBtn = $('closeGroupDetailsModal');
      closeBtn.onclick = () => { 
        modal.classList.remove('show'); 
        modal.style.display = 'none'; 
      };
    } catch (e) {
      alert('Failed to load group details: ' + e.message);
    }
  }

  // Event Details Modal
  async function showEventDetailsModal(appointmentId) {
    console.log('Showing event details for ID:', appointmentId);
    
    if (!state.token) {
      showAuthModal('login');
      return;
    }
    
    try {
      const response = await api(`/api/appointments/${appointmentId}`);
      const { appointment, participants } = response;
      
      console.log('Event details loaded:', appointment);
      
      // Store current appointment for editing
      state.currentAppointment = appointment;
      
      // Populate appointment details
      $('detailTitle').textContent = appointment.title;
      $('detailDescription').textContent = appointment.description || 'No description';
      $('detailStart').textContent = formatDateTime(appointment.start);
      $('detailEnd').textContent = formatDateTime(appointment.end);
      $('detailPrivacy').textContent = appointment.privacy === 'full' ? 'Full details' : 'Free/Busy only';
      $('detailStatus').textContent = capitalizeFirst(appointment.status);
      
      // Show participants section if there are participants
      const participantsSection = $('participantsSection');
      const participantsList = $('participantsList');
      
      if (participants && participants.length > 0) {
        participantsSection.style.display = 'flex';
        participantsList.innerHTML = '';
        
        participants.forEach(participant => {
          const participantEl = document.createElement('div');
          participantEl.className = 'participant-item';
          
          participantEl.innerHTML = `
            <div class="participant-info">
              <div class="participant-name">${participant.display_name}</div>
              <div class="participant-username">@${participant.username}</div>
            </div>
            <span class="status-badge status-${participant.status}">${participant.status}</span>
          `;
          
          participantsList.appendChild(participantEl);
        });
      } else {
        participantsSection.style.display = 'none';
      }
      
      // Show/hide edit and delete buttons based on ownership
      const editBtn = $('editEventBtn');
      const deleteBtn = $('deleteEventBtn');
      
      if (appointment.owner_id === state.user.id) {
        editBtn.style.display = 'inline-block';
        deleteBtn.style.display = 'inline-block';
      } else {
        editBtn.style.display = 'none';
        deleteBtn.style.display = 'none';
      }
      
      $('eventDetailsModal').classList.add('show');
      console.log('Event details modal shown');
    } catch (error) {
      console.error('Failed to load event details:', error);
      alert('Failed to load event details: ' + error.message);
    }
  }

  function hideEventDetailsModal() {
    $('eventDetailsModal').classList.remove('show');
    
    // If we're viewing from Personal modal, return to Personal events list
    if (state.viewingFromPersonal) {
      // The Personal modal should still be open, so we just return to it
      // No need to do anything special as the Personal modal remains open
    }
  }

  // Add new function to handle event editing
  function editEvent() {
    if (!state.currentAppointment) return;
    
    const appointment = state.currentAppointment;
    
    // Populate the event form with current data
    $('eventTitle').value = appointment.title;
    $('eventDescription').value = appointment.description || '';
    $('eventStart').value = formatDateTimeLocal(new Date(appointment.start));
    $('eventEnd').value = formatDateTimeLocal(new Date(appointment.end));
    $('eventPrivacy').value = appointment.privacy;
    $('eventGroup').value = appointment.group_id || '';
    
    // Set flag to indicate we're editing
    state.editingAppointment = appointment.id;
    
    // Hide event details modal and show event modal
    hideEventDetailsModal();
    showEventModal();
    
    // Update modal title
    $('eventModalTitle').textContent = 'Edit Event';
  }

  // Add new function to handle event deletion
  async function deleteEvent() {
    if (!state.currentAppointment) return;
    
    const appointment = state.currentAppointment;
    
    // Confirm deletion
    const confirmed = confirm(`Are you sure you want to delete "${appointment.title}"? This action cannot be undone.`);
    if (!confirmed) return;
    
    try {
      await api(`/api/appointments/${appointment.id}`, {
        method: 'DELETE'
      });
      
      console.log('Event deleted successfully');
      hideEventDetailsModal();
      loadEvents(); // Refresh the calendar
      alert('Event deleted successfully');
    } catch (error) {
      console.error('Failed to delete event:', error);
      alert('Failed to delete event: ' + error.message);
    }
  }

  function formatDateTime(dateString) {
    const date = new Date(dateString);
    return date.toLocaleDateString('en-US', {
      weekday: 'short',
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
      hour12: true
    });
  }

  function capitalizeFirst(str) {
    return str.charAt(0).toUpperCase() + str.slice(1);
  }

  // Calendar Picker Functions
  function showCalendarPicker() {
    $('calendarPickerModal').classList.add('show');
    state.selectedDate = new Date(state.currentDate);
    
    // Update modal title based on current view
    const title = $('calendarPickerTitle');
    if (state.currentView === 'week') {
      title.textContent = 'Select Week';
    } else if (state.currentView === 'day') {
      title.textContent = 'Select Day';
    } else {
      title.textContent = 'Select Date';
    }
    
    renderCalendarPicker();
  }

  function hideCalendarPicker() {
    $('calendarPickerModal').classList.remove('show');
  }

  function renderCalendarPicker() {
    const pickerGrid = $('calendarPickerGrid');
    const monthYear = $('calendarMonthYear');
    const date = state.selectedDate;
    
    // Update month/year display
    monthYear.textContent = date.toLocaleDateString('en-US', { 
      month: 'long', 
      year: 'numeric' 
    });
    
    // Clear existing content
    pickerGrid.innerHTML = '';
    
    // Add day headers
    const dayHeaders = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
    dayHeaders.forEach(day => {
      const dayHeader = document.createElement('div');
      dayHeader.className = 'calendar-picker-day-header';
      dayHeader.textContent = day;
      pickerGrid.appendChild(dayHeader);
    });
    
    // Get first day of month and number of days
    const firstDay = new Date(date.getFullYear(), date.getMonth(), 1);
    const lastDay = new Date(date.getFullYear(), date.getMonth() + 1, 0);
    const startDate = new Date(firstDay);
    startDate.setDate(startDate.getDate() - firstDay.getDay());
    
    // Generate 42 days (6 weeks)
    for (let i = 0; i < 42; i++) {
      const cellDate = new Date(startDate);
      cellDate.setDate(startDate.getDate() + i);
      
      const dayCell = document.createElement('div');
      dayCell.className = 'calendar-picker-day';
      dayCell.textContent = cellDate.getDate();
      dayCell.dataset.date = cellDate.toISOString().split('T')[0];
      
      // Add classes for styling
      if (cellDate.getMonth() !== date.getMonth()) {
        dayCell.classList.add('other-month');
      }
      
      const today = new Date();
      if (cellDate.toDateString() === today.toDateString()) {
        dayCell.classList.add('today');
      }
      
      // Add selection styling based on current view - HIGHLIGHT CURRENT VIEW
      if (state.currentView === 'week') {
        // Highlight the current week being viewed
        if (isDateInCurrentWeek(cellDate, state.currentDate)) {
          dayCell.classList.add('week-selected');
        }
      } else if (state.currentView === 'day') {
        // Highlight the current day being viewed
        if (cellDate.toDateString() === state.currentDate.toDateString()) {
          dayCell.classList.add('day-selected');
        }
      } else {
        // Month view - highlight the current date
        if (cellDate.toDateString() === state.currentDate.toDateString()) {
          dayCell.classList.add('selected');
        }
      }
      
      // Add click handler
      dayCell.onclick = () => selectCalendarDate(cellDate);
      
      pickerGrid.appendChild(dayCell);
    }
  }

  function isDateInCurrentWeek(date, currentDate) {
    // Get the start of the week (Sunday) for the current date
    const startOfWeek = new Date(currentDate);
    startOfWeek.setDate(currentDate.getDate() - currentDate.getDay());
    
    // Get the end of the week (Saturday) for the current date
    const endOfWeek = new Date(startOfWeek);
    endOfWeek.setDate(startOfWeek.getDate() + 6);
    
    return date >= startOfWeek && date <= endOfWeek;
  }

  function selectCalendarDate(date) {
    // Remove previous selections
    $$('.calendar-picker-day.selected, .calendar-picker-day.week-selected, .calendar-picker-day.day-selected').forEach(day => {
      day.classList.remove('selected', 'week-selected', 'day-selected');
    });
    
    // Add selection to clicked date based on current view
    const dayElement = $(`.calendar-picker-day[data-date="${date.toISOString().split('T')[0]}"]`);
    if (dayElement) {
      if (state.currentView === 'week') {
        // For week view, highlight the entire week of the selected date
        const weekStart = new Date(date);
        weekStart.setDate(date.getDate() - date.getDay());
        
        // Highlight all days in the selected week
        for (let i = 0; i < 7; i++) {
          const weekDay = new Date(weekStart);
          weekDay.setDate(weekStart.getDate() + i);
          const weekDayElement = $(`.calendar-picker-day[data-date="${weekDay.toISOString().split('T')[0]}"]`);
          if (weekDayElement) {
            weekDayElement.classList.add('week-selected');
          }
        }
      } else if (state.currentView === 'day') {
        dayElement.classList.add('day-selected');
      } else {
        dayElement.classList.add('selected');
      }
    }
    
    state.selectedDate = new Date(date);
  }

  function applyCalendarSelection() {
    if (state.currentView === 'week') {
      // For week view, set to the start of the selected week
      const selectedDate = new Date(state.selectedDate);
      selectedDate.setDate(selectedDate.getDate() - selectedDate.getDay());
      state.currentDate = selectedDate;
    } else if (state.currentView === 'day') {
      // For day view, set to the selected day
      state.currentDate = new Date(state.selectedDate);
    } else {
      // For month view, set to the selected date
      state.currentDate = new Date(state.selectedDate);
    }
    
    renderCalendar();
    loadEvents();
    hideCalendarPicker();
  }

  function goToTodayInPicker() {
    const today = new Date();
    state.selectedDate = new Date(today);
    renderCalendarPicker();
  }

  function navigateCalendarPicker(direction) {
    if (direction === 'prev-month') {
      state.selectedDate.setMonth(state.selectedDate.getMonth() - 1);
    } else if (direction === 'next-month') {
      state.selectedDate.setMonth(state.selectedDate.getMonth() + 1);
    } else if (direction === 'prev-year') {
      state.selectedDate.setFullYear(state.selectedDate.getFullYear() - 1);
    } else if (direction === 'next-year') {
      state.selectedDate.setFullYear(state.selectedDate.getFullYear() + 1);
    }
    renderCalendarPicker();
  }

  // Personal Events Modal Functions
  function showPersonalEventsModal() {
    if (!state.token) {
      showAuthModal('login');
      return;
    }
    
    const modal = $('personalEventsModal');
    modal.classList.add('show');
    modal.style.display = 'flex';
    
    // Clear search input when opening modal
    const searchInput = $('personalEventsSearch');
    if (searchInput) {
      searchInput.value = '';
    }
    
    // Set flag to track that we're viewing from Personal modal
    state.viewingFromPersonal = true;
    
    loadPersonalEvents();
  }

  function hidePersonalEventsModal() {
    const modal = $('personalEventsModal');
    modal.classList.remove('show');
    modal.style.display = 'none';
    
    // Clear the flag when closing Personal modal
    state.viewingFromPersonal = false;
  }

  async function loadPersonalEvents() {
    if (!state.token) return;
    
    try {
      // Get events for a wider range (past 6 months to future 6 months)
      const now = new Date();
      const start = new Date(now.getFullYear(), now.getMonth() - 6, 1);
      const end = new Date(now.getFullYear(), now.getMonth() + 6, 0);
      
      const res = await api(`/api/agenda?start=${start.toISOString()}&end=${end.toISOString()}`);
      const personalEvents = (res || []).filter(e => !e.group_id);
      
      // Store all events for filtering
      state.personalEvents = personalEvents;
      
      renderPersonalEventsList(personalEvents);
      setupPersonalEventsFilters();
    } catch (error) {
      console.error('Failed to load personal events:', error);
      state.personalEvents = [];
      renderPersonalEventsList([]);
    }
  }

  function renderPersonalEventsList(events) {
    const container = $('personalEventsList');
    container.innerHTML = '';
    
    if (events.length === 0) {
      container.innerHTML = `
        <div class="no-events">
          <div class="no-events-icon">��</div>
          <h3>No Personal Events</h3>
          <p>You don't have any personal events yet.<br>Create your first event to get started!</p>
        </div>
      `;
      return;
    }
    
    // Sort events by date (newest first)
    const sortedEvents = events.sort((a, b) => new Date(b.start) - new Date(a.start));
    
    sortedEvents.forEach(event => {
      const eventEl = document.createElement('div');
      eventEl.className = 'event-list-item';
      
      const now = new Date();
      const eventDate = new Date(event.start);
      const isPast = eventDate < now;
      
      if (isPast) {
        eventEl.classList.add('past');
      }
      
      const startTime = formatDateTime(event.start);
      const endTime = formatDateTime(event.end);
      
      // Get search term for highlighting
      const searchTerm = ($('personalEventsSearch')?.value || '').toLowerCase();
      
      // Highlight search term in title and description
      const highlightedTitle = highlightSearchTerm(event.title, searchTerm);
      const highlightedDescription = highlightSearchTerm(event.description || 'No description', searchTerm);
      
      eventEl.innerHTML = `
        <div class="event-color" style="background: #1a73e8;"></div>
        <div class="event-content">
          <div class="event-title">${highlightedTitle}</div>
          <div class="event-description">${highlightedDescription}</div>
          <div class="event-time">${startTime} - ${endTime}</div>
        </div>
        <div class="event-status ${event.status}">${event.status}</div>
      `;
      
      eventEl.addEventListener('click', () => {
        // Don't hide Personal modal, just show event details
        showEventDetailsModal(event.id);
      });
      
      container.appendChild(eventEl);
    });
  }

  function highlightSearchTerm(text, searchTerm) {
    if (!searchTerm || !text) return text;
    
    const regex = new RegExp(`(${searchTerm})`, 'gi');
    return text.replace(regex, '<span class="search-highlight">$1</span>');
  }

  function setupPersonalEventsFilters() {
    const searchInput = $('personalEventsSearch');
    const filterSelect = $('personalEventsFilter');
    const sortSelect = $('personalEventsSort');
    
    if (!searchInput || !filterSelect || !sortSelect) return;
    
    // Search input with debouncing
    let searchTimeout;
    searchInput.oninput = () => {
      clearTimeout(searchTimeout);
      searchTimeout = setTimeout(() => {
        applyPersonalEventsFilter();
      }, 300);
    };
    
    // Filter and sort change handlers
    filterSelect.onchange = applyPersonalEventsFilter;
    sortSelect.onchange = applyPersonalEventsFilter;
  }

  function applyPersonalEventsFilter() {
    if (!state.personalEvents) return;
    
    const searchTerm = ($('personalEventsSearch')?.value || '').toLowerCase();
    const filter = $('personalEventsFilter')?.value || 'all';
    const sort = $('personalEventsSort')?.value || 'date';
    
    let filteredEvents = [...state.personalEvents];
    
    // Apply search filter
    if (searchTerm) {
      filteredEvents = filteredEvents.filter(event => 
        event.title.toLowerCase().includes(searchTerm) ||
        (event.description && event.description.toLowerCase().includes(searchTerm))
      );
    }
    
    // Apply date filter
    if (filter === 'upcoming') {
      const now = new Date();
      filteredEvents = filteredEvents.filter(e => new Date(e.start) >= now);
    } else if (filter === 'past') {
      const now = new Date();
      filteredEvents = filteredEvents.filter(e => new Date(e.start) < now);
    }
    
    // Apply sort
    if (sort === 'title') {
      filteredEvents.sort((a, b) => a.title.localeCompare(b.title));
    } else {
      filteredEvents.sort((a, b) => new Date(b.start) - new Date(a.start));
    }
    
    renderPersonalEventsList(filteredEvents);
  }

  async function loadNotifications() {
    if (!state.token) return;
    try {
      const items = await api('/api/notifications');
      // Sort by created_at descending
      items.sort((a, b) => new Date(b.created_at) - new Date(a.created_at));
      state.notifications = items;
      renderNotificationsList();
    } catch (err) {
      console.error('Failed to load notifications:', err);
    }
  }

  async function updateUnreadNotificationsCount() {
    if (!state.token) {
      const badge = $('unreadNotificationsCount');
      if (badge) badge.textContent = '0';
      return;
    }
    try {
      const unread = await api('/api/notifications/unread');
      const badge = $('unreadNotificationsCount');
      if (badge) badge.textContent = String(unread.length);
    } catch (err) {
      console.error('Failed to load unread notifications:', err);
    }
  }

  function renderNotificationsList() {
    const container = $('notificationsList');
    if (!container) return;

    const items = state.notifications || [];
    if (!items.length) {
      container.innerHTML = '<div class="notification-item"><div class="notification-content"><div class="notification-title">No notifications</div></div></div>';
      return;
    }

    container.innerHTML = items.map(n => {
      const unread = !n.read_at;
      const title = escapeHtml(parseNotificationTitle(n));
      const created = formatDateTime(n.created_at);
      return `
        <div class="notification-item ${unread ? 'unread' : ''}" data-id="${n.id}">
          <div class="notification-dot"></div>
          <div class="notification-content">
            <div class="notification-title">${title}</div>
            <div class="notification-meta">${created}${unread ? ' · Unread' : ''}</div>
          </div>
        </div>
      `;
    }).join('');

    // Attach click handlers to show notification details
    container.querySelectorAll('.notification-item').forEach(el => {
      el.addEventListener('click', async () => {
        const id = el.getAttribute('data-id');
        const item = (state.notifications || []).find(x => String(x.id) === String(id));
        if (!item) return;
        
        // Mark as read if not already read
        if (!item.read_at) {
          try {
            await api(`/api/notifications/${id}/read`, { method: 'POST' });
            item.read_at = new Date().toISOString();
            el.classList.remove('unread');
            updateUnreadNotificationsCount();
          } catch (err) {
            console.error('Failed to mark notification read:', err);
          }
        }
        
        // Show notification details
        await showNotificationDetails(item);
      });
    });
  }

  // Small util to prevent XSS when injecting text
  function escapeHtml(s) {
    if (s == null) return '';
    return String(s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;')
      .replace(/>/g, '&gt;').replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  }

  function formatDateTime(dt) {
    try {
      const d = new Date(dt);
      return d.toLocaleString();
    } catch {
      return dt;
    }
  }

  function parseNotificationTitle(n) {
    // Try to use payload JSON title if present; fallback to type
    try {
      const payload = n.payload ? JSON.parse(n.payload) : null;
      if (payload && payload.title) {
        // For group events, show group name and creator info
        if (payload.group_name && payload.created_by_username) {
          return `${payload.title} (${payload.group_name} - by @${payload.created_by_username})`;
        }
        return payload.title;
      }
    } catch {}
    return n.type || 'Notification';
  }

  // ======================
  // Group Management Functions
  // ======================

  function showEditGroupModal(field) {
    if (!state.currentGroup) return;
    
    state.editingGroupField = field;
    $('editGroupName').value = state.currentGroup.name;
    $('editGroupDesc').value = state.currentGroup.description || '';
    
    // Show/hide fields based on what we're editing
    if (field === 'name') {
      $('editGroupName').style.display = 'block';
      $('editGroupDesc').style.display = 'none';
      $('editGroupName').parentElement.style.display = 'block';
      $('editGroupDesc').parentElement.style.display = 'none';
    } else if (field === 'description') {
      $('editGroupName').style.display = 'none';
      $('editGroupDesc').style.display = 'block';
      $('editGroupName').parentElement.style.display = 'none';
      $('editGroupDesc').parentElement.style.display = 'block';
    }
    
    $('editGroupModal').classList.add('show');
    $('editGroupModal').style.display = 'flex';
  }

  function hideEditGroupModal() {
    $('editGroupModal').classList.remove('show');
    $('editGroupModal').style.display = 'none';
    state.editingGroupField = null;
  }

  async function saveEditGroup() {
    try {
      if (!state.currentGroup) return;
      
      const groupId = state.currentGroup.id;
      let updateData = {};
      
      if (state.editingGroupField === 'name') {
        const name = $('editGroupName').value.trim();
        if (!name) { alert('Group name is required'); return; }
        updateData.name = name;
        updateData.description = state.currentGroup.description || ''; // Keep current description
      } else if (state.editingGroupField === 'description') {
        updateData.name = state.currentGroup.name; // Keep current name
        updateData.description = $('editGroupDesc').value.trim();
      }
      
      const response = await api(`/api/groups/${groupId}`, {
        method: 'PUT',
        body: JSON.stringify(updateData)
      });
      
      console.log('Group updated successfully:', response);
      
      // Update local state
      if (updateData.name) state.currentGroup.name = updateData.name;
      if (updateData.description !== undefined) state.currentGroup.description = updateData.description;
      
      // Update UI
      $('groupDetailsName').textContent = state.currentGroup.name;
      $('groupDetailsDesc').textContent = state.currentGroup.description || '';
      
      hideEditGroupModal();
      await loadGroups(); // Refresh groups list
    } catch (e) {
      console.error('Failed to update group:', e);
      alert('Failed to update group: ' + e.message);
    }
  }

  async function deleteGroup() {
    if (!state.currentGroup) return;
    
    const confirmed = confirm(`Are you sure you want to delete the group "${state.currentGroup.name}"? This will also delete all group appointments and remove all members. This action cannot be undone.`);
    if (!confirmed) return;
    
    try {
      const groupId = state.currentGroup.id;
      await api(`/api/groups/${groupId}`, {
        method: 'DELETE'
      });
      
      console.log('Group deleted successfully');
      
      // Close modals and refresh
      $('groupDetailsModal').classList.remove('show');
      $('groupDetailsModal').style.display = 'none';
      state.currentGroup = null;
      
      await loadGroups();
      updateEventGroupSelect();
      alert('Group deleted successfully');
    } catch (e) {
      console.error('Failed to delete group:', e);
      alert('Failed to delete group: ' + e.message);
    }
  }

  async function editMemberRank(userId, currentRank) {
    const newRank = prompt(`Enter new rank for this member (current: ${currentRank}):`, currentRank);
    if (newRank === null) return; // User cancelled
    
    const rank = Number(newRank);
    if (Number.isNaN(rank) || rank < 0) {
      alert('Rank must be a positive number');
      return;
    }
    
    try {
      const groupId = state.currentGroup.id;
      await api(`/api/groups/${groupId}/members/${userId}`, {
        method: 'PUT',
        body: JSON.stringify({ rank })
      });
      
      console.log('Member rank updated successfully');
      await openGroupDetails(groupId); // Refresh group details
    } catch (e) {
      console.error('Failed to update member rank:', e);
      alert('Failed to update member rank: ' + e.message);
    }
  }

  async function removeMember(userId, username) {
    const confirmed = confirm(`Are you sure you want to remove @${username} from this group? This will also remove them from all group appointments.`);
    if (!confirmed) return;
    
    try {
      const groupId = state.currentGroup.id;
      await api(`/api/groups/${groupId}/members/${userId}`, {
        method: 'DELETE'
      });
      
      console.log('Member removed successfully');
      await openGroupDetails(groupId); // Refresh group details
    } catch (e) {
      console.error('Failed to remove member:', e);
      alert('Failed to remove member: ' + e.message);
    }
  }

  // Make functions globally available for onclick handlers
  window.editMemberRank = editMemberRank;
  window.removeMember = removeMember;

  // ======================
  // Notification Details Functions
  // ======================

  async function showNotificationDetails(notification) {
    try {
      state.currentNotification = notification;
      
      // Parse notification payload
      let payload = {};
      try {
        payload = JSON.parse(notification.payload || '{}');
      } catch (e) {
        console.warn('Failed to parse notification payload:', e);
      }
      
      // Update modal content
      $('notificationType').textContent = getNotificationTypeLabel(notification.type);
      $('notificationMessage').textContent = getNotificationMessage(notification, payload);
      $('notificationDate').textContent = formatDateTime(notification.created_at);
      
      // Show additional details from payload if available
      const additionalDetails = $('additionalNotificationDetails');
      if (additionalDetails) {
        let detailsHtml = '';
        
        // Show group information if available
        if (payload.group_name) {
          detailsHtml += `<div style="margin-bottom: 8px;"><strong>Group:</strong> ${escapeHtml(payload.group_name)}</div>`;
        }
        
        // Show creator information if available
        if (payload.created_by_username) {
          detailsHtml += `<div style="margin-bottom: 8px;"><strong>Created by:</strong> @${escapeHtml(payload.created_by_username)}`;
          if (payload.created_by_display_name) {
            detailsHtml += ` (${escapeHtml(payload.created_by_display_name)})`;
          }
          detailsHtml += `</div>`;
        }
        
        // Show event time information if available
        if (payload.start && payload.end) {
          detailsHtml += `<div style="margin-bottom: 8px;"><strong>Time:</strong> ${formatDateTime(payload.start)} - ${formatDateTime(payload.end)}</div>`;
        }
        
        // Show privacy information if available
        if (payload.privacy) {
          detailsHtml += `<div style="margin-bottom: 8px;"><strong>Privacy:</strong> ${payload.privacy}</div>`;
        }
        
        // Show status information if available
        if (payload.status) {
          detailsHtml += `<div style="margin-bottom: 8px;"><strong>Status:</strong> ${payload.status}</div>`;
        }
        
        if (detailsHtml) {
          additionalDetails.innerHTML = detailsHtml;
          additionalDetails.style.display = 'block';
        } else {
          additionalDetails.style.display = 'none';
        }
      }
      
      // Show/hide sections based on notification type
      const appointmentDetailsSection = $('appointmentDetailsSection');
      const invitationActionsSection = $('invitationActionsSection');
      
      if (notification.type === 'invite' && payload.appointment_id) {
        // Load appointment details
        try {
          const response = await api(`/api/appointments/${payload.appointment_id}`);
          const { appointment, participants } = response;
          
          if (appointment) {
            $('appointmentDetails').innerHTML = `
              <div style="margin-bottom: 8px;"><strong>Title:</strong> ${escapeHtml(appointment.title)}</div>
              <div style="margin-bottom: 8px;"><strong>Description:</strong> ${escapeHtml(appointment.description || 'No description')}</div>
              <div style="margin-bottom: 8px;"><strong>Start:</strong> ${formatDateTime(appointment.start)}</div>
              <div style="margin-bottom: 8px;"><strong>End:</strong> ${formatDateTime(appointment.end)}</div>
              <div><strong>Privacy:</strong> ${appointment.privacy}</div>
            `;
            appointmentDetailsSection.style.display = 'block';
            
            // Get current user's participant status
            const myParticipation = participants.find(p => p.user_id === state.user.id);
            const myStatus = myParticipation ? myParticipation.status : null;
            
            // Show/hide action buttons based on status
            const statusSection = $('invitationStatusSection');
            const statusMessage = $('invitationStatusMessage');
            
            if (myStatus === 'pending') {
              invitationActionsSection.style.display = 'block';
              statusSection.style.display = 'none';
            } else {
              invitationActionsSection.style.display = 'none';
              statusSection.style.display = 'block';
              
              if (myStatus === 'auto') {
                statusMessage.textContent = 'This invitation was automatically accepted (lower rank member)';
              } else if (myStatus === 'accepted') {
                statusMessage.textContent = 'You have already accepted this invitation';
              } else if (myStatus === 'declined') {
                statusMessage.textContent = 'You have declined this invitation';
              } else {
                statusSection.style.display = 'none';
              }
            }
          }
        } catch (e) {
          console.error('Failed to load appointment details:', e);
          appointmentDetailsSection.style.display = 'none';
          invitationActionsSection.style.display = 'none';
        }
      } else {
        appointmentDetailsSection.style.display = 'none';
        invitationActionsSection.style.display = 'none';
      }
      
      // Show modal
      $('notificationDetailsModal').classList.add('show');
      $('notificationDetailsModal').style.display = 'flex';
      
    } catch (e) {
      console.error('Failed to show notification details:', e);
      alert('Failed to load notification details: ' + e.message);
    }
  }

  function hideNotificationDetailsModal() {
    $('notificationDetailsModal').classList.remove('show');
    $('notificationDetailsModal').style.display = 'none';
    state.currentNotification = null;
  }

  async function acceptInvitation() {
    if (!state.currentNotification) return;
    
    try {
      const payload = JSON.parse(state.currentNotification.payload || '{}');
      if (!payload.appointment_id) {
        alert('Invalid notification data');
        return;
      }
      
      await api(`/api/appointments/${payload.appointment_id}/accept`, {
        method: 'POST'
      });
      
      console.log('Invitation accepted successfully');
      alert('Invitation accepted successfully!');
      
      // Hide modal and refresh notifications
      hideNotificationDetailsModal();
      await loadNotifications();
      await loadEvents(); // Refresh calendar
      
    } catch (e) {
      console.error('Failed to accept invitation:', e);
      alert('Failed to accept invitation: ' + e.message);
    }
  }

  async function rejectInvitation() {
    if (!state.currentNotification) return;
    
    const confirmed = confirm('Are you sure you want to reject this invitation?');
    if (!confirmed) return;
    
    try {
      const payload = JSON.parse(state.currentNotification.payload || '{}');
      if (!payload.appointment_id) {
        alert('Invalid notification data');
        return;
      }
      
      await api(`/api/appointments/${payload.appointment_id}/reject`, {
        method: 'POST'
      });
      
      console.log('Invitation rejected successfully');
      alert('Invitation rejected successfully!');
      
      // Hide modal and refresh notifications
      hideNotificationDetailsModal();
      await loadNotifications();
      await loadEvents(); // Refresh calendar
      
    } catch (e) {
      console.error('Failed to reject invitation:', e);
      alert('Failed to reject invitation: ' + e.message);
    }
  }

  function getNotificationTypeLabel(type) {
    const labels = {
      'invite': 'Event Invitation',
      'created': 'Event Created',
      'accepted': 'Invitation Accepted',
      'declined': 'Invitation Declined',
      'invitation_accepted': 'Invitation Accepted',
      'invitation_declined': 'Invitation Declined',
      'group_created': 'Group Created',
      'group_invite': 'Group Invitation'
    };
    return labels[type] || type;
  }

  function getNotificationMessage(notification, payload) {
    switch (notification.type) {
      case 'invite':
        if (payload.group_name && payload.created_by_username) {
          return `You have been invited to "${payload.title}" in group "${payload.group_name}" by @${payload.created_by_username}`;
        } else if (payload.title) {
          return `You have been invited to "${payload.title}"`;
        }
        return `You have been invited to an event`;
      case 'created':
        if (payload.title) {
          return `A new event "${payload.title}" has been created`;
        }
        return `A new event has been created`;
      case 'accepted':
      case 'invitation_accepted':
        if (payload.title && payload.user_username) {
          return `@${payload.user_username} has accepted your invitation to "${payload.title}"`;
        }
        return `Your invitation has been accepted`;
      case 'declined':
      case 'invitation_declined':
        if (payload.title && payload.user_username) {
          return `@${payload.user_username} has declined your invitation to "${payload.title}"`;
        }
        return `Your invitation has been declined`;
      case 'group_created':
        if (payload.group_name && payload.created_by_username) {
          return `Group "${payload.group_name}" has been created by @${payload.created_by_username}`;
        }
        return `A new group has been created`;
      case 'group_invite':
        if (payload.group_name && payload.added_by_username) {
          return `You have been invited to join group "${payload.group_name}" by @${payload.added_by_username}`;
        }
        return `You have been invited to join a group`;
      default:
        return `Notification: ${notification.type}`;
    }
  }

  // ======================
  // Profile Settings Functions
  // ======================

  function showProfileSettings() {
    if (!state.user) return;
    
    // Pre-fill with current user data
    $('settingsDisplayName').value = state.user.display_name || '';
    $('settingsUsername').value = state.user.username || '';
    $('settingsEmail').value = state.user.email || '';
    $('settingsCurrentPassword').value = '';
    $('settingsNewPassword').value = '';
    $('settingsConfirmPassword').value = '';
    
    $('profileSettingsModal').classList.add('show');
    $('profileSettingsModal').style.display = 'flex';
  }

  function hideProfileSettings() {
    $('profileSettingsModal').classList.remove('show');
    $('profileSettingsModal').style.display = 'none';
  }

  async function saveProfileSettings() {
    try {
      const displayName = $('settingsDisplayName').value.trim();
      const username = $('settingsUsername').value.trim();
      const email = $('settingsEmail').value.trim();
      const currentPassword = $('settingsCurrentPassword').value;
      const newPassword = $('settingsNewPassword').value;
      const confirmPassword = $('settingsConfirmPassword').value;
      
      // Validate required fields
      if (!currentPassword) {
        alert('Current password is required for security');
        return;
      }
      
      if (!displayName || !username || !email) {
        alert('Display name, username, and email are required');
        return;
      }
      
      // Update profile
      const profileData = {
        display_name: displayName,
        username: username,
        email: email,
        current_password: currentPassword
      };
      
      const updatedUser = await api('/api/me/profile', {
        method: 'PUT',
        body: JSON.stringify(profileData)
      });
      
      // Update password if provided
      if (newPassword) {
        if (newPassword !== confirmPassword) {
          alert('New passwords do not match');
          return;
        }
        
        if (newPassword.length < 6) {
          alert('New password must be at least 6 characters');
          return;
        }
        
        await api('/api/me/password', {
          method: 'PUT',
          body: JSON.stringify({
            current_password: currentPassword,
            new_password: newPassword
          })
        });
        
        alert('Profile and password updated successfully!');
      } else {
        alert('Profile updated successfully!');
      }
      
      // Update local state
      state.user = updatedUser;
      $('userName').textContent = updatedUser.display_name || updatedUser.username;
      
      hideProfileSettings();
    } catch (e) {
      console.error('Failed to update profile:', e);
      alert('Failed to update profile: ' + e.message);
    }
  }

  // Initialize the app
  init();
})(); 