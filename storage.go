// storage.go
package agendadistribuida

import (
	"database/sql"
	"errors"
	"fmt"
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
	return nil
}

func (s *Storage) AddGroupMember(groupID, userID int64, rank int, addedBy *int64) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO group_members(group_id,user_id,rank,added_by,created_at)
		VALUES(?,?,?,?,?)`, groupID, userID, rank, addedBy, time.Now())
	return err
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
	return err
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
		1, "", 0, now, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	a.ID = id
	a.CreatedAt = now
	a.UpdatedAt = now
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

	// Evento (fuera de la transacción)
	evt := &Event{
		Entity:     "appointment",
		EntityID:   a.ID,
		Action:     "create",
		Payload:    "",
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
	return nil
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
