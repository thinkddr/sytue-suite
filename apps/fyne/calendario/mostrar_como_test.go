// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestElFormularioAvisaDelChoqueYGuardaDisponible: con el «Comité» del 10 de 08:00 a
// 09:00 cargado, un evento nuevo a las 08:30 avisa «Coincide con…»; marcarlo
// «Disponible» quita el aviso (no ocupa) y viaja como TRANSP:TRANSPARENT. Rechazado el
// Comité, ya no cuenta como choque.
func TestElFormularioAvisaDelChoqueYGuardaDisponible(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	inicio := time.Date(2026, 9, 10, 8, 30, 0, 0, time.UTC)
	e := Edicion{Inicio: inicio, Fin: inicio.Add(time.Hour), Zona: "UTC"}
	f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
	f.d.Mostrar(w.Canvas())
	if !strings.Contains(f.aviso, "Comité") || !strings.Contains(f.aviso, "08:00–09:00") {
		t.Fatalf("sin aviso de choque con el Comité: %q", f.aviso)
	}
	// Moverlo a las 09:00 (toca el borde) ya no choca.
	f.inicio.PonerTexto(ui.EscribirFecha(inicio) + ", 09:00")
	if f.aviso != "" {
		t.Errorf("tocar el borde avisa: %q", f.aviso)
	}
	f.inicio.PonerTexto(ui.EscribirFecha(inicio) + ", 08:30")
	if f.aviso == "" {
		t.Fatal("volver a pisarlo no avisa")
	}

	f.mostrar.Elegida = 1
	f.mostrar.AlElegir(1)
	if f.aviso != "" {
		t.Errorf("un evento disponible avisa de choque: %q", f.aviso)
	}
	f.titulo.PonerTexto("Recoger el coche")
	f.enviar()
	hechas, cuerpos := s.apuntadas()
	if len(hechas) != 1 || cuerpos[0]["transp"] != "TRANSPARENT" {
		t.Fatalf("guardar hizo %v con %v", hechas, cuerpos)
	}

	// Lo que he rechazado no cuenta como choque.
	a.Correo = "yo@sytue.eu"
	for i := range a.P.Eventos {
		for j := range a.P.Eventos[i].Event.Attendees {
			a.P.Eventos[i].Event.Attendees[j].PartStat = "DECLINED"
		}
	}
	f2 := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
	if f2.aviso != "" {
		t.Errorf("lo rechazado avisa de choque: %q", f2.aviso)
	}
}

// TestUnEventoDisponibleSeReabreDisponible: la edición conserva «Disponible» y la vista
// rápida lo dice.
func TestUnEventoDisponibleSeReabreDisponible(t *testing.T) {
	o := conHoras("u", time.Date(2026, 9, 10, 18, 0, 0, 0, time.UTC), time.Date(2026, 9, 10, 19, 0, 0, 0, time.UTC),
		func(e *api.Event) { e.Summary, e.Transparency = "Pádel", "transparent" })
	p := &Pantalla{T: traductor(t), Zona: time.UTC}
	ed := p.EdicionDe(o)
	if !ed.Libre || ed.EntradaDe(p).Transparency != "TRANSPARENT" {
		t.Fatalf("se reabre ocupado: %+v", ed)
	}
	ed.Libre = false
	if got := ed.EntradaDe(p).Transparency; got != "OPAQUE" {
		t.Errorf("volver a ocupado manda %q", got)
	}
	test.NewTempApp(t)
	vista := p.nuevaVistaRapida(o, "", AccionesVistaRapida{Cerrar: func() {}, Editar: func() {}, Eliminar: func() {}, DescargarICS: func() {}})
	libre := p.t("popover.showsAsFree", nil)
	if buscarObjeto(vista, func(o fyne.CanvasObject) bool { pr, ok := o.(*ui.Parrafo); return ok && pr.Texto == libre }) == nil {
		t.Error("la vista rápida no dice que se muestra como disponible")
	}
	if buscarBoton(vista, p.t("popover.downloadIcs", nil)) == nil {
		t.Error("la vista rápida no ofrece «Descargar .ics»")
	}
}

