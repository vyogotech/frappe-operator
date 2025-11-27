/*
Copyright 2023 Vyogo Technologies.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DomainDetector detects the cluster's domain suffix from Ingress Controller services
type DomainDetector struct {
	Client client.Client
}

// DetectDomainSuffix attempts to detect the cluster's external domain suffix
// by examining Ingress Controller services and their annotations
func (d *DomainDetector) DetectDomainSuffix(ctx context.Context, namespace string) (string, error) {
	logger := log.FromContext(ctx)

	// Common Ingress Controller service names and namespaces
	ingressServices := []types.NamespacedName{
		{Name: "ingress-nginx-controller", Namespace: "ingress-nginx"},
		{Name: "nginx-ingress-controller", Namespace: "ingress-nginx"},
		{Name: "traefik", Namespace: "traefik"},
		{Name: "traefik", Namespace: "kube-system"},
	}

	for _, svcRef := range ingressServices {
		svc := &corev1.Service{}
		if err := d.Client.Get(ctx, svcRef, svc); err != nil {
			continue // Try next service
		}

		logger.V(1).Info("Found Ingress Controller service", "service", svcRef.Name, "namespace", svcRef.Namespace)

		// Check for external-dns annotation
		if hostname, ok := svc.Annotations["external-dns.alpha.kubernetes.io/hostname"]; ok && hostname != "" {
			// Extract domain from hostname (e.g., "*.example.com" -> ".example.com")
			suffix := extractDomainSuffix(hostname)
			if suffix != "" {
				logger.Info("Detected domain suffix from external-dns annotation", "suffix", suffix, "service", svcRef.Name)
				return suffix, nil
			}
		}

		// Check LoadBalancer hostname
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer && len(svc.Status.LoadBalancer.Ingress) > 0 {
			lbIngress := svc.Status.LoadBalancer.Ingress[0]
			if lbIngress.Hostname != "" {
				// Try to extract domain from LB hostname (e.g., "a1b2c3.us-west-2.elb.amazonaws.com")
				suffix := extractDomainSuffix(lbIngress.Hostname)
				if suffix != "" {
					logger.Info("Detected domain suffix from LoadBalancer hostname", "suffix", suffix, "service", svcRef.Name)
					return suffix, nil
				}
			}
		}
	}

	logger.V(1).Info("Could not auto-detect domain suffix")
	return "", fmt.Errorf("no domain suffix detected from Ingress Controller services")
}

// extractDomainSuffix extracts a domain suffix from a hostname
// Examples:
//   - "*.example.com" -> ".example.com"
//   - "ingress.example.com" -> ".example.com"
//   - "example.com" -> ".example.com"
func extractDomainSuffix(hostname string) string {
	if hostname == "" {
		return ""
	}

	// Remove wildcard prefix
	hostname = strings.TrimPrefix(hostname, "*")
	hostname = strings.TrimPrefix(hostname, ".")

	// Skip IP addresses
	if strings.Contains(hostname, ":") || isIPAddress(hostname) {
		return ""
	}

	// Split into parts
	parts := strings.Split(hostname, ".")
	if len(parts) < 2 {
		return ""
	}

	// For hostnames like "ingress.example.com", extract ".example.com"
	// For hostnames like "example.com", return ".example.com"
	if len(parts) >= 2 {
		// Take last two parts (domain + TLD)
		domain := "." + strings.Join(parts[len(parts)-2:], ".")
		return domain
	}

	return ""
}

// isIPAddress checks if a string looks like an IP address
func isIPAddress(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 || len(part) > 3 {
			return false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}
