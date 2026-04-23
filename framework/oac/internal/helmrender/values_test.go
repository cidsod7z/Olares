package helmrender

import (
	"testing"

	olm "github.com/beclab/Olares/framework/oac/internal/manifest"
)

func TestBuildValues_DefaultsOwner(t *testing.T) {
	v := BuildValues("", "", nil)
	bfl, ok := v["bfl"].(map[string]interface{})
	if !ok {
		t.Fatalf("bfl missing or wrong type: %T", v["bfl"])
	}
	if bfl["username"] != "bfl-username" {
		t.Fatalf("default owner = %v, want bfl-username", bfl["username"])
	}
	if _, hasAdmin := v["admin"]; hasAdmin {
		t.Fatal("admin must be unset when caller passes empty admin")
	}
}

func TestBuildValues_OwnerAndAdmin(t *testing.T) {
	v := BuildValues("alice", "root", nil)
	if v["bfl"].(map[string]interface{})["username"] != "alice" {
		t.Fatalf("owner not propagated: %v", v["bfl"])
	}
	if v["admin"] != "root" {
		t.Fatalf("admin not propagated: %v", v["admin"])
	}
}

func TestBuildValues_DomainPerEntrance(t *testing.T) {
	v := BuildValues("alice", "root", []olm.EntranceInfo{
		{Name: "main"},
		{Name: "api"},
	})
	domain, ok := v["domain"].(map[string]interface{})
	if !ok {
		t.Fatalf("domain missing or wrong type: %T", v["domain"])
	}
	if _, ok := domain["main"]; !ok {
		t.Fatal("missing entry for entrance 'main'")
	}
	if _, ok := domain["api"]; !ok {
		t.Fatal("missing entry for entrance 'api'")
	}
	if len(domain) != 2 {
		t.Fatalf("expected 2 entries, got %d (%v)", len(domain), domain)
	}
}

func TestSetGPUType_PopulatesMissingMap(t *testing.T) {
	v := BuildValues("alice", "alice", nil)
	delete(v, "GPU")
	SetGPUType(v, "nvidia")
	gpu, ok := v["GPU"].(map[string]interface{})
	if !ok {
		t.Fatalf("GPU missing or wrong type after SetGPUType: %T", v["GPU"])
	}
	if gpu["Type"] != "nvidia" {
		t.Fatalf("GPU.Type = %v, want nvidia", gpu["Type"])
	}
}

func TestSetGPUType_PreservesExistingMap(t *testing.T) {
	v := map[string]interface{}{
		"GPU": map[string]interface{}{"Other": "stays"},
	}
	SetGPUType(v, "amd-gpu")
	gpu := v["GPU"].(map[string]interface{})
	if gpu["Other"] != "stays" {
		t.Fatalf("sibling field clobbered: %v", gpu)
	}
	if gpu["Type"] != "amd-gpu" {
		t.Fatalf("GPU.Type = %v, want amd-gpu", gpu["Type"])
	}
}

func TestSetGPUType_EmptyClearsType(t *testing.T) {
	v := BuildValues("alice", "alice", nil)
	SetGPUType(v, "nvidia")
	SetGPUType(v, "")
	gpu := v["GPU"].(map[string]interface{})
	if _, ok := gpu["Type"]; ok {
		t.Fatalf("empty mode should clear GPU.Type, got %v", gpu)
	}
}

func TestSetGPUType_RebuildsWrongType(t *testing.T) {
	v := map[string]interface{}{"GPU": "not-a-map"}
	SetGPUType(v, "nvidia")
	gpu, ok := v["GPU"].(map[string]interface{})
	if !ok {
		t.Fatalf("GPU should be rewritten as map, got %T", v["GPU"])
	}
	if gpu["Type"] != "nvidia" {
		t.Fatalf("GPU.Type = %v, want nvidia", gpu["Type"])
	}
}

func TestBuildValues_HasStableScaffold(t *testing.T) {
	v := BuildValues("alice", "alice", nil)
	required := []string{
		"user", "schedule", "userspace", "os", "postgres", "redis",
		"mongodb", "zinc", "mariadb", "mysql", "minio", "rabbitmq",
		"elasticsearch", "nats", "svcs", "cluster", "GPU", "oidc", "olaresEnv",
	}
	for _, k := range required {
		if _, ok := v[k]; !ok {
			t.Errorf("missing key %q from helm values scaffold", k)
		}
	}
}

func TestSetClientOnlyInstall(t *testing.T) {
	v := BuildValues("a", "b", nil)
	SetClientOnlyInstall(v)
	if v["client"] != true {
		t.Fatalf("client = %v, want true", v["client"])
	}
	if _, ok := v["clientAndServer"]; ok {
		t.Fatalf("clientAndServer should be cleared, got %v", v["clientAndServer"])
	}
}

func TestSetClientAndServerInstall(t *testing.T) {
	v := map[string]interface{}{"client": true}
	SetClientAndServerInstall(v)
	if v["clientAndServer"] != true {
		t.Fatalf("clientAndServer = %v, want true", v["clientAndServer"])
	}
	if _, ok := v["client"]; ok {
		t.Fatalf("client should be cleared, got %v", v["client"])
	}
}

func TestApplyDefaultInstallProfile_V2Only(t *testing.T) {
	v := BuildValues("a", "b", nil)
	ApplyDefaultInstallProfile(v, "v1")
	if _, ok := v["clientAndServer"]; ok {
		t.Fatalf("v1 must not set clientAndServer")
	}
	v2 := BuildValues("a", "b", nil)
	ApplyDefaultInstallProfile(v2, "V2")
	if v2["clientAndServer"] != true {
		t.Fatalf("v2 profile: want clientAndServer=true, got %v", v2["clientAndServer"])
	}
}
