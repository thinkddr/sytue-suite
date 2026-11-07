// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestFinDePausa es el gemelo de lib/pausa.test.ts.
func TestFinDePausa(t *testing.T) {
	ahora := time.Date(2026, 11, 5, 17, 0, 0, 0, time.Local) // jueves
	for d, quiero := range map[DuracionPausa]time.Time{
		Pausa30m:    ahora.Add(30 * time.Minute),
		Pausa1h:     ahora.Add(time.Hour),
		Pausa2h:     ahora.Add(2 * time.Hour),
		PausaManana: time.Date(2026, 11, 6, 9, 0, 0, 0, time.Local),
		PausaLunes:  time.Date(2026, 11, 9, 9, 0, 0, 0, time.Local),
	} {
		if got := FinDePausa(d, ahora); !got.Equal(quiero) {
			t.Errorf("%s acaba %v; quiero %v", d, got, quiero)
		}
	}
	if PausaActiva(time.Time{}, ahora) || PausaActiva(ahora.Add(-time.Minute), ahora) || !PausaActiva(ahora.Add(time.Minute), ahora) {
		t.Error("PausaActiva no es la de la web")
	}
}

// TestPausarYReanudarAvisos: la pausa vigente se lee al arrancar; elegir una duración
// la manda al servicio y la sala queda en pausa —el menú dice hasta cuándo y ofrece
// reanudar—; reanudar la quita; y un fallo avisa sin tocar nada.
func TestPausarYReanudarAvisos(t *testing.T) {
	ahora := time.Date(2026, 11, 5, 17, 0, 0, 0, time.UTC)
	s := servicioConDos()
	ya := ahora.Add(10 * time.Minute)
	s.prefs = api.RoomsNotifyPrefs{PausedUntil: &ya}
	sa, _ := salaDePrueba(t, s)
	sa.Ahora = func() time.Time { return ahora }
	sa.Arrancar(context.Background())
	if !sa.AvisosEnPausa() {
		t.Fatal("la pausa del servicio no se leyó al arrancar")
	}
	if got := rotulos(sa.entradasPausa(sa.P.T, sa.P.Locale, func() {})); len(got) != 1 || got[0] != sa.P.T.T("pause.resume", nil) {
		t.Fatalf("en pausa el menú ofrece %v", got)
	}
	sa.PausarAvisos("")
	if !s.pidio("SetNotifyPause(0001-01-01T00:00:00Z)") || sa.AvisosEnPausa() {
		t.Fatalf("reanudar: %v, en pausa %v", s.llamadas, sa.AvisosEnPausa())
	}
	entradas := rotulos(sa.entradasPausa(sa.P.T, sa.P.Locale, func() {}))
	if len(entradas) != 5 || entradas[0] != sa.P.T.T("pause.30m", nil) || entradas[4] != sa.P.T.T("pause.monday", nil) {
		t.Fatalf("sin pausa el menú ofrece %v", entradas)
	}
	sa.PausarAvisos(Pausa2h)
	if !s.pidio("SetNotifyPause(2026-11-05T19:00:00Z)") || !sa.AvisosEnPausa() {
		t.Fatalf("pausar 2 h: %v, en pausa %v", s.llamadas, sa.AvisosEnPausa())
	}
	s.errAccion = &api.Error{Status: 500}
	sa.PausarAvisos("")
	if !sa.AvisosEnPausa() {
		t.Error("un fallo al reanudar quitó la pausa")
	}
}

// TestLaPausaDeOtroDispositivoSeReleeAlVolver: la pausa vive en el servicio; si se pone
// o se quita en otro dispositivo, esta app se entera al volver a primer plano.
func TestLaPausaDeOtroDispositivoSeReleeAlVolver(t *testing.T) {
	ahora := time.Date(2026, 11, 5, 17, 0, 0, 0, time.UTC)
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Ahora = func() time.Time { return ahora }
	sa.EnPrimerPlano() // sin arrancar no hay nada que releer
	if s.pidio("NotifyPrefs") {
		t.Fatal("se pidió la pausa antes de arrancar")
	}
	sa.Arrancar(context.Background())
	if sa.AvisosEnPausa() {
		t.Fatal("en pausa sin que nadie la pusiera")
	}
	hasta := ahora.Add(time.Hour)
	s.prefs.PausedUntil = &hasta // la pone otro dispositivo
	sa.EnPrimerPlano()
	if !sa.AvisosEnPausa() {
		t.Fatal("al volver no se releyó la pausa puesta en otro dispositivo")
	}
	s.prefs.PausedUntil = nil // y la quita
	sa.EnPrimerPlano()
	if sa.AvisosEnPausa() {
		t.Error("al volver no se releyó que la pausa se quitó")
	}
}

