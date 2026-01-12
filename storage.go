// storage.go
package agendadistribuida

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Storage struct {
	db *sql.DB
}

// 🔥 NUEVO: aseguramos que Storage cumple con todas las interfaces
var (
	_ UserRepository         = (*Storage)(nil)
	_ GroupRepository        = (*Storage)(nil)
	_ AppointmentRepository  = (*Storage)(nil)
	_ NotificationRepository = (*Storage)(nil)
	_ EventRepository        = (*Storage)(nil)
	_ AuditRepository        = (*Storage)(nil)
)

// Inicializa conexión y migraciones
func NewStorage(dsn string) (*Storage, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	s := &Storage{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// ====================
// Migraciones
// ====================
func (s *Storage) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT UNIQUE NOT NULL,
	email TEXT UNIQUE,
	password_hash TEXT NOT NULL,
	display_name TEXT,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    creator_id INTEGER,
    creator_username TEXT,
    group_type TEXT NOT NULL DEFAULT 'hierarchical'
);

CREATE TABLE IF NOT EXISTS group_members (
	group_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	rank INTEGER NOT NULL DEFAULT 0,
	added_by INTEGER,
	created_at DATETIME NOT NULL,
	PRIMARY KEY (group_id, user_id)
);

CREATE TABLE IF NOT EXISTS appointments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	description TEXT,
	owner_id INTEGER NOT NULL,
	group_id INTEGER,
	start_ts INTEGER NOT NULL,
	end_ts INTEGER NOT NULL,
	privacy TEXT NOT NULL,
	status TEXT NOT NULL,
	version INTEGER DEFAULT 1,
	origin_node TEXT,
	deleted INTEGER DEFAULT 0,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS participants (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	appointment_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	status TEXT NOT NULL,
	is_optional INTEGER DEFAULT 0,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS notifications (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	type TEXT NOT NULL,
	payload TEXT,
	read_at DATETIME,
	created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	entity TEXT NOT NULL,
	entity_id INTEGER NOT NULL,
	action TEXT NOT NULL,
	payload TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	origin_node TEXT,
	version INTEGER
);

CREATE TABLE IF NOT EXISTS cluster_nodes (
    node_id TEXT PRIMARY KEY,
    address TEXT NOT NULL,
    source TEXT,
    last_seen DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS cluster_nodes_last_seen_idx ON cluster_nodes(last_seen);

CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    component TEXT NOT NULL,
    action TEXT NOT NULL,
    level TEXT NOT NULL,
    message TEXT,
    actor_id INTEGER,
    request_id TEXT,
    node_id TEXT,
    payload TEXT,
    occurred_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS audit_component_idx ON audit_logs(component, action);

-- Raft / Consenso: log replicado y metadatos persistentes
CREATE TABLE IF NOT EXISTS raft_log (
    term INTEGER NOT NULL,
    idx INTEGER NOT NULL,
    event_id TEXT UNIQUE,
    aggregate TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    op TEXT NOT NULL,
    payload TEXT NOT NULL,
    ts DATETIME NOT NULL,
    PRIMARY KEY(term, idx)
);

CREATE INDEX IF NOT EXISTS raft_log_idx ON raft_log(idx);

CREATE TABLE IF NOT EXISTS raft_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS raft_applied (
    event_id TEXT PRIMARY KEY,
    idx INTEGER NOT NULL,
    applied_at DATETIME NOT NULL
);

-- Inicialización básica de claves si no existen
INSERT OR IGNORE INTO raft_meta(key, value) VALUES
    ('currentTerm', '0'),
    ('votedFor', ''),
    ('commitIndex', '0'),
    ('lastApplied', '0');
`
	_, err := s.db.Exec(schema)
	return err
}

// ====================
// Usuarios
// ====================
func (s *Storage) CreateUser(u *User) error {
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO users(username,email,password_hash,display_name,created_at,updated_at)
		VALUES(?,?,?,?,?,?)`, u.Username, u.Email, u.PasswordHash, u.DisplayName, now, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	u.ID = id
	u.CreatedAt = now
	u.UpdatedAt = now
	return nil
}

func (s *Storage) GetUserByUsername(username string) (*User, error) {
	row := s.db.QueryRow(`SELECT id, username, email, password_hash, display_name, created_at, updated_at 
		FROM users WHERE username=?`, username)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.DisplayName, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Storage) GetUserByEmail(email string) (*User, error) {
	row := s.db.QueryRow(`SELECT id, username, email, password_hash, display_name, created_at, updated_at 
		FROM users WHERE email=?`, email)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.DisplayName, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Storage) GetUserByID(id int64) (*User, error) {
	row := s.db.QueryRow(`SELECT id, username, email, password_hash, display_name, created_at, updated_at FROM users WHERE id=?`, id)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.DisplayName, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Storage) UpdateUser(user *User) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE users 
		SET username=?, email=?, display_name=?, updated_at=?
		WHERE id=?`,
		user.Username, user.Email, user.DisplayName, now, user.ID)
	if err != nil {
		return err
	}
	user.UpdatedAt = now
	return nil
}

func (s *Storage) UpdatePassword(userID int64, newPasswordHash string) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE users 
		SET password_hash=?, updated_at=?
		WHERE id=?`,
		newPasswordHash, now, userID)
	return err
}

