// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// LOS BANCOS DE CALENDAR: repintar el mes y la agenda con unos 170 eventos en tres
// calendarios, en escritorio (1280×800) y en el teléfono (390×844), y una vuelta del
// sondeo de recordatorios que no trae nada nuevo. Son los de la medición del 2026-11-24
// (docs/roadmap.md, «Rendimiento de las apps nativas»).
//
//	go test -tags goolm -run '^$' -bench 'Calendar' -benchmem ./calendario/

var cursorDeBanco = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

// instanciasDeBanco son ~170 eventos de noviembre: cuatro o cinco al día en horas
// distintas y uno de día completo cada semana, repartidos en tres calendarios.
func instanciasDeBanco() map[string][]api.Instance {
	por := map[string][]api.Instance{}
	cals := []string{"c1", "c2", "c3"}
	tonos := []string{"", "#b0413a", "#0e7490", "#86198f"}
	n := 0
	for d := 1; d <= 30; d++ {
		for h := range 5 + d%2 {
			if (d+h)%7 == 0 {
				continue
			}
			ini := time.Date(2026, 11, d, 8+2*h, 30*(h%2), 0, 0, time.UTC)
			ev := api.Event{
				ID: fmt.Sprintf("e%d", n), UID: fmt.Sprintf("u%d", n), Summary: fmt.Sprintf("Reunión %d", n),
				DTStart: ini, DTEnd: ini.Add(time.Hour), Color: tonos[n%len(tonos)],
			}
			c := cals[n%len(cals)]
			por[c] = append(por[c], api.Instance{Event: ev, Start: ini, End: ini.Add(time.Hour)})
			n++
		}
		if d%7 == 3 {
			dia := time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC)
			ev := api.Event{ID: fmt.Sprintf("t%d", d), Summary: "Congreso", AllDay: true, DTStart: dia, DTEnd: dia.AddDate(0, 0, 2)}
			por["c1"] = append(por["c1"], api.Instance{Event: ev, Start: dia, End: dia.AddDate(0, 0, 2)})
		}
	}
	return por
}

func appDeBanco(b testing.TB, ancho, alto float32, vista Vista) (*App, fyne.Window) {
	b.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Calendar())
	if err != nil {
		b.Fatal(err)
	}
	p := &Pantalla{T: tr, Vista: vista, Cursor: cursorDeBanco, Zona: time.UTC, Ahora: func() time.Time { return cursorDeBanco }}
	a := NuevaApp(p, nil)
	a.lanzar = func(f func(context.Context)) bool { f(a.vida); return true }
	a.enUI = func(f func()) { f() }
	a.cargandoCal = false
	p.Calendarios = []api.Calendar{
		{ID: "c1", DisplayName: "Personal", Color: "#16794a"},
		{ID: "c2", DisplayName: "Trabajo", Color: "#0e7490"},
		{ID: "c3", DisplayName: "Familia", Color: "#b0413a"},
	}
	a.porCal = instanciasDeBanco()
	w := test.NewWindow(a.Objeto())
	b.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ancho, alto))
	desde, hasta := p.rango()
	a.clave = desde.Format(time.RFC3339) + ".." + hasta.Format(time.RFC3339)
	a.pintar()
	return a, w
}

// BenchmarkCalendarPintar es `pintar` con el estado sin cambiar: lo que cuesta cada
// cambio que no toca la vista (la campana, una invitación, el menú).
func BenchmarkCalendarPintar(b *testing.B) {
	for _, pantalla := range []struct {
		nombre      string
		ancho, alto float32
	}{{"escritorio", 1280, 800}, {"movil", 390, 844}} {
		for _, vista := range []struct {
			nombre string
			v      Vista
		}{{"mes", VistaMes}, {"agenda", VistaAgenda}} {
			b.Run(pantalla.nombre+"/"+vista.nombre, func(b *testing.B) {
				a, _ := appDeBanco(b, pantalla.ancho, pantalla.alto, vista.v)
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					a.pintar()
				}
			})
		}
	}
}

// BenchmarkCalendarRecordatorios es una vuelta del sondeo de recordatorios (cada
// minuto) que trae lo mismo que la anterior.
func BenchmarkCalendarRecordatorios(b *testing.B) {
	a, _ := appDeBanco(b, 1280, 800, VistaMes)
	lista := []api.Reminder{{ID: "r1", Summary: "Dentista", DTStart: cursorDeBanco.Add(time.Hour)}}
	a.llegaronRecordatorios(lista, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		a.llegaronRecordatorios(lista, nil)
	}
}

