// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// CRUD de la taxonomía de etiquetas contra Postgres real + AISLAMIENTO POR ORG: una
// etiqueta de otra organización no se lista, ni se lee, ni se edita, ni se borra.
func TestSensitivityLabelsCRUDAndOrgIsolation(t *testing.T) {
	st, ctx := newStore(t)

	acme, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrg(acme): %v", err)
	}
	otra, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "otra", Name: "Otra"})
	if err != nil {
		t.Fatalf("CreateOrg(otra): %v", err)
	}

	// Alta desordenada: el listado debe salir por rank ascendente.
	mk := func(orgID, name string, rank int, block bool) domain.SensitivityLabel {
		l, err := st.CreateSensitivityLabel(ctx, nil, domain.SensitivityLabel{
			OrgID: orgID, Name: name, Rank: rank, BlockExternalShare: block,
		})
		if err != nil {
			t.Fatalf("CreateSensitivityLabel(%s): %v", name, err)
		}
		return l
	}
	conf := mk(acme.ID, "Confidencial", 30, true)
	mk(acme.ID, "Público", 0, false)
	mk(acme.ID, "Interno", 10, false)
	ajena := mk(otra.ID, "Secreta de otra", 99, true)

	list, err := st.ListSensitivityLabels(ctx, acme.ID)
	if err != nil {
		t.Fatalf("ListSensitivityLabels: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("acme debería ver 3 etiquetas (las suyas), got %d", len(list))
	}
	if list[0].Name != "Público" || list[1].Name != "Interno" || list[2].Name != "Confidencial" {
		t.Fatalf("orden por rank incorrecto: %v", []string{list[0].Name, list[1].Name, list[2].Name})
	}
	if !list[2].BlockExternalShare {
		t.Fatalf("Confidencial debería bloquear la compartición externa")
	}

	// Nombre repetido en la MISMA org (case-insensitive) ⇒ ErrConflict.
	if _, err := st.CreateSensitivityLabel(ctx, nil, domain.SensitivityLabel{
		OrgID: acme.ID, Name: "confidencial",
	}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("nombre duplicado: se esperaba ErrConflict, got %v", err)
	}
	// El MISMO nombre en OTRA org sí vale (taxonomías independientes).
	if _, err := st.CreateSensitivityLabel(ctx, nil, domain.SensitivityLabel{
		OrgID: otra.ID, Name: "Confidencial",
	}); err != nil {
		t.Fatalf("mismo nombre en otra org debería valer: %v", err)
	}

	// Get acotado por org: la propia sí, la ajena NO (404 sin fuga cross-tenant).
	if got, err := st.GetSensitivityLabel(ctx, acme.ID, conf.ID); err != nil || got.Name != "Confidencial" {
		t.Fatalf("GetSensitivityLabel(propia): %+v %v", got, err)
	}
	if _, err := st.GetSensitivityLabel(ctx, acme.ID, ajena.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("etiqueta ajena: se esperaba ErrNotFound, got %v", err)
	}
	if _, err := st.GetSensitivityLabel(ctx, acme.ID, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("etiqueta inexistente: se esperaba ErrNotFound, got %v", err)
	}

	// Update acotado por org.
	conf.Name = "Confidencial (interna)"
	conf.BlockExternalShare = false
	upd, err := st.UpdateSensitivityLabel(ctx, nil, conf)
	if err != nil {
		t.Fatalf("UpdateSensitivityLabel: %v", err)
	}
	if upd.Name != "Confidencial (interna)" || upd.BlockExternalShare {
		t.Fatalf("update no aplicó: %+v", upd)
	}
	ajenaEnAcme := ajena
	ajenaEnAcme.OrgID = acme.ID
	ajenaEnAcme.Name = "secuestrada"
	if _, err := st.UpdateSensitivityLabel(ctx, nil, ajenaEnAcme); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("update cross-org: se esperaba ErrNotFound, got %v", err)
	}
	if got, err := st.GetSensitivityLabel(ctx, otra.ID, ajena.ID); err != nil || got.Name != "Secreta de otra" {
		t.Fatalf("la etiqueta ajena no debió cambiar: %+v %v", got, err)
	}

	// Delete acotado por org.
	if ok, err := st.DeleteSensitivityLabel(ctx, acme.ID, ajena.ID); err != nil || ok {
		t.Fatalf("delete cross-org debería no borrar nada: ok=%v err=%v", ok, err)
	}
	if ok, err := st.DeleteSensitivityLabel(ctx, acme.ID, conf.ID); err != nil || !ok {
		t.Fatalf("DeleteSensitivityLabel: ok=%v err=%v", ok, err)
	}
	if _, err := st.GetSensitivityLabel(ctx, acme.ID, conf.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("borrada: se esperaba ErrNotFound, got %v", err)
	}
}
