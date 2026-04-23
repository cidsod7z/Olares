package oac_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/beclab/Olares/framework/oac"
)

const testdataFirefox = "testdata/firefox"

func TestValidateManifestFile(t *testing.T) {
	if err := oac.ValidateManifestFile(testdataFirefox); err != nil {
		t.Fatalf("ValidateManifestFile: unexpected error: %v", err)
	}
}

func TestValidateManifestContent_MissingRequired(t *testing.T) {
	content := []byte(`
olaresManifest.version: "0.8.1"
olaresManifest.type: app
metadata:
  name: firefox
entrances:
- name: firefox
  host: firefox
  port: 8080
  title: Firefox
spec:
  requiredMemory: 200Mi
  requiredDisk: 10Mi
  requiredCpu: 0.1
  limitedMemory: 1000Mi
  limitedCpu: "1"
  supportClient:
    edge: ""
    android: ""
    ios: ""
    windows: ""
    mac: ""
    linux: ""
  supportArch:
    - amd64
permission:
  appData: true
options:
  policies: []
  analytics: { enabled: false }
  resetCookie: { enabled: false }
`)
	err := oac.ValidateManifestContent(content)
	if err == nil {
		t.Fatalf("expected validation error; metadata.icon is missing")
	}
	var vErr *oac.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("expected *oac.ValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(vErr.Error(), "metadata") {
		t.Fatalf("expected validation error to mention metadata field, got: %v", vErr)
	}
}

func TestValidateManifestContent_APIVersionRestricted(t *testing.T) {
	content := []byte(`
olaresManifest.version: "0.8.1"
apiVersion: v99
metadata:
  name: x
  icon: a
  description: d
  title: t
  version: 1.0.0
entrances:
- name: x
  host: x
  port: 1
  title: T
permission:
  appData: false
options: {}
`)
	err := oac.ValidateManifestContent(content)
	if err == nil {
		t.Fatalf("expected ErrNotImplemented or strategy-unknown error for apiVersion=v99")
	}
}

func TestLint_Firefox(t *testing.T) {
	err := oac.Lint(testdataFirefox,
		oac.WithOwnerAdmin("alice"),
		oac.SkipResourceCheck(),
	)
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
}

func TestListImagesFromOAC(t *testing.T) {
	imgs, err := oac.ListImagesFromOAC(testdataFirefox, oac.WithOwnerAdmin("alice"))
	if err != nil {
		t.Fatalf("ListImagesFromOAC: %v", err)
	}
	if len(imgs) == 0 {
		t.Fatalf("expected at least one image, got zero")
	}
	for i, v := range imgs {
		if v == "" {
			t.Fatalf("empty entry at index %d", i)
		}
		if i > 0 && imgs[i-1] > v {
			t.Fatalf("images are not sorted: %v", imgs)
		}
	}
}

func TestLintBothOwnerScenarios(t *testing.T) {
	err := oac.LintBothOwnerScenarios(testdataFirefox, oac.SkipResourceCheck())
	if err != nil {
		t.Fatalf("LintBothOwnerScenarios: %v", err)
	}
}
