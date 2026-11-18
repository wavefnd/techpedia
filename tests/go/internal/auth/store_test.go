package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	badger "github.com/dgraph-io/badger/v4"
)

func TestEmptyPermissionsStayJSONArray(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	u, err := store.Register(ctx, Registration{Username: "readonly_user", Password: "long-test-password", UseRegularPermissions: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetRegularUserPermissions(ctx, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := store.getUserByUsername(ctx, u.Username)
	if err != nil {
		t.Fatal(err)
	}
	if raw.Permissions == nil {
		t.Error("empty policy persisted null permissions")
	}
	// Legacy accounts with null permissions must also remain readable without migration.
	raw.Permissions = nil
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.db.Update(func(txn *badger.Txn) error { return txn.Set([]byte(userKey(u.ID)), encoded) }); err != nil {
		t.Fatal(err)
	}
	users, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := store.GetProfile(ctx, u.Username)
	if err != nil {
		t.Fatal(err)
	}
	loggedIn, token, _, err := store.Login(ctx, u.Username, "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.Authenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range append(users, profile, loggedIn, session) {
		encoded, err := json.Marshal(user)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		if string(fields["permissions"]) != "[]" {
			t.Errorf("permissions = %s, want []", fields["permissions"])
		}
		if len(user.PasswordHash) != 0 || len(user.PasswordSalt) != 0 {
			t.Error("public user leaked credentials")
		}
	}
}

func TestRegisterLoginAndPermissions(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	registered, err := store.Register(ctx, Registration{
		Username:    "Editor_One",
		DisplayName: "Editor One",
		Password:    "correct-horse-battery-staple",
		Permissions: []Permission{PermissionEdit},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if registered.Username != "editor_one" || !registered.Has(PermissionEdit) {
		t.Fatalf("unexpected registered user: %#v", registered)
	}
	if len(registered.PasswordHash) != 0 || len(registered.PasswordSalt) != 0 {
		t.Fatal("public user leaked password material")
	}

	loggedIn, token, _, err := store.Login(ctx, "editor_one", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if token == "" || loggedIn.ID != registered.ID {
		t.Fatalf("unexpected login result: %#v", loggedIn)
	}

	sessionUser, err := store.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if sessionUser.Username != registered.Username {
		t.Fatalf("unexpected session user: %#v", sessionUser)
	}

	updated, err := store.SetPermissions(ctx, registered.ID, []Permission{PermissionEdit, PermissionTechdata})
	if err != nil {
		t.Fatalf("set permissions: %v", err)
	}
	if !updated.Has(PermissionTechdata) {
		t.Fatalf("expected techdata permission: %#v", updated)
	}
	profile, err := store.UpdateProfile(ctx, registered.ID, ProfileUpdate{
		DisplayName: "Editor One Updated",
		Bio:         "Technical writer\nInterested in compilers.",
	})
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if profile.DisplayName != "Editor One Updated" || profile.Bio != "Technical writer\nInterested in compilers." {
		t.Fatalf("unexpected updated profile: %#v", profile)
	}
	publicProfile, err := store.GetProfile(ctx, "EDITOR_ONE")
	if err != nil || publicProfile.Bio != profile.Bio || len(publicProfile.PasswordHash) != 0 {
		t.Fatalf("get public profile: %#v, %v", publicProfile, err)
	}
	if _, err := store.UpdateProfile(ctx, registered.ID, ProfileUpdate{DisplayName: "", Bio: "bio"}); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("expected empty display name rejection, got %v", err)
	}
	if _, err := store.UpdateProfile(ctx, registered.ID, ProfileUpdate{DisplayName: "Editor", Bio: string([]byte{'a', 0, 'b'})}); !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("expected control character rejection, got %v", err)
	}

	if err := store.Logout(ctx, token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := store.Authenticate(ctx, token); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected expired session, got %v", err)
	}
}

func TestRegistrationRules(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	registration := Registration{Username: "editor", Password: "long-enough-password", Permissions: []Permission{PermissionEdit}}
	if _, err := store.Register(ctx, registration); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	if _, err := store.Register(ctx, registration); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("expected duplicate username error, got %v", err)
	}
	if _, err := store.Register(ctx, Registration{Username: "x", Password: "short"}); !errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("expected invalid registration, got %v", err)
	}
}

func TestRegularUserPermissionsApplyToExistingAndFutureUsers(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	admin, err := store.Register(ctx, Registration{
		Username: "admin_user", Password: "long-admin-password", Permissions: []Permission{PermissionAdmin},
	})
	if err != nil {
		t.Fatalf("register administrator: %v", err)
	}
	first, err := store.Register(ctx, Registration{
		Username: "first_user", Password: "long-first-password", UseRegularPermissions: true,
	})
	if err != nil {
		t.Fatalf("register first regular user: %v", err)
	}
	if len(first.Permissions) != 0 {
		t.Fatalf("new store must default to no regular permissions: %#v", first.Permissions)
	}

	updated, err := store.SetRegularUserPermissions(ctx, []Permission{PermissionEdit, PermissionTechdata})
	if err != nil {
		t.Fatalf("set regular user permissions: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected one regular user update, got %d", updated)
	}
	users, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	for _, user := range users {
		switch user.ID {
		case admin.ID:
			if !user.Has(PermissionAdmin) || containsExactPermission(user.Permissions, PermissionEdit) {
				t.Fatalf("global policy changed administrator: %#v", user)
			}
		case first.ID:
			if !containsExactPermission(user.Permissions, PermissionEdit) || !containsExactPermission(user.Permissions, PermissionTechdata) {
				t.Fatalf("global policy did not update regular user: %#v", user)
			}
		}
	}

	second, err := store.Register(ctx, Registration{
		Username: "second_user", Password: "long-second-password", UseRegularPermissions: true,
	})
	if err != nil {
		t.Fatalf("register second regular user: %v", err)
	}
	if !containsExactPermission(second.Permissions, PermissionEdit) || !containsExactPermission(second.Permissions, PermissionTechdata) {
		t.Fatalf("future user did not inherit global policy: %#v", second)
	}
	if _, err := store.SetRegularUserPermissions(ctx, []Permission{PermissionAdmin}); !errors.Is(err, ErrInvalidPermissions) {
		t.Fatalf("expected admin permission to be rejected from global policy, got %v", err)
	}
	demoted, err := store.SetPermissions(ctx, admin.ID, nil)
	if err != nil || demoted.Has(PermissionAdmin) {
		t.Fatalf("expected administrator to be demotable, got %#v, %v", demoted, err)
	}
}

func TestBootstrapAccountIsTheSingleImmutableOwner(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.EnsureBootstrapAdmin(ctx, "site_owner", "long-owner-password"); err != nil {
		t.Fatalf("create bootstrap owner: %v", err)
	}
	owner, err := store.getUserByUsername(ctx, "site_owner")
	if err != nil {
		t.Fatalf("get bootstrap owner: %v", err)
	}
	if !owner.Has(PermissionOwner) || !owner.Has(PermissionAdmin) || !owner.Has(PermissionEdit) || !owner.Has(PermissionTechdata) {
		t.Fatalf("bootstrap account did not receive owner capabilities: %#v", owner)
	}
	if _, err := store.SetPermissions(ctx, owner.ID, nil); !errors.Is(err, ErrOwnerImmutable) {
		t.Fatalf("expected owner permissions to be immutable, got %v", err)
	}
	if err := store.EnsureBootstrapAdmin(ctx, "site_owner", ""); err != nil {
		t.Fatalf("existing owner should not require bootstrap password: %v", err)
	}

	other, err := store.Register(ctx, Registration{
		Username: "other_admin", Password: "long-other-password", Permissions: []Permission{PermissionAdmin, PermissionOwner},
	})
	if err != nil {
		t.Fatalf("register other administrator: %v", err)
	}
	if other.Has(PermissionOwner) {
		t.Fatalf("ordinary registration assigned owner permission: %#v", other)
	}
	if err := store.EnsureBootstrapAdmin(ctx, other.Username, ""); !errors.Is(err, ErrOwnerAlreadyExists) {
		t.Fatalf("expected second owner assignment to fail, got %v", err)
	}
}

func TestOpeningLegacyStoreInitializesRegularPolicyWithoutChangingAdmin(t *testing.T) {
	path := t.TempDir()
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	ctx := context.Background()
	regular, err := store.Register(ctx, Registration{
		Username: "legacy_user", Password: "long-legacy-password", Permissions: []Permission{PermissionEdit},
	})
	if err != nil {
		t.Fatalf("register legacy user: %v", err)
	}
	admin, err := store.Register(ctx, Registration{
		Username: "legacy_admin", Password: "long-admin-password", Permissions: []Permission{PermissionAdmin},
	})
	if err != nil {
		t.Fatalf("register legacy administrator: %v", err)
	}
	if err := store.db.Update(func(txn *badger.Txn) error { return txn.Delete([]byte(regularPermissionsKey)) }); err != nil {
		t.Fatalf("remove policy to simulate legacy store: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close legacy store: %v", err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatalf("reopen legacy store: %v", err)
	}
	defer store.Close()
	users, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list migrated users: %v", err)
	}
	for _, user := range users {
		switch user.ID {
		case regular.ID:
			if len(user.Permissions) != 0 {
				t.Fatalf("legacy regular user permissions were not normalized: %#v", user)
			}
		case admin.ID:
			if len(user.Permissions) != 1 || user.Permissions[0] != PermissionAdmin {
				t.Fatalf("legacy administrator was changed: %#v", user)
			}
		}
	}
}

func containsExactPermission(permissions []Permission, expected Permission) bool {
	for _, permission := range permissions {
		if permission == expected {
			return true
		}
	}
	return false
}
