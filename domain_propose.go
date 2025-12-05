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

func BuildEntryUserCreate(u *User) (LogEntry, error) {
	p := userCreatePayload{
		Username:     u.Username,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		DisplayName:  u.DisplayName,
	}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "user",
		AggregateID: strconv.FormatInt(u.ID, 10),
		Op:          OpUserCreate,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryUserUpdateProfile(u *User) (LogEntry, error) {
	p := userUpdateProfilePayload{
		UserID:      u.ID,
		Username:    &u.Username,
		Email:       &u.Email,
		DisplayName: &u.DisplayName,
	}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "user",
		AggregateID: strconv.FormatInt(u.ID, 10),
		Op:          OpUserUpdateProfile,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryUserUpdatePassword(userID int64, passwordHash string) (LogEntry, error) {
	p := userUpdatePasswordPayload{UserID: userID, PasswordHash: passwordHash}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "user",
		AggregateID: strconv.FormatInt(userID, 10),
		Op:          OpUserUpdatePassword,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}
