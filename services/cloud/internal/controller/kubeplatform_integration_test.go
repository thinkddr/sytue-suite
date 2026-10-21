//go:build integration

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"os"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// TestKubePlatformAgainstRealCluster ejercita EnsureNamespace + ApplyApp contra
// un clúster real (Kapsule/kind/minikube) apuntado por KUBECONFIG. Se salta si no
// hay clúster disponible, de modo que no rompe el CI sin infraestructura.
//
//	Ejecutar: go test -tags=integration ./services/cloud/internal/controller/ \
//		-run TestKubePlatformAgainstRealCluster
func TestKubePlatformAgainstRealCluster(t *testing.T) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("KUBECONFIG no definido; se omite el test de integración K8s")
	}

	cfg := config.Config{}
	cfg.KubeconfigPath = kubeconfig
	cfg.CertIssuer = "sytue-letsencrypt"
	cfg.AppsDomain = "apps.sytue.eu"

	p, err := NewKubePlatform(cfg)
	if err != nil {
		t.Fatalf("NewKubePlatform: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	const tenant = "itest"
	ns := "tenant-" + tenant
	t.Cleanup(func() {
		_ = p.cs.CoreV1().Namespaces().Delete(context.Background(), ns, metav1.DeleteOptions{})
	})

	if err := p.EnsureNamespace(ctx, ns, tenant, domain.QuotasFor(domain.PlanFree)); err != nil {
		t.Fatalf("EnsureNamespace: %v", err)
	}
	if _, err := p.cs.CoreV1().ResourceQuotas(ns).Get(ctx, "tenant-quota", metav1.GetOptions{}); err != nil {
		t.Fatalf("ResourceQuota ausente en clúster: %v", err)
	}
	if _, err := p.cs.NetworkingV1().NetworkPolicies(ns).Get(ctx, "default-deny-all", metav1.GetOptions{}); err != nil {
		t.Fatalf("NetworkPolicy deny ausente: %v", err)
	}

	spec := AppSpec{
		Tenant: tenant, Namespace: ns, ProjectSlug: "web",
		DeploymentID: "itest-1111-2222-3333-444455556666",
		Target:       domain.TargetPreview,
		Image:        "nginxinc/nginx-unprivileged:stable",
		Replicas:     1, Port: 8080,
		Hostnames: []string{"web-pr-1.preview.sytue.eu"},
	}
	if _, err := p.ApplyApp(ctx, spec); err != nil {
		t.Fatalf("ApplyApp: %v", err)
	}
	name := appResourceName(spec)
	if _, err := p.cs.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{}); err != nil {
		t.Fatalf("Deployment no aplicado: %v", err)
	}
	if err := p.RetireApp(ctx, ns, spec.DeploymentID); err != nil {
		t.Fatalf("RetireApp: %v", err)
	}
}
