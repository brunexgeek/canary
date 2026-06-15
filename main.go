package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const COOKIE_CSRF = "canary_csrf_Token"
const X_ORIGINAL_URI = "X-Original-Uri"
const X_ORIGINAL_PATH = "X-Original-Path"
const X_REMOTE_USER = "X-Remote-User"

const ENDPOINT_COMMENT = "/comments"
const ENDPOINT_AVATAR = "/avatar"

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

	http.HandleFunc(ENDPOINT_COMMENT, dispatcher)
	http.HandleFunc(ENDPOINT_AVATAR, handleAvatar)

	log.Infof("Server running on :8001")
	log.Fatal(http.ListenAndServe(":8001", nil))
}

func getQueryParameter(r *http.Request, name string, value string) (string, bool) {
	values, ok := r.URL.Query()[name]
	if !ok || len(values) == 0 {
		return value, false
	}
	temp, err := url.QueryUnescape(strings.Join(values, ""))
	if err != nil {
		return value, false
	}
	return temp, true
}

func handleAvatar(w http.ResponseWriter, r *http.Request) {
	username, _ := getQueryParameter(r, "user", "anonymous")

	w.Header().Add("Content-Type", "image/svg+xml")
	w.Write([]byte(GenerateIdenticon(username)))
}

func dispatcher(w http.ResponseWriter, r *http.Request) {
	r.Header[X_ORIGINAL_PATH] = []string{getOriginalPath(r)}
	if r.Method == http.MethodPost {
		handlePostComment(w, r)
		return
	}
	if r.Method == http.MethodGet {
		handleGetComments(w, r)
		return
	}
	sendError(w, "method not allowed", 405)
}

func computeNonce() string {
	b := make([]byte, 8)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func generate_csrf_token(sessionID string) (string, error) {
	payload := fmt.Sprintf("%s:%d:%s",
		sessionID,
		time.Now().Unix(),
		computeNonce(),
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

func getOriginalPath(r *http.Request) string {
	location := "/"
	// TODO use configuration to know if we should trust this header
	if surl, ok := r.Header[X_ORIGINAL_URI]; ok {
		url, err := url.Parse(strings.Join(surl, ""))
		if err == nil {
			pos := strings.LastIndex(url.Path, r.URL.Path)
			if pos >= 0 {
				location = url.Path[:pos] + "/"
			}
		}
	}
	return location
}

func set_csrf_cookie(w http.ResponseWriter, r *http.Request) error {
	log := GetDefaultLog()

	cookie, err := r.Cookie(COOKIE_CSRF)
	if err != nil || !validate_csrf_token("", cookie.Value) {
		token, err := generate_csrf_token("")
		if err != nil {
			return err
		}
		http.SetCookie(w, &http.Cookie{
			Name:     COOKIE_CSRF,
			Value:    token,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   false, // should be true in production
			Path:     r.Header.Get(X_ORIGINAL_PATH),
		})
		log.Debugf("Set CSRF token '%s'", token)
	}
	return nil
}

func handlePostComment(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(COOKIE_CSRF)
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
	req.URL = strings.Split(strings.Split(req.URL, "?")[0], "#")[0]
	// TODO use configuration to know if we should trust this header
	user := r.Header.Get(X_REMOTE_USER)
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

	log := GetDefaultLog()
	log.Infof("Received comment from '%s' for '%s'", user, req.URL)

	w.Header().Set("Content-Type", "application/json")
	resp.Id = comment.ID
	json.NewEncoder(w).Encode(resp)
}

func sendError(w http.ResponseWriter, message string, code int) {
	var log = GetDefaultLog()
	log.Errorf("%s", message)

	output := struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}{
		Message: message,
		Code:    code,
	}
	content, err := json.Marshal(output)
	if err != nil {
		http.Error(w, message, code)
	} else {
		http.Error(w, string(content), code)
	}
}

func handleGetComments(w http.ResponseWriter, r *http.Request) {
	var log = GetDefaultLog()

	if err := set_csrf_cookie(w, r); err != nil {
		sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	pageURL, ok := getQueryParameter(r, "url", "")
	if !ok {
		sendError(w, "expected page URL", http.StatusBadRequest)
		return
	}
	pageURL = strings.Split(strings.Split(pageURL, "?")[0], "#")[0]

	var comments []*Comment
	var err error
	if comments, err = GetCommentsByURL(db, pageURL); err != nil {
		sendError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(comments) == 1 {
		log.Infof("Found %d comment from '%s'", len(comments), pageURL)
	} else {
		log.Infof("Found %d comments from '%s'", len(comments), pageURL)
	}

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
