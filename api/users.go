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

// countActiveAdmins reports how many users are currently active platform
// admins — used to block an update/delete that would leave the app with no
// platform admin able to manage users/backups/restore. isPlatformAdmin is
// the authoritative signal (not the legacy role column — see migration
// 0069), since that's what api/middleware.go's platformAdmin check actually
// gates on.
// checkNotSoleOrgAdmin refuses (409) a delete/deactivate that would strip an
// organization of its only admin — organization_users cascades on user
// delete, so deleting the sole admin doesn't just lock the org out, it erases
// the membership row too, and there is no platform-admin bypass to recover
// it (orgAdmin is per-organization by design — see api/middleware.go).
// Deactivating has the same effect in practice: authMiddleware rejects a
// deactivated user's requests, so an "admin" who can't log in isn't one.
func (h *handler) checkNotSoleOrgAdmin(w http.ResponseWriter, userID string) bool {
	orgs, err := h.db.GetOrganizationsWhereSoleAdmin(userID)
	if err != nil {
		writeInternalError(w, err)
		return false
	}
	if len(orgs) == 0 {
		return true
	}
	names := make([]string, len(orgs))
	for i, o := range orgs {
		name := o.Name
		if name == nil || *name == "" {
			names[i] = o.ID
		} else {
			names[i] = *name
		}
	}
	writeError(w, http.StatusConflict, fmt.Sprintf(
		"cannot proceed: this user is the sole admin of %s — assign another admin there first",
		strings.Join(names, ", "),
	))
	return false
}

func (h *handler) countActiveAdmins() (int, error) {
	var count int
	err := h.db.DB.Get(&count, `SELECT COUNT(*) FROM users WHERE isPlatformAdmin = 1 AND isActive = 1`)
	return count, err
}

type userRow struct {
	ID              string `db:"id"              json:"id"`
	Email           string `db:"email"           json:"email"`
	PasswordHash    string `db:"passwordHash"    json:"-"`
	DisplayName     string `db:"displayName"     json:"displayName"`
	Role            string `db:"role"            json:"role"`
	IsPlatformAdmin int    `db:"isPlatformAdmin" json:"isPlatformAdmin"`
	IsActive        int    `db:"isActive"        json:"isActive"`
	CreatedAt       string `db:"createdAt"       json:"createdAt"`
	LastLoginAt     *int64 `db:"lastLoginAt"     json:"lastLoginAt"`
}

// userColumns is an explicit column list for every users query in this file,
// used in place of SELECT * so a sensitive column added by a future
// migration isn't silently loaded into every user lookup by default —
// PasswordHash itself is already safe (json:"-"), this just keeps that true
// on purpose rather than by accident.
const userColumns = `id, email, passwordHash, displayName, role, isPlatformAdmin, isActive, createdAt, lastLoginAt`

func userToJSON(u userRow) map[string]any {
	return map[string]any{
		"id":              u.ID,
		"email":           u.Email,
		"displayName":     u.DisplayName,
		"role":            u.Role,
		"isPlatformAdmin": u.IsPlatformAdmin,
		"isActive":        u.IsActive,
		"createdAt":       u.CreatedAt,
		"lastLoginAt":     u.LastLoginAt,
	}
}

