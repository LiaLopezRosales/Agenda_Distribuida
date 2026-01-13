package agendadistribuida

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	OpApptCreatePersonal            = "appointment.create_personal"
	OpApptCreateGroup               = "appointment.create_group"
	OpApptUpdate                    = "appointment.update"
	OpApptDelete                    = "appointment.delete"
	OpUserCreate                    = "user.create"
	OpUserUpdateProfile             = "user.update_profile"
	OpUserUpdatePassword            = "user.update_password"
	OpGroupCreate                   = "group.create"
	OpGroupUpdate                   = "group.update"
	OpGroupDelete                   = "group.delete"
	OpGroupMemberAdd                = "group.member_add"
	OpGroupMemberUpdate             = "group.member_update"
	OpGroupMemberRemove             = "group.member_remove"
	OpInvitationAccept              = "invitation.accept"
	OpInvitationReject              = "invitation.reject"
	OpRepairUserClearEmailIfMatches = "repair.user.clear_email_if_matches"
	OpRepairEnsureUser              = "repair.user.ensure"
	OpRepairEnsureGroupMember       = "repair.group.ensure_member"
	OpRepairEnsureParticipant       = "repair.appointment.ensure_participant"
	OpRepairEnsureNotification      = "repair.notification.ensure"
)

type repairUserClearEmailPayload struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

type repairEnsureUserPayload struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
	DisplayName  string `json:"display_name"`
}

type repairEnsureGroupMemberPayload struct {
	GroupID string `json:"group_id"`
	UserID  string `json:"user_id"`
	Rank    int    `json:"rank"`
}

type repairEnsureParticipantPayload struct {
	AppointmentID string     `json:"appointment_id"`
	UserID        string     `json:"user_id"`
	Status        ApptStatus `json:"status"`
	IsOptional    bool       `json:"is_optional"`
}

type repairEnsureNotificationPayload struct {
	UserID  string `json:"user_id"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

type userCreatePayload struct {
	Username     string `json:"username"`
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
	ID           string `json:"id"`
	DisplayName  string `json:"display_name"`
}

type apptCreatePayload struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	OwnerID     string    `json:"owner_id"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	Privacy     Privacy   `json:"privacy"`
}

type apptCreateGroupPayload struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	OwnerID     string    `json:"owner_id"`
	GroupID     string    `json:"group_id"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	Privacy     Privacy   `json:"privacy"`
}

type apptUpdatePayload struct {
	AppointmentID string     `json:"appointment_id"`
	Title         *string    `json:"title,omitempty"`
	Description   *string    `json:"description,omitempty"`
	Start         *time.Time `json:"start,omitempty"`
	End           *time.Time `json:"end,omitempty"`
	Privacy       *Privacy   `json:"privacy,omitempty"`
}

type apptDeletePayload struct {
	AppointmentID string `json:"appointment_id"`
}

type userUpdateProfilePayload struct {
	UserID      string  `json:"user_id"`
	Username    *string `json:"username,omitempty"`
	Email       *string `json:"email,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
}

type userUpdatePasswordPayload struct {
	UserID       string `json:"user_id"`
	PasswordHash string `json:"password_hash"`
}

type groupCreatePayload struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatorID   string    `json:"creator_id"`
	CreatorUser string    `json:"creator_username"`
	GroupType   GroupType `json:"group_type"`
}

