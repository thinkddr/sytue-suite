// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

func TestTPVCobroAtomicoIdempotente(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	st.Version = "v-test"
	c := cat(t)
	ctx := tenantCtx("acme")
	for _, m := range []string{"tpv", "productos", "contactos", "facturacion"} {
		if err := st.ActivarModulo(ctx, m, true); err != nil {
			t.Fatal(err)
		}
	}
	organizacionVerifactu(ctx, t, st, true)
	producto := crear(ctx, t, st, modelo(t, c, "producto"), `{"nombre":"Café","tipo":"bien","precio":100,"precio_con_iva":121,"tipo_iva":"21"}`)
	sesion := crear(ctx, t, st, modelo(t, c, "sesion_tpv"), fmt.Sprintf(`{"caja":"Mostrador","vendedor":"Ada","fecha_inicio":%q}`, time.Now().Format(time.RFC3339)))
	cobro := store.CobroTPV{Operacion: uuid.NewString(), Sesion: sesion.ID, Medio: "efectivo", Recibido: 500, Lineas: []store.LineaTPV{{Producto: producto.ID, Cantidad: 2, Precio: 121, TipoIVA: "21"}}}
	// Dos envíos simultáneos son una venta, una factura, dos asientos.
	var wg sync.WaitGroup
	resultados := make(chan store.VentaTPV, 2)
	fallos := make(chan error, 2)
	for range 2 {
		wg.Go(func() { v, e := st.CobrarTPV(ctx, c, cobro, "ada"); resultados <- v; fallos <- e })
	}
	wg.Wait()
	close(resultados)
	close(fallos)
	for err := range fallos {
		if err != nil {
			t.Fatal(err)
		}
	}
	id := ""
	for v := range resultados {
		if id != "" && id != v.Factura.ID {
			t.Fatal("duplicó la factura")
		}
		id = v.Factura.ID
		if v.Cambio != 258 || v.Factura.Estado != "pagado" {
			t.Fatalf("venta=%+v", v)
		}
		if v.Factura.Data["externo_id"] != nil || v.Factura.Data["tpv_operacion"] != cobro.Operacion {
			t.Fatalf("el TPV se marcó como factura de otro sistema: %+v", v.Factura.Data)
		}
	}
	registros, err := st.RegistrosVerifactu(ctx, id)
	if err != nil || len(registros) != 1 || registros[0].Tipo != "alta" || registros[0].Estado != "pendiente" {
		t.Fatalf("VERI*FACTU de la venta: %+v %v", registros, err)
	}
	xml := xmlDeRegistros(ctx, t, st)
	if len(xml) != 1 || !strings.Contains(xml[0], "<sum1:TipoFactura>F2</sum1:TipoFactura>") || !strings.Contains(xml[0], "<sum1:ImporteTotal>2.42</sum1:ImporteTotal>") {
		t.Fatalf("factura simplificada: %+v", xml)
	}
	// Verificamos directamente los invariantes que importan, no el retorno del método.
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var facturas, asientos int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM records WHERE model='factura_venta'`).Scan(&facturas); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM moves WHERE estado='asentado'`).Scan(&asientos); err != nil {
			return err
		}
		if facturas != 1 || asientos != 2 {
			return fmt.Errorf("facturas=%d, asientos=%d", facturas, asientos)
		}
		var efectivo, total int64
		if err := tx.QueryRow(ctx, `SELECT (data->>'efectivo')::bigint,(data->>'total')::bigint FROM records WHERE id=$1::uuid`, sesion.ID).Scan(&efectivo, &total); err != nil {
			return err
		}
		if efectivo != 242 || total != 242 {
			return fmt.Errorf("caja=%d, total=%d", efectivo, total)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Reutilizar el UUID con otros datos se rechaza, no se cobra otra vez.
	cobro.Lineas[0].Cantidad = 3
	if _, err := st.CobrarTPV(ctx, c, cobro, "ada"); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("otra cesta: %v", err)
	}
	// Un período cerrado revierte factura, líneas y caja, sin rastros parciales.
	cobro.Operacion = uuid.NewString()
	cobro.Lineas[0].Cantidad = 2
	if err := st.Cerrar(ctx, time.Now().AddDate(0, 0, 1), "cierre de prueba", "ada"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CobrarTPV(ctx, c, cobro, "ada"); !errors.Is(err, store.ErrPeriodoCerrado) {
		t.Fatalf("cierre: %v", err)
	}
	p, err := st.Listar(ctx, store.Filtro{Modelo: "factura_venta"})
	if err != nil || len(p.Registros) != 1 {
		t.Fatalf("rollback: %+v %v", p, err)
	}
	// Ninguna referencia de otra organización se puede usar en la caja.
	if _, err := st.CobrarTPV(tenantCtx("globex"), c, cobro, "ada"); err == nil {
		t.Fatal("cobro entre organizaciones")
	}
}

