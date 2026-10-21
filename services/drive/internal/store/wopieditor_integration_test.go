// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	db "git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// TestEditorEsDeSuOrganizacion: el editor lo elige cada organización, así que dos que
// conviven en la misma instalación no pueden verse ni pisarse el ajuste.
func TestEditorEsDeSuOrganizacion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	acme, otra := tenantCtx("acme"), tenantCtx("otra")

	if _, err := st.SaveWopiEditor(acme, store.WopiEditor{
		DiscoveryURL: "https://office.acme.test/hosting/discovery",
		Label:        "Collabora de Acme",
		Enabled:      true,
		CheckOK:      true,
		Extensions:   []string{"docx", "xlsx"},
	}, "u1"); err != nil {
		t.Fatalf("guardar: %v", err)
	}

	got, err := st.GetWopiEditor(acme)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if got.DiscoveryURL != "https://office.acme.test/hosting/discovery" || !got.CheckOK {
		t.Fatalf("editor guardado = %+v", got)
	}
	if len(got.Extensions) != 2 {
		t.Fatalf("las extensiones no sobrevivieron: %v", got.Extensions)
	}
	if _, err := st.GetWopiEditor(otra); err != domain.ErrNotFound {
		t.Fatal("otra organización vio el editor de Acme")
	}
}

// TestEditorSeReemplazaNoSeDuplica: reconfigurar deja UNA fila. Con dos, cuál gana
// dependería del orden de la consulta.
func TestEditorSeReemplazaNoSeDuplica(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	for _, url := range []string{"https://uno.test/hosting/discovery", "https://dos.test/hosting/discovery"} {
		if _, err := st.SaveWopiEditor(ctx, store.WopiEditor{DiscoveryURL: url, Enabled: true}, "u1"); err != nil {
			t.Fatalf("guardar %s: %v", url, err)
		}
	}
	got, err := st.GetWopiEditor(ctx)
	if err != nil {
		t.Fatalf("leer: %v", err)
	}
	if got.DiscoveryURL != "https://dos.test/hosting/discovery" {
		t.Fatalf("gana el último: %+v", got)
	}
}

// TestEditorSeResuelveSinSesion es la que importa para el host WOPI: el editor vuelve con
// su propio token y en esa petición no hay JWT del que sacar la organización. Si esta
// lectura no funcionara, abrir un documento fallaría justo en el paso final.
func TestEditorSeResuelveSinSesion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	if _, err := st.SaveWopiEditor(tenantCtx("acme"), store.WopiEditor{
		DiscoveryURL: "https://office.acme.test/hosting/discovery", Enabled: true,
	}, "u1"); err != nil {
		t.Fatalf("guardar: %v", err)
	}

	// Contexto SIN tenant: es el del host WOPI.
	e, err := st.WopiEditorFor(context.Background(), "acme")
	if err != nil {
		t.Fatalf("resolver sin sesión: %v", err)
	}
	if e.DiscoveryURL != "https://office.acme.test/hosting/discovery" {
		t.Fatalf("editor = %+v", e)
	}
	// Una organización sin editor no hereda el de otra.
	if _, err := st.WopiEditorFor(context.Background(), "otra"); err != domain.ErrNotFound {
		t.Fatalf("otra organización = %v, quiero ErrNotFound", err)
	}
}

// TestEditorEnPausaNoSeSirve: apagarlo sin perder la configuración es justo el motivo de
// tener una columna `enabled` en vez de borrar la fila.
func TestEditorEnPausaNoSeSirve(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.SaveWopiEditor(ctx, store.WopiEditor{
		DiscoveryURL: "https://office.acme.test/hosting/discovery", Enabled: false,
	}, "u1"); err != nil {
		t.Fatalf("guardar: %v", err)
	}
	// La consola SÍ lo ve —hay que poder volver a encenderlo—…
	if _, err := st.GetWopiEditor(ctx); err != nil {
		t.Fatalf("la consola debe seguir viéndolo: %v", err)
	}
	// …pero el host WOPI no lo usa.
	if _, err := st.WopiEditorFor(context.Background(), "acme"); err != domain.ErrNotFound {
		t.Fatalf("un editor en pausa se sirvió igual; err=%v", err)
	}
}
