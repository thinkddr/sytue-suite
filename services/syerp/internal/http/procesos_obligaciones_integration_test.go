// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package syerphttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

func TestProcesoObligacionPermisosPendientesYCalendario(t *testing.T) {
	harness := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(harness.Pool)
	hs := manejadoresCompletos(t, st)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	if _, err := st.GuardarOrganizacion(ctx, store.Organizacion{Pais: "ES", CP: "28013", Forma: "sl", Nombre: "Acme", Condiciones: map[string]bool{"iva": true}}, "ada"); err != nil {
		t.Fatal(err)
	}
	ponerPermisos := func(obligaciones, facturacion string) {
		t.Helper()
		if err := st.GuardarPermisos(ctx, "luis", "luis@acme.es", "Luis", map[string]string{"obligaciones": obligaciones, "facturacion": facturacion}, nil, "ada"); err != nil {
			t.Fatal(err)
		}
	}
	miembro := []string{"member"}
	ruta := "/api/v1/obligaciones/proceso?clave=es_303&periodo=2026-T3"
	pendientes := "/api/v1/obligaciones/proceso/pendientes?clave=es_303&periodo=2026-T3"
	procesoHandler := hs.exigir("obligaciones", "lectura")(http.HandlerFunc(hs.ProcesoObligacion))
	pendientesHandler := hs.exigir("obligaciones", "lectura")(http.HandlerFunc(hs.PendientesProcesoObligacion))
	calendarioHandler := hs.exigir("obligaciones", "lectura")(http.HandlerFunc(hs.Obligaciones))
	ponerPermisos("lectura", "ninguno")
	if w := comoPersona(t, "luis", miembro, http.MethodGet, pendientes, "", nil, pendientesHandler); w.Code != http.StatusForbidden {
		t.Fatalf("obligaciones no concede lectura de facturas: %d %s", w.Code, w.Body.String())
	}
	if w := comoPersona(t, "luis", miembro, http.MethodPost, ruta, `{"fase":1}`, nil, procesoHandler); w.Code != http.StatusForbidden {
		t.Fatalf("lectura no avanza el proceso: %d %s", w.Code, w.Body.String())
	}
	ponerPermisos("usuario", "lectura")
	if err := harness.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO records (tenant,model,estado,data,owner_sub)
			VALUES ('acme','factura_venta','confirmado','{"fecha":"2026-09-10"}','ada'),
			('acme','factura_compra','pagado','{"fecha":"2026-09-11"}','ada')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	w := comoPersona(t, "luis", miembro, http.MethodGet, pendientes+"&limite=1", "", nil, pendientesHandler)
	var pagina store.Pagina
	if err := json.Unmarshal(w.Body.Bytes(), &pagina); w.Code != http.StatusOK || err != nil || len(pagina.Registros) != 1 || pagina.Siguiente == "" {
		t.Fatalf("página de pendientes: %d %s %v", w.Code, w.Body.String(), err)
	}
	if w := comoPersona(t, "luis", miembro, http.MethodPost, ruta, `{"fase":1}`, nil, procesoHandler); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("no avanza con facturas confirmadas sin asiento: %d %s", w.Code, w.Body.String())
	}
	if w := comoPersona(t, "luis", miembro, http.MethodGet, pendientes+"&cursor=incorrecto", "", nil, pendientesHandler); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cursor inválido: %d %s", w.Code, w.Body.String())
	}
	if err := harness.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE records SET estado='anulado' WHERE tenant='acme'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for fase := 1; fase <= 3; fase++ {
		w := comoPersona(t, "luis", miembro, http.MethodPost, ruta, fmt.Sprintf(`{"fase":%d}`, fase), nil, procesoHandler)
		if w.Code != http.StatusOK {
			t.Fatalf("avanzar fase %d: %d %s", fase, w.Code, w.Body.String())
		}
	}
	a, _, err := st.GuardarAdjunto(ctx, "obligacion", "es_303:2026-T3:justificante", "documento.pdf", "application/pdf", []byte("%PDF-1.7 prueba"), "luis")
	if err != nil {
		t.Fatal(err)
	}
	if w := comoPersona(t, "luis", miembro, http.MethodPost, ruta, `{"fase":4,"referencia":"CSV-123"}`, nil, procesoHandler); w.Code != http.StatusOK {
		t.Fatalf("archivar justificante: %d %s", w.Code, w.Body.String())
	}
	verFase := func(esperada int) {
		t.Helper()
		w := comoPersona(t, "luis", miembro, http.MethodGet, "/api/v1/obligaciones?anio=2026", "", nil, calendarioHandler)
		var calendario struct {
			Obligaciones []instanciaVista `json:"obligaciones"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &calendario); w.Code != http.StatusOK || err != nil {
			t.Fatalf("calendario: %d %s %v", w.Code, w.Body.String(), err)
		}
		for _, o := range calendario.Obligaciones {
			if o.Clave == "es_303" && o.Periodo == "2026-T3" && o.Proceso != nil && o.Proceso.Fase == esperada {
				return
			}
		}
		t.Fatalf("calendario no incluye fase %d: %s", esperada, w.Body.String())
	}
	verFase(4)
	if err := st.BorrarAdjunto(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	verFase(3)
}
