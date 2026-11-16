// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// permisosDeAprobador concede `aprobar` y, con `admin`, `administrar` en contabilidad.
func permisosDeAprobador(admin bool) api.ERPPermisos {
	verbos := []string{"asentar", "aprobar"}
	if admin {
		verbos = append(verbos, "administrar")
	}
	return api.ERPPermisos{
		Modulos: map[string]string{"contabilidad": api.ERPAdministrador, "facturacion": api.ERPUsuario},
		Verbos:  map[string][]string{"contabilidad": verbos},
	}
}

// servicioAprobaciones contesta dos pendientes —una pedida por «eva», que es quien
// mira, y otra por «ana»— y un umbral de 5.000 € en facturación.
func servicioAprobaciones(cuerpos *[]string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if len(b) > 0 {
			*cuerpos = append(*cuerpos, r.Method+" "+r.URL.Path+" "+string(b))
		}
		switch {
		case r.URL.Path == "/api/v1/aprobaciones":
			_, _ = io.WriteString(w, `{"aprobaciones":[`+
				`{"id":"ap1","modelo":"factura_venta","registro_id":"f1","importe":1250000,"moneda":"EUR","estado":"pendiente",`+
				`"solicitada_por":"eva","solicitada_en":"2026-11-10T09:30:00Z"},`+
				`{"id":"ap2","modelo":"factura_venta","registro_id":"f2","importe":980000,"moneda":"EUR","estado":"pendiente",`+
				`"solicitada_por":"ana","solicitada_en":"2026-11-10T10:00:00Z","creada_por":"luis"}]}`)
		case r.URL.Path == "/api/v1/admin/umbrales" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"umbrales":[{"modulo":"facturacion","importe":500000,"moneda":"EUR"}],`+
				`"moneda_por_defecto":"EUR","modulos":[{"clave":"obligaciones","rotulo":"Obligaciones"},`+
				`{"clave":"facturacion","rotulo":"Facturación"},{"clave":"compras","rotulo":"Compras"}]}`)
		case strings.HasPrefix(r.URL.Path, "/api/v1/aprobaciones/"):
			_, _ = io.WriteString(w, `{"id":"ap2","estado":"aprobada"}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func botonDe(o fyne.CanvasObject, texto string) *ui.Boton {
	var b *ui.Boton
	recorrer(o, func(x fyne.CanvasObject) {
		if y, ok := x.(*ui.Boton); ok && y.Texto == texto && b == nil {
			b = y
		}
	})
	return b
}

// TestLaColaDeAprobacionesRespetaLosCuatroOjos: a quien mira solo se le ofrece aprobar
// lo que pidió OTRA persona —lo suyo sale «esperando a otra persona»—, el menú cuenta
// solo esas, y aprobar va a la ruta del servicio.
//
// VISTO EN ROJO: ofreciendo «Aprobar» en todas las pendientes, salen dos botones y el
// contador del menú dice 2.
func TestLaColaDeAprobacionesRespetaLosCuatroOjos(t *testing.T) {
	var cuerpos []string
	p, sf := pantallaConServicio(t, servicioAprobaciones(&cuerpos))
	p.Yo = func() string { return "eva" }
	p.Permisos, p.SabePermisos = permisosDeAprobador(false), true
	p.IrA(VistaAprobaciones)

	if p.AprobacionesPendientes != 1 {
		t.Errorf("el menú cuenta %d; solo una la puede resolver quien mira", p.AprobacionesPendientes)
	}
	p.rellenarMenu()
	var entrada *Destino
	for i, d := range p.menu.Fijas {
		if d.Rotulo == p.t("nav.approvals") {
			entrada = &p.menu.Fijas[i]
		}
	}
	if entrada == nil || !entrada.Activa || entrada.Contador != 1 {
		t.Fatalf("la entrada «Aprobaciones» del menú: %+v", entrada)
	}

	o := p.aprob.Objeto(tema.Claro)
	textos := textosDe(o)
	for _, s := range []string{"12.500,00", p.t("approvals.waiting"), p.t("approvals.byYou")} {
		if !contiene(textos, s) {
			t.Errorf("no se pinta «%s»; se pintó %v", s, textos)
		}
	}
	// El modelo es un enlace que abre el registro.
	var enlace *ui.Enlace
	recorrer(o, func(x fyne.CanvasObject) {
		if e, ok := x.(*ui.Enlace); ok && e.AccessibilityLabel() == "Facturas de cliente" {
			enlace = e
		}
	})
	if enlace == nil {
		t.Error("el modelo de la solicitud no es un enlace al registro")
	}
	n := 0
	recorrer(o, func(x fyne.CanvasObject) {
		if b, ok := x.(*ui.Boton); ok && b.Texto == p.t("approvals.approve") {
			n++
		}
	})
	if n != 1 {
		t.Fatalf("se ofrece «Aprobar» %d veces; solo la de «ana»", n)
	}
	// Sin umbrales: quien solo aprueba no los administra.
	if contiene(textos, p.t("approvals.thresholds")) || slices.ContainsFunc(sf.vistas(), func(v string) bool {
		return strings.Contains(v, "/admin/umbrales")
	}) {
		t.Error("quien no administra la contabilidad ve o pide los umbrales")
	}

	test.Tap(botonDe(o, p.t("approvals.approve")))
	if !slices.Contains(cuerpos, `POST /api/v1/aprobaciones/ap2/aprobar {"motivo":""}`) {
		t.Errorf("aprobar no llegó al servicio: %q", cuerpos)
	}
	// El filtro pide al servicio las del estado elegido.
	p.aprob.AlEstado("rechazada")
	if v := sf.vistas(); !slices.Contains(v, "GET /api/v1/aprobaciones?estado=rechazada") {
		t.Errorf("el filtro no llegó al servicio: %q", v)
	}
}

// TestSinElVerboNoHayColaDeAprobaciones: sin `aprobar` ni `administrar`, el menú no la
// ofrece y no se pide nada.
func TestSinElVerboNoHayColaDeAprobaciones(t *testing.T) {
	var cuerpos []string
	p, sf := pantallaConServicio(t, servicioAprobaciones(&cuerpos))
	p.Permisos, p.SabePermisos = permisosDeVenta(), true
	p.contarAprobaciones()
	p.rellenarMenu()
	for _, d := range p.menu.Fijas {
		if d.Rotulo == p.t("nav.approvals") {
			t.Error("el menú ofrece la cola a quien no puede aprobar")
		}
	}
	if slices.ContainsFunc(sf.vistas(), func(v string) bool { return strings.Contains(v, "/aprobaciones") }) {
		t.Error("se piden las aprobaciones sin poder resolverlas")
	}
}

// TestLosUmbralesSeEscribenEnCentimos: quien administra ve un campo por módulo (no el
// de obligaciones, que el servicio no admite), con el importe puesto; en blanco lo
// quita, y algo que no es un importe ni se manda.
//
// VISTO EN ROJO: mandando lo escrito sin pasar por ACentimos, el PUT lleva 5000 en vez
// de 500000.
func TestLosUmbralesSeEscribenEnCentimos(t *testing.T) {
	var cuerpos []string
	p, _ := pantallaConServicio(t, servicioAprobaciones(&cuerpos))
	p.Yo = func() string { return "eva" }
	p.Permisos, p.SabePermisos = permisosDeAprobador(true), true
	p.IrA(VistaAprobaciones)

	a := p.aprob
	if ms := a.ModulosConUmbral(); len(ms) != 2 || ms[0].Clave != "facturacion" {
		t.Fatalf("módulos con umbral: %+v", ms)
	}
	if got := a.Campo("facturacion").Texto(); got != "5000.00" {
		t.Errorf("el umbral puesto se escribe %q", got)
	}
	textos := textosDe(a.Objeto(tema.Claro))
	for _, s := range []string{p.t("approvals.thresholdsHint"), "Facturación", "Compras", p.t("approvals.thresholdNone")} {
		if !contiene(textos, s) {
			t.Errorf("no se pinta «%s»; se pintó %v", s, textos)
		}
	}
	if contiene(textos, "Obligaciones") {
		t.Error("se ofrece umbral para obligaciones, que el servicio no admite")
	}

	a.Campo("compras").PonerTexto("1234,5")
	a.AlGuardarUmbral("compras", a.Campo("compras").Texto(), "EUR")
	a.AlGuardarUmbral("facturacion", "", "EUR")
	a.AlGuardarUmbral("facturacion", "mucho", "EUR")
	a.AlGuardarUmbral("facturacion", "-3", "EUR")
	var puts []string
	for _, c := range cuerpos {
		if strings.HasPrefix(c, "PUT ") {
			puts = append(puts, c)
		}
	}
	quiere := []string{
		`PUT /api/v1/admin/umbrales/compras {"importe":123450,"moneda":"EUR"}`,
		`PUT /api/v1/admin/umbrales/facturacion {"importe":-1,"moneda":"EUR"}`,
	}
	if !slices.Equal(puts, quiere) {
		t.Errorf("umbrales guardados:\n%q\nquiere\n%q", puts, quiere)
	}
}

// TestAsentarPorEncimaDelUmbralOfrecePedirAprobacion: el 409 `requiere_aprobacion` no
// sale como error a secas, sino como la banda ámbar con «Pedir aprobación», que abre la
// solicitud con el cuerpo vacío (el importe lo calcula el servicio).
//
// VISTO EN ROJO: sin el caso del código en `asentar`, la ficha pinta la banda roja y no
// hay botón que pulsar.
func TestAsentarPorEncimaDelUmbralOfrecePedirAprobacion(t *testing.T) {
	var cuerpos []string
	p, _ := pantallaConServicio(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/asentar"):
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"requiere_aprobacion","message":"over the threshold"}}`)
		case strings.HasSuffix(r.URL.Path, "/aprobacion"):
			cuerpos = append(cuerpos, r.Method+" "+r.URL.Path+" "+string(b))
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"ap9","estado":"pendiente"}`)
		default:
			_, _ = io.WriteString(w, `{"registros":[]}`)
		}
	})
	m := facturas()
	m.Contable = true
	p.lista.Modelo = m
	p.abrirFicha(&Registro{ID: "f1"}, nil)
	f := p.ficha
	if f == nil {
		t.Fatal("no se abrió la ficha")
	}
	f.AlAsentar()
	if !f.FaltaAprobacion || f.Fallo != "" {
		t.Fatalf("tras el 409: falta aprobación %v, fallo %q", f.FaltaAprobacion, f.Fallo)
	}
	o := f.Objeto(tema.Claro, 480)
	if !contiene(textosDe(o), "pasa del umbral") {
		t.Errorf("no se explica por qué no se asienta: %v", textosDe(o))
	}
	b := botonDe(o, p.t("form.requestApproval"))
	if b == nil {
		t.Fatal("no se ofrece «Pedir aprobación»")
	}
	test.Tap(b)
	if !slices.Equal(cuerpos, []string{"POST /api/v1/factura_venta/f1/aprobacion {}"}) {
		t.Errorf("la solicitud: %q", cuerpos)
	}
	if f.FaltaAprobacion || f.Aviso != p.t("form.approvalRequested") {
		t.Errorf("pedida, la ficha dice %q (banda ámbar: %v)", f.Aviso, f.FaltaAprobacion)
	}
}

