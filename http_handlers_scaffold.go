// http_handlers_scaffold.go
package agendadistribuida

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
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
	groupsRepo GroupRepository       // Add direct access to group repository
	appsRepo   AppointmentRepository // Add direct access to appointment repository
	logger     *slog.Logger
	auditRepo  AuditRepository
	auditToken string
	cons       Consensus
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (rr *responseRecorder) WriteHeader(status int) {
	rr.status = status
	rr.ResponseWriter.WriteHeader(status)
}

func (a *API) requestIDMiddleware() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, reqID := WithRequestID(r.Context())
			w.Header().Set("X-Request-ID", reqID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (a *API) loggingMiddleware() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rec, r)
			duration := time.Since(start)
			a.logger.Info("http_request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", duration.Milliseconds(),
				"request_id", RequestIDFromContext(r.Context()))
		})
	}
}

func (a *API) log(ctx context.Context, level slog.Level, msg string, attrs ...any) {
	if ctx == nil {
		ctx = context.Background()
	}
	attrs = append(attrs, "request_id", RequestIDFromContext(ctx))
	switch level {
	case slog.LevelDebug:
		a.logger.Debug(msg, attrs...)
	case slog.LevelWarn:
		a.logger.Warn(msg, attrs...)
	case slog.LevelError:
		a.logger.Error(msg, attrs...)
	default:
		a.logger.Info(msg, attrs...)
	}
}

