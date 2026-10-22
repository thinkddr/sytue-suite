// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// TestCambiarElEstadoPideElVerboDelGesto (NEG-11): quien solo puede crear
// facturas no las emite, anula ni da por pagadas cambiando el estado a mano.
func TestCambiarElEstadoPideElVerboDelGesto(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	hs := manejadoresCompletos(t, st)
	miembro := []string{"member"}

	w := comoPersona(t, "jefa", []string{"owner"}, http.MethodPut, "/api/v1/admin/permisos/luis",
		`{"email":"luis@acme.es","nombre":"Luis","modulos":{"facturacion":"usuario"},"verbos":{"facturacion":["crear"]}}`,
		map[string]string{"sub": "luis"}, http.HandlerFunc(hs.GuardarPermisos))
	if w.Code != http.StatusNoContent {
		t.Fatalf("guardar permisos: %d %s", w.Code, w.Body.String())
	}
	w = comoPersona(t, "luis", miembro, http.MethodPost, "/api/v1/factura_venta",
		`{"data":{"cliente":"`+nuevoContacto(t, hs)+`","fecha":"2026-09-12","base":1000,"iva":210,"total":1210}}`,
		map[string]string{"modelo": "factura_venta"}, hs.exigirModelo(http.HandlerFunc(hs.Crear)))
	if w.Code != http.StatusCreated {
		t.Fatalf("crear: %d %s", w.Code, w.Body.String())
	}
	id := between(w.Body.String(), `"id":"`, `"`)

	for estado, verbo := range map[string]string{"confirmado": "asentar", "anulado": "asentar", "pagado": "pagar"} {
		w = comoPersona(t, "luis", miembro, http.MethodPost, "/api/v1/factura_venta/"+id+"/estado", `{"estado":"`+estado+`"}`,
			map[string]string{"modelo": "factura_venta", "id": id}, hs.exigirModelo(http.HandlerFunc(hs.CambiarEstado)))
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "sin_verbo_"+verbo) {
			t.Errorf("/estado %s sin %s: %d %s", estado, verbo, w.Code, w.Body.String())
		}
		w = comoPersona(t, "luis", miembro, http.MethodPost, "/api/v1/factura_venta/lote",
			`{"accion":"estado","ids":["`+id+`"],"estado":"`+estado+`"}`,
			map[string]string{"modelo": "factura_venta"}, hs.exigirModelo(http.HandlerFunc(hs.Lote)))
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "sin_verbo_"+verbo) {
			t.Errorf("/lote %s sin %s: %d %s", estado, verbo, w.Code, w.Body.String())
		}
	}

	// Lo que no es un gesto contable sigue igual: un borrador vuelve a borrador.
	w = comoPersona(t, "luis", miembro, http.MethodPost, "/api/v1/factura_venta/"+id+"/estado", `{"estado":"borrador"}`,
		map[string]string{"modelo": "factura_venta", "id": id}, hs.exigirModelo(http.HandlerFunc(hs.CambiarEstado)))
	if w.Code != http.StatusOK {
		t.Fatalf("a borrador: %d %s", w.Code, w.Body.String())
	}
}

func nuevoContacto(t *testing.T, hs *Handlers) string {
	t.Helper()
	w := comoPersona(t, "jefa", []string{"owner"}, http.MethodPost, "/api/v1/contacto",
		`{"data":{"nombre":"Suministros Acme S.L.","tipo":"organizacion"}}`,
		map[string]string{"modelo": "contacto"}, hs.exigirModelo(http.HandlerFunc(hs.Crear)))
	if w.Code != http.StatusCreated {
		t.Fatalf("contacto: %d %s", w.Code, w.Body.String())
	}
	return between(w.Body.String(), `"id":"`, `"`)
}

func between(s, desde, hasta string) string {
	_, resto, _ := strings.Cut(s, desde)
	v, _, _ := strings.Cut(resto, hasta)
	return v
}

// TestNadaSeLeeDeLadoDesdeOtroModulo (NEG-17): quien solo lee contactos no ve los
// asientos de una factura pidiéndolos por la ruta de contactos, ni los importes
// de la cola de aprobaciones de facturación.
func TestNadaSeLeeDeLadoDesdeOtroModulo(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	hs := manejadoresCompletos(t, st)
	jefa := []string{"owner"}
	proveedor := nuevoContacto(t, hs)

	w := comoPersona(t, "jefa", jefa, http.MethodPost, "/api/v1/factura_compra",
		`{"data":{"proveedor":"`+proveedor+`","fecha":"2026-09-12","base":200000,"iva":42000,"total":242000}}`,
		map[string]string{"modelo": "factura_compra"}, hs.exigirModelo(http.HandlerFunc(hs.Crear)))
	if w.Code != http.StatusCreated {
		t.Fatalf("crear: %d %s", w.Code, w.Body.String())
	}
	id := between(w.Body.String(), `"id":"`, `"`)
	w = comoPersona(t, "jefa", jefa, http.MethodPost, "/api/v1/factura_compra/"+id+"/asentar", "",
		map[string]string{"modelo": "factura_compra", "id": id}, hs.exigirModelo(http.HandlerFunc(hs.Asentar)))
	if w.Code != http.StatusCreated {
		t.Fatalf("asentar: %d %s", w.Code, w.Body.String())
	}
	w = comoPersona(t, "jefa", jefa, http.MethodGet, "/api/v1/factura_compra/"+id+"/asientos", "",
		map[string]string{"modelo": "factura_compra", "id": id}, hs.exigirModelo(http.HandlerFunc(hs.AsientosDe)))
	if !strings.Contains(w.Body.String(), `"diario"`) {
		t.Fatalf("por su ruta, los asientos se ven: %s", w.Body.String())
	}
	w = comoPersona(t, "luis", []string{"member"}, http.MethodGet, "/api/v1/contacto/"+id+"/asientos", "",
		map[string]string{"modelo": "contacto", "id": id}, hs.exigirModelo(http.HandlerFunc(hs.AsientosDe)))
	if strings.Contains(w.Body.String(), `"diario"`) {
		t.Fatalf("por la ruta de contactos, los asientos de una factura: %s", w.Body.String())
	}

	otra := comoPersona(t, "jefa", jefa, http.MethodPost, "/api/v1/factura_compra",
		`{"data":{"proveedor":"`+proveedor+`","fecha":"2026-09-12","base":200000,"iva":42000,"total":242000}}`,
		map[string]string{"modelo": "factura_compra"}, hs.exigirModelo(http.HandlerFunc(hs.Crear)))
	otroID := between(otra.Body.String(), `"id":"`, `"`)
	w = comoPersona(t, "jefa", jefa, http.MethodPost, "/api/v1/factura_compra/"+otroID+"/aprobacion", "",
		map[string]string{"modelo": "factura_compra", "id": otroID}, hs.exigirModelo(http.HandlerFunc(hs.PedirAprobacion)))
	if w.Code != http.StatusCreated {
		t.Fatalf("pedir aprobación: %d %s", w.Code, w.Body.String())
	}
	cola := func(quien string, roles []string) string {
		return comoPersona(t, quien, roles, http.MethodGet, "/api/v1/aprobaciones", "", nil, http.HandlerFunc(hs.Aprobaciones)).Body.String()
	}
	if !strings.Contains(cola("jefa", jefa), otroID) {
		t.Fatal("quien lee facturación ve la cola")
	}
	if strings.Contains(cola("luis", []string{"member"}), otroID) {
		t.Fatal("quien no lee facturación no ve sus importes en la cola")
	}
}
