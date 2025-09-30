(() => {
  const state = { 
    token: null, 
    ws: null, 
    notifications: [],
    currentView: 'month', // Add this line
    currentDate: new Date(),
    selectedDate: null, // Add this for calendar picker
    events: [],
    groups: [],
    user: null,
    personalEvents: [], // Add this line
    viewingFromPersonal: false // Add this line
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
    
    // Event details
    $('closeEventDetailsModal').onclick = hideEventDetailsModal;
    $('closeEventDetails').onclick = hideEventDetailsModal;
    
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

  async function openGroupDetails(groupId) {
    try {
      if (!state.token) { showAuthModal('login'); return; }
      const res = await api(`/api/groups/${groupId}`);
      const { group, members } = res || {};
      if (!group) return;
      
      $('groupDetailsTitle').textContent = group.name;
      $('groupDetailsDesc').textContent = group.description || '';
      
      const list = $('groupMembersList');
      list.innerHTML = '';
      
      // Render members with user_id and rank
      for (const m of members) {
        const row = document.createElement('div');
        row.style.display = 'flex';
        row.style.justifyContent = 'space-between';
        row.style.padding = '6px 8px';
        row.style.borderBottom = '1px solid #eee';
        row.innerHTML = `<span>User #${m.user_id}</span><span>Rank: ${m.rank}</span>`;
        list.appendChild(row);
      }
      
      // Bind add member action
      const addBtn = $('addMemberBtn');
      addBtn.onclick = async () => {
        try {
          const userId = Number(($('newMemberUserId').value || '').trim());
          const rank = Number(($('newMemberRank').value || '').trim());
          if (!userId || Number.isNaN(userId)) { 
            alert('Enter a valid user ID'); 
            return; 
          }
          if (Number.isNaN(rank)) { 
            alert('Enter a valid rank'); 
            return; 
          }
          
          await api(`/api/groups/${groupId}/members`, { 
            method: 'POST', 
            body: JSON.stringify({ user_id: userId, rank }) 
          });
          
          // Clear form
          $('newMemberUserId').value = '';
          $('newMemberRank').value = '';
          
          // Reload details
          await openGroupDetails(groupId);
        } catch (e) {
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

  // Initialize the app
  init();
})();