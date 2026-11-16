// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioQuePospone es el de pruebas con posponer y silenciar (posponer.go). `privado`
// es el modo privado, que los tiene pero dice que no.
type servicioQuePospone struct {
	*servicioFalso
	privado   bool
	vuelven   []api.MailSnooze
	estado    api.MailEstadoHilo
	errPospon error
}

func (s *servicioQuePospone) MailPospone() bool { return !s.privado }

func (s *servicioQuePospone) Snoozes(context.Context) ([]api.MailSnooze, error) {
	s.apuntar("Snoozes")
	return s.vuelven, nil
}

func (s *servicioQuePospone) SnoozeThread(_ context.Context, hilo string, hasta time.Time) (api.MailSnooze, error) {
	s.apuntar("SnoozeThread", hilo, hasta.UTC().Format(time.RFC3339))
	if s.errPospon != nil {
		return api.MailSnooze{}, s.errPospon
	}
	return api.MailSnooze{ThreadID: hilo, Until: hasta}, nil
}

func (s *servicioQuePospone) UnsnoozeThread(_ context.Context, hilo string) error {
	s.apuntar("UnsnoozeThread", hilo)
	return nil
}

func (s *servicioQuePospone) MuteThread(_ context.Context, hilo string, silenciar bool) (int, error) {
	s.apuntar("MuteThread", hilo, strconvBool(silenciar))
	return 2, nil
}

func (s *servicioQuePospone) ThreadState(_ context.Context, hilo string) (api.MailEstadoHilo, error) {
	s.apuntar("ThreadState", hilo)
	return s.estado, nil
}

func conPospuestos() *servicioQuePospone {
	s := servicioConTodas()
	s.carpetas = append(s.carpetas, api.Mailbox{ID: "sn", Name: "Snoozed", Role: "snoozed"})
	return &servicioQuePospone{servicioFalso: s}
}

// TestLasOpcionesDePosponerSonLasDeLaWeb son los casos de posponer.test.tsx: dependen
// de la hora y del día, y van en hora LOCAL.
func TestLasOpcionesDePosponerSonLasDeLaWeb(t *testing.T) {
	local := func(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, time.Local) }
	claves := func(ops []OpcionPosponer) (cs []string, fs []time.Time) {
		for _, o := range ops {
			cs, fs = append(cs, o.Clave), append(fs, o.Cuando)
		}
		return cs, fs
	}
	// 2026-11-09 es lunes.
	cs, fs := claves(OpcionesDePosponer(local(9, 10, 30)))
	if want := []string{"snooze.laterToday", "snooze.tomorrow", "snooze.weekend", "snooze.nextWeek"}; !slices.Equal(cs, want) {
		t.Errorf("lunes por la mañana: %v", cs)
	}
	if want := []time.Time{local(9, 18, 0), local(10, 8, 0), local(14, 8, 0), local(16, 8, 0)}; !slices.EqualFunc(fs, want, time.Time.Equal) {
		t.Errorf("lunes por la mañana: %v", fs)
	}
	// Un viernes por la noche, ni «Más tarde hoy» ni «Este fin de semana».
	cs, fs = claves(OpcionesDePosponer(local(13, 20, 0)))
	if !slices.Equal(cs, []string{"snooze.tomorrow", "snooze.nextWeek"}) ||
		!fs[0].Equal(local(14, 8, 0)) || !fs[1].Equal(local(16, 8, 0)) {
		t.Errorf("viernes por la noche: %v %v", cs, fs)
	}
	// Un domingo, «Mañana» ya es el lunes y no se repite.
	cs, _ = claves(OpcionesDePosponer(local(15, 10, 0)))
	if !slices.Equal(cs, []string{"snooze.laterToday", "snooze.tomorrow"}) {
		t.Errorf("domingo: %v", cs)
	}
	ahora := local(9, 10, 0)
	if FechaDePosponerValida(ahora.Add(30*time.Second), ahora) || FechaDePosponerValida(ahora.AddDate(1, 1, 0), ahora) ||
		!FechaDePosponerValida(ahora.Add(time.Hour), ahora) {
		t.Error("la fecha a mano se valida con lo del servicio: entre un minuto y un año")
	}
}

