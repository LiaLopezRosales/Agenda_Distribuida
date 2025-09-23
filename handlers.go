// handlers.go
package agendadistribuida

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// ======================
// Helpers
// ======================

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

// ======================
// User Handlers
// ======================

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func handleRegister(storage *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request")
			return
		}

		hash, err := HashPassword(req.Password)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Error hashing password")
			return
		}

		user := &User{
			Username:     req.Username,
			Email:        req.Email,
			PasswordHash: hash,
			DisplayName:  req.Name,
		}
		if err := storage.CreateUser(user); err != nil {
			respondError(w, http.StatusBadRequest, "User already exists")
			return
		}

		respondJSON(w, http.StatusCreated, user)
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func handleLogin(storage *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request")
			return
		}

		user, token, err := AuthenticateUser(storage, req.Username, req.Password)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "Invalid credentials")
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"token": token,
			"user":  user,
		})
	}
}

// ======================
// Group Handlers
// ======================

type createGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func handleCreateGroup(storage *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := GetUserFromContext(r)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		var req createGroupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request")
			return
		}

		group := &Group{Name: req.Name, Description: req.Description}
		if err := storage.CreateGroup(group); err != nil {
			respondError(w, http.StatusInternalServerError, "Could not create group")
			return
		}

		// Creador con mayor rank (ej: 10)
		if err := storage.AddGroupMember(group.ID, user.ID, 10, nil); err != nil {
			respondError(w, http.StatusInternalServerError, "Could not add creator to group")
			return
		}

		respondJSON(w, http.StatusCreated, group)
	}
}

type addMemberRequest struct {
	UserID int64 `json:"user_id"`
	Rank   int   `json:"rank"`
}

func handleAddGroupMember(storage *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		groupID := parseID(vars["groupID"])

		var req addMemberRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request")
			return
		}

		if err := storage.AddGroupMember(groupID, req.UserID, req.Rank, nil); err != nil {
			respondError(w, http.StatusInternalServerError, "Could not add member")
			return
		}

		respondJSON(w, http.StatusCreated, map[string]string{"status": "member added"})
	}
}

// ======================
// Appointment Handlers
// ======================

type createAppointmentRequest struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	Privacy     Privacy   `json:"privacy"`
	GroupID     *int64    `json:"group_id,omitempty"`
}

func handleCreateAppointment(storage *Storage, wsManager *WSManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := GetUserFromContext(r)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		var req createAppointmentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "Invalid request")
			return
		}

		appt := &Appointment{
			Title:       req.Title,
			Description: req.Description,
			OwnerID:     user.ID,
			GroupID:     req.GroupID,
			Start:       req.Start,
			End:         req.End,
			Privacy:     req.Privacy,
			Status:      StatusPending,
		}

		if req.GroupID != nil {
			// Cita grupal con jerarquía
			participants, err := storage.CreateGroupAppointment(appt)
			if err != nil {
				respondError(w, http.StatusInternalServerError, "Could not create group appointment")
				return
			}

			// Crear notificaciones para todos los participantes
			for _, p := range participants {
				payload := fmt.Sprintf(`{"appointment_id": %d, "status": "%s"}`, appt.ID, p.Status)
				storage.AddNotification(&Notification{
					UserID:    p.UserID,
					Type:      "invite",
					Payload:   payload,
					CreatedAt: time.Now(),
				})
				// 🔔 Nuevo: enviar por WebSocket en tiempo real
				wsManager.BroadcastToUser(p.UserID, map[string]interface{}{
					"type":           "invite",
					"appointment_id": appt.ID,
					"status":         p.Status,
				})
			}

			respondJSON(w, http.StatusCreated, map[string]interface{}{
				"appointment":  appt,
				"participants": participants,
			})
			return
		}

		// Cita personal -> verificar conflictos
		conflict, err := storage.HasConflict(user.ID, req.Start, req.End)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Error checking conflicts")
			return
		}
		if conflict {
			respondError(w, http.StatusConflict, "Time conflict with existing appointment")
			return
		}

		if err := storage.CreateAppointment(appt); err != nil {
			respondError(w, http.StatusInternalServerError, "Could not create appointment")
			return
		}

		// Añadir al dueño como participante aceptado para que aparezca en su agenda y cuente en conflictos
		_ = storage.AddParticipant(&Participant{
			AppointmentID: appt.ID,
			UserID:        user.ID,
			Status:        StatusAccepted,
			IsOptional:    false,
		})

		// Notificación solo al dueño
		storage.AddNotification(&Notification{
			UserID:    user.ID,
			Type:      "created",
			Payload:   fmt.Sprintf(`{"appointment_id": %d}`, appt.ID),
			CreatedAt: time.Now(),
		})
		// 🔔 Nuevo: enviar por WebSocket en tiempo real
		wsManager.BroadcastToUser(user.ID, map[string]interface{}{
			"type":           "created",
			"appointment_id": appt.ID,
		})

		respondJSON(w, http.StatusCreated, appt)
	}
}

