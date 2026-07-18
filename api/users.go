package api

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
	nanoid "github.com/matoous/go-nanoid/v2"
	"golang.org/x/crypto/bcrypt"
)

// errUserDeactivated is returned by provisionOrSyncUser when an
// admin-deactivated local user attempts to authenticate via SSO — it must
// not be silently re-authorized just because the IdP still accepts them.
var errUserDeactivated = errors.New("user is deactivated")

// validUserRoles are the only values users.role may take (also enforced by a
// DB-level CHECK constraint — validating here just gives a clean 400 instead
// of a raw constraint-violation error).
var validUserRoles = map[string]bool{"user": true, "admin": true}

// minPasswordLength is the only rule enforced on local-login passwords — no
// complexity requirements beyond length.
const minPasswordLength = 8

// countActiveAdmins reports how many users currently hold the admin role and
// are active — used to block an update/delete that would leave the app with
// no admin able to log in.
func (h *handler) countActiveAdmins() (int, error) {
	var count int
	err := h.db.DB.Get(&count, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND isActive = 1`)
	return count, err
}

type userRow struct {
	ID           string `db:"id"           json:"id"`
	Email        string `db:"email"        json:"email"`
	PasswordHash string `db:"passwordHash" json:"-"`
	DisplayName  string `db:"displayName"  json:"displayName"`
	Role         string `db:"role"         json:"role"`
	IsActive     int    `db:"isActive"     json:"isActive"`
	CreatedAt    string `db:"createdAt"    json:"createdAt"`
	LastLoginAt  *int64 `db:"lastLoginAt"  json:"lastLoginAt"`
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
	var rows []userRow
	var err error
	if search != "" {
		like := "%" + search + "%"
		err = h.db.DB.Select(&rows, `SELECT * FROM users WHERE displayName LIKE ? OR email LIKE ? ORDER BY displayName`, like, like)
	} else {
		err = h.db.DB.Select(&rows, `SELECT * FROM users ORDER BY displayName`)
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if rows == nil {
		rows = []userRow{}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) getUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var u userRow
	err := h.db.DB.Get(&u, `SELECT * FROM users WHERE id = ?`, id)
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
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if _, err := mail.ParseAddress(body.Email); err != nil {
		writeError(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	if len(body.Password) < minPasswordLength {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("password must be at least %d characters", minPasswordLength))
		return
	}
	if body.Role == "" {
		body.Role = "user"
	}
	if !validUserRoles[body.Role] {
		writeError(w, http.StatusBadRequest, `role must be "user" or "admin"`)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	id, _ := nanoid.New()
	_, err = h.db.DB.Exec(
		`INSERT INTO users (id, email, passwordHash, displayName, role) VALUES (?, ?, ?, ?, ?)`,
		id, body.Email, string(hash), body.DisplayName, body.Role,
	)
	if err != nil {
		if isDuplicateEmail(err) {
			writeError(w, http.StatusConflict, "email already exists")
			return
		}
		writeInternalError(w, err)
		return
	}
	var u userRow
	if err := h.db.DB.Get(&u, `SELECT * FROM users WHERE id = ?`, id); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, userToJSON(u))
}

// isDuplicateEmail recognizes the raw SQLite unique-index violation on
// users.email (mirrors isDuplicateSKU's pattern in db/product.go).
func isDuplicateEmail(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), "users.email")
}

func (h *handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
		IsActive    *int   `json:"isActive"`
		Password    string `json:"password"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	if body.Role != "" && !validUserRoles[body.Role] {
		writeError(w, http.StatusBadRequest, `role must be "user" or "admin"`)
		return
	}
	if body.Password != "" && len(body.Password) < minPasswordLength {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("password must be at least %d characters", minPasswordLength))
		return
	}

	var current userRow
	if err := h.db.DB.Get(&current, `SELECT * FROM users WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	demoting := body.Role != "" && current.Role == "admin" && body.Role != "admin"
	deactivating := body.IsActive != nil && *body.IsActive == 0 && current.IsActive == 1

	if claims := getClaims(r); claims != nil && claims.UserID == id {
		if demoting {
			writeError(w, http.StatusBadRequest, "cannot demote your own account")
			return
		}
		if deactivating {
			writeError(w, http.StatusBadRequest, "cannot deactivate your own account")
			return
		}
	}
	if (demoting || deactivating) && current.Role == "admin" && current.IsActive == 1 {
		activeAdmins, err := h.countActiveAdmins()
		if err != nil {
			writeInternalError(w, err)
			return
		}
		if activeAdmins <= 1 {
			writeError(w, http.StatusBadRequest, "cannot remove the last active admin")
			return
		}
	}

	if body.IsActive != nil {
		if _, err := h.db.DB.Exec(`UPDATE users SET isActive = ? WHERE id = ?`, *body.IsActive, id); err != nil {
			writeInternalError(w, err)
			return
		}
	}
	if body.DisplayName != "" {
		if _, err := h.db.DB.Exec(`UPDATE users SET displayName = ? WHERE id = ?`, body.DisplayName, id); err != nil {
			writeInternalError(w, err)
			return
		}
	}
	if body.Role != "" {
		if _, err := h.db.DB.Exec(`UPDATE users SET role = ? WHERE id = ?`, body.Role, id); err != nil {
			writeInternalError(w, err)
			return
		}
	}
	if body.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		if _, err := h.db.DB.Exec(`UPDATE users SET passwordHash = ? WHERE id = ?`, string(hash), id); err != nil {
			writeInternalError(w, err)
			return
		}
	}

	var u userRow
	if err := h.db.DB.Get(&u, `SELECT * FROM users WHERE id = ?`, id); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userToJSON(u))
}

func (h *handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	claims := getClaims(r)
	if claims != nil && claims.UserID == id {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	var target userRow
	if err := h.db.DB.Get(&target, `SELECT * FROM users WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if target.Role == "admin" && target.IsActive == 1 {
		activeAdmins, err := h.countActiveAdmins()
		if err != nil {
			writeInternalError(w, err)
			return
		}
		if activeAdmins <= 1 {
			writeError(w, http.StatusBadRequest, "cannot delete the last active admin")
			return
		}
	}

	if _, err := h.db.DB.Exec(`DELETE FROM users WHERE id = ?`, id); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// provisionOrSyncUser looks up a user by email — the identity anchor for SSO
// logins — JIT-provisioning one on first login and re-syncing its role (never
// displayName) on every subsequent login so group changes at the identity
// provider take effect without waiting for an admin to edit the account here.
func (h *handler) provisionOrSyncUser(email, name string, isAdmin bool) (userRow, error) {
	email = strings.TrimSpace(email)
	role := "user"
	if isAdmin {
		role = "admin"
	}

	h.dbMu.Lock()
	defer h.dbMu.Unlock()

	var u userRow
	err := h.db.DB.Get(&u, `SELECT * FROM users WHERE email = ?`, email)
	if err != nil {
		if name == "" {
			name = email
		}
		// A random, never-reused password satisfies passwordHash's NOT NULL
		// constraint without producing a usable local-login credential.
		randomPassword := make([]byte, 32)
		if _, rerr := rand.Read(randomPassword); rerr != nil {
			return userRow{}, rerr
		}
		hash, herr := bcrypt.GenerateFromPassword(randomPassword, bcrypt.DefaultCost)
		if herr != nil {
			return userRow{}, herr
		}
		id, _ := nanoid.New()
		// ON CONFLICT DO NOTHING handles two near-simultaneous first logins
		// for the same new SSO email; the SELECT below picks up whichever
		// row won.
		if _, err = h.db.DB.Exec(
			`INSERT INTO users (id, email, passwordHash, displayName, role, createdAt)
			 VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(email) DO NOTHING`,
			id, email, string(hash), name, role, time.Now().Format("2006-01-02 15:04:05"),
		); err != nil {
			return userRow{}, err
		}
		if err := h.db.DB.Get(&u, `SELECT * FROM users WHERE email = ?`, email); err != nil {
			return userRow{}, err
		}
	}

	if u.IsActive == 0 {
		return userRow{}, errUserDeactivated
	}

	if u.Role != role {
		demoting := u.Role == "admin" && role != "admin"
		blocked := false
		if demoting {
			activeAdmins, err := h.countActiveAdmins()
			if err != nil {
				return userRow{}, err
			}
			blocked = activeAdmins <= 1
		}
		if blocked {
			log.Printf("provisionOrSyncUser: refusing to demote last active admin %s via SSO role sync", email)
		} else {
			if _, err := h.db.DB.Exec(`UPDATE users SET role = ? WHERE id = ?`, role, u.ID); err != nil {
				return userRow{}, err
			}
			u.Role = role
		}
	}

	h.db.DB.Exec(`UPDATE users SET lastLoginAt = ? WHERE id = ?`, time.Now().UnixMilli(), u.ID)

	return u, nil
}

// EnsureFirstAdmin creates an admin user if no users exist yet.
func EnsureFirstAdmin(database *db.Database, email, password string) {
	var count int
	if err := database.DB.Get(&count, `SELECT COUNT(*) FROM users`); err != nil {
		log.Printf("EnsureFirstAdmin: failed to count existing users: %v", err)
		return
	}
	if count > 0 {
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("EnsureFirstAdmin: failed to hash password: %v", err)
		return
	}
	id, _ := nanoid.New()
	if _, err := database.DB.Exec(
		`INSERT INTO users (id, email, passwordHash, displayName, role, createdAt) VALUES (?, ?, ?, 'Administrator', 'admin', ?)`,
		id, email, string(hash), time.Now().Format("2006-01-02 15:04:05"),
	); err != nil {
		log.Printf("EnsureFirstAdmin: failed to create initial admin user: %v", err)
	}
}
