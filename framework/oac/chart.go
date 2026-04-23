package oac

import (
	"errors"
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/kube"

	"github.com/beclab/Olares/framework/oac/internal/chartfolder"
	"github.com/beclab/Olares/framework/oac/internal/helmrender"
	"github.com/beclab/Olares/framework/oac/internal/manifest"
	"github.com/beclab/Olares/framework/oac/internal/resources"
)

// Lint runs the full lint pipeline against oacPath. The exact set of checks
// executed depends on the Skip* options set on the Checker:
//
//  1. Folder layout (chartfolder.CheckLayout) - skipped by SkipFolderCheck
//  2. Manifest parse + ozzo validation + custom validators -
//     skipped by SkipManifestCheck
//  3. Helm dry-run and mandatory workload-integrity checks (upload mount
//     path, `type=app` workload naming) - ALWAYS run; not governed by any
//     Skip* option
//  4. Container-level resource limits check - skipped by SkipResourceCheck
//  5. Chart.yaml <-> manifest same-version check - ON by default, turn off
//     with SkipSameVersionCheck()
//  6. ServiceAccount RBAC inspection - OFF by default, turn on with
//     WithServiceAccountRulesCheck()
//
// When WithAutoOwnerScenarios() is set, step 3/4/6 — the portions that
// depend on the rendered chart — run twice: once with owner == admin and
// once with owner != admin. All other steps run once.
func (c *OAC) Lint(oacPath string) error {
	if !c.skipFolder {
		if err := c.CheckChartFolder(oacPath); err != nil {
			return err
		}
	}

	raw, err := readManifestFile(oacPath)
	if err != nil {
		return err
	}
	m, err := c.parseManifest(raw)
	if err != nil {
		return err
	}

	if !c.skipManifest {
		if err := c.validateManifestBytes(raw, m); err != nil {
			return err
		}
	}

	for _, v := range c.customValidators {
		if err := v(oacPath, m); err != nil {
			return err
		}
	}

	for _, sc := range c.ownerScenarios() {
		if err := c.lintRenderedScenario(oacPath, m, sc); err != nil {
			if sc.label != "" {
				return fmt.Errorf("%s scenario: %w", sc.label, err)
			}
			return err
		}
	}

	if !c.skipSameVersion {
		if err := c.CheckSameVersion(oacPath, m); err != nil {
			return err
		}
	}
	return nil
}

// ownerScenario captures a single (owner, admin) pair that the chart must
// render cleanly under. The label is surfaced as a prefix on any returned
// error so a user running both scenarios can tell which one tripped.
type ownerScenario struct {
	label string
	owner string
	admin string
}

// ownerScenarios returns the list of (owner, admin) combinations to run the
// rendered portion of Lint against. It collapses to a single entry using the
// Checker's configured owner/admin unless WithAutoOwnerScenarios() asked for
// both install modes.
func (c *OAC) ownerScenarios() []ownerScenario {
	if c.autoOwner {
		return []ownerScenario{
			{label: "owner==admin", owner: "admin", admin: "admin"},
			{label: "owner!=admin", owner: "owner", admin: "admin"},
		}
	}
	return []ownerScenario{{owner: c.owner, admin: c.admin}}
}

// lintRenderedScenario performs the helm-render-dependent part of Lint for a
// single owner scenario: render, mandatory structural checks, and the
// optional resource/RBAC inspections. Manifest validation and folder layout
// are owner-independent and live on the outer Lint call.
func (c *OAC) lintRenderedScenario(oacPath string, m Manifest, sc ownerScenario) error {
	values := helmrender.BuildValues(sc.owner, sc.admin, m.Entrances())
	helmrender.ApplyDefaultInstallProfile(values, m.APIVersion())
	list, err := helmrender.Render(oacPath, values, m.AppName())
	if err != nil {
		return fmt.Errorf("helm render: %w", err)
	}

	// Mandatory workload-integrity checks — not gated by SkipResourceCheck.
	if err := resources.CheckUploadConfig(list, extractUploadDest(m)); err != nil {
		return err
	}
	if err := resources.CheckDeploymentName(list, m.ConfigType(), m.AppName()); err != nil {
		return err
	}

	if !c.skipResource {
		if err := c.checkResourceLimits(oacPath, m, sc, list); err != nil {
			return err
		}
	}

	if c.runRBAC {
		rules, err := resources.LoadForbiddenRules("")
		if err != nil {
			return err
		}
		if err := resources.CheckServiceAccountRules(list, rules); err != nil {
			return err
		}
	}
	return nil
}

// CheckChartFolder validates that oacPath is a structurally-valid chart
// directory (Chart.yaml/values.yaml/templates/OlaresManifest.yaml present,
// folder name well-formed).
func (c *OAC) CheckChartFolder(oacPath string) error {
	return chartfolder.CheckLayout(oacPath)
}

// CheckSameVersion cross-validates the folder name, Chart.yaml metadata, and
// parsed manifest metadata. Provide nil for m to have it loaded on demand.
func (c *OAC) CheckSameVersion(oacPath string, m Manifest) error {
	chartFile, err := chartfolder.LoadChart(oacPath)
	if err != nil {
		return err
	}
	if m == nil {
		m, err = c.LoadManifestFile(oacPath)
		if err != nil {
			return err
		}
	}
	return chartfolder.CheckConsistency(oacPath, chartFile, m)
}

// CheckResources dry-runs the chart and performs the resource-list level
// checks limits. The manifest is parsed implicitly.
//
// For legacy manifests (<0.12.0) the chart is rendered once and the
// container-level limits are compared against spec.required*/spec.limited*.
// For modern manifests (>=0.12.0) the flat spec fields are forbidden by
// Rule 7, so limits come from spec.resources[]. For apiVersion=v1 each mode
// uses the inline ResourceRequirement and a render with .Values.GPU.Type
// only. For apiVersion=v2 each mode is checked twice: a .Values.client render
// against the client section and a .Values.clientAndServer render against the
// summed server+client envelope (see checkResourceLimits).
func (c *OAC) CheckResources(oacPath string) error {
	m, err := c.LoadManifestFile(oacPath)
	if err != nil {
		return err
	}
	sc := ownerScenario{owner: c.owner, admin: c.admin}
	values := helmrender.BuildValues(sc.owner, sc.admin, m.Entrances())
	helmrender.ApplyDefaultInstallProfile(values, m.APIVersion())
	list, err := helmrender.Render(oacPath, values, m.AppName())
	if err != nil {
		return err
	}
	return c.checkResourceLimits(oacPath, m, sc, list)
}

// CheckServiceAccountRules inspects Role/ClusterRole bindings in the rendered
// chart and returns an error if any of them grants the ServiceAccount one of
// the built-in forbidden permissions.
func (c *OAC) CheckServiceAccountRules(oacPath string) error {
	m, err := c.LoadManifestFile(oacPath)
	if err != nil {
		return err
	}
	values := helmrender.BuildValues(c.owner, c.admin, m.Entrances())
	helmrender.ApplyDefaultInstallProfile(values, m.APIVersion())
	list, err := helmrender.Render(oacPath, values, m.AppName())
	if err != nil {
		return err
	}
	rules, err := resources.LoadForbiddenRules("")
	if err != nil {
		return err
	}
	return resources.CheckServiceAccountRules(list, rules)
}

// ValidateManifestFile parses and validates oacPath/OlaresManifest.yaml. No
// chart rendering is performed. For legacy manifests (<0.12.0) the
// underlying pipeline re-parses the payload under both admin=owner and
// admin!=owner scenarios and aggregates any failures into a single
// ValidationError.
func (c *OAC) ValidateManifestFile(oacPath string) error {
	raw, err := readManifestFile(oacPath)
	if err != nil {
		return err
	}
	return c.ValidateManifestContent(raw)
}

// ValidateManifestContent is the byte-slice counterpart of ValidateManifestFile.
func (c *OAC) ValidateManifestContent(content []byte) error {
	m, err := c.parseManifest(content)
	if err != nil {
		return err
	}
	return c.validateManifestBytes(content, m)
}

