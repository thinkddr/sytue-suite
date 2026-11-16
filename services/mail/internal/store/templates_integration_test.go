// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Plantillas de respuesta en el SERVIDOR (tabla mail_templates, migración 0021):
// CRUD por cuenta, nombre único sin distinguir mayúsculas, tope por cuenta y
// aislamiento entre cuentas y organizaciones.
package store_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

func TestTemplatesCRUDPorCuenta(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ada := seedAccount(ctx, t, st, "ada@sytue.eu")
	bob := seedAccount(ctx, t, st, "bob@sytue.eu")

	horario, err := st.CreateTemplate(ctx, ada.ID, "  Horario ", "De 9 a 14.\n")
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if horario.Name != "Horario" || horario.Body != "De 9 a 14.\n" {
		t.Fatalf("creada = %+v; quiero el nombre recortado y el cuerpo intacto", horario)
	}
	if _, err := st.CreateTemplate(ctx, ada.ID, "agradecimiento", "Gracias."); err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	// Mismo nombre con otras mayúsculas: no dos «Horario» en el menú.
	if _, err := st.CreateTemplate(ctx, ada.ID, "HORARIO", "x"); !errors.Is(err, domain.ErrTemplateExists) {
		t.Fatalf("nombre repetido: err = %v, quiero ErrTemplateExists", err)
	}
	// Pero otra cuenta sí puede tener su «Horario».
	if _, err := st.CreateTemplate(ctx, bob.ID, "Horario", "De 8 a 15."); err != nil {
		t.Fatalf("otra cuenta, mismo nombre: %v", err)
	}

	lista, err := st.ListTemplates(ctx, ada.ID)
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(lista) != 2 || lista[0].Name != "agradecimiento" || lista[1].Name != "Horario" {
		t.Fatalf("lista = %+v; quiero las dos de Ada por nombre, sin distinguir mayúsculas", lista)
	}

	cambiada, err := st.UpdateTemplate(ctx, ada.ID, horario.ID, "Horario de verano", "De 8 a 15.")
	if err != nil || cambiada.Name != "Horario de verano" || cambiada.Body != "De 8 a 15." {
		t.Fatalf("UpdateTemplate = %+v, %v", cambiada, err)
	}
	if _, err := st.UpdateTemplate(ctx, ada.ID, horario.ID, "Agradecimiento", "x"); !errors.Is(err, domain.ErrTemplateExists) {
		t.Fatalf("renombrar a uno que existe: err = %v", err)
	}
	// La de Ada no se toca con la cuenta de Bob.
	if _, err := st.UpdateTemplate(ctx, bob.ID, horario.ID, "Mía", "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("editar la de otra cuenta: err = %v", err)
	}
	if err := st.DeleteTemplate(ctx, bob.ID, horario.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("borrar la de otra cuenta: err = %v", err)
	}
	if err := st.DeleteTemplate(ctx, ada.ID, horario.ID); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}
	if err := st.DeleteTemplate(ctx, ada.ID, horario.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("borrar dos veces: err = %v", err)
	}
	if lista, _ := st.ListTemplates(ctx, ada.ID); len(lista) != 1 {
		t.Fatalf("tras borrar quedan %d", len(lista))
	}
	if _, err := st.UpdateTemplate(ctx, ada.ID, uuid.New(), "x", ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("editar una que no existe: err = %v", err)
	}
}

func TestTemplatesTopePorCuenta(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	ada := seedAccount(ctx, t, st, "ada@sytue.eu")
	for i := range domain.TemplatesPorCuenta {
		if _, err := st.CreateTemplate(ctx, ada.ID, fmt.Sprintf("P%03d", i), "x"); err != nil {
			t.Fatalf("plantilla %d: %v", i, err)
		}
	}
	if _, err := st.CreateTemplate(ctx, ada.ID, "Una más", "x"); !errors.Is(err, domain.ErrTemplateLimite) {
		t.Fatalf("pasado el tope: err = %v, quiero ErrTemplateLimite", err)
	}
}

func TestTemplatesNoCruzanOrganizaciones(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ada := seedAccount(tenantCtx("acme"), t, st, "ada@sytue.eu")
	tpl, err := st.CreateTemplate(tenantCtx("acme"), ada.ID, "Horario", "De 9 a 14.")
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	otra := tenantCtx("other")
	if lista, err := st.ListTemplates(otra, ada.ID); err != nil || len(lista) != 0 {
		t.Fatalf("otra organización ve %d plantillas (err=%v)", len(lista), err)
	}
	if _, err := st.CreateTemplate(otra, ada.ID, "Intrusa", "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("crear en una cuenta de otra organización: err = %v", err)
	}
	if _, err := st.UpdateTemplate(otra, ada.ID, tpl.ID, "Cambiada", "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("editar desde otra organización: err = %v", err)
	}
	if err := st.DeleteTemplate(otra, ada.ID, tpl.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("borrar desde otra organización: err = %v", err)
	}
	if lista, _ := st.ListTemplates(tenantCtx("acme"), ada.ID); len(lista) != 1 || lista[0].Name != "Horario" {
		t.Fatalf("la plantilla cambió desde otra organización: %+v", lista)
	}
}