// EnsureUser attempts to ensure that a user with the given logical identity
// (username/email) exists. It is idempotent and enforces a strict policy:
//   - If a user with the same username exists and all fields match, it is a no-op.
//   - If a user with the same email exists and all fields match, it is a no-op.
//   - If username or email collide with different data, it returns a conflict error.
//   - If only the ID collides (i.e. an existing row has a different logical
//     identity), a new row is inserted and the caller's ID is updated.
func (s *Storage) EnsureUser(u *User) error {
	// Prefer to identify by username/email; ID is best-effort only.
	if strings.TrimSpace(u.Username) == "" && strings.TrimSpace(u.Email) == "" {
		return errors.New("ensure_user: missing username and email")
	}

	// Check existing by username first.
	if u.Username != "" {
		if existing, err := s.GetUserByUsername(u.Username); err == nil && existing != nil {
			// Same logical user?
			if existing.Email == u.Email && existing.DisplayName == u.DisplayName && existing.PasswordHash == u.PasswordHash {
				// Idempotent: update caller with existing details and return success.
				u.ID = existing.ID
				u.CreatedAt = existing.CreatedAt
				u.UpdatedAt = existing.UpdatedAt
				return nil
			}
			// Strict conflict on username with different data.
			return fmt.Errorf("ensure_user conflict: username %s already exists with different data", u.Username)
		}
	}

	// Check existing by email.
	if u.Email != "" {
		if existing, err := s.GetUserByEmail(u.Email); err == nil && existing != nil {
			if existing.Username == u.Username && existing.DisplayName == u.DisplayName && existing.PasswordHash == u.PasswordHash {
				u.ID = existing.ID
				u.CreatedAt = existing.CreatedAt
				u.UpdatedAt = existing.UpdatedAt
				return nil
			}
			// Strict conflict on email with different data.
			return fmt.Errorf("ensure_user conflict: email %s already exists with different data", u.Email)
		}
	}

	// At this point, username/email are free (or not set). We create a new user
	// letting the database choose a fresh ID, which naturally avoids ID
	// collisions. This also covers the case where the only conflicting field was
	// the ID: the new row will get a new ID and both users will coexist.

	// We intentionally ignore u.ID here and let CreateUser assign one.
	nu := &User{
		Username:     u.Username,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		DisplayName:  u.DisplayName,
	}
	if err := s.CreateUser(nu); err != nil {
		// In case of a race on UNIQUE(username/email), re-check using the
		// idempotency rules above.
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			if u.Username != "" {
				if existing, gerr := s.GetUserByUsername(u.Username); gerr == nil && existing != nil {
					if existing.Email == u.Email && existing.DisplayName == u.DisplayName && existing.PasswordHash == u.PasswordHash {
						u.ID = existing.ID
						u.CreatedAt = existing.CreatedAt
						u.UpdatedAt = existing.UpdatedAt
						return nil
					}
					return fmt.Errorf("ensure_user conflict after insert race: username %s now exists with different data", u.Username)
				}
			}
			if u.Email != "" {
				if existing, gerr := s.GetUserByEmail(u.Email); gerr == nil && existing != nil {
					if existing.Username == u.Username && existing.DisplayName == u.DisplayName && existing.PasswordHash == u.PasswordHash {
						u.ID = existing.ID
						u.CreatedAt = existing.CreatedAt
						u.UpdatedAt = existing.UpdatedAt
						return nil
					}
					return fmt.Errorf("ensure_user conflict after insert race: email %s now exists with different data", u.Email)
				}
			}
		}
		return err
	}

	// Propagate assigned ID/timestamps back to caller.
	u.ID = nu.ID
	u.CreatedAt = nu.CreatedAt
	u.UpdatedAt = nu.UpdatedAt
	return nil
}

func (s *Storage) ClearUserEmailIfMatches(userID int64, email string) error {
	_, err := s.db.Exec(`UPDATE users SET email=NULL, updated_at=? WHERE id=? AND email=?`, time.Now(), userID, email)
	return err
}

