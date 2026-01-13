// services.go
package agendadistribuida

import (
	"fmt"
	"time"
)

// Concrete service implementations. They take interfaces in constructors.
// For now, business methods are scaffolded and return ErrNotImplemented,
// except for trivial delegations that are safe to wire now.

// authService implements AuthService using existing helpers in auth.go
type authService struct {
	users UserRepository
}

// 🔥 MODIFICADO: constructor recibe el repo
func NewAuthService(users UserRepository) AuthService {
	return &authService{users: users}
}

func (s *authService) HashPassword(password string) (string, error) {
	return HashPassword(password)
}

func (s *authService) CheckPassword(password, hash string) bool {
	return CheckPasswordHash(password, hash)
}

func (s *authService) GenerateToken(user *User) (string, error) {
	return GenerateToken(user)
}

func (s *authService) ParseToken(token string) (*Claims, error) {
	return ParseToken(token)
}

// 🔥 MODIFICADO: usa UserRepository inyectado
func (s *authService) Authenticate(username, password string) (*User, string, error) {
	user, err := s.users.GetUserByUsername(username)
	if err != nil {
		return nil, "", ErrUnauthorized
	}
	if !CheckPasswordHash(password, user.PasswordHash) {
		return nil, "", ErrUnauthorized
	}
	token, err := GenerateToken(user)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

// groupService composes GroupRepository and NotificationRepository
type groupService struct {
	groups GroupRepository
	notes  NotificationRepository
}

func NewGroupService(groups GroupRepository, notes NotificationRepository) GroupService {
	return &groupService{groups: groups, notes: notes}
}

// 🔥 MODIFICADO: crear grupo y añadir owner como admin (rank alto)
func (s *groupService) CreateGroup(ownerID string, name, description string) (*Group, error) {
	now := time.Now()
	g := &Group{
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatorID:   ownerID,               // Nuevo campo
		GroupType:   GroupTypeHierarchical, // Por defecto, grupos jerárquicos
	}
	// Intentar obtener el nombre de usuario del creador
	if userRepo, ok := s.groups.(interface{ GetUserByID(string) (*User, error) }); ok {
		if creator, err := userRepo.GetUserByID(ownerID); err == nil {
			g.CreatorUserName = creator.Username
		}
	}
	if err := s.groups.CreateGroup(g); err != nil {
		return nil, err
	}
	// owner como rank más alto (ejemplo: 10)
	if err := s.groups.AddGroupMember(g.ID, ownerID, 5, nil); err != nil {
		return nil, err
	}
	// notificar dueño con detalles enriquecidos
	var creatorUsername, creatorDisplayName string
	if userRepo, ok := s.groups.(interface{ GetUserByID(string) (*User, error) }); ok {
		if creator, err := userRepo.GetUserByID(ownerID); err == nil && creator != nil {
			creatorUsername = creator.Username
			creatorDisplayName = creator.DisplayName
		}
	}
	payload := fmt.Sprintf(`{"group_id":%q,"group_name":%q,"group_description":%q,"created_by_id":%q,"created_by_username":%q,"created_by_display_name":%q}`,
		g.ID, g.Name, g.Description, ownerID, creatorUsername, creatorDisplayName)
	if err := s.notes.AddNotification(&Notification{
		UserID:    ownerID,
		Type:      "group_created",
		Payload:   payload,
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	return g, nil
}

// UpdateGroup updates group information
func (s *groupService) UpdateGroup(ownerID string, groupID string, name, description string) (*Group, error) {
	// Verify ownership
	group, err := s.groups.GetGroupByID(groupID)
	if err != nil {
		return nil, err
	}
	if group.CreatorID != ownerID {
		return nil, fmt.Errorf("unauthorized: only group creator can update")
	}

	// Update only provided fields
	if name != "" {
		group.Name = name
	}
	if description != "" || name != "" { // Allow empty description if explicitly updating
		group.Description = description
	}

	if err := s.groups.UpdateGroup(group); err != nil {
		return nil, err
	}

	// Note: Event publishing would be handled by the event bus if available

	return group, nil
}

// DeleteGroup deletes a group and all its members
func (s *groupService) DeleteGroup(ownerID string, groupID string) error {
	// Verify ownership
	group, err := s.groups.GetGroupByID(groupID)
	if err != nil {
		return err
	}
	if group.CreatorID != ownerID {
		return fmt.Errorf("unauthorized: only group creator can delete")
	}

	// Delete group (this will cascade to members and appointments)
	if err := s.groups.DeleteGroup(groupID); err != nil {
		return err
	}

	// Note: Event publishing would be handled by the event bus if available

	return nil
}

// 🔥 MODIFICADO: añadir miembro con verificación de jerarquía
func (s *groupService) AddMember(actorID, groupID, userID string, rank int) error {
	// Obtener información del grupo para determinar el tipo
	group, err := s.groups.GetGroupByID(groupID)
	if err != nil {
		return err
	}

	// Verificar permisos según el tipo de grupo
	if group.GroupType == GroupTypeNonHierarchical {
		// En grupos sin jerarquía, solo el creador puede añadir miembros
		if group.CreatorID != actorID {
			return ErrUnauthorized
		}
		// En grupos sin jerarquía, todos los miembros tienen el mismo rango (0)
		rank = 0
	} else {
		// En grupos jerárquicos, usar la lógica existente
		actorRank, err := s.groups.GetMemberRank(groupID, actorID)
		if err != nil {
			return ErrUnauthorized
		}
		if rank >= actorRank {
			return ErrUnauthorized
		}
	}

	if err := s.groups.AddGroupMember(groupID, userID, rank, &actorID); err != nil {
		return err
	}
	// notificar nuevo miembro con detalles enriquecidos
	groupInfo, _ := s.groups.GetGroupByID(groupID)
	var groupName string
	if groupInfo != nil {
		groupName = groupInfo.Name
	}
	var actorUsername, actorDisplayName string
	if userRepo, ok := s.groups.(interface{ GetUserByID(string) (*User, error) }); ok {
		if actor, err := userRepo.GetUserByID(actorID); err == nil && actor != nil {
			actorUsername = actor.Username
			actorDisplayName = actor.DisplayName
		}
	}
	payload := fmt.Sprintf(`{"group_id":%q,"group_name":%q,"added_by_id":%q,"added_by_username":%q,"added_by_display_name":%q,"rank":%d}`,
		groupID, groupName, actorID, actorUsername, actorDisplayName, rank)
	return s.notes.AddNotification(&Notification{
		UserID:    userID,
		Type:      "group_invite",
		Payload:   payload,
		CreatedAt: time.Now(),
	})
}

// UpdateMember updates a member's rank
func (s *groupService) UpdateMember(actorID, groupID, userID string, rank int) error {
	// Verify actor has permission (must be higher rank)
	actorRank, err := s.groups.GetMemberRank(groupID, actorID)
	if err != nil {
		return ErrUnauthorized
	}

	// Get target member's current rank
	targetRank, err := s.groups.GetMemberRank(groupID, userID)
	if err != nil {
		return fmt.Errorf("member not found")
	}

	// Actor must have higher rank than target
	if actorRank <= targetRank {
		return fmt.Errorf("unauthorized: insufficient rank to modify this member")
	}

	// Update member rank
	if err := s.groups.UpdateGroupMember(groupID, userID, rank); err != nil {
		return err
	}

	// Note: Event publishing would be handled by the event bus if available

	return nil
}

// RemoveMember removes a member from a group
func (s *groupService) RemoveMember(actorID, groupID, userID string) error {
	// Verify actor has permission (must be higher rank)
	actorRank, err := s.groups.GetMemberRank(groupID, actorID)
	if err != nil {
		return ErrUnauthorized
	}

	// Get target member's current rank
	targetRank, err := s.groups.GetMemberRank(groupID, userID)
	if err != nil {
		return fmt.Errorf("member not found")
	}

	// Actor must have higher rank than target
	if actorRank <= targetRank {
		return fmt.Errorf("unauthorized: insufficient rank to remove this member")
	}

	// Remove member (this will also remove from all group appointments)
	if err := s.groups.RemoveGroupMember(groupID, userID); err != nil {
		return err
	}

	// Note: Event publishing would be handled by the event bus if available

	return nil
}

// AcceptInvitation accepts an appointment invitation
func (s *appointmentService) AcceptInvitation(userID string, appointmentID string) error {
	// Verify the user is a participant
	participant, err := s.apps.GetParticipantByAppointmentAndUser(appointmentID, userID)
	if err != nil {
		return fmt.Errorf("invitation not found")
	}

	// Check if already accepted or declined
	if participant.Status == StatusAccepted {
		return fmt.Errorf("invitation already accepted")
	}
	if participant.Status == StatusDeclined {
		return fmt.Errorf("invitation already declined")
	}

	// Update status to accepted
	if err := s.apps.UpdateParticipantStatus(appointmentID, userID, StatusAccepted); err != nil {
		return err
	}

	// Create notification for the appointment owner with enriched details
	appointment, err := s.apps.GetAppointmentByID(appointmentID)
	if err == nil && appointment != nil {
		var userUsername, userDisplayName string
		if userRepo, ok := s.apps.(interface{ GetUserByID(string) (*User, error) }); ok {
			if user, err := userRepo.GetUserByID(userID); err == nil && user != nil {
				userUsername = user.Username
				userDisplayName = user.DisplayName
			}
		}
		payload := fmt.Sprintf(`{"appointment_id":%q,"title":%q,"user_id":%q,"user_username":%q,"user_display_name":%q,"status":"accepted","start":%q,"end":%q}`,
			appointmentID, appointment.Title, userID, userUsername, userDisplayName,
			appointment.Start.Format(time.RFC3339), appointment.End.Format(time.RFC3339))
		_ = s.notes.AddNotification(&Notification{
			UserID:    appointment.OwnerID,
			Type:      "invitation_accepted",
			Payload:   payload,
			CreatedAt: time.Now(),
		})
	}

	return nil
}

// RejectInvitation rejects an appointment invitation
func (s *appointmentService) RejectInvitation(userID string, appointmentID string) error {
	// Verify the user is a participant
	participant, err := s.apps.GetParticipantByAppointmentAndUser(appointmentID, userID)
	if err != nil {
		return fmt.Errorf("invitation not found")
	}

	// Check if already accepted or declined
	if participant.Status == StatusAccepted {
		return fmt.Errorf("invitation already accepted")
	}
	if participant.Status == StatusDeclined {
		return fmt.Errorf("invitation already declined")
	}

	// Update status to declined
	if err := s.apps.UpdateParticipantStatus(appointmentID, userID, StatusDeclined); err != nil {
		return err
	}

	// Create notification for the appointment owner with enriched details
	appointment, err := s.apps.GetAppointmentByID(appointmentID)
	if err == nil && appointment != nil {
		var userUsername, userDisplayName string
		if userRepo, ok := s.apps.(interface{ GetUserByID(string) (*User, error) }); ok {
			if user, err := userRepo.GetUserByID(userID); err == nil && user != nil {
				userUsername = user.Username
				userDisplayName = user.DisplayName
			}
		}
		payload := fmt.Sprintf(`{"appointment_id":%q,"title":%q,"user_id":%q,"user_username":%q,"user_display_name":%q,"status":"declined","start":%q,"end":%q}`,
			appointmentID, appointment.Title, userID, userUsername, userDisplayName,
			appointment.Start.Format(time.RFC3339), appointment.End.Format(time.RFC3339))
		_ = s.notes.AddNotification(&Notification{
			UserID:    appointment.OwnerID,
			Type:      "invitation_declined",
			Payload:   payload,
			CreatedAt: time.Now(),
		})
	}

	return nil
}

// appointmentService enforces conflicts, privacy, and hierarchy rules.
// It emits notifications and events as needed.
type appointmentService struct {
	apps   AppointmentRepository
	groups GroupRepository
	notes  NotificationRepository
	events EventBus
	repl   ReplicationService
	cons   Consensus
}

func NewAppointmentService(
	apps AppointmentRepository,
	groups GroupRepository,
	notes NotificationRepository,
	events EventBus,
	repl ReplicationService,
) AppointmentService {
	return &appointmentService{apps: apps, groups: groups, notes: notes, events: events, repl: repl}
}

// SetConsensus allows wiring the consensus component after construction
func (s *appointmentService) SetConsensus(c Consensus) {
	s.cons = c
}

// 🔥 MODIFICADO: cita personal
func (s *appointmentService) CreatePersonalAppointment(ownerID string, a Appointment) (*Appointment, error) {
	if a.Start.After(a.End) {
		return nil, ErrInvalidInput
	}
	// conflicto
	conflict, err := s.apps.HasConflict(ownerID, a.Start, a.End)
	if err != nil {
		return nil, err
	}
	if conflict {
		return nil, fmt.Errorf("conflict detected")
	}
	a.OwnerID = ownerID
	a.Status = StatusAccepted
	// If consensus is wired and this node is leader, propose via log
	if s.cons != nil && s.cons.IsLeader() {
		entry, err := BuildEntryApptCreatePersonal(ownerID, a)
		if err != nil {
			return nil, err
		}
		if err := s.cons.Propose(entry); err != nil {
			return nil, err
		}
		// Try to retrieve the created appointment heuristically
		// using a tight time window around now
		windowStart := a.Start.Add(-1 * time.Second)
		windowEnd := a.End.Add(1 * time.Second)
		agenda, err := s.apps.GetUserAgenda(ownerID, windowStart, windowEnd)
		if err == nil {
			for _, cand := range agenda {
				if cand.Title == a.Title && cand.Start.Equal(a.Start) && cand.End.Equal(a.End) {
					a = cand
					break
				}
			}
		}
	} else {
		if err := s.apps.CreateAppointment(&a); err != nil {
			return nil, err
		}
		// añadir owner como participante
		p := Participant{
			AppointmentID: a.ID,
			UserID:        ownerID,
			Status:        StatusAccepted,
		}
		if err := s.apps.AddParticipant(&p); err != nil {
			return nil, err
		}
	}
	// notificación con detalles enriquecidos
	var ownerUsername, ownerDisplayName string
	if userRepo, ok := s.apps.(interface{ GetUserByID(string) (*User, error) }); ok {
		if owner, err := userRepo.GetUserByID(ownerID); err == nil && owner != nil {
			ownerUsername = owner.Username
			ownerDisplayName = owner.DisplayName
		}
	}
	payload := fmt.Sprintf(`{"appointment_id":%q,"title":%q,"description":%q,"start":%q,"end":%q,"created_by_id":%q,"created_by_username":%q,"created_by_display_name":%q,"privacy":%q}`,
		a.ID, a.Title, a.Description, a.Start.Format(time.RFC3339), a.End.Format(time.RFC3339),
		ownerID, ownerUsername, ownerDisplayName, a.Privacy)
	if err := s.notes.AddNotification(&Notification{
		UserID:    ownerID,
		Type:      "appt_created",
		Payload:   payload,
		CreatedAt: time.Now(),
	}); err != nil {
		return nil, err
	}
	// evento
	evt := Event{
		Entity:   "appointment",
		EntityID: a.ID,
		Action:   "create",
		Payload:  payload,
		Version:  a.Version,
	}
	_ = s.events.Publish(evt)
	_ = s.repl.EmitAppointmentCreated(a)
	return &a, nil
}

// 🔥 MODIFICADO: cita grupal
func (s *appointmentService) CreateGroupAppointment(ownerID string, a Appointment) (*Appointment, []Participant, error) {
	if a.GroupID == nil {
		return nil, nil, ErrInvalidInput
	}
	if a.Start.After(a.End) {
		return nil, nil, ErrInvalidInput
	}
	a.OwnerID = ownerID
	a.Status = StatusPending // estado inicial global

	// If consensus is wired and this node is leader, create via Raft
	if s.cons != nil && s.cons.IsLeader() {
		entry, err := BuildEntryApptCreateGroup(ownerID, a)
		if err != nil {
			return nil, nil, err
		}
		if err := s.cons.Propose(entry); err != nil {
			return nil, nil, err
		}
		// After commit, try to reload the created appointment and its participants
		// using a small time window and matching by title/group.
		windowStart := a.Start.Add(-1 * time.Second)
		windowEnd := a.End.Add(1 * time.Second)
		apps, err := s.apps.GetGroupAgenda(*a.GroupID, windowStart, windowEnd)
		if err == nil {
			for _, cand := range apps {
				if cand.Title == a.Title && cand.Start.Equal(a.Start) && cand.End.Equal(a.End) {
					// Found the created appointment; load participants
					partsDetails, err := s.apps.GetAppointmentParticipants(cand.ID)
					if err != nil {
						return &cand, nil, nil
					}
					parts := make([]Participant, 0, len(partsDetails))
					for _, pd := range partsDetails {
						parts = append(parts, Participant{
							ID:            pd.ID,
							AppointmentID: pd.AppointmentID,
							UserID:        pd.UserID,
							Status:        pd.Status,
							IsOptional:    pd.IsOptional,
							CreatedAt:     pd.CreatedAt,
							UpdatedAt:     pd.UpdatedAt,
						})
					}
					return &cand, parts, nil
				}
			}
		}
		// If we cannot reliably reload, return the original struct without participants
		return &a, nil, nil
	}

	// Fallback: single-node / no-consensus path
	participants, err := s.apps.CreateGroupAppointment(&a)
	if err != nil {
		return nil, nil, err
	}
	// Notifications are handled by handlers.go / storage for better UI integration
	// evento
	evt := Event{
		Entity:   "appointment",
		EntityID: a.ID,
		Action:   "create_group",
		Version:  a.Version,
	}
	_ = s.events.Publish(evt)
	_ = s.repl.EmitAppointmentCreated(a)
	return &a, participants, nil
}

// GetAppointmentByID retrieves a specific appointment by ID
func (s *appointmentService) GetAppointmentByID(appointmentID string) (*Appointment, error) {
	return s.apps.GetAppointmentByID(appointmentID)
}

// GetAppointmentParticipants retrieves all participants for an appointment with user details
func (s *appointmentService) GetAppointmentParticipants(appointmentID string) ([]ParticipantDetails, error) {
	return s.apps.GetAppointmentParticipants(appointmentID)
}

// UpdateAppointment updates an existing appointment
func (s *appointmentService) UpdateAppointment(ownerID string, a Appointment) (*Appointment, error) {
	// First, get the existing appointment to verify ownership
	existing, err := s.apps.GetAppointmentByID(a.ID)
	if err != nil {
		return nil, err
	}
	if existing.OwnerID != ownerID {
		return nil, fmt.Errorf("unauthorized: only appointment owner can update")
	}

	// Validate the update
	if a.Start.After(a.End) {
		return nil, ErrInvalidInput
	}

	// Check for conflicts (excluding the current appointment)
	conflict, err := s.apps.HasConflictExcluding(ownerID, a.Start, a.End, a.ID)
	if err != nil {
		return nil, err
	}
	if conflict {
		return nil, fmt.Errorf("time conflict with existing appointment")
	}

	// Update the appointment (via consensus if available)
	a.OwnerID = ownerID // Ensure ownership is preserved
	if s.cons != nil && s.cons.IsLeader() {
		entry, err := BuildEntryApptUpdate(a)
		if err != nil {
			return nil, err
		}
		if err := s.cons.Propose(entry); err != nil {
			return nil, err
		}
	} else {
		if err := s.apps.UpdateAppointment(&a); err != nil {
			return nil, err
		}
	}

	// Emit event for replication
	evt := Event{
		Entity:   "appointment",
		EntityID: a.ID,
		Action:   "update",
		Payload:  fmt.Sprintf(`{"appointment_id": %q}`, a.ID),
		Version:  a.Version,
	}
	_ = s.events.Publish(evt)

	return &a, nil
}

// DeleteAppointment deletes an appointment
func (s *appointmentService) DeleteAppointment(ownerID string, appointmentID string) error {
	// First, get the existing appointment to verify ownership
	existing, err := s.apps.GetAppointmentByID(appointmentID)
	if err != nil {
		return err
	}
	if existing.OwnerID != ownerID {
		return fmt.Errorf("unauthorized: only appointment owner can delete")
	}

	// Delete the appointment (via consensus if available)
	if s.cons != nil && s.cons.IsLeader() {
		entry, err := BuildEntryApptDelete(appointmentID)
		if err != nil {
			return err
		}
		if err := s.cons.Propose(entry); err != nil {
			return err
		}
	} else {
		if err := s.apps.DeleteAppointment(appointmentID); err != nil {
			return err
		}
	}

	// Emit event for replication
	evt := Event{
		Entity:   "appointment",
		EntityID: appointmentID,
		Action:   "delete",
		Payload:  fmt.Sprintf(`{"appointment_id": %q}`, appointmentID),
		Version:  existing.Version + 1,
	}
	_ = s.events.Publish(evt)

	return nil
}

// agendaService applies privacy filtering based on viewer, owner, and hierarchy.
type agendaService struct {
	apps   AppointmentRepository
	groups GroupRepository
}

func NewAgendaService(apps AppointmentRepository, groups GroupRepository) AgendaService {
	return &agendaService{apps: apps, groups: groups}
}

func (s *agendaService) GetUserAgendaForViewer(viewerID string, start, end time.Time) ([]Appointment, error) {
	// For own agenda, no filtering usually required (owner sees full)
	return s.apps.GetUserAgenda(viewerID, start, end)
}

func (s *agendaService) GetGroupAgendaForViewer(viewerID, groupID string, start, end time.Time) ([]Appointment, error) {
	appointments, err := s.apps.GetGroupAgenda(groupID, start, end)
	if err != nil {
		return nil, err
	}
	// Apply viewer-based privacy filtering (similar to filterAppointmentForViewer)
	var filtered []Appointment
	for _, a := range appointments {
		if a.OwnerID == viewerID {
			filtered = append(filtered, a)
			continue
		}
		superior, _ := s.groups.IsSuperior(groupID, viewerID, a.OwnerID)
		if superior {
			filtered = append(filtered, a)
			continue
		}
		// Hide details if not superior and not owner
		a.Title = "Busy"
		a.Description = ""
		filtered = append(filtered, a)
	}
	return filtered, nil
}

// notificationService wraps NotificationRepository for possible future logic.
type notificationService struct {
	notes NotificationRepository
}

func NewNotificationService(notes NotificationRepository) NotificationService {
	return &notificationService{notes: notes}
}

func (s *notificationService) List(userID string) ([]Notification, error) {
	return s.notes.GetUserNotifications(userID)
}

func (s *notificationService) Notify(userID string, typ string, payload string) error {
	n := &Notification{
		UserID:    userID,
		Type:      typ,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
	return s.notes.AddNotification(n)
}

// 🔥 nuevo: unread y mark-read
func (s *notificationService) ListUnread(userID string) ([]Notification, error) {
	return s.notes.GetUnreadNotifications(userID)
}

func (s *notificationService) MarkRead(notificationID string) error {
	return s.notes.MarkNotificationRead(notificationID)
}

// No-op implementations for EventBus and ReplicationService to allow compilation
// without external infra. Replace with real broker or gRPC later.

type noopEventBus struct{}

func NewNoopEventBus() EventBus { return noopEventBus{} }

func (noopEventBus) Publish(e Event) error { return nil }

type noopReplication struct{}

func NewNoopReplication() ReplicationService { return noopReplication{} }

func (noopReplication) EmitAppointmentCreated(a Appointment) error {
	fmt.Printf("emit appointment created id=%s\n", a.ID)
	return nil
}
