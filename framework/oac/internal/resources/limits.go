// Package resources hosts cross-version resource-level checks that run over
// the kube.ResourceList produced by helmrender.Render.
package resources

import (
	"errors"
	"fmt"
	"path/filepath"

	"helm.sh/helm/v3/pkg/kube"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes/scheme"
)

// ResourceLimits carries the CPU / memory limits declared in the manifest
// spec, normalised to plain strings so this package stays decoupled from any
// specific manifest version.
type ResourceLimits struct {
	RequiredCPU    string
	RequiredMemory string
	LimitedCPU     string
	LimitedMemory  string
}

// CheckResourceLimits validates requests/limits on every container of every
// workload in list against the manifest-level limits carried by limits. All
// violations are collected and returned as an aggregated error.
func CheckResourceLimits(list kube.ResourceList, limits ResourceLimits) error {
	var errs []error
	rcpu, _ := resource.ParseQuantity(limits.RequiredCPU)
	rmem, _ := resource.ParseQuantity(limits.RequiredMemory)
	lcpu, _ := resource.ParseQuantity(limits.LimitedCPU)
	lmem, _ := resource.ParseQuantity(limits.LimitedMemory)

	appRequiredCPU := rcpu.AsApproximateFloat64()
	appRequiredMemory := rmem.AsApproximateFloat64()
	appLimitedCPU := lcpu.AsApproximateFloat64()
	appLimitedMemory := lmem.AsApproximateFloat64()

	if appRequiredCPU > appLimitedCPU {
		errs = append(errs, fmt.Errorf("spec.requiredCpu should be <= spec.limitedCpu"))
	}
	if appRequiredMemory > appLimitedMemory {
		errs = append(errs, fmt.Errorf("spec.requiredMemory should be <= spec.limitedMemory"))
	}

	limitCPU, limitMemory, requiredCPU, requiredMemory := 0.0, 0.0, 0.0, 0.0

	walkPodContainers(list, func(kind, wlName string, c corev1.Container) {
		requests := c.Resources.Requests
		cLimits := c.Resources.Limits
		if !requests.Cpu().IsZero() && !cLimits.Cpu().IsZero() && requests.Cpu().Cmp(*cLimits.Cpu()) > 0 {
			errs = append(errs, fmt.Errorf("%s: %s, container: %s requests.cpu must be <= limits.cpu", kind, wlName, c.Name))
		}
		if !requests.Memory().IsZero() && !cLimits.Memory().IsZero() && requests.Memory().Cmp(*cLimits.Memory()) > 0 {
			errs = append(errs, fmt.Errorf("%s: %s, container: %s requests.memory must be <= limits.memory", kind, wlName, c.Name))
		}
		if requests.Memory().IsZero() {
			errs = append(errs, fmt.Errorf("%s: %s, container: %s must set memory request", kind, wlName, c.Name))
		} else {
			requiredMemory += requests.Memory().AsApproximateFloat64()
		}
		if requests.Cpu().IsZero() {
			errs = append(errs, fmt.Errorf("%s: %s, container: %s must set cpu request", kind, wlName, c.Name))
		} else {
			requiredCPU += requests.Cpu().AsApproximateFloat64()
		}
		if cLimits.Memory().IsZero() {
			errs = append(errs, fmt.Errorf("%s: %s, container: %s must set memory limit", kind, wlName, c.Name))
		} else {
			limitMemory += cLimits.Memory().AsApproximateFloat64()
		}
		if cLimits.Cpu().IsZero() {
			errs = append(errs, fmt.Errorf("%s: %s, container: %s must set cpu limit", kind, wlName, c.Name))
		} else {
			limitCPU += cLimits.Cpu().AsApproximateFloat64()
		}
	})

	if limitCPU > appLimitedCPU {
		errs = append(errs, fmt.Errorf("sum of container resources.limits.cpu must be <= spec.limitedCpu"))
	}
	if limitMemory > appLimitedMemory {
		errs = append(errs, fmt.Errorf("sum of container resources.limits.memory must be <= spec.limitedMemory"))
	}
	if requiredCPU > appRequiredCPU {
		errs = append(errs, fmt.Errorf("sum of container resources.requests.cpu must be <= spec.requiredCpu"))
	}
	if requiredMemory > appRequiredMemory {
		errs = append(errs, fmt.Errorf("sum of container resources.requests.memory must be <= spec.requiredMemory"))
	}
	return errors.Join(errs...)
}

// CheckUploadConfig ensures that, if the manifest declares an options.upload
// destination, at least one primary container mounts it.
func CheckUploadConfig(list kube.ResourceList, uploadDest string) error {
	if uploadDest == "" {
		return nil
	}
	dest := filepath.Clean(uploadDest)
	found := false
	walkPodContainers(list, func(_, _ string, c corev1.Container) {
		if found {
			return
		}
		for _, v := range c.VolumeMounts {
			if filepath.Clean(v.MountPath) == dest {
				found = true
				return
			}
		}
	})
	if !found {
		return fmt.Errorf("cannot find volumemount path equal to options.upload.dest %q", uploadDest)
	}
	return nil
}

// CheckDeploymentName enforces that, for apps, at least one Deployment or
// StatefulSet is named after the app itself. Non-app configs are skipped.
func CheckDeploymentName(list kube.ResourceList, configType, appName string) error {
	if configType != "app" {
		return nil
	}
	for _, r := range list {
		kind := r.Object.GetObjectKind().GroupVersionKind().Kind
		if (kind == KindDeployment || kind == KindStatefulSet) && r.Name == appName {
			return nil
		}
	}
	return fmt.Errorf("must have a Deployment or StatefulSet named %q", appName)
}

// walkPodContainers iterates over Deployment and StatefulSet workloads only
// (per current requirements) and yields each primary container.
func walkPodContainers(list kube.ResourceList, fn func(kind, wlName string, c corev1.Container)) {
	for _, r := range list {
		kind := r.Object.GetObjectKind().GroupVersionKind().Kind
		switch kind {
		case KindDeployment:
			var dep appsv1.Deployment
			if err := scheme.Scheme.Convert(r.Object, &dep, nil); err != nil {
				continue
			}
			for _, c := range dep.Spec.Template.Spec.Containers {
				fn(kind, dep.Name, c)
			}
		case KindStatefulSet:
			var sts appsv1.StatefulSet
			if err := scheme.Scheme.Convert(r.Object, &sts, nil); err != nil {
				continue
			}
			for _, c := range sts.Spec.Template.Spec.Containers {
				fn(kind, sts.Name, c)
			}
		}
	}
}
