// Package helmrender wraps helm's dry-run engine with a set of sensible fake
// values so oac can lint charts without talking to a real cluster.
package helmrender

import (
	"strings"

	olm "github.com/beclab/Olares/framework/oac/internal/manifest"
)

// BuildValues builds the fake values map historically used by oac's
// helm dry-run.
//
//   - owner controls .Values.bfl.username (defaults to "bfl-username" so old
//     test fixtures keep working).
//   - admin controls .Values.admin. When admin == owner the value is also set
//     so that templates gated on "admin equals current user" render.
//   - entrances populates .Values.domain with a random-string per entrance
//     name, which is what the legacy renderer did.
func BuildValues(owner, admin string, entrances []olm.EntranceInfo) map[string]interface{} {
	if owner == "" {
		owner = "bfl-username"
	}
	values := map[string]interface{}{
		"bfl": map[string]interface{}{
			"username": owner,
		},
		"user": map[string]interface{}{
			"zone": "user-zone",
		},
		"schedule": map[string]interface{}{
			"nodeName": "node",
		},
		"userspace": map[string]interface{}{
			"appdata": "appdata",
			"data":    "userspace/Home",
		},
		"os": map[string]interface{}{
			"appKey":    "appKey",
			"appSecret": "appSecret",
		},
		"dep":      map[string]interface{}{},
		"postgres": map[string]interface{}{"databases": map[string]interface{}{}},
		"redis":    map[string]interface{}{},
		"mongodb":  map[string]interface{}{"databases": map[string]interface{}{}},
		"zinc":     map[string]interface{}{"indexes": map[string]interface{}{}},
		"mariadb":  map[string]interface{}{"databases": map[string]interface{}{}},
		"mysql":    map[string]interface{}{"databases": map[string]interface{}{}},
		"minio":    map[string]interface{}{"buckets": map[string]interface{}{}},
		"rabbitmq": map[string]interface{}{"vhosts": map[string]interface{}{}},
		"elasticsearch": map[string]interface{}{
			"indexes": map[string]interface{}{},
		},
		"nats": map[string]interface{}{
			"subjects": map[string]interface{}{},
			"refs":     map[string]interface{}{},
		},
		"svcs":    map[string]interface{}{},
		"cluster": map[string]interface{}{},
		"GPU":     map[string]interface{}{},
		"oidc": map[string]interface{}{
			"client": map[string]interface{}{},
			"issuer": "issuer",
		},
		"olaresEnv": map[string]interface{}{},
	}

	if admin != "" {
		values["admin"] = admin
	}

	entries := make(map[string]interface{}, len(entrances))
	for _, e := range entrances {
		entries[e.Name] = "random-string"
	}
	values["domain"] = entries

	return values
}

// SetGPUType mutates values so that `.Values.GPU.Type` renders as mode.
// If values["GPU"] is missing or is not a map, it is (re-)initialised.
// Passing an empty mode clears the field so templates guarded on "GPU.Type
// is set" skip their GPU-specific branches.
func SetGPUType(values map[string]interface{}, mode string) {
	gpu, ok := values["GPU"].(map[string]interface{})
	if !ok {
		gpu = map[string]interface{}{}
		values["GPU"] = gpu
	}
	if mode == "" {
		delete(gpu, "Type")
		return
	}
	gpu["Type"] = mode
}

// SetClientOnlyInstall sets .Values.client=true and clears .Values.clientAndServer
// so charts can render their client-only workload branch.
func SetClientOnlyInstall(values map[string]interface{}) {
	values["client"] = true
	delete(values, "clientAndServer")
}

// SetClientAndServerInstall sets .Values.clientAndServer=true and clears .Values.client
// so charts can render combined server+client workloads.
func SetClientAndServerInstall(values map[string]interface{}) {
	values["clientAndServer"] = true
	delete(values, "client")
}

// ApplyDefaultInstallProfile sets helm values that mirror a typical Olares install
// before integrity checks (deployment name, upload mounts, image listing).
// apiVersion=v2 charts often gate workloads on .Values.client / .Values.clientAndServer;
// priming clientAndServer ensures server+client resources appear in the default dry-run.
func ApplyDefaultInstallProfile(values map[string]interface{}, apiVersion string) {
	if strings.EqualFold(apiVersion, "v2") {
		SetClientAndServerInstall(values)
	}
}