// checkResourceLimits runs CheckResourceLimits against the right render for
// the manifest's schema version.
//
//   - Legacy (<0.12.0): the defaultList that was already rendered by the
//     caller is reused and limits come from the flat
//     spec.required*/spec.limited* fields.
//   - Modern (>=0.12.0): every entry in spec.resources[] drives dedicated
//     helm renders with .Values.GPU.Type set to rm.Mode, because chart
//     templates may emit different workloads per GPU family.
//   - apiVersion=v1: limits come from the inline fields on each mode row
//     (the embedded ResourceRequirement); the chart is rendered without
//     client/clientAndServer install flags.
//   - apiVersion=v2: for each mode the chart is rendered twice — with
//     .Values.client=true (limits from spec.resources[].client) and with
//     .Values.clientAndServer=true (limits are the sum of server and client
//     sections, matching CoreLimits on a split manifest).
//
// Charts that carry no resources[] on a modern manifest skip the check
// entirely — Rule 7 already guaranteed the legacy flat fields are empty,
// so there is nothing to compare container limits against.
func (c *OAC) checkResourceLimits(oacPath string, m Manifest, sc ownerScenario, defaultList kube.ResourceList) error {
	cfg, ok := m.Raw().(*manifest.AppConfiguration)
	if !ok || !manifest.IsModernResourcesManifest(cfg.ConfigVersion) {
		return resources.CheckResourceLimits(defaultList, extractResourceLimits(m))
	}
	if len(cfg.Spec.Resources) == 0 {
		return nil
	}
	var errs []error
	for _, rm := range cfg.Spec.Resources {
		if strings.EqualFold(cfg.APIVersion, manifest.APIVersionV2) {
			if rm.Client == nil {
				errs = append(errs, fmt.Errorf("resources mode=%s: client section is required for apiVersion=v2", rm.Mode))
				continue
			}
			vClient := helmrender.BuildValues(sc.owner, sc.admin, m.Entrances())
			helmrender.SetGPUType(vClient, rm.Mode)
			helmrender.SetClientOnlyInstall(vClient)
			listClient, err := helmrender.Render(oacPath, vClient, m.AppName())
			if err != nil {
				errs = append(errs, fmt.Errorf("helm render (resources mode=%s client): %w", rm.Mode, err))
			} else if err := resources.CheckResourceLimits(listClient, resourceLimitsFromRequirement(*rm.Client)); err != nil {
				errs = append(errs, fmt.Errorf("resources mode=%s (client render): %w", rm.Mode, err))
			}

			vBoth := helmrender.BuildValues(sc.owner, sc.admin, m.Entrances())
			helmrender.SetGPUType(vBoth, rm.Mode)
			helmrender.SetClientAndServerInstall(vBoth)
			listBoth, err := helmrender.Render(oacPath, vBoth, m.AppName())
			if err != nil {
				errs = append(errs, fmt.Errorf("helm render (resources mode=%s clientAndServer): %w", rm.Mode, err))
			} else {
				reqCPU, reqMem, limCPU, limMem := manifest.ResourceModeCoreLimits(rm)
				if err := resources.CheckResourceLimits(listBoth, resources.ResourceLimits{
					RequiredCPU:    reqCPU,
					RequiredMemory: reqMem,
					LimitedCPU:     limCPU,
					LimitedMemory:  limMem,
				}); err != nil {
					errs = append(errs, fmt.Errorf("resources mode=%s (clientAndServer render): %w", rm.Mode, err))
				}
			}
			continue
		}

		values := helmrender.BuildValues(sc.owner, sc.admin, m.Entrances())
		helmrender.SetGPUType(values, rm.Mode)
		list, err := helmrender.Render(oacPath, values, m.AppName())
		if err != nil {
			errs = append(errs, fmt.Errorf("helm render (resources mode=%s): %w", rm.Mode, err))
			continue
		}
		limits := resourceLimitsFromRequirement(rm.ResourceRequirement)
		if err := resources.CheckResourceLimits(list, limits); err != nil {
			errs = append(errs, fmt.Errorf("resources mode=%s: %w", rm.Mode, err))
		}
	}
	return errors.Join(errs...)
}

func resourceLimitsFromRequirement(rr manifest.ResourceRequirement) resources.ResourceLimits {
	return resources.ResourceLimits{
		RequiredCPU:    rr.RequiredCPU,
		RequiredMemory: rr.RequiredMemory,
		LimitedCPU:     rr.LimitedCPU,
		LimitedMemory:  rr.LimitedMemory,
	}
}

// extractResourceLimits projects the manifest spec CPU/memory fields into the
// version-agnostic ResourceLimits DTO used by legacy manifests. Non-v1
// Raw() types and modern manifests (whose flat fields must be empty per
// Rule 7) are treated as having no declared limits.
func extractResourceLimits(m Manifest) resources.ResourceLimits {
	if cfg, ok := m.Raw().(*manifest.AppConfiguration); ok {
		return resources.ResourceLimits{
			RequiredCPU:    cfg.Spec.RequiredCPU,
			RequiredMemory: cfg.Spec.RequiredMemory,
			LimitedCPU:     cfg.Spec.LimitedCPU,
			LimitedMemory:  cfg.Spec.LimitedMemory,
		}
	}
	return resources.ResourceLimits{}
}

// extractUploadDest pulls the options.upload.dest field out of the active
// manifest, returning "" when no upload stanza is configured.
func extractUploadDest(m Manifest) string {
	if cfg, ok := m.Raw().(*manifest.AppConfiguration); ok {
		return cfg.Options.Upload.Dest
	}
	return ""
}

// -- Top-level convenience functions -----------------------------------------

// ValidateManifestFile is the Checker-less shortcut for one-off callers.
func ValidateManifestFile(oacPath string, opts ...Option) error {
	return New(opts...).ValidateManifestFile(oacPath)
}

// ValidateManifestContent is the byte-slice counterpart of
// ValidateManifestFile.
func ValidateManifestContent(content []byte, opts ...Option) error {
	return New(opts...).ValidateManifestContent(content)
}

// Lint is the Checker-less shortcut for (*Checker).Lint.
func Lint(oacPath string, opts ...Option) error {
	return New(opts...).Lint(oacPath)
}

// LintBothOwnerScenarios runs Lint twice: once with owner == admin (cluster
// admin install) and once with owner != admin (regular user install). Both
// scenarios must pass.
//
// This is kept as a named shortcut; it is equivalent to calling
// Lint(oacPath, append(extraOpts, WithAutoOwnerScenarios())...).
func LintBothOwnerScenarios(oacPath string, extraOpts ...Option) error {
	return Lint(oacPath, append(extraOpts, WithAutoOwnerScenarios())...)
}