type groupUpdatePayload struct {
	GroupID     string  `json:"group_id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type groupDeletePayload struct {
	GroupID string `json:"group_id"`
}

type groupMemberPayload struct {
	GroupID string `json:"group_id"`
	UserID  string `json:"user_id"`
	Rank    int    `json:"rank"`
}

type invitationStatusPayload struct {
	AppointmentID string     `json:"appointment_id"`
	UserID        string     `json:"user_id"`
	Status        ApptStatus `json:"status"`
}

func NewRaftApplier(store *Storage) func(LogEntry) error {
	return func(e LogEntry) error {
		switch e.Op {
		case OpApptCreatePersonal:
			var p apptCreatePayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			if existingID, err := store.FindAppointmentBySignature(p.OwnerID, nil, p.Start, p.End, p.Title); err == nil && existingID != "" {
				if _, err := store.GetParticipantByAppointmentAndUser(existingID, p.OwnerID); err == nil {
					return nil
				}
				part := &Participant{AppointmentID: existingID, UserID: p.OwnerID, Status: StatusAccepted}
				if err := store.AddParticipant(part); err != nil {
					if strings.Contains(err.Error(), "UNIQUE constraint failed") {
						return nil
					}
					return err
				}
				return nil
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
			if _, err := store.GetParticipantByAppointmentAndUser(a.ID, p.OwnerID); err == nil {
				return nil
			}
			if err := store.AddParticipant(part); err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					return nil
				}
				return err
			}
			return nil
		case OpRepairEnsureUser:
			var p repairEnsureUserPayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			u := &User{
				ID:           p.ID,
				Username:     p.Username,
				Email:        p.Email,
				PasswordHash: p.PasswordHash,
				DisplayName:  p.DisplayName,
			}
			return store.EnsureUser(u)
		case OpApptCreateGroup:
			var p apptCreateGroupPayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			if existingID, err := store.FindAppointmentBySignature(p.OwnerID, &p.GroupID, p.Start, p.End, p.Title); err == nil && existingID != "" {
				return nil
			}
			gID := p.GroupID
			a := &Appointment{
				Title:       p.Title,
				Description: p.Description,
				OwnerID:     p.OwnerID,
				GroupID:     &gID,
				Start:       p.Start,
				End:         p.End,
				Privacy:     p.Privacy,
				Status:      StatusPending,
			}
			// This will insert the appointment, compute participants based on group membership
			// and create the corresponding invite notifications on every node.
			_, err := store.CreateGroupAppointment(a)
			return err
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
			// Idempotency / safety: if the user already exists, treat as success.
			if existing, err := store.GetUserByUsername(p.Username); err == nil && existing != nil {
				return nil
			}
			// If email is already taken by a different user, this is a real conflict.
			if p.Email != "" {
				if byEmail, err := store.GetUserByEmail(p.Email); err == nil && byEmail != nil {
					if byEmail.Username == p.Username {
						return nil
					}
					return errors.New("user.create apply conflict: email already exists")
				}
			}
			u := &User{
				Username:     p.Username,
				Email:        p.Email,
				PasswordHash: p.PasswordHash,
				ID:           p.ID,
				DisplayName:  p.DisplayName,
			}
			if err := store.CreateUser(u); err != nil {
				// If we lost a race or the row already exists, treat as success.
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					if existing, gerr := store.GetUserByUsername(p.Username); gerr == nil && existing != nil {
						return nil
					}
					if p.Email != "" {
						if byEmail, gerr := store.GetUserByEmail(p.Email); gerr == nil && byEmail != nil {
							if byEmail.Username == p.Username {
								return nil
							}
						}
					}
				}
				return err
			}
			return nil
		case OpUserUpdateProfile:
			var p userUpdateProfilePayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			u, err := store.GetUserByID(p.UserID)
			if err != nil {
				return err
			}
			desired := *u
			if p.Username != nil {
				desired.Username = *p.Username
			}
			if p.Email != nil {
				desired.Email = *p.Email
			}
			if p.DisplayName != nil {
				desired.DisplayName = *p.DisplayName
			}
			// Idempotency: if already at desired state, treat as success.
			if desired.Username == u.Username && desired.Email == u.Email && desired.DisplayName == u.DisplayName {
				return nil
			}
			u.Username = desired.Username
			u.Email = desired.Email
			u.DisplayName = desired.DisplayName
			if err := store.UpdateUser(u); err != nil {
				// If this fails due to UNIQUE constraints, re-check if state is already applied.
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					if fresh, gerr := store.GetUserByID(p.UserID); gerr == nil && fresh != nil {
						if fresh.Username == desired.Username && fresh.Email == desired.Email && fresh.DisplayName == desired.DisplayName {
							return nil
						}
					}
					return errors.New("user.update_profile apply conflict: unique constraint")
				}
				return err
			}
			return nil
		case OpUserUpdatePassword:
			var p userUpdatePasswordPayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			// Idempotency: if password hash already matches, treat as success.
			if u, err := store.GetUserByID(p.UserID); err == nil && u != nil {
				if u.PasswordHash == p.PasswordHash {
					return nil
				}
			}
			return store.UpdatePassword(p.UserID, p.PasswordHash)
		case OpGroupCreate:
			var p groupCreatePayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			g := &Group{
				Name:            p.Name,
				Description:     p.Description,
				CreatorID:       p.CreatorID,
				CreatorUserName: p.CreatorUser,
				GroupType:       p.GroupType,
			}
			if err := store.CreateGroup(g); err != nil {
				return err
			}
			// Owner rank depends on group type: hierarchical -> higher rank, non_hierarchical -> 0
			ownerRank := 5
			if p.GroupType == GroupTypeNonHierarchical {
				ownerRank = 0
			}
			return store.AddGroupMember(g.ID, p.CreatorID, ownerRank, nil)
		case OpGroupUpdate:
			var p groupUpdatePayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			g, err := store.GetGroupByID(p.GroupID)
			if err != nil {
				return err
			}
			if p.Name != nil {
				g.Name = *p.Name
			}
			if p.Description != nil {
				g.Description = *p.Description
			}
			return store.UpdateGroup(g)
		case OpGroupDelete:
			var p groupDeletePayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			if _, err := store.GetGroupByID(p.GroupID); err != nil {
				return nil
			}
			return store.DeleteGroup(p.GroupID)
		case OpGroupMemberAdd:
			var p groupMemberPayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			return store.AddGroupMember(p.GroupID, p.UserID, p.Rank, nil)
		case OpGroupMemberUpdate:
			var p groupMemberPayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			return store.UpdateGroupMember(p.GroupID, p.UserID, p.Rank)
		case OpGroupMemberRemove:
			var p groupMemberPayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			if err := store.RemoveGroupMember(p.GroupID, p.UserID); err != nil {
				// Idempotency: removing a non-existing member is treated as success.
				if strings.Contains(err.Error(), "member not found") {
					return nil
				}
				return err
			}
			return nil
		case OpInvitationAccept, OpInvitationReject:
			var p invitationStatusPayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			// Idempotency: if participant already has this status, treat as success and
			// avoid creating duplicate notifications.
			if existing, err := store.GetParticipantByAppointmentAndUser(p.AppointmentID, p.UserID); err == nil && existing != nil {
				if existing.Status == p.Status {
					return nil
				}
			}
			if err := store.UpdateParticipantStatus(p.AppointmentID, p.UserID, p.Status); err != nil {
				return err
			}
			// Create notification for the appointment owner with enriched details
			appointment, err := store.GetAppointmentByID(p.AppointmentID)
			if err != nil || appointment == nil {
				return nil
			}
			var userUsername, userDisplayName string
			if user, err := store.GetUserByID(p.UserID); err == nil && user != nil {
				userUsername = user.Username
				userDisplayName = user.DisplayName
			}
			statusStr := "accepted"
			if p.Status == StatusDeclined {
				statusStr = "declined"
			}
			payload := struct {
				AppointmentID string `json:"appointment_id"`
				Title         string `json:"title"`
				UserID        string `json:"user_id"`
				UserUsername  string `json:"user_username"`
				UserName      string `json:"user_display_name"`
				Status        string `json:"status"`
				Start         string `json:"start"`
				End           string `json:"end"`
			}{
				AppointmentID: p.AppointmentID,
				Title:         appointment.Title,
				UserID:        p.UserID,
				UserUsername:  userUsername,
				UserName:      userDisplayName,
				Status:        statusStr,
				Start:         appointment.Start.Format(time.RFC3339),
				End:           appointment.End.Format(time.RFC3339),
			}
			b, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			noteType := "invitation_accepted"
			if p.Status == StatusDeclined {
				noteType = "invitation_declined"
			}
			return store.AddNotification(&Notification{
				UserID:    appointment.OwnerID,
				Type:      noteType,
				Payload:   string(b),
				CreatedAt: time.Now(),
			})
		case OpRepairUserClearEmailIfMatches:
			var p repairUserClearEmailPayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			return store.ClearUserEmailIfMatches(p.UserID, p.Email)
		case OpRepairEnsureGroupMember:
			var p repairEnsureGroupMemberPayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			return store.EnsureGroupMember(p.GroupID, p.UserID, p.Rank, nil)
		case OpRepairEnsureParticipant:
			var p repairEnsureParticipantPayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			if err := store.EnsureParticipant(p.AppointmentID, p.UserID, p.Status, p.IsOptional); err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					return nil
				}
				return err
			}
			return nil
		case OpRepairEnsureNotification:
			var p repairEnsureNotificationPayload
			if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
				return err
			}
			return store.EnsureNotification(&Notification{UserID: p.UserID, Type: p.Type, Payload: p.Payload, CreatedAt: time.Now()})

		default:
			return errors.New("unsupported op: " + e.Op)
		}
	}
}
