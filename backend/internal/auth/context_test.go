package auth

import "testing"

func TestPrincipalHas(t *testing.T) {
	// Explicit permission grant.
	p := &Principal{Permissions: map[string]bool{"Host.View": true}}
	if !p.Has("Host.View") {
		t.Fatal("should have explicitly granted permission")
	}
	if p.Has("Host.Delete") {
		t.Fatal("should not have ungranted permission")
	}

	// Admin.All is a wildcard.
	wild := &Principal{Permissions: map[string]bool{"Admin.All": true}}
	if !wild.Has("Host.Delete") || !wild.Has("anything") {
		t.Fatal("Admin.All must satisfy any permission")
	}

	// Super admins satisfy everything regardless of explicit perms.
	super := &Principal{IsSuperAdmin: true, Permissions: map[string]bool{}}
	if !super.Has("System.Configure") {
		t.Fatal("super admin must satisfy any permission")
	}

	// nil principal has nothing.
	var none *Principal
	if none.Has("Host.View") {
		t.Fatal("nil principal must not have permissions")
	}
}