// TestPosponerDesdeRecibidosYDeshacer: el hilo sale de la lista, el aviso dice hasta
// cuándo y «Deshacer» lo devuelve ya. VISTO EN ROJO sin ponerPosponer: no había
// acción.
func TestPosponerDesdeRecibidosYDeshacer(t *testing.T) {
	s := conPospuestos()
	b := buzonConAvisos(t, s)
	b.Arrancar(context.Background())
	a := b.P.Acciones
	if a.Posponer == nil || a.Silenciar == nil || a.QuitarPosposicion == nil {
		t.Fatal("en Recibidos se pospone y se silencia")
	}
	hasta := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	c := b.P.Bandeja.Conversaciones[0]
	a.Posponer(c, hasta)
	if !slices.Contains(s.llamadas, "SnoozeThread("+c.HiloID+",2026-11-20T08:00:00Z)") {
		t.Fatalf("no se ha pospuesto: %v", s.llamadas)
	}
	if slices.ContainsFunc(b.P.Bandeja.Conversaciones, func(x Conversacion) bool { return x.HiloID == c.HiloID }) {
		t.Error("el hilo pospuesto sigue en Recibidos")
	}
	av := b.P.Avisos.Avisos()
	if len(av) != 1 || av[0].AlPulsarAccion == nil || !strings.HasPrefix(av[0].Titulo, "Pospuesto hasta") {
		t.Fatalf("el aviso de posponer no ofrece deshacer: %+v", av)
	}
	av[0].AlPulsarAccion()
	if !slices.Contains(s.llamadas, "UnsnoozeThread("+c.HiloID+")") {
		t.Errorf("deshacer no devuelve el hilo: %v", s.llamadas)
	}
}

// TestLoVueltoDePospuestosVaArribaConSuHoraDeVuelta: como en Gmail, lo que vuelve de
// «Pospuestos» al vencer encabeza Recibidos (el servicio lo manda primero) y la fila
// enseña la hora de VUELTA con el reloj, no la de llegada; y un delta del sondeo no
// lo baja a su sitio de antes. VISTO EN ROJO sin Vuelto: la fila decía cuándo llegó
// (hace un mes) y AplicarDelta lo mandaba al fondo.
func TestLoVueltoDePospuestosVaArribaConSuHoraDeVuelta(t *testing.T) {
	s := conPospuestos()
	volvio := hace.Add(-10 * time.Minute)
	vuelto := sobre("mv", "hv", "in", "Factura de marzo", false, 30*24*time.Hour)
	vuelto.UnsnoozedAt = &volvio
	nuevo := sobre("mn", "hn", "in", "Hoy", true, time.Hour)
	s.paginas = map[string][]api.MailMessage{"": {vuelto, nuevo}}
	s.siguiente = nil
	b := buzonConAvisos(t, s)
	b.Arrancar(context.Background())

	cs := b.P.Bandeja.Conversaciones
	if len(cs) != 2 || cs[0].HiloID != "hv" {
		t.Fatalf("lo vuelto no encabeza Recibidos: %+v", cs)
	}
	f := NuevaFilaConversacion()
	b.P.Bandeja.rellenar(f, 0, cs[0])
	if !f.Vuelta || f.Pospuesta {
		t.Errorf("la fila de lo vuelto: vuelta %v, pospuesta %v", f.Vuelta, f.Pospuesta)
	}
	if want := FechaCorta("es", volvio, b.P.Bandeja.Ahora()); f.Fecha != want {
		t.Errorf("la fila dice %q y no la hora de vuelta %q", f.Fecha, want)
	}
	if !strings.Contains(f.AccessibilityLabel(), "Pospuesto") {
		t.Errorf("el lector de pantalla no oye que volvió de «Pospuestos»: %q", f.AccessibilityLabel())
	}
	w := test.NewTempWindow(t, f)
	w.Resize(f.MinSize().AddWidthHeight(300, 0))
	if r := test.WidgetRenderer(f).(*filaRenderer); !r.reloj.Objeto().Visible() {
		t.Error("la hora de vuelta va sin el reloj")
	}
	// Lo que no se pospuso, con su llegada y sin reloj.
	g := NuevaFilaConversacion()
	b.P.Bandeja.rellenar(g, 1, cs[1])
	if g.Vuelta || g.Fecha != FechaCorta("es", nuevo.ReceivedAt, b.P.Bandeja.Ahora()) {
		t.Errorf("lo demás: vuelta %v, fecha %q", g.Vuelta, g.Fecha)
	}

	// El sondeo trae un cambio de lo nuevo (leído): lo vuelto sigue arriba.
	nuevo.Keywords = nil
	ms := AplicarDelta([]api.MailMessage{vuelto, nuevo}, api.MailCambios{Changed: []api.MailMessage{nuevo}}, "in", "")
	if len(ms) != 2 || ms[0].ID != "mv" {
		t.Errorf("tras el delta, lo vuelto baja: %v", []string{ms[0].ID, ms[1].ID})
	}
}

