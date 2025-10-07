// http_handlers_scaffold.go
package agendadistribuida

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// This file shows how to wire handlers to services (instead of Storage).
// You can gradually migrate existing handlers to use services.

type API struct {
	router     *mux.Router
	auth       AuthService
	groups     GroupService
	apps       AppointmentService
	agenda     AgendaService
	notes      NotificationService
	users      UserRepository
	groupsRepo GroupRepository // Add direct access to group repository
}

// NewAPI builds a router backed by services. Implementation details
// (e.g., auth middleware) can delegate to existing helpers.
func NewAPI(
	auth AuthService,
	groups GroupService,
	apps AppointmentService,
	agenda AgendaService,
	notes NotificationService,
	users UserRepository,
	groupsRepo GroupRepository, // Add group repository parameter
) *API {
	r := mux.NewRouter()
	api := &API{
		router:     r,
		auth:       auth,
		groups:     groups,
		apps:       apps,
		agenda:     agenda,
		notes:      notes,
		users:      users,
		groupsRepo: groupsRepo, // Store group repository
	}

	// Public
	r.HandleFunc("/register", api.handleRegister()).Methods("POST")
	r.HandleFunc("/login", api.handleLogin()).Methods("POST")

	// Protected
	protected := r.PathPrefix("/api").Subrouter()
	protected.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "missing token", http.StatusUnauthorized)
				return
			}
			// Expecting "Bearer <token>"
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				http.Error(w, "invalid authorization header", http.StatusUnauthorized)
				return
			}
			tokenStr := parts[1]
			claims, err := api.auth.ParseToken(tokenStr)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			ctx := SetUserContext(r.Context(), claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	// Routes protegidas
	protected.HandleFunc("/groups", api.handleCreateGroup()).Methods("POST")
	protected.HandleFunc("/groups/{groupID}/members", api.handleAddMember()).Methods("POST")
	protected.HandleFunc("/appointments", api.handleCreateAppointment()).Methods("POST")
	protected.HandleFunc("/appointments/{appointmentID}", api.handleUpdateAppointment()).Methods("PUT")
	protected.HandleFunc("/appointments/{appointmentID}", api.handleDeleteAppointment()).Methods("DELETE")
	protected.HandleFunc("/agenda", api.handleGetUserAgenda()).Methods("GET")
	protected.HandleFunc("/groups/{groupID}/agenda", api.handleGetGroupAgenda()).Methods("GET")
	// Notifications
	protected.HandleFunc("/notifications", api.handleListNotifications()).Methods("GET")
	protected.HandleFunc("/notifications/unread", api.handleListUnreadNotifications()).Methods("GET")
	protected.HandleFunc("/notifications/{id}/read", api.handleMarkNotificationRead()).Methods("POST")
	// NEW endpoints used by UI
	protected.HandleFunc("/me", api.handleMe()).Methods("GET")
	protected.HandleFunc("/groups", api.handleListMyGroups()).Methods("GET")
	protected.HandleFunc("/groups/{groupID}", api.handleGetGroupDetail()).Methods("GET")
	// Appointment Details
	protected.HandleFunc("/appointments/{appointmentID}", api.handleGetAppointmentDetails()).Methods("GET")

	return api
}

func (a *API) Router() *mux.Router { return a.router }

func (a *API) handleRegister() http.HandlerFunc {
	type req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Name        string `json:"name"` // UI may send this field instead
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		in.Username = strings.TrimSpace(in.Username)
		in.Email = strings.TrimSpace(in.Email)
		in.DisplayName = strings.TrimSpace(in.DisplayName)
		in.Name = strings.TrimSpace(in.Name)
		if in.Username == "" || in.Password == "" {
			http.Error(w, "username and password are required", http.StatusBadRequest)
			return
		}
		if _, err := mail.ParseAddress(in.Email); err != nil {
			http.Error(w, "invalid email", http.StatusBadRequest)
			return
		}
		hash, err := a.auth.HashPassword(in.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		display := in.DisplayName
		if display == "" {
			display = in.Name
		}
		u := &User{Username: in.Username, Email: in.Email, DisplayName: display, PasswordHash: hash}
		if err := a.users.CreateUser(u); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// issue token so UI can auto-login after register
		token, err := a.auth.GenerateToken(u)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user":  u,
			"token": token,
		})
	}
}

func (a *API) handleLogin() http.HandlerFunc {
	type req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		user, token, err := a.auth.Authenticate(in.Username, in.Password)
		if err != nil {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user":  user,
			"token": token,
		})
	}
}

func (a *API) handleCreateGroup() http.HandlerFunc {
	type req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		uid, _ := GetUserIDFromContext(r.Context())
		user, _ := a.users.GetUserByID(uid)
		g := &Group{
			Name:            in.Name,
			Description:     in.Description,
			CreatorID:       user.ID,
			CreatorUserName: user.Username,
		}
		if err := a.groupsRepo.CreateGroup(g); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Añadir como miembro con mayor rango
		_ = a.groupsRepo.AddGroupMember(g.ID, user.ID, 10, nil)
		json.NewEncoder(w).Encode(g)
	}
}

