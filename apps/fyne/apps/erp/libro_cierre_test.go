// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// cierreFalso es el libro de cierres del servicio: contesta el estado y apunta lo que
// se le manda, moviendo el corte como el de verdad.
type cierreFalso struct {
	mu       sync.Mutex
	estado   string // JSON del GET
	escritos []string
}

func (c *cierreFalso) responder(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case r.URL.Path == "/api/v1/contabilidad/cierre" && r.Method == http.MethodGet:
		_, _ = io.WriteString(w, c.estado)
	case strings.HasPrefix(r.URL.Path, "/api/v1/contabilidad/"):
		b, _ := io.ReadAll(r.Body)
		c.escritos = append(c.escritos, r.URL.Path+" "+string(b))
		var x struct{ Hasta, Motivo string }
		_ = json.Unmarshal(b, &x)
		accion := "cierre"
		if strings.HasSuffix(r.URL.Path, "/reapertura") {
			accion = "reapertura"
		}
		var viejo api.ERPCierre
		_ = json.Unmarshal([]byte(c.estado), &viejo)
		nuevo := api.ERPCierre{CerradoHasta: x.Hasta}
		m := api.ERPMovimientoCierre{Accion: accion, Motivo: x.Motivo, En: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)}
		if x.Hasta != "" {
			d, _ := time.Parse(time.DateOnly, x.Hasta)
			m.Hasta = &d
		}
		nuevo.Historial = append([]api.ERPMovimientoCierre{m}, viejo.Historial...)
		e, _ := json.Marshal(nuevo)
		c.estado = string(e)
		_, _ = io.WriteString(w, `{}`)
	default:
		_, _ = io.WriteString(w, `{"asientos":[]}`)
	}
}

const cerradoA2025 = `{"cerrado_hasta":"2025-12-31","historial":[` +
	`{"hasta":"2025-12-31T00:00:00Z","accion":"cierre","en":"2026-02-01T09:00:00Z"},` +
	`{"hasta":"2024-12-31T00:00:00Z","accion":"cierre","en":"2025-02-01T09:00:00Z"}]}`

// permisosDeCierre: administrador de contabilidad, con o sin el verbo `administrar`.
func permisosDeCierre(administrar bool) api.ERPPermisos {
	verbos := []string{"asentar"}
	if administrar {
		verbos = append(verbos, "administrar")
	}
	return api.ERPPermisos{
		Modulos: map[string]string{"contabilidad": api.ERPAdministrador},
		Verbos:  map[string][]string{"contabilidad": verbos},
	}
}

func libroConCierre(t *testing.T, estado string, permisos api.ERPPermisos) (*Pantalla, *cierreFalso, fyne.Window) {
	t.Helper()
	cf := &cierreFalso{estado: estado}
	p, _ := pantallaConServicio(t, cf.responder)
	w := test.NewWindow(p.raiz)
	w.Resize(fyne.NewSize(1280, 800))
	t.Cleanup(w.Close)
	p.lienzo = w.Canvas()
	p.Ventana = fyne.NewSize(1280, 800)
	p.Permisos, p.SabePermisos = permisos, true
	p.IrA(VistaLibro)
	return p, cf, w
}

func botonDelPie(d *ui.Dialogo, rotulo string) *ui.Boton {
	var b *ui.Boton
	recorrer(d.Pie, func(o fyne.CanvasObject) {
		if x, ok := o.(*ui.Boton); ok && b == nil && x.Texto == rotulo {
			b = x
		}
	})
	return b
}

func TestLibroReaperturaDeshaceUnPaso(t *testing.T) {
	var c api.ERPCierre
	if err := json.Unmarshal([]byte(cerradoA2025), &c); err != nil {
		t.Fatal(err)
	}
	if got := ReaperturaHasta(c); got != "2024-12-31" {
		t.Errorf("reabrir 2025 vuelve a %q; quiere el corte anterior", got)
	}
	c.CerradoHasta, c.Historial = "2024-12-31", c.Historial[1:]
	if got := ReaperturaHasta(c); got != "" {
		t.Errorf("reabrir el primer cierre deja %q; quiere el libro entero", got)
	}
}

func TestLibroPropuestasDeCierre(t *testing.T) {
	hoy := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	quiere := []PropuestaCierre{{"mesAnterior", "2026-10-31"}, {"trimestreAnterior", "2026-09-30"}, {"anioAnterior", "2025-12-31"}}
	if got := PropuestasDeCierre(hoy); !slices.Equal(got, quiere) {
		t.Errorf("noviembre: %v", got)
	}
	// VISTO EN ROJO: sin quitar las repetidas, en enero salían tres «31 de diciembre».
	enero := PropuestasDeCierre(time.Date(2027, 1, 15, 12, 0, 0, 0, time.Local))
	if !slices.Equal(enero, []PropuestaCierre{{"anioAnterior", "2026-12-31"}}) {
		t.Errorf("enero: %v", enero)
	}
}

