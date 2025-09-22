// http_handlers_scaffold.go
package agendadistribuida

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// This file shows how to wire handlers to services (instead of Storage).
// You can gradually migrate existing handlers to use services.

type API struct {
	router *mux.Router
	auth   AuthService
	groups GroupService
	apps   AppointmentService
	agenda AgendaService
	notes  NotificationService
}

// NewAPI builds a router backed by services. Implementation details
// (e.g., auth middleware) can delegate to existing helpers.
func NewAPI(
	auth AuthService,
	groups GroupService,
	apps AppointmentService,
	agenda AgendaService,
	notes NotificationService,
) *API {
	r := mux.NewRouter()
	api := &API{
		router: r,
		auth:   auth,
		groups: groups,
		apps:   apps,
		agenda: agenda,
		notes:  notes,
	}
	// Public
	r.HandleFunc("/register", api.handleRegister()).Methods("POST")
	r.HandleFunc("/login", api.handleLogin()).Methods("POST")

	// Protected
	protected := r.PathPrefix("/api").Subrouter()
	protected.Use(func(next http.Handler) http.Handler {
		// Placeholder: reuse existing AuthMiddleware but needs a UserRepository-backed AuthService
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "middleware not wired", http.StatusUnauthorized)
		})
	})

	protected.HandleFunc("/groups", api.handleCreateGroup()).Methods("POST")
	protected.HandleFunc("/groups/{groupID}/members", api.handleAddMember()).Methods("POST")
	protected.HandleFunc("/appointments", api.handleCreateAppointment()).Methods("POST")
	protected.HandleFunc("/agenda", api.handleGetUserAgenda()).Methods("GET")
	protected.HandleFunc("/groups/{groupID}/agenda", api.handleGetGroupAgenda()).Methods("GET")

	return api
}

func (a *API) Router() *mux.Router { return a.router }

func (a *API) handleRegister() http.HandlerFunc {
	// Placeholder: call a.auth.HashPassword + a user repository through a service
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, ErrNotImplemented.Error(), http.StatusNotImplemented)
	}
}

func (a *API) handleLogin() http.HandlerFunc {
	// Placeholder: a.auth.Authenticate
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, ErrNotImplemented.Error(), http.StatusNotImplemented)
	}
}

func (a *API) handleCreateGroup() http.HandlerFunc {
	type req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, ErrNotImplemented.Error(), http.StatusNotImplemented)
	}
}

func (a *API) handleAddMember() http.HandlerFunc {
	type req struct {
		UserID int64 `json:"user_id"`
		Rank   int   `json:"rank"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&req{})
		http.Error(w, ErrNotImplemented.Error(), http.StatusNotImplemented)
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
		http.Error(w, ErrNotImplemented.Error(), http.StatusNotImplemented)
	}
}

func (a *API) handleGetUserAgenda() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, ErrNotImplemented.Error(), http.StatusNotImplemented)
	}
}

func (a *API) handleGetGroupAgenda() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, ErrNotImplemented.Error(), http.StatusNotImplemented)
	}
}
