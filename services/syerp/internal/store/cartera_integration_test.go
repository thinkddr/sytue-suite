// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// La antigüedad cuenta las facturas abiertas por días desde su vencimiento al día dado,
// con los cinco tramos siempre; las pagadas, los borradores y las de otra organización
// no cuentan.
func TestAntiguedadDeCobros(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)
	contacto, factura := modelo(t, c, "contacto"), modelo(t, c, "factura_venta")
	cli := crear(ctx, t, st, contacto, `{"nombre":"Acme","tipo":"organizacion"}`)
	nueva := func(venc string, total int, estado string) {
		f := crear(ctx, t, st, factura, fmt.Sprintf(`{"cliente":%q,"fecha":"2026-08-01","vencimiento":%q,"base":%d,"total":%d}`, cli.ID, venc, total, total))
		if estado != "borrador" {
			if _, err := st.CambiarEstado(ctx, factura, f.ID, estado, "ada"); err != nil {
				t.Fatalf("estado %s: %v", estado, err)
			}
		}
	}
	nueva("2026-12-01", 100, "confirmado") // aún no vence
	nueva("2026-11-20", 200, "confirmado") // 5 días
	nueva("2026-08-01", 400, "confirmado") // 116 días
	nueva("2026-11-20", 800, "pagado")     // cobrada: fuera
	nueva("2026-11-20", 1600, "borrador")  // sin emitir: fuera
	otra := crear(tenantCtx("globex"), t, st, contacto, `{"nombre":"Globex","tipo":"organizacion"}`)
	crear(tenantCtx("globex"), t, st, factura, fmt.Sprintf(`{"cliente":%q,"fecha":"2026-08-01","base":5,"total":5}`, otra.ID))

	a, err := st.Antiguedad(ctx, "factura_venta", time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	quiero := map[string]int64{store.TramoAlDia: 100, store.Tramo1a30: 200, store.Tramo31a60: 0, store.Tramo61a90: 0, store.TramoMas90: 400}
	if len(a.Tramos) != 5 || a.Total != 700 || a.Vencido != 600 {
		t.Fatalf("antigüedad: %+v", a)
	}
	for _, tr := range a.Tramos {
		if tr.Centimos != quiero[tr.Tramo] {
			t.Errorf("tramo %s: %d, quería %d", tr.Tramo, tr.Centimos, quiero[tr.Tramo])
		}
	}
}

// El estado de cuenta suma a los dos lados del contacto y lista primero lo abierto.
func TestEstadoDeCuenta(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)
	contacto, venta, compra := modelo(t, c, "contacto"), modelo(t, c, "factura_venta"), modelo(t, c, "factura_compra")
	ana := crear(ctx, t, st, contacto, `{"nombre":"Ana","tipo":"organizacion"}`)
	otro := crear(ctx, t, st, contacto, `{"nombre":"Otro","tipo":"organizacion"}`)
	pagada := crear(ctx, t, st, venta, fmt.Sprintf(`{"cliente":%q,"fecha":"2026-10-01","base":300,"total":300}`, ana.ID))
	if _, err := st.CambiarEstado(ctx, venta, pagada.ID, "pagado", "ada"); err != nil {
		t.Fatal(err)
	}
	abierta := crear(ctx, t, st, venta, fmt.Sprintf(`{"cliente":%q,"fecha":"2026-10-01","vencimiento":"2026-11-01","base":100,"total":100}`, ana.ID))
	if _, err := st.CambiarEstado(ctx, venta, abierta.ID, "confirmado", "ada"); err != nil {
		t.Fatal(err)
	}
	crear(ctx, t, st, venta, fmt.Sprintf(`{"cliente":%q,"fecha":"2026-10-01","base":999,"total":999}`, otro.ID))
	gasto := crear(ctx, t, st, compra, fmt.Sprintf(`{"proveedor":%q,"fecha":"2026-10-01","base":50,"total":50}`, ana.ID))
	if _, err := st.CambiarEstado(ctx, compra, gasto.ID, "confirmado", "ada"); err != nil {
		t.Fatal(err)
	}

	e, err := st.EstadoDeCuenta(ctx, ana.ID, time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if e.Ventas != (store.ResumenCuenta{Facturado: 400, Pagado: 300, Pendiente: 100, Vencido: 100}) || e.Compras.Pendiente != 50 {
		t.Fatalf("estado de cuenta: %+v", e)
	}
	if len(e.Documentos) != 3 || e.Documentos[0].ID != abierta.ID || e.Documentos[0].DiasVencida != 24 {
		t.Fatalf("documentos (la abierta y vencida primero): %+v", e.Documentos)
	}
}