// TestUnaLecturaViejaNoPisaLaPausa: una lectura lanzada antes de pausar que responde
// después trae el estado de antes, y no puede quitar la pausa recién confirmada.
func TestUnaLecturaViejaNoPisaLaPausa(t *testing.T) {
	ahora := time.Date(2026, 11, 5, 17, 0, 0, 0, time.UTC)
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Ahora = func() time.Time { return ahora }
	sa.Arrancar(context.Background())

	var pendientes []func()
	sa.Lanzar = func(f func()) { pendientes = append(pendientes, f) }
	sa.EnPrimerPlano()       // abre el menú: relee
	sa.PausarAvisos(Pausa1h) // y enseguida pausa
	if len(pendientes) != 2 {
		t.Fatalf("se lanzaron %d peticiones, no 2", len(pendientes))
	}
	pendientes[1]() // la pausa responde primero
	fyne.DoAndWait(func() {})
	// La lectura la atendió el servicio antes que la pausa: trae «sin pausa».
	s.prefs.PausedUntil = nil
	pendientes[0]()
	fyne.DoAndWait(func() {})
	if !sa.AvisosEnPausa() {
		t.Fatal("la lectura vieja quitó la pausa que el servicio acababa de confirmar")
	}
	// Una lectura lanzada después sí vale: si la pausa se quitó en otro dispositivo, llega.
	sa.EnPrimerPlano()
	pendientes[2]()
	fyne.DoAndWait(func() {})
	if sa.AvisosEnPausa() {
		t.Error("una lectura posterior a la pausa no se aplicó")
	}
}

// TestLasDuracionesDicenQuePausan: quien recorre el menú por sus ítems no pasa por el
// título, así que cada duración lo lleva en su nombre accesible (design.md §2.6).
func TestLasDuracionesDicenQuePausan(t *testing.T) {
	sa, _ := salaDePrueba(t, servicioConDos())
	tr := sa.P.T
	var hora *ui.ElementoMenu
	for _, o := range sa.entradasPausa(tr, sa.P.Locale, func() {}) {
		if e, ok := o.(*ui.ElementoMenu); ok && e.Texto == tr.T("pause.1h", nil) {
			hora = e
		}
	}
	if hora == nil {
		t.Fatal("el menú no ofrece «Durante 1 hora»")
	}
	want := tr.T("pause.optionAria", i18n.Vars{"option": tr.T("pause.1h", nil)})
	if got := hora.AccessibilityLabel(); got != want || got == hora.Texto {
		t.Errorf("nombre accesible %q, no %q", got, want)
	}
}

// TestUnaPausaEnVueloNoDejaPedirOtra es `pauseSaving` de la web: mientras la escritura
// está en vuelo, el menú va inerte y otra petición no sale; si salieran dos, la que
// respondiera la última ganaría aunque fuera la más vieja.
func TestUnaPausaEnVueloNoDejaPedirOtra(t *testing.T) {
	ahora := time.Date(2026, 11, 5, 17, 0, 0, 0, time.UTC)
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Ahora = func() time.Time { return ahora }
	sa.Arrancar(context.Background())
	inertes := func() (int, int) {
		si, no := 0, 0
		for _, o := range sa.entradasPausa(sa.P.T, sa.P.Locale, func() {}) {
			if e, ok := o.(*ui.ElementoMenu); ok {
				if e.Inerte {
					si++
				} else {
					no++
				}
			}
		}
		return si, no
	}

	var pendientes []func()
	sa.Lanzar = func(f func()) { pendientes = append(pendientes, f) }
	sa.PausarAvisos(Pausa30m)
	if si, no := inertes(); si != 5 || no != 0 {
		t.Fatalf("con la pausa en vuelo hay %d entradas inertes y %d vivas", si, no)
	}
	sa.PausarAvisos(Pausa2h) // el menú sigue mostrando las duraciones
	if len(pendientes) != 1 {
		t.Fatalf("con una pausa en vuelo salieron %d escrituras", len(pendientes))
	}
	pendientes[0]()
	fyne.DoAndWait(func() {})
	if si, no := inertes(); si != 0 || no != 1 {
		t.Fatalf("confirmada la pausa hay %d entradas inertes y %d vivas", si, no)
	}
	sa.PausarAvisos("")
	if len(pendientes) != 2 {
		t.Error("confirmada la primera, reanudar no salió")
	}
}
