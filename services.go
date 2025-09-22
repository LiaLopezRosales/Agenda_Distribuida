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
type authService struct{}

func NewAuthService() AuthService {
	return &authService{}
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

func (s *authService) Authenticate(username, password string) (*User, string, error) {
	// This requires a UserRepository to fetch user. Keep as placeholder to be wired later
	return nil, "", ErrNotImplemented
}

// groupService composes GroupRepository and NotificationRepository
type groupService struct {
	groups GroupRepository
	notes  NotificationRepository
}

func NewGroupService(groups GroupRepository, notes NotificationRepository) GroupService {
	return &groupService{groups: groups, notes: notes}
}

func (s *groupService) CreateGroup(ownerID int64, name, description string) (*Group, error) {
	// Placeholder: create group and add owner as rank 10; notify owner.
	return nil, ErrNotImplemented
}

func (s *groupService) AddMember(actorID, groupID, userID int64, rank int) error {
	// Placeholder: verify actor has higher rank; then add member; notify new member.
	return ErrNotImplemented
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

func (s *appointmentService) CreatePersonalAppointment(ownerID int64, a Appointment) (*Appointment, error) {
	// Placeholder flow:
	// - Validate times (start < end), privacy value
	// - Check HasConflict(ownerID, a.Start, a.End)
	// - Create appointment
	// - Add owner as participant accepted
	// - Notify owner; Emit event
	return nil, ErrNotImplemented
}

func (s *appointmentService) CreateGroupAppointment(ownerID int64, a Appointment) (*Appointment, []Participant, error) {
	// Placeholder flow:
	// - Validate GroupID != nil; validate times
	// - Use repo.CreateGroupAppointment to insert and compute participant statuses by rank
	// - Notify all participants (invite/auto); Emit event
	return nil, nil, ErrNotImplemented
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
