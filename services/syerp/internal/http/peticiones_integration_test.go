// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"encoding/json"
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// El permiso de «Pedir acceso de edición» de un módulo: pide quien tiene `lectura` (no
// quien no tiene nada); `mis-permisos` lo dice; la administración concede y el nivel
// pasa a `usuario`; y no se concede a quien no lo pidió.
func TestPedirYConcederEdicionDeUnModulo(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	hs := manejadoresCompletos(t, st)
	miembro := []string{"member"}
	fac := map[string]string{"modulo": "facturacion"}

	if w := comoPersona(t, "luis", miembro, http.MethodPost, "/", "", fac, http.HandlerFunc(hs.PedirEdicion)); w.Code != http.StatusForbidden {
		t.Fatalf("sin acceso al módulo pidió editarlo: %d", w.Code)
	}
	if w := comoPersona(t, "jefa", []string{"owner"}, http.MethodPut, "/", `{"email":"luis@acme.es","nombre":"Luis","modulos":{"facturacion":"lectura"}}`,
		map[string]string{"sub": "luis"}, http.HandlerFunc(hs.GuardarPermisos)); w.Code != http.StatusNoContent {
		t.Fatalf("guardar permisos: %d", w.Code)
	}
	if w := comoPersona(t, "luis", miembro, http.MethodPost, "/", "", fac, http.HandlerFunc(hs.PedirEdicion)); w.Code != http.StatusOK {
		t.Fatalf("con lectura no pudo pedir: %d %s", w.Code, w.Body)
	}
	mis := func() (nivel string, pedidos []string) {
		w := comoPersona(t, "luis", miembro, http.MethodGet, "/", "", nil, http.HandlerFunc(hs.MisPermisos))
		var r struct {
			Modulos map[string]string `json:"modulos"`
			Pedidos []string          `json:"pedidos"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &r)
		return r.Modulos["facturacion"], r.Pedidos
	}
	if n, p := mis(); n != "lectura" || len(p) != 1 || p[0] != "facturacion" {
		t.Fatalf("mis-permisos: %q %v", n, p)
	}
	jefa := func(sub string) int {
		return comoPersona(t, "jefa", []string{"owner"}, http.MethodPost, "/", "", map[string]string{"modulo": "facturacion", "sub": sub},
			http.HandlerFunc(hs.ConcederPeticion)).Code
	}
	if c := jefa("nadie"); c != http.StatusNotFound {
		t.Errorf("concedió a quien no lo pidió: %d", c)
	}
	if c := jefa("luis"); c != http.StatusNoContent {
		t.Fatalf("conceder: %d", c)
	}
	if n, p := mis(); n != "usuario" || len(p) != 0 {
		t.Errorf("tras conceder, Luis tiene %q y pedidos %v", n, p)
	}
}
