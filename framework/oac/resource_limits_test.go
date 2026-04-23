package oac

import (
	"testing"

	appv1 "github.com/beclab/api/api/app.bytetrade.io/v1alpha1"
	"github.com/beclab/api/manifest"

	olm "github.com/beclab/Olares/framework/oac/internal/manifest"
)

func TestResourceLimitsForResourceMode(t *testing.T) {
	cfg := &AppConfiguration{
		ConfigVersion: "0.13.0",
		ConfigType:    "app",
		APIVersion:    olm.APIVersionV2,
		Metadata:      manifest.AppMetaData{Name: "demo", Title: "D", Version: "1"},
		Entrances:     []appv1.Entrance{{Name: "w", Host: "d", Port: 80}},
		Spec: manifest.AppSpec{
			SupportArch: []string{"amd64"},
			Resources: []manifest.ResourceMode{{
				Mode: olm.ResourceModeNvidia,
				Server: &manifest.ResourceRequirement{
					RequiredCPU: "100m", LimitedCPU: "200m",
					RequiredMemory: "128Mi", LimitedMemory: "256Mi",
					RequiredDisk: "2Gi", LimitedDisk: "4Gi",
					RequiredGPU: "4Gi", LimitedGPU: "8Gi",
				},
				Client: &manifest.ResourceRequirement{
					RequiredCPU: "50m", LimitedCPU: "100m",
					RequiredMemory: "64Mi", LimitedMemory: "128Mi",
					RequiredDisk: "1Gi", LimitedDisk: "2Gi",
					RequiredGPU: "2Gi", LimitedGPU: "4Gi",
				},
			}},
		},
	}
	lim, err := ResourceLimitsForResourceMode(cfg, "nvidia", InstallProfileClientOnly)
	if err != nil {
		t.Fatal(err)
	}
	if lim.RequiredCPU != "50m" || lim.LimitedMemory != "128Mi" {
		t.Fatalf("clientOnly: %+v", lim)
	}
	if lim.RequiredDisk != "1Gi" || lim.RequiredGPU != "2Gi" || lim.LimitedGPU != "4Gi" {
		t.Fatalf("clientOnly disk/gpu: %+v", lim)
	}
	lim2, err := ResourceLimitsForResourceMode(cfg, "NVIDIA", InstallProfileClientAndServer)
	if err != nil {
		t.Fatal(err)
	}
	if lim2.RequiredCPU != "150m" || lim2.LimitedCPU != "300m" {
		t.Fatalf("clientAndServer: %+v", lim2)
	}
	if lim2.RequiredDisk != "3Gi" || lim2.LimitedDisk != "6Gi" || lim2.RequiredGPU != "6Gi" || lim2.LimitedGPU != "12Gi" {
		t.Fatalf("clientAndServer disk/gpu: %+v", lim2)
	}
	if _, err := ResourceLimitsForResourceMode(cfg, "cpu", InstallProfileClientOnly); err == nil {
		t.Fatal("expected missing mode error")
	}
}

func TestResourceLimitsForResourceMode_NilCfg(t *testing.T) {
	if _, err := ResourceLimitsForResourceMode(nil, "cpu", InstallProfileClientOnly); err == nil {
		t.Fatal("expected nil cfg error")
	}
}
