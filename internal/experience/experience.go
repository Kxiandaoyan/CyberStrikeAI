// Package experience — model layer for experience drafts.
package experience

import (
	"database/sql"
	"time"
)

type Draft struct {
	ID         int64      `json:"id"`
	ProjectID  string     `json:"project_id"`
	Source     string     `json:"source"`
	SourceType string     `json:"source_type"`
	Title      string     `json:"title"`
	Content    string     `json:"content"`
	Category   string     `json:"category"`
	FactKeys   string     `json:"fact_keys"`
	Status     string     `json:"status"`
	Reviewer   string     `json:"reviewer"`
	ReviewedAt *time.Time `json:"reviewed_at"`
	AppliedTo  string     `json:"applied_to"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func EnsureSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS experience_drafts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			source_type TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT 'methodology',
			fact_keys TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'draft',
			reviewer TEXT NOT NULL DEFAULT '',
			reviewed_at DATETIME,
			applied_to TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_experience_status ON experience_drafts(status)`,
		`CREATE INDEX IF NOT EXISTS idx_experience_project ON experience_drafts(project_id)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func CreateDraft(db *sql.DB, d *Draft) (int64, error) {
	res, err := db.Exec(`INSERT INTO experience_drafts
		(project_id, source, source_type, title, content, category, fact_keys, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ProjectID, d.Source, d.SourceType, d.Title, d.Content, d.Category, d.FactKeys, "draft")
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

const draftCols = `id, project_id, source, source_type, title, content, category, fact_keys, status, reviewer, reviewed_at, applied_to, created_at, updated_at`

func ListAll(db *sql.DB) ([]Draft, error) {
	return scanDrafts(db.Query(`SELECT ` + draftCols + ` FROM experience_drafts ORDER BY created_at DESC LIMIT 100`))
}

func ListByStatus(db *sql.DB, status string) ([]Draft, error) {
	return scanDrafts(db.Query(`SELECT ` + draftCols + ` FROM experience_drafts WHERE status = ? ORDER BY created_at DESC LIMIT 100`, status))
}

func scanDrafts(rows *sql.Rows, err error) ([]Draft, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Draft
	for rows.Next() {
		var d Draft
		var ra sql.NullTime
		if e := rows.Scan(&d.ID, &d.ProjectID, &d.Source, &d.SourceType, &d.Title,
			&d.Content, &d.Category, &d.FactKeys, &d.Status, &d.Reviewer,
			&ra, &d.AppliedTo, &d.CreatedAt, &d.UpdatedAt); e != nil {
			continue
		}
		if ra.Valid {
			t := ra.Time
			d.ReviewedAt = &t
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

func GetDraft(db *sql.DB, id int64) (*Draft, error) {
	var d Draft
	var ra sql.NullTime
	err := db.QueryRow(`SELECT ` + draftCols + ` FROM experience_drafts WHERE id = ?`, id).
		Scan(&d.ID, &d.ProjectID, &d.Source, &d.SourceType, &d.Title,
			&d.Content, &d.Category, &d.FactKeys, &d.Status, &d.Reviewer,
			&ra, &d.AppliedTo, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if ra.Valid {
		t := ra.Time
		d.ReviewedAt = &t
	}
	return &d, nil
}

func ApproveDraft(db *sql.DB, id int64, reviewer, appliedTo string) error {
	_, err := db.Exec(`UPDATE experience_drafts SET status = ?, reviewer = ?,
		reviewed_at = datetime('now'), applied_to = ?, updated_at = datetime('now')
		WHERE id = ? AND status = ?`, "approved", reviewer, appliedTo, id, "draft")
	return err
}

func RejectDraft(db *sql.DB, id int64, reviewer string) error {
	_, err := db.Exec(`UPDATE experience_drafts SET status = ?, reviewer = ?,
		reviewed_at = datetime('now'), updated_at = datetime('now')
		WHERE id = ? AND status = ?`, "rejected", reviewer, id, "draft")
	return err
}

func CountByStatus(db *sql.DB) (map[string]int, error) {
	rows, err := db.Query(`SELECT status, COUNT(*) FROM experience_drafts GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]int)
	for rows.Next() {
		var s string
		var n int
		if rows.Scan(&s, &n) == nil {
			m[s] = n
		}
	}
	return m, nil
}