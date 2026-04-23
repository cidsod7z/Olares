package manifest

import (
	"strings"
	"testing"
)

func newValidConfig() *AppConfiguration {
	return &AppConfiguration{
		ConfigVersion: "0.11.0",
		APIVersion:    APIVersionV1,
		Metadata: AppMetaData{
			Name:        "firefox",
			Icon:        "https://example.com/icon.png",
			Description: "a browser",
			Title:       "Firefox",
			Version:     "1.2.3",
		},
		Entrances: []Entrance{{
			Name:       "main",
			Host:       "firefox",
			Port:       8080,
			Title:      "Main",
			Icon:       "https://example.com/entrance.png",
			AuthLevel:  "public",
			OpenMethod: "default",
		}},
	}
}

func TestAppConfiguration_Valid(t *testing.T) {
	if err := ValidateAppConfiguration(newValidConfig()); err != nil {
		t.Fatalf("baseline fixture must pass: %v", err)
	}
}

func TestAppConfiguration_MissingConfigVersion(t *testing.T) {
	c := newValidConfig()
	c.ConfigVersion = ""
	err := ValidateAppConfiguration(c)
	if err == nil {
		t.Fatal("expected error for missing olaresManifest.version")
	}
	if !strings.Contains(err.Error(), "olaresManifest.version") {
		t.Fatalf("error should mention olaresManifest.version, got: %v", err)
	}
}

func TestAppConfiguration_APIVersionEnum(t *testing.T) {
	c := newValidConfig()
	c.APIVersion = "v99"
	err := ValidateAppConfiguration(c)
	if err == nil {
		t.Fatal("expected error for apiVersion=v99")
	}
	if !strings.Contains(err.Error(), "apiVersion") {
		t.Fatalf("error should mention apiVersion, got: %v", err)
	}

	c = newValidConfig()
	c.APIVersion = ""
	if err := ValidateAppConfiguration(c); err != nil {
		t.Fatalf("empty apiVersion should be accepted: %v", err)
	}
}