func TestConvertirVentaReintento(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	c := cat(t)
	ctx := tenantCtx("acme")
	for _, m := range []string{"ventas", "facturacion"} {
		if err := st.ActivarModulo(ctx, m, true); err != nil {
			t.Fatal(err)
		}
	}
	cliente := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Cliente","tipo":"organizacion"}`)
	presupuesto := crear(ctx, t, st, modelo(t, c, "presupuesto"), fmt.Sprintf(`{"cliente":%q,"fecha":"2026-10-03","base":10000,"iva":2100,"total":12100}`, cliente.ID))
	if _, err := st.ConvertirVenta(ctx, c, "presupuesto", presupuesto.ID, "ada"); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("borrador: %v", err)
	}
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "presupuesto"), presupuesto.ID, "aceptado", "ada"); err != nil {
		t.Fatal(err)
	}
	pedido, err := st.ConvertirVenta(ctx, c, "presupuesto", presupuesto.ID, "ada")
	if err != nil {
		t.Fatal(err)
	}
	repetido, err := st.ConvertirVenta(ctx, c, "presupuesto", presupuesto.ID, "ada")
	if err != nil || repetido.ID != pedido.ID {
		t.Fatalf("reintento: %+v %v", repetido, err)
	}
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "pedido_venta"), pedido.ID, "confirmado", "ada"); err != nil {
		t.Fatal(err)
	}
	factura, err := st.ConvertirVenta(ctx, c, "pedido_venta", pedido.ID, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if factura.Estado != "borrador" || factura.Data["pedido"] != pedido.ID || factura.Data["cliente"] != cliente.ID {
		t.Fatalf("factura=%+v", factura)
	}
}

func TestTPVLimiteEfectivo(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	c := cat(t)
	ctx := tenantCtx("acme")
	for _, m := range []string{"tpv", "productos", "contactos", "facturacion"} {
		if err := st.ActivarModulo(ctx, m, true); err != nil {
			t.Fatal(err)
		}
	}
	organizacionVerifactu(ctx, t, st, false)
	cliente := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Cliente","tipo":"organizacion","nif":"B12345674"}`)
	sesion := crear(ctx, t, st, modelo(t, c, "sesion_tpv"), fmt.Sprintf(`{"caja":"Mostrador","vendedor":"Ada","fecha_inicio":%q}`, time.Now().Format(time.RFC3339)))
	for _, importe := range []int64{99999, 100000} {
		producto := crear(ctx, t, st, modelo(t, c, "producto"), fmt.Sprintf(`{"nombre":"Producto","tipo":"bien","precio":%d,"precio_con_iva":%d,"tipo_iva":"21"}`, domain.SinIVA(importe, 21), importe))
		cobro := store.CobroTPV{Operacion: uuid.NewString(), Sesion: sesion.ID, Cliente: cliente.ID, Medio: "efectivo", Recibido: importe, Lineas: []store.LineaTPV{{Producto: producto.ID, Cantidad: 1, Precio: importe, TipoIVA: "21"}}}
		_, err := st.CobrarTPV(ctx, c, cobro, "ada")
		if importe == 99999 && err != nil {
			t.Fatalf("999,99 € en efectivo: %v", err)
		}
		if importe == 100000 && !errors.Is(err, domain.ErrDatos) {
			t.Fatalf("1.000 € en efectivo: %v", err)
		}
	}
	facturas, err := st.Listar(ctx, store.Filtro{Modelo: "factura_venta"})
	if err != nil || len(facturas.Registros) != 1 {
		t.Fatalf("la venta bloqueada dejó factura: %+v %v", facturas, err)
	}
	caja, err := st.Obtener(ctx, "sesion_tpv", sesion.ID)
	if err != nil {
		t.Fatal(err)
	}
	total, err := domain.Centimos(caja.Data, "total")
	if err != nil || total != 99999 {
		t.Fatalf("la venta bloqueada modificó caja: %d %v", total, err)
	}
}

