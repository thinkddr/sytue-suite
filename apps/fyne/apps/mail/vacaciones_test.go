// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestElEstadoDeLaAutorespuestaEsElDeLaWeb: `statusOf`. El último día cuenta entero
// (hasta hoy sigue respondiendo) y el primero también (desde hoy ya responde).
func TestElEstadoDeLaAutorespuestaEsElDeLaWeb(t *testing.T) {
	tr := traductor(t)
	hoy := time.Date(2026, 9, 23, 17, 40, 0, 0, time.Local)
	dia := func(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, time.Local) }
	var cero time.Time
	for _, c := range []struct {
		que          string
		activa       bool
		desde, hasta time.Time
		tono         ui.Tono
		rotulo       string
	}{
		{"apagada", false, cero, dia(30), ui.InsigniaNeutra, "vacation.statusOffLabel"},
		{"terminada ayer", true, dia(1), dia(22), ui.InsigniaAviso, "vacation.statusEndedLabel"},
		{"acaba hoy", true, dia(1), dia(23), ui.InsigniaExito, "vacation.statusOnLabel"},
		{"empieza mañana", true, dia(24), dia(30), ui.InsigniaAviso, "vacation.statusScheduledLabel"},
		{"empieza hoy", true, dia(23), cero, ui.InsigniaExito, "vacation.statusOnLabel"},
	} {
		tono, rotulo, detalle := EstadoVacaciones(tr, "es", c.activa, c.desde, c.hasta, hoy)
		if tono != c.tono || rotulo != tr.T(c.rotulo, nil) || detalle == "" {
			t.Errorf("%s: %v «%s» «%s»", c.que, tono, rotulo, detalle)
		}
	}
	if _, _, d := EstadoVacaciones(tr, "es", true, cero, dia(30), hoy); d != "Quien te escriba recibe la respuesta hasta el 30 de septiembre incluido." {
		t.Errorf("con fecha de vuelta dice «%s»", d)
	}
	if _, _, d := EstadoVacaciones(tr, "es", true, cero, cero, hoy); d != tr.T("vacation.statusOnDetailOpen", nil) {
		t.Errorf("sin fecha de vuelta dice «%s»", d)
	}
}

// TestElPeriodoIncluyeElUltimoDia: `fromDateInput(end, true)` es las 23:59:59 del
// último día, y el primero empieza a las 00:00.
func TestElPeriodoIncluyeElUltimoDia(t *testing.T) {
	desde := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	hasta := time.Date(2026, 10, 14, 0, 0, 0, 0, time.Local)
	ini, fin := PeriodoVacaciones(desde, hasta)
	if ini == nil || !ini.Equal(desde) || fin == nil || !fin.Equal(hasta.Add(24*time.Hour-time.Second)) {
		t.Errorf("periodo %v – %v", ini, fin)
	}
	if ini, fin := PeriodoVacaciones(time.Time{}, time.Time{}); ini != nil || fin != nil {
		t.Error("sin fechas, el periodo no tiene límites")
	}
}

// TestLaAutorespuestaSeCargaSeValidaYSeGuarda: lo del servidor rellena el formulario;
// la vuelta antes de la salida pone la pista en rojo y no deja guardar; guardar
// manda el periodo entero y cierra.
func TestLaAutorespuestaSeCargaSeValidaYSeGuarda(t *testing.T) {
	s := servicioConTodas()
	fin := time.Date(2026, 10, 14, 23, 59, 59, 0, time.Local)
	s.vacaciones = api.MailVacation{Enabled: true, Subject: "Fuera", Body: "Hola\n\nVuelvo pronto", EndsAt: &fin}
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	tr := traductor(t)
	g := b.verVacaciones()
	if !hayDialogo(w.Canvas(), tr.T("vacation.title", nil)) || g.estado != Listo {
		t.Fatalf("la autorespuesta no se abre cargada: %v", s.llamadas)
	}
	if !g.conmutador.Encendido || g.asunto.Texto() != "Fuera" || g.hasta.Texto() != "14/10/2026" {
		t.Errorf("formulario: activa %v, asunto %q, hasta %q", g.conmutador.Encendido, g.asunto.Texto(), g.hasta.Texto())
	}
	if len(g.previoCuerpo.Objects) != 3 {
		t.Errorf("la vista previa tiene %d líneas y el cuerpo tiene tres (una en blanco)", len(g.previoCuerpo.Objects))
	}

	g.desde.PonerTexto("20/10/2026")
	pista, _ := g.grupoHasta.Objects[len(g.grupoHasta.Objects)-1].(*ui.PistaCampo)
	if !g.guardar.Inerte || !g.hasta.Invalida || pista == nil || pista.Tono != ui.AyudaPeligro {
		t.Fatal("con la vuelta antes de la salida se deja guardar o la pista no está en rojo")
	}
	g.salvar()
	if s.pidio("SetVacation") != 0 {
		t.Fatal("se ha guardado un periodo al revés")
	}

	g.desde.PonerTexto("01/10/2026")
	if g.guardar.Inerte {
		t.Fatal("con el rango bien, «Guardar» sigue apagado")
	}
	g.salvar()
	if s.pidio("SetVacation") != 1 || s.vacaciones.EndsAt == nil || !s.vacaciones.EndsAt.Equal(fin) ||
		s.vacaciones.StartsAt == nil || s.vacaciones.StartsAt.Hour() != 0 {
		t.Fatalf("guardado: %v %+v", s.llamadas, s.vacaciones)
	}
	if hayDialogo(w.Canvas(), tr.T("vacation.title", nil)) {
		t.Error("tras guardar, el diálogo sigue abierto")
	}
}