func TestAppMetaData_VersionSemver(t *testing.T) {
	cases := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"valid semver", "1.2.3", false},
		{"with prerelease", "1.2.3-beta", false},
		{"bad format", "not-a-version", true},
		{"empty", "", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			c := newValidConfig()
			c.Metadata.Version = tc.version
			err := ValidateAppConfiguration(c)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestEntrance_AuthLevelEnum(t *testing.T) {
	cases := []struct {
		value   string
		wantErr bool
	}{
		{"", false},
		{"public", false},
		{"private", false},
		{"bogus", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.value, func(t *testing.T) {
			c := newValidConfig()
			c.Entrances[0].AuthLevel = tc.value
			err := ValidateAppConfiguration(c)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestEntrance_OpenMethodEnum(t *testing.T) {
	cases := []struct {
		value   string
		wantErr bool
	}{
		{"", false},
		{"default", false},
		{"iframe", false},
		{"window", false},
		{"popup", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.value, func(t *testing.T) {
			c := newValidConfig()
			c.Entrances[0].OpenMethod = tc.value
			err := ValidateAppConfiguration(c)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestEntrance_IconURL(t *testing.T) {
	cases := []struct {
		icon    string
		wantErr bool
	}{
		{"", false},
		{"http://example.com/x.png", false},
		{"https://example.com/x.png", false},
		{"ftp://example.com/x.png", true},
		{"not-a-url", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.icon, func(t *testing.T) {
			c := newValidConfig()
			c.Entrances[0].Icon = tc.icon
			err := ValidateAppConfiguration(c)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestEntrance_Required(t *testing.T) {
	c := newValidConfig()
	c.Entrances = nil
	if err := ValidateAppConfiguration(c); err == nil {
		t.Fatal("expected error: entrances is required")
	}
}

func TestEntrance_UniqueNames(t *testing.T) {
	c := newValidConfig()
	c.Entrances = []Entrance{
		{Name: "dup", Host: "alpha", Port: 1, Title: "A"},
		{Name: "dup", Host: "bravo", Port: 2, Title: "B"},
	}
	err := ValidateAppConfiguration(c)
	if err == nil {
		t.Fatal("expected error for duplicate entrance names")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error should mention duplicate, got: %v", err)
	}
}

func TestEntrance_PortNegative(t *testing.T) {
	c := newValidConfig()
	c.Entrances[0].Port = -1
	if err := ValidateAppConfiguration(c); err == nil {
		t.Fatal("expected error: port must be > 0")
	}
}

func TestAppSpec_QuantityFields(t *testing.T) {
	c := newValidConfig()
	c.Spec.RequiredMemory = "not-a-quantity"
	err := ValidateAppConfiguration(c)
	if err == nil {
		t.Fatal("expected error for invalid requiredMemory")
	}
}

func TestSubCharts_OnlyEnforcedForV2(t *testing.T) {
	c := newValidConfig()
	c.APIVersion = APIVersionV1
	if err := ValidateAppConfiguration(c); err != nil {
		t.Fatalf("v1 should not require subCharts: %v", err)
	}

	c = newValidConfig()
	c.APIVersion = APIVersionV2
	err := ValidateAppConfiguration(c)
	if err == nil {
		t.Fatal("v2 without subCharts should fail")
	}
	if !strings.Contains(err.Error(), "subCharts") {
		t.Fatalf("error should mention subCharts, got: %v", err)
	}

	c.Spec.SubCharts = []Chart{{Name: "main", Shared: false}}
	err = ValidateAppConfiguration(c)
	if err == nil {
		t.Fatal("v2 needs at least one shared=true subChart")
	}
	if !strings.Contains(err.Error(), "shared=true") {
		t.Fatalf("error should mention shared=true, got: %v", err)
	}

	c.Spec.SubCharts = []Chart{{Name: "main", Shared: true}}
	if err := ValidateAppConfiguration(c); err != nil {
		t.Fatalf("v2 with shared subchart should pass: %v", err)
	}
}

func TestSubCharts_V2TriggersRegardlessOfOlaresVersion(t *testing.T) {
	legacyVersions := []string{"0.11.0", "0.11.9"}
	for _, v := range legacyVersions {
		v := v
		t.Run("olaresManifest.version="+v, func(t *testing.T) {
			c := newValidConfig()
			c.ConfigVersion = v
			c.APIVersion = APIVersionV2
			err := ValidateAppConfiguration(c)
			if err == nil {
				t.Fatalf("v2 manifest (olaresManifest.version=%s) without subCharts must fail", v)
			}
			if !strings.Contains(err.Error(), "subCharts is required for apiVersion=v2") {
				t.Fatalf("error should mention subCharts requirement, got: %v", err)
			}
		})
	}
}

func TestDependency_TypeEnum(t *testing.T) {
	c := newValidConfig()
	c.Options.Dependencies = []Dependency{{Name: "foo", Version: "1.0.0", Type: "bogus"}}
	if err := ValidateAppConfiguration(c); err == nil {
		t.Fatal("expected error for bad dependency type")
	}

	c.Options.Dependencies = []Dependency{{Name: "foo", Version: "1.0.0", Type: "system"}}
	if err := ValidateAppConfiguration(c); err != nil {
		t.Fatalf("system is a valid dep type: %v", err)
	}

	c.Options.Dependencies = []Dependency{{Name: "foo", Version: "1.0.0", Type: "application"}}
	if err := ValidateAppConfiguration(c); err != nil {
		t.Fatalf("application is a valid dep type: %v", err)
	}
}

func TestPolicy_DurationPattern(t *testing.T) {
	c := newValidConfig()
	c.Options.Policies = []Policy{{
		URIRegex: "^/",
		Level:    "one_factor",
		Duration: "not-a-duration",
	}}
	if err := ValidateAppConfiguration(c); err == nil {
		t.Fatal("expected error for bad validDuration")
	}

	c.Options.Policies[0].Duration = "3600s"
	if err := ValidateAppConfiguration(c); err != nil {
		t.Fatalf("3600s should be a valid duration: %v", err)
	}
}
