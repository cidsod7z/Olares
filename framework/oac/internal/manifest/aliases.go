package manifest

import (
	appv1 "github.com/beclab/api/api/app.bytetrade.io/v1alpha1"
	sysv1alpha1 "github.com/beclab/api/api/sys.bytetrade.io/v1alpha1"
	apimanifest "github.com/beclab/api/manifest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIVersion values accepted by the parser.
const (
	APIVersionV1 = "v1"
	APIVersionV2 = "v2"
)

// Install / upgrade mode strings match github.com/beclab/api/manifest.
const (
	InstallOrUpgradeClientOnly      = apimanifest.InstallOrUpgradeClientOnly
	InstallOrUpgradeServerAndClient = apimanifest.InstallOrUpgradeServerAndClient
	InstallOrUpgradeV1              = apimanifest.InstallOrUpgradeV1
)

// The following aliases mirror github.com/beclab/api/manifest (and related
// API packages) so internal code can refer to one stable import path.
type (
	AppConfiguration         = apimanifest.AppConfiguration
	AppSpec                  = apimanifest.AppSpec
	AppMetaData              = apimanifest.AppMetaData
	Chart                    = apimanifest.Chart
	Provider                 = apimanifest.Provider
	Permission               = apimanifest.Permission
	ProviderPermission       = apimanifest.ProviderPermission
	Policy                   = apimanifest.Policy
	Dependency               = apimanifest.Dependency
	Conflict                 = apimanifest.Conflict
	Options                  = apimanifest.Options
	ResetCookie              = apimanifest.ResetCookie
	AppScope                 = apimanifest.AppScope
	WsConfig                 = apimanifest.WsConfig
	Upload                   = apimanifest.Upload
	OIDC                     = apimanifest.OIDC
	Middleware               = apimanifest.Middleware
	Database                 = apimanifest.Database
	PostgresConfig           = apimanifest.PostgresConfig
	ArgoConfig               = apimanifest.ArgoConfig
	MinioConfig              = apimanifest.MinioConfig
	Bucket                   = apimanifest.Bucket
	RabbitMQConfig           = apimanifest.RabbitMQConfig
	VHost                    = apimanifest.VHost
	ElasticsearchConfig      = apimanifest.ElasticsearchConfig
	Index                    = apimanifest.Index
	RedisConfig              = apimanifest.RedisConfig
	MongodbConfig            = apimanifest.MongodbConfig
	MariaDBConfig            = apimanifest.MariaDBConfig
	MySQLConfig              = apimanifest.MySQLConfig
	ClickHouseConfig         = apimanifest.ClickHouseConfig
	NatsConfig               = apimanifest.NatsConfig
	Subject                  = apimanifest.Subject
	Export                   = apimanifest.Export
	Ref                      = apimanifest.Ref
	RefSubject               = apimanifest.RefSubject
	PermissionNats           = apimanifest.PermissionNats
	ConfigOverlay            = apimanifest.ConfigOverlay
	Hardware                 = apimanifest.Hardware
	CpuConfig                = apimanifest.CpuConfig
	GpuConfig                = apimanifest.GpuConfig
	SupportClient            = apimanifest.SupportClient
	SpecialResource          = apimanifest.SpecialResource
	Entrance                 = appv1.Entrance
	ServicePort              = appv1.ServicePort
	TailScale                = appv1.TailScale
	ACL                      = appv1.ACL
	AppEnvVar                = sysv1alpha1.AppEnvVar
	ResourceMode             = apimanifest.ResourceMode
	ResourceRequirement      = apimanifest.ResourceRequirement
	LabelSelector              = metav1.LabelSelector
	LabelSelectorRequirement = metav1.LabelSelectorRequirement
)
