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
func (s *groupService) CreateGroup(ownerID int64, name, description string) (*Group, error) {
	now := time.Now()
	g := &Group{
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatorID:   ownerID, // Nuevo campo
	}
	// Intentar obtener el nombre de usuario del creador
	if userRepo, ok := s.groups.(interface{ GetUserByID(int64) (*User, error) }); ok {
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
	// notificar dueño
	payload := fmt.Sprintf(`{"group_id": %d}`, g.ID)
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
func (s *groupService) UpdateGroup(ownerID int64, groupID int64, name, description string) (*Group, error) {
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
func (s *groupService) DeleteGroup(ownerID int64, groupID int64) error {
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
func (s *groupService) AddMember(actorID, groupID, userID int64, rank int) error {
	actorRank, err := s.groups.GetMemberRank(groupID, actorID)
	if err != nil {
		return ErrUnauthorized
	}
	if rank >= actorRank {
		return ErrUnauthorized
	}
	if err := s.groups.AddGroupMember(groupID, userID, rank, &actorID); err != nil {
		return err
	}
	// notificar nuevo miembro
	payload := fmt.Sprintf(`{"group_id": %d}`, groupID)
	return s.notes.AddNotification(&Notification{
		UserID:    userID,
		Type:      "group_invite",
		Payload:   payload,
		CreatedAt: time.Now(),
	})
}

// UpdateMember updates a member's rank
func (s *groupService) UpdateMember(actorID, groupID, userID int64, rank int) error {
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
func (s *groupService) RemoveMember(actorID, groupID, userID int64) error {
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

// appointmentService enforces conflicts, privacy, and hierarchy rules.
// It emits notifications and events as needed.
type appointmentService struct {
	apps   AppointmentRepository
	groups GroupRepository
	notes  NotificationRepository
	events EventBus
	repl   ReplicationService
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

// 🔥 MODIFICADO: cita personal
func (s *appointmentService) CreatePersonalAppointment(ownerID int64, a Appointment) (*Appointment, error) {
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
	// notificación
	payload := fmt.Sprintf(`{"appointment_id": %d}`, a.ID)
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
func (s *appointmentService) CreateGroupAppointment(ownerID int64, a Appointment) (*Appointment, []Participant, error) {
	if a.GroupID == nil {
		return nil, nil, ErrInvalidInput
	}
	if a.Start.After(a.End) {
		return nil, nil, ErrInvalidInput
	}
	a.OwnerID = ownerID
	a.Status = StatusPending // estado inicial global
	participants, err := s.apps.CreateGroupAppointment(&a)
	if err != nil {
		return nil, nil, err
	}
	// notificar todos los participantes
	for _, p := range participants {
		payload := fmt.Sprintf(`{"appointment_id": %d, "status": "%s"}`, a.ID, p.Status)
		_ = s.notes.AddNotification(&Notification{
			UserID:    p.UserID,
			Type:      "appt_invite",
			Payload:   payload,
			CreatedAt: time.Now(),
		})
	}
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
func (s *appointmentService) GetAppointmentByID(appointmentID int64) (*Appointment, error) {
	return s.apps.GetAppointmentByID(appointmentID)
}

// GetAppointmentParticipants retrieves all participants for an appointment with user details
func (s *appointmentService) GetAppointmentParticipants(appointmentID int64) ([]ParticipantDetails, error) {
	return s.apps.GetAppointmentParticipants(appointmentID)
}

// UpdateAppointment updates an existing appointment
func (s *appointmentService) UpdateAppointment(ownerID int64, a Appointment) (*Appointment, error) {
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

	// Update the appointment
	a.OwnerID = ownerID // Ensure ownership is preserved
	if err := s.apps.UpdateAppointment(&a); err != nil {
		return nil, err
	}

	// Emit event for replication
	evt := Event{
		Entity:   "appointment",
		EntityID: a.ID,
		Action:   "update",
		Payload:  fmt.Sprintf(`{"appointment_id": %d}`, a.ID),
		Version:  a.Version,
	}
	_ = s.events.Publish(evt)

	return &a, nil
}

// DeleteAppointment deletes an appointment
func (s *appointmentService) DeleteAppointment(ownerID int64, appointmentID int64) error {
	// First, get the existing appointment to verify ownership
	existing, err := s.apps.GetAppointmentByID(appointmentID)
	if err != nil {
		return err
	}
	if existing.OwnerID != ownerID {
		return fmt.Errorf("unauthorized: only appointment owner can delete")
	}

	// Delete the appointment (soft delete)
	if err := s.apps.DeleteAppointment(appointmentID); err != nil {
		return err
	}

	// Emit event for replication
	evt := Event{
		Entity:   "appointment",
		EntityID: appointmentID,
		Action:   "delete",
		Payload:  fmt.Sprintf(`{"appointment_id": %d}`, appointmentID),
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

func (s *agendaService) GetUserAgendaForViewer(viewerID int64, start, end time.Time) ([]Appointment, error) {
	// For own agenda, no filtering usually required (owner sees full)
	return s.apps.GetUserAgenda(viewerID, start, end)
}

func (s *agendaService) GetGroupAgendaForViewer(viewerID, groupID int64, start, end time.Time) ([]Appointment, error) {
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

func (s *notificationService) List(userID int64) ([]Notification, error) {
	return s.notes.GetUserNotifications(userID)
}

func (s *notificationService) Notify(userID int64, typ string, payload string) error {
	n := &Notification{
		UserID:    userID,
		Type:      typ,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
	return s.notes.AddNotification(n)
}

// 🔥 nuevo: unread y mark-read
func (s *notificationService) ListUnread(userID int64) ([]Notification, error) {
	return s.notes.GetUnreadNotifications(userID)
}

func (s *notificationService) MarkRead(notificationID int64) error {
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
	fmt.Printf("emit appointment created id=%d\n", a.ID)
	return nil
}
