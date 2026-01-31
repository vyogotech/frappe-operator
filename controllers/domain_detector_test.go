/*
Copyright 2023 Vyogo Technologies.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use it except in compliance with the License.
*/

package controllers

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIsIPAddress(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"0.0.0.0", true},
		{"255.255.255.255", true},
		{"example.com", false},
		{"192.168.1", false},
		{"192.168.1.1.1", false},
		{"", false},
		{"1.2.3.a", false},
		// isIPAddress only checks format (four dot-separated digit groups), not octet range
		{"256.1.1.1", true},
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			if got := isIPAddress(tt.s); got != tt.want {
				t.Errorf("isIPAddress(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestExtractDomainSuffix(t *testing.T) {
	tests := []struct {
		hostname string
		want     string
	}{
		{"*.example.com", ".example.com"},
		{"ingress.example.com", ".example.com"},
		{"example.com", ".example.com"},
		{"sub.foo.example.com", ".example.com"},
		{"", ""},
		{"192.168.1.1", ""},
		{"localhost", ""},
		{"single", ""},
	}
	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			if got := extractDomainSuffix(tt.hostname); got != tt.want {
				t.Errorf("extractDomainSuffix(%q) = %q, want %q", tt.hostname, got, tt.want)
			}
		})
	}
}

func TestDetectDomainSuffix_NilClient(t *testing.T) {
	d := &DomainDetector{}
	_, err := d.DetectDomainSuffix(context.Background(), "default")
	if err == nil {
		t.Error("DetectDomainSuffix with nil client expected error")
	}
}

func TestDetectDomainSuffix_NoIngressFound(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	d := &DomainDetector{Client: client}
	_, err := d.DetectDomainSuffix(context.Background(), "default")
	if err == nil {
		t.Error("DetectDomainSuffix with no ingress services expected error")
	}
}

func TestDetectDomainSuffix_FromAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ingress-nginx-controller",
			Namespace: "ingress-nginx",
			Annotations: map[string]string{
				"external-dns.alpha.kubernetes.io/hostname": "*.example.com",
			},
		},
		Spec: corev1.ServiceSpec{},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(svc).Build()
	d := &DomainDetector{Client: client}
	suffix, err := d.DetectDomainSuffix(context.Background(), "default")
	if err != nil {
		t.Fatalf("DetectDomainSuffix: %v", err)
	}
	if suffix != ".example.com" {
		t.Errorf("expected .example.com, got %q", suffix)
	}
}
