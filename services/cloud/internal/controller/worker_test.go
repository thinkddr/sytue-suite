// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"testing"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// La propiedad que define un worker: corre y NO se publica. Si algún día alguien
// «uniformara» los tipos dándole Service e Ingress, esto tiene que ponerse rojo:
// sería publicar en internet un consumidor de cola que nadie debe llamar.
func TestProvisionWorkerNoPublicaNada(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, testCfg(), nil, nil)
	ctx := context.Background()
	svc := domain.ManagedService{
		ID: uuid.New(), Tenant: "acme", ProjectID: uuid.New(),
		Kind: domain.ServiceWorker, Name: "consumidor",
		Config: map[string]string{
			"image":    "rg.fr-par.scw.cloud/sytue/web@sha256:" + strings40(),
			"command":  "node worker.js",
			"replicas": "2",
		},
	}

	res, err := p.provisionWorker(ctx, svc)
	if err != nil {
		t.Fatalf("provisionWorker: %v", err)
	}
	if res.Config["state"] != "running" || res.Config["replicas"] != "2" {
		t.Errorf("config devuelta = %v", res.Config)
	}

	name := svcResourceName(svc) + "-worker"
	dep, err := cs.AppsV1().Deployments("tenant-acme").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment del worker: %v", err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 2 {
		t.Errorf("réplicas = %v; se esperaban 2", dep.Spec.Replicas)
	}
	// Un pod de Job termina; el de un worker no debe, y `Never` en un Deployment
	// lo rechaza el API server.
	if got := dep.Spec.Template.Spec.RestartPolicy; got != corev1.RestartPolicyAlways {
		t.Errorf("RestartPolicy = %q; un worker tiene que reiniciarse", got)
	}

	// Ni Service ni Ingress: es LA diferencia con una función.
	if _, err := cs.CoreV1().Services("tenant-acme").Get(ctx, name, metav1.GetOptions{}); err == nil {
		t.Error("un worker no puede tener Service: no atiende peticiones")
	}
	ings, err := cs.NetworkingV1().Ingresses("tenant-acme").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("listar ingresses: %v", err)
	}
	if len(ings.Items) != 0 {
		t.Errorf("un worker no puede publicar Ingress; hay %d", len(ings.Items))
	}
}

// Cero copias es una forma legítima de pararlo sin borrarlo, así que tiene que
// llegar tal cual al Deployment (y no colarse el 1 por defecto).
func TestProvisionWorkerCeroReplicasLoPara(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, testCfg(), nil, nil)
	ctx := context.Background()
	svc := domain.ManagedService{
		ID: uuid.New(), Tenant: "acme", ProjectID: uuid.New(),
		Kind: domain.ServiceWorker, Name: "parado",
		Config: map[string]string{
			"image":    "rg.fr-par.scw.cloud/sytue/web@sha256:" + strings40(),
			"replicas": "0",
		},
	}
	if _, err := p.provisionWorker(ctx, svc); err != nil {
		t.Fatalf("provisionWorker: %v", err)
	}
	dep, err := cs.AppsV1().Deployments("tenant-acme").
		Get(ctx, svcResourceName(svc)+"-worker", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment: %v", err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 0 {
		t.Errorf("réplicas = %v; se esperaban 0", dep.Spec.Replicas)
	}
}

// strings40 devuelve un digest de relleno con la longitud de un sha256.
func strings40() string {
	const hex = "0123456789abcdef"
	out := make([]byte, 64)
	for i := range out {
		out[i] = hex[i%len(hex)]
	}
	return string(out)
}