// Bajo mínimo: suma las existencias de todos los almacenes y deja fuera lo que llega o
// no tiene mínimo.
func TestProductosBajoMinimo(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)
	producto, existencia, almacen := modelo(t, c, "producto"), modelo(t, c, "existencia"), modelo(t, c, "almacen")
	a1 := crear(ctx, t, st, almacen, `{"nombre":"Norte"}`)
	a2 := crear(ctx, t, st, almacen, `{"nombre":"Sur"}`)
	corto := crear(ctx, t, st, producto, `{"nombre":"Tornillo","stock_minimo":"100"}`)
	justo := crear(ctx, t, st, producto, `{"nombre":"Tuerca","stock_minimo":"10"}`)
	crear(ctx, t, st, producto, `{"nombre":"Arandela"}`)
	for _, e := range []string{
		fmt.Sprintf(`{"producto":%q,"almacen":%q,"existencias":"30.5"}`, corto.ID, a1.ID),
		fmt.Sprintf(`{"producto":%q,"almacen":%q,"existencias":"20"}`, corto.ID, a2.ID),
		fmt.Sprintf(`{"producto":%q,"almacen":%q,"existencias":"10"}`, justo.ID, a1.ID),
	} {
		crear(ctx, t, st, existencia, e)
	}
	ps, err := st.BajoMinimo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].ID != corto.ID || ps[0].Stock != "50.5" || ps[0].Faltan != "49.5" || ps[0].Almacen != 2 {
		t.Fatalf("bajo mínimo: %+v", ps)
	}
}

// El mayor arrastra el saldo de antes del período y deja en cada apunte el saldo
// acumulado; las estadísticas de una lista filtrada suman solo lo filtrado.
func TestMayorYEstadisticasFiltradas(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)
	factura, fv := facturaDePrueba(ctx, t, st, c)
	asiento, err := st.Asentar(ctx, factura, fv.ID, "ada")
	if err != nil {
		t.Fatal(err)
	}
	cuenta := asiento.Apuntes[0].Cuenta
	m, err := st.Mayor(ctx, cuenta, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Apuntes) == 0 || m.SaldoFinal != m.Debe-m.Haber || m.Apuntes[len(m.Apuntes)-1].Saldo != m.SaldoFinal {
		t.Fatalf("mayor: %+v", m)
	}
	// Desde después del asiento: entra con su saldo y no tiene apuntes.
	despues, err := st.Mayor(ctx, cuenta, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if despues.SaldoInicial != m.SaldoFinal || len(despues.Apuntes) != 0 || despues.SaldoFinal != m.SaldoFinal {
		t.Fatalf("mayor desde 2027: %+v", despues)
	}

	borrador := crear(ctx, t, st, factura, fmt.Sprintf(`{"cliente":%q,"fecha":"2026-09-13","base":1,"total":1}`, fv.Data["cliente"]))
	_ = borrador
	// La lista encuentra la factura por su número, no solo por el nombre del cliente.
	pag, err := st.Listar(ctx, store.Filtro{Modelo: factura.Clave, Texto: strings.ToLower(fv.Ref), Limite: 10})
	if err != nil || len(pag.Registros) != 1 || pag.Registros[0].ID != fv.ID {
		t.Fatalf("buscar %s en la lista: %+v, %v", fv.Ref, pag.Registros, err)
	}
	stats, err := st.EstadisticasDe(ctx, factura, store.Filtro{Modelo: factura.Clave, Estado: "confirmado"})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 2 || stats.Filtrados != 1 || stats.SumaDinero != 121000 {
		t.Fatalf("estadísticas de lo confirmado: %+v", stats)
	}
}