// TestSinCargaNoSeGuardaEnBlanco: si la carga falla, ni formulario ni «Guardar»:
// guardar en blanco borraría la autorespuesta de verdad.
func TestSinCargaNoSeGuardaEnBlanco(t *testing.T) {
	s := servicioConTodas()
	s.errVacaciones = errors.New("caído")
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	g := b.verVacaciones()
	if g.estado != Error || !g.guardar.Inerte {
		t.Fatalf("estado %v, guardar inerte %v", g.estado, g.guardar.Inerte)
	}
	g.salvar()
	if s.pidio("SetVacation") != 0 {
		t.Error("se ha guardado sin haber cargado")
	}
}

// TestEnElMovilLaAutorespuestaCabe: a 360 el cuerpo del diálogo mide 312, como en la
// web (la hoja del teléfono, a todo el ancho y sin filetes a los lados); el estado se
// parte (la insignia baja de línea) en vez de ensanchar el diálogo.
func TestEnElMovilLaAutorespuestaCabe(t *testing.T) {
	s := servicioConTodas()
	s.vacaciones = api.MailVacation{Enabled: true}
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(360, 740))
	g := b.verVacaciones()
	if g.ancho != 312 {
		t.Errorf("el cuerpo del diálogo mide %g y en la web 312 (360−2×24)", g.ancho)
	}
	if w := g.form.MinSize().Width; w > g.ancho {
		t.Errorf("el formulario pide %g de ancho y el cuerpo tiene %g: se sale por la derecha", w, g.ancho)
	}
}

// TestLaCapturaDeLaAutorespuestaSeGuarda (SYTUE_CAPTURAS): escritorio y móvil.
func TestLaCapturaDeLaAutorespuestaSeGuarda(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	for _, c := range []struct {
		nombre string
		tam    fyne.Size
	}{{"vacaciones.png", fyne.NewSize(1280, 800)}, {"movil-vacaciones.png", fyne.NewSize(360, 740)}} {
		s := servicioConTodas()
		fin := time.Now().AddDate(0, 0, 10)
		s.vacaciones = api.MailVacation{Enabled: true, Subject: "Fuera de la oficina", Body: "Hola:\n\nEstoy fuera hasta el lunes.", EndsAt: &fin}
		b, w, _ := buzonConVentana(t, s, c.tam)
		b.verVacaciones()
		guardar(t, filepath.Join(dir, c.nombre), w.Canvas().Capture())
	}
}

// TestSinMensajeVaEnCursiva: la vista previa sin cuerpo dice «Sin mensaje» en
// `italic text-text-subtle`, como la web.
func TestSinMensajeVaEnCursiva(t *testing.T) {
	s := servicioConTodas()
	s.vacaciones = api.MailVacation{Enabled: true, Subject: "Fuera"}
	b, _, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	g := b.verVacaciones()
	p, _ := g.previoCuerpo.Objects[0].(*ui.Parrafo)
	if p == nil || p.Peso != ui.Cursiva || p.Texto != traductor(t).T("vacation.previewEmpty", nil) {
		t.Fatalf("la vista previa vacía: %+v", p)
	}
}
