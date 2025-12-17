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

func BuildEntryApptCreateGroup(ownerID int64, a Appointment) (LogEntry, error) {
	var groupID int64
	if a.GroupID != nil {
		groupID = *a.GroupID
	}
	p := apptCreateGroupPayload{
		Title:       a.Title,
		Description: a.Description,
		OwnerID:     ownerID,
		GroupID:     groupID,
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
		Aggregate:   "appointment_group",
		AggregateID: strconv.FormatInt(ownerID, 10),
		Op:          OpApptCreateGroup,
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

func BuildEntryGroupCreate(g *Group) (LogEntry, error) {
	p := groupCreatePayload{
		Name:        g.Name,
		Description: g.Description,
		CreatorID:   g.CreatorID,
		CreatorUser: g.CreatorUserName,
		GroupType:   g.GroupType,
	}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "group",
		AggregateID: strconv.FormatInt(g.ID, 10),
		Op:          OpGroupCreate,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryGroupUpdate(groupID int64, name, description *string) (LogEntry, error) {
	p := groupUpdatePayload{GroupID: groupID, Name: name, Description: description}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "group",
		AggregateID: strconv.FormatInt(groupID, 10),
		Op:          OpGroupUpdate,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryGroupDelete(groupID int64) (LogEntry, error) {
	p := groupDeletePayload{GroupID: groupID}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "group",
		AggregateID: strconv.FormatInt(groupID, 10),
		Op:          OpGroupDelete,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryGroupMemberOp(op string, groupID, userID int64, rank int) (LogEntry, error) {
	p := groupMemberPayload{GroupID: groupID, UserID: userID, Rank: rank}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "group_member",
		AggregateID: strconv.FormatInt(groupID, 10),
		Op:          op,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryInvitationStatus(appointmentID, userID int64, status ApptStatus) (LogEntry, error) {
	op := OpInvitationAccept
	if status == StatusDeclined {
		op = OpInvitationReject
	}
	p := invitationStatusPayload{AppointmentID: appointmentID, UserID: userID, Status: status}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "invitation",
		AggregateID: strconv.FormatInt(appointmentID, 10),
		Op:          op,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryRepairUserClearEmailIfMatches(userID int64, email string) (LogEntry, error) {
	p := repairUserClearEmailPayload{UserID: userID, Email: email}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "repair_user",
		AggregateID: strconv.FormatInt(userID, 10),
		Op:          OpRepairUserClearEmailIfMatches,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryRepairEnsureGroupMember(groupID, userID int64, rank int) (LogEntry, error) {
	p := repairEnsureGroupMemberPayload{GroupID: groupID, UserID: userID, Rank: rank}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "repair_group",
		AggregateID: strconv.FormatInt(groupID, 10),
		Op:          OpRepairEnsureGroupMember,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryRepairEnsureParticipant(appointmentID, userID int64, status ApptStatus, isOptional bool) (LogEntry, error) {
	p := repairEnsureParticipantPayload{AppointmentID: appointmentID, UserID: userID, Status: status, IsOptional: isOptional}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "repair_appointment",
		AggregateID: strconv.FormatInt(appointmentID, 10),
		Op:          OpRepairEnsureParticipant,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryRepairEnsureNotification(userID int64, nType, payload string) (LogEntry, error) {
	p := repairEnsureNotificationPayload{UserID: userID, Type: nType, Payload: payload}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "repair_notification",
		AggregateID: strconv.FormatInt(userID, 10),
		Op:          OpRepairEnsureNotification,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}
