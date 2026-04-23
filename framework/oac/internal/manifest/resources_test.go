package manifest

import (
	"strings"
	"testing"
)

// newResourcesConfig builds a config that exercises the resource block at a
// version above the gate (so checkResources actually runs).
func newResourcesConfig(modes ...ResourceMode) *AppConfiguration {
	c := newValidConfig()
	c.ConfigVersion = "0.13.0" // >= 0.12.0 -> rules apply
	c.APIVersion = APIVersionV1
	c.Spec.SupportArch = []string{"amd64", "arm64"}
	c.Spec.Resources = modes
	return c
}

func TestResourcesCheckApplies(t *testing.T) {
	cases := []struct {
		v      string
		want   bool
		reason string
	}{
		{"", false, "missing"},
		{"junk", false, "malformed"},
		{"0.11.0", false, "below gate"},
		{"0.12.0", true, "boundary is inclusive"},
		{"0.12.1", true, "above gate"},
		{"1.0.0", true, "well above gate"},
	}
	for _, tc := range cases {
		if got := resourcesCheckApplies(tc.v); got != tc.want {
			t.Errorf("resourcesCheckApplies(%q) = %v, want %v (%s)", tc.v, got, tc.want, tc.reason)
		}
	}
}

func TestIsModernResourcesManifest(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"", false},
		{"junk", false},
		{"0.11.0", false},
		{"0.12.0", true},
		{"0.12.1", true},
		{"1.0.0", true},
	}
	for _, tc := range cases {
		if got := IsModernResourcesManifest(tc.v); got != tc.want {
			t.Errorf("IsModernResourcesManifest(%q) = %v, want %v", tc.v, got, tc.want)
		}
	}
}

// Inline-only mode: CoreLimits returns the inline fields verbatim.
func TestCoreLimits_Inline(t *testing.T) {
	rm := ResourceMode{
		Mode: ResourceModeCPU,
		ResourceRequirement: ResourceRequirement{
			RequiredCPU:    "100m",
			LimitedCPU:     "200m",
			RequiredMemory: "256Mi",
			LimitedMemory:  "512Mi",
		},
	}
	reqCPU, reqMem, limCPU, limMem := ResourceModeCoreLimits(rm)
	if reqCPU != "100m" || reqMem != "256Mi" || limCPU != "200m" || limMem != "512Mi" {
		t.Fatalf("inline CoreLimits mismatched: (%s, %s, %s, %s)", reqCPU, reqMem, limCPU, limMem)
	}
}

// Server+client mode: CoreLimits sums the two splits via k8s quantity math.
func TestCoreLimits_SumsServerAndClient(t *testing.T) {
	rm := ResourceMode{
		Mode: ResourceModeCPU,
		Server: &ResourceRequirement{
			RequiredCPU:    "100m",
			LimitedCPU:     "200m",
			RequiredMemory: "128Mi",
			LimitedMemory:  "256Mi",
		},
		Client: &ResourceRequirement{
			RequiredCPU:    "50m",
			LimitedCPU:     "100m",
			RequiredMemory: "64Mi",
			LimitedMemory:  "128Mi",
		},
	}
	reqCPU, reqMem, limCPU, limMem := ResourceModeCoreLimits(rm)
	// 100m + 50m = 150m, 200m + 100m = 300m, etc. k8s quantity arithmetic
	// normalises the canonical form.
	if reqCPU != "150m" || limCPU != "300m" {
		t.Fatalf("cpu sum mismatch: req=%s lim=%s", reqCPU, limCPU)
	}
	if reqMem != "192Mi" || limMem != "384Mi" {
		t.Fatalf("memory sum mismatch: req=%s lim=%s", reqMem, limMem)
	}
}

