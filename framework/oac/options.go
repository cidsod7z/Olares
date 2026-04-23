package oac

// Option mutates a Checker built via New. Options are idempotent and safe to
// apply in any order.
type Option func(*OAC)

// WithOwner sets the .Values.bfl.username template value and the owner field
// used when rendering helm charts. When owner is empty the Checker keeps its
// existing value.
func WithOwner(owner string) Option {
	return func(c *OAC) {
		if owner != "" {
			c.owner = owner
		}
	}
}

// WithAdmin sets the .Values.admin template value. An empty admin is ignored.
func WithAdmin(admin string) Option {
	return func(c *OAC) {
		if admin != "" {
			c.admin = admin
		}
	}
}

// WithOwnerAdmin sets both owner and admin to the same value, modelling the
// "installed as admin" scenario where the cluster administrator is also the
// acting user.
func WithOwnerAdmin(value string) Option {
	return func(c *OAC) {
		if value != "" {
			c.owner = value
			c.admin = value
		}
	}
}

// SkipManifestCheck disables OlaresManifest.yaml structural validation.
func SkipManifestCheck() Option {
	return func(c *OAC) { c.skipManifest = true }
}

// SkipResourceCheck disables the container-level resource-limits check.
//
// Note: this option does NOT disable the upload-mount and workload-naming
// checks, which Lint always runs because they guard structural integrity
// (a chart that declares options.upload.dest but mounts it nowhere, or an
// app whose templates produce no Deployment/StatefulSet named after the
// app, is broken regardless of limit accounting).
func SkipResourceCheck() Option {
	return func(c *OAC) { c.skipResource = true }
}

// SkipFolderCheck disables the chart-folder layout check.
func SkipFolderCheck() Option {
	return func(c *OAC) { c.skipFolder = true }
}

// SkipSameVersionCheck disables the Chart.yaml <-> manifest version
// consistency check. By default the check runs; callers that roll their own
// version-alignment step can opt out here.
func SkipSameVersionCheck() Option {
	return func(c *OAC) { c.skipSameVersion = true }
}

// WithSameVersionCheck re-enables the Chart.yaml <-> manifest version
// consistency check. Mostly useful when composing an option set that had
// SkipSameVersionCheck baked in and a particular call-site wants it back on.
func WithSameVersionCheck() Option {
	return func(c *OAC) { c.skipSameVersion = false }
}

// WithServiceAccountRulesCheck enables the RBAC rule inspection which makes
// sure the chart doesn't grant ServiceAccounts forbidden permissions. It is
// disabled by default to match historical Lint behaviour; callers that need
// it can opt in explicitly.
func WithServiceAccountRulesCheck() Option {
	return func(c *OAC) { c.runRBAC = true }
}

// WithAutoOwnerScenarios makes Lint ignore any explicit WithOwner / WithAdmin
// / WithOwnerAdmin values and instead run the chart-rendering portion of the
// pipeline twice:
//
//  1. owner == admin (cluster-admin install)
//  2. owner != admin (regular user install)
//
// Both scenarios must pass. The manifest-level checks (folder layout, ozzo
// validation, custom validators, same-version) are owner-independent and
// still only run once.
//
// This is the programmatic equivalent of the LintBothOwnerScenarios helper —
// use it whenever the caller does not have a concrete owner/admin pair and
// wants the linter to cover both install modes automatically.
func WithAutoOwnerScenarios() Option {
	return func(c *OAC) { c.autoOwner = true }
}

// WithoutAutoOwnerScenarios clears the auto-owner flag, pinning Lint back to
// the explicit owner/admin values. Mostly useful when composing option sets
// that have WithAutoOwnerScenarios baked in and a particular call-site wants
// to opt out.
func WithoutAutoOwnerScenarios() Option {
	return func(c *OAC) { c.autoOwner = false }
}

// CustomValidator is invoked with the chart directory path and the parsed
// Manifest after the built-in structural checks have run.
type CustomValidator func(oacPath string, m Manifest) error

// WithCustomValidator adds a user-defined validator to the Checker.
func WithCustomValidator(fn CustomValidator) Option {
	return func(c *OAC) {
		if fn != nil {
			c.customValidators = append(c.customValidators, fn)
		}
	}
}

// WithAppDataValidator registers the built-in check that warns when a chart's
// templates reference .Values.userspace.appdata without declaring
// permission.appData in the manifest.
func WithAppDataValidator() Option {
	return WithCustomValidator(checkAppDataUsage)
}
