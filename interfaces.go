// interfaces.go
package agendadistribuida

import "time"

// Repositories define data persistence contracts. They should be pure CRUD-ish.
// Business rules belong in services, not here.

type UserRepository interface {
	CreateUser(user *User) error
	GetUserByUsername(username string) (*User, error)
	GetUserByEmail(email string) (*User, error)
	GetUserByID(id string) (*User, error)
	UpdateUser(user *User) error
	UpdatePassword(userID string, newPasswordHash string) error
}

type GroupRepository interface {
	CreateGroup(group *Group) error
	UpdateGroup(group *Group) error
	DeleteGroup(groupID string) error
	AddGroupMember(groupID, userID string, rank int, addedBy *string) error
	UpdateGroupMember(groupID, userID string, rank int) error
	RemoveGroupMember(groupID, userID string) error
	GetMemberRank(groupID, userID string) (int, error)
	GetGroupMembers(groupID string) ([]GroupMember, error)
	IsSuperior(groupID, userA, userB string) (bool, error)
	GetGroupsForUser(userID string) ([]Group, error)
	GetGroupByID(id string) (*Group, error)
}

type AppointmentRepository interface {
	CreateAppointment(a *Appointment) error
	UpdateAppointment(a *Appointment) error
	DeleteAppointment(appointmentID string) error
	AddParticipant(p *Participant) error
	UpdateParticipantStatus(appointmentID, userID string, status ApptStatus) error
	GetParticipantByAppointmentAndUser(appointmentID, userID string) (*Participant, error)
	HasConflict(userID string, start, end time.Time) (bool, error)
	HasConflictExcluding(userID string, start, end time.Time, excludeAppointmentID string) (bool, error)
	CreateGroupAppointment(a *Appointment) ([]Participant, error)
	GetUserAgenda(userID string, start, end time.Time) ([]Appointment, error)
	GetGroupAgenda(groupID string, start, end time.Time) ([]Appointment, error)
	GetAppointmentByID(appointmentID string) (*Appointment, error)
	GetAppointmentParticipants(appointmentID string) ([]ParticipantDetails, error)
}

type NotificationRepository interface {
	AddNotification(n *Notification) error
	GetUserNotifications(userID string) ([]Notification, error)
	// 🔥 nuevo: soporte para no leídas y marcar leídas
	GetUnreadNotifications(userID string) ([]Notification, error)
	MarkNotificationRead(notificationID string) error
}

// Event log for replication and audit. The EventBus can be no-op locally,
// or publish to a message broker in a distributed deployment.
type EventRepository interface {
	AppendEvent(e *Event) error
}

type AuditRepository interface {
	AppendAudit(entry *AuditLog) error
	ListAuditLogs(filter AuditFilter) ([]AuditLog, error)
}

type EventBus interface {
	Publish(e Event) error
}

// ----------------- Consenso / Raft-like -----------------

type AppendEntriesRequest struct {
	Term         int64      `json:"term"`
	LeaderID     string     `json:"leader_id"`
	PrevLogIndex int64      `json:"prev_log_index"`
	PrevLogTerm  int64      `json:"prev_log_term"`
	Entries      []LogEntry `json:"entries"`
	LeaderCommit int64      `json:"leader_commit"`
}

type AppendEntriesResponse struct {
	Term       int64 `json:"term"`
	Success    bool  `json:"success"`
	MatchIndex int64 `json:"match_index"`
}

type RequestVoteRequest struct {
	Term         int64  `json:"term"`
	CandidateID  string `json:"candidate_id"`
	LastLogIndex int64  `json:"last_log_index"`
	LastLogTerm  int64  `json:"last_log_term"`
	PreVote      bool   `json:"pre_vote,omitempty"`
}

type RequestVoteResponse struct {
	Term        int64 `json:"term"`
	VoteGranted bool  `json:"vote_granted"`
}

type Consensus interface {
	NodeID() string
	IsLeader() bool
	LeaderID() string
	Propose(entry LogEntry) error
	HandleAppendEntries(req AppendEntriesRequest) (AppendEntriesResponse, error)
	HandleRequestVote(req RequestVoteRequest) (RequestVoteResponse, error)
	Start() error
	Stop() error
}

type PeerStore interface {
	LocalID() string
	ListPeers() []string
	SetLeader(id string)
	GetLeader() string
	ResolveAddr(id string) string
}

// Services define business use-cases. They compose repositories and infrastructure.

type AuthService interface {
	HashPassword(password string) (string, error)
	CheckPassword(password, hash string) bool
	GenerateToken(user *User) (string, error)
	ParseToken(token string) (*Claims, error)
	Authenticate(username, password string) (*User, string, error)
}

type GroupService interface {
	CreateGroup(ownerID string, name, description string) (*Group, error)
	UpdateGroup(ownerID string, groupID string, name, description string) (*Group, error)
	DeleteGroup(ownerID string, groupID string) error
	AddMember(actorID, groupID, userID string, rank int) error
	UpdateMember(actorID, groupID, userID string, rank int) error
	RemoveMember(actorID, groupID, userID string) error
}

type AppointmentService interface {
	CreatePersonalAppointment(ownerID string, a Appointment) (*Appointment, error)
	CreateGroupAppointment(ownerID string, a Appointment) (*Appointment, []Participant, error)
	UpdateAppointment(ownerID string, a Appointment) (*Appointment, error)
	DeleteAppointment(ownerID string, appointmentID string) error
	AcceptInvitation(userID string, appointmentID string) error
	RejectInvitation(userID string, appointmentID string) error
	GetAppointmentByID(appointmentID string) (*Appointment, error)
	GetAppointmentParticipants(appointmentID string) ([]ParticipantDetails, error)
	// Wiring de consenso (permitir inyectarlo desde main)
	SetConsensus(c Consensus)
}

type AgendaService interface {
	GetUserAgendaForViewer(viewerID string, start, end time.Time) ([]Appointment, error)
	GetGroupAgendaForViewer(viewerID, groupID string, start, end time.Time) ([]Appointment, error)
}

type NotificationService interface {
	List(userID string) ([]Notification, error)
	Notify(userID string, typ string, payload string) error
	// 🔥 nuevo: endpoints para UI
	ListUnread(userID string) ([]Notification, error)
	MarkRead(notificationID string) error
}

type ReplicationService interface {
	EmitAppointmentCreated(a Appointment) error
	// Future: ResolveConflicts, ApplyRemoteEvent, etc.
}