// ====================
// Grupos y miembros
// ====================
func (s *Storage) CreateGroup(g *Group) error {
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO groups(name, description, created_at, updated_at, creator_id, creator_username, group_type) VALUES(?,?,?,?,?,?,?)`,
		g.Name, g.Description, now, now, g.CreatorID, g.CreatorUserName, g.GroupType)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	g.ID = id
	g.CreatedAt = now
	g.UpdatedAt = now
	// Persist group create event for reconciliation
	payload := fmt.Sprintf(`{"name":%q,"description":%q,"creator_id":%d,"creator_username":%q,"group_type":%q}`,
		g.Name, g.Description, g.CreatorID, g.CreatorUserName, g.GroupType)
	evt := &Event{
		Entity:     "group",
		EntityID:   g.ID,
		Action:     "create",
		Payload:    payload,
		OriginNode: g.CreatorUserName,
		Version:    1,
	}
	_ = s.AppendEvent(evt)
	return nil
}

func (s *Storage) AddGroupMember(groupID, userID int64, rank int, addedBy *int64) error {
	now := time.Now()
	_, err := s.db.Exec(`INSERT OR REPLACE INTO group_members(group_id,user_id,rank,added_by,created_at)
		VALUES(?,?,?,?,?)`, groupID, userID, rank, addedBy, now)
	if err != nil {
		return err
	}
	// Emit event for reconciliation of memberships. Include username for ID mapping.
	var username string
	if user, err := s.GetUserByID(userID); err == nil && user != nil {
		username = user.Username
	}
	payload := fmt.Sprintf(`{"group_id":%d,"user_id":%d,"username":%q,"rank":%d}`, groupID, userID, username, rank)
	evt := &Event{
		Entity:     "group_member",
		EntityID:   groupID,
		Action:     "add",
		Payload:    payload,
		OriginNode: "",
		Version:    1,
	}
	_ = s.AppendEvent(evt)
	return nil
}

func (s *Storage) EnsureGroupMember(groupID, userID int64, rank int, addedBy *int64) error {
	return s.AddGroupMember(groupID, userID, rank, addedBy)
}

func (s *Storage) GetMemberRank(groupID, userID int64) (int, error) {
	row := s.db.QueryRow(`SELECT rank FROM group_members WHERE group_id=? AND user_id=?`, groupID, userID)
	var r int
	if err := row.Scan(&r); err != nil {
		return 0, err
	}
	return r, nil
}

func (s *Storage) IsSuperior(groupID, userA, userB int64) (bool, error) {
	rankA, err := s.GetMemberRank(groupID, userA)
	if err != nil {
		return false, err
	}
	rankB, err := s.GetMemberRank(groupID, userB)
	if err != nil {
		return false, err
	}
	return rankA > rankB, nil
}

func (s *Storage) GetGroupMembers(groupID int64) ([]GroupMember, error) {
	rows, err := s.db.Query(`
        SELECT gm.group_id, gm.user_id, gm.rank, gm.added_by, gm.created_at, u.username
        FROM group_members gm
        LEFT JOIN users u ON gm.user_id = u.id
        WHERE gm.group_id=?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []GroupMember
	for rows.Next() {
		var gm GroupMember
		var username sql.NullString
		if err := rows.Scan(&gm.GroupID, &gm.UserID, &gm.Rank, &gm.AddedBy, &gm.CreatedAt, &username); err != nil {
			return nil, err
		}
		if username.Valid {
			gm.Username = username.String
		}
		members = append(members, gm)
	}
	return members, nil
}

// Fetch group by ID
func (s *Storage) GetGroupByID(id int64) (*Group, error) {
	row := s.db.QueryRow(`SELECT id,name,description,created_at,updated_at,creator_id,creator_username,group_type FROM groups WHERE id=?`, id)
	var g Group
	if err := row.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt, &g.CreatorID, &g.CreatorUserName, &g.GroupType); err != nil {
		return nil, err
	}
	return &g, nil
}

// FindGroupBySignature provides a best-effort natural key to detect if a group
// already exists based on creator and name/group_type.
func (s *Storage) FindGroupBySignature(name string, creatorID int64, groupType GroupType) (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM groups WHERE name=? AND creator_id=? AND group_type=? ORDER BY id DESC LIMIT 1`,
		name, creatorID, groupType).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

// UpdateGroup updates group information
func (s *Storage) UpdateGroup(g *Group) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE groups 
		SET name=?, description=?, updated_at=?
		WHERE id=?`,
		g.Name, g.Description, now, g.ID)
	if err != nil {
		return err
	}
	g.UpdatedAt = now
	return nil
}

