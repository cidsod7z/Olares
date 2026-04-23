package manifest

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"

	appv1 "github.com/beclab/api/api/app.bytetrade.io/v1alpha1"
	"github.com/Masterminds/semver/v3"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

var isSemver = validation.NewStringRuleWithError(func(s string) bool {
	if s == "" {
		return false
	}
	_, err := semver.NewVersion(s)
	return err == nil
}, validation.NewError("validation_is_semver", "must be a valid semantic version (e.g. 1.2.3)"))

var isHTTPURL = validation.NewStringRuleWithError(func(s string) bool {
	if s == "" {
		return true
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}, validation.NewError("validation_is_http_url", "must be a valid http(s) URL"))

var k8sQuantity = regexp.MustCompile(`^(?:\d+(?:\.\d+)?(?:[eE][-+]?(\d+|i))?(?:[kKMGTP]?i?|[mMGTPE])?|[kKMGTP]i|[mMGTPE])$`)

var (
	entranceNameRegex    = regexp.MustCompile(`^([a-z0-9A-Z-]*)$`)
	entranceHostRegex    = regexp.MustCompile(`^([a-z]([-a-z0-9]*[a-z0-9]))$`)
	entranceTitleRegex   = regexp.MustCompile(`^([a-z0-9A-Z-\s]*)$`)
	validDurationRegex   = regexp.MustCompile(`^((?:[-+]?\d+(?:\.\d+)?([smhdwy]|us|ns|ms))+)$`)
	validAPIVersions     = []any{APIVersionV1, APIVersionV2}
	validDependencyTypes = []any{"system", "application"}
	validAuthLevels      = []any{"", "public", "private"}
	validOpenMethods     = []any{"", "default", "iframe", "window"}
)

// ValidateAppConfiguration runs structural and cross-field checks on the manifest.
func ValidateAppConfiguration(c *AppConfiguration) error {
	structErr := validation.ValidateStruct(c,
		validation.Field(&c.ConfigVersion,
			validation.Required.Error("olaresManifest.version is required")),
		validation.Field(&c.APIVersion,
			validation.When(c.APIVersion != "",
				validation.In(validAPIVersions...).Error("apiVersion must be v1 or v2"),
			),
		),
		validation.Field(&c.Metadata, validation.By(validateAppMetaData)),
		validation.Field(&c.Entrances,
			validation.Required.Error("entrances is required"),
			validation.Length(1, 10).Error("entrances must have between 1 and 10 items"),
			validation.Each(validation.By(validateEntranceValue)),
			validation.By(uniqueEntranceNames),
		),
		validation.Field(&c.Spec,
			validation.Required.Error("spec is required"),
			validation.By(validateAppSpec),
		),
		validation.Field(&c.Options, validation.By(validateOptions)),
	)
	return errors.Join(structErr, checkSubCharts(c), checkSpecResources(c))
}

func validateAppMetaData(v interface{}) error {
	m, ok := v.(AppMetaData)
	if !ok {
		return fmt.Errorf("metadata: unexpected type %T", v)
	}
	return validation.ValidateStruct(&m,
		validation.Field(&m.Name,
			validation.Required.Error("metadata.name is required"),
			validation.Length(1, 30).Error("metadata.name must be 1-30 characters"),
		),
		validation.Field(&m.Icon,
			validation.Required.Error("metadata.icon is required"),
		),
		validation.Field(&m.Description,
			validation.Required.Error("metadata.description is required"),
		),
		validation.Field(&m.Title,
			validation.Required.Error("metadata.title is required"),
			validation.Length(1, 30).Error("metadata.title must be 1-30 characters"),
		),
		validation.Field(&m.Version,
			validation.Required.Error("metadata.version is required"),
			isSemver.Error("metadata.version must be a valid semantic version (e.g. 1.2.3)"),
		),
	)
}

func validateEntranceValue(v interface{}) error {
	e, ok := v.(appv1.Entrance)
	if !ok {
		return fmt.Errorf("entrance: unexpected type %T", v)
	}
	return validation.ValidateStruct(&e,
		validation.Field(&e.Name,
			validation.Match(entranceNameRegex).Error("entrance.name must match ^[a-z0-9A-Z-]*$"),
			validation.Length(0, 63).Error("entrance.name must be <= 63 characters"),
		),
		validation.Field(&e.Host,
			validation.Match(entranceHostRegex).Error("entrance.host must match ^[a-z]([-a-z0-9]*[a-z0-9])$"),
			validation.Length(0, 63).Error("entrance.host must be <= 63 characters"),
		),
		validation.Field(&e.Port,
			validation.Min(int32(1)).Error("entrance.port must be > 0"),
		),
		validation.Field(&e.Icon,
			isHTTPURL.Error("entrance.icon must be a valid http(s) URL"),
		),
		validation.Field(&e.Title,
			validation.Required.Error("entrance.title is required"),
			validation.Length(1, 30).Error("entrance.title must be 1-30 characters"),
			validation.Match(entranceTitleRegex).Error("entrance.title must match ^[a-z0-9A-Z-\\s]*$"),
		),
		validation.Field(&e.AuthLevel,
			validation.In(validAuthLevels...).Error(`entrance.authLevel must be one of "", "public", "private"`),
		),
		validation.Field(&e.OpenMethod,
			validation.In(validOpenMethods...).Error(`entrance.openMethod must be one of "", "default", "iframe", "window"`),
		),
	)
}

func validateAppSpec(v interface{}) error {
	s, ok := v.(AppSpec)
	if !ok {
		return fmt.Errorf("spec: unexpected type %T", v)
	}
	quantityRule := validation.Match(k8sQuantity).Error("must be a valid Kubernetes quantity")
	optionalQuantity := validation.When(s.RequiredGPU != "", quantityRule)
	return validation.ValidateStruct(&s,
		validation.Field(&s.RequiredMemory, quantityRule),
		validation.Field(&s.RequiredDisk, quantityRule),
		validation.Field(&s.RequiredCPU, quantityRule),
		validation.Field(&s.LimitedMemory, quantityRule),
		validation.Field(&s.LimitedCPU, quantityRule),
		validation.Field(&s.RequiredGPU, optionalQuantity),
		validation.Field(&s.Resources,
			validation.Each(validation.By(validateResourceModeValue)),
		),
	)
}

func validateResourceModeValue(v interface{}) error {
	rm, ok := v.(ResourceMode)
	if !ok {
		return fmt.Errorf("resources: unexpected type %T", v)
	}
	return ValidateResourceMode(rm)
}

func validateOptions(v interface{}) error {
	o, ok := v.(Options)
	if !ok {
		return fmt.Errorf("options: unexpected type %T", v)
	}
	return validation.ValidateStruct(&o,
		validation.Field(&o.Policies, validation.Each(validation.By(validatePolicyValue))),
		validation.Field(&o.ResetCookie),
		validation.Field(&o.Dependencies, validation.Each(validation.By(validateDependencyValue))),
		validation.Field(&o.AppScope),
		validation.Field(&o.WsConfig),
	)
}

func validatePolicyValue(v interface{}) error {
	p, ok := v.(Policy)
	if !ok {
		return fmt.Errorf("policy: unexpected type %T", v)
	}
	return validation.ValidateStruct(&p,
		validation.Field(&p.URIRegex, validation.Required.Error("policy.uriRegex is required")),
		validation.Field(&p.Level, validation.Required.Error("policy.level is required")),
		validation.Field(&p.Duration,
			validation.When(p.Duration != "",
				validation.Match(validDurationRegex).Error("policy.validDuration is malformed"),
			),
		),
	)
}

func validateDependencyValue(v interface{}) error {
	d, ok := v.(Dependency)
	if !ok {
		return fmt.Errorf("dependency: unexpected type %T", v)
	}
	return validation.ValidateStruct(&d,
		validation.Field(&d.Name, validation.Required.Error("dependency.name is required")),
		validation.Field(&d.Version, validation.Required.Error("dependency.version is required")),
		validation.Field(&d.Type,
			validation.Required.Error("dependency.type is required"),
			validation.In(validDependencyTypes...).Error("dependency.type must be system or application"),
		),
	)
}

func checkSubCharts(cfg *AppConfiguration) error {
	if cfg.APIVersion != APIVersionV2 {
		return nil
	}
	if len(cfg.Spec.SubCharts) == 0 {
		return fmt.Errorf("spec.subCharts is required for apiVersion=v2")
	}
	for _, c := range cfg.Spec.SubCharts {
		if c.Shared {
			return nil
		}
	}
	return fmt.Errorf("spec.subCharts must contain at least one entry with shared=true")
}

func uniqueEntranceNames(value interface{}) error {
	entrances, ok := value.([]appv1.Entrance)
	if !ok {
		return fmt.Errorf("entrances: unexpected type %T", value)
	}
	seen := make(map[string]struct{}, len(entrances))
	for i, e := range entrances {
		if _, dup := seen[e.Name]; dup {
			return fmt.Errorf("entrances[%d].name: duplicate entrance name %q", i, e.Name)
		}
		seen[e.Name] = struct{}{}
	}
	return nil
}