func (h *handler) listUsers(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	var rows []userRow
	var err error
	if search != "" {
		like := "%" + search + "%"
		err = h.db.DB.Select(&rows, `SELECT `+userColumns+` FROM users WHERE displayName LIKE ? OR email LIKE ? ORDER BY displayName`, like, like)
	} else {
		err = h.db.DB.Select(&rows, `SELECT `+userColumns+` FROM users ORDER BY displayName`)
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
	err := h.db.DB.Get(&u, `SELECT `+userColumns+` FROM users WHERE id = ?`, id)
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
	// role and isPlatformAdmin are kept in lockstep here — the "Users" admin
	// page's role toggle is still exactly "can this person do the global
	// admin things"; per-organization roles are a separate, additive
	// concept managed through the organization membership UI instead.
	isPlatformAdmin := 0
	if body.Role == "admin" {
		isPlatformAdmin = 1
	}
	_, err = h.db.DB.Exec(
		`INSERT INTO users (id, email, passwordHash, displayName, role, isPlatformAdmin) VALUES (?, ?, ?, ?, ?, ?)`,
		id, body.Email, string(hash), body.DisplayName, body.Role, isPlatformAdmin,
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
	if err := h.db.DB.Get(&u, `SELECT `+userColumns+` FROM users WHERE id = ?`, id); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, userToJSON(u))
}

// isDuplicateEmail recognizes the raw SQLite unique-index violation on
// users.email (mirrors isDuplicateSKU's pattern in db/product.go). This
// depends on SQLite's own error message text, not a stable API contract —
// accepted deliberately (consistent with isDuplicateSKU) rather than
// replaced with a pre-check SELECT, which would just move the race from "an
// unstable string match" to "a TOCTOU gap between the check and the insert"
// without actually removing the string match, since the DB-level catch has
// to remain the authoritative guard either way.
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
	if err := h.db.DB.Get(&current, `SELECT `+userColumns+` FROM users WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	demoting := body.Role != "" && current.IsPlatformAdmin == 1 && body.Role != "admin"
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
	if (demoting || deactivating) && current.IsPlatformAdmin == 1 && current.IsActive == 1 {
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
	if deactivating && !h.checkNotSoleOrgAdmin(w, id) {
		return
	}

	// Hashing runs before the transaction opens, mirroring the F59 rationale
	// in provisionOrSyncUser: it's ~50-100ms of pure CPU that doesn't need a
	// held connection, and db.SetMaxOpenConns(1) means holding one during it
	// would stall every other request in the app for no reason.
	var passwordHash string
	if body.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		passwordHash = string(hash)
	}

	// The four updates below used to run as independent statements — a
	// failure partway through (e.g. on the role update) left the user
	// partially changed, such as a new password persisted but the intended
	// role change silently dropped. One transaction makes them atomic.
	tx, err := h.db.DB.Beginx()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if body.IsActive != nil {
		if _, err := tx.Exec(`UPDATE users SET isActive = ? WHERE id = ?`, *body.IsActive, id); err != nil {
			writeInternalError(w, err)
			return
		}
	}
	if body.DisplayName != "" {
		if _, err := tx.Exec(`UPDATE users SET displayName = ? WHERE id = ?`, body.DisplayName, id); err != nil {
			writeInternalError(w, err)
			return
		}
	}
	if body.Role != "" {
		isPlatformAdmin := 0
		if body.Role == "admin" {
			isPlatformAdmin = 1
		}
		if _, err := tx.Exec(`UPDATE users SET role = ?, isPlatformAdmin = ? WHERE id = ?`, body.Role, isPlatformAdmin, id); err != nil {
			writeInternalError(w, err)
			return
		}
	}
	if passwordHash != "" {
		if _, err := tx.Exec(`UPDATE users SET passwordHash = ? WHERE id = ?`, passwordHash, id); err != nil {
			writeInternalError(w, err)
			return
		}
	}

	var u userRow
	if err := tx.Get(&u, `SELECT `+userColumns+` FROM users WHERE id = ?`, id); err != nil {
		writeInternalError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
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
	if err := h.db.DB.Get(&target, `SELECT `+userColumns+` FROM users WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if target.IsPlatformAdmin == 1 && target.IsActive == 1 {
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
	if !h.checkNotSoleOrgAdmin(w, id) {
		return
	}

	if _, err := h.db.DB.Exec(`DELETE FROM users WHERE id = ?`, id); err != nil {
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// hashRandomPassword generates a bcrypt hash of a random, never-reused
// password — satisfies passwordHash's NOT NULL constraint for a
// JIT-provisioned SSO user without producing a usable local-login
// credential. Touches no database state, only crypto/rand and CPU — safe to
// call without holding dbMu.
func hashRandomPassword() ([]byte, error) {
	randomPassword := make([]byte, 32)
	if _, err := rand.Read(randomPassword); err != nil {
		return nil, err
	}
	return bcrypt.GenerateFromPassword(randomPassword, bcrypt.DefaultCost)
}

// provisionOrSyncUser looks up a user by email — the identity anchor for SSO
// logins — JIT-provisioning one on first login and re-syncing its role (never
// displayName) on every subsequent login so group changes at the identity
// provider take effect without waiting for an admin to edit the account here.
func (h *handler) provisionOrSyncUser(email, name string, isAdmin bool) (userRow, error) {
	email = strings.TrimSpace(email)
	if name == "" {
		name = email
	}
	role := "user"
	wantPlatformAdmin := 0
	if isAdmin {
		role = "admin"
		wantPlatformAdmin = 1
	}

	// F59 (2026-08-13 audit): bcrypt.GenerateFromPassword (~50-100ms at
	// DefaultCost) doesn't touch the database, but used to run inside
	// dbMu's write lock below whenever this was a first login — and
	// because dbMu is the global RWMutex every withDB-wrapped handler
	// RLocks for its whole request, that stalled all other API traffic for
	// the duration. Decide whether hashing is even needed with a quick
	// RLock'd pre-check, then hash with no lock held at all. The write
	// section below re-checks under the write lock regardless (same as
	// before this fix), so a user created concurrently between the
	// pre-check and the write lock is still handled correctly — just via
	// the fallback hash below, the same rare-race cost this function
	// always had.
	var hash []byte
	h.dbMu.RLock()
	var precheck userRow
	existsErr := h.db.DB.Get(&precheck, `SELECT `+userColumns+` FROM users WHERE email = ?`, email)
	h.dbMu.RUnlock()
	if existsErr != nil {
		var herr error
		hash, herr = hashRandomPassword()
		if herr != nil {
			return userRow{}, herr
		}
	}

	h.dbMu.Lock()
	defer h.dbMu.Unlock()

	var u userRow
	err := h.db.DB.Get(&u, `SELECT `+userColumns+` FROM users WHERE email = ?`, email)
	if err != nil {
		if hash == nil {
			var herr error
			hash, herr = hashRandomPassword()
			if herr != nil {
				return userRow{}, herr
			}
		}
		id, _ := nanoid.New()
		// ON CONFLICT DO NOTHING handles two near-simultaneous first logins
		// for the same new SSO email; the SELECT below picks up whichever
		// row won.
		if _, err = h.db.DB.Exec(
			`INSERT INTO users (id, email, passwordHash, displayName, role, isPlatformAdmin, createdAt)
			 VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(email) DO NOTHING`,
			id, email, string(hash), name, role, wantPlatformAdmin, time.Now().Format("2006-01-02 15:04:05"),
		); err != nil {
			return userRow{}, err
		}
		if err := h.db.DB.Get(&u, `SELECT `+userColumns+` FROM users WHERE email = ?`, email); err != nil {
			return userRow{}, err
		}
	}

	if u.IsActive == 0 {
		return userRow{}, errUserDeactivated
	}

	if u.Role != role || u.IsPlatformAdmin != wantPlatformAdmin {
		demoting := u.IsPlatformAdmin == 1 && wantPlatformAdmin == 0
		blocked := false
		if demoting {
			activeAdmins, err := h.countActiveAdmins()
			if err != nil {
				return userRow{}, err
			}
			blocked = activeAdmins <= 1
		}
		if blocked {
			log.Printf("provisionOrSyncUser: refusing to demote last active platform admin %s via SSO role sync", email)
		} else {
			if _, err := h.db.DB.Exec(`UPDATE users SET role = ?, isPlatformAdmin = ? WHERE id = ?`, role, wantPlatformAdmin, u.ID); err != nil {
				return userRow{}, err
			}
			u.Role = role
			u.IsPlatformAdmin = wantPlatformAdmin
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
