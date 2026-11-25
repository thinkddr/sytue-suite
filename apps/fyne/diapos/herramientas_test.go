// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// Lo nuevo de presentar contra los VECTORES COMPARTIDOS con la web
// (`packages/office/test/vectores/diapos-herramientas.json`, que recorre
// `slides-herramientas.test.ts`), y sobre el visor y la tira de verdad.
const rutaHerramientas = "../../frontend/packages/office/test/vectores/diapos-herramientas.json"

type vectoresDiapos struct {
	Avance []struct {
		Ocultas     []bool
		Desde, Sale int
	}
	Rueda []struct {
		Acumulado, Abajo, Umbral float32
		Dir                      int
		Resto                    float32
	}
	Crono []struct {
		Inicio       int64
		Pausa        *int64
		Ahora        int64
		Transcurrido int64
	}
	Pausa []struct {
		Inicio int64
		Pausa  *int64
		Ahora  int64
		Sale   struct {
			Inicio int64
			Pausa  *int64
		}
	}
	Notas []struct {
		Notas []string
		Sale  string
	}
}

func leerVectoresDiapos(t *testing.T) vectoresDiapos {
	t.Helper()
	datos, err := os.ReadFile(rutaHerramientas)
	if err != nil {
		t.Fatal(err)
	}
	var v vectoresDiapos
	if err := json.Unmarshal(datos, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// ms pasa los milisegundos de los vectores a una hora (la pausa nula es la hora cero).
func ms(n *int64) time.Time {
	if n == nil {
		return time.Time{}
	}
	return time.UnixMilli(*n)
}

func TestLasHerramientasDePresentarConLosVectoresDeLaWeb(t *testing.T) {
	v := leerVectoresDiapos(t)
	for _, c := range v.Avance {
		diapos := make([]Diapo, len(c.Ocultas))
		for i, o := range c.Ocultas {
			if o {
				oculta := true
				diapos[i].Oculta = &oculta
			}
		}
		if d, p := AvanceAutomatico(diapos, c.Desde, 0); d != c.Sale || p != 0 {
			t.Errorf("avance desde %d con %v: %d/%d, quería %d", c.Desde, c.Ocultas, d, p, c.Sale)
		}
	}
	for _, c := range v.Rueda {
		if dir, resto := PasoDeRueda(c.Acumulado, c.Abajo, c.Umbral); dir != c.Dir || resto != c.Resto {
			t.Errorf("rueda %+v: %d %v", c, dir, resto)
		}
	}
	for _, c := range v.Crono {
		cr := Cronometro{Inicio: time.UnixMilli(c.Inicio), Pausa: ms(c.Pausa)}
		if got := cr.Transcurrido(time.UnixMilli(c.Ahora)).Milliseconds(); got != c.Transcurrido {
			t.Errorf("cronómetro %+v: %d", c, got)
		}
	}
	for _, c := range v.Pausa {
		got := Cronometro{Inicio: time.UnixMilli(c.Inicio), Pausa: ms(c.Pausa)}.Alternar(time.UnixMilli(c.Ahora))
		if got.Inicio.UnixMilli() != c.Sale.Inicio || !got.Pausa.Equal(ms(c.Sale.Pausa)) {
			t.Errorf("pausa %+v: %v", c, got)
		}
	}
	for _, c := range v.Notas {
		diapos := make([]Diapo, len(c.Notas))
		for i, n := range c.Notas {
			diapos[i].Notas = n
		}
		if got := NotasComoTexto(diapos, func(n int) string { return "Diapositiva " + strconv.Itoa(n) }); got != c.Sale {
			t.Errorf("notas %q: %q, quería %q", c.Notas, got, c.Sale)
		}
	}
}

// L enciende el láser: el punto sigue al ratón y el cursor se esconde; otra L lo apaga.
func TestElPunteroLaser(t *testing.T) {
	p := cincoDiapos(t)
	p.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(100, 50)}})
	if p.laserEn != nil || p.Cursor() != desktop.DefaultCursor {
		t.Fatal("sin láser, el ratón no deja punto y el cursor es el de siempre")
	}
	p.Runa('l')
	p.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(100, 50)}})
	if !p.Laser || p.laserEn == nil || *p.laserEn != fyne.NewPos(100, 50) || p.Cursor() != desktop.HiddenCursor {
		t.Fatalf("con L, el punto va donde el ratón y sin cursor: %v %v", p.laserEn, p.Cursor())
	}
	r := p.r
	if pos := r.laser.Position(); pos != fyne.NewPos(100-ladoLaser/2, 50-ladoLaser/2) {
		t.Errorf("el punto está en %v", pos)
	}
	p.Runa('L')
	if p.Laser || p.laserEn != nil {
		t.Error("otra L lo apaga")
	}
}