// TestRepintarConservaElMesElMenuYLaCabecera: un repintado que no cambia nada deja en
// la ventana el MISMO mes (con sus celdas), el mismo menú y la misma cabecera; lo que
// cambia una pieza la rehace a ella sola. VISTO EN ROJO con el mes, el menú y la
// cabecera rehechos en cada `pintar` (el banco del mes pasaba de 11 ms a 0,09 ms).
func TestRepintarConservaElMesElMenuYLaCabecera(t *testing.T) {
	a, _ := appDeBanco(t, 1280, 800, VistaMes)
	p := a.P
	mes, celda, menu, cab := p.mes, p.mes.renderer.celdas[10], p.menuHecho, p.cabeceraHecha
	a.pintar()
	if p.mes != mes || p.mes.renderer.celdas[10] != celda || p.menuHecho != menu || p.cabeceraHecha != cab {
		t.Fatal("un repintado sin cambios ha rehecho el mes, el menú o la cabecera")
	}

	// Llega un recordatorio: la campana de la cabecera cambia; el menú y el mes, no.
	a.llegaronRecordatorios([]api.Reminder{{ID: "r1", Summary: "Dentista", DTStart: cursorDeBanco}}, nil)
	if p.cabeceraHecha == cab || p.menuHecho != menu || p.mes.renderer.celdas[10] != celda {
		t.Error("un recordatorio tenía que rehacer solo la cabecera")
	}

	// Ocultar un calendario cambia la casilla del menú y los eventos del mes.
	p.Visibles = map[string]bool{"c1": true, "c2": true, "c3": false}
	a.pintar()
	if p.menuHecho == menu || p.mes.renderer.celdas[10] == celda || p.mes != mes {
		t.Error("ocultar un calendario tenía que rehacer el menú y las celdas, en el mismo mes")
	}
	menu, celda = p.menuHecho, p.mes.renderer.celdas[10]

	// Otro mes: el mini-mes, el título y la rejilla.
	cab = p.cabeceraHecha
	p.Cursor = cursorDeBanco.AddDate(0, 1, 0)
	a.pintar()
	if p.menuHecho == menu || p.cabeceraHecha == cab || p.mes.renderer.celdas[10] == celda {
		t.Error("cambiar de mes tenía que rehacer el menú, la cabecera y la rejilla")
	}
	if got := p.cabeceraHecha.Titulo; got != "Diciembre de 2026" {
		t.Errorf("la cabecera dice %q", got)
	}
}

// TestElMesNoRepartePorCadaMedida: MinSize reparte los eventos una vez por mes y lista
// de eventos, no en cada colocación. VISTO EN ROJO sin `dispDe`: era casi la mitad de
// lo que asignaba un repintado del mes.
func TestElMesNoRepartePorCadaMedida(t *testing.T) {
	a, _ := appDeBanco(t, 1280, 800, VistaMes)
	m := a.P.mes
	antes := m.disponer()
	m.MinSize()
	if d := m.disponer(); &d.chips[0] != &antes.chips[0] {
		t.Error("medir el mes ha vuelto a repartir los eventos")
	}
	m.Eventos = slices.Clone(m.Eventos)
	if d := m.disponer(); &d.chips[0] == &antes.chips[0] {
		t.Error("con otra lista de eventos se reparte la vieja")
	}
}

// retenidoPorPintado es lo que queda vivo en el montón, por pintado, tras n pintados y
// una recolección: lo que la caché de renderers de Fyne sigue guardando de los árboles
// descolgados (el driver de pruebas no la limpia nunca, así que se ve entero).
func retenidoPorPintado(a *App, n int) int64 {
	var antes, despues runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&antes)
	for range n {
		a.pintar()
	}
	runtime.GC()
	runtime.ReadMemStats(&despues)
	return (int64(despues.HeapAlloc) - int64(antes.HeapAlloc)) / int64(n)
}

// sinSoltar mide con `soltarArboles` apagado, que es como era antes del parche 0021.
func sinSoltar(t testing.TB, f func()) {
	t.Helper()
	original := soltarArboles
	soltarArboles = func([]fyne.CanvasObject, ...fyne.CanvasObject) {}
	defer func() { soltarArboles = original }()
	f()
}

// TestPintarNoRetieneLosArbolesViejos: el mes del teléfono y la agenda se rehacen
// enteros en cada pintado, y lo descolgado se suelta en el acto (ui.SoltarArboles): tras
// cuarenta pintados queda en el montón menos de la mitad de lo que quedaba cuando la
// caché de Fyne lo guardaba hasta caducar (medido el 2026-11-25: de 148 a 47 KB por
// pintado en el mes del teléfono y de 231 a 37 KB en la agenda). Lo que sigue quedando
// en el mes del teléfono es el mapa de ámbitos de `ThemeOverride` del fork, que guarda
// una entrada por cada primitiva que pasa por debajo de uno y no la borra al soltarla
// (upstream/README.md). VISTO EN ROJO con `soltarArboles` apagado.
func TestPintarNoRetieneLosArbolesViejos(t *testing.T) {
	for _, c := range []struct {
		nombre      string
		ancho, alto float32
		vista       Vista
	}{{"movil/mes", 390, 844, VistaMes}, {"escritorio/agenda", 1280, 800, VistaAgenda}} {
		t.Run(c.nombre, func(t *testing.T) {
			a, _ := appDeBanco(t, c.ancho, c.alto, c.vista)
			a.pintar()
			con := retenidoPorPintado(a, 40)
			var sin int64
			sinSoltar(t, func() { sin = retenidoPorPintado(a, 40) })
			t.Logf("retenido por pintado: %d B soltando, %d B sin soltar", con, sin)
			if sin <= 0 || con*2 > sin {
				t.Errorf("soltando quedan %d B por pintado y sin soltar %d B", con, sin)
			}
		})
	}
}

// BenchmarkCalendarRetenido es lo que queda vivo por pintado con los árboles viejos
// soltados (`retenido-B/op`) y, aparte, sin soltarlos (`sin-soltar`).
func BenchmarkCalendarRetenido(b *testing.B) {
	for _, soltar := range []bool{true, false} {
		nombre := "soltando"
		if !soltar {
			nombre = "sin-soltar"
		}
		b.Run("movil/mes/"+nombre, func(b *testing.B) {
			a, _ := appDeBanco(b, 390, 844, VistaMes)
			medir := func() {
				var total int64
				for b.Loop() {
					total += retenidoPorPintado(a, 10)
				}
				b.ReportMetric(float64(total)/float64(b.N), "retenido-B/op")
			}
			if soltar {
				medir()
			} else {
				sinSoltar(b, medir)
			}
		})
	}
}