// ======================
// Agenda Handlers
// ======================

// Aplica reglas de privacidad según jerarquía y dueño
func filterAppointmentForViewer(storage *Storage, a Appointment, viewer *User, groupID *int64) Appointment {
	// Dueño siempre ve todo
	if a.OwnerID == viewer.ID {
		return a
	}

	// Si es cita grupal, chequear jerarquía
	if groupID != nil && a.GroupID != nil {
		superior, _ := storage.IsSuperior(*a.GroupID, viewer.ID, a.OwnerID)
		if superior {
			return a // superiores ven detalles
		}
	}

	// Si privacidad es FreeBusy o el viewer no tiene privilegios -> ocultar detalles
	if a.Privacy == PrivacyFreeBusy {
		a.Title = "Busy"
		a.Description = ""
	}
	return a
}

func handleGetUserAgenda(storage *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, err := GetUserFromContext(r)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")
		start, _ := time.Parse(time.RFC3339, startStr)
		end, _ := time.Parse(time.RFC3339, endStr)

		appointments, err := storage.GetUserAgenda(viewer.ID, start, end)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Error loading agenda")
			return
		}

		// Aquí no aplicamos filtro porque el dueño consulta su propia agenda
		respondJSON(w, http.StatusOK, appointments)
	}
}

func handleGetGroupAgenda(storage *Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, err := GetUserFromContext(r)
		if err != nil {
			respondError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		vars := mux.Vars(r)
		groupID := parseID(vars["groupID"])

		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")
		start, _ := time.Parse(time.RFC3339, startStr)
		end, _ := time.Parse(time.RFC3339, endStr)

		appointments, err := storage.GetGroupAgenda(groupID, start, end)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Error loading group agenda")
			return
		}

		// Aplicar reglas de privacidad por cada cita
		var filtered []Appointment
		for _, a := range appointments {
			filtered = append(filtered, filterAppointmentForViewer(storage, a, viewer, &groupID))
		}

		respondJSON(w, http.StatusOK, filtered)
	}
}

// ======================
// Router Setup
// ======================

func NewRouter(storage *Storage, wsManager *WSManager) *mux.Router {
	r := mux.NewRouter()

	// Auth
	r.HandleFunc("/register", handleRegister(storage)).Methods("POST")
	r.HandleFunc("/login", handleLogin(storage)).Methods("POST")

	// Protected routes
	api := r.PathPrefix("/api").Subrouter()
	api.Use(func(next http.Handler) http.Handler {
		return AuthMiddleware(next, storage)
	})

	// Groups
	api.HandleFunc("/groups", handleCreateGroup(storage)).Methods("POST")
	api.HandleFunc("/groups/{groupID}/members", handleAddGroupMember(storage)).Methods("POST")

	// Appointments
	api.HandleFunc("/appointments", handleCreateAppointment(storage, wsManager)).Methods("POST")

	// Agenda
	api.HandleFunc("/agenda", handleGetUserAgenda(storage)).Methods("GET")
	api.HandleFunc("/groups/{groupID}/agenda", handleGetGroupAgenda(storage)).Methods("GET")

	return r
}

// ======================
// Utils
// ======================

// func parseID(idStr string) int64 {
// 	var id int64
// 	_, err := fmt.Sscan(idStr, &id)
// 	if err != nil {
// 		return 0
// 	}
// 	return id
// }
