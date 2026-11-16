// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// El aviso de una aprobación llega a quien después puede pulsar «Aprobar», con
// las mismas reglas que la ruta, y nunca a quien los cuatro ojos dejan fuera.
func TestElAvisoDeAprobacionLlegaSoloAQuienPuedeAprobar(t *testing.T) {
	sinAprobar := []string{"crear", "asentar"}
	conAprobar := []string{"aprobar"}
	cs := []store.Candidato{
		{Sub: "duena", Roles: []string{"owner"}},                          // admin de la org: aprueba por defecto
		{Sub: "pide", Roles: []string{"admin"}},                           // la pide: fuera
		{Sub: "creo", Nivel: "administrador"},                             // creó el documento: fuera
		{Sub: "jefa", Nivel: "administrador"},                             // administra contabilidad
		{Sub: "becario", Nivel: "usuario"},                                // asienta, no aprueba
		{Sub: "auditora", Nivel: "usuario", Verbos: &conAprobar},          // se lo pusieron a mano
		{Sub: "admin-sin", Roles: []string{"admin"}, Verbos: &sinAprobar}, // se lo quitaron
	}
	got := aprobadores(cs, moduloAprobador, "pide", "creo")
	want := []string{"duena", "jefa", "auditora"}
	if !slices.Equal(got, want) {
		t.Fatalf("aprobadores = %v, quiero %v", got, want)
	}
}

func TestElAvisoDeAprobacionDiceQuienQueYCuantoYAbreLaCola(t *testing.T) {
	a := store.Aprobacion{ID: "ap1", Importe: 5000050, Moneda: "EUR"}
	n := avisoDeAprobacion("https://erp.sytue.eu", "acme", "jefa", "Luis", "FAC-2026-0042", a)
	if n.Subject != "jefa" || n.Tenant != "acme" || n.Source != "erp" || n.Type != "erp.approval.requested" {
		t.Fatalf("aviso = %+v", n)
	}
	if n.URL != "https://erp.sytue.eu/erp/acme/?vista=aprobaciones" {
		t.Fatalf("enlace = %q", n.URL)
	}
	if n.Params["actor"] != "Luis" || n.Params["name"] != "FAC-2026-0042" || n.Body != "50000.50 EUR" {
		t.Fatalf("variables = %+v, cuerpo %q", n.Params, n.Body)
	}
}
