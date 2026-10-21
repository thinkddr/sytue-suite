// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Períodos contables (migración 0094), contra PostgreSQL real.
package store_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// TestPeriodoCerradoNoSeToca vigila el fallo contable clásico que la
// inmutabilidad por asiento NO evita: asentar —o anular— dentro de un ejercicio
// ya declarado, con lo que el balance de ese año cambia solo. Cada asiento es,
// por separado, perfectamente válido; lo que no vale es su FECHA.
func TestPeriodoCerradoNoSeToca(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)

	// Sin cierre, el libro está abierto entero.
	if _, cerrado, err := st.CierreVigente(ctx); err != nil || cerrado {
		t.Fatalf("libro nuevo: cerrado=%v %v, quiero abierto", cerrado, err)
	}

	// Una factura de septiembre de 2026 se asienta sin problema.
	factura, fv := facturaDePrueba(ctx, t, st, c)
	cliente := fv.Data["cliente"]
	asiento, err := st.Asentar(ctx, factura, fv.ID, "ada")
	if err != nil {
		t.Fatalf("Asentar con el libro abierto: %v", err)
	}

	// Se cierra hasta el 30 de septiembre de 2026.
	corte := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	if err := st.Cerrar(ctx, corte, "cierre del tercer trimestre", "jefa"); err != nil {
		t.Fatalf("Cerrar: %v", err)
	}
	if hasta, cerrado, err := st.CierreVigente(ctx); err != nil || !cerrado || !hasta.Equal(corte) {
		t.Fatalf("CierreVigente: %v cerrado=%v %v", hasta, cerrado, err)
	}

	// ANULAR un asiento de ese período: no. Es el agujero por el que un
	// ejercicio declarado cambiaba solo, y «anular es la única transición
	// permitida» no lo tapa.
	if err := st.Anular(ctx, asiento.ID, "me equivoqué"); !errors.Is(err, store.ErrPeriodoCerrado) {
		t.Fatalf("anular dentro del período cerrado: %v, quiero ErrPeriodoCerrado", err)
	}

	// ASENTAR otra factura con fecha del período cerrado: tampoco.
	otra := crear(ctx, t, st, factura, fmt.Sprintf(
		`{"cliente":%q,"fecha":"2026-09-15","base":50000,"iva":10500,"total":60500}`, cliente))
	if _, err := st.Asentar(ctx, factura, otra.ID, "ada"); !errors.Is(err, store.ErrPeriodoCerrado) {
		t.Fatalf("asentar dentro del período cerrado: %v, quiero ErrPeriodoCerrado", err)
	}

	// Con fecha POSTERIOR al corte sí, que es lo que tiene que seguir
	// funcionando: cerrar un trimestre no para la empresa.
	nueva := crear(ctx, t, st, factura, fmt.Sprintf(
		`{"cliente":%q,"fecha":"2026-10-02","base":50000,"iva":10500,"total":60500}`, cliente))
	if _, err := st.Asentar(ctx, factura, nueva.ID, "ada"); err != nil {
		t.Fatalf("asentar después del corte: %v", err)
	}

	// Cerrar HACIA ATRÁS se rechaza: sería una reapertura con otro nombre.
	if err := st.Cerrar(ctx, corte.AddDate(0, -1, 0), "", "jefa"); !errors.Is(err, store.ErrPeriodoCerrado) {
		t.Fatalf("cerrar hacia atrás: %v, quiero ErrPeriodoCerrado", err)
	}

	// La reapertura excepcional sí retrocede, y deja su motivo en el historial.
	atras := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	if err := st.ReabrirPeriodo(ctx, &atras, "inspección: falta una factura de septiembre", "jefa"); err != nil {
		t.Fatalf("ReabrirPeriodo: %v", err)
	}
	if _, err := st.Asentar(ctx, factura, otra.ID, "ada"); err != nil {
		t.Fatalf("tras reabrir, la de septiembre tiene que poder asentarse: %v", err)
	}
	hist, err := st.HistorialDeCierres(ctx)
	if err != nil || len(hist) != 2 {
		t.Fatalf("historial: %+v %v, quiero el cierre y la reapertura", hist, err)
	}
	if hist[0].Accion != "reapertura" || hist[0].Motivo == "" || hist[0].Por != "jefa" {
		t.Errorf("la reapertura no queda explicada: %+v", hist[0])
	}

	// Y reabrir del todo deja el libro entero abierto.
	if err := st.ReabrirPeriodo(ctx, nil, "cierre anulado", "jefa"); err != nil {
		t.Fatalf("reapertura total: %v", err)
	}
	if _, cerrado, err := st.CierreVigente(ctx); err != nil || cerrado {
		t.Fatalf("tras la reapertura total: cerrado=%v %v, quiero abierto", cerrado, err)
	}
}
