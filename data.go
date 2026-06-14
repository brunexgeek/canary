package main

import (
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
