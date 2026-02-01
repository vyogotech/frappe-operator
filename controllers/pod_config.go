package controllers

import (
	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// applyPodConfig merges the desired pod configuration with initial labels and returns the resolved kubernetes spec fields.
func applyPodConfig(config *vyogotechv1alpha1.PodConfig, initialLabels map[string]string) (
	nodeSelector map[string]string,
	affinity *corev1.Affinity,
	tolerations []corev1.Toleration,
	labels map[string]string,
) {
	labels = make(map[string]string)
	for k, v := range initialLabels {
		labels[k] = v
	}

	if config == nil {
		return nil, nil, nil, labels
	}

	// Apply custom labels
	for k, v := range config.Labels {
		labels[k] = v
	}

	// Node Selector
	nodeSelector = config.NodeSelector

	// Tolerations
	tolerations = config.Tolerations

	// Affinity
	affinity = config.Affinity

	// Geo Tag Logic
	if config.GeoTag != nil {
		if labels == nil {
			labels = make(map[string]string)
		}

		geoRequirements := []corev1.NodeSelectorRequirement{}

		if config.GeoTag.Region != "" {
			labels["topology.kubernetes.io/region"] = config.GeoTag.Region
			geoRequirements = append(geoRequirements, corev1.NodeSelectorRequirement{
				Key:      "topology.kubernetes.io/region",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{config.GeoTag.Region},
			})
		}

		if config.GeoTag.Zone != "" {
			labels["topology.kubernetes.io/zone"] = config.GeoTag.Zone
			geoRequirements = append(geoRequirements, corev1.NodeSelectorRequirement{
				Key:      "topology.kubernetes.io/zone",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{config.GeoTag.Zone},
			})
		}

		if len(geoRequirements) > 0 {
			if affinity == nil {
				affinity = &corev1.Affinity{}
			}
			if affinity.NodeAffinity == nil {
				affinity.NodeAffinity = &corev1.NodeAffinity{}
			}
			if affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
				affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
			}

			// Add a new term or merge into existing terms? 
			// For simplicity, we add it as a requirement to all existing terms if they exist,
			// or create a new term if none exist.
			if len(affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) == 0 {
				affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = []corev1.NodeSelectorTerm{
					{
						MatchExpressions: geoRequirements,
					},
				}
			} else {
				for i := range affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
					affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[i].MatchExpressions = append(
						affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[i].MatchExpressions,
						geoRequirements...)
				}
			}
		}
	}

	return nodeSelector, affinity, tolerations, labels
}
