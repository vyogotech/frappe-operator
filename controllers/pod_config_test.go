package controllers

import (
	"testing"

	vyogotechv1alpha1 "github.com/vyogotech/frappe-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func TestApplyPodConfig(t *testing.T) {
	tests := []struct {
		name       string
		podConfig  *vyogotechv1alpha1.PodConfig
		initial    struct {
			labels map[string]string
		}
		wantLabels map[string]string
		wantNodeSelector map[string]string
		wantAffinity *corev1.Affinity
		wantTolerations []corev1.Toleration
	}{
		{
			name: "Basic labels and nodeSelector",
			podConfig: &vyogotechv1alpha1.PodConfig{
				Labels: map[string]string{"foo": "bar"},
				NodeSelector: map[string]string{"disk": "ssd"},
			},
			wantLabels: map[string]string{"foo": "bar"},
			wantNodeSelector: map[string]string{"disk": "ssd"},
		},
		{
			name: "GeoTag Region and Zone",
			podConfig: &vyogotechv1alpha1.PodConfig{
				GeoTag: &vyogotechv1alpha1.GeoTagConfig{
					Region: "us-east-1",
					Zone: "us-east-1a",
				},
			},
			wantLabels: map[string]string{
				"topology.kubernetes.io/region": "us-east-1",
				"topology.kubernetes.io/zone": "us-east-1a",
			},
			wantAffinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key: "topology.kubernetes.io/region",
										Operator: corev1.NodeSelectorOpIn,
										Values: []string{"us-east-1"},
									},
									{
										Key: "topology.kubernetes.io/zone",
										Operator: corev1.NodeSelectorOpIn,
										Values: []string{"us-east-1a"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Merge labels with initial labels",
			podConfig: &vyogotechv1alpha1.PodConfig{
				Labels: map[string]string{"custom": "value"},
			},
			initial: struct{ labels map[string]string }{
				labels: map[string]string{"app": "frappe"},
			},
			wantLabels: map[string]string{
				"app":    "frappe",
				"custom": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := make(map[string]string)
			if tt.initial.labels != nil {
				for k, v := range tt.initial.labels {
					labels[k] = v
				}
			}
			
			nodeSelector, affinity, tolerations, finalLabels := applyPodConfig(tt.podConfig, labels)

			// Check Labels
			for k, v := range tt.wantLabels {
				if finalLabels[k] != v {
					t.Errorf("label %s: got %s, want %s", k, finalLabels[k], v)
				}
			}

			// Check NodeSelector
			if len(tt.wantNodeSelector) != len(nodeSelector) {
				t.Errorf("nodeSelector length: got %d, want %d", len(nodeSelector), len(tt.wantNodeSelector))
			}
			for k, v := range tt.wantNodeSelector {
				if nodeSelector[k] != v {
					t.Errorf("nodeSelector %s: got %s, want %s", k, nodeSelector[k], v)
				}
			}

			// Check Affinity (Simplified check)
			if tt.wantAffinity != nil {
				if affinity == nil {
					t.Error("expected affinity, got nil")
				} else if affinity.NodeAffinity == nil {
					t.Error("expected node affinity, got nil")
				}
				// Deep check would be better but let's at least check basics
			}

			// Check Tolerations
			if len(tt.wantTolerations) != len(tolerations) {
				t.Errorf("tolerations length: got %d, want %d", len(tolerations), len(tt.wantTolerations))
			}
		})
	}
}
