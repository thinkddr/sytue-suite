// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

func reglasSpec() AppSpec {
	return AppSpec{
		Tenant: "acme", Namespace: "tenant-acme", ProjectSlug: "web",
		DeploymentID: "44444444-4444-4444-4444-444444444444",
		Target:       domain.TargetProduction,
		HTTPRules: domain.HTTPRules{
			Redirects: []domain.Redirect{
				{From: "/viejo", To: "/nuevo"},
				{From: "/blog/*", To: "/noticias/", Status: 301},
			},
			Headers: map[string]string{"X-Frame-Options": "DENY"},
		},
	}
}

// Un middleware por redirección y uno para las cabeceras, y el ORDEN importa: si
// una petición se va por una redirección, no hay respuesta a la que añadir nada.
func TestEnsureHTTPRulesCreaYOrdena(t *testing.T) {
	p, _ := newTestPlatform()
	ctx := context.Background()

	refs, err := p.ensureHTTPRules(ctx, reglasSpec())
	if err != nil {
		t.Fatalf("ensureHTTPRules: %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("referencias = %d (%v); se esperaban 3 (2 redirecciones + 1 cabeceras)", len(refs), refs)
	}
	// Las cabeceras van LAS ÚLTIMAS.
	if !strings.HasSuffix(refs[2], "-h@kubernetescrd") {
		t.Errorf("la última referencia debería ser la de cabeceras: %q", refs[2])
	}
	// Traefik solo entiende esta forma; con otra, el Ingress apuntaría a un
	// middleware inexistente y la regla no se aplicaría sin que nada fallara.
	for _, r := range refs {
		if !strings.HasPrefix(r, "tenant-acme-") || !strings.HasSuffix(r, "@kubernetescrd") {
			t.Errorf("referencia con formato inesperado: %q", r)
		}
	}
}

// Idempotente: el reconciliador pasa por aquí en cada ciclo, y una segunda
// llamada tiene que actualizar en vez de fallar por «ya existe».
func TestEnsureHTTPRulesEsIdempotente(t *testing.T) {
	p, _ := newTestPlatform()
	ctx := context.Background()
	spec := reglasSpec()

	if _, err := p.ensureHTTPRules(ctx, spec); err != nil {
		t.Fatalf("primera pasada: %v", err)
	}
	if _, err := p.ensureHTTPRules(ctx, spec); err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}

	lista, err := p.dyn.Resource(traefikMiddlewareGVR).Namespace("tenant-acme").
		List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("listar middlewares: %v", err)
	}
	if len(lista.Items) != 3 {
		t.Errorf("middlewares = %d; se esperaban 3 (la segunda pasada no debe duplicar)", len(lista.Items))
	}
}

// Sin reglas no se toca el clúster: es el caso de la inmensa mayoría de
// proyectos, y tiene que salir gratis.
func TestEnsureHTTPRulesSinReglasNoHaceNada(t *testing.T) {
	p, _ := newTestPlatform()
	refs, err := p.ensureHTTPRules(context.Background(), AppSpec{
		Tenant: "acme", Namespace: "tenant-acme",
		DeploymentID: "55555555-5555-5555-5555-555555555555",
	})
	if err != nil || refs != nil {
		t.Errorf("sin reglas: refs=%v err=%v; se esperaba nada", refs, err)
	}
}
