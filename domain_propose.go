package agendadistribuida

import (
	"encoding/json"
	"strconv"
	"time"
)

func BuildEntryApptCreatePersonal(ownerID string, a Appointment) (LogEntry, error) {
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
		AggregateID: ownerID,
		Op:          OpApptCreatePersonal,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryRepairEnsureUser(u *User) (LogEntry, error) {
	p := repairEnsureUserPayload{
		ID:           u.ID,
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
		Aggregate:   "repair_user",
		AggregateID: u.ID,
		Op:          OpRepairEnsureUser,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryApptCreateGroup(ownerID string, a Appointment) (LogEntry, error) {
	var groupID string
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
		AggregateID: ownerID,
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
		AggregateID: a.ID,
		Op:          OpApptUpdate,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryApptDelete(appointmentID string) (LogEntry, error) {
	p := apptDeletePayload{AppointmentID: appointmentID}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "appointment",
		AggregateID: appointmentID,
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
		AggregateID: u.ID,
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
		AggregateID: u.ID,
		Op:          OpUserUpdateProfile,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryUserUpdatePassword(userID string, passwordHash string) (LogEntry, error) {
	p := userUpdatePasswordPayload{UserID: userID, PasswordHash: passwordHash}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "user",
		AggregateID: userID,
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
		AggregateID: g.ID,
		Op:          OpGroupCreate,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryGroupUpdate(groupID string, name, description *string) (LogEntry, error) {
	p := groupUpdatePayload{GroupID: groupID, Name: name, Description: description}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "group",
		AggregateID: groupID,
		Op:          OpGroupUpdate,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryGroupDelete(groupID string) (LogEntry, error) {
	p := groupDeletePayload{GroupID: groupID}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "group",
		AggregateID: groupID,
		Op:          OpGroupDelete,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryGroupMemberOp(op string, groupID, userID string, rank int) (LogEntry, error) {
	p := groupMemberPayload{GroupID: groupID, UserID: userID, Rank: rank}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "group_member",
		AggregateID: groupID,
		Op:          op,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryInvitationStatus(appointmentID, userID string, status ApptStatus) (LogEntry, error) {
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
		AggregateID: appointmentID,
		Op:          op,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryRepairUserClearEmailIfMatches(userID string, email string) (LogEntry, error) {
	p := repairUserClearEmailPayload{UserID: userID, Email: email}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "repair_user",
		AggregateID: userID,
		Op:          OpRepairUserClearEmailIfMatches,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryRepairEnsureGroupMember(groupID, userID string, rank int) (LogEntry, error) {
	p := repairEnsureGroupMemberPayload{GroupID: groupID, UserID: userID, Rank: rank}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "repair_group",
		AggregateID: groupID,
		Op:          OpRepairEnsureGroupMember,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryRepairEnsureParticipant(appointmentID, userID string, status ApptStatus, isOptional bool) (LogEntry, error) {
	p := repairEnsureParticipantPayload{AppointmentID: appointmentID, UserID: userID, Status: status, IsOptional: isOptional}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "repair_appointment",
		AggregateID: appointmentID,
		Op:          OpRepairEnsureParticipant,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}

func BuildEntryRepairEnsureNotification(userID string, nType, payload string) (LogEntry, error) {
	p := repairEnsureNotificationPayload{UserID: userID, Type: nType, Payload: payload}
	b, err := json.Marshal(p)
	if err != nil {
		return LogEntry{}, err
	}
	return LogEntry{
		EventID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Aggregate:   "repair_notification",
		AggregateID: userID,
		Op:          OpRepairEnsureNotification,
		Payload:     string(b),
		Timestamp:   time.Now(),
	}, nil
}
