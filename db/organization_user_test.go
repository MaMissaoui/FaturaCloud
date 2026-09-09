package db

import (
	"errors"
	"testing"
)

// seedTestUser inserts a minimal users row directly — the db package has no
// CreateUser (user CRUD lives in api/users.go), so organization_users tests
// that need a userId to satisfy the FK seed one by hand.
func seedTestUser(t *testing.T, d *Database, id, role string, isPlatformAdmin int) {
	t.Helper()
	_, err := d.DB.Exec(
		`INSERT INTO users (id, email, passwordHash, displayName, role, isActive, isPlatformAdmin)
		 VALUES (?, ?, 'x', ?, ?, 1, ?)`,
		id, id+"@test.local", id, role, isPlatformAdmin,
	)
	if err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func TestOrganizationUserCRUD(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	seedTestUser(t, d, "user-1", "user", 0)

	if _, isMember, err := d.GetOrganizationRole(org.ID, "user-1"); err != nil || isMember {
		t.Fatalf("expected no membership yet, got isMember=%v err=%v", isMember, err)
	}

	ou, err := d.AddOrganizationUser(org.ID, "user-1", "user")
	if err != nil {
		t.Fatalf("AddOrganizationUser: %v", err)
	}
	if ou.Role != "user" || ou.OrganizationID != org.ID || ou.UserID != "user-1" {
		t.Fatalf("unexpected membership row: %+v", ou)
	}

	role, isMember, err := d.GetOrganizationRole(org.ID, "user-1")
	if err != nil {
		t.Fatalf("GetOrganizationRole: %v", err)
	}
	if !isMember || role != "user" {
		t.Fatalf("expected member with role=user, got isMember=%v role=%q", isMember, role)
	}

	if err := d.UpdateOrganizationUserRole(org.ID, "user-1", "admin"); err != nil {
		t.Fatalf("UpdateOrganizationUserRole: %v", err)
	}
	role, _, err = d.GetOrganizationRole(org.ID, "user-1")
	if err != nil {
		t.Fatalf("GetOrganizationRole after promote: %v", err)
	}
	if role != "admin" {
		t.Fatalf("expected role=admin after update, got %q", role)
	}

	members, err := d.GetOrganizationUsers(org.ID)
	if err != nil {
		t.Fatalf("GetOrganizationUsers: %v", err)
	}
	if len(members) != 1 || members[0].UserID != "user-1" || members[0].Email != "user-1@test.local" {
		t.Fatalf("unexpected members list: %+v", members)
	}

	orgs, err := d.GetUserOrganizations("user-1")
	if err != nil {
		t.Fatalf("GetUserOrganizations: %v", err)
	}
	if len(orgs) != 1 || orgs[0].ID != org.ID {
		t.Fatalf("expected user-1 to see exactly org-1, got %+v", orgs)
	}

	// user-1 is the org's sole admin at this point, so removing them outright
	// is correctly blocked by the same last-admin guard (covered by
	// TestRemoveLastOrgAdminBlocked) — add a second admin first so removal
	// has somewhere safe to land.
	seedTestUser(t, d, "user-2", "user", 0)
	if _, err := d.AddOrganizationUser(org.ID, "user-2", "admin"); err != nil {
		t.Fatalf("seed second admin: %v", err)
	}
	if err := d.RemoveOrganizationUser(org.ID, "user-1"); err != nil {
		t.Fatalf("RemoveOrganizationUser: %v", err)
	}
	if _, isMember, err := d.GetOrganizationRole(org.ID, "user-1"); err != nil || isMember {
		t.Fatalf("expected membership gone, got isMember=%v err=%v", isMember, err)
	}
}

func TestAddOrganizationUser_UpsertsRoleOnReAdd(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	seedTestUser(t, d, "user-1", "user", 0)

	if _, err := d.AddOrganizationUser(org.ID, "user-1", "user"); err != nil {
		t.Fatalf("first add: %v", err)
	}
	ou, err := d.AddOrganizationUser(org.ID, "user-1", "admin")
	if err != nil {
		t.Fatalf("re-add with new role should upsert, got: %v", err)
	}
	if ou.Role != "admin" {
		t.Fatalf("expected upsert to change role to admin, got %q", ou.Role)
	}

	members, err := d.GetOrganizationUsers(org.ID)
	if err != nil {
		t.Fatalf("GetOrganizationUsers: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected re-adding the same user to not duplicate the row, got %d rows", len(members))
	}
}

// TestAddOrganizationUser_ReAddingSoleAdminAtLowerRoleBlocked covers the
// upsert path demoting an existing member exactly like
// UpdateOrganizationUserRole can — re-adding the sole admin at role "user"
// (e.g. a re-invite that got the role wrong) must not silently leave the
// organization with zero admins.
func TestAddOrganizationUser_ReAddingSoleAdminAtLowerRoleBlocked(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	seedTestUser(t, d, "admin-1", "admin", 1)
	if _, err := d.AddOrganizationUser(org.ID, "admin-1", "admin"); err != nil {
		t.Fatalf("seed sole admin: %v", err)
	}

	if _, err := d.AddOrganizationUser(org.ID, "admin-1", "user"); !errors.Is(err, ErrLastOrgAdmin) {
		t.Fatalf("expected ErrLastOrgAdmin re-adding the sole admin at role=user, got %v", err)
	}

	role, isMember, err := d.GetOrganizationRole(org.ID, "admin-1")
	if err != nil || !isMember || role != "admin" {
		t.Fatalf("expected admin-1 to remain admin after the blocked re-add, got isMember=%v role=%q err=%v", isMember, role, err)
	}
}

func TestAddOrganizationUser_RejectsInvalidRole(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	seedTestUser(t, d, "user-1", "user", 0)

	if _, err := d.AddOrganizationUser(org.ID, "user-1", "superadmin"); err == nil {
		t.Fatal("expected an invalid role to be rejected")
	}
}

func TestRemoveLastOrgAdminBlocked(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	seedTestUser(t, d, "admin-1", "admin", 1)
	if _, err := d.AddOrganizationUser(org.ID, "admin-1", "admin"); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	if err := d.RemoveOrganizationUser(org.ID, "admin-1"); !errors.Is(err, ErrLastOrgAdmin) {
		t.Fatalf("expected ErrLastOrgAdmin removing the sole admin, got %v", err)
	}

	role, isMember, err := d.GetOrganizationRole(org.ID, "admin-1")
	if err != nil {
		t.Fatalf("GetOrganizationRole: %v", err)
	}
	if !isMember || role != "admin" {
		t.Fatalf("expected membership to survive a blocked removal, got isMember=%v role=%q", isMember, role)
	}
}

func TestUpdateOrganizationUserRole_DemotingLastAdminBlocked(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	seedTestUser(t, d, "admin-1", "admin", 1)
	if _, err := d.AddOrganizationUser(org.ID, "admin-1", "admin"); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	if err := d.UpdateOrganizationUserRole(org.ID, "admin-1", "user"); !errors.Is(err, ErrLastOrgAdmin) {
		t.Fatalf("expected ErrLastOrgAdmin demoting the sole admin, got %v", err)
	}
}

// TestRemoveOrgAdmin_SucceedsWithAnotherAdminPresent is the positive
// counterpart — the last-admin guard must not over-block when a second admin
// exists.
func TestRemoveOrgAdmin_SucceedsWithAnotherAdminPresent(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	seedTestUser(t, d, "admin-1", "admin", 1)
	seedTestUser(t, d, "admin-2", "admin", 1)
	if _, err := d.AddOrganizationUser(org.ID, "admin-1", "admin"); err != nil {
		t.Fatalf("seed admin-1: %v", err)
	}
	if _, err := d.AddOrganizationUser(org.ID, "admin-2", "admin"); err != nil {
		t.Fatalf("seed admin-2: %v", err)
	}

	if err := d.RemoveOrganizationUser(org.ID, "admin-1"); err != nil {
		t.Fatalf("expected removing one of two admins to succeed, got %v", err)
	}
	if _, isMember, err := d.GetOrganizationRole(org.ID, "admin-1"); err != nil || isMember {
		t.Fatalf("expected admin-1 membership gone, got isMember=%v err=%v", isMember, err)
	}
	role, isMember, err := d.GetOrganizationRole(org.ID, "admin-2")
	if err != nil || !isMember || role != "admin" {
		t.Fatalf("expected admin-2 to remain admin, got isMember=%v role=%q err=%v", isMember, role, err)
	}
}

// TestIsLastOrgAdmin_IgnoresInactiveAdmins covers the isActive filter in
// isLastOrgAdmin's "other admins" count: a deactivated admin shouldn't count
// as coverage that lets the last active admin be removed or demoted.
func TestIsLastOrgAdmin_IgnoresInactiveAdmins(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	seedTestUser(t, d, "admin-1", "admin", 1)
	seedTestUser(t, d, "admin-2", "admin", 1)
	if _, err := d.AddOrganizationUser(org.ID, "admin-1", "admin"); err != nil {
		t.Fatalf("seed admin-1: %v", err)
	}
	if _, err := d.AddOrganizationUser(org.ID, "admin-2", "admin"); err != nil {
		t.Fatalf("seed admin-2: %v", err)
	}
	if _, err := d.DB.Exec(`UPDATE users SET isActive = 0 WHERE id = ?`, "admin-2"); err != nil {
		t.Fatalf("deactivate admin-2: %v", err)
	}

	if err := d.RemoveOrganizationUser(org.ID, "admin-1"); !errors.Is(err, ErrLastOrgAdmin) {
		t.Fatalf("expected ErrLastOrgAdmin — the only other admin is inactive, got %v", err)
	}
}

// TestGetOrganizationsWhereSoleAdmin covers the guard api/users.go's
// deleteUser/updateUser use before deleting or deactivating a user, so that
// action can't silently orphan an organization's admin membership.
func TestGetOrganizationsWhereSoleAdmin(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgSolo, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-solo", Name: ptr("Solo Co")})
	if err != nil {
		t.Fatalf("CreateOrganization org-solo: %v", err)
	}
	orgShared, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-shared"})
	if err != nil {
		t.Fatalf("CreateOrganization org-shared: %v", err)
	}
	orgUntouched, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-untouched"})
	if err != nil {
		t.Fatalf("CreateOrganization org-untouched: %v", err)
	}
	seedTestUser(t, d, "user-1", "admin", 1)
	seedTestUser(t, d, "user-2", "admin", 1)

	if _, err := d.AddOrganizationUser(orgSolo.ID, "user-1", "admin"); err != nil {
		t.Fatalf("seed org-solo admin: %v", err)
	}
	if _, err := d.AddOrganizationUser(orgShared.ID, "user-1", "admin"); err != nil {
		t.Fatalf("seed org-shared admin user-1: %v", err)
	}
	if _, err := d.AddOrganizationUser(orgShared.ID, "user-2", "admin"); err != nil {
		t.Fatalf("seed org-shared admin user-2: %v", err)
	}
	if _, err := d.AddOrganizationUser(orgUntouched.ID, "user-2", "admin"); err != nil {
		t.Fatalf("seed org-untouched admin: %v", err)
	}

	solo, err := d.GetOrganizationsWhereSoleAdmin("user-1")
	if err != nil {
		t.Fatalf("GetOrganizationsWhereSoleAdmin: %v", err)
	}
	if len(solo) != 1 || solo[0].ID != orgSolo.ID {
		t.Fatalf("expected user-1 to be sole admin of exactly org-solo, got %+v", solo)
	}

	none, err := d.GetOrganizationsWhereSoleAdmin("user-2")
	if err != nil {
		t.Fatalf("GetOrganizationsWhereSoleAdmin user-2: %v", err)
	}
	for _, o := range none {
		if o.ID == orgShared.ID {
			t.Fatalf("expected user-2 not flagged sole admin of org-shared (user-1 is also admin), got %+v", none)
		}
	}

	// Deactivating user-2 (not deleting) makes them ineligible to count as
	// "another admin" for user-1's org-shared membership too — the guard
	// filters on isActive, not just row existence.
	if _, err := d.DB.Exec(`UPDATE users SET isActive = 0 WHERE id = ?`, "user-2"); err != nil {
		t.Fatalf("deactivate user-2: %v", err)
	}
	solo, err = d.GetOrganizationsWhereSoleAdmin("user-1")
	if err != nil {
		t.Fatalf("GetOrganizationsWhereSoleAdmin after deactivation: %v", err)
	}
	if len(solo) != 2 {
		t.Fatalf("expected user-1 now flagged sole admin of org-solo AND org-shared, got %+v", solo)
	}
}

// TestOrganizationUserBackfillPreservesAccess exercises migration 0069's
// backfill directly against a live migrated DB: since NewDatabase always runs
// every migration, this seeds users/orgs with plain INSERTs (bypassing the
// app-level Create* helpers, which only run at startup after 0069 already
// applied) to simulate rows that existed before 0069's ALTER/backfill ran —
// what matters is that the backfill INSERT...SELECT's CROSS JOIN shape does
// the right thing structurally, which this test verifies by replaying it by
// hand against seeded data.
func TestOrganizationUserBackfillPreservesAccess(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org1, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization org-1: %v", err)
	}
	org2, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-2"})
	if err != nil {
		t.Fatalf("CreateOrganization org-2: %v", err)
	}
	seedTestUser(t, d, "admin-1", "admin", 1)
	seedTestUser(t, d, "user-1", "user", 0)

	// Replay the backfill INSERT...SELECT from migration 0069 by hand,
	// against data seeded after the migration already ran — confirms it
	// grants every user membership in every organization at their existing
	// role, matching pre-migration "any authenticated user, any org" access.
	if _, err := d.DB.Exec(
		`INSERT INTO organization_users (id, organizationId, userId, role, createdAt)
		 SELECT lower(hex(randomblob(16))), o.id, u.id, u.role, strftime('%Y-%m-%d %H:%M:%S', 'now')
		 FROM organizations o CROSS JOIN users u`,
	); err != nil {
		t.Fatalf("replay backfill: %v", err)
	}

	for _, org := range []*Organization{org1, org2} {
		role, isMember, err := d.GetOrganizationRole(org.ID, "admin-1")
		if err != nil || !isMember || role != "admin" {
			t.Fatalf("expected admin-1 backfilled as admin of %s, got isMember=%v role=%q err=%v", org.ID, isMember, role, err)
		}
		role, isMember, err = d.GetOrganizationRole(org.ID, "user-1")
		if err != nil || !isMember || role != "user" {
			t.Fatalf("expected user-1 backfilled as user of %s, got isMember=%v role=%q err=%v", org.ID, isMember, role, err)
		}
	}
}
