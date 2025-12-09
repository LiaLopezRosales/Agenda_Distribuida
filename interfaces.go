// interfaces.go
package agendadistribuida

import "time"

// Repositories define data persistence contracts. They should be pure CRUD-ish.
// Business rules belong in services, not here.

type UserRepository interface {
	CreateUser(user *User) error
	GetUserByUsername(username string) (*User, error)
	GetUserByEmail(email string) (*User, error)
	GetUserByID(id int64) (*User, error)
	UpdateUser(user *User) error
	UpdatePassword(userID int64, newPasswordHash string) error
}

type GroupRepository interface {
	CreateGroup(group *Group) error
	UpdateGroup(group *Group) error
	DeleteGroup(groupID int64) error
	AddGroupMember(groupID, userID int64, rank int, addedBy *int64) error
	UpdateGroupMember(groupID, userID int64, rank int) error
	RemoveGroupMember(groupID, userID int64) error
	GetMemberRank(groupID, userID int64) (int, error)
	GetGroupMembers(groupID int64) ([]GroupMember, error)
	IsSuperior(groupID, userA, userB int64) (bool, error)
	GetGroupsForUser(userID int64) ([]Group, error)
	GetGroupByID(id int64) (*Group, error)
}

type AppointmentRepository interface {
	CreateAppointment(a *Appointment) error
	UpdateAppointment(a *Appointment) error
	DeleteAppointment(appointmentID int64) error
	AddParticipant(p *Participant) error
	UpdateParticipantStatus(appointmentID, userID int64, status ApptStatus) error
	GetParticipantByAppointmentAndUser(appointmentID, userID int64) (*Participant, error)
	HasConflict(userID int64, start, end time.Time) (bool, error)
	HasConflictExcluding(userID int64, start, end time.Time, excludeAppointmentID int64) (bool, error)
	CreateGroupAppointment(a *Appointment) ([]Participant, error)
	GetUserAgenda(userID int64, start, end time.Time) ([]Appointment, error)
	GetGroupAgenda(groupID int64, start, end time.Time) ([]Appointment, error)
	GetAppointmentByID(appointmentID int64) (*Appointment, error)
	GetAppointmentParticipants(appointmentID int64) ([]ParticipantDetails, error)
}

type NotificationRepository interface {
	AddNotification(n *Notification) error
	GetUserNotifications(userID int64) ([]Notification, error)
	// 🔥 nuevo: soporte para no leídas y marcar leídas
	GetUnreadNotifications(userID int64) ([]Notification, error)
	MarkNotificationRead(notificationID int64) error
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
	CreateGroup(ownerID int64, name, description string) (*Group, error)
	UpdateGroup(ownerID int64, groupID int64, name, description string) (*Group, error)
	DeleteGroup(ownerID int64, groupID int64) error
	AddMember(actorID, groupID, userID int64, rank int) error
	UpdateMember(actorID, groupID, userID int64, rank int) error
	RemoveMember(actorID, groupID, userID int64) error
}

type AppointmentService interface {
	CreatePersonalAppointment(ownerID int64, a Appointment) (*Appointment, error)
	CreateGroupAppointment(ownerID int64, a Appointment) (*Appointment, []Participant, error)
	UpdateAppointment(ownerID int64, a Appointment) (*Appointment, error)
	DeleteAppointment(ownerID int64, appointmentID int64) error
	AcceptInvitation(userID int64, appointmentID int64) error
	RejectInvitation(userID int64, appointmentID int64) error
	GetAppointmentByID(appointmentID int64) (*Appointment, error)
	GetAppointmentParticipants(appointmentID int64) ([]ParticipantDetails, error)
	// Wiring de consenso (permitir inyectarlo desde main)
	SetConsensus(c Consensus)
}

type AgendaService interface {
	GetUserAgendaForViewer(viewerID int64, start, end time.Time) ([]Appointment, error)
	GetGroupAgendaForViewer(viewerID, groupID int64, start, end time.Time) ([]Appointment, error)
}

type NotificationService interface {
	List(userID int64) ([]Notification, error)
	Notify(userID int64, typ string, payload string) error
	// 🔥 nuevo: endpoints para UI
	ListUnread(userID int64) ([]Notification, error)
	MarkRead(notificationID int64) error
}

type ReplicationService interface {
	EmitAppointmentCreated(a Appointment) error
	// Future: ResolveConflicts, ApplyRemoteEvent, etc.
}