func TestPresupuestoTPVIdempotenteYConversionConDetalle(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	c := cat(t)
	ctx := tenantCtx("acme")
	cliente := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Cliente","tipo":"organizacion"}`)
	otroCliente := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Otro cliente","tipo":"organizacion"}`)
	uno := crear(ctx, t, st, modelo(t, c, "producto"), `{"nombre":"Café","tipo":"bien","precio":100,"precio_con_iva":121,"tipo_iva":"21"}`)
	dos := crear(ctx, t, st, modelo(t, c, "producto"), `{"nombre":"Pan","tipo":"bien","precio":100,"precio_con_iva":110,"tipo_iva":"10"}`)
	cesta := store.CobroTPV{Operacion: uuid.NewString(), Cliente: cliente.ID, Lineas: []store.LineaTPV{{Producto: uno.ID, Cantidad: 2, Precio: 121, TipoIVA: "21"}, {Producto: dos.ID, Cantidad: 3, Precio: 110, TipoIVA: "10"}}}
	if _, err := st.PresupuestoTPV(ctx, c, cesta, "ada"); !errors.Is(err, store.ErrModulosDesactivados) {
		t.Fatalf("presupuesto sin módulos: %v", err)
	}
	for _, m := range []string{"tpv", "ventas", "productos", "contactos", "facturacion"} {
		if err := st.ActivarModulo(ctx, m, true); err != nil {
			t.Fatal(err)
		}
	}
	organizacionVerifactu(ctx, t, st, false)
	presupuesto, err := st.PresupuestoTPV(ctx, c, cesta, "ada")
	if err != nil {
		t.Fatal(err)
	}
	repetido, err := st.PresupuestoTPV(ctx, c, cesta, "ada")
	if err != nil || repetido.ID != presupuesto.ID {
		t.Fatalf("reintento: %+v %v", repetido, err)
	}
	cesta.Cliente = otroCliente.ID
	if _, err := st.PresupuestoTPV(ctx, c, cesta, "ada"); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("misma operación con otro cliente: %v", err)
	}
	cesta.Cliente = cliente.ID
	cesta.Lineas[0].Cantidad++
	if _, err := st.PresupuestoTPV(ctx, c, cesta, "ada"); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("misma operación con otra cesta: %v", err)
	}
	cesta.Lineas[0].Cantidad--
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "presupuesto"), presupuesto.ID, "aceptado", "ada"); err != nil {
		t.Fatal(err)
	}
	pedido, err := st.ConvertirVenta(ctx, c, "presupuesto", presupuesto.ID, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "pedido_venta"), pedido.ID, "confirmado", "ada"); err != nil {
		t.Fatal(err)
	}
	factura, err := st.ConvertirVenta(ctx, c, "pedido_venta", pedido.ID, "ada")
	if err != nil {
		t.Fatal(err)
	}
	lineas, err := st.LineasDeFactura(ctx, factura.ID)
	if err != nil || len(lineas) != 2 {
		t.Fatalf("detalle convertido: %+v %v", lineas, err)
	}
	porNombre := map[string]domain.LineaFactura{}
	for _, l := range lineas {
		porNombre[l.Descripcion] = l
	}
	if l := porNombre["Café"]; l.Cantidad != "2" || l.Precio != 121 || l.Tipo != 21 || l.Importe != 242 {
		t.Fatalf("línea de café cambió: %+v", l)
	}
	if l := porNombre["Pan"]; l.Cantidad != "3" || l.Precio != 110 || l.Tipo != 10 || l.Importe != 330 {
		t.Fatalf("línea de pan cambió: %+v", l)
	}
	if _, err := st.Asentar(ctx, modelo(t, c, "factura_venta"), factura.ID, "ada"); err != nil {
		t.Fatalf("factura convertida no cuadra: %v", err)
	}
	if err := st.ActivarModulo(ctx, "ventas", false); err != nil {
		t.Fatal(err)
	}
	repetido, err = st.PresupuestoTPV(ctx, c, cesta, "ada")
	if err != nil || repetido.ID != presupuesto.ID {
		t.Fatalf("reintento después de desactivar: %+v %v", repetido, err)
	}
	cesta.Operacion = uuid.NewString()
	if _, err := st.PresupuestoTPV(ctx, c, cesta, "ada"); !errors.Is(err, store.ErrModulosDesactivados) {
		t.Fatalf("presupuesto nuevo en módulo desactivado: %v", err)
	}
}

