// http_handlers_scaffold.go
package agendadistribuida

import (
	"encoding/json"
	"net/http"
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
		g, err := a.groups.CreateGroup(uid, in.Name, in.Description)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(g)
	}
}

func (a *API) handleAddMember() http.HandlerFunc {
	type req struct {
		UserID int64 `json:"user_id"`
		Rank   int   `json:"rank"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		vars := mux.Vars(r)
		groupID := parseID(vars["groupID"])
		actorID, _ := GetUserIDFromContext(r.Context())
		if err := a.groups.AddMember(actorID, groupID, in.UserID, in.Rank); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *API) handleCreateAppointment() http.HandlerFunc {
	type req struct {
		Title       string    `json:"title"`
		Description string    `json:"description"`
		Start       time.Time `json:"start"`
		End         time.Time `json:"end"`
		Privacy     Privacy   `json:"privacy"`
		GroupID     *int64    `json:"group_id,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		uid, _ := GetUserIDFromContext(r.Context())
		appt := Appointment{
			Title: in.Title, Description: in.Description,
			OwnerID: uid, Start: in.Start, End: in.End,
			Privacy: in.Privacy, GroupID: in.GroupID,
		}
		if in.GroupID != nil {
			created, parts, err := a.apps.CreateGroupAppointment(uid, appt)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"appointment": created, "participants": parts})
		} else {
			created, err := a.apps.CreatePersonalAppointment(uid, appt)
			if err != nil {
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
