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
)

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
		default:
			return errors.New("unsupported op: " + e.Op)
		}
	}
}
