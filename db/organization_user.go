package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// ErrLastOrgAdmin is returned by RemoveOrganizationUser or
// UpdateOrganizationUserRole when the change would leave an organization
// with no admin member at all — mirrors api/users.go's countActiveAdmins
// guard, scoped to one organization instead of the whole app.
var ErrLastOrgAdmin = errors.New("organization must keep at least one admin member")

// validOrganizationUserRoles are the only values organization_users.role may
// take (also enforced by a DB-level CHECK constraint, migration 0081).
// "admin" and "general" (a straight rename of the old "user") can read/write
// everything non-admin-gated; the other four are narrow domain roles —
// write access to their own domain only, read access to everything, same as
// every member already had — enforced in api/router.go and the Create*
// handlers, not here.
var validOrganizationUserRoles = map[string]bool{
	"admin":      true,
	"general":    true,
	"sales":      true,
	"purchasing": true,
	"accounting": true,
	"cashbook":   true,
}

// OrganizationUser mirrors the organization_users table.
type OrganizationUser struct {
	ID             string `db:"id"             json:"id"`
	OrganizationID string `db:"organizationId" json:"organizationId"`
	UserID         string `db:"userId"         json:"userId"`
	Role           string `db:"role"           json:"role"`
	CreatedAt      string `db:"createdAt"      json:"createdAt"`
}

// OrganizationUserWithUser joins in the identifying fields the membership
// management UI needs — email/displayName aren't on organization_users
// itself, only userId.
type OrganizationUserWithUser struct {
	OrganizationUser
	Email       string `db:"email"       json:"email"`
	DisplayName string `db:"displayName" json:"displayName"`
	IsActive    int    `db:"isActive"    json:"isActive"`
}

// GetOrganizationUsers lists an organization's members, joined with user
// identity fields, ordered by when they were added.
func (d *Database) GetOrganizationUsers(organizationID string) ([]OrganizationUserWithUser, error) {
	rows := []OrganizationUserWithUser{}
	err := d.DB.Select(&rows,
		`SELECT ou.id, ou.organizationId, ou.userId, ou.role, ou.createdAt,
		        u.email, u.displayName, u.isActive
		 FROM organization_users ou
		 JOIN users u ON u.id = ou.userId
		 WHERE ou.organizationId = ?
		 ORDER BY ou.createdAt ASC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_organization_users: %w", err)
	}
	return rows, nil
}

// GetUserOrganizations lists every organization a user is a member of — this
// is what GET /api/organizations returns going forward, replacing "every
// organization that exists".
func (d *Database) GetUserOrganizations(userID string) ([]Organization, error) {
	orgs := []Organization{}
	err := d.DB.Select(&orgs,
		`SELECT `+organizationColumns+` FROM organizations
		 WHERE id IN (SELECT organizationId FROM organization_users WHERE userId = ?)
		 ORDER BY name ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_user_organizations: %w", err)
	}
	return orgs, nil
}

// GetOrganizationRole reports whether userID is a member of organizationID
// and, if so, their role. This is the one primitive every authorization
// check (org-member, org-admin) is built on.
func (d *Database) GetOrganizationRole(organizationID, userID string) (role string, isMember bool, err error) {
	err = d.DB.Get(&role,
		`SELECT role FROM organization_users WHERE organizationId = ? AND userId = ?`,
		organizationID, userID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get_organization_role: %w", err)
	}
	return role, true, nil
}

// GetUserOrganizationRoles is the batch counterpart to GetOrganizationRole —
// every organization userID belongs to, in one query, instead of one round
// trip per organization (issue #147: the Organizations list page was calling
// GetOrganizationRole/my-role once per row). An organization absent from the
// map means "not a member" (mirrors GetOrganizationRole's isMember=false),
// same as GetUserOrganizations already means "the caller's own memberships."
func (d *Database) GetUserOrganizationRoles(userID string) (map[string]string, error) {
	rows := []struct {
		OrganizationID string `db:"organizationId"`
		Role           string `db:"role"`
	}{}
	err := d.DB.Select(&rows,
		`SELECT organizationId, role FROM organization_users WHERE userId = ?`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_user_organization_roles: %w", err)
	}
	roles := make(map[string]string, len(rows))
	for _, row := range rows {
		roles[row.OrganizationID] = row.Role
	}
	return roles, nil
}

