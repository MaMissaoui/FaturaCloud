package api

import (
	"net/http"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
	nanoid "github.com/matoous/go-nanoid/v2"
	"golang.org/x/crypto/bcrypt"
)

type userRow struct {
	ID           string  `db:"id"           json:"id"`
	Email        string  `db:"email"        json:"email"`
	PasswordHash string  `db:"passwordHash" json:"-"`
	DisplayName  string  `db:"displayName"  json:"displayName"`
	Role         string  `db:"role"         json:"role"`
	IsActive     int     `db:"isActive"     json:"isActive"`
	CreatedAt    string  `db:"createdAt"    json:"createdAt"`
	LastLoginAt  *int64  `db:"lastLoginAt"  json:"lastLoginAt"`
}

func userToJSON(u userRow) map[string]any {
	return map[string]any{
		"id":          u.ID,
		"email":       u.Email,
		"displayName": u.DisplayName,
		"role":        u.Role,
		"isActive":    u.IsActive,
		"createdAt":   u.CreatedAt,
		"lastLoginAt": u.LastLoginAt,
	}
}

func (h *handler) listUsers(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	h.dbMu.RLock()
	var rows []userRow
	if search != "" {
		like := "%" + search + "%"
		h.db.DB.Select(&rows, `SELECT * FROM users WHERE displayName LIKE ? OR email LIKE ? ORDER BY displayName`, like, like)
	} else {
		h.db.DB.Select(&rows, `SELECT * FROM users ORDER BY displayName`)
	}
	h.dbMu.RUnlock()
	if rows == nil {
		rows = []userRow{}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) getUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.dbMu.RLock()
	var u userRow
	err := h.db.DB.Get(&u, `SELECT * FROM users WHERE id = ?`, id)
	h.dbMu.RUnlock()
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, userToJSON(u))
}

func (h *handler) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if body.Email == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	id, _ := nanoid.New()
	if body.Role == "" {
		body.Role = "user"
	}
	h.dbMu.RLock()
	_, err = h.db.DB.Exec(
		`INSERT INTO users (id, email, passwordHash, displayName, role) VALUES (?, ?, ?, ?, ?)`,
		id, body.Email, string(hash), body.DisplayName, body.Role,
	)
	h.dbMu.RUnlock()
	if err != nil {
		writeError(w, http.StatusConflict, "email already exists")
		return
	}
	h.dbMu.RLock()
	var u userRow
	h.db.DB.Get(&u, `SELECT * FROM users WHERE id = ?`, id)
	h.dbMu.RUnlock()
	writeJSON(w, http.StatusCreated, userToJSON(u))
}

func (h *handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
		IsActive    *int   `json:"isActive"`
		Password    string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	h.dbMu.RLock()
	if body.IsActive != nil {
		h.db.DB.Exec(`UPDATE users SET isActive = ? WHERE id = ?`, *body.IsActive, id)
	}
	if body.DisplayName != "" {
		h.db.DB.Exec(`UPDATE users SET displayName = ?, role = ? WHERE id = ?`, body.DisplayName, body.Role, id)
	}
	if body.Password != "" {
		hash, _ := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		h.db.DB.Exec(`UPDATE users SET passwordHash = ? WHERE id = ?`, string(hash), id)
	}
	var u userRow
	h.db.DB.Get(&u, `SELECT * FROM users WHERE id = ?`, id)
	h.dbMu.RUnlock()
	writeJSON(w, http.StatusOK, userToJSON(u))
}

func (h *handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Prevent deleting yourself
	claims := getClaims(r)
	if claims != nil && claims.UserID == id {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}
	h.dbMu.RLock()
	h.db.DB.Exec(`DELETE FROM users WHERE id = ?`, id)
	h.dbMu.RUnlock()
	w.WriteHeader(http.StatusNoContent)
}

// EnsureFirstAdmin creates an admin user if no users exist yet.
func EnsureFirstAdmin(database *db.Database, email, password string) {
	var count int
	database.DB.Get(&count, `SELECT COUNT(*) FROM users`)
	if count > 0 {
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return
	}
	id, _ := nanoid.New()
	database.DB.Exec(
		`INSERT INTO users (id, email, passwordHash, displayName, role, createdAt) VALUES (?, ?, ?, 'Administrator', 'admin', ?)`,
		id, email, string(hash), time.Now().Format("2006-01-02 15:04:05"),
	)
}
