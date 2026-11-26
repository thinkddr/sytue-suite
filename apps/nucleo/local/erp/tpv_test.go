// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/tpv"
)

// Un cobro en efectivo emite la factura simplificada pagada, la lleva al libro con su
// cobro en caja y acumula la sesión; reintentarlo devuelve la misma venta.
func TestCobrarTPV(t *testing.T) {
	ctx := context.Background()
	e, alm := abrir(t, t.TempDir())
	if _, err := e.GuardarOrganizacion(ctx, api.ERPOrganizacion{Nombre: "Panadería", NIF: "B12345678", Pais: "ES", Forma: "sl", CP: "28013"}); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"tpv", "facturacion", "productos", "ventas"} {
		if _, err := e.CambiarModulo(ctx, m, true); err != nil {
			t.Fatalf("%s: %v", m, err)
		}
	}
	prod, err := e.Crear(ctx, "producto", map[string]any{"nombre": "Hogaza", "tipo": "bien", "precio": 240, "precio_con_iva": 250, "tipo_iva": "4"})
	if err != nil {
		t.Fatal(err)
	}
	ses, err := e.Crear(ctx, api.ERPModeloSesionTPV, map[string]any{"caja": "1", "vendedor": "Ana", "fecha_inicio": "2026-11-02T09:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	cobro := api.ERPCobroTPV{
		Operacion: uuid.NewString(), Sesion: ses.ID, Medio: api.ERPMedioEfectivo, Recibido: 1000,
		Lineas: []api.ERPLineaTPV{{Producto: prod.ID, Cantidad: 3, Precio: 250, TipoIVA: "4"}},
	}
	v, err := e.CobrarTPV(ctx, cobro)
	if err != nil {
		t.Fatal(err)
	}
	if v.Cambio != 250 || v.Factura.Estado != "pagado" || v.Factura.Data["tipo_factura"] != "F2" {
		t.Fatalf("venta: %+v", v)
	}
	if otra, err := e.CobrarTPV(ctx, cobro); err != nil || otra.Factura.ID != v.Factura.ID || otra.Cambio != 250 {
		t.Fatalf("reintento: %v %+v", err, otra)
	}
	cambiada := cobro
	cambiada.Lineas = []api.ERPLineaTPV{{Producto: prod.ID, Cantidad: 2, Precio: 250, TipoIVA: "4"}}
	if _, err := e.CobrarTPV(ctx, cambiada); estado(err) != http.StatusUnprocessableEntity {
		t.Fatalf("misma operación, otra cesta: %v", err)
	}
	// El libro: la venta (430 contra 700 y 477) y el cobro en caja (570 contra 430).
	saldos, err := alm.Balance(ctx, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	por := map[string]SaldoCuenta{}
	for _, s := range saldos {
		por[s.Cuenta[:3]] = s
	}
	if por["570"].Debe != 750 || por["430"].Saldo != 0 || por["477"].Haber != 29 {
		t.Fatalf("libro: %+v", saldos)
	}
	s, err := e.Obtener(ctx, api.ERPModeloSesionTPV, ses.ID)
	if err != nil || toString(s.Data["efectivo"]) != "750" || toString(s.Data["total"]) != "750" {
		t.Fatalf("sesión: %v %+v", err, s.Data)
	}
	// Precio cambiado en el catálogo: se pide revisar la cesta.
	caro := cobro
	caro.Operacion = uuid.NewString()
	caro.Lineas = []api.ERPLineaTPV{{Producto: prod.ID, Cantidad: 1, Precio: 300, TipoIVA: "4"}}
	if _, err := e.CobrarTPV(ctx, caro); estado(err) != http.StatusUnprocessableEntity {
		t.Fatalf("precio cambiado: %v", err)
	}
}

// La cesta como presupuesto: con cliente, sus líneas y sin tocar el libro.
func TestPresupuestoTPV(t *testing.T) {
	ctx := context.Background()
	cat, obl, err := Cargar()
	if err != nil {
		t.Fatal(err)
	}
	alm, err := Abrir(t.TempDir(), cat)
	if err != nil {
		t.Fatal(err)
	}
	s := NuevoServidor(cat, obl, alm)
	e := api.NewERP(Base, nil, s.Cliente(), "es")
	for _, m := range []string{"tpv", "ventas", "productos"} {
		if _, err := e.CambiarModulo(ctx, m, true); err != nil {
			t.Fatal(err)
		}
	}
	prod, err := e.Crear(ctx, "producto", map[string]any{"nombre": "Tarta", "tipo": "bien", "precio": 2200, "precio_con_iva": 2420, "tipo_iva": "10"})
	if err != nil {
		t.Fatal(err)
	}
	cli, err := e.Crear(ctx, "contacto", map[string]any{"nombre": "Bodas Luz", "tipo": "organizacion"})
	if err != nil {
		t.Fatal(err)
	}
	cesta := tpv.Cobro{Operacion: uuid.NewString(), Cliente: cli.ID, Lineas: []tpv.Linea{{Producto: prod.ID, Cantidad: 2, Precio: 2420, TipoIVA: "10"}}}
	p, err := alm.PresupuestoTPV(ctx, cesta)
	if err != nil || p.Modelo != "presupuesto" || toString(p.Data["total"]) != "4840" {
		t.Fatalf("presupuesto: %v %+v", err, p)
	}
	if otro, err := alm.PresupuestoTPV(ctx, cesta); err != nil || otro.ID != p.ID {
		t.Fatalf("reintento: %v %+v", err, otro)
	}
	if _, err := alm.PresupuestoTPV(ctx, tpv.Cobro{Operacion: uuid.NewString(), Lineas: cesta.Lineas}); err == nil {
		t.Fatal("sin cliente no hay presupuesto")
	}
}
