//go:build mock

package main

import (
	"errors"
	"sync"
	"time"
)

type MemDB struct {
}

var (
	mu       sync.RWMutex
	nextID   uint64 = 1
	comments        = make(map[uint64]*Comment)
	db       *MemDB
)

func OpenDB(connString string) (*MemDB, error) {
	return &MemDB{}, nil
}

func (db *MemDB) Close() {
}

func SaveComment(db interface{}, c *Comment) error {
	mu.Lock()
	defer mu.Unlock()

	if c == nil {
		return errors.New("nil comment")
	}

	if c.ID == 0 {
		c.ID = nextID
		nextID++
	}

	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}

	// store a copy to avoid external mutation
	cp := *c
	comments[c.ID] = &cp

	return nil
}

func GetComment(db interface{}, id uint64) (*Comment, error) {
	mu.RLock()
	defer mu.RUnlock()

	c, ok := comments[id]
	if !ok {
		return nil, errors.New("comment not found")
	}

	// return copy
	cp := *c
	return &cp, nil
}

func GetChildren(db interface{}, parentID uint64) ([]Comment, error) {
	mu.RLock()
	defer mu.RUnlock()

	var result []Comment

	for _, c := range comments {
		if c.ParentID != nil && *c.ParentID == parentID {
			result = append(result, *c)
		}
	}

	return result, nil
}

func GetCommentsByURL(db interface{}, url string) ([]*Comment, error) {
	mu.RLock()
	defer mu.RUnlock()

	var result []*Comment

	for _, c := range comments {
		if c.URL == url && c.ParentID == nil {
			cp := *c
			result = append(result, &cp)
		}
	}

	return result, nil
}
