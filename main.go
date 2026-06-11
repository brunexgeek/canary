package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var db *bolt.DB
var idMutex sync.Mutex
var idCounter uint64 = 1

type Comment struct {
	ID        uint64     `json:"id"`
	URL       string     `json:"url"`
	ParentID  *uint64    `json:"parent_id,omitempty"`
	Username  string     `json:"username"`
	Text      string     `json:"text"`
	CreatedAt time.Time  `json:"created_at"`
	Replies   []*Comment `json:"replies,omitempty"`
}

func main() {
	var err error
	db, err = bolt.Open("comments.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	http.HandleFunc("/comment", handlePostComment)
	http.HandleFunc("/comments", handleGetComments)

	log.Println("Server running on :8001")
	log.Fatal(http.ListenAndServe(":8001", nil))
}

func nextID() uint64 {
	idMutex.Lock()
	defer idMutex.Unlock()
	idCounter++
	return idCounter
}

func bucketName(pageURL string) []byte {
	return []byte(url.QueryEscape(pageURL))
}

func handlePostComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL      string  `json:"url"`
		Username string  `json:"username"`
		Text     string  `json:"text"`
		ParentID *uint64 `json:"parent_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.URL == "" || req.Username == "" || req.Text == "" {
		http.Error(w, "missing fields", http.StatusBadRequest)
		return
	}

	comment := Comment{
		ID:        nextID(),
		URL:       req.URL,
		ParentID:  req.ParentID,
		Username:  req.Username,
		Text:      req.Text,
		CreatedAt: time.Now().UTC(),
	}

	err := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucketName(req.URL))
		if err != nil {
			return err
		}

		data, err := json.Marshal(comment)
		if err != nil {
			return err
		}

		return b.Put([]byte(strconv.FormatUint(comment.ID, 10)), data)
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comment)
}

func handleGetComments(w http.ResponseWriter, r *http.Request) {
	pageURL := r.URL.Query().Get("url")
	if pageURL == "" {
		http.Error(w, "missing url", http.StatusBadRequest)
		return
	}

	var comments []*Comment

	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName(pageURL))
		if b == nil {
			return nil
		}

		return b.ForEach(func(_, v []byte) error {
			var c Comment
			if err := json.Unmarshal(v, &c); err != nil {
				return err
			}
			comments = append(comments, &c)
			return nil
		})
	})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tree := buildTree(comments)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tree)
}

func buildTree(comments []*Comment) []*Comment {
	m := map[uint64]*Comment{}
	var roots []*Comment

	for _, c := range comments {
		m[c.ID] = c
		c.Replies = []*Comment{}
	}

	for _, c := range comments {
		if c.ParentID != nil {
			parent := m[*c.ParentID]
			if parent != nil {
				parent.Replies = append(parent.Replies, c)
				continue
			}
		}
		roots = append(roots, c)
	}

	return roots
}