func TestResourceCPUMemLimitsForInstallProfile_V2Split(t *testing.T) {
	rm := ResourceMode{
		Mode: ResourceModeNvidia,
		Server: &ResourceRequirement{
			RequiredCPU: "100m", LimitedCPU: "200m",
			RequiredMemory: "128Mi", LimitedMemory: "256Mi",
			RequiredDisk: "2Gi", LimitedDisk: "4Gi",
			RequiredGPU: "4Gi", LimitedGPU: "8Gi",
		},
		Client: &ResourceRequirement{
			RequiredCPU: "50m", LimitedCPU: "100m",
			RequiredMemory: "64Mi", LimitedMemory: "128Mi",
			RequiredDisk: "1Gi", LimitedDisk: "2Gi",
			RequiredGPU: "2Gi", LimitedGPU: "4Gi",
		},
	}
	t.Run("clientOnly", func(t *testing.T) {
		reqCPU, reqMem, limCPU, limMem, err := ResourceCPUMemLimitsForInstallProfile(rm, APIVersionV2, InstallOrUpgradeClientOnly)
		if err != nil {
			t.Fatal(err)
		}
		if reqCPU != "50m" || limCPU != "100m" || reqMem != "64Mi" || limMem != "128Mi" {
			t.Fatalf("clientOnly: got (%s,%s,%s,%s)", reqCPU, reqMem, limCPU, limMem)
		}
		full, err := ResourceLimitsForInstallProfile(rm, APIVersionV2, InstallOrUpgradeClientOnly)
		if err != nil {
			t.Fatal(err)
		}
		if full.RequiredDisk != "1Gi" || full.LimitedDisk != "2Gi" || full.RequiredGPU != "2Gi" || full.LimitedGPU != "4Gi" {
			t.Fatalf("clientOnly full: %+v", full)
		}
	})
	t.Run("clientAndServer", func(t *testing.T) {
		reqCPU, reqMem, limCPU, limMem, err := ResourceCPUMemLimitsForInstallProfile(rm, APIVersionV2, InstallOrUpgradeServerAndClient)
		if err != nil {
			t.Fatal(err)
		}
		if reqCPU != "150m" || limCPU != "300m" || reqMem != "192Mi" || limMem != "384Mi" {
			t.Fatalf("clientAndServer: got (%s,%s,%s,%s)", reqCPU, reqMem, limCPU, limMem)
		}
		full, err := ResourceLimitsForInstallProfile(rm, APIVersionV2, InstallOrUpgradeServerAndClient)
		if err != nil {
			t.Fatal(err)
		}
		if full.RequiredDisk != "3Gi" || full.LimitedDisk != "6Gi" || full.RequiredGPU != "6Gi" || full.LimitedGPU != "12Gi" {
			t.Fatalf("clientAndServer full: %+v", full)
		}
	})
}

func TestResourceCPUMemLimitsForInstallProfile_InlineIgnoresProfile(t *testing.T) {
	rm := ResourceMode{
		Mode: ResourceModeCPU,
		ResourceRequirement: ResourceRequirement{
			RequiredCPU: "10m", LimitedCPU: "20m",
			RequiredMemory: "32Mi", LimitedMemory: "64Mi",
			RequiredDisk: "1Gi", LimitedDisk: "2Gi",
		},
	}
	for _, prof := range []string{InstallOrUpgradeClientOnly, InstallOrUpgradeServerAndClient} {
		reqCPU, reqMem, limCPU, limMem, err := ResourceCPUMemLimitsForInstallProfile(rm, APIVersionV2, prof)
		if err != nil {
			t.Fatal(err)
		}
		if reqCPU != "10m" || limCPU != "20m" || reqMem != "32Mi" || limMem != "64Mi" {
			t.Fatalf("profile=%s: (%s,%s,%s,%s)", prof, reqCPU, reqMem, limCPU, limMem)
		}
		full, err := ResourceLimitsForInstallProfile(rm, APIVersionV2, prof)
		if err != nil {
			t.Fatal(err)
		}
		if full.RequiredDisk != "1Gi" || full.LimitedDisk != "2Gi" {
			t.Fatalf("profile=%s disk: %+v", prof, full)
		}
	}
}

func TestResourceCPUMemLimitsForInstallProfile_Errors(t *testing.T) {
	rm := ResourceMode{Mode: ResourceModeCPU, Client: &ResourceRequirement{
		RequiredCPU: "1", LimitedCPU: "2", RequiredMemory: "1", LimitedMemory: "2",
	}}
	if _, _, _, _, err := ResourceCPUMemLimitsForInstallProfile(rm, APIVersionV2, "bogus"); err == nil {
		t.Fatal("expected error for bogus installProfile")
	}
	if _, _, _, _, err := ResourceCPUMemLimitsForInstallProfile(rm, APIVersionV2, ""); err == nil {
		t.Fatal("expected error for empty installProfile")
	}
	split := ResourceMode{
		Mode: ResourceModeCPU,
		Server: &ResourceRequirement{
			RequiredCPU: "1", LimitedCPU: "2", RequiredMemory: "1", LimitedMemory: "2",
		},
	}
	if _, _, _, _, err := ResourceCPUMemLimitsForInstallProfile(split, APIVersionV2, InstallOrUpgradeClientOnly); err == nil {
		t.Fatal("expected error: v2 clientOnly requires client section")
	}
}

