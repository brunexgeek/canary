package main

import (
	"database/sql"
	"time"

	_ "github.com/lib/pq"
)

type Comment struct {
	ID        uint64     `json:"id"`
	URL       string     `json:"url,omitempty"`
	ParentID  *uint64    `json:"parent_id,omitempty"`
	Username  string     `json:"username"`
	Text      string     `json:"text"`
	CreatedAt time.Time  `json:"created_at"`
	Replies   []*Comment `json:"replies,omitempty"`
}

func OpenDB(connString string) (*sql.DB, error) {
	db, err := sql.Open("postgres", connString)
	if err != nil {
		return nil, err
	}

	schema := `
CREATE TABLE IF NOT EXISTS comments (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    url TEXT NOT NULL,
    parent_id BIGINT NULL REFERENCES comments(id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comments_parent_id
    ON comments(parent_id);

CREATE INDEX IF NOT EXISTS idx_comments_url
    ON comments(url);

CREATE INDEX IF NOT EXISTS idx_comments_url_created_at
    ON comments(url, created_at);
`

	_, err = db.Exec(schema)
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func SaveComment(db *sql.DB, c *Comment) error {
	const q = `
INSERT INTO comments (
    url,
    parent_id,
    username,
    text,
    created_at
)
VALUES ($1, $2, $3, $4, $5)
RETURNING id
`

	var parent any
	if c.ParentID != nil {
		parent = int64(*c.ParentID)
	}

	var id int64

	err := db.QueryRow(
		q,
		c.URL,
		parent,
		c.Username,
		c.Text,
		c.CreatedAt.UTC(),
	).Scan(&id)

	if err != nil {
		return err
	}

	c.ID = uint64(id)

	return nil
}

func GetComment(db *sql.DB, id uint64) (*Comment, error) {
	const q = `
SELECT
    id,
    url,
    parent_id,
    username,
    text,
    created_at
FROM comments
WHERE id = $1
`

	var c Comment
	var dbID int64
	var parent sql.NullInt64

	err := db.QueryRow(q, int64(id)).Scan(
		&dbID,
		&c.URL,
		&parent,
		&c.Username,
		&c.Text,
		&c.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	c.ID = uint64(dbID)

	if parent.Valid {
		v := uint64(parent.Int64)
		c.ParentID = &v
	}

	return &c, nil
}

func GetChildren(db *sql.DB, parentID uint64) ([]Comment, error) {
	const q = `
SELECT
    id,
    url,
    parent_id,
    username,
    text,
    created_at
FROM comments
WHERE parent_id = $1
ORDER BY created_at
`

	rows, err := db.Query(q, int64(parentID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment

	for rows.Next() {
		var c Comment
		var id int64
		var parent sql.NullInt64

		err := rows.Scan(
			&id,
			&c.URL,
			&parent,
			&c.Username,
			&c.Text,
			&c.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		c.ID = uint64(id)

		if parent.Valid {
			v := uint64(parent.Int64)
			c.ParentID = &v
		}

		comments = append(comments, c)
	}

	return comments, rows.Err()
}

func GetCommentsByURL(db *sql.DB, url string) ([]*Comment, error) {
	const q = `
SELECT
    id,
    parent_id,
    username,
    text,
    created_at
FROM comments
WHERE url = $1
ORDER BY created_at
`

	rows, err := db.Query(q, url)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []*Comment

	for rows.Next() {
		var c Comment
		var id int64
		var parent sql.NullInt64

		err := rows.Scan(
			&id,
			&parent,
			&c.Username,
			&c.Text,
			&c.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		c.ID = uint64(id)

		if parent.Valid {
			v := uint64(parent.Int64)
			c.ParentID = &v
		}

		comments = append(comments, &c)
	}

	return comments, rows.Err()
}