// TestLibroSinPermisoNoOfreceCerrar: se ve hasta cuándo está cerrado, pero ni «Cerrar
// período…» ni «Reabrir…» sin el NIVEL y el verbo que exige la ruta.
//
// VISTO EN ROJO: sin la franja, la insignia no se pintaba; con solo el verbo como
// condición, a quien tiene `administrar` sin nivel de administrador se le ofrecía.
func TestLibroSinPermisoNoOfreceCerrar(t *testing.T) {
	sinVerbo := permisosDeCierre(false)
	soloVerbo := permisosDeCierre(true)
	soloVerbo.Modulos["contabilidad"] = api.ERPUsuario
	for nombre, permisos := range map[string]api.ERPPermisos{"sin verbo": sinVerbo, "sin nivel": soloVerbo} {
		p, _, _ := libroConCierre(t, cerradoA2025, permisos)
		textos := textosDe(p.raiz)
		if !contiene(textos, p.t("close.closedUntil", i18n.Vars{"fecha": diaLegible(p.tr.Locale(), "2025-12-31")})) {
			t.Errorf("%s: no se ve hasta cuándo está cerrado: %v", nombre, textos)
		}
		if botonDe(p.raiz, p.t("close.close")) != nil || botonDe(p.raiz, p.t("close.reopen")) != nil {
			t.Errorf("%s: se ofrece cerrar o reabrir", nombre)
		}
	}
}

// TestLibroCerrarPeriodo: el diálogo propone el fin del período acabado, no deja
// confirmar una fecha futura ni una que no avance el corte, dice la consecuencia y
// manda la fecha elegida; la insignia cambia al volver.
func TestLibroCerrarPeriodo(t *testing.T) {
	p, cf, w := libroConCierre(t, `{"cerrado_hasta":"","historial":[]}`, permisosDeCierre(true))
	if botonDe(p.raiz, p.t("close.reopen")) != nil {
		t.Error("con el libro abierto no hay nada que reabrir")
	}
	b := botonDe(p.raiz, p.t("close.close"))
	if b == nil {
		t.Fatalf("no se ofrece «Cerrar período…»: %v", textosDe(p.raiz))
	}
	test.Tap(b)
	d := dialogoArriba(w)
	if d == nil {
		t.Fatal("no se abrió el diálogo")
	}
	guardarPNG(t, w.Canvas().Capture(), "erp-cierre-1.png")
	primera := PropuestasDeCierre(time.Now())[0].Hasta
	legible := diaLegible(p.tr.Locale(), primera)
	if txt := textosDe(d.Cuerpo); !contiene(txt, legible) {
		t.Fatalf("no se propone el fin del período acabado: %v", txt)
	}
	campo := primeroDe[*ui.Entrada](d.Cuerpo)
	confirmar := botonDelPie(d, p.t("close.closeConfirm"))
	if campo == nil || confirmar == nil {
		t.Fatal("falta el campo o el botón de confirmar")
	}
	// Una fecha futura tecleada a mano no se puede confirmar.
	campo.PonerTexto("31/12/2999")
	if !confirmar.Inerte || !contiene(textosDe(d.Cuerpo), p.t("close.futureDate")) {
		t.Fatalf("una fecha futura se deja cerrar: inerte %v, %v", confirmar.Inerte, textosDe(d.Cuerpo))
	}
	pulsarDelCuerpo(t, d, legible)
	if confirmar.Inerte {
		t.Fatal("con la propuesta elegida no se puede confirmar")
	}
	if txt := textosDe(d.Cuerpo); !contiene(txt, "Nadie podrá asentar") {
		t.Fatalf("no se dice la consecuencia: %v", txt)
	}
	confirmar.Tapped(nil)
	if !slices.Equal(cf.escritos, []string{`/api/v1/contabilidad/cierre {"hasta":"` + primera + `"}`}) {
		t.Fatalf("lo mandado: %q", cf.escritos)
	}
	if !contiene(textosDe(p.raiz), p.t("close.closedUntil", i18n.Vars{"fecha": legible})) {
		t.Errorf("la insignia no dice el corte nuevo: %v", textosDe(p.raiz))
	}

	// Cerrado ya hasta esa fecha, una anterior no avanza el corte y no se confirma.
	test.Tap(botonDe(p.raiz, p.t("close.close")))
	d = dialogoArriba(w)
	campo = primeroDe[*ui.Entrada](d.Cuerpo)
	campo.PonerTexto("01/01/2020")
	if !botonDelPie(d, p.t("close.closeConfirm")).Inerte {
		t.Error("cerrar hacia atrás se deja confirmar")
	}
}

