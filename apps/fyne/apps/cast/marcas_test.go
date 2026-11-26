// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestCapitulosComoEnLaWeb: los casos de `capitulosDe` en lib/marcas.test.ts.
func TestCapitulosComoEnLaWeb(t *testing.T) {
	s := time.Second
	d := "Charla de octubre.\n\n0:00 Presentación\n- 1:30 – Demo\n(12:05) Preguntas\nGracias."
	want := []Capitulo{{0, "Presentación"}, {90 * s, "Demo"}, {725 * s, "Preguntas"}}
	if got := CapitulosDe(d); !reflect.DeepEqual(got, want) {
		t.Errorf("capítulos = %+v, want %+v", got, want)
	}
	for _, malo := range []string{
		"0:10 Uno\n1:00 Dos",
		"0:00 Solo",
		"0:00 Uno\n2:00 Dos\n1:00 Tres",
		"Quedamos a las 0:00 y a las 1:00",
	} {
		if got := CapitulosDe(malo); got != nil {
			t.Errorf("%q no es un índice y dio %+v", malo, got)
		}
	}
}

func TestEnlaceEnUnMomento(t *testing.T) {
	for _, c := range []struct {
		enlace string
		en     time.Duration
		want   string
	}{
		{"https://x.eu/cast/watch/v1", 83900 * time.Millisecond, "https://x.eu/cast/watch/v1?t=83"},
		{"https://x.eu/cast/watch/v1?t=5", 0, "https://x.eu/cast/watch/v1"},
		{"/cast/watch/v1?a=1&t=5", 90 * time.Second, "/cast/watch/v1?a=1&t=90"},
	} {
		if got := EnlaceEn(c.enlace, c.en); got != c.want {
			t.Errorf("EnlaceEn(%q, %v) = %q, want %q", c.enlace, c.en, got, c.want)
		}
	}
}

// TestTeclasQueLlevanAUnSitio: los casos de `destinoDeTecla` de la web.
func TestTeclasQueLlevanAUnSitio(t *testing.T) {
	s := time.Second
	for _, c := range []struct {
		k        fyne.KeyName
		pos, dur time.Duration
		want     time.Duration
		ok       bool
	}{
		{fyne.KeyJ, 30 * s, 100 * s, 20 * s, true},
		{fyne.KeyL, 95 * s, 100 * s, 100 * s, true},
		{fyne.KeyJ, 4 * s, 100 * s, 0, true},
		{fyne.KeyHome, 50 * s, 100 * s, 0, true},
		{fyne.KeyEnd, 50 * s, 100 * s, 100 * s, true},
		{fyne.Key0, 50 * s, 100 * s, 0, true},
		{fyne.Key7, 0, 200 * s, 140 * s, true},
		{fyne.KeyX, 0, 100 * s, 0, false},
		{fyne.Key5, 0, 0, 0, false},
	} {
		got, ok := DestinoDeTecla(c.k, c.pos, c.dur)
		if got != c.want || ok != c.ok {
			t.Errorf("DestinoDeTecla(%s, %v, %v) = %v, %v; want %v, %v", c.k, c.pos, c.dur, got, ok, c.want, c.ok)
		}
	}
}

// TestLosCapitulosDeLaDescripcionLlevanAlli: «Capítulos» bajo la descripción, cada uno
// con su marca, y pulsarlo pide el salto (Capitulos de ConMarcas.tsx). En directo no.
func TestLosCapitulosDeLaDescripcionLlevanAlli(t *testing.T) {
	test.NewTempApp(t)
	v := vistaDePrueba(t, tema.Claro)
	var saltos []int64
	vid := servicioCompleto().mios[0]
	vid.Description = "Orden del día:\n0:00 Presentación\n1:30 Demo"
	d := Ver{Video: vid, AlCapitulo: func(ms int64) { saltos = append(saltos, ms) }, Comentarios: Comentarios{Campo: ui.NuevaEntrada("")}}
	capitulos := func(d Ver) []*ui.Enlace {
		var out []*ui.Enlace
		recorrer(PantallaVer(v, d), func(o fyne.CanvasObject) {
			if e, ok := o.(*ui.Enlace); ok && strings.Contains(e.Texto, "  ") {
				out = append(out, e)
			}
		})
		return out
	}
	enlaces := capitulos(d)
	if len(enlaces) != 2 || enlaces[1].Texto != "1:30  Demo" {
		t.Fatalf("capítulos = %d, el segundo %q", len(enlaces), func() string {
			if len(enlaces) > 1 {
				return enlaces[1].Texto
			}
			return ""
		}())
	}
	test.Tap(enlaces[1])
	if len(saltos) != 1 || saltos[0] != 90000 {
		t.Errorf("el capítulo pidió %v", saltos)
	}
	d.Video.Status = api.CastStatusLive
	if n := len(capitulos(d)); n != 0 {
		t.Errorf("un directo no tiene a qué capítulo ir, y salen %d", n)
	}
}

// TestCompartirDesdeUnMomento: con el vídeo empezado, «Empezar en 1:23» pone el «?t=»
// en el enlace que se copia, como la casilla de ShareDialog.tsx.
func TestCompartirDesdeUnMomento(t *testing.T) {
	s := servicioCompleto()
	a, _ := appDePrueba(t, s, true)
	a.AbrirVideo("v01")
	a.abrirCompartir()
	if d := a.datosCompartir(); d.AlEnMomento != nil {
		t.Fatal("sin haber empezado no hay «Empezar en…»")
	}
	a.ver.momento = 83 * time.Second
	var casilla *ui.Eleccion
	recorrer(CuerpoCompartir(a.v, a.datosCompartir()), func(o fyne.CanvasObject) {
		if c, ok := o.(*ui.Eleccion); ok && c.Etiqueta == a.v.Traduce("share.startAt", i18n.Vars{"time": "1:23"}) {
			casilla = c
		}
	})
	if casilla == nil {
		t.Fatal("no hay «Empezar en 1:23»")
	}
	casilla.AlCambiar(true)
	a.copiarEnlace()
	if got := fyne.CurrentApp().Clipboard().Content(); got != "https://cast.sytue.eu/cast/watch/v01?t=83" {
		t.Errorf("el enlace es %q", got)
	}
}
