// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Los glifos de Cast son los de Icon.tsx: el catálogo los tiene todos.
//
// VISTO EN ROJO (2026-11-06): ocho glifos dibujados a mano en seis ficheros (casa,
// ondas, vídeo, reproductor), dos de ellos copia exacta del catálogo.
func TestCastNoDibujaGlifosPropios(t *testing.T) {
	ficheros, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	propio := regexp.MustCompile(`iconos\.(TrazoDeRuta|RellenoDeRuta|TrazoDeRutaRelleno|TrazoDeCuerpo)\(`)
	for _, f := range ficheros {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		datos, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range propio.FindAllString(string(datos), -1) {
			t.Errorf("%s: %s — el glifo de la web está en el catálogo", f, m)
		}
	}
}

// El menú lleva los iconos de Nav.tsx, en su orden.
//
// VISTO EN ROJO (2026-11-06): Explorar era un globo y la web pinta una brújula.
func TestLosGlifosDelMenuSonLosDeNav(t *testing.T) {
	web := webref.Read(t, "apps/frontend/apps/cast/src/components/Nav.tsx")
	hallados := regexp.MustCompile(`icono: '(\w+)'`).FindAllStringSubmatch(web, -1)
	claves := make([]string, 0, len(hallados))
	for _, m := range hallados {
		claves = append(claves, m[1])
	}
	if len(claves) != len(destinos) {
		t.Fatalf("Nav.tsx tiene %d destinos y aquí hay %d", len(claves), len(destinos))
	}
	for i, d := range destinos {
		if got := d.icono.Clave(); got != claves[i] {
			t.Errorf("destino %d: %q, la web %q", i, got, claves[i])
		}
	}
}

// «Emitir» en la cabecera de Inicio lleva su glifo, como «Subir» y como en HomeApp.tsx.
//
// VISTO EN ROJO (2026-11-06): «Emitir» salía sin glifo junto a «Subir» con el suyo.
func TestEmitirDeInicioLlevaSuGlifo(t *testing.T) {
	v := vistaDePrueba(t, tema.Claro)
	var emitir *ui.Boton
	recorrer(PantallaInicio(v, Inicio{}), func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok && b.Texto == v.Traduce("nav.studio") {
			emitir = b
		}
	})
	if emitir == nil {
		t.Fatal("no está «Emitir» en la cabecera")
	}
	if emitir.Icono != iconos.Emitir {
		t.Errorf("«Emitir» lleva %q, la web «emitir»", emitir.Icono.Clave())
	}
}

// El glifo de «Emitir ahora» va con la tinta del rótulo, también apagado.
//
// VISTO EN ROJO (2026-11-06): el glifo salía en `text`, casi negro sobre el primario,
// y sin atenuar con el botón inerte.
func TestEmitirAhoraTineSuGlifoConElRotulo(t *testing.T) {
	conVariante(t, tema.Claro)
	v := vistaDePrueba(t, tema.Claro)
	elegido := videoDePrueba("d1", "Directo", "Ana", 0, 0, 0)
	for _, listo := range []bool{true, false} {
		var boton *ui.Boton
		recorrer(panelEmision(v, Emision{Elegido: &elegido, Camara: listo}), func(o fyne.CanvasObject) {
			if b, ok := o.(*ui.Boton); ok && b.Texto == v.Traduce("studio.browser.start") {
				boton = b
			}
		})
		if boton == nil {
			t.Fatal("no está «Emitir ahora»")
		}
		// Sin glifo no habría píxeles que contar y la prueba saldría verde sin mirar nada.
		if boton.Icono != iconos.Emitir || boton.Glifo != nil {
			t.Fatalf("listo=%v: «Emitir ahora» lleva %q (glifo a mano: %v), la web «emitir»", listo, boton.Icono.Clave(), boton.Glifo != nil)
		}
		w := test.NewWindow(boton)
		w.SetPadded(false)
		m := boton.MinSize()
		w.Resize(m)
		img := w.Canvas().Capture()
		texto := tema.Color("text", v.Variante)
		cerca := func(c color.NRGBA) bool {
			d := func(x, y uint8) int { return max(int(x)-int(y), int(y)-int(x)) }
			return d(c.R, texto.R) < 40 && d(c.G, texto.G) < 40 && d(c.B, texto.B) < 40
		}
		oscuros := 0
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if cerca(color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)) {
					oscuros++
				}
			}
		}
		w.Close()
		if oscuros > 0 {
			t.Errorf("listo=%v: %d píxeles en la tinta de `text` sobre el primario", listo, oscuros)
		}
	}
}
