package oac

import (
	"fmt"
	"strings"

	"github.com/beclab/Olares/framework/oac/internal/manifest"
)

// ManifestResourceLimits is the full resource envelope (CPU, memory, disk, GPU
// required/limited pairs) for one spec.resources[] mode row under a given
// install profile.
type ManifestResourceLimits = manifest.ResourceRequirementLimits

const (
	// InstallProfileClientOnly matches manifest.InstallOrUpgradeClientOnly ("clientOnly").
	InstallProfileClientOnly = manifest.InstallOrUpgradeClientOnly
	// InstallProfileClientAndServer matches manifest.InstallOrUpgradeServerAndClient ("clientAndServer").
	InstallProfileClientAndServer = manifest.InstallOrUpgradeServerAndClient
)

// ResourceLimitsForResourceMode returns required/limited CPU, memory, disk,
// and GPU for the spec.resources[] element whose mode matches
// (case-insensitive). installProfile must be InstallProfileClientOnly or
// InstallProfileClientAndServer (or the same strings from
// github.com/beclab/api/manifest).
//
// Dispatch:
//   - apiVersion v1 (or empty): returns the inline ResourceRequirement
//     verbatim; installProfile is still validated but otherwise ignored.
//   - apiVersion v2 with inline quantities: returns the inline envelope.
//   - apiVersion v2 with server/client splits: clientOnly returns the
//     client section; clientAndServer returns the k8s-quantity sum of
//     server and client per dimension (ResourceModeFullCoreLimits).
func ResourceLimitsForResourceMode(cfg *AppConfiguration, mode, installProfile string) (ManifestResourceLimits, error) {
	if cfg == nil {
		return ManifestResourceLimits{}, fmt.Errorf("oac: AppConfiguration is nil")
	}
	for i := range cfg.Spec.Resources {
		rm := &cfg.Spec.Resources[i]
		if strings.EqualFold(rm.Mode, mode) {
			return manifest.ResourceLimitsForInstallProfile(*rm, cfg.APIVersion, installProfile)
		}
	}
	return ManifestResourceLimits{}, fmt.Errorf("oac: no spec.resources entry with mode %q", mode)
}