// v1 always returns the inline ResourceRequirement, regardless of
// installProfile or whether the inline envelope is empty.
func TestResourceLimitsForInstallProfile_V1ReturnsInline(t *testing.T) {
	rm := ResourceMode{
		Mode: ResourceModeCPU,
		ResourceRequirement: ResourceRequirement{
			RequiredCPU: "10m", LimitedCPU: "20m",
			RequiredMemory: "32Mi", LimitedMemory: "64Mi",
			RequiredDisk: "1Gi", LimitedDisk: "2Gi",
		},
	}
	for _, apiV := range []string{APIVersionV1, ""} {
		for _, prof := range []string{InstallOrUpgradeClientOnly, InstallOrUpgradeServerAndClient} {
			got, err := ResourceLimitsForInstallProfile(rm, apiV, prof)
			if err != nil {
				t.Fatalf("apiVersion=%q profile=%s: %v", apiV, prof, err)
			}
			want := limitsFromEmbedded(rm.ResourceRequirement)
			if got != want {
				t.Fatalf("apiVersion=%q profile=%s: got %+v, want %+v", apiV, prof, got, want)
			}
		}
	}
	empty := ResourceMode{Mode: ResourceModeCPU}
	got, err := ResourceLimitsForInstallProfile(empty, APIVersionV1, InstallOrUpgradeClientOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got != (ResourceRequirementLimits{}) {
		t.Fatalf("v1 with empty inline must yield zero struct, got %+v", got)
	}
}

// Server-only mode (no client, no inline): sum collapses to server's values.
func TestCoreLimits_ServerOnly(t *testing.T) {
	rm := ResourceMode{
		Mode: ResourceModeCPU,
		Server: &ResourceRequirement{
			RequiredCPU:    "100m",
			LimitedCPU:     "200m",
			RequiredMemory: "128Mi",
			LimitedMemory:  "256Mi",
		},
	}
	reqCPU, reqMem, limCPU, limMem := ResourceModeCoreLimits(rm)
	if reqCPU != "100m" || limCPU != "200m" || reqMem != "128Mi" || limMem != "256Mi" {
		t.Fatalf("server-only CoreLimits mismatch: (%s, %s, %s, %s)", reqCPU, reqMem, limCPU, limMem)
	}
}

// Totally empty mode: CoreLimits returns canonical zeros rather than empty
// strings so downstream parsers can treat the result uniformly.
func TestCoreLimits_EmptyCollapsesToZero(t *testing.T) {
	rm := ResourceMode{Mode: ResourceModeCPU}
	reqCPU, reqMem, limCPU, limMem := ResourceModeCoreLimits(rm)
	for name, got := range map[string]string{
		"requiredCpu":    reqCPU,
		"requiredMemory": reqMem,
		"limitedCpu":     limCPU,
		"limitedMemory":  limMem,
	} {
		if got != "0" {
			t.Errorf("%s = %q, want 0", name, got)
		}
	}
}

// Unparseable quantity in a split must not panic; addQuantity skips it.
func TestCoreLimits_UnparseableSkipped(t *testing.T) {
	rm := ResourceMode{
		Mode: ResourceModeCPU,
		Server: &ResourceRequirement{
			RequiredCPU: "not-a-quantity",
			LimitedCPU:  "200m",
		},
	}
	reqCPU, _, limCPU, _ := ResourceModeCoreLimits(rm)
	if reqCPU != "0" {
		t.Fatalf("unparseable required cpu must collapse to 0, got %s", reqCPU)
	}
	if limCPU != "200m" {
		t.Fatalf("parseable sibling ignored: got %s", limCPU)
	}
}

func TestResourceMode_Required(t *testing.T) {
	rm := ResourceMode{
		Mode: "",
		ResourceRequirement: ResourceRequirement{
			RequiredCPU: "100m",
		},
	}
	if err := ValidateResourceMode(rm); err == nil {
		t.Fatal("expected error: mode is required")
	}
}

func TestResourceMode_BadMode(t *testing.T) {
	rm := ResourceMode{Mode: "rocm-mi300"}
	err := ValidateResourceMode(rm)
	if err == nil {
		t.Fatal("expected error for unknown mode")
	}
	if !strings.Contains(err.Error(), "must be one of") {
		t.Fatalf("error should list valid modes, got: %v", err)
	}
}

func TestResourceMode_BadQuantity(t *testing.T) {
	rm := ResourceMode{
		Mode: ResourceModeCPU,
		ResourceRequirement: ResourceRequirement{
			RequiredCPU:    "lots",
			RequiredMemory: "200Mi",
		},
	}
	err := ValidateResourceMode(rm)
	if err == nil {
		t.Fatal("expected quantity parse error")
	}
	if !strings.Contains(err.Error(), "requiredCpu") {
		t.Fatalf("error should mention requiredCpu, got: %v", err)
	}
}

// Rule 1: chip family <-> required CPU arch.
func TestRule1_ModeArchRequirement(t *testing.T) {
	cases := []struct {
		name        string
		mode        string
		supportArch []string
		wantErr     bool
		wantNeed    string // arch substring expected in the error
	}{
		{"nvidia needs amd64", ResourceModeNvidia, []string{"arm64"}, true, "amd64"},
		{"nvidia ok with amd64", ResourceModeNvidia, []string{"amd64"}, false, ""},
		{"amd-gpu needs amd64", ResourceModeAMDGPU, []string{"arm64"}, true, "amd64"},
		{"nvidia-gb10 needs arm64", ResourceModeNvidiaGB10, []string{"amd64"}, true, "arm64"},
		{"mthreads-m1000 needs arm64", ResourceModeMThreadsM1000, []string{"amd64"}, true, "arm64"},
		{"cpu has no arch constraint", ResourceModeCPU, []string{"amd64"}, false, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			c := newResourcesConfig(ResourceMode{Mode: tc.mode})
			c.Spec.SupportArch = tc.supportArch
			err := ValidateAppConfiguration(c)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tc.wantNeed) {
					t.Fatalf("error should mention required arch %q, got: %v", tc.wantNeed, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Rule 2: apiVersion=v2 needs both server and client sections, both
// declaring CPU. GPU is only mandated on modes that actually support a
// standalone gpu memory requirement.
func TestRule2_V2RequiresServerAndClient(t *testing.T) {
	c := newResourcesConfig(ResourceMode{
		Mode: ResourceModeCPU,
		ResourceRequirement: ResourceRequirement{
			RequiredCPU:    "100m",
			RequiredMemory: "200Mi",
		},
	})
	c.APIVersion = APIVersionV2
	c.Spec.SubCharts = []Chart{{Name: "main", Shared: true}}
	err := ValidateAppConfiguration(c)
	if err == nil {
		t.Fatal("expected error: server/client missing for v2")
	}
	if !strings.Contains(err.Error(), "server is required") || !strings.Contains(err.Error(), "client is required") {
		t.Fatalf("error should mention both server and client, got: %v", err)
	}
}

// fullCPUMemoryDisk returns a ResourceRequirement with every cpu/memory/
// disk field populated so that ensureSectionComplete does not trip. Tests
// add gpu fields on top when the mode is gpu-capable.
func fullCPUMemoryDisk() ResourceRequirement {
	return ResourceRequirement{
		RequiredCPU:    "100m",
		LimitedCPU:     "200m",
		RequiredMemory: "100Mi",
		LimitedMemory:  "200Mi",
		RequiredDisk:   "1Gi",
		LimitedDisk:    "2Gi",
	}
}

func TestRule2_V2NvidiaModeRequiresGPU(t *testing.T) {
	// Server declares a full cpu/memory/disk envelope but no gpu; the
	// completeness rule must demand both requiredGpu and limitedGpu on
	// nvidia (gpu-capable). Client declares a complete gpu envelope so
	// any error must originate from the server section.
	server := fullCPUMemoryDisk()
	client := fullCPUMemoryDisk()
	client.RequiredGPU = "1Gi"
	client.LimitedGPU = "1Gi"

	c := newResourcesConfig(ResourceMode{
		Mode:   ResourceModeNvidia,
		Server: &server,
		Client: &client,
	})
	c.APIVersion = APIVersionV2
	c.Spec.SubCharts = []Chart{{Name: "main", Shared: true}}
	err := ValidateAppConfiguration(c)
	if err == nil {
		t.Fatal("expected error: server section missing gpu on nvidia v2")
	}
	if !strings.Contains(err.Error(), "server.requiredGpu") ||
		!strings.Contains(err.Error(), "server.limitedGpu") {
		t.Fatalf("error should mention both server.requiredGpu and server.limitedGpu, got: %v", err)
	}
}

func TestRule2_V2NonGPUModeDoesNotRequireGPU(t *testing.T) {
	// Non-GPU-memory modes (cpu / amd-apu / apple-m / nvidia-gb10 /
	// mthreads-m1000) CANNOT declare standalone gpu fields. A fully
	// populated cpu/memory/disk envelope on both sections therefore must
	// validate cleanly under v2.
	server := fullCPUMemoryDisk()
	client := fullCPUMemoryDisk()
	c := newResourcesConfig(ResourceMode{
		Mode:   ResourceModeCPU,
		Server: &server,
		Client: &client,
	})
	c.APIVersion = APIVersionV2
	c.Spec.SubCharts = []Chart{{Name: "main", Shared: true}}
	if err := ValidateAppConfiguration(c); err != nil {
		t.Fatalf("cpu mode with complete envelope must be accepted under v2: %v", err)
	}
}

func TestRule2_V2AMDGPUModeRequiresGPU(t *testing.T) {
	// amd-gpu is in the gpu-capable whitelist, so the completeness rule
	// demands both gpu fields when any field on a section is populated.
	server := fullCPUMemoryDisk() // no gpu
	client := fullCPUMemoryDisk()
	client.RequiredGPU = "1Gi"
	client.LimitedGPU = "1Gi"

	c := newResourcesConfig(ResourceMode{
		Mode:   ResourceModeAMDGPU,
		Server: &server,
		Client: &client,
	})
	c.APIVersion = APIVersionV2
	c.Spec.SubCharts = []Chart{{Name: "main", Shared: true}}
	err := ValidateAppConfiguration(c)
	if err == nil {
		t.Fatal("expected error: server section missing gpu on amd-gpu v2")
	}
	if !strings.Contains(err.Error(), "server.requiredGpu") ||
		!strings.Contains(err.Error(), "server.limitedGpu") {
		t.Fatalf("error should mention both server.requiredGpu and server.limitedGpu, got: %v", err)
	}
}

func TestRule2_V2CPURequiresBothRequiredAndLimited(t *testing.T) {
	// The completeness rule mandates every cpu/memory/disk field on any
	// populated section. Missing a single side (required or limited)
	// must be flagged with the scoped path (server.<field> / client.<field>).
	cases := []struct {
		name   string
		server ResourceRequirement
		client ResourceRequirement
		want   string // expected substring in error
	}{
		{
			name: "server missing limitedCpu",
			server: func() ResourceRequirement {
				r := fullCPUMemoryDisk()
				r.LimitedCPU = ""
				return r
			}(),
			client: fullCPUMemoryDisk(),
			want:   "server.limitedCpu is required",
		},
		{
			name:   "client missing requiredCpu",
			server: fullCPUMemoryDisk(),
			client: func() ResourceRequirement {
				r := fullCPUMemoryDisk()
				r.RequiredCPU = ""
				return r
			}(),
			want: "client.requiredCpu is required",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			c := newResourcesConfig(ResourceMode{
				Mode:   ResourceModeCPU,
				Server: &tc.server,
				Client: &tc.client,
			})
			c.APIVersion = APIVersionV2
			c.Spec.SubCharts = []Chart{{Name: "main", Shared: true}}
			err := ValidateAppConfiguration(c)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error should mention %q, got: %v", tc.want, err)
			}
		})
	}
}

// Rule 3: modes outside the gpuMemoryModes whitelist (nvidia / amd-gpu)
// cannot declare gpu fields. Disk is allowed on every mode after the
// relaxation of the old "cpu+memory only" constraint.
func TestRule3_NonGPUFamilyModesForbidGPU(t *testing.T) {
	for _, mode := range []string{
		ResourceModeCPU, ResourceModeAMDAPU, ResourceModeAppleM, ResourceModeNvidiaGB10, ResourceModeMThreadsM1000,
	} {
		mode := mode
		t.Run(mode+"_disk_allowed", func(t *testing.T) {
			// Disk used to be rejected for these modes. It is now accepted
			// alongside cpu/memory.
			c := newResourcesConfig(ResourceMode{
				Mode: mode,
				ResourceRequirement: ResourceRequirement{
					RequiredCPU:  "100m",
					RequiredDisk: "1Gi",
					LimitedDisk:  "2Gi",
				},
			})
			if err := ValidateAppConfiguration(c); err != nil {
				if strings.Contains(err.Error(), "requiredDisk") ||
					strings.Contains(err.Error(), "limitedDisk") {
					t.Fatalf("disk must be allowed for mode=%s, got: %v", mode, err)
				}
			}
		})
		t.Run(mode+"_gpu_forbidden", func(t *testing.T) {
			c := newResourcesConfig(ResourceMode{
				Mode: mode,
				ResourceRequirement: ResourceRequirement{
					RequiredCPU: "100m",
					RequiredGPU: "1",
				},
			})
			err := ValidateAppConfiguration(c)
			if err == nil {
				t.Fatalf("expected error: gpu not allowed for mode=%s", mode)
			}
			if !strings.Contains(err.Error(), "requiredGpu") {
				t.Fatalf("error should mention requiredGpu, got: %v", err)
			}
		})
	}
}

// Rule 4: nvidia and amd-gpu may declare a standalone gpu memory quantity;
// every other mode must leave requiredGpu/limitedGpu empty.
func TestRule4_GPUMemoryAllowedModes(t *testing.T) {
	cases := []struct {
		mode    string
		wantErr bool
	}{
		{ResourceModeNvidia, false},
		{ResourceModeAMDGPU, false},
		{ResourceModeCPU, true},           // non-GPU family
		{ResourceModeMThreadsM1000, true}, // GPU-family but not yet whitelisted
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.mode, func(t *testing.T) {
			rr := fullCPUMemoryDisk()
			rr.RequiredGPU = "8Gi"
			rr.LimitedGPU = "8Gi"
			c := newResourcesConfig(ResourceMode{
				Mode:                tc.mode,
				ResourceRequirement: rr,
			})
			err := ValidateAppConfiguration(c)
			if tc.wantErr && err == nil {
				t.Fatalf("mode=%s: expected error for gpu declaration, got nil", tc.mode)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("mode=%s: unexpected error: %v", tc.mode, err)
			}
		})
	}
}

// Rule 5: limit >= required within the same dimension. Error cases only
// need to populate enough fields to trigger the comparison; completeness
// will also complain, but the assertion only checks for ANY error. The
// passing cases use the full envelope so they do not trip completeness.
func TestRule5_LimitGERequired(t *testing.T) {
	full := fullCPUMemoryDisk()
	flippedCPU := full
	flippedCPU.RequiredCPU = "200m"
	flippedCPU.LimitedCPU = "100m"

	flippedMemory := full
	flippedMemory.RequiredMemory = "200Mi"
	flippedMemory.LimitedMemory = "100Mi"

	cases := []struct {
		name string
		rr   ResourceRequirement
		want bool // true => expect error
	}{
		{"cpu limit < required", flippedCPU, true},
		{"memory limit < required", flippedMemory, true},
		{"limit >= required is fine", full, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			c := newResourcesConfig(ResourceMode{
				Mode:                ResourceModeCPU,
				ResourceRequirement: tc.rr,
			})
			err := ValidateAppConfiguration(c)
			if tc.want && err == nil {
				t.Fatal("expected error")
			}
			if !tc.want && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// The version gate: legacy manifests must be left alone.
func TestResources_VersionGateBelowThreshold(t *testing.T) {
	c := newResourcesConfig(ResourceMode{Mode: "completely-bogus"})
	c.ConfigVersion = "0.11.0" // below the gate
	if err := ValidateAppConfiguration(c); err != nil {
		// Mode validation runs via ValidateAppConfiguration -> ValidateResourceMode.
		// which is a per-element rule, BUT the cross-field checkResources is gated.
		// We accept either outcome here as long as the cross-field arch rule
		// (which we'd otherwise expect to trigger) is NOT in the error.
		if strings.Contains(err.Error(), "supportArch") {
			t.Fatalf("cross-field arch rule should be skipped below the gate, got: %v", err)
		}
	}
}

func TestRule6_InlineAndServerMutuallyExclusive(t *testing.T) {
	c := newResourcesConfig(ResourceMode{
		Mode: ResourceModeCPU,
		ResourceRequirement: ResourceRequirement{
			RequiredCPU: "100m",
		},
		Server: &ResourceRequirement{
			RequiredCPU: "200m",
		},
	})
	err := ValidateAppConfiguration(c)
	if err == nil {
		t.Fatal("expected error: inline resources must be empty when server is set")
	}
	if !strings.Contains(err.Error(), "inline resource requirements must be empty") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRule6_InlineAndClientMutuallyExclusive(t *testing.T) {
	c := newResourcesConfig(ResourceMode{
		Mode: ResourceModeCPU,
		ResourceRequirement: ResourceRequirement{
			RequiredMemory: "64Mi",
		},
		Client: &ResourceRequirement{
			RequiredMemory: "32Mi",
		},
	})
	err := ValidateAppConfiguration(c)
	if err == nil {
		t.Fatal("expected error: inline resources must be empty when client is set")
	}
	if !strings.Contains(err.Error(), "inline resource requirements must be empty") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestRule6_EmptyServerSectionDoesNotTrigger(t *testing.T) {
	// A stray `server: {}` (non-nil but empty) should NOT force the inline
	// block to go empty — there's no actual data being declared on the
	// server side.
	c := newResourcesConfig(ResourceMode{
		Mode: ResourceModeCPU,
		ResourceRequirement: ResourceRequirement{
			RequiredCPU: "100m",
		},
		Server: &ResourceRequirement{}, // non-nil but empty
	})
	if err := ValidateAppConfiguration(c); err != nil {
		if strings.Contains(err.Error(), "inline resource requirements must be empty") {
			t.Fatalf("empty server should not trip rule 6, got: %v", err)
		}
	}
}

func TestRule6_SplitOnlyIsValid(t *testing.T) {
	// Server/client declared, inline empty — the canonical "split" form
	// must pass rule 6.
	c := newResourcesConfig(ResourceMode{
		Mode:   ResourceModeCPU,
		Server: &ResourceRequirement{RequiredCPU: "200m", RequiredMemory: "64Mi"},
		Client: &ResourceRequirement{RequiredCPU: "100m", RequiredMemory: "32Mi"},
	})
	if err := ValidateAppConfiguration(c); err != nil {
		if strings.Contains(err.Error(), "inline resource requirements must be empty") {
			t.Fatalf("split-only form must not trip rule 6, got: %v", err)
		}
	}
}

// The completeness rule demands every cpu/memory/disk field on any
// section that is populated at all. Each test case flips one field to
// empty on a fully-populated inline section and expects that exact field
// to show up in the error.
func TestSectionComplete_InlineMissingFieldReported(t *testing.T) {
	fields := []string{
		"requiredCpu", "limitedCpu",
		"requiredMemory", "limitedMemory",
		"requiredDisk", "limitedDisk",
	}
	mutators := map[string]func(*ResourceRequirement){
		"requiredCpu":    func(r *ResourceRequirement) { r.RequiredCPU = "" },
		"limitedCpu":     func(r *ResourceRequirement) { r.LimitedCPU = "" },
		"requiredMemory": func(r *ResourceRequirement) { r.RequiredMemory = "" },
		"limitedMemory":  func(r *ResourceRequirement) { r.LimitedMemory = "" },
		"requiredDisk":   func(r *ResourceRequirement) { r.RequiredDisk = "" },
		"limitedDisk":    func(r *ResourceRequirement) { r.LimitedDisk = "" },
	}
	for _, field := range fields {
		field := field
		t.Run("inline_missing_"+field, func(t *testing.T) {
			rr := fullCPUMemoryDisk()
			mutators[field](&rr)
			c := newResourcesConfig(ResourceMode{
				Mode:                ResourceModeCPU,
				ResourceRequirement: rr,
			})
			err := ValidateAppConfiguration(c)
			if err == nil {
				t.Fatalf("expected error: inline section missing %s", field)
			}
			if !strings.Contains(err.Error(), field+" is required to declare a complete resource envelope") {
				t.Fatalf("error should mention %s, got: %v", field, err)
			}
		})
	}
}

// Empty sections are exempt — an app that does not declare any resource
// limits on a mode is still a valid config (rule 6 even treats `server: {}`
// as "not declared").
func TestSectionComplete_EmptySectionExempt(t *testing.T) {
	c := newResourcesConfig(ResourceMode{Mode: ResourceModeCPU})
	if err := ValidateAppConfiguration(c); err != nil {
		t.Fatalf("empty section must not trip completeness, got: %v", err)
	}
}

// GPU-capable modes (nvidia / amd-gpu) must declare both gpu sides once
// any field on the section is populated. Only requiredGpu with no
// limitedGpu (or vice versa) is rejected.
func TestSectionComplete_GPUModePairsGPU(t *testing.T) {
	cases := []struct {
		name      string
		rr        ResourceRequirement
		wantField string
	}{
		{
			name: "missing limitedGpu",
			rr: func() ResourceRequirement {
				r := fullCPUMemoryDisk()
				r.RequiredGPU = "8Gi"
				return r
			}(),
			wantField: "limitedGpu",
		},
		{
			name: "missing requiredGpu",
			rr: func() ResourceRequirement {
				r := fullCPUMemoryDisk()
				r.LimitedGPU = "8Gi"
				return r
			}(),
			wantField: "requiredGpu",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			c := newResourcesConfig(ResourceMode{
				Mode:                ResourceModeNvidia,
				ResourceRequirement: tc.rr,
			})
			err := ValidateAppConfiguration(c)
			if err == nil {
				t.Fatalf("expected error: nvidia must pair %s", tc.wantField)
			}
			if !strings.Contains(err.Error(), tc.wantField+" is required to declare a complete resource envelope") {
				t.Fatalf("error should mention %s, got: %v", tc.wantField, err)
			}
		})
	}
}

// GPU pair-consistency also applies inside server/client sections, not
// just inline. An amd-gpu server with only requiredGpu must be rejected.
func TestSectionComplete_GPUPairingInServerSection(t *testing.T) {
	server := fullCPUMemoryDisk()
	server.RequiredGPU = "4Gi"
	// limitedGpu intentionally missing
	c := newResourcesConfig(ResourceMode{
		Mode:   ResourceModeAMDGPU,
		Server: &server,
	})
	err := ValidateAppConfiguration(c)
	if err == nil {
		t.Fatal("expected error: amd-gpu server must pair requiredGpu/limitedGpu")
	}
	if !strings.Contains(err.Error(), "server.limitedGpu is required to declare a complete resource envelope") {
		t.Fatalf("error should mention server.limitedGpu, got: %v", err)
	}
}

// Rule 7: the legacy flat spec.required*/spec.limited* fields must be
// rejected when olaresManifest.version >= 0.12.0. Every field is tested
// individually so a regression on any single setter is caught.
func TestRule7_LegacySpecResourceFieldsRejected(t *testing.T) {
	cases := []struct {
		name  string
		apply func(*AppSpec)
		want  string
	}{
		{"requiredCpu", func(s *AppSpec) { s.RequiredCPU = "100m" }, "spec.requiredCpu"},
		{"limitedCpu", func(s *AppSpec) { s.LimitedCPU = "200m" }, "spec.limitedCpu"},
		{"requiredMemory", func(s *AppSpec) { s.RequiredMemory = "256Mi" }, "spec.requiredMemory"},
		{"limitedMemory", func(s *AppSpec) { s.LimitedMemory = "512Mi" }, "spec.limitedMemory"},
		{"requiredDisk", func(s *AppSpec) { s.RequiredDisk = "1Gi" }, "spec.requiredDisk"},
		{"limitedDisk", func(s *AppSpec) { s.LimitedDisk = "2Gi" }, "spec.limitedDisk"},
		{"requiredGpu", func(s *AppSpec) { s.RequiredGPU = "1" }, "spec.requiredGpu"},
		{"limitedGpu", func(s *AppSpec) { s.LimitedGPU = "2" }, "spec.limitedGpu"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			c := newResourcesConfig()
			tc.apply(&c.Spec)
			err := ValidateAppConfiguration(c)
			if err == nil {
				t.Fatalf("expected error: %s must be empty for olaresManifest.version >= 0.12.0", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want+" must be empty for olaresManifest.version >= 0.12.0") {
				t.Fatalf("error should mention %s, got: %v", tc.want, err)
			}
		})
	}
}

// The legacy-field prohibition is version-gated. Pre-0.12.0 manifests must
// still be allowed to carry the flat quantities.
func TestRule7_LegacyFieldsAllowedBelowGate(t *testing.T) {
	c := newResourcesConfig()
	c.ConfigVersion = "0.11.0"
	c.Spec.RequiredCPU = "100m"
	c.Spec.LimitedMemory = "512Mi"
	if err := ValidateAppConfiguration(c); err != nil {
		if strings.Contains(err.Error(), "must be empty for olaresManifest.version") {
			t.Fatalf("legacy flat fields must be permitted below the gate, got: %v", err)
		}
	}
}

// All eight prohibited fields populated simultaneously must be reported in
// one shot — errors.Join lets callers see the full picture instead of
// fixing them one at a time.
func TestRule7_LegacyFieldsAggregated(t *testing.T) {
	c := newResourcesConfig()
	c.Spec.RequiredCPU = "100m"
	c.Spec.LimitedCPU = "200m"
	c.Spec.RequiredMemory = "256Mi"
	c.Spec.LimitedMemory = "512Mi"
	c.Spec.RequiredDisk = "1Gi"
	c.Spec.LimitedDisk = "2Gi"
	c.Spec.RequiredGPU = "1"
	c.Spec.LimitedGPU = "2"
	err := ValidateAppConfiguration(c)
	if err == nil {
		t.Fatal("expected aggregated error listing every forbidden field")
	}
	msg := err.Error()
	for _, field := range []string{
		"spec.requiredCpu", "spec.limitedCpu",
		"spec.requiredMemory", "spec.limitedMemory",
		"spec.requiredDisk", "spec.limitedDisk",
		"spec.requiredGpu", "spec.limitedGpu",
	} {
		if !strings.Contains(msg, field) {
			t.Fatalf("error should mention %s, got: %v", field, err)
		}
	}
}

func TestResources_ServerSectionRulesEnforced(t *testing.T) {
	// Rule 3 (gpu forbidden for non-nvidia) and rule 5 should also consider
	// the server section, not just the inline section. We use gpu here
	// because disk is now legal on every mode.
	c := newResourcesConfig(ResourceMode{
		Mode: ResourceModeCPU,
		Server: &ResourceRequirement{
			RequiredCPU: "100m",
			RequiredGPU: "1",
		},
	})
	err := ValidateAppConfiguration(c)
	if err == nil {
		t.Fatal("expected error: server.requiredGpu forbidden under cpu mode")
	}
	if !strings.Contains(err.Error(), "server.requiredGpu") {
		t.Fatalf("error should be scoped to server section, got: %v", err)
	}
}
