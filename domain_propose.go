package agendadistribuida

import (
	"encoding/json"
	"strconv"
	"time"
)

func BuildEntryApptCreatePersonal(ownerID int64, a Appointment) (LogEntry, error) {
	p := apptCreatePayload{
		Title:       a.Title,
		Description: a.Description,
		OwnerID:     ownerID,
		Start:       a.Start,
		End:         a.End,
		Privacy:     a.Privacy,
	}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "appointment",
		AggregateID: strconv.FormatInt(ownerID, 10),
		Op:          OpApptCreatePersonal,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryApptUpdate(a Appointment) (LogEntry, error) {
	p := apptUpdatePayload{
		AppointmentID: a.ID,
		Title:         &a.Title,
		Description:   &a.Description,
		Start:         &a.Start,
		End:           &a.End,
		Privacy:       &a.Privacy,
	}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "appointment",
		AggregateID: strconv.FormatInt(a.ID, 10),
		Op:          OpApptUpdate,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryApptDelete(appointmentID int64) (LogEntry, error) {
	p := apptDeletePayload{AppointmentID: appointmentID}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "appointment",
		AggregateID: strconv.FormatInt(appointmentID, 10),
		Op:          OpApptDelete,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}