// DeleteGroup deletes a group and all its members
func (s *Storage) DeleteGroup(groupID int64) error {
	// Start transaction to ensure atomicity
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete group members first
	_, err = tx.Exec(`DELETE FROM group_members WHERE group_id=?`, groupID)
	if err != nil {
		return err
	}

	// Delete group appointments and participants
	_, err = tx.Exec(`DELETE FROM participants WHERE appointment_id IN (SELECT id FROM appointments WHERE group_id=?)`, groupID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`DELETE FROM appointments WHERE group_id=?`, groupID)
	if err != nil {
		return err
	}

	// Delete the group
	_, err = tx.Exec(`DELETE FROM groups WHERE id=?`, groupID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateGroupMember updates a member's rank
func (s *Storage) UpdateGroupMember(groupID, userID int64, rank int) error {
	_, err := s.db.Exec(`UPDATE group_members 
		SET rank=?
		WHERE group_id=? AND user_id=?`,
		rank, groupID, userID)
	return err
}

// RemoveGroupMember removes a member from a group
func (s *Storage) RemoveGroupMember(groupID, userID int64) error {
	// Start transaction to ensure atomicity
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Remove from group members first
	result, err := tx.Exec(`DELETE FROM group_members WHERE group_id=? AND user_id=?`, groupID, userID)
	if err != nil {
		return err
	}

	// Check if any rows were affected
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("member not found in group")
	}

	// Remove from all group appointments
	_, err = tx.Exec(`DELETE FROM participants WHERE user_id=? AND appointment_id IN (SELECT id FROM appointments WHERE group_id=?)`, userID, groupID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateParticipantStatus updates a participant's status (accept/reject invitation)
func (s *Storage) UpdateParticipantStatus(appointmentID, userID int64, status ApptStatus) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE participants 
		SET status=?, updated_at=?
		WHERE appointment_id=? AND user_id=?`,
		status, now, appointmentID, userID)
	if err != nil {
		return err
	}
	// Emit event to allow reconciliation of invitation status across partitions. Include username and appointment info for ID mapping.
	var username string
	if user, err := s.GetUserByID(userID); err == nil && user != nil {
		username = user.Username
	}
	// Get appointment info for ID mapping
	var apptOwnerUsername, apptGroupName, apptGroupCreatorUsername, apptTitle, apptStart, apptEnd string
	var apptGroupID *int64
	if appt, err := s.GetAppointmentByID(appointmentID); err == nil && appt != nil {
		apptTitle = appt.Title
		apptStart = appt.Start.Format(time.RFC3339)
		apptEnd = appt.End.Format(time.RFC3339)
		apptGroupID = appt.GroupID
		if owner, err := s.GetUserByID(appt.OwnerID); err == nil && owner != nil {
			apptOwnerUsername = owner.Username
		}
		if appt.GroupID != nil {
			if group, err := s.GetGroupByID(*appt.GroupID); err == nil && group != nil {
				apptGroupName = group.Name
				apptGroupCreatorUsername = group.CreatorUserName
			}
		}
	}
	groupIDVal := "null"
	if apptGroupID != nil {
		groupIDVal = strconv.FormatInt(*apptGroupID, 10)
	}
	payload := fmt.Sprintf(`{"appointment_id":%d,"user_id":%d,"username":%q,"status":%q,"appt_owner_username":%q,"appt_group_id":%s,"appt_group_name":%q,"appt_group_creator_username":%q,"appt_title":%q,"appt_start":%q,"appt_end":%q}`,
		appointmentID, userID, username, status, apptOwnerUsername, groupIDVal, apptGroupName, apptGroupCreatorUsername, apptTitle, apptStart, apptEnd)
	evt := &Event{
		Entity:   "invitation",
		EntityID: appointmentID,
		Action:   "status_change",
		Payload:  payload,
		// OriginNode left empty; it is mainly logical and not required here.
		Version: 1,
	}
	_ = s.AppendEvent(evt)
	return nil
}

// GetParticipantByAppointmentAndUser gets a specific participant
func (s *Storage) GetParticipantByAppointmentAndUser(appointmentID, userID int64) (*Participant, error) {
	row := s.db.QueryRow(`SELECT id, appointment_id, user_id, status, is_optional, created_at, updated_at 
		FROM participants WHERE appointment_id=? AND user_id=?`, appointmentID, userID)
	var p Participant
	if err := row.Scan(&p.ID, &p.AppointmentID, &p.UserID, &p.Status, &p.IsOptional, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

// FindAppointmentBySignature tries to find an existing (non-deleted) appointment by a
// natural signature. This is used to provide best-effort idempotency for create ops.
func (s *Storage) FindAppointmentBySignature(ownerID int64, groupID *int64, start, end time.Time, title string) (int64, error) {
	if groupID == nil {
		var id int64
		err := s.db.QueryRow(`SELECT id FROM appointments
			WHERE owner_id=? AND group_id IS NULL AND start_ts=? AND end_ts=? AND title=? AND deleted=0
			ORDER BY id DESC LIMIT 1`, ownerID, start.Unix(), end.Unix(), title).Scan(&id)
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return id, err
	}
	var id int64
	err := s.db.QueryRow(`SELECT id FROM appointments
		WHERE owner_id=? AND group_id=? AND start_ts=? AND end_ts=? AND title=? AND deleted=0
		ORDER BY id DESC LIMIT 1`, ownerID, *groupID, start.Unix(), end.Unix(), title).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

// List all groups the user belongs to
func (s *Storage) GetGroupsForUser(userID int64) ([]Group, error) {
	rows, err := s.db.Query(`
		SELECT g.id, g.name, g.description, g.created_at, g.updated_at, g.creator_id, g.creator_username, g.group_type
		FROM groups g
		JOIN group_members gm ON gm.group_id = g.id
		WHERE gm.user_id = ?
		ORDER BY g.name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt, &g.CreatorID, &g.CreatorUserName, &g.GroupType); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// ====================
// Citas
// ====================
func (s *Storage) CreateAppointment(a *Appointment) error {
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO appointments(title,description,owner_id,group_id,start_ts,end_ts,privacy,status,version,origin_node,deleted,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?, ?,?)`,
		a.Title, a.Description, a.OwnerID, a.GroupID,
		a.Start.Unix(), a.End.Unix(), a.Privacy, a.Status,
		1, a.OriginNode, 0, now, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	a.ID = id
	a.CreatedAt = now
	a.UpdatedAt = now

	// Persist an event for reconciliation. Payload includes owner_username and group info for ID mapping.
	// Get owner username for reconciliation (to map remote IDs to local IDs)
	var ownerUsername string
	if owner, err := s.GetUserByID(a.OwnerID); err == nil && owner != nil {
		ownerUsername = owner.Username
	}
	// Get group info for ID mapping during reconciliation
	var groupName, groupCreatorUsername string
	groupIDVal := "null"
	if a.GroupID != nil {
		groupIDVal = strconv.FormatInt(*a.GroupID, 10)
		if group, err := s.GetGroupByID(*a.GroupID); err == nil && group != nil {
			groupName = group.Name
			groupCreatorUsername = group.CreatorUserName
		}
	}
	payload := fmt.Sprintf(`{"owner_id":%d,"owner_username":%q,"group_id":%s,"group_name":%q,"group_creator_username":%q,"title":%q,"description":%q,"start":%q,"end":%q,"privacy":%q}`,
		a.OwnerID, ownerUsername, groupIDVal, groupName, groupCreatorUsername, a.Title, a.Description, a.Start.Format(time.RFC3339), a.End.Format(time.RFC3339), a.Privacy)
	evt := &Event{
		Entity:     "appointment",
		EntityID:   a.ID,
		Action:     "create",
		Payload:    payload,
		OriginNode: a.OriginNode,
		Version:    a.Version,
	}
	_ = s.AppendEvent(evt)
	return nil
}

func (s *Storage) UpdateAppointment(a *Appointment) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE appointments 
		SET title=?, description=?, start_ts=?, end_ts=?, privacy=?, updated_at=?, version=version+1
		WHERE id=? AND deleted=0`,
		a.Title, a.Description, a.Start.Unix(), a.End.Unix(), a.Privacy, now, a.ID)
	if err != nil {
		return err
	}
	a.UpdatedAt = now
	return nil
}

func (s *Storage) DeleteAppointment(appointmentID int64) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE appointments 
		SET deleted=1, updated_at=?, version=version+1
		WHERE id=?`,
		now, appointmentID)
	return err
}

func (s *Storage) AddParticipant(p *Participant) error {
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO participants(appointment_id,user_id,status,is_optional,created_at,updated_at)
		VALUES(?,?,?,?,?,?)`,
		p.AppointmentID, p.UserID, p.Status, p.IsOptional, now, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	p.ID = id
	p.CreatedAt = now
	p.UpdatedAt = now
	return nil
}