// AddOrganizationUser grants userID a role on organizationID. Re-adding an
// existing member (e.g. changing their role via re-invite) upserts rather
// than erroring, since the org_user_org_user unique index would otherwise
// turn a well-meaning re-add into a 500 — which means this can demote an
// existing admin exactly like UpdateOrganizationUserRole can, so it needs the
// same last-org-admin guard or re-adding the sole admin at role "user" would
// silently leave the organization with none.
//
// F98: the guard and the write run in one transaction. Reading the admin
// count and then writing as two separate d.DB statements is a TOCTOU — two
// concurrent demotions of an organization's last two admins can each read
// "another admin exists" before either writes, and both commit, leaving zero.
// There is no single-row index that can express this invariant (it's an
// aggregate over the org's members), so the transaction is what makes the
// guard race-free.
func (d *Database) AddOrganizationUser(organizationID, userID, role string) (*OrganizationUser, error) {
	if !validOrganizationUserRoles[role] {
		return nil, newValidationError("role must be one of %q, %q, %q, %q, %q, %q", "admin", "general", "sales", "purchasing", "accounting", "cashbook")
	}
	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("add_organization_user begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if role != "admin" {
		isLast, err := isLastOrgAdminTx(tx, organizationID, userID)
		if err != nil {
			return nil, err
		}
		if isLast {
			return nil, ErrLastOrgAdmin
		}
	}
	id, _ := gonanoid.New()
	if _, err := tx.Exec(
		`INSERT INTO organization_users (id, organizationId, userId, role)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(organizationId, userId) DO UPDATE SET role = excluded.role`,
		id, organizationID, userID, role,
	); err != nil {
		return nil, fmt.Errorf("add_organization_user: %w", err)
	}
	var ou OrganizationUser
	if err := tx.Get(&ou,
		`SELECT * FROM organization_users WHERE organizationId = ? AND userId = ?`,
		organizationID, userID,
	); err != nil {
		return nil, fmt.Errorf("add_organization_user reload: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("add_organization_user commit: %w", err)
	}
	return &ou, nil
}

// UpdateOrganizationUserRole changes an existing member's role, refusing a
// change that would demote the organization's last remaining admin. The
// guard and the UPDATE share one transaction (F98) — see AddOrganizationUser
// for why the read-then-write shape it replaces was racy.
func (d *Database) UpdateOrganizationUserRole(organizationID, userID, role string) error {
	if !validOrganizationUserRoles[role] {
		return newValidationError("role must be one of %q, %q, %q, %q, %q, %q", "admin", "general", "sales", "purchasing", "accounting", "cashbook")
	}
	tx, err := d.DB.Beginx()
	if err != nil {
		return fmt.Errorf("update_organization_user_role begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if role != "admin" {
		isLast, err := isLastOrgAdminTx(tx, organizationID, userID)
		if err != nil {
			return err
		}
		if isLast {
			return ErrLastOrgAdmin
		}
	}
	result, err := tx.Exec(
		`UPDATE organization_users SET role = ? WHERE organizationId = ? AND userId = ?`,
		role, organizationID, userID,
	)
	if err != nil {
		return fmt.Errorf("update_organization_user_role: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("update_organization_user_role commit: %w", err)
	}
	return nil
}

// RemoveOrganizationUser revokes userID's membership in organizationID,
// refusing to remove the organization's last remaining admin. The guard and
// the DELETE share one transaction (F98) — see AddOrganizationUser for why
// the read-then-write shape it replaces was racy.
func (d *Database) RemoveOrganizationUser(organizationID, userID string) error {
	tx, err := d.DB.Beginx()
	if err != nil {
		return fmt.Errorf("remove_organization_user begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	isLast, err := isLastOrgAdminTx(tx, organizationID, userID)
	if err != nil {
		return err
	}
	if isLast {
		return ErrLastOrgAdmin
	}
	result, err := tx.Exec(
		`DELETE FROM organization_users WHERE organizationId = ? AND userId = ?`,
		organizationID, userID,
	)
	if err != nil {
		return fmt.Errorf("remove_organization_user: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("remove_organization_user commit: %w", err)
	}
	return nil
}

// GetOrganizationsWhereSoleAdmin lists every organization where userID is an
// admin member and no other active admin member exists — deleting or
// deactivating this user would leave those organizations with no one able to
// manage membership, close fiscal years, or export GL data (organization_users
// has ON DELETE CASCADE on userId, so a delete doesn't just lock the org out,
// it silently erases the admin's own membership row too). Callers use this
// the same way DeleteVendor uses GetVendorDocumentCount — refuse with a 409
// naming what's blocking, rather than letting the action proceed and
// discovering the orphaned organization later.
func (d *Database) GetOrganizationsWhereSoleAdmin(userID string) ([]Organization, error) {
	orgs := []Organization{}
	err := d.DB.Select(&orgs,
		`SELECT `+organizationColumns+` FROM organizations o
		 WHERE EXISTS (
		     SELECT 1 FROM organization_users ou
		     WHERE ou.organizationId = o.id AND ou.userId = ? AND ou.role = 'admin'
		 )
		 AND NOT EXISTS (
		     SELECT 1 FROM organization_users ou2
		     JOIN users u2 ON u2.id = ou2.userId
		     WHERE ou2.organizationId = o.id AND ou2.userId != ? AND ou2.role = 'admin' AND u2.isActive = 1
		 )
		 ORDER BY o.name ASC`,
		userID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_organizations_where_sole_admin: %w", err)
	}
	return orgs, nil
}

// isLastOrgAdminTx reports whether userID is currently an admin member of
// organizationID and no other active admin member exists — i.e. whether
// removing or demoting them would leave the organization with none. It takes
// the caller's *sqlx.Tx and must be called after Beginx and before Commit:
// this is the whole point of F98 — the same read under the same transaction
// as the write it guards is what closes the concurrent-demotion race. It
// deliberately does NOT touch d.DB (a d.DB read while a tx is open deadlocks
// under db.SetMaxOpenConns(1), it doesn't error).
func isLastOrgAdminTx(tx *sqlx.Tx, organizationID, userID string) (bool, error) {
	var role string
	err := tx.Get(&role,
		`SELECT role FROM organization_users WHERE organizationId = ? AND userId = ?`,
		organizationID, userID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get_organization_role: %w", err)
	}
	if role != "admin" {
		return false, nil
	}
	var otherAdmins int
	err = tx.Get(&otherAdmins,
		`SELECT COUNT(*) FROM organization_users ou
		 JOIN users u ON u.id = ou.userId
		 WHERE ou.organizationId = ? AND ou.userId != ? AND ou.role = 'admin' AND u.isActive = 1`,
		organizationID, userID,
	)
	if err != nil {
		return false, fmt.Errorf("count_other_org_admins: %w", err)
	}
	return otherAdmins == 0, nil
}