// TestEnPospuestosLaFilaDiceCuandoVuelveYFueraSuLlegada: en «Pospuestos» la fecha de
// la fila es la de vuelta, con el reloj; en Recibidos, la de llegada de siempre —un
// hilo devuelto a mano se ve con su fecha original, en su sitio—. VISTO EN ROJO sin Vuelven:
// la fila de «Pospuestos» decía cuándo llegó.
func TestEnPospuestosLaFilaDiceCuandoVuelveYFueraSuLlegada(t *testing.T) {
	s := conPospuestos()
	vuelve := time.Date(2026, 12, 24, 8, 0, 0, 0, time.Local)
	s.vuelven = []api.MailSnooze{{ThreadID: "h1", Until: vuelve}}
	b := buzonConAvisos(t, s)
	b.Arrancar(context.Background())
	if b.P.Bandeja.Vuelven != nil || slices.Contains(s.llamadas, "Snoozes()") {
		t.Error("fuera de «Pospuestos» no se piden ni se pintan fechas de vuelta")
	}
	fila := func() *FilaConversacion {
		for i, c := range b.P.Bandeja.Conversaciones {
			if c.HiloID == "h1" {
				f := NuevaFilaConversacion()
				b.P.Bandeja.rellenar(f, i, c)
				return f
			}
		}
		t.Fatal("no está h1")
		return nil
	}
	f := fila()
	if llegada := FechaCorta("es", f.Conv.Ultimo.Recibido, b.P.Bandeja.Ahora()); f.Pospuesta || f.Fecha != llegada {
		t.Errorf("en Recibidos la fila dice %q (pospuesta %v) y no su llegada %q", f.Fecha, f.Pospuesta, llegada)
	}

	b.ElegirCarpeta(carpetaPorRol(b, "snoozed"))
	if b.P.Bandeja.Vuelven["h1"] != vuelve {
		t.Fatalf("en «Pospuestos» no se sabe cuándo vuelve: %v", b.P.Bandeja.Vuelven)
	}
	f = fila()
	if !f.Pospuesta || f.Fecha != FechaCorta("es", vuelve, b.P.Bandeja.Ahora()) {
		t.Errorf("en «Pospuestos» la fila dice %q (pospuesta %v)", f.Fecha, f.Pospuesta)
	}
	w := test.NewTempWindow(t, f)
	w.Resize(f.MinSize().AddWidthHeight(300, 0))
	r := test.WidgetRenderer(f).(*filaRenderer)
	if !r.reloj.Objeto().Visible() {
		t.Error("la fecha de vuelta va sin el reloj")
	}

	// «Volver a Recibidos»: sale de la lista y ya no es de las que vuelven.
	b.P.Acciones.QuitarPosposicion(Conversacion{HiloID: "h1"})
	if !slices.Contains(s.llamadas, "UnsnoozeThread(h1)") {
		t.Errorf("no se ha devuelto: %v", s.llamadas)
	}
	if _, sigue := b.vuelven["h1"]; sigue {
		t.Error("el hilo devuelto sigue con su fecha de vuelta")
	}
}

// TestSilenciarDesdeElLector: «Más acciones» ofrece silenciar, y con el hilo ya
// silenciado, dejar de hacerlo; deshacer devuelve a Recibidos solo lo de Recibidos.
func TestSilenciarDesdeElLector(t *testing.T) {
	s := conPospuestos()
	b := buzonConAvisos(t, s)
	b.Arrancar(context.Background())
	x := b.P.Lector
	x.Conversacion = b.P.Bandeja.Conversaciones[0]
	textos := func() []string {
		var out []string
		for _, e := range x.menuMas().Elementos {
			if el, es := e.(*ui.ElementoMenu); es {
				out = append(out, el.Texto)
			}
		}
		return out
	}
	if !slices.Contains(textos(), "Silenciar conversación") {
		t.Errorf("«Más acciones» no ofrece silenciar: %v", textos())
	}
	x.Silenciada, x.PospuestaHasta = true, time.Now().Add(time.Hour)
	if got := textos(); !slices.Contains(got, "Dejar de silenciar") || !slices.Contains(got, "Volver a Recibidos") {
		t.Errorf("un hilo silenciado y pospuesto no ofrece deshacerlo: %v", got)
	}

	c := b.P.Bandeja.Conversaciones[0]
	b.P.Acciones.Silenciar(c, true)
	if !slices.Contains(s.llamadas, "MuteThread("+c.HiloID+",true)") {
		t.Fatalf("no se ha silenciado: %v", s.llamadas)
	}
	av := b.P.Avisos.Avisos()
	if len(av) == 0 || av[len(av)-1].AlPulsarAccion == nil {
		t.Fatal("silenciar no ofrece deshacer")
	}
	av[len(av)-1].AlPulsarAccion()
	if !slices.Contains(s.llamadas, "MuteThread("+c.HiloID+",false)") {
		t.Errorf("deshacer no deja de silenciar: %v", s.llamadas)
	}
}

// TestSinElServicioNoSePospone: en el modo privado (y con un servicio que no sabe) no
// hay ni posponer ni silenciar, y no se pregunta nada al abrir un hilo.
func TestSinElServicioNoSePospone(t *testing.T) {
	for _, s := range []Servicio{servicioConTodas(), &servicioQuePospone{servicioFalso: servicioConTodas(), privado: true}} {
		b := buzonConAvisos(t, s)
		b.Arrancar(context.Background())
		a := b.P.Acciones
		if a.Posponer != nil || a.Silenciar != nil || a.QuitarPosposicion != nil {
			t.Errorf("%T: se ofrece posponer o silenciar", s)
		}
	}
}
