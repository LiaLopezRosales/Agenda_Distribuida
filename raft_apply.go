package agendadistribuida

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	OpApptCreatePersonal = "appointment.create_personal"
	OpApptUpdate         = "appointment.update"
	OpApptDelete         = "appointment.delete"
	OpUserCreate         = "user.create"
	OpUserUpdateProfile  = "user.update_profile"
	OpUserUpdatePassword = "user.update_password"
)

type userCreatePayload struct {
	Username     string `json:"username"`
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
	ID           int64  `json:"id"`
	DisplayName  string `json:"display_name"`
}

type apptCreatePayload struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	OwnerID     int64     `json:"owner_id"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	Privacy     Privacy   `json:"privacy"`
}

type apptUpdatePayload struct {
	AppointmentID int64      `json:"appointment_id"`
	Title         *string    `json:"title,omitempty"`
	Description   *string    `json:"description,omitempty"`
	Start         *time.Time `json:"start,omitempty"`
	End           *time.Time `json:"end,omitempty"`
	Privacy       *Privacy   `json:"privacy,omitempty"`
}

type apptDeletePayload struct {
	AppointmentID int64 `json:"appointment_id"`
}

type userUpdateProfilePayload struct {
	UserID      int64   `json:"user_id"`
	Username    *string `json:"username,omitempty"`
	Email       *string `json:"email,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
}

type userUpdatePasswordPayload struct {
	UserID       int64  `json:"user_id"`
	PasswordHash string `json:"password_hash"`
}

func NewRaftApplier(store *Storage) func(LogEntry) error {
	return func(e LogEntry) error {
		switch e.Op {
		case OpApptCreatePersonal:
			var p apptCreatePayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			a := &Appointment{
				Title:       p.Title,
				Description: p.Description,
				OwnerID:     p.OwnerID,
				Start:       p.Start,
				End:         p.End,
				Privacy:     p.Privacy,
				Status:      StatusAccepted,
			}
			if err := store.CreateAppointment(a); err != nil {
				return err
			}
			part := &Participant{AppointmentID: a.ID, UserID: p.OwnerID, Status: StatusAccepted}
			return store.AddParticipant(part)
		case OpApptUpdate:
			var p apptUpdatePayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			a, err := store.GetAppointmentByID(p.AppointmentID)
			if err != nil {
				return err
			}
			if p.Title != nil {
				a.Title = *p.Title
			}
			if p.Description != nil {
				a.Description = *p.Description
			}
			if p.Start != nil {
				a.Start = *p.Start
			}
			if p.End != nil {
				a.End = *p.End
			}
			if p.Privacy != nil {
				a.Privacy = *p.Privacy
			}
			return store.UpdateAppointment(a)
		case OpApptDelete:
			var p apptDeletePayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			return store.DeleteAppointment(p.AppointmentID)
		case OpUserCreate:
			var p userCreatePayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			u := &User{
				Username:     p.Username,
				Email:        p.Email,
				PasswordHash: p.PasswordHash,
				ID:           p.ID,
				DisplayName:  p.DisplayName,
			}
			return store.CreateUser(u)
		case OpUserUpdateProfile:
			var p userUpdateProfilePayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			u, err := store.GetUserByID(p.UserID)
			if err != nil {
				return err
			}
			if p.Username != nil {
				u.Username = *p.Username
			}
			if p.Email != nil {
				u.Email = *p.Email
			}
			if p.DisplayName != nil {
				u.DisplayName = *p.DisplayName
			}
			return store.UpdateUser(u)
		case OpUserUpdatePassword:
			var p userUpdatePasswordPayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			return store.UpdatePassword(p.UserID, p.PasswordHash)

		default:
			return errors.New("unsupported op: " + e.Op)
		}
	}
}
