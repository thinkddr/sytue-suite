// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # Tareas, libro diario y sumas y saldos: Chromium/Figtree, raíz 16 px, 2026-10-04

func tareasArnes() []api.ERPActividad {
	return []api.ERPActividad{
		{
			ID: "a1", Modelo: "factura_venta", Registro: "f1", RegistroEtiqueta: "FV-2026-00041", Tipo: "subir_documento",
			Resumen: "Sube la factura firmada en PDF", Vence: "2026-09-21", ResponsableNombre: "Ada Lovelace",
		},
		{
			ID: "a2", Modelo: "factura_venta", Registro: "f1", RegistroEtiqueta: "FV-2026-00041", Tipo: "llamada",
			Resumen: "Llamar para confirmar el cobro", Vence: "2026-09-23", ResponsableNombre: "Ada Lovelace",
		},
		{
			ID: "a3", Modelo: "contacto", Registro: "c2", RegistroEtiqueta: "Talleres Sur", Tipo: "reunion",
			Resumen: "Revisar las condiciones del contrato", Vence: "2026-09-28", ResponsableNombre: "Grace Hopper",
		},
	}
}

func asientoArnes(ref, estado string) api.ERPAsiento {
	return api.ERPAsiento{
		ID: ref, Ref: ref, Fecha: "2026-09-01", Concepto: "Factura FV-2026-00041", Estado: estado,
		Apuntes: []api.ERPApunte{
			{Cuenta: "430000", Concepto: "Ferretería Lobo", Debe: 123456},
			{Cuenta: "700000", Concepto: "Ventas", Haber: 102030},
			{Cuenta: "477000", Concepto: "IVA repercutido", Haber: 21426},
		},
	}
}

// TestTareasMideLoDeLaWeb: cabecera, grupos y filas. Medido: el título del primer grupo
// a 89 del de la pantalla (cabecera de 65 y el `space-y-6`), la fila 58 y el grupo
// de una fila 60 con su borde. El probe body854 conserva el ancho del guardián.
//
// VISTO EN ROJO (frontal, 2026-09-23): con el `py-2` de la ficha en vez del `py-3` de
// esta lista, la fila mide 43,75.
func TestTareasMideLoDeLaWeb(t *testing.T) {
	tr := traductor(t)
	ta := NuevaTareas(tr)
	ta.Lista, ta.Hoy, ta.Ventana = tareasArnes(), "2026-09-23", 1280
	if gs := ta.Grupos(); len(gs) != 3 || gs[0].titulo != tr.T("tasks.overdue", nil) {
		t.Fatalf("grupos: %+v", gs)
	}
	_, altoCab := ta.cabecera(tema.Claro, 854)
	_, altoLista := ta.lista(tema.Claro, ta.Lista[:1], ta.Hoy)
	for _, c := range []struct {
		que         string
		got, quiero float32
	}{
		{"la cabecera", altoCab, 65},
		{"la fila", AltoFilaTarea, 58},
		{"un grupo de una fila", altoLista, 60},
		{"65ch", anchoProsa(), 541.640625},
		{"el conmutador", NuevoConmutadorMias("", tr.T("tasks.mine", nil), tr.T("tasks.all", nil), true, nil).MinSize().Height, 28},
	} {
		if d := c.got - c.quiero; d < -0.3 || d > 0.3 {
			t.Errorf("%s mide %.2f; la web %.2f", c.que, c.got, c.quiero)
		}
	}
	// «De todos» enseña quién tiene cada tarea; «Mías», no.
	ta.Mias = false
	if !contiene(textosDe(ta.Objeto(tema.Claro)), "Grace Hopper") {
		t.Error("en «De todos» no se ve quién tiene la tarea")
	}
}

// TestElLibroYElBalanceMidenLoDeLaWeb: el asiento a 854 cabe en una línea (161) y la
// tabla del balance mide 174 con tres cuentas.
//
// VISTO EN ROJO (frontal, 2026-09-23): sin el `mr-auto` del concepto, la insignia del
// asiento queda pegada al concepto y no a la derecha.
func TestElLibroYElBalanceMidenLoDeLaWeb(t *testing.T) {
	tr := traductor(t)
	o, alto := AsientoCompacto(tema.Claro, asientoArnes("AS-2026-00017", "asentado"), "es", tr, 854)
	if d := alto - 161; d < -0.3 || d > 0.3 {
		t.Errorf("el asiento del libro mide %.2f; la web 161", alto)
	}
	o.Resize(fyne.NewSize(854, alto))
	var ins fyne.CanvasObject
	recorrer(o, func(x fyne.CanvasObject) {
		if strings.Contains(fyneTexto(x), "asentado") {
			ins = x
		}
	})
	if ins == nil || ins.Position().X+ins.Size().Width < 854-2*AsientoPX-2 {
		t.Errorf("la insignia del estado no va a la derecha del asiento")
	}
	b := &Libro{tr: tr, loc: "es", Ventana: 1280, Balance: &api.ERPBalance{
		TotalDebe: 123456, TotalHaber: 123456,
		Cuentas: []api.ERPSaldoCuenta{
			{Cuenta: "430000", Debe: 123456, Saldo: 123456},
			{Cuenta: "477000", Haber: 21426, Saldo: -21426},
			{Cuenta: "700000", Haber: 102030, Saldo: -102030},
		},
	}}
	if _, alto := b.tablaBalance(tema.Claro, 742); alto != 174 {
		t.Errorf("la tabla del balance mide %.2f; la web 174", alto)
	}
	if !contiene(textosDe(b.ObjetoBalance(tema.Claro)), tr.T("balance.squares", nil)) {
		t.Error("el balance que cuadra no lo dice")
	}
}

// fyneTexto es el texto de una insignia, si lo es.
func fyneTexto(o fyne.CanvasObject) string {
	if i, ok := o.(interface{ AccessibilityLabel() string }); ok {
		return i.AccessibilityLabel()
	}
	return ""
}

// TestLasVistasDelMenuSePintan guarda Tareas, el libro y el balance para mirarlos, y
// comprueba que la entrada del menú de la vista abierta es la activa.
func TestLasVistasDelMenuSePintan(t *testing.T) {
	p, w := pantallaArnes(t, fyne.NewSize(1280, 710))
	defer w.Close()
	p.TareasHoy = 3
	p.IrA(VistaTareas)
	p.tareas.Lista, p.tareas.Hoy = tareasArnes(), "2026-09-23"
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-tareas.png")
	if e, ok := buscarEntrada(p.menu.Objeto(tema.Claro), p.t("nav.tasks")).(*Entrada); !ok || e.Contador != 3 {
		t.Error("la entrada de Tareas no lleva el número de las de hoy")
	}
	p.IrA(VistaLibro)
	p.libro.Asientos = []api.ERPAsiento{asientoArnes("AS-2026-00017", "asentado"), asientoArnes("AS-2026-00018", "anulado")}
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-libro.png")
	p.IrA(VistaBalance)
	p.libro.Balance = &api.ERPBalance{TotalDebe: 123456, TotalHaber: 123456, Cuentas: []api.ERPSaldoCuenta{
		{Cuenta: "430000", Debe: 123456, Saldo: 123456},
		{Cuenta: "477000", Haber: 21426, Saldo: -21426},
		{Cuenta: "700000", Haber: 102030, Saldo: -102030},
	}}
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-balance.png")
	p.rellenarMenu()
	for _, d := range p.menu.Fijas {
		if d.Activa != (d.Rotulo == p.t("nav.balance")) {
			t.Errorf("«%s» activa = %v con el balance abierto", d.Rotulo, d.Activa)
		}
	}
}
