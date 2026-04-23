package oac

import (
	"errors"
	"testing"
)

func TestNew_Defaults(t *testing.T) {
	c := New()
	if c.owner != "" || c.admin != "" {
		t.Fatalf("expected empty owner/admin, got %q/%q", c.owner, c.admin)
	}
	if c.runRBAC {
		t.Fatal("RBAC inspection must be off by default")
	}
	if c.skipManifest || c.skipResource || c.skipFolder {
		t.Fatal("manifest/resource/folder checks must be on by default")
	}
}

func TestOptions_OwnerAdmin(t *testing.T) {
	c := New(WithOwner("alice"), WithAdmin("root"))
	if c.owner != "alice" || c.admin != "root" {
		t.Fatalf("got %q/%q, want alice/root", c.owner, c.admin)
	}
	if c.Owner() != "alice" || c.Admin() != "root" {
		t.Fatal("accessors must mirror fields")
	}
}

func TestOptions_OwnerEmptyIsIgnored(t *testing.T) {
	c := New(WithOwner("alice"), WithOwner(""))
	if c.owner != "alice" {
		t.Fatalf("empty owner must not overwrite, got %q", c.owner)
	}
	c = New(WithAdmin("root"), WithAdmin(""))
	if c.admin != "root" {
		t.Fatalf("empty admin must not overwrite, got %q", c.admin)
	}
}

func TestOptions_OwnerAdminCombo(t *testing.T) {
	c := New(WithOwnerAdmin("ada"))
	if c.owner != "ada" || c.admin != "ada" {
		t.Fatalf("WithOwnerAdmin should set both, got %q/%q", c.owner, c.admin)
	}
	// Empty value is a no-op.
	c2 := New(WithOwnerAdmin("ada"), WithOwnerAdmin(""))
	if c2.owner != "ada" || c2.admin != "ada" {
		t.Fatalf("empty WithOwnerAdmin must not overwrite, got %q/%q", c2.owner, c2.admin)
	}
}

func TestOptions_SkipFlags(t *testing.T) {
	c := New(SkipManifestCheck(), SkipResourceCheck(), SkipFolderCheck())
	if !c.skipManifest || !c.skipResource || !c.skipFolder {
		t.Fatalf("Skip* options were not applied: %+v", c)
	}
}

func TestOptions_SameVersionToggle(t *testing.T) {
	// Default: same-version check runs (skipSameVersion stays zero-valued).
	c := New()
	if c.skipSameVersion {
		t.Fatal("same-version check must be on by default")
	}
	// SkipSameVersionCheck turns it off.
	c = New(SkipSameVersionCheck())
	if !c.skipSameVersion {
		t.Fatal("SkipSameVersionCheck should set skipSameVersion")
	}
	// WithSameVersionCheck flips it back on after SkipSameVersionCheck.
	c = New(SkipSameVersionCheck(), WithSameVersionCheck())
	if c.skipSameVersion {
		t.Fatal("WithSameVersionCheck should clear skipSameVersion")
	}
}

func TestOptions_WithServiceAccountRulesCheck(t *testing.T) {
	c := New(WithServiceAccountRulesCheck())
	if !c.runRBAC {
		t.Fatal("WithServiceAccountRulesCheck must enable runRBAC")
	}
}

func TestOptions_AutoOwnerScenariosToggle(t *testing.T) {
	// Default: not auto-expanding owner scenarios; explicit owner/admin
	// are honoured as-is.
	c := New(WithOwnerAdmin("alice"))
	if c.autoOwner {
		t.Fatal("autoOwner must be off by default")
	}
	if scenarios := c.ownerScenarios(); len(scenarios) != 1 ||
		scenarios[0].owner != "alice" || scenarios[0].admin != "alice" {
		t.Fatalf("default scenario must mirror explicit owner/admin, got %+v", scenarios)
	}

	// WithAutoOwnerScenarios flips it on and expands to exactly two
	// scenarios (owner==admin / owner!=admin) in a fixed order.
	c = New(WithOwnerAdmin("alice"), WithAutoOwnerScenarios())
	if !c.autoOwner {
		t.Fatal("WithAutoOwnerScenarios should set autoOwner")
	}
	scenarios := c.ownerScenarios()
	if len(scenarios) != 2 {
		t.Fatalf("auto-owner must yield two scenarios, got %d", len(scenarios))
	}
	if scenarios[0].label != "owner==admin" ||
		scenarios[0].owner != scenarios[0].admin {
		t.Fatalf("first scenario must be owner==admin, got %+v", scenarios[0])
	}
	if scenarios[1].label != "owner!=admin" ||
		scenarios[1].owner == scenarios[1].admin {
		t.Fatalf("second scenario must be owner!=admin, got %+v", scenarios[1])
	}

	// WithoutAutoOwnerScenarios clears the flag again.
	c = New(WithAutoOwnerScenarios(), WithoutAutoOwnerScenarios())
	if c.autoOwner {
		t.Fatal("WithoutAutoOwnerScenarios should clear autoOwner")
	}
}

func TestOptions_NilOptionIsIgnored(t *testing.T) {
	// New must tolerate a nil option (legitimate when callers conditionally
	// build slices).
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("New must not panic on nil option, got %v", r)
		}
	}()
	c := New(nil, WithOwner("alice"))
	if c.owner != "alice" {
		t.Fatalf("good options must still apply alongside nil, got %q", c.owner)
	}
}

func TestOptions_CustomValidator(t *testing.T) {
	called := 0
	c := New(WithCustomValidator(func(_ string, _ Manifest) error {
		called++
		return nil
	}))
	if len(c.customValidators) != 1 {
		t.Fatalf("validator not registered: %d", len(c.customValidators))
	}
	for _, v := range c.customValidators {
		if err := v("anywhere", nil); err != nil {
			t.Fatalf("validator returned: %v", err)
		}
	}
	if called != 1 {
		t.Fatalf("validator must be invoked once, got %d", called)
	}
}

func TestOptions_CustomValidator_NilIsIgnored(t *testing.T) {
	c := New(WithCustomValidator(nil))
	if len(c.customValidators) != 0 {
		t.Fatalf("nil validator must be dropped, got %d", len(c.customValidators))
	}
}

func TestOptions_CustomValidator_ChainsErrors(t *testing.T) {
	want := errors.New("custom failure")
	c := New(WithCustomValidator(func(_ string, _ Manifest) error { return want }))
	got := c.customValidators[0]("path", nil)
	if !errors.Is(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