func (a *API) recordAudit(ctx context.Context, component, action, message string, fields map[string]any) {
	RecordAudit(ctx, AuditLevelInfo, component, action, message, fields)
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
	appsRepo AppointmentRepository, // Add appointment repository parameter
	auditRepo AuditRepository,
	cons Consensus,
) *API {
	r := mux.NewRouter()
	logger := Logger()
	api := &API{
		router:     r,
		auth:       auth,
		groups:     groups,
		apps:       apps,
		agenda:     agenda,
		notes:      notes,
		users:      users,
		groupsRepo: groupsRepo, // Store group repository
		appsRepo:   appsRepo,   // Store appointment repository
		logger:     logger,
		auditRepo:  auditRepo,
		auditToken: os.Getenv("AUDIT_API_TOKEN"),
		cons:       cons,
	}

	r.Use(api.requestIDMiddleware())
	r.Use(api.loggingMiddleware())

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
	protected.HandleFunc("/groups/{groupID}", api.handleUpdateGroup()).Methods("PUT")
	protected.HandleFunc("/groups/{groupID}", api.handleDeleteGroup()).Methods("DELETE")
	protected.HandleFunc("/groups/{groupID}/members", api.handleAddMember()).Methods("POST")
	protected.HandleFunc("/groups/{groupID}/members/{userID}", api.handleUpdateMember()).Methods("PUT")
	protected.HandleFunc("/groups/{groupID}/members/{userID}", api.handleRemoveMember()).Methods("DELETE")
	protected.HandleFunc("/appointments", api.handleCreateAppointment()).Methods("POST")
	protected.HandleFunc("/appointments/{appointmentID}", api.handleUpdateAppointment()).Methods("PUT")
	protected.HandleFunc("/appointments/{appointmentID}", api.handleDeleteAppointment()).Methods("DELETE")
	protected.HandleFunc("/agenda", api.handleGetUserAgenda()).Methods("GET")
	protected.HandleFunc("/groups/{groupID}/agenda", api.handleGetGroupAgenda()).Methods("GET")
	// Notifications
	protected.HandleFunc("/notifications", api.handleListNotifications()).Methods("GET")
	protected.HandleFunc("/notifications/unread", api.handleListUnreadNotifications()).Methods("GET")
	protected.HandleFunc("/notifications/{id}/read", api.handleMarkNotificationRead()).Methods("POST")
	protected.HandleFunc("/appointments/{appointmentID}/accept", api.handleAcceptInvitation()).Methods("POST")
	protected.HandleFunc("/appointments/{appointmentID}/reject", api.handleRejectInvitation()).Methods("POST")
	protected.HandleFunc("/appointments/{appointmentID}/my-status", api.handleGetMyParticipationStatus()).Methods("GET")
	// NEW endpoints used by UI
	protected.HandleFunc("/me", api.handleMe()).Methods("GET")
	protected.HandleFunc("/me/profile", api.handleUpdateProfile()).Methods("PUT")
	protected.HandleFunc("/me/password", api.handleUpdatePassword()).Methods("PUT")
	protected.HandleFunc("/groups", api.handleListMyGroups()).Methods("GET")
	protected.HandleFunc("/groups/{groupID}", api.handleGetGroupDetail()).Methods("GET")
	// Appointment Details
	protected.HandleFunc("/appointments/{appointmentID}", api.handleGetAppointmentDetails()).Methods("GET")
	if api.auditRepo != nil && api.auditToken != "" {
		protected.HandleFunc("/admin/audit/logs", api.handleListAuditLogs()).Methods("GET")
	}

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
		ctx := r.Context()
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			a.log(ctx, slog.LevelWarn, "register_decode_failed", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		in.Username = strings.TrimSpace(in.Username)
		in.Email = strings.TrimSpace(in.Email)
		in.DisplayName = strings.TrimSpace(in.DisplayName)
		in.Name = strings.TrimSpace(in.Name)
		if in.Username == "" || in.Password == "" {
			a.log(ctx, slog.LevelWarn, "register_invalid_input", "username", in.Username)
			http.Error(w, "username and password are required", http.StatusBadRequest)
			return
		}
		if _, err := mail.ParseAddress(in.Email); err != nil {
			a.log(ctx, slog.LevelWarn, "register_invalid_email", "email", in.Email, "err", err)
			http.Error(w, "invalid email", http.StatusBadRequest)
			return
		}
		hash, err := a.auth.HashPassword(in.Password)
		if err != nil {
			a.log(ctx, slog.LevelError, "register_hash_failed", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		display := in.DisplayName
		if display == "" {
			display = in.Name
		}
		u := &User{Username: in.Username, Email: in.Email, DisplayName: display, PasswordHash: hash}
		// If consensus is wired and this node is leader, replicate user via Raft
		if a.cons != nil && a.cons.IsLeader() {
			entry, err := BuildEntryUserCreate(u)
			if err != nil {
				a.log(ctx, slog.LevelError, "register_build_entry_failed", "err", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if err := a.cons.Propose(entry); err != nil {
				a.log(ctx, slog.LevelError, "register_propose_failed", "err", err)
				http.Error(w, "failed to replicate user", http.StatusInternalServerError)
				return
			}
			// After commit, load the user from storage to get its ID
			if created, err := a.users.GetUserByUsername(u.Username); err == nil && created != nil {
				u = created
			}
		} else {
			if err := a.users.CreateUser(u); err != nil {
				a.log(ctx, slog.LevelWarn, "register_create_failed", "err", err, "username", in.Username)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
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
		ctx = SetUserContext(ctx, u.ID)
		a.recordAudit(ctx, "auth", "register", "user registered", map[string]any{
			"user_id":  u.ID,
			"username": u.Username,
			"email":    u.Email,
		})
		a.log(ctx, slog.LevelInfo, "register_success", "user_id", u.ID, "username", u.Username)
	}
}

func (a *API) handleLogin() http.HandlerFunc {
	type req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			a.log(ctx, slog.LevelWarn, "login_decode_failed", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		user, token, err := a.auth.Authenticate(in.Username, in.Password)
		if err != nil {
			a.log(ctx, slog.LevelWarn, "login_failed", "username", in.Username)
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user":  user,
			"token": token,
		})
		ctx = SetUserContext(ctx, user.ID)
		a.recordAudit(ctx, "auth", "login", "user logged in", map[string]any{
			"user_id":  user.ID,
			"username": user.Username,
		})
		a.log(ctx, slog.LevelInfo, "login_success", "user_id", user.ID, "username", user.Username)
	}
}

func (a *API) handleCreateGroup() http.HandlerFunc {
	type req struct {
		Name        string    `json:"name"`
		Description string    `json:"description"`
		GroupType   GroupType `json:"group_type,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			a.log(ctx, slog.LevelWarn, "group_create_decode_failed", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		uid, _ := GetUserIDFromContext(r.Context())
		user, err := a.users.GetUserByID(uid)
		if err != nil {
			a.log(ctx, slog.LevelError, "group_create_user_lookup_failed", "err", err, "user_id", uid)
			http.Error(w, "user not found", http.StatusUnauthorized)
			return
		}

		// Validar tipo de grupo o asignar por defecto
		gt := in.GroupType
		if gt == "" {
			gt = "non_hierarchical"
		}

		g := &Group{
			Name:            in.Name,
			Description:     in.Description,
			CreatorID:       user.ID,
			CreatorUserName: user.Username,
			GroupType:       GroupType(gt), // ✅ guardar correctamente
		}

		// If consensus is wired and this node is leader, replicate group creation via Raft
		if a.cons != nil && a.cons.IsLeader() {
			entry, err := BuildEntryGroupCreate(g)
			if err != nil {
				a.log(ctx, slog.LevelError, "group_create_build_entry_failed", "err", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if err := a.cons.Propose(entry); err != nil {
				a.log(ctx, slog.LevelError, "group_create_propose_failed", "err", err)
				http.Error(w, "failed to replicate group", http.StatusInternalServerError)
				return
			}
		} else {
			// Fallback for single-node or no-consensus setups: write directly
			if err := a.groupsRepo.CreateGroup(g); err != nil {
				a.log(ctx, slog.LevelError, "group_create_failed", "err", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if gt == "hierarchical" {
				if err := a.groupsRepo.AddGroupMember(g.ID, user.ID, 10, nil); err != nil {
					a.log(ctx, slog.LevelError, "group_add_owner_failed", "err", err, "group_id", g.ID)
				}
			} else {
				if err := a.groupsRepo.AddGroupMember(g.ID, user.ID, 0, nil); err != nil {
					a.log(ctx, slog.LevelError, "group_add_owner_failed", "err", err, "group_id", g.ID)
				}
			}
		}

		json.NewEncoder(w).Encode(g)
		a.recordAudit(ctx, "group", "create", "group created", map[string]any{
			"group_id": g.ID,
			"user_id":  user.ID,
			"type":     gt,
		})
		a.log(ctx, slog.LevelInfo, "group_create_success", "group_id", g.ID, "type", gt)
	}
}

func (a *API) handleAddMember() http.HandlerFunc {
	type req struct {
		Username string `json:"username"` // Cambiado a username
		Rank     int    `json:"rank"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			a.log(ctx, slog.LevelWarn, "group_member_add_decode_failed", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Validate input
		if in.Username == "" {
			a.log(ctx, slog.LevelWarn, "group_member_add_username_missing")
			http.Error(w, "Username is required", http.StatusBadRequest)
			return
		}
		if in.Rank < 0 {
			a.log(ctx, slog.LevelWarn, "group_member_add_bad_rank", "rank", in.Rank)
			http.Error(w, "Rank must be a positive number", http.StatusBadRequest)
			return
		}
		vars := mux.Vars(r)
		groupID := parseID(vars["groupID"])
		actorID, _ := GetUserIDFromContext(r.Context())

		// Buscar el usuario por username
		user, err := a.users.GetUserByUsername(in.Username)
		if err != nil || user == nil {
			a.log(ctx, slog.LevelWarn, "group_member_add_user_not_found", "username", in.Username)
			http.Error(w, "User not found", http.StatusBadRequest)
			return
		}

		// Validar que no sea ya miembro
		if _, err := a.groupsRepo.GetMemberRank(groupID, user.ID); err == nil {
			a.log(ctx, slog.LevelWarn, "group_member_add_duplicate", "group_id", groupID, "user_id", user.ID)
			http.Error(w, "User is already a member", http.StatusBadRequest)
			return
		}

		if err := a.groups.AddMember(actorID, groupID, user.ID, in.Rank); err != nil {
			if err == ErrUnauthorized {
				a.log(ctx, slog.LevelWarn, "group_member_add_unauthorized", "group_id", groupID, "actor_id", actorID)
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			a.log(ctx, slog.LevelError, "group_member_add_failed", "err", err, "group_id", groupID)
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
		a.recordAudit(ctx, "group", "add_member", "group member added", map[string]any{
			"group_id": groupID,
			"user_id":  user.ID,
			"actor_id": actorID,
			"rank":     in.Rank,
		})
		a.log(ctx, slog.LevelInfo, "group_member_add_success", "group_id", groupID, "user_id", user.ID)
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
		return time.Time{}, fmt.Errorf("invalid time format: %s", v)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			a.log(ctx, slog.LevelWarn, "appointment_create_decode_failed", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		uid, _ := GetUserIDFromContext(r.Context())
		start, err := toRFC3339(in.Start, false)
		if err != nil {
			a.log(ctx, slog.LevelWarn, "appointment_create_invalid_start", "start", in.Start)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		end, err := toRFC3339(in.End, true)
		if err != nil {
			a.log(ctx, slog.LevelWarn, "appointment_create_invalid_end", "end", in.End)
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
		var payload map[string]any
		if in.GroupID != nil {
			created, parts, err := a.apps.CreateGroupAppointment(uid, appt)
			if err != nil {
				a.log(ctx, slog.LevelWarn, "appointment_create_group_failed", "err", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"appointment": created, "participants": parts})
			payload = map[string]any{"appointment_id": created.ID, "group_id": in.GroupID}
		} else {
			created, err := a.apps.CreatePersonalAppointment(uid, appt)
			if err != nil {
				a.log(ctx, slog.LevelWarn, "appointment_create_personal_failed", "err", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(created)
			payload = map[string]any{"appointment_id": created.ID}
		}
		a.recordAudit(ctx, "appointment", "create", "appointment created", payload)
		a.log(ctx, slog.LevelInfo, "appointment_create_success", "group_id", in.GroupID)
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

func (a *API) handleListAuditLogs() http.HandlerFunc {
	type auditResponse struct {
		Logs []AuditLog `json:"logs"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if a.auditRepo == nil || a.auditToken == "" {
			http.Error(w, "audit endpoint disabled", http.StatusNotFound)
			return
		}
		headerToken := r.Header.Get("X-Audit-Token")
		if subtle.ConstantTimeCompare([]byte(a.auditToken), []byte(headerToken)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		q := r.URL.Query()
		filter := AuditFilter{
			Component: q.Get("component"),
			Action:    q.Get("action"),
			Level:     q.Get("level"),
			RequestID: q.Get("request_id"),
		}
		if since := q.Get("since"); since != "" {
			ts, err := time.Parse(time.RFC3339, since)
			if err != nil {
				http.Error(w, "invalid since (RFC3339 required)", http.StatusBadRequest)
				return
			}
			filter.Since = ts
		}
		if limit := q.Get("limit"); limit != "" {
			val, err := strconv.Atoi(limit)
			if err != nil || val <= 0 {
				http.Error(w, "invalid limit", http.StatusBadRequest)
				return
			}
			filter.Limit = val
		}
		logs, err := a.auditRepo.ListAuditLogs(filter)
		if err != nil {
			a.log(r.Context(), slog.LevelError, "audit_logs_fetch_failed", "err", err)
			http.Error(w, "failed to fetch audit logs", http.StatusInternalServerError)
			return
		}
		a.recordAudit(r.Context(), "observability", "audit_logs_list", "audit logs fetched", map[string]any{
			"component": filter.Component,
			"count":     len(logs),
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(auditResponse{Logs: logs})
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
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(r.Context())
		if !ok {
			a.log(ctx, slog.LevelWarn, "appointment_update_unauthorized")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		vars := mux.Vars(r)
		appointmentID := parseID(vars["appointmentID"])
		if appointmentID == 0 {
			a.log(ctx, slog.LevelWarn, "appointment_update_invalid_id")
			http.Error(w, "invalid appointment ID", http.StatusBadRequest)
			return
		}

		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			a.log(ctx, slog.LevelWarn, "appointment_update_decode_failed", "err", err)
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
			a.log(ctx, slog.LevelError, "appointment_update_failed", "err", err, "appointment_id", appointmentID)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(updated)
		a.recordAudit(ctx, "appointment", "update", "appointment updated", map[string]any{
			"appointment_id": appointmentID,
			"user_id":        userID,
		})
		a.log(ctx, slog.LevelInfo, "appointment_update_success", "appointment_id", appointmentID)
	}
}

// handleDeleteAppointment handles DELETE /api/appointments/{appointmentID}
func (a *API) handleDeleteAppointment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(r.Context())
		if !ok {
			a.log(ctx, slog.LevelWarn, "appointment_delete_unauthorized")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		vars := mux.Vars(r)
		appointmentID := parseID(vars["appointmentID"])
		if appointmentID == 0 {
			a.log(ctx, slog.LevelWarn, "appointment_delete_invalid_id")
			http.Error(w, "invalid appointment ID", http.StatusBadRequest)
			return
		}

		err := a.apps.DeleteAppointment(userID, appointmentID)
		if err != nil {
			a.log(ctx, slog.LevelError, "appointment_delete_failed", "err", err, "appointment_id", appointmentID)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		a.recordAudit(ctx, "appointment", "delete", "appointment deleted", map[string]any{
			"appointment_id": appointmentID,
			"user_id":        userID,
		})
		a.log(ctx, slog.LevelInfo, "appointment_delete_success", "appointment_id", appointmentID)
	}
}

// handleUpdateGroup handles PUT /api/groups/{groupID}
func (a *API) handleUpdateGroup() http.HandlerFunc {
	type req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(r.Context())
		if !ok {
			a.log(ctx, slog.LevelWarn, "group_update_unauthorized")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		vars := mux.Vars(r)
		groupID := parseID(vars["groupID"])
		if groupID == 0 {
			a.log(ctx, slog.LevelWarn, "group_update_invalid_group")
			http.Error(w, "invalid group ID", http.StatusBadRequest)
			return
		}

		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			a.log(ctx, slog.LevelWarn, "group_update_decode_failed", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Validate input - name is required if provided, but we allow partial updates
		if in.Name == "" && in.Description == "" {
			a.log(ctx, slog.LevelWarn, "group_update_empty_payload")
			http.Error(w, "At least one field (name or description) must be provided", http.StatusBadRequest)
			return
		}

		updated, err := a.groups.UpdateGroup(userID, groupID, in.Name, in.Description)
		if err != nil {
			a.log(ctx, slog.LevelError, "group_update_failed", "err", err, "group_id", groupID)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(updated)
		a.recordAudit(ctx, "group", "update", "group updated", map[string]any{
			"group_id": groupID,
			"user_id":  userID,
		})
		a.log(ctx, slog.LevelInfo, "group_update_success", "group_id", groupID)
	}
}

// handleDeleteGroup handles DELETE /api/groups/{groupID}
func (a *API) handleDeleteGroup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(r.Context())
		if !ok {
			a.log(ctx, slog.LevelWarn, "group_delete_unauthorized")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		vars := mux.Vars(r)
		groupID := parseID(vars["groupID"])
		if groupID == 0 {
			a.log(ctx, slog.LevelWarn, "group_delete_invalid_group")
			http.Error(w, "invalid group ID", http.StatusBadRequest)
			return
		}

		err := a.groups.DeleteGroup(userID, groupID)
		if err != nil {
			a.log(ctx, slog.LevelError, "group_delete_failed", "err", err, "group_id", groupID)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		a.recordAudit(ctx, "group", "delete", "group deleted", map[string]any{
			"group_id": groupID,
			"user_id":  userID,
		})
		a.log(ctx, slog.LevelInfo, "group_delete_success", "group_id", groupID)
	}
}

// handleUpdateMember handles PUT /api/groups/{groupID}/members/{userID}
func (a *API) handleUpdateMember() http.HandlerFunc {
	type req struct {
		Rank int `json:"rank"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(r.Context())
		if !ok {
			a.log(ctx, slog.LevelWarn, "group_member_update_unauthorized")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		vars := mux.Vars(r)
		groupID := parseID(vars["groupID"])
		memberUserID := parseID(vars["userID"])
		if groupID == 0 || memberUserID == 0 {
			a.log(ctx, slog.LevelWarn, "group_member_update_invalid_ids", "group_id", groupID, "member_id", memberUserID)
			http.Error(w, "invalid group or user ID", http.StatusBadRequest)
			return
		}

		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			a.log(ctx, slog.LevelWarn, "group_member_update_decode_failed", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Validate input
		if in.Rank < 0 {
			a.log(ctx, slog.LevelWarn, "group_member_update_bad_rank", "rank", in.Rank)
			http.Error(w, "Rank must be a positive number", http.StatusBadRequest)
			return
		}

		err := a.groups.UpdateMember(userID, groupID, memberUserID, in.Rank)
		if err != nil {
			if err == ErrUnauthorized {
				a.log(ctx, slog.LevelWarn, "group_member_update_forbidden", "group_id", groupID)
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			a.log(ctx, slog.LevelError, "group_member_update_failed", "err", err, "group_id", groupID)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
		a.recordAudit(ctx, "group", "update_member", "group member updated", map[string]any{
			"group_id": groupID,
			"user_id":  memberUserID,
			"actor_id": userID,
			"rank":     in.Rank,
		})
		a.log(ctx, slog.LevelInfo, "group_member_update_success", "group_id", groupID, "member_id", memberUserID)
	}
}

// handleRemoveMember handles DELETE /api/groups/{groupID}/members/{userID}
func (a *API) handleRemoveMember() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(r.Context())
		if !ok {
			a.log(ctx, slog.LevelWarn, "group_member_remove_unauthorized")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		vars := mux.Vars(r)
		groupID := parseID(vars["groupID"])
		memberUserID := parseID(vars["userID"])
		if groupID == 0 || memberUserID == 0 {
			a.log(ctx, slog.LevelWarn, "group_member_remove_invalid_ids", "group_id", groupID, "member_id", memberUserID)
			http.Error(w, "invalid group or user ID", http.StatusBadRequest)
			return
		}

		err := a.groups.RemoveMember(userID, groupID, memberUserID)
		if err != nil {
			if err == ErrUnauthorized {
				a.log(ctx, slog.LevelWarn, "group_member_remove_forbidden", "group_id", groupID)
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			a.log(ctx, slog.LevelError, "group_member_remove_failed", "err", err, "group_id", groupID)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
		a.recordAudit(ctx, "group", "remove_member", "group member removed", map[string]any{
			"group_id": groupID,
			"user_id":  memberUserID,
			"actor_id": userID,
		})
		a.log(ctx, slog.LevelInfo, "group_member_remove_success", "group_id", groupID, "member_id", memberUserID)
	}
}

// handleAcceptInvitation handles POST /api/appointments/{appointmentID}/accept
func (a *API) handleAcceptInvitation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(r.Context())
		if !ok {
			a.log(ctx, slog.LevelWarn, "invitation_accept_unauthorized")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		vars := mux.Vars(r)
		appointmentID := parseID(vars["appointmentID"])
		if appointmentID == 0 {
			a.log(ctx, slog.LevelWarn, "invitation_accept_invalid_id")
			http.Error(w, "invalid appointment ID", http.StatusBadRequest)
			return
		}

		err := a.apps.AcceptInvitation(userID, appointmentID)
		if err != nil {
			a.log(ctx, slog.LevelWarn, "invitation_accept_failed", "err", err, "appointment_id", appointmentID)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
		a.recordAudit(ctx, "appointment", "accept_invitation", "invitation accepted", map[string]any{
			"appointment_id": appointmentID,
			"user_id":        userID,
		})
		a.log(ctx, slog.LevelInfo, "invitation_accept_success", "appointment_id", appointmentID)
	}
}

// handleRejectInvitation handles POST /api/appointments/{appointmentID}/reject
func (a *API) handleRejectInvitation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID, ok := GetUserIDFromContext(r.Context())
		if !ok {
			a.log(ctx, slog.LevelWarn, "invitation_reject_unauthorized")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		vars := mux.Vars(r)
		appointmentID := parseID(vars["appointmentID"])
		if appointmentID == 0 {
			a.log(ctx, slog.LevelWarn, "invitation_reject_invalid_id")
			http.Error(w, "invalid appointment ID", http.StatusBadRequest)
			return
		}

		err := a.apps.RejectInvitation(userID, appointmentID)
		if err != nil {
			a.log(ctx, slog.LevelWarn, "invitation_reject_failed", "err", err, "appointment_id", appointmentID)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "rejected"})
		a.recordAudit(ctx, "appointment", "reject_invitation", "invitation rejected", map[string]any{
			"appointment_id": appointmentID,
			"user_id":        userID,
		})
		a.log(ctx, slog.LevelInfo, "invitation_reject_success", "appointment_id", appointmentID)
	}
}

// handleGetMyParticipationStatus handles GET /api/appointments/{appointmentID}/my-status
func (a *API) handleGetMyParticipationStatus() http.HandlerFunc {
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

		participant, err := a.appsRepo.GetParticipantByAppointmentAndUser(appointmentID, userID)
		if err != nil {
			http.Error(w, "not a participant", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": participant.Status,
		})
	}
}

// handleUpdateProfile handles PUT /api/me/profile
func (a *API) handleUpdateProfile() http.HandlerFunc {
	type req struct {
		DisplayName     string `json:"display_name"`
		Username        string `json:"username"`
		Email           string `json:"email"`
		CurrentPassword string `json:"current_password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID, _ := GetUserIDFromContext(r.Context())

		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			a.log(ctx, slog.LevelWarn, "profile_update_decode_failed", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Validate current password
		user, err := a.users.GetUserByID(userID)
		if err != nil {
			a.log(ctx, slog.LevelWarn, "profile_update_user_not_found", "user_id", userID)
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.CurrentPassword)); err != nil {
			a.log(ctx, slog.LevelWarn, "profile_update_invalid_password", "user_id", userID)
			http.Error(w, "Invalid current password", http.StatusUnauthorized)
			return
		}

		// Validate email format
		if in.Email != "" {
			if _, err := mail.ParseAddress(in.Email); err != nil {
				a.log(ctx, slog.LevelWarn, "profile_update_invalid_email", "email", in.Email)
				http.Error(w, "Invalid email format", http.StatusBadRequest)
				return
			}
		}

		// Check if username is being changed and if it's already taken
		if in.Username != user.Username {
			existing, _ := a.users.GetUserByUsername(in.Username)
			if existing != nil && existing.ID != userID {
				a.log(ctx, slog.LevelWarn, "profile_update_username_conflict", "username", in.Username)
				http.Error(w, "Username already taken", http.StatusConflict)
				return
			}
		}

		// Check if email is being changed and if it's already taken
		if in.Email != user.Email {
			existing, _ := a.users.GetUserByEmail(in.Email)
			if existing != nil && existing.ID != userID {
				a.log(ctx, slog.LevelWarn, "profile_update_email_conflict", "email", in.Email)
				http.Error(w, "Email already in use", http.StatusConflict)
				return
			}
		}

		// Update user
		user.DisplayName = in.DisplayName
		user.Username = in.Username
		user.Email = in.Email

		if err := a.users.UpdateUser(user); err != nil {
			a.log(ctx, slog.LevelError, "profile_update_failed", "err", err, "user_id", userID)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(user)
		a.recordAudit(ctx, "user", "update_profile", "profile updated", map[string]any{
			"user_id": userID,
		})
		a.log(ctx, slog.LevelInfo, "profile_update_success", "user_id", userID)
	}
}

// handleUpdatePassword handles PUT /api/me/password
func (a *API) handleUpdatePassword() http.HandlerFunc {
	type req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID, _ := GetUserIDFromContext(r.Context())

		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			a.log(ctx, slog.LevelWarn, "password_update_decode_failed", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Validate current password
		user, err := a.users.GetUserByID(userID)
		if err != nil {
			a.log(ctx, slog.LevelWarn, "password_update_user_not_found", "user_id", userID)
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.CurrentPassword)); err != nil {
			a.log(ctx, slog.LevelWarn, "password_update_invalid_current", "user_id", userID)
			http.Error(w, "Invalid current password", http.StatusUnauthorized)
			return
		}

		// Validate new password
		if len(in.NewPassword) < 6 {
			a.log(ctx, slog.LevelWarn, "password_update_too_short")
			http.Error(w, "Password must be at least 6 characters", http.StatusBadRequest)
			return
		}

		// Hash new password
		hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			a.log(ctx, slog.LevelError, "password_update_hash_failed", "err", err)
			http.Error(w, "Error hashing password", http.StatusInternalServerError)
			return
		}

		if err := a.users.UpdatePassword(userID, string(hash)); err != nil {
			a.log(ctx, slog.LevelError, "password_update_failed", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "Password updated successfully"})
		a.recordAudit(ctx, "user", "update_password", "password updated", map[string]any{
			"user_id": userID,
		})
		a.log(ctx, slog.LevelInfo, "password_update_success", "user_id", userID)
	}
}
