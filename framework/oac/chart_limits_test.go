package oac

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckResourceLimits_ModernV1_UsesInlineManifestLimits(t *testing.T) {
	c := New(WithOwner("alice"), WithAdmin("admin"))
	dir := filepath.Join("testdata", "resourcelimits_v1_inline")
	m, err := c.LoadManifestFile(dir)
	if err != nil {
		t.Fatalf("LoadManifestFile: %v", err)
	}
	sc := ownerScenario{owner: "alice", admin: "admin"}
	if err := c.checkResourceLimits(dir, m, sc, nil); err != nil {
		t.Fatalf("checkResourceLimits: %v", err)
	}
}

func TestCheckResourceLimits_ModernV1_InlineMismatchFails(t *testing.T) {
	c := New(WithOwner("alice"), WithAdmin("admin"))
	dir := filepath.Join("testdata", "resourcelimits_v1_inline_toobig")
	m, err := c.LoadManifestFile(dir)
	if err != nil {
		t.Fatalf("LoadManifestFile: %v", err)
	}
	sc := ownerScenario{owner: "alice", admin: "admin"}
	limErr := c.checkResourceLimits(dir, m, sc, nil)
	if limErr == nil {
		t.Fatal("expected error: container requests exceed inline spec.requiredCpu")
	}
	if !strings.Contains(limErr.Error(), "spec.requiredCpu") {
		t.Fatalf("expected spec.requiredCpu in error, got: %v", limErr)
	}
}

func TestCheckResourceLimits_ModernV2_ClientAndClientAndServerRenders(t *testing.T) {
	c := New(WithOwner("alice"), WithAdmin("admin"))
	dir := filepath.Join("testdata", "resourcelimits_v2_split")
	m, err := c.LoadManifestFile(dir)
	if err != nil {
		t.Fatalf("LoadManifestFile: %v", err)
	}
	sc := ownerScenario{owner: "alice", admin: "admin"}
	if err := c.checkResourceLimits(dir, m, sc, nil); err != nil {
		t.Fatalf("checkResourceLimits: %v", err)
	}
}

func TestCheckResourceLimits_ModernV2_ClientRenderOverLimitFails(t *testing.T) {
	c := New(WithOwner("alice"), WithAdmin("admin"))
	dir := filepath.Join("testdata", "resourcelimits_v2_client_too_big")
	m, err := c.LoadManifestFile(dir)
	if err != nil {
		t.Fatalf("LoadManifestFile: %v", err)
	}
	sc := ownerScenario{owner: "alice", admin: "admin"}
	limErr := c.checkResourceLimits(dir, m, sc, nil)
	if limErr == nil {
		t.Fatal("expected error from client render over client limits")
	}
	if !strings.Contains(limErr.Error(), "client render") || !strings.Contains(limErr.Error(), "spec.requiredCpu") {
		t.Fatalf("expected client render + spec.requiredCpu in error, got: %v", limErr)
	}
}

func TestCheckResourceLimits_ModernV2_ClientAndServerRenderOverCombinedFails(t *testing.T) {
	c := New(WithOwner("alice"), WithAdmin("admin"))
	dir := filepath.Join("testdata", "resourcelimits_v2_both_too_big")
	m, err := c.LoadManifestFile(dir)
	if err != nil {
		t.Fatalf("LoadManifestFile: %v", err)
	}
	sc := ownerScenario{owner: "alice", admin: "admin"}
	limErr := c.checkResourceLimits(dir, m, sc, nil)
	if limErr == nil {
		t.Fatal("expected error from clientAndServer render over combined limits")
	}
	if !strings.Contains(limErr.Error(), "clientAndServer render") || !strings.Contains(limErr.Error(), "spec.limitedCpu") {
		t.Fatalf("expected clientAndServer render + spec.limitedCpu in error, got: %v", limErr)
	}
}
