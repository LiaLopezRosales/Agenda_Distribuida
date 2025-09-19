// storage.go
package agendadistribuida

import (
	"database/sql"
	"errors"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Storage struct {
	db *sql.DB
}

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
	updated_at DATETIME NOT NULL
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

// ====================
// Grupos y miembros
// ====================
func (s *Storage) CreateGroup(g *Group) error {
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO groups(name, description, created_at, updated_at) VALUES(?,?,?,?)`,
		g.Name, g.Description, now, now)
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
	rows, err := s.db.Query(`SELECT group_id,user_id,rank,added_by,created_at FROM group_members WHERE group_id=?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []GroupMember
	for rows.Next() {
		var gm GroupMember
		if err := rows.Scan(&gm.GroupID, &gm.UserID, &gm.Rank, &gm.AddedBy, &gm.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, gm)
	}
	return members, nil
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

// Crear cita grupal con reglas de jerarquía
func (s *Storage) CreateGroupAppointment(a *Appointment) ([]Participant, error) {
	if a.GroupID == nil {
		return nil, errors.New("group appointment requires GroupID")
	}

	// Crear cita
	if err := s.CreateAppointment(a); err != nil {
		return nil, err
	}

	// Obtener miembros
	members, err := s.GetGroupMembers(*a.GroupID)
	if err != nil {
		return nil, err
	}

	// Rank del creador
	creatorRank, err := s.GetMemberRank(*a.GroupID, a.OwnerID)
	if err != nil {
		return nil, err
	}

	participants := []Participant{}
	for _, m := range members {
		var status ApptStatus
		if m.Rank > creatorRank {
			status = StatusPending
		} else {
			status = StatusAuto
		}
		p := Participant{
			AppointmentID: a.ID,
			UserID:        m.UserID,
			Status:        status,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		if err := s.AddParticipant(&p); err != nil {
			return nil, err
		}
		participants = append(participants, p)
	}

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
