package manifest

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	ResourceModeCPU           = "cpu"
	ResourceModeAMDAPU        = "amd-apu"
	ResourceModeAMDGPU        = "amd-gpu"
	ResourceModeAppleM        = "apple-m"
	ResourceModeNvidia        = "nvidia"
	ResourceModeNvidiaGB10    = "nvidia-gb10"
	ResourceModeMThreadsM1000 = "mthreads-m1000"
)

var validResourceModes = []any{
	ResourceModeCPU,
	ResourceModeAMDAPU,
	ResourceModeAMDGPU,
	ResourceModeAppleM,
	ResourceModeNvidia,
	ResourceModeNvidiaGB10,
	ResourceModeMThreadsM1000,
}

var modeArchRequirement = map[string]string{
	ResourceModeAMDGPU:        "amd64",
	ResourceModeNvidia:        "amd64",
	ResourceModeNvidiaGB10:    "arm64",
	ResourceModeMThreadsM1000: "arm64",
}

var gpuMemoryModes = map[string]struct{}{
	ResourceModeNvidia: {},
	ResourceModeAMDGPU: {},
}

var minResourcesManifestVersion = semver.MustParse("0.12.0")

func ValidateResourceMode(r ResourceMode) error {
	modeInvalid := validation.ValidateStruct(&r,
		validation.Field(&r.Mode,
			validation.Required.Error("resources[].mode is required"),
			validation.In(validResourceModes...).
				Error(fmt.Sprintf("resources[].mode must be one of %s", joinModes(validResourceModes))),
		),
	)
	if modeInvalid != nil {
		return modeInvalid
	}

	_, gpuAllowed := gpuMemoryModes[r.Mode]
	var errs []error
	for _, sec := range sectionsOfResourceMode(r) {
		if err := validateQuantities(sec.path, sec.rr); err != nil {
			errs = append(errs, err)
		}
		if err := ensureSectionComplete(sec.path, r.Mode, sec.rr, gpuAllowed); err != nil {
			errs = append(errs, err)
		}
		if !gpuAllowed {
			if err := ensureNoGPUSection(sec.path, r.Mode, sec.rr); err != nil {
				errs = append(errs, err)
			}
		}
		if err := checkLimitGERequired(sec.path, sec.rr); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type section struct {
	path string
	rr   ResourceRequirement
}

func sectionsOfResourceMode(r ResourceMode) []section {
	out := []section{{path: "", rr: r.ResourceRequirement}}
	if r.Server != nil {
		out = append(out, section{path: "server.", rr: *r.Server})
	}
	if r.Client != nil {
		out = append(out, section{path: "client.", rr: *r.Client})
	}
	return out
}

func checkSpecResources(cfg *AppConfiguration) error {
	if !resourcesCheckApplies(cfg.ConfigVersion) {
		return nil
	}

	var errs []error
	if err := ensureNoLegacySpecResourceFields(&cfg.Spec); err != nil {
		errs = append(errs, err)
	}

	supportArch := make(map[string]struct{}, len(cfg.Spec.SupportArch))
	for _, a := range cfg.Spec.SupportArch {
		supportArch[a] = struct{}{}
	}

	for i, rm := range cfg.Spec.Resources {
		path := fmt.Sprintf("spec.resources[%d]", i)

		if required, ok := modeArchRequirement[rm.Mode]; ok {
			if _, has := supportArch[required]; !has {
				errs = append(errs, fmt.Errorf(
					"%s: mode=%s requires spec.supportArch to contain %q",
					path, rm.Mode, required,
				))
			}
		}

		if cfg.APIVersion == APIVersionV2 {
			if rm.Server == nil {
				errs = append(errs, fmt.Errorf("%s.server is required for apiVersion=v2", path))
			}
			if rm.Client == nil {
				errs = append(errs, fmt.Errorf("%s.client is required for apiVersion=v2", path))
			}
		}

		serverPopulated := rm.Server != nil && hasAnyQuantity(*rm.Server)
		clientPopulated := rm.Client != nil && hasAnyQuantity(*rm.Client)
		if (serverPopulated || clientPopulated) && hasAnyQuantity(rm.ResourceRequirement) {
			errs = append(errs, fmt.Errorf(
				"%s: inline resource requirements must be empty when server or client is declared",
				path,
			))
		}
	}

	return errors.Join(errs...)
}

func hasAnyQuantity(rr ResourceRequirement) bool {
	return rr.RequiredCPU != "" || rr.LimitedCPU != "" ||
		rr.RequiredMemory != "" || rr.LimitedMemory != "" ||
		rr.RequiredDisk != "" || rr.LimitedDisk != "" ||
		rr.RequiredGPU != "" || rr.LimitedGPU != ""
}

var legacySpecResourceFields = [...]struct {
	name string
	get  func(*AppSpec) string
}{
	{"spec.requiredCpu", func(s *AppSpec) string { return s.RequiredCPU }},
	{"spec.limitedCpu", func(s *AppSpec) string { return s.LimitedCPU }},
	{"spec.requiredMemory", func(s *AppSpec) string { return s.RequiredMemory }},
	{"spec.limitedMemory", func(s *AppSpec) string { return s.LimitedMemory }},
	{"spec.requiredDisk", func(s *AppSpec) string { return s.RequiredDisk }},
	{"spec.limitedDisk", func(s *AppSpec) string { return s.LimitedDisk }},
	{"spec.requiredGpu", func(s *AppSpec) string { return s.RequiredGPU }},
	{"spec.limitedGpu", func(s *AppSpec) string { return s.LimitedGPU }},
}

func ensureNoLegacySpecResourceFields(spec *AppSpec) error {
	var errs []error
	for _, f := range legacySpecResourceFields {
		if f.get(spec) != "" {
			errs = append(errs, fmt.Errorf(
				"%s must be empty for olaresManifest.version >= 0.12.0; move it under spec.resources[]",
				f.name,
			))
		}
	}
	return errors.Join(errs...)
}

func resourcesCheckApplies(v string) bool {
	got, err := semver.NewVersion(v)
	if err != nil {
		return false
	}
	return !got.LessThan(minResourcesManifestVersion)
}

func IsModernResourcesManifest(version string) bool {
	return resourcesCheckApplies(version)
}

// ResourceRequirementLimits holds required/limited CPU, memory, disk, and GPU
// quantities as unparsed Kubernetes quantity strings.
type ResourceRequirementLimits struct {
	RequiredCPU    string
	LimitedCPU     string
	RequiredMemory string
	LimitedMemory  string
	RequiredDisk   string
	LimitedDisk    string
	RequiredGPU    string
	LimitedGPU     string
}

func limitsFromRRPtr(rr *ResourceRequirement) ResourceRequirementLimits {
	if rr == nil {
		return ResourceRequirementLimits{}
	}
	return ResourceRequirementLimits{
		RequiredCPU:    rr.RequiredCPU,
		LimitedCPU:     rr.LimitedCPU,
		RequiredMemory: rr.RequiredMemory,
		LimitedMemory:  rr.LimitedMemory,
		RequiredDisk:   rr.RequiredDisk,
		LimitedDisk:    rr.LimitedDisk,
		RequiredGPU:    rr.RequiredGPU,
		LimitedGPU:     rr.LimitedGPU,
	}
}

func limitsFromEmbedded(rr ResourceRequirement) ResourceRequirementLimits {
	return limitsFromRRPtr(&rr)
}

// ResourceModeFullCoreLimits returns inline resource fields when the mode row
// carries inline quantities; otherwise it sums server and client per dimension
// using Kubernetes quantity arithmetic (same idea as ResourceModeCoreLimits
// for CPU/memory, extended to disk and GPU).
func ResourceModeFullCoreLimits(r ResourceMode) ResourceRequirementLimits {
	if hasAnyQuantity(r.ResourceRequirement) {
		return limitsFromEmbedded(r.ResourceRequirement)
	}
	var rc, lc, rm, lm, rd, ld, rg, lg resource.Quantity
	for _, sec := range []*ResourceRequirement{r.Server, r.Client} {
		if sec == nil {
			continue
		}
		addQuantity(&rc, sec.RequiredCPU)
		addQuantity(&lc, sec.LimitedCPU)
		addQuantity(&rm, sec.RequiredMemory)
		addQuantity(&lm, sec.LimitedMemory)
		addQuantity(&rd, sec.RequiredDisk)
		addQuantity(&ld, sec.LimitedDisk)
		addQuantity(&rg, sec.RequiredGPU)
		addQuantity(&lg, sec.LimitedGPU)
	}
	return ResourceRequirementLimits{
		RequiredCPU:    rc.String(),
		LimitedCPU:     lc.String(),
		RequiredMemory: rm.String(),
		LimitedMemory:  lm.String(),
		RequiredDisk:   rd.String(),
		LimitedDisk:    ld.String(),
		RequiredGPU:    rg.String(),
		LimitedGPU:     lg.String(),
	}
}

func ResourceModeCoreLimits(r ResourceMode) (requiredCPU, requiredMemory, limitedCPU, limitedMemory string) {
	f := ResourceModeFullCoreLimits(r)
	return f.RequiredCPU, f.RequiredMemory, f.LimitedCPU, f.LimitedMemory
}

func normalizeInstallProfileForLimits(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("installProfile is required (use %q or %q)",
			InstallOrUpgradeClientOnly, InstallOrUpgradeServerAndClient)
	}
	if strings.EqualFold(s, InstallOrUpgradeClientOnly) {
		return InstallOrUpgradeClientOnly, nil
	}
	if strings.EqualFold(s, InstallOrUpgradeServerAndClient) {
		return InstallOrUpgradeServerAndClient, nil
	}
	return "", fmt.Errorf("installProfile must be %q or %q, got %q",
		InstallOrUpgradeClientOnly, InstallOrUpgradeServerAndClient, s)
}

// ResourceLimitsForInstallProfile returns required/limited CPU, memory, disk,
// and GPU for one spec.resources[] row. Dispatch matches chart resource-limit
// checks:
//   - apiVersion v1 (or empty): returns the inline ResourceRequirement
//     verbatim (empty fields collapse to ""), regardless of installProfile.
//   - apiVersion v2 with inline quantities: returns the inline envelope and
//     ignores installProfile (Rule 6 already makes inline and server/client
//     mutually exclusive).
//   - apiVersion v2 with server/client splits: InstallOrUpgradeClientOnly
//     returns the client section only; InstallOrUpgradeServerAndClient
//     returns the k8s-quantity sum of server and client per dimension
//     (ResourceModeFullCoreLimits).
//
// installProfile is still normalised first, so callers cannot silently pass
// garbage values even on v1.
func ResourceLimitsForInstallProfile(rm ResourceMode, apiVersion, installProfile string) (ResourceRequirementLimits, error) {
	prof, err := normalizeInstallProfileForLimits(installProfile)
	if err != nil {
		return ResourceRequirementLimits{}, err
	}
	if strings.EqualFold(apiVersion, APIVersionV1) || apiVersion == "" {
		return limitsFromEmbedded(rm.ResourceRequirement), nil
	}
	if !strings.EqualFold(apiVersion, APIVersionV2) {
		return ResourceRequirementLimits{}, fmt.Errorf("unsupported apiVersion %q", apiVersion)
	}
	if hasAnyQuantity(rm.ResourceRequirement) {
		return limitsFromEmbedded(rm.ResourceRequirement), nil
	}
	if prof == InstallOrUpgradeClientOnly {
		if rm.Client == nil {
			return ResourceRequirementLimits{}, fmt.Errorf("client section is required for installProfile=%q", prof)
		}
		return limitsFromRRPtr(rm.Client), nil
	}
	return ResourceModeFullCoreLimits(rm), nil
}

// ResourceCPUMemLimitsForInstallProfile returns only the CPU/memory subset of
// ResourceLimitsForInstallProfile for callers that match CheckResourceLimits.
func ResourceCPUMemLimitsForInstallProfile(rm ResourceMode, apiVersion, installProfile string) (requiredCPU, requiredMemory, limitedCPU, limitedMemory string, err error) {
	full, err := ResourceLimitsForInstallProfile(rm, apiVersion, installProfile)
	if err != nil {
		return "", "", "", "", err
	}
	return full.RequiredCPU, full.RequiredMemory, full.LimitedCPU, full.LimitedMemory, nil
}

func addQuantity(dst *resource.Quantity, s string) {
	if s == "" {
		return
	}
	parsed, err := resource.ParseQuantity(s)
	if err != nil {
		return
	}
	dst.Add(parsed)
}

func ensureSectionComplete(sectionPath, mode string, rr ResourceRequirement, gpuAllowed bool) error {
	if !hasAnyQuantity(rr) {
		return nil
	}
	pairs := []struct {
		field string
		value string
	}{
		{"requiredCpu", rr.RequiredCPU},
		{"limitedCpu", rr.LimitedCPU},
		{"requiredMemory", rr.RequiredMemory},
		{"limitedMemory", rr.LimitedMemory},
		{"requiredDisk", rr.RequiredDisk},
		{"limitedDisk", rr.LimitedDisk},
	}
	if gpuAllowed {
		pairs = append(pairs,
			struct {
				field string
				value string
			}{"requiredGpu", rr.RequiredGPU},
			struct {
				field string
				value string
			}{"limitedGpu", rr.LimitedGPU},
		)
	}
	var errs []error
	for _, p := range pairs {
		if p.value != "" {
			continue
		}
		errs = append(errs, fmt.Errorf(
			"%s%s is required to declare a complete resource envelope (mode=%s)",
			sectionPath, p.field, mode,
		))
	}
	return errors.Join(errs...)
}

func ensureNoGPUSection(sectionPath, mode string, rr ResourceRequirement) error {
	var errs []error
	pairs := []struct {
		field string
		value string
	}{
		{"requiredGpu", rr.RequiredGPU},
		{"limitedGpu", rr.LimitedGPU},
	}
	for _, p := range pairs {
		if p.value == "" {
			continue
		}
		errs = append(errs, fmt.Errorf(
			"%s%s must be empty for mode=%s (only %s support a standalone gpu memory requirement)",
			sectionPath, p.field, mode, joinGPUMemoryModes(),
		))
	}
	return errors.Join(errs...)
}

func validateQuantities(prefix string, rr ResourceRequirement) error {
	pairs := []struct {
		field string
		value string
	}{
		{"requiredCpu", rr.RequiredCPU},
		{"limitedCpu", rr.LimitedCPU},
		{"requiredMemory", rr.RequiredMemory},
		{"limitedMemory", rr.LimitedMemory},
		{"requiredDisk", rr.RequiredDisk},
		{"limitedDisk", rr.LimitedDisk},
		{"requiredGpu", rr.RequiredGPU},
		{"limitedGpu", rr.LimitedGPU},
	}
	var errs []error
	for _, p := range pairs {
		if p.value == "" {
			continue
		}
		if !k8sQuantity.MatchString(p.value) {
			errs = append(errs, fmt.Errorf("%s%s must be a valid Kubernetes quantity (got %q)", prefix, p.field, p.value))
		}
	}
	return errors.Join(errs...)
}

func checkLimitGERequired(prefix string, rr ResourceRequirement) error {
	dims := []struct {
		name     string
		required string
		limited  string
	}{
		{"cpu", rr.RequiredCPU, rr.LimitedCPU},
		{"memory", rr.RequiredMemory, rr.LimitedMemory},
		{"disk", rr.RequiredDisk, rr.LimitedDisk},
		{"gpu", rr.RequiredGPU, rr.LimitedGPU},
	}
	var errs []error
	for _, d := range dims {
		if d.required == "" || d.limited == "" {
			continue
		}
		req, err := resource.ParseQuantity(d.required)
		if err != nil {
			continue
		}
		lim, err := resource.ParseQuantity(d.limited)
		if err != nil {
			continue
		}
		if lim.Cmp(req) < 0 {
			errs = append(errs, fmt.Errorf(
				"%slimited%s (%s) must be >= required%s (%s)",
				prefix, capFirst(d.name), d.limited, capFirst(d.name), d.required,
			))
		}
	}
	return errors.Join(errs...)
}

func capFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func joinModes(modes []any) string {
	parts := make([]string, 0, len(modes))
	for _, m := range modes {
		parts = append(parts, fmt.Sprintf("%q", m))
	}
	return strings.Join(parts, ", ")
}

func joinGPUMemoryModes() string {
	names := make([]string, 0, len(gpuMemoryModes))
	for m := range gpuMemoryModes {
		names = append(names, fmt.Sprintf("mode=%s", m))
	}
	sort.Strings(names)
	return strings.Join(names, " / ")
}
