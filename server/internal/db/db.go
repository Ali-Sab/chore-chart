package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

// ─── Domain types ─────────────────────────────────────────────────────────────

type Group struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

type Chore struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	AssignedTo   string  `json:"assigned_to"`
	CadenceWeeks float64 `json:"cadence_weeks"`
}

// GroupData is the full load used at startup for schedule pre-computation (seed path).
type GroupData struct {
	ID      int64
	Name    string
	Members []string
	Chores  []Chore
}

// ─── Schema ───────────────────────────────────────────────────────────────────

const schema = `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;

CREATE TABLE IF NOT EXISTS groups (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS group_members (
    group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    PRIMARY KEY (group_id, username)
);

CREATE TABLE IF NOT EXISTS chores (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id      INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    assigned_to   TEXT NOT NULL,
    cadence_weeks REAL NOT NULL CHECK(cadence_weeks > 0)
);
`

// Open opens (or creates) the SQLite database at path and applies the schema.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}
	// Single writer to avoid locking contention with WAL.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	return db, nil
}

// ─── Seed ─────────────────────────────────────────────────────────────────────

type seedChore struct {
	Name         string  `json:"name"`
	AssignedTo   string  `json:"assigned_to"`
	CadenceWeeks float64 `json:"cadence_weeks"`
}

type seedGroup struct {
	Name    string      `json:"name"`
	Members []string    `json:"members"`
	Chores  []seedChore `json:"chores"`
}

type seedFile struct {
	Groups []seedGroup `json:"groups"`
}

// Seed reads seedPath and inserts groups/members/chores if the DB has no groups yet.
func Seed(db *sql.DB, seedPath string) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM groups`).Scan(&count); err != nil {
		return fmt.Errorf("checking group count: %w", err)
	}
	if count > 0 {
		return nil
	}

	raw, err := os.ReadFile(seedPath)
	if err != nil {
		return fmt.Errorf("reading seed file: %w", err)
	}
	var seed seedFile
	if err := json.Unmarshal(raw, &seed); err != nil {
		return fmt.Errorf("parsing seed file: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning seed transaction: %w", err)
	}
	defer tx.Rollback()

	for _, g := range seed.Groups {
		res, err := tx.Exec(`INSERT INTO groups(name) VALUES(?)`, g.Name)
		if err != nil {
			return fmt.Errorf("inserting group %q: %w", g.Name, err)
		}
		gid, _ := res.LastInsertId()
		for _, m := range g.Members {
			if _, err := tx.Exec(`INSERT INTO group_members(group_id, username) VALUES(?,?)`, gid, m); err != nil {
				return fmt.Errorf("inserting member %q: %w", m, err)
			}
		}
		for _, c := range g.Chores {
			if _, err := tx.Exec(`INSERT INTO chores(group_id,name,assigned_to,cadence_weeks) VALUES(?,?,?,?)`,
				gid, c.Name, c.AssignedTo, c.CadenceWeeks); err != nil {
				return fmt.Errorf("inserting chore %q: %w", c.Name, err)
			}
		}
	}
	return tx.Commit()
}

// ─── Group queries ────────────────────────────────────────────────────────────

// CreateGroup inserts a new group and adds creator as its first member.
// Returns the new group's ID.
func CreateGroup(db *sql.DB, name, creatorUsername string) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO groups(name) VALUES(?)`, name)
	if err != nil {
		return 0, fmt.Errorf("inserting group: %w", err)
	}
	gid, _ := res.LastInsertId()
	if _, err := tx.Exec(`INSERT INTO group_members(group_id,username) VALUES(?,?)`, gid, creatorUsername); err != nil {
		return 0, fmt.Errorf("adding creator as member: %w", err)
	}
	return gid, tx.Commit()
}