func TestConversionDescuadradaRevierteDestinoYLineas(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	c := cat(t)
	ctx := tenantCtx("acme")
	if err := st.ActivarModulo(ctx, "ventas", true); err != nil {
		t.Fatal(err)
	}
	cliente := crear(ctx, t, st, modelo(t, c, "contacto"), `{"nombre":"Cliente","tipo":"organizacion"}`)
	producto := crear(ctx, t, st, modelo(t, c, "producto"), `{"nombre":"Producto","tipo":"bien","precio":100}`)
	presupuesto := crear(ctx, t, st, modelo(t, c, "presupuesto"), fmt.Sprintf(`{"cliente":%q,"fecha":"2026-10-03","base":100,"iva":21,"total":121}`, cliente.ID))
	crear(ctx, t, st, modelo(t, c, "linea_presupuesto"), fmt.Sprintf(`{"presupuesto":%q,"producto":%q,"nombre":"Producto","cantidad":"2","precio_con_iva":121,"tipo_iva":"21"}`, presupuesto.ID, producto.ID))
	if _, err := st.CambiarEstado(ctx, modelo(t, cat(t), "presupuesto"), presupuesto.ID, "aceptado", "ada"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ConvertirVenta(ctx, c, "presupuesto", presupuesto.ID, "ada"); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("se convirtió un presupuesto descuadrado: %v", err)
	}
	for _, clave := range []string{"pedido_venta", "linea_venta"} {
		p, err := st.Listar(ctx, store.Filtro{Modelo: clave})
		if err != nil || len(p.Registros) != 0 {
			t.Fatalf("rollback de %s: %+v %v", clave, p, err)
		}
	}
}

func TestProcesoNoTerminaSinJustificante(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	if _, err := st.AvanzarProceso(ctx, "es_303", "2026-T3", 3, "", nil, "ada"); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("saltar pasos: %v", err)
	}
	for fase := 1; fase <= 3; fase++ {
		if _, err := st.AvanzarProceso(ctx, "es_303", "2026-T3", fase, "", nil, "ada"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.AvanzarProceso(ctx, "es_303", "2026-T3", 4, "CSV-123", nil, "ada"); !errors.Is(err, domain.ErrDatos) {
		t.Fatalf("sin justificante: %v", err)
	}
	p, err := st.ProcesoObligacion(ctx, "es_303", "2026-T3")
	if err != nil || p.Fase != 3 {
		t.Fatalf("persistencia: %+v %v", p, err)
	}
	p, err = st.ProcesoObligacion(tenantCtx("globex"), "es_303", "2026-T3")
	if err != nil || p.Fase != 0 {
		t.Fatalf("RLS: %+v %v", p, err)
	}
}