// La rueda pasa de diapositiva: una muesca hacia abajo, la siguiente; justo después, nada.
func TestLaRuedaPasaDeDiapositiva(t *testing.T) {
	p := cincoDiapos(t)
	p.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.Delta{DY: -25}})
	if p.Diapo != 1 {
		t.Fatalf("una muesca hacia abajo lleva a la 2, y va en la %d", p.Diapo+1)
	}
	p.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.Delta{DY: -25}})
	if p.Diapo != 1 {
		t.Error("tras un paso, la calma: la rueda no salta otra al momento")
	}
	p.ultimaRueda = time.Time{}
	p.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.Delta{DY: 25}})
	if p.Diapo != 0 {
		t.Errorf("hacia arriba vuelve a la 1, y va en la %d", p.Diapo+1)
	}
}

// El avance automático pasa solo y, al acabar, vuelve a la primera.
func TestElAvanceAutomaticoDaLaVuelta(t *testing.T) {
	p := cincoDiapos(t)
	var pendiente func()
	var espera time.Duration
	p.Programar = func(d time.Duration, f func()) func() {
		pendiente, espera = f, d
		return func() { pendiente = nil }
	}
	p.Avance = 10
	p.ArmarAvance()
	if pendiente == nil || espera != 10*time.Second {
		t.Fatalf("se programa a los 10 s: %v", espera)
	}
	for _, quiero := range []int{1, 2, 3, 4, 0} {
		pendiente()
		if p.Diapo != quiero {
			t.Fatalf("el avance lleva a la %d, quería la %d", p.Diapo+1, quiero+1)
		}
	}
	p.Runa('b') // con la pantalla en negro espera
	pendiente()
	if p.Diapo != 0 || pendiente == nil {
		t.Errorf("apagada no avanza y sigue contando: %d", p.Diapo)
	}
}

// «Pausar» para el cronómetro y «Reanudar» lo sigue donde se quedó.
func TestElCronometroSePausa(t *testing.T) {
	p := cincoDiapos(t)
	mod := NuevoModerador(p, "es")
	ahora := time.Date(2026, 11, 25, 10, 0, 0, 0, time.Local)
	mod.ahora = func() time.Time { return ahora }
	mod.crono = Cronometro{Inicio: ahora}
	ahora = ahora.Add(30 * time.Second)
	mod.Pausar()
	ahora = ahora.Add(time.Minute)
	if !mod.EnPausa() || mod.crono.Transcurrido(ahora) != 30*time.Second {
		t.Fatalf("en pausa no corre: %v", mod.crono.Transcurrido(ahora))
	}
	mod.Pausar()
	ahora = ahora.Add(5 * time.Second)
	if mod.EnPausa() || mod.crono.Transcurrido(ahora) != 35*time.Second {
		t.Errorf("reanudado sigue donde se quedó: %v", mod.crono.Transcurrido(ahora))
	}
}

// Ctrl/⌘+D sobre una miniatura la duplica; con Mayús, no.
func TestCtrlDDuplicaLaDiapositiva(t *testing.T) {
	m, err := Leer([]byte(`{"slides":[{"id":"a","boxes":[]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	veces := 0
	mini := NuevaMiniatura(m, 0, nil)
	mini.AlDuplicar = func() { veces++ }
	mini.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyD, Modifier: fyne.KeyModifierShortcutDefault})
	mini.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyD, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift})
	if veces != 1 {
		t.Errorf("Ctrl+D duplica una vez y Ctrl+Mayús+D ninguna: %d", veces)
	}
}