// GetGroupsForUser returns all groups the username belongs to, with members list.
func GetGroupsForUser(db *sql.DB, username string) ([]Group, error) {
	rows, err := db.Query(`
		SELECT g.id, g.name
		FROM groups g
		JOIN group_members gm ON gm.group_id = g.id
		WHERE gm.username = ?
		ORDER BY g.id`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, g := range groups {
		members, err := membersOf(db, g.ID)
		if err != nil {
			return nil, err
		}
		groups[i].Members = members
	}
	return groups, nil
}

// GetGroup returns a single group by ID (without membership check).
func GetGroup(db *sql.DB, groupID int64) (Group, error) {
	var g Group
	if err := db.QueryRow(`SELECT id, name FROM groups WHERE id=?`, groupID).Scan(&g.ID, &g.Name); err != nil {
		return Group{}, err
	}
	members, err := membersOf(db, groupID)
	if err != nil {
		return Group{}, err
	}
	g.Members = members
	return g, nil
}

// IsMember reports whether username is a member of groupID.
func IsMember(db *sql.DB, groupID int64, username string) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM group_members WHERE group_id=? AND username=?`, groupID, username).Scan(&n)
	return n > 0, err
}

// AddMember adds username to groupID. Silently succeeds if already a member.
func AddMember(db *sql.DB, groupID int64, username string) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO group_members(group_id,username) VALUES(?,?)`, groupID, username)
	return err
}

// RemoveMember removes username from groupID.
// If this makes the group empty, the group is deleted.
// Returns (groupDeleted bool, error).
func RemoveMember(db *sql.DB, groupID int64, username string) (bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM group_members WHERE group_id=? AND username=?`, groupID, username); err != nil {
		return false, err
	}

	var remaining int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM group_members WHERE group_id=?`, groupID).Scan(&remaining); err != nil {
		return false, err
	}

	deleted := false
	if remaining == 0 {
		if _, err := tx.Exec(`DELETE FROM groups WHERE id=?`, groupID); err != nil {
			return false, err
		}
		deleted = true
	}
	return deleted, tx.Commit()
}

// ─── Chore queries ────────────────────────────────────────────────────────────

// GetChores returns all chores for a group ordered by assigned_to, name.
func GetChores(db *sql.DB, groupID int64) ([]Chore, error) {
	rows, err := db.Query(
		`SELECT id, name, assigned_to, cadence_weeks FROM chores WHERE group_id=? ORDER BY assigned_to, name`,
		groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chores []Chore
	for rows.Next() {
		var c Chore
		if err := rows.Scan(&c.ID, &c.Name, &c.AssignedTo, &c.CadenceWeeks); err != nil {
			return nil, err
		}
		chores = append(chores, c)
	}
	return chores, rows.Err()
}

// AddChore inserts a chore and returns its new ID.
func AddChore(db *sql.DB, groupID int64, name, assignedTo string, cadenceWeeks float64) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO chores(group_id,name,assigned_to,cadence_weeks) VALUES(?,?,?,?)`,
		groupID, name, assignedTo, cadenceWeeks)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RemoveChore deletes a chore, verifying it belongs to groupID.
func RemoveChore(db *sql.DB, groupID, choreID int64) error {
	res, err := db.Exec(`DELETE FROM chores WHERE id=? AND group_id=?`, choreID, groupID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("chore not found")
	}
	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func membersOf(db *sql.DB, groupID int64) ([]string, error) {
	rows, err := db.Query(`SELECT username FROM group_members WHERE group_id=? ORDER BY username`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		members = append(members, u)
	}
	return members, rows.Err()
}

// LoadGroups returns all groups (used only by seed path at startup).
func LoadGroups(db *sql.DB) ([]GroupData, error) {
	rows, err := db.Query(`SELECT id, name FROM groups ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []GroupData
	for rows.Next() {
		var g GroupData
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}

	for i, g := range groups {
		members, err := membersOf(db, g.ID)
		if err != nil {
			return nil, err
		}
		groups[i].Members = members

		chores, err := GetChores(db, g.ID)
		if err != nil {
			return nil, err
		}
		groups[i].Chores = chores
	}
	return groups, nil
}