func (s *Storage) EnsureParticipant(appointmentID, userID int64, status ApptStatus, isOptional bool) error {
	if existing, err := s.GetParticipantByAppointmentAndUser(appointmentID, userID); err == nil && existing != nil {
		if existing.Status != status {
			return s.UpdateParticipantStatus(appointmentID, userID, status)
		}
		return nil
	}
	return s.AddParticipant(&Participant{AppointmentID: appointmentID, UserID: userID, Status: status, IsOptional: isOptional})
}

func (s *Storage) HasConflict(userID int64, start, end time.Time) (bool, error) {
	q := `
SELECT COUNT(1)
FROM appointments a
JOIN participants p ON p.appointment_id = a.id
WHERE p.user_id = ?
  AND a.deleted = 0
  AND p.status IN ('accepted','auto')
  AND NOT (a.end_ts <= ? OR a.start_ts >= ?)`
	var cnt int
	row := s.db.QueryRow(q, userID, start.Unix(), end.Unix())
	if err := row.Scan(&cnt); err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func (s *Storage) HasConflictExcluding(userID int64, start, end time.Time, excludeAppointmentID int64) (bool, error) {
	q := `
SELECT COUNT(1)
FROM appointments a
JOIN participants p ON p.appointment_id = a.id
WHERE p.user_id = ?
  AND a.deleted = 0
  AND a.id != ?
  AND p.status IN ('accepted','auto')
  AND NOT (a.end_ts <= ? OR a.start_ts >= ?)`
	var cnt int
	row := s.db.QueryRow(q, userID, excludeAppointmentID, start.Unix(), end.Unix())
	if err := row.Scan(&cnt); err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// Crear cita grupal con reglas de jerarquía
func (s *Storage) CreateGroupAppointment(a *Appointment) ([]Participant, error) {
	if a.GroupID == nil {
		return nil, errors.New("group appointment requires GroupID")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	rollback := func() { _ = tx.Rollback() }

	now := time.Now()
	// Insertar cita
	res, err := tx.Exec(`INSERT INTO appointments(title,description,owner_id,group_id,start_ts,end_ts,privacy,status,version,origin_node,deleted,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.Title, a.Description, a.OwnerID, a.GroupID,
		a.Start.Unix(), a.End.Unix(), a.Privacy, a.Status,
		1, a.OriginNode, 0, now, now)
	if err != nil {
		rollback()
		return nil, err
	}
	apptID, _ := res.LastInsertId()
	a.ID = apptID
	a.CreatedAt = now
	a.UpdatedAt = now

	// Obtener información del grupo para determinar el tipo
	group, err := s.GetGroupByID(*a.GroupID)
	if err != nil {
		rollback()
		return nil, err
	}

	// Miembros del grupo
	members, err := s.GetGroupMembers(*a.GroupID)
	if err != nil {
		rollback()
		return nil, err
	}

	participants := []Participant{}
	for _, m := range members {
		var status ApptStatus

		// Determinar el estado según el tipo de grupo
		if group.GroupType == GroupTypeNonHierarchical {
			// En grupos sin jerarquía, todos los eventos requieren aprobación
			// excepto para el creador que se auto-acepta
			if m.UserID == a.OwnerID {
				status = StatusAuto
			} else {
				status = StatusPending
			}
		} else {
			// En grupos jerárquicos, usar la lógica existente
			creatorRank, err := s.GetMemberRank(*a.GroupID, a.OwnerID)
			if err != nil {
				rollback()
				return nil, err
			}
			if m.Rank > creatorRank {
				status = StatusPending
			} else {
				status = StatusAuto
			}
		}
		res2, err := tx.Exec(`INSERT INTO participants(appointment_id,user_id,status,is_optional,created_at,updated_at)
			VALUES(?,?,?,?,?,?)`,
			apptID, m.UserID, status, 0, now, now)
		if err != nil {
			rollback()
			return nil, err
		}
		pid, _ := res2.LastInsertId()
		p := Participant{
			ID:            pid,
			AppointmentID: apptID,
			UserID:        m.UserID,
			Status:        status,
			IsOptional:    false,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		participants = append(participants, p)

		// Obtener información del creador para enriquecer la notificación
		var creatorUsername, creatorDisplayName string
		if creator, err := s.GetUserByID(a.OwnerID); err == nil && creator != nil {
			creatorUsername = creator.Username
			creatorDisplayName = creator.DisplayName
		}

		// Obtener información del grupo
		var groupName string
		if group, err := s.GetGroupByID(*a.GroupID); err == nil && group != nil {
			groupName = group.Name
		}

		// Notificación inicial con payload enriquecido
		payload := fmt.Sprintf(`{"appointment_id":%d,"title":"%s","description":"%s","start":"%s","end":"%s","group_id":%d,"group_name":"%s","created_by_id":%d,"created_by_username":"%s","created_by_display_name":"%s","status":"%s","privacy":"%s"}`,
			apptID, a.Title, a.Description, a.Start.Format(time.RFC3339), a.End.Format(time.RFC3339),
			*a.GroupID, groupName, a.OwnerID, creatorUsername, creatorDisplayName, status, a.Privacy)
		if _, err := tx.Exec(`INSERT INTO notifications(user_id,type,payload,created_at)
			VALUES(?,?,?,?)`, m.UserID, "invite", payload, now); err != nil {
			rollback()
			return nil, err
		}
	}

	// Commit
	if err := tx.Commit(); err != nil {
		rollback()
		return nil, err
	}

	// Evento (fuera de la transacción) con payload estructurado para reconciliación.
	// Include owner_username and group info for ID mapping during reconciliation
	var ownerUsername string
	if owner, err := s.GetUserByID(a.OwnerID); err == nil && owner != nil {
		ownerUsername = owner.Username
	}
	// Get group info for ID mapping
	var groupName, groupCreatorUsername string
	if group, err := s.GetGroupByID(*a.GroupID); err == nil && group != nil {
		groupName = group.Name
		groupCreatorUsername = group.CreatorUserName
	}
	payload := fmt.Sprintf(`{"owner_id":%d,"owner_username":%q,"group_id":%d,"group_name":%q,"group_creator_username":%q,"title":%q,"description":%q,"start":%q,"end":%q,"privacy":%q}`,
		a.OwnerID, ownerUsername, *a.GroupID, groupName, groupCreatorUsername, a.Title, a.Description, a.Start.Format(time.RFC3339), a.End.Format(time.RFC3339), a.Privacy)
	evt := &Event{
		Entity:     "appointment",
		EntityID:   a.ID,
		Action:     "create",
		Payload:    payload,
		OriginNode: a.OriginNode,
		Version:    a.Version,
	}
	_ = s.AppendEvent(evt)

	return participants, nil
}

// ====================
// Notificaciones
// ====================
func (s *Storage) AddNotification(n *Notification) error {
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO notifications(user_id,type,payload,read_at,created_at)
		VALUES(?,?,?,?,?)`,
		n.UserID, n.Type, n.Payload, n.ReadAt, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	n.ID = id
	n.CreatedAt = now
	// Emit event so notifications can be reconciled across partitions.
	// Include user_id and username for ID mapping during reconciliation
	var username string
	if user, err := s.GetUserByID(n.UserID); err == nil && user != nil {
		username = user.Username
	}
	// Enrich payload with user_id and username for reconciliation
	enrichedPayload := fmt.Sprintf(`{"user_id":%d,"username":%q,"type":%q,"payload":%q}`,
		n.UserID, username, n.Type, n.Payload)
	evt := &Event{
		Entity:   "notification",
		EntityID: n.ID,
		Action:   "create",
		Payload:  enrichedPayload,
		Version:  1,
	}
	_ = s.AppendEvent(evt)
	return nil
}

func (s *Storage) FindNotificationBySignature(userID int64, nType, payload string) (int64, error) {
	row := s.db.QueryRow(`SELECT id FROM notifications WHERE user_id=? AND type=? AND payload=? ORDER BY id DESC LIMIT 1`, userID, nType, payload)
	var id int64
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return id, nil
}

func (s *Storage) EnsureNotification(n *Notification) error {
	id, err := s.FindNotificationBySignature(n.UserID, n.Type, n.Payload)
	if err != nil {
		return err
	}
	if id != 0 {
		return nil
	}
	return s.AddNotification(n)
}

func (s *Storage) GetUserNotifications(userID int64) ([]Notification, error) {
	rows, err := s.db.Query(`SELECT id,user_id,type,payload,read_at,created_at FROM notifications WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notes []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Payload, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, nil
}

// 🔥 NUEVO: marcar notificación como leída
func (s *Storage) MarkNotificationRead(notificationID int64) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE notifications SET read_at=? WHERE id=?`, now, notificationID)
	return err
}

// 🔥 NUEVO: obtener solo notificaciones no leídas
func (s *Storage) GetUnreadNotifications(userID int64) ([]Notification, error) {
	rows, err := s.db.Query(`SELECT id,user_id,type,payload,read_at,created_at FROM notifications WHERE user_id=? AND read_at IS NULL`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Payload, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, nil
}

// ====================
// Consultas de Agenda
// ====================

// Devuelve todas las citas de un usuario en un rango de tiempo.
// Incluye citas personales y grupales donde el usuario es participante.
func (s *Storage) GetUserAgenda(userID int64, start, end time.Time) ([]Appointment, error) {
	q := `
SELECT a.id, a.title, a.description, a.owner_id, a.group_id,
       a.start_ts, a.end_ts, a.privacy, a.status,
       a.created_at, a.updated_at, a.version, a.origin_node, a.deleted
FROM appointments a
JOIN participants p ON p.appointment_id = a.id
WHERE p.user_id = ?
  AND a.deleted = 0
  AND NOT (a.end_ts <= ? OR a.start_ts >= ?)
ORDER BY a.start_ts ASC`
	rows, err := s.db.Query(q, userID, start.Unix(), end.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []Appointment
	for rows.Next() {
		var a Appointment
		var startTS, endTS int64
		if err := rows.Scan(&a.ID, &a.Title, &a.Description, &a.OwnerID, &a.GroupID,
			&startTS, &endTS, &a.Privacy, &a.Status,
			&a.CreatedAt, &a.UpdatedAt, &a.Version, &a.OriginNode, &a.Deleted); err != nil {
			return nil, err
		}
		a.Start = time.Unix(startTS, 0)
		a.End = time.Unix(endTS, 0)
		apps = append(apps, a)
	}
	return apps, nil
}

// Devuelve todas las citas de un grupo en un rango de tiempo.
// Incluye tanto citas creadas a nivel de grupo como personales de sus miembros (si se requiere).
func (s *Storage) GetGroupAgenda(groupID int64, start, end time.Time) ([]Appointment, error) {
	q := `
SELECT a.id, a.title, a.description, a.owner_id, a.group_id,
       a.start_ts, a.end_ts, a.privacy, a.status,
       a.created_at, a.updated_at, a.version, a.origin_node, a.deleted
FROM appointments a
WHERE a.group_id = ?
  AND a.deleted = 0
  AND NOT (a.end_ts <= ? OR a.start_ts >= ?)
ORDER BY a.start_ts ASC`
	rows, err := s.db.Query(q, groupID, start.Unix(), end.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []Appointment
	for rows.Next() {
		var a Appointment
		var startTS, endTS int64
		if err := rows.Scan(&a.ID, &a.Title, &a.Description, &a.OwnerID, &a.GroupID,
			&startTS, &endTS, &a.Privacy, &a.Status,
			&a.CreatedAt, &a.UpdatedAt, &a.Version, &a.OriginNode, &a.Deleted); err != nil {
			return nil, err
		}
		a.Start = time.Unix(startTS, 0)
		a.End = time.Unix(endTS, 0)
		apps = append(apps, a)
	}
	return apps, nil
}

// ====================
// Eventos
// ====================

// 🔥 NUEVO: AppendEvent para EventRepository
func (s *Storage) AppendEvent(e *Event) error {
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO events(entity, entity_id, action, payload, created_at, origin_node, version)
		VALUES(?,?,?,?,?,?,?)`,
		e.Entity, e.EntityID, e.Action, e.Payload, now, e.OriginNode, e.Version)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	e.ID = id
	e.CreatedAt = now
	return nil
}

// ListEvents returns events matching the given filter, ordered by creation.
func (s *Storage) ListEvents(filter EventFilter) ([]Event, error) {
	qry := `SELECT id, entity, entity_id, action, payload, created_at, origin_node, version
		FROM events WHERE 1=1`
	args := []any{}
	if filter.Entity != "" {
		qry += " AND entity=?"
		args = append(args, filter.Entity)
	}
	if filter.Action != "" {
		qry += " AND action=?"
		args = append(args, filter.Action)
	}
	if !filter.Since.IsZero() {
		qry += " AND created_at>=?"
		args = append(args, filter.Since)
	}
	qry += " ORDER BY created_at ASC, id ASC"
	if filter.Limit > 0 {
		qry += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	rows, err := s.db.Query(qry, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Entity, &e.EntityID, &e.Action, &e.Payload, &e.CreatedAt, &e.OriginNode, &e.Version); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// AppendAudit stores an immutable audit record.
func (s *Storage) AppendAudit(entry *AuditLog) error {
	if entry == nil {
		return errors.New("nil audit entry")
	}
	if entry.OccurredAt.IsZero() {
		entry.OccurredAt = time.Now()
	}
	res, err := s.db.Exec(`INSERT INTO audit_logs(component, action, level, message, actor_id, request_id, node_id, payload, occurred_at)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		entry.Component, entry.Action, entry.Level, entry.Message, entry.ActorID, entry.RequestID, entry.NodeID, entry.Payload, entry.OccurredAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	entry.ID = id
	return nil
}

// ListAuditLogs returns the newest audit entries that match the provided filter.
func (s *Storage) ListAuditLogs(filter AuditFilter) ([]AuditLog, error) {
	query := `SELECT id, component, action, level, message, actor_id, request_id, node_id, payload, occurred_at
		FROM audit_logs`
	var clauses []string
	var args []any
	if filter.Component != "" {
		clauses = append(clauses, "component = ?")
		args = append(args, filter.Component)
	}
	if filter.Action != "" {
		clauses = append(clauses, "action = ?")
		args = append(args, filter.Action)
	}
	if filter.Level != "" {
		clauses = append(clauses, "level = ?")
		args = append(args, filter.Level)
	}
	if filter.RequestID != "" {
		clauses = append(clauses, "request_id = ?")
		args = append(args, filter.RequestID)
	}
	if !filter.Since.IsZero() {
		clauses = append(clauses, "occurred_at >= ?")
		args = append(args, filter.Since)
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY occurred_at DESC"
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	query += " LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var entry AuditLog
		var actor sql.NullInt64
		if err := rows.Scan(&entry.ID, &entry.Component, &entry.Action, &entry.Level, &entry.Message,
			&actor, &entry.RequestID, &entry.NodeID, &entry.Payload, &entry.OccurredAt); err != nil {
			return nil, err
		}
		if actor.Valid {
			val := actor.Int64
			entry.ActorID = &val
		}
		logs = append(logs, entry)
	}
	return logs, rows.Err()
}

// ====================
// Raft applied tracking
// ====================

func (s *Storage) HasAppliedEvent(eventID string) (bool, error) {
	if strings.TrimSpace(eventID) == "" {
		return false, errors.New("empty event id")
	}
	var dummy int
	err := s.db.QueryRow(`SELECT 1 FROM raft_applied WHERE event_id=?`, eventID).Scan(&dummy)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Storage) RecordAppliedEvent(eventID string, idx int64) error {
	if strings.TrimSpace(eventID) == "" {
		return errors.New("empty event id")
	}
	if idx < 0 {
		idx = 0
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO raft_applied(event_id, idx, applied_at) VALUES(?,?,?)`,
		eventID, idx, time.Now())
	return err
}

// ====================
// Cluster Nodes
// ====================

func (s *Storage) UpsertClusterNode(node *ClusterNode) error {
	if node == nil {
		return errors.New("nil cluster node")
	}
	if node.NodeID == "" {
		return errors.New("empty node id")
	}
	if node.Address == "" {
		node.Address = node.NodeID
	}
	if node.LastSeen.IsZero() {
		node.LastSeen = time.Now()
	}
	_, err := s.db.Exec(`INSERT INTO cluster_nodes(node_id, address, source, last_seen)
		VALUES(?,?,?,?)
		ON CONFLICT(node_id) DO UPDATE SET address=excluded.address, source=excluded.source, last_seen=excluded.last_seen`,
		node.NodeID, node.Address, node.Source, node.LastSeen)
	return err
}

func (s *Storage) RemoveClusterNode(nodeID string) error {
	if nodeID == "" {
		return errors.New("empty node id")
	}
	_, err := s.db.Exec(`DELETE FROM cluster_nodes WHERE node_id=?`, nodeID)
	return err
}

func (s *Storage) ListClusterNodes() ([]ClusterNode, error) {
	rows, err := s.db.Query(`SELECT node_id, address, source, last_seen FROM cluster_nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []ClusterNode
	for rows.Next() {
		var n ClusterNode
		if err := rows.Scan(&n.NodeID, &n.Address, &n.Source, &n.LastSeen); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// ====================
// Appointment Details
// ====================

// GetAppointmentByID retrieves a specific appointment by ID
func (s *Storage) GetAppointmentByID(appointmentID int64) (*Appointment, error) {
	q := `
SELECT a.id, a.title, a.description, a.owner_id, a.group_id,
       a.start_ts, a.end_ts, a.privacy, a.status,
       a.created_at, a.updated_at, a.version, a.origin_node, a.deleted
FROM appointments a
WHERE a.id = ? AND a.deleted = 0`

	var a Appointment
	var startTS, endTS int64
	err := s.db.QueryRow(q, appointmentID).Scan(
		&a.ID, &a.Title, &a.Description, &a.OwnerID, &a.GroupID,
		&startTS, &endTS, &a.Privacy, &a.Status,
		&a.CreatedAt, &a.UpdatedAt, &a.Version, &a.OriginNode, &a.Deleted)

	if err != nil {
		return nil, err
	}

	a.Start = time.Unix(startTS, 0)
	a.End = time.Unix(endTS, 0)
	return &a, nil
}

// GetAppointmentParticipants retrieves all participants for an appointment with user details
func (s *Storage) GetAppointmentParticipants(appointmentID int64) ([]ParticipantDetails, error) {
	// Check if all users exist, create missing ones if needed
	rows, err := s.db.Query(`SELECT DISTINCT user_id FROM participants WHERE appointment_id = ?`, appointmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			continue
		}
		userIDs = append(userIDs, userID)
	}

	// Create missing users
	for _, userID := range userIDs {
		var count int
		err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, userID).Scan(&count)
		if err == nil && count == 0 {
			// Create a placeholder user
			_, _ = s.db.Exec(`INSERT INTO users(id, username, email, password_hash, display_name, created_at, updated_at) 
				VALUES(?, ?, ?, ?, ?, ?, ?)`,
				userID, fmt.Sprintf("user_%d", userID), "", "", fmt.Sprintf("User %d", userID), time.Now(), time.Now())
		}
	}

	q := `
SELECT p.id, p.appointment_id, p.user_id, p.status, p.is_optional,
       p.created_at, p.updated_at, 
       COALESCE(u.username, 'Unknown') as username, 
       COALESCE(u.display_name, 'Unknown User') as display_name
FROM participants p
LEFT JOIN users u ON p.user_id = u.id
WHERE p.appointment_id = ?
ORDER BY p.created_at ASC`

	rows, err = s.db.Query(q, appointmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var participants []ParticipantDetails
	for rows.Next() {
		var p ParticipantDetails
		err := rows.Scan(
			&p.ID, &p.AppointmentID, &p.UserID, &p.Status, &p.IsOptional,
			&p.CreatedAt, &p.UpdatedAt, &p.Username, &p.DisplayName)
		if err != nil {
			return nil, err
		}
		participants = append(participants, p)
	}
	return participants, nil
}