// TestLibroReabrirSinMotivoDeshabilitado: reabrir dice hasta dónde queda cerrado, no se
// confirma sin motivo, vuelve al corte anterior y el historial lo cuenta.
func TestLibroReabrirSinMotivoDeshabilitado(t *testing.T) {
	p, cf, w := libroConCierre(t, cerradoA2025, permisosDeCierre(true))
	b := botonDe(p.raiz, p.t("close.reopen"))
	if b == nil {
		t.Fatalf("no se ofrece «Reabrir…»: %v", textosDe(p.raiz))
	}
	test.Tap(b)
	d := dialogoArriba(w)
	if d == nil {
		t.Fatal("no se abrió la pregunta")
	}
	anterior := diaLegible(p.tr.Locale(), "2024-12-31")
	if d.Titulo != p.t("close.reopenTo", i18n.Vars{"fecha": anterior}) {
		t.Errorf("el título no dice hasta dónde queda cerrado: %q", d.Titulo)
	}
	aceptar := botonDelPie(d, p.t("close.reopenConfirm"))
	if aceptar == nil || !aceptar.Inerte {
		t.Fatal("sin motivo se puede reabrir")
	}
	campo := primeroDe[*ui.Entrada](d.Cuerpo)
	campo.PonerTexto("   ")
	if !aceptar.Inerte {
		t.Fatal("con solo espacios se puede reabrir")
	}
	campo.PonerTexto("Factura de diciembre mal fechada")
	aceptar.Tapped(nil)
	if !slices.Equal(cf.escritos, []string{`/api/v1/contabilidad/reapertura {"hasta":"2024-12-31","motivo":"Factura de diciembre mal fechada"}`}) {
		t.Fatalf("lo mandado: %q", cf.escritos)
	}
	if !contiene(textosDe(p.raiz), p.t("close.closedUntil", i18n.Vars{"fecha": anterior})) {
		t.Errorf("la insignia no vuelve al corte anterior: %v", textosDe(p.raiz))
	}

	test.Tap(botonDe(p.raiz, p.t("close.history")))
	textos := textosDe(p.raiz)
	for _, s := range []string{
		p.t("close.historyReopened", i18n.Vars{"fecha": anterior}),
		"Factura de diciembre mal fechada",
		p.t("close.closedUntil", i18n.Vars{"fecha": diaLegible(p.tr.Locale(), "2025-12-31")}),
	} {
		if !contiene(textos, s) {
			t.Errorf("el historial no dice «%s»: %v", s, textos)
		}
	}
	guardarPNG(t, w.Canvas().Capture(), "erp-cierre-historial.png")
}

// TestLibroFranjaCierreEstrechaParte: en una ventana de móvil la insignia y los tres
// botones no caben en una línea; la franja se parte (`flex-wrap`) en vez de montarlos
// unos sobre otros, y el historial baja el motivo a su línea.
//
// VISTO EN ROJO: con la fila de alto fijo, los botones pisaban la insignia.
func TestLibroFranjaCierreEstrechaParte(t *testing.T) {
	estado := `{"cerrado_hasta":"2025-12-31","historial":[` +
		`{"hasta":"2025-12-31T00:00:00Z","accion":"cierre","motivo":"Ejercicio declarado","en":"2026-02-01T09:00:00Z"}]}`
	p, _, w := libroConCierre(t, estado, permisosDeCierre(true))
	w.Resize(fyne.NewSize(375, 800))
	p.Ventana = fyne.NewSize(375, 800)
	p.libro.VerHistorial = true
	p.Repintar()
	d := fyne.CurrentApp().Driver()
	var cajas []struct {
		nombre string
		pos    fyne.Position
		tam    fyne.Size
	}
	recorrer(p.raiz, func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Insignia:
			cajas = append(cajas, struct {
				nombre string
				pos    fyne.Position
				tam    fyne.Size
			}{x.Texto, d.AbsolutePositionForObject(o), o.Size()})
		case *ui.Boton:
			for _, clave := range []string{"close.history", "close.reopen", "close.close"} {
				if x.Texto == p.t(clave) {
					cajas = append(cajas, struct {
						nombre string
						pos    fyne.Position
						tam    fyne.Size
					}{x.Texto, d.AbsolutePositionForObject(o), o.Size()})
				}
			}
		}
	})
	if len(cajas) != 4 {
		t.Fatalf("la franja tiene %d piezas, quiere la insignia y tres botones", len(cajas))
	}
	for i, a := range cajas {
		if a.pos.X < 0 || a.pos.X+a.tam.Width > 375+1 {
			t.Errorf("«%s» se sale de la ventana: x %v ancho %v", a.nombre, a.pos.X, a.tam.Width)
		}
		for _, b := range cajas[i+1:] {
			if a.pos.X < b.pos.X+b.tam.Width && b.pos.X < a.pos.X+a.tam.Width &&
				a.pos.Y < b.pos.Y+b.tam.Height && b.pos.Y < a.pos.Y+a.tam.Height {
				t.Errorf("«%s» y «%s» se pisan: %v/%v y %v/%v", a.nombre, b.nombre, a.pos, a.tam, b.pos, b.tam)
			}
		}
	}
	if !contiene(textosDe(p.raiz), "Ejercicio declarado") {
		t.Errorf("en estrecho el motivo del historial no se ve: %v", textosDe(p.raiz))
	}
	guardarPNG(t, w.Canvas().Capture(), "erp-cierre-estrecho.png")
}
