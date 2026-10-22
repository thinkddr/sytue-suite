// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # Banco real, Chromium/Figtree, rem16, 2026-10-04.
// medidas-dominios-final.json: cuerpos854/318, filas852/316.

func bancoArnes() *api.ERPBanco {
	m := func(id, fecha, desc string, imp int64) api.ERPMovimientoBanco {
		return api.ERPMovimientoBanco{ID: id, Fecha: fecha, Descripcion: desc, Importe: imp}
	}
	google := api.ERPTareaBanco{
		ERPMovimientoBanco: m("m1", "2026-09-18", "RECIBO GOOGLE *WORKSPACE", -1210),
		Candidatas:         []api.ERPFacturaCandidata{{ID: "fc1", Ref: "FC-2026-00012", Tercero: "Google Ireland", Total: 1210}},
	}
	google.Empresa, google.URLFactura = "Google", "https://payments.google.com"
	return &api.ERPBanco{
		TotalPendientes: 3,
		Motivos:         []api.ERPMotivoBanco{{Clave: "comision", Rotulo: "Comisión bancaria"}, {Clave: "otro", Rotulo: "Otro"}},
		Pendientes: []api.ERPTareaBanco{
			google,
			{
				ERPMovimientoBanco: m("m2", "2026-09-15", "TRANSFERENCIA DE FERRETERIA LOBO FV-2026-00043", 302500),
				Candidatas:         []api.ERPFacturaCandidata{{ID: "f3", Ref: "FV-2026-00043", Tercero: "Construcciones Almagro S.L.", Total: 302500, Segura: true}},
			},
			{ERPMovimientoBanco: m("m3", "2026-09-10", "COMISION MANTENIMIENTO", -600), Sugerencia: "comision"},
		},
		Hechas: []api.ERPMovimientoBanco{{ID: "h1", Fecha: "2026-09-05", Descripcion: "COBRO FV-2026-00041", Importe: 123456, FacturaRef: "FV-2026-00041"}},
	}
}

// TestElBancoMideLoDeLaWeb: la cabecera, el `details` plegado y las tres tareas.
//
// El DOM mide el li con el filete de divide-y en sus primeras dos filas. El cobro
// positivo incluye su párrafo explicativo:36px a820 de interior,72px a284.
func TestElBancoMideLoDeLaWeb(t *testing.T) {
	b := NuevoBanco(traductor(t))
	b.Datos, b.Ventana, b.AlSubir = bancoArnes(), 1280, func() {}
	v := tema.Claro
	_, alto := b.cabecera(v, 854)
	cerca(t, "la cabecera", alto, 83)
	_, alto = b.plegado(v, "como", b.t("bank.howTo"), 854, nil, 0)
	cerca(t, "«¿Cómo saco el fichero…?» plegado", alto, 44)
	for i, quiero := range []float32{183, 203, 90} {
		_, alto := b.tarea(v, b.Datos.Pendientes[i], 852)
		if i < 2 {
			alto += tema.Borde
		} // divide-y del li real
		cerca(t, "la tarea "+b.Datos.Pendientes[i].ID, alto, quiero)
	}
	m := NuevoBanco(traductor(t))
	m.Datos, m.Ventana, m.AlSubir = bancoArnes(), 360, func() {}
	_, alto = m.cabecera(v, 318)
	cerca(t, "la cabecera en el móvil", alto, 145)
	for i, quiero := range []float32{239, 331, 120} {
		_, alto := m.tarea(v, m.Datos.Pendientes[i], 316)
		if i < 2 {
			alto += tema.Borde
		}
		cerca(t, "la tarea "+m.Datos.Pendientes[i].ID+" en el móvil", alto, quiero)
	}
	if base, iva := Desglosar(1210, 21); base != 1000 || iva != 210 {
		t.Errorf("12,10 con el 21 %%: base %d e IVA %d", base, iva)
	}
	if base, iva := Desglosar(1001, 10); base+iva != 1001 || base != 910 {
		t.Errorf("el IVA es lo que falta hasta el total: base %d e IVA %d", base, iva)
	}
	if got := Importe("es", -1210); got != "−12,10 €" {
		t.Errorf("importe: %q", got)
	}
}

// TestElBancoSePinta guarda la pantalla para mirarla, con «No lleva factura»
// desplegado, y comprueba la entrada del menú y el correo que pide la factura.
func TestElBancoSePinta(t *testing.T) {
	p, w := pantallaArnes(t, fyne.NewSize(1280, 1000))
	defer w.Close()
	p.IrA(VistaBanco)
	p.banco.Datos = bancoArnes()
	p.banco.Modo["m3"] = "motivo"
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-banco.png")
	textos := textosDe(p.banco.Objeto(tema.Claro))
	for _, s := range []string{"RECIBO GOOGLE *WORKSPACE", "Descarga la factura en su web", "¿Es esta factura?", "Comisión bancaria · sugerido"} {
		if !contiene(textos, s) {
			t.Errorf("el banco no dice «%s»", s)
		}
	}
	p.rellenarMenu()
	var activa bool
	for _, d := range p.menu.Fijas {
		if d.Rotulo == p.t("nav.bank") {
			activa = d.Activa
		}
	}
	if !activa {
		t.Error("la entrada del banco no está o no es la activa")
	}
	g := bancoArnes().Pendientes[0]
	g.EmailFacturas = "facturas@google.com"
	ruta := PedirFacturaRuta(p.tr, "es", g, &api.ERPOrganizacion{Nombre: "Acme S.L.", NIF: "B12345678"})
	for _, s := range []string{"action=compose", "to=facturas%40google.com", "B12345678"} {
		if !strings.Contains(ruta, s) {
			t.Errorf("el correo que pide la factura no lleva %q: %s", s, ruta)
		}
	}
}

func TestElCobroDeClienteExplicaQueFacturaCorresponde(t *testing.T) {
	b := NuevoBanco(traductor(t))
	b.Datos = bancoArnes()
	aviso := b.t("bank.customerPaymentHint")
	if aviso == "bank.customerPaymentHint" {
		t.Fatal("falta el mensaje compartido del cobro de cliente")
	}
	for _, c := range []struct{ ancho, alto float32 }{{852, 36}, {316, 72}} {
		o, h := b.tarea(tema.Claro, b.Datos.Pendientes[1], c.ancho)
		o.Resize(fyne.NewSize(c.ancho, h))
		var encontrado bool
		recorrer(o, func(x fyne.CanvasObject) {
			if p, ok := x.(*ui.Parrafo); ok && p.Texto == aviso {
				encontrado = true
				cerca(t, "el aviso de cobro", p.Size().Height, c.alto)
			}
		})
		if !encontrado {
			t.Fatal("el cobro positivo no explica cómo vincular una factura emitida")
		}
	}
	o, _ := b.tarea(tema.Claro, b.Datos.Pendientes[0], 852)
	recorrer(o, func(x fyne.CanvasObject) {
		if p, ok := x.(*ui.Parrafo); ok && p.Texto == aviso {
			t.Error("un pago al proveedor no es un cobro de cliente")
		}
	})
}
