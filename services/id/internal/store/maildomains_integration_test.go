// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de LOS dominios de correo de una organización (mig 0052),
// contra PostgreSQL real. Lo que se fija aquí es lo que hace que tener varios sea
// distinto de tener uno: que añadir el segundo no toque al primero, que solo un
// dominio verificado pueda estrenarse, que ninguna organización se quede con
// dominios propios y ninguno principal, y que un dominio verificado no se lo
// pueda llevar otra organización.
package store_test

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// reclamado añade un dominio y devuelve su fila.
func reclamado(t *testing.T, ctx context.Context, st *store.Store, orgID, host string) store.OrgMailDomain {
	t.Helper()
	d, err := st.AddOrgMailDomain(ctx, nil, orgID, host, "tok-"+host)
	if err != nil {
		t.Fatalf("AddOrgMailDomain(%q): %v", host, err)
	}
	return d
}

// verificado añade un dominio y sella su titularidad.
func verificado(t *testing.T, ctx context.Context, st *store.Store, orgID, host string) store.OrgMailDomain {
	t.Helper()
	reclamado(t, ctx, st, orgID, host)
	if err := st.MarkOrgMailDomainVerified(ctx, nil, orgID, host); err != nil {
		t.Fatalf("MarkOrgMailDomainVerified(%q): %v", host, err)
	}
	for _, d := range listados(t, ctx, st, orgID) {
		if d.Domain == host {
			return d
		}
	}
	t.Fatalf("el dominio %q no aparece tras verificarlo", host)
	return store.OrgMailDomain{}
}

func listados(t *testing.T, ctx context.Context, st *store.Store, orgID string) []store.OrgMailDomain {
	t.Helper()
	lista, err := st.ListOrgMailDomains(ctx, orgID)
	if err != nil {
		t.Fatalf("ListOrgMailDomains: %v", err)
	}
	return lista
}

func TestVariosDominiosDeCorreo(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")
	org := orgConBuzones(t, ctx, st, "acme")

	// El PRIMERO verificado se estrena solo: si no, la organización tendría un
	// dominio propio conectado y sus altas seguirían naciendo en el subdominio de
	// la suite sin que nada lo explicara.
	primero := verificado(t, ctx, st, org.ID, "acme.eu")
	if !primero.Primary {
		t.Fatal("el primer dominio verificado tiene que quedar como principal")
	}
	if principal, _, err := st.MailDomainsForOrg(ctx, nil, org.ID); err != nil || principal != "acme.eu" {
		t.Fatalf("MailDomainsForOrg = %q/%v, quiero acme.eu", principal, err)
	}

	// El SEGUNDO no mueve a nadie. Es toda la diferencia con lo que había antes:
	// conectar otro dominio no puede cambiar la dirección de la plantilla.
	segundo := verificado(t, ctx, st, org.ID, "acme.es")
	if segundo.Primary {
		t.Fatal("añadir un dominio no puede robarle el puesto al principal")
	}
	if principal, _, _ := st.MailDomainsForOrg(ctx, nil, org.ID); principal != "acme.eu" {
		t.Fatalf("el principal cambió solo al añadir otro dominio: %q", principal)
	}

	// Y los dos viajan en el token, que es lo que deja a Sytue Mail reconocer una
	// dirección en el segundo dominio como propia y no como ajena.
	dominios, err := st.VerifiedOrgMailDomains(ctx, nil, org.ID)
	if err != nil || len(dominios) != 2 {
		t.Fatalf("VerifiedOrgMailDomains = %v/%v, quiero los dos", dominios, err)
	}

	// Un dominio sin verificar NO se puede estrenar: repartir direcciones en él
	// crearía correo que nuestro MX no sirve y que pisa el del dueño real.
	pendiente := reclamado(t, ctx, st, org.ID, "acme.example")
	if _, err := st.SetPrimaryOrgMailDomain(ctx, org.ID, pendiente.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("estrenar un dominio sin verificar = %v, quiero ErrNotFound", err)
	}

	// Cambiar el principal: exactamente uno queda marcado.
	if _, err := st.SetPrimaryOrgMailDomain(ctx, org.ID, segundo.ID); err != nil {
		t.Fatalf("SetPrimaryOrgMailDomain: %v", err)
	}
	principales := 0
	for _, d := range listados(t, ctx, st, org.ID) {
		if d.Primary {
			principales++
			if d.Domain != "acme.es" {
				t.Fatalf("el principal es %q, quiero acme.es", d.Domain)
			}
		}
	}
	if principales != 1 {
		t.Fatalf("hay %d dominios principales, tiene que haber exactamente 1", principales)
	}

	// Quitar el principal deja el puesto al verificado más antiguo. Sin esto la
	// organización se queda con dominios propios y las altas nuevas volviendo en
	// silencio al subdominio de la suite.
	if err := st.DeleteOrgMailDomain(ctx, org.ID, segundo.ID); err != nil {
		t.Fatalf("DeleteOrgMailDomain: %v", err)
	}
	if principal, _, _ := st.MailDomainsForOrg(ctx, nil, org.ID); principal != "acme.eu" {
		t.Fatalf("tras quitar el principal quedó %q, quiero que herede acme.eu", principal)
	}

	// Reclamar no prueba nada, pero VERIFICAR sí: el dominio de otra organización
	// no se puede reclamar una vez está verificado.
	otra := orgConBuzones(t, ctx, st, "otra")
	if _, err := st.AddOrgMailDomain(ctx, nil, otra.ID, "acme.eu", "tok"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("reclamar el dominio verificado de otra = %v, quiero ErrConflict", err)
	}
	// El suyo sin verificar sí puede coexistir: quien publique el TXT se lo queda.
	if _, err := st.AddOrgMailDomain(ctx, nil, otra.ID, "acme.example", "tok"); err != nil {
		t.Fatalf("reclamar un dominio que otra tiene SIN verificar: %v", err)
	}

	// Y una dirección solo se asigna en un dominio de la organización: con varios,
	// componerla con el principal ya no basta como comprobación.
	ada := usuarioConNombre(t, ctx, st, "ada@correo-personal.example", "Ada", "Lovelace")
	if err := st.UpsertMembership(ctx, nil, domain.Membership{
		OrgID: org.ID, UserID: ada.ID, Role: "member",
	}); err != nil {
		t.Fatalf("UpsertMembership: %v", err)
	}
	if _, err := st.AddMailboxAlias(ctx, org.ID, ada.ID, "ventas@acme.eu"); err != nil {
		t.Fatalf("alias en un dominio propio: %v", err)
	}
	if _, err := st.AddMailboxAlias(ctx, org.ID, ada.ID, "ada@dominio-ajeno.example"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("alias en un dominio ajeno = %v, quiero ErrInvalidInput", err)
	}
}