// TestPosponerUnRecordatorio: el reloj abre las opciones, «10 min» lo saca de la lista,
// pide al servicio 10 minutos y avisa de cuándo vuelve; y deja de contar como anunciado.
func TestPosponerUnRecordatorio(t *testing.T) {
	s := &conRecordatorio{escrituras: &escrituras{}}
	a, w := appConVentana(t, s)
	_ = w
	r := api.Reminder{ID: "r1", EventID: "e1", Summary: "Comité", DTStart: time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)}
	a.P.Recordatorios = []api.Reminder{r}
	fyne.CurrentApp().Preferences().SetStringList(preferenciaAnunciados, []string{"r1", "r0"})

	a.alternarPosponer(r)
	if a.P.recPosponiendo != "r1" {
		t.Fatal("el reloj no abre las opciones")
	}
	lista := a.P.listaRecordatorios(false, false, a.saltarA, a.descartar)
	diez := buscarBoton(lista, a.P.etiquetaDuracion(10))
	if diez == nil {
		t.Fatal("sin la opción de 10 min")
	}
	diez.Tapped(nil)
	if len(a.P.Recordatorios) != 0 || a.P.recPosponiendo != "" {
		t.Errorf("sigue en la campana: %+v", a.P.Recordatorios)
	}
	hechas, cuerpos := s.apuntadas()
	if len(hechas) != 1 || hechas[0] != "POST /api/v1/reminders/r1/snooze" || cuerpos[0]["minutes"] != float64(10) {
		t.Fatalf("posponer hizo %v con %v", hechas, cuerpos)
	}
	if got := fyne.CurrentApp().Preferences().StringList(preferenciaAnunciados); len(got) != 1 || got[0] != "r0" {
		t.Errorf("sigue anunciado y no avisaría al volver: %v", got)
	}
	avisos := a.avisos.Avisos()
	if len(avisos) == 0 || !strings.Contains(avisos[len(avisos)-1].Titulo, "08:10") {
		t.Errorf("no dice cuándo vuelve: %+v", avisos)
	}
}

// conRecordatorio contesta al posponer con la hora a la que vuelve.
type conRecordatorio struct{ *escrituras }

func (s *conRecordatorio) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/reminders/r1/snooze" {
		rec := &grabadora{ResponseWriter: w}
		s.escrituras.ServeHTTP(rec, r)
		return
	}
	s.escrituras.ServeHTTP(w, r)
}

// grabadora deja pasar la petición al falso de siempre (que la apunta) pero contesta
// ella con el `trigger_at` del servicio.
type grabadora struct {
	http.ResponseWriter
	escrito bool
}

func (g *grabadora) Write([]byte) (int, error) {
	if g.escrito {
		return 0, nil
	}
	g.escrito = true
	return g.ResponseWriter.Write([]byte(`{"trigger_at":"2026-09-10T08:10:00Z"}`))
}

// TestCompartirDisponibilidadCopiaLosHuecos: el menú lo abre, pide los huecos con la
// jornada de los ajustes y copia el texto con la zona en la cabecera.
func TestCompartirDisponibilidadCopiaLosHuecos(t *testing.T) {
	s := &escrituras{}
	a, w := appConVentana(t, s)
	if a.P.AlCompartirDisponibilidad == nil {
		t.Fatal("sin «Compartir mi disponibilidad» en el menú")
	}
	aj := AjustesPorDefecto()
	aj.InicioLaboral, aj.FinLaboral, aj.FinesDeSemana = 8, 15, false
	a.P.Ajustes = aj
	a.P.AlCompartirDisponibilidad()
	s.mu.Lock()
	q := s.huecos
	s.mu.Unlock()
	if q.Get("work_start") != "08:00" || q.Get("work_end") != "15:00" || q.Get("weekdays") != "true" || q.Get("duration") != "30" {
		t.Fatalf("la consulta no lleva la jornada de los ajustes: %v", q)
	}
	copiar := buscarBoton(w.Canvas().Overlays().Top(), a.P.t("availability.copy", nil))
	if copiar == nil {
		t.Fatal("sin «Copiar»")
	}
	copiar.Tapped(nil)
	texto := fyne.CurrentApp().Clipboard().Content()
	if !strings.HasPrefix(texto, a.P.t("availability.textHeader", map[string]any{"zone": "UTC"})) || strings.Count(texto, "\n") != 2 {
		t.Errorf("copiado:\n%s", texto)
	}
	if len(w.Canvas().Overlays().List()) != 0 {
		t.Error("el diálogo sigue abierto tras copiar")
	}
}
