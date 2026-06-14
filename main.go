package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const CSRF_COOKIE_NAME = "X-CSRF-Token"

var csrfSecret = []byte("replace-with-a-long-random-secret-key")

func main() {
	var log = GetDefaultLog()
	SetDefaultLevel(DebugLevel)

	var err error
	connStr := "host=localhost port=5432 user=postgres password=secret dbname=comments sslmode=disable"

	db, err = OpenDB(connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	http.HandleFunc("/", dispatcher)

	log.Infof("Server running on :8001")
	log.Fatal(http.ListenAndServe(":8001", nil))
}

func dispatcher(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		handlePostComment(w, r)
		return
	}
	if r.Method == http.MethodGet {
		handleGetComments(w, r)
		return
	}
	sendError(w, "method not allowed", 405)
}

func randInt() int64 {
	b := make([]byte, 8)
	rand.Read(b)
	return int64(binary.LittleEndian.Uint64(b))
}

func generate_csrf_token(sessionID string) (string, error) {
	payload := fmt.Sprintf("%s:%d:%d",
		sessionID,
		time.Now().Unix(),
		randInt(),
	)

	mac := hmac.New(sha256.New, csrfSecret)
	mac.Write([]byte(payload))
	sig := mac.Sum(nil)

	token := fmt.Sprintf("%s.%s", base64.RawURLEncoding.EncodeToString([]byte(payload)),
		base64.RawURLEncoding.EncodeToString(sig))

	return token, nil
}

func validate_csrf_token(sessionID, token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	payload := string(payloadBytes)

	// recompute HMAC
	mac := hmac.New(sha256.New, csrfSecret)
	mac.Write([]byte(payload))
	expectedSig := mac.Sum(nil)

	// constant-time compare
	if subtle.ConstantTimeCompare(sig, expectedSig) != 1 {
		return false
	}

	// validate session binding
	if !strings.HasPrefix(payload, sessionID+":") {
		return false
	}

	return true
}

func set_csrf_cookie(w http.ResponseWriter, r *http.Request) error {
	log := GetDefaultLog()

	cookie, err := r.Cookie(CSRF_COOKIE_NAME)
	if err != nil || !validate_csrf_token("", cookie.Value) {
		token, err := generate_csrf_token("")
		if err != nil {
			return err
		}
		http.SetCookie(w, &http.Cookie{
			Name:     CSRF_COOKIE_NAME,
			Value:    token,
			HttpOnly: true,
			SameSite: http.SameSiteNoneMode,
			Secure:   false, // should be true in production
		})
		log.Debugf("Set CSRF token '%s'", token)
	}
	return nil
}

func handlePostComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie(CSRF_COOKIE_NAME)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	validate_csrf_token("", cookie.Value)

	var req struct {
		URL      string  `json:"url"`
		Text     string  `json:"text"`
		ParentID *uint64 `json:"parent_id,omitempty"`
	}

	var resp struct {
		Id uint64 `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.URL == "" || req.Text == "" {
		http.Error(w, "missing fields", http.StatusBadRequest)
		return
	}
	user := r.Header.Get("X-Remote-User")
	if user == "" {
		user = "Anonymous"
	}

	comment := Comment{
		URL:       req.URL,
		ParentID:  req.ParentID,
		Username:  user,
		Text:      req.Text, // TODO HTML escape
		CreatedAt: time.Now().UTC(),
	}

	if err := SaveComment(db, &comment); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	resp.Id = comment.ID
	json.NewEncoder(w).Encode(resp)
}

func base64URLDecode(s string) (string, error) {
	// restore padding
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}

	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func parse_resource_ref(path string) ([]string, error) {
	var err error

	parts := strings.Split(strings.Trim(path, "/"), "/")
	length := len(parts)
	if length == 0 {
		return nil, fmt.Errorf("missing site")
	}
	if length > 2 {
		return nil, fmt.Errorf("invalid resource name")
	}

	parts[0], err = base64URLDecode(parts[0])
	if err != nil {
		return nil, err
	}

	if length == 1 {
		parts[1], err = base64URLDecode(parts[1])
		if err != nil {
			return nil, err
		}
	}

	return parts, nil
}

func sendError(w http.ResponseWriter, err string, code int) {
	var log = GetDefaultLog()
	log.Errorf("%s", err)
	http.Error(w, err, http.StatusInternalServerError)
}

func handleGetComments(w http.ResponseWriter, r *http.Request) {
	var log = GetDefaultLog()

	if err := set_csrf_cookie(w, r); err != nil {
		sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	resource, err := parse_resource_ref(r.URL.Path)
	if err != nil {
		sendError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(resource) != 2 {
		sendError(w, "expected absolute resource", http.StatusBadRequest)
		return
	}

	pageURL := strings.Join(resource, "")

	var comments []*Comment
	if comments, err = GetCommentsByURL(db, pageURL); err != nil {
		sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Infof("Found %d comments from '%s'", len(comments), r.URL)

	tree := buildTree(comments)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tree)
}

func buildTree(comments []*Comment) []*Comment {
	m := map[uint64]*Comment{}
	roots := make([]*Comment, 0)

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
