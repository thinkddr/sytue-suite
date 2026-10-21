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

// La propiedad que importa: la contraseña cierra las PREVIEWS y nunca producción.
// Equivocarse en esa dirección tira el sitio del cliente detrás de una ventana de
// credenciales, así que se prueba explícitamente el caso de producción.
func TestPreviewAuthSoloEnPreviews(t *testing.T) {
	p, cs := newTestPlatform()
	ctx := context.Background()

	prod := AppSpec{
		Tenant: "acme", Namespace: "tenant-acme", ProjectSlug: "web",
		DeploymentID:    "11111111-1111-1111-1111-111111111111",
		Target:          domain.TargetProduction,
		PreviewAuthHash: "preview:$2y$10$abcdefghijklmnopqrstuv",
	}
	ref, err := p.ensurePreviewAuth(ctx, prod)
	if err != nil {
		t.Fatalf("ensurePreviewAuth(producción): %v", err)
	}
	if ref != "" {
		t.Errorf("producción NO puede quedar detrás de una contraseña; middleware = %q", ref)
	}
	if _, err := cs.CoreV1().Secrets("tenant-acme").
		Get(ctx, previewAuthName(prod), metav1.GetOptions{}); err == nil {
		t.Error("se creó el Secret de autenticación para una producción")
	}
}

func TestPreviewAuthCierraLaPreview(t *testing.T) {
	p, cs := newTestPlatform()
	ctx := context.Background()

	spec := AppSpec{
		Tenant: "acme", Namespace: "tenant-acme", ProjectSlug: "web",
		DeploymentID:    "22222222-2222-2222-2222-222222222222",
		Target:          domain.TargetPreview,
		PreviewAuthHash: "preview:$2y$10$abcdefghijklmnopqrstuv",
	}
	ref, err := p.ensurePreviewAuth(ctx, spec)
	if err != nil {
		t.Fatalf("ensurePreviewAuth: %v", err)
	}
	// Traefik solo entiende esta forma; con otra, el Ingress apuntaría a un
	// middleware inexistente y la preview saldría ABIERTA sin que nada fallara.
	if want := "tenant-acme-" + previewAuthName(spec) + "@kubernetescrd"; ref != want {
		t.Errorf("referencia del middleware = %q, quiero %q", ref, want)
	}

	sec, err := cs.CoreV1().Secrets("tenant-acme").
		Get(ctx, previewAuthName(spec), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Secret htpasswd: %v", err)
	}
	// La clave TIENE que llamarse `users`: es la que lee el basicAuth de Traefik.
	users := string(sec.Data["users"])
	if !strings.HasPrefix(users, domain.PreviewAuthUser+":$2") {
		t.Errorf("htpasswd = %q; se esperaba «usuario:bcrypt»", users)
	}

	// Idempotente: el reconciliador pasa por aquí en cada ciclo.
	if _, err := p.ensurePreviewAuth(ctx, spec); err != nil {
		t.Errorf("segunda pasada: %v", err)
	}

	// Y al retirar la preview no queda nada suelto en el namespace del cliente.
	p.removePreviewAuth(ctx, "tenant-acme", spec.DeploymentID)
	if _, err := cs.CoreV1().Secrets("tenant-acme").
		Get(ctx, previewAuthName(spec), metav1.GetOptions{}); err == nil {
		t.Error("el Secret de la preview sigue ahí tras retirarla")
	}
}

// Sin hash no hay protección: es el caso de todos los proyectos que no la han
// activado, y tiene que salir barato (ni una llamada al clúster).
func TestPreviewAuthSinHashNoHaceNada(t *testing.T) {
	p, _ := newTestPlatform()
	ref, err := p.ensurePreviewAuth(context.Background(), AppSpec{
		Tenant: "acme", Namespace: "tenant-acme",
		DeploymentID: "33333333-3333-3333-3333-333333333333",
		Target:       domain.TargetPreview,
	})
	if err != nil || ref != "" {
		t.Errorf("sin hash: ref=%q err=%v; se esperaba vacío y sin error", ref, err)
	}
}
