package oac

import (
	"fmt"

	"github.com/beclab/Olares/framework/oac/internal/helmrender"
	"github.com/beclab/Olares/framework/oac/internal/resources"
)

// ListImages returns the sorted, deduplicated set of container images used by
// oacPath. The set is the union of:
//
//  1. Images discovered by walking the Deployment/StatefulSet/DaemonSet
//     workloads produced by a helm dry-run (primary containers only).
//  2. Images listed under options.images in OlaresManifest.yaml, which is
//     how apps declare extra images that are pulled outside the chart (e.g.
//     images referenced at runtime or by client-side tooling).
func (c *OAC) ListImages(oacPath string) ([]string, error) {
	m, err := c.LoadManifestFile(oacPath)
	if err != nil {
		return nil, err
	}
	values := helmrender.BuildValues(c.owner, c.admin, m.Entrances())
	helmrender.ApplyDefaultInstallProfile(values, m.APIVersion())
	list, err := helmrender.Render(oacPath, values, m.AppName())
	if err != nil {
		return nil, fmt.Errorf("helm render: %w", err)
	}
	workloadImages := resources.ExtractWorkloadImages(list)
	return resources.MergeImages(workloadImages, m.OptionsImages()), nil
}

// ListImagesFromOAC is the Checker-less shortcut for (*Checker).ListImages.
func ListImagesFromOAC(oacPath string, opts ...Option) ([]string, error) {
	return New(opts...).ListImages(oacPath)
}