// TestElGrupoDeFiltroEligeLaOpcionPulsada: con tres opciones, pulsar sobre la tercera
// la elige, y el grupo mide lo que sus tres botones.
func TestElGrupoDeFiltroEligeLaOpcionPulsada(t *testing.T) {
	elegida := -1
	g := NuevoGrupoFiltro("Aprobaciones", []string{"Pendientes", "Aprobadas", "Rechazadas"}, 0, func(i int) { elegida = i })
	m := g.MinSize()
	ancho := 2 * tema.Borde
	for i := range g.Opciones {
		ancho += g.anchoOpcion(i)
	}
	if m.Width != ancho {
		t.Errorf("el grupo mide %.2f; sus botones, %.2f", m.Width, ancho)
	}
	g.Tapped(&fyne.PointEvent{Position: fyne.NewPos(m.Width-2, m.Height/2)})
	if elegida != 2 {
		t.Errorf("pulsando a la derecha se eligió %d", elegida)
	}
	elegida = -1
	g.Tapped(&fyne.PointEvent{Position: fyne.NewPos(2, m.Height/2)})
	if elegida != -1 {
		t.Error("pulsar la ya elegida no tiene que avisar")
	}
}

// TestLasAprobacionesSePintan guarda la cola con los umbrales para mirarla.
func TestLasAprobacionesSePintan(t *testing.T) {
	p, w := pantallaArnes(t, fyne.NewSize(1280, 800))
	defer w.Close()
	p.Permisos, p.SabePermisos = permisosDeAprobador(true), true
	p.Yo = func() string { return "eva" }
	p.IrA(VistaAprobaciones)
	p.AprobacionesPendientes = 1
	a := p.aprob
	a.Lista = []api.ERPAprobacion{
		{
			ID: "ap1", Modelo: "factura_venta", RegistroID: "f1", Importe: 1250000, Moneda: "EUR", Estado: "pendiente",
			SolicitadaPor: "eva", SolicitadaEn: time.Date(2026, 11, 10, 9, 30, 0, 0, time.Local),
		},
		{
			ID: "ap2", Modelo: "factura_venta", RegistroID: "f2", Importe: 980000, Moneda: "EUR", Estado: "pendiente",
			SolicitadaPor: "ana", SolicitadaEn: time.Date(2026, 11, 10, 10, 0, 0, 0, time.Local), Motivo: "Proveedor nuevo",
		},
	}
	a.PonerUmbrales(api.ERPUmbrales{
		MonedaPorDefecto: "EUR",
		Umbrales:         []api.ERPUmbral{{Modulo: "facturacion", Importe: 500000, Moneda: "EUR"}},
		Modulos:          []api.ERPModuloRotulado{{Clave: "facturacion", Rotulo: "Facturación"}, {Clave: "compras", Rotulo: "Compras"}},
	})
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-aprobaciones.png")
	if !contiene(textosDe(w.Content()), p.t("approvals.subtitle")[:20]) {
		t.Error("no se pinta la cabecera de Aprobaciones")
	}
	// Y la banda de la ficha tras el 409 al asentar.
	p.Vista, p.lista.Modelo, p.lista.Pagina = "", facturaArnes(), facturasArnes()
	p.abrirFicha(&p.lista.Pagina.Registros[0], nil)
	if p.ficha == nil {
		t.Fatal("no se abrió la ficha")
	}
	p.ficha.FaltaAprobacion = true
	p.Repintar()
	guardarPNG(t, w.Canvas().Capture(), "erp-ficha-pedir-aprobacion.png")
	if botonDe(w.Content(), p.t("form.requestApproval")) == nil {
		t.Error("la ficha no ofrece «Pedir aprobación»")
	}
}