func (a *API) handleAddMember() http.HandlerFunc {
	type req struct {
		Username string `json:"username"` // Cambiado a username
		Rank     int    `json:"rank"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Validate input
		if in.Username == "" {
			http.Error(w, "Username is required", http.StatusBadRequest)
			return
		}
		if in.Rank <= 0 {
			http.Error(w, "Rank must be a positive number", http.StatusBadRequest)
			return
		}
		vars := mux.Vars(r)
		groupID := parseID(vars["groupID"])
		actorID, _ := GetUserIDFromContext(r.Context())

		// Buscar el usuario por username
		user, err := a.users.GetUserByUsername(in.Username)
		if err != nil || user == nil {
			http.Error(w, "User not found", http.StatusBadRequest)
			return
		}

		// Validar que no sea ya miembro
		if _, err := a.groupsRepo.GetMemberRank(groupID, user.ID); err == nil {
			http.Error(w, "User is already a member", http.StatusBadRequest)
			return
		}

		if err := a.groups.AddMember(actorID, groupID, user.ID, in.Rank); err != nil {
			if err == ErrUnauthorized {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp := map[string]interface{}{
			"status":   "member added",
			"group_id": groupID,
			"user_id":  user.ID,
			"username": user.Username,
			"rank":     in.Rank,
			"added_by": actorID,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func (a *API) handleCreateAppointment() http.HandlerFunc {
	type req struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Start       string  `json:"start"`
		End         string  `json:"end"`
		Privacy     Privacy `json:"privacy"`
		GroupID     *int64  `json:"group_id,omitempty"`
	}
	// Acepta múltiples formatos de fecha/hora
	toRFC3339 := func(v string, end bool) (time.Time, error) {
		v = strings.TrimSpace(v)
		if v == "" {
			return time.Time{}, fmt.Errorf("missing time")
		}
		formats := []string{
			time.RFC3339, // "2006-01-02T15:04:05Z07:00"
			"2006-01-02T15:04:05.000Z07:00",
			"2006-01-02T15:04:05.000",
			"2006-01-02T15:04:05Z07:00",
			"2006-01-02T15:04:05",
			"2006-01-02T15:04Z07:00",
			"2006-01-02T15:04",
			"2006-01-02",
		}
		var t time.Time
		var err error
		for _, layout := range formats {
			t, err = time.Parse(layout, v)
			if err == nil {
				if layout == "2006-01-02" {
					if end {
						return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 0, 0, time.UTC), nil
					}
					return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
				}
				return t, nil
			}
		}
		log.Printf("[toRFC3339] No se pudo parsear '%s': %v", v, err)
		return time.Time{}, fmt.Errorf("invalid time format: %s", v)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("[handleCreateAppointment] === INICIO HANDLER ===")
		bodyBytes, _ := io.ReadAll(r.Body)
		log.Printf("[handleCreateAppointment] Raw body: %s", string(bodyBytes))
		r.Body = io.NopCloser(strings.NewReader(string(bodyBytes))) // reset body for decoder

		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			log.Printf("[handleCreateAppointment] Error decodificando JSON: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("[handleCreateAppointment] JSON recibido: %+v", in)
		uid, _ := GetUserIDFromContext(r.Context())
		start, err := toRFC3339(in.Start, false)
		if err != nil {
			log.Printf("[handleCreateAppointment] Fecha start inválida: '%s'", in.Start)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		end, err := toRFC3339(in.End, true)
		if err != nil {
			log.Printf("[handleCreateAppointment] Fecha end inválida: '%s'", in.End)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		privacy := in.Privacy
		if privacy == "" {
			privacy = PrivacyFull
		}
		appt := Appointment{
			Title: in.Title, Description: in.Description,
			OwnerID: uid, Start: start, End: end,
			Privacy: privacy, GroupID: in.GroupID,
		}
		log.Printf("[handleCreateAppointment] Cita a crear: %+v", appt)
		if in.GroupID != nil {
			created, parts, err := a.apps.CreateGroupAppointment(uid, appt)
			if err != nil {
				log.Printf("[handleCreateAppointment] Error creando cita de grupo: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"appointment": created, "participants": parts})
		} else {
			created, err := a.apps.CreatePersonalAppointment(uid, appt)
			if err != nil {
				log.Printf("[handleCreateAppointment] Error creando cita personal: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(created)
		}
	}
}

func (a *API) handleGetUserAgenda() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, _ := GetUserIDFromContext(r.Context())
		start, end := parseTimeRange(r) // helper: parse query params "start", "end"
		apps, err := a.agenda.GetUserAgendaForViewer(uid, start, end)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(apps)
	}
}

func (a *API) handleGetGroupAgenda() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, _ := GetUserIDFromContext(r.Context())
		vars := mux.Vars(r)
		groupID := parseID(vars["groupID"])
		start, end := parseTimeRange(r)
		apps, err := a.agenda.GetGroupAgendaForViewer(uid, groupID, start, end)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(apps)
	}
}

// 🔔 Notifications handlers
func (a *API) handleListNotifications() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, _ := GetUserIDFromContext(r.Context())
		items, err := a.notes.List(uid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(items)
	}
}

func (a *API) handleListUnreadNotifications() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, _ := GetUserIDFromContext(r.Context())
		items, err := a.notes.ListUnread(uid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(items)
	}
}

func (a *API) handleMarkNotificationRead() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := parseID(vars["id"])
		if err := a.notes.MarkRead(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// NEW: return current authenticated user (used by UI auto-login)
func (a *API) handleMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, _ := GetUserIDFromContext(r.Context())
		u, err := a.users.GetUserByID(uid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(u)
	}
}

// NEW: list groups for current user
func (a *API) handleListMyGroups() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, _ := GetUserIDFromContext(r.Context())
		groups, err := a.groupsRepo.GetGroupsForUser(uid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(groups)
	}
}

// NEW: get group details + members
func (a *API) handleGetGroupDetail() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		gid := parseID(vars["groupID"])

		// Get group details
		group, err := a.groupsRepo.GetGroupByID(gid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		// Get group members
		members, err := a.groupsRepo.GetGroupMembers(gid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"group":   group,
			"members": members,
		})
	}
}

// handleGetAppointmentDetails retrieves detailed information about an appointment
func (a *API) handleGetAppointmentDetails() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, _ := GetUserIDFromContext(r.Context())
		vars := mux.Vars(r)
		appointmentID := parseID(vars["appointmentID"])
		if appointmentID == 0 {
			http.Error(w, "Invalid appointment ID", http.StatusBadRequest)
			return
		}

		// Get appointment details
		appointment, err := a.apps.GetAppointmentByID(appointmentID)
		if err != nil {
			http.Error(w, "Appointment not found", http.StatusNotFound)
			return
		}

		// Check if user has permission to view this appointment
		hasPermission := false

		// Owner can always view
		if appointment.OwnerID == userID {
			hasPermission = true
		}

		// For group appointments, check if user is a member or has hierarchy access
		if appointment.GroupID != nil {
			// Check if user is a participant
			participants, err := a.apps.GetAppointmentParticipants(appointmentID)
			if err == nil {
				for _, p := range participants {
					if p.UserID == userID {
						hasPermission = true
						break
					}
				}
			}

			// Check if user has superior rank in the group
			if !hasPermission {
				superior, _ := a.groupsRepo.IsSuperior(*appointment.GroupID, userID, appointment.OwnerID)
				if superior {
					hasPermission = true
				}
			}
		}

		if !hasPermission {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		// Get participants
		participants, err := a.apps.GetAppointmentParticipants(appointmentID)
		if err != nil {
			http.Error(w, "Error loading participants", http.StatusInternalServerError)
			return
		}

		// Apply privacy filter
		filteredAppointment := a.filterAppointmentForViewer(*appointment, userID, appointment.GroupID)

		response := map[string]interface{}{
			"appointment":  filteredAppointment,
			"participants": participants,
		}

		json.NewEncoder(w).Encode(response)
	}
}

// filterAppointmentForViewer applies privacy filtering based on viewer, owner, and hierarchy
func (a *API) filterAppointmentForViewer(appointment Appointment, viewerID int64, groupID *int64) Appointment {
	// Owner always sees everything
	if appointment.OwnerID == viewerID {
		return appointment
	}

	// If it's a group appointment, check hierarchy
	if groupID != nil && appointment.GroupID != nil {
		superior, _ := a.groupsRepo.IsSuperior(*appointment.GroupID, viewerID, appointment.OwnerID)
		if superior {
			return appointment // superiors see details
		}
	}

	// If privacy is FreeBusy or the viewer doesn't have privileges -> hide details
	if appointment.Privacy == PrivacyFreeBusy {
		appointment.Title = "Busy"
		appointment.Description = ""
	}
	return appointment
}

// handleUpdateAppointment handles PUT /api/appointments/{appointmentID}
func (a *API) handleUpdateAppointment() http.HandlerFunc {
	type req struct {
		Title       string    `json:"title"`
		Description string    `json:"description"`
		Start       time.Time `json:"start"`
		End         time.Time `json:"end"`
		Privacy     Privacy   `json:"privacy"`
		GroupID     *int64    `json:"group_id,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		vars := mux.Vars(r)
		appointmentID := parseID(vars["appointmentID"])
		if appointmentID == 0 {
			http.Error(w, "invalid appointment ID", http.StatusBadRequest)
			return
		}

		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		appointment := Appointment{
			ID:          appointmentID,
			Title:       in.Title,
			Description: in.Description,
			Start:       in.Start,
			End:         in.End,
			Privacy:     in.Privacy,
			GroupID:     in.GroupID,
		}

		updated, err := a.apps.UpdateAppointment(userID, appointment)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(updated)
	}
}

// handleDeleteAppointment handles DELETE /api/appointments/{appointmentID}
func (a *API) handleDeleteAppointment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		vars := mux.Vars(r)
		appointmentID := parseID(vars["appointmentID"])
		if appointmentID == 0 {
			http.Error(w, "invalid appointment ID", http.StatusBadRequest)
			return
		}

		err := a.apps.DeleteAppointment(userID, appointmentID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	}
}
