package agendadistribuida

// models.go

import "time"

// ---------- enums / tipos ----------
type Privacy string

const (
	PrivacyFull     Privacy = "full"     // mostrar título/descr.
	PrivacyFreeBusy Privacy = "freebusy" // solo "Busy"
)

type ApptStatus string

const (
	StatusPending  ApptStatus = "pending"  // requiere aceptación (invitaciones)
	StatusAccepted ApptStatus = "accepted" // aceptado
	StatusDeclined ApptStatus = "declined" // rechazado
	StatusAuto     ApptStatus = "auto"     // insertado automáticamente (según jerarquía)
)

// ---------- core models ----------
type User struct {
	ID           int64     `json:"id" db:"id"`
	Username     string    `json:"username" db:"username"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"` // never serializar
	DisplayName  string    `json:"display_name" db:"display_name"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type Group struct {
	ID              int64     `json:"id" db:"id"`
	Name            string    `json:"name" db:"name"`
	Description     string    `json:"description,omitempty" db:"description"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
	CreatorID       int64     `json:"creator_id" db:"creator_id"`
	CreatorUserName string    `json:"creator_username,omitempty" db:"creator_username"`
}

// GroupMember con Rank para jerarquías dinámicas
type GroupMember struct {
	GroupID   int64     `json:"group_id" db:"group_id"`
	UserID    int64     `json:"user_id" db:"user_id"`
	Rank      int       `json:"rank" db:"rank"`
	AddedBy   *int64    `json:"added_by,omitempty" db:"added_by"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	Username  string    `json:"username,omitempty"` // <-- Añadido para respuesta
}

type Appointment struct {
	ID          int64      `json:"id" db:"id"`
	Title       string     `json:"title" db:"title"`
	Description string     `json:"description,omitempty" db:"description"`
	OwnerID     int64      `json:"owner_id" db:"owner_id"`           // quien lo creó
	GroupID     *int64     `json:"group_id,omitempty" db:"group_id"` // si es cita de grupo
	Start       time.Time  `json:"start" db:"start_ts"`              // almacenar como timestamp
	End         time.Time  `json:"end" db:"end_ts"`
	Privacy     Privacy    `json:"privacy" db:"privacy"`
	Status      ApptStatus `json:"status" db:"status"` // estado global
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`

	// Para replicación/conflicto
	Version    int64  `json:"version" db:"version"`
	OriginNode string `json:"origin_node,omitempty" db:"origin_node"`
	Deleted    bool   `json:"deleted" db:"deleted"`
}

type Participant struct {
	ID            int64      `json:"id" db:"id"`
	AppointmentID int64      `json:"appointment_id" db:"appointment_id"`
	UserID        int64      `json:"user_id" db:"user_id"`
	Status        ApptStatus `json:"status" db:"status"` // pending, accepted, declined, auto
	IsOptional    bool       `json:"is_optional" db:"is_optional"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}

type Notification struct {
	ID        int64      `json:"id" db:"id"`
	UserID    int64      `json:"user_id" db:"user_id"`
	Type      string     `json:"type" db:"type"`       // "invite","created","accepted",...
	Payload   string     `json:"payload" db:"payload"` // JSON serializado
	ReadAt    *time.Time `json:"read_at,omitempty" db:"read_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

type Event struct {
	ID         int64     `json:"id" db:"id"`
	Entity     string    `json:"entity" db:"entity"` // "appointment","group",...
	EntityID   int64     `json:"entity_id" db:"entity_id"`
	Action     string    `json:"action" db:"action"` // "create","update","delete"
	Payload    string    `json:"payload" db:"payload"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	OriginNode string    `json:"origin_node" db:"origin_node"`
	Version    int64     `json:"version" db:"version"`
}

// ParticipantDetails extends Participant with user information
type ParticipantDetails struct {
	ID            int64      `json:"id" db:"id"`
	AppointmentID int64      `json:"appointment_id" db:"appointment_id"`
	UserID        int64      `json:"user_id" db:"user_id"`
	Status        ApptStatus `json:"status" db:"status"`
	IsOptional    bool       `json:"is_optional" db:"is_optional"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
	Username      string     `json:"username" db:"username"`
	DisplayName   string     `json:"display_name" db:"display_name"`
}
