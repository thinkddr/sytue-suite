// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// cambiarTema cambia de tema con la ventana abierta, como el menú de cuenta: la variante
// de la suite y el SetTheme con el que Fyne refresca lo que hay.
func cambiarTema(t *testing.T, nombre string) {
	t.Helper()
	tema.Elegir(nombre)
	fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
}

// TestElLisoSigueAlTemaEnCaliente: un Liso compuesto en claro pinta su token en oscuro
// en cuanto se cambia de tema, y uno calculado (NuevoLisoCon) vuelve a calcularse. Es lo
// que no hacía el `canvas.NewRectangle(tema.Color(…))` al que sustituye: el refresco de
// Fyne lo repintaba con el color de la variante en que nació.
func TestElLisoSigueAlTemaEnCaliente(t *testing.T) {
	t.Cleanup(func() { cambiarTema(t, "") })
	cambiarTema(t, "claro")
	liso := NuevoLiso("surface")
	velo := NuevoLisoCon(func(v tema.Variante) color.NRGBA {
		return tokens.SobreSRGB(tema.Color("selected", v), tema.Color("surface", v))
	})
	w := test.NewTempWindow(t, container.NewGridWithColumns(2, liso, velo))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(40, 20))
	en := func(x int) color.NRGBA {
		return color.NRGBAModel.Convert(w.Canvas().Capture().At(x, 10)).(color.NRGBA) //nolint:forcetypeassert // el modelo devuelve eso
	}
	for _, v := range []struct {
		nombre string
		v      tema.Variante
	}{{"claro", tema.Claro}, {"oscuro", tema.Oscuro}, {"claro", tema.Claro}} {
		cambiarTema(t, v.nombre)
		if got, quiero := en(5), tema.Color("surface", v.v); got != quiero {
			t.Errorf("en %s el liso pinta %v y su `surface` es %v", v.nombre, got, quiero)
		}
		if got, quiero := en(35), velo.Color(v.v); got != quiero {
			t.Errorf("en %s el liso calculado pinta %v y su cuenta da %v", v.nombre, got, quiero)
		}
	}
}

// TestRehacerConElTemaRehaceAlCambiarlo: lo envuelto se rehace UNA vez al cambiar de
// tema en caliente, y no al colocarse otra vez con el mismo tema.
func TestRehacerConElTemaRehaceAlCambiarlo(t *testing.T) {
	t.Cleanup(func() { cambiarTema(t, "") })
	cambiarTema(t, "claro")
	veces := 0
	o := RehacerConElTema(NuevoLiso("surface"), func() { veces++ })
	w := test.NewTempWindow(t, o)
	w.Resize(fyne.NewSize(40, 20))
	w.Resize(fyne.NewSize(60, 30))
	if veces != 0 {
		t.Fatalf("sin cambiar de tema se ha rehecho %d veces", veces)
	}
	cambiarTema(t, "oscuro")
	if veces != 1 {
		t.Errorf("al pasar a oscuro se ha rehecho %d veces; tiene que ser una", veces)
	}
	w.Resize(fyne.NewSize(80, 40))
	cambiarTema(t, "claro")
	if veces != 2 {
		t.Errorf("tras volver a claro se ha rehecho %d veces en total; tienen que ser dos", veces)
	}
}

// TestElParrafoSigueASuTokenEnCaliente: el párrafo de NuevoParrafo, compuesto con la
// variante en vigor, vuelve a resolver su token al cambiar de tema (las pistas de los
// paneles de CertiPDF, Docus y Calcus se quedaban con la tinta clara sobre el panel
// oscuro). El que se tiñó a mano sigue con lo que le pusieron, y el que se pidió en
// otra variante —uno sobre papel, que es claro en los dos temas— no se mueve.
func TestElParrafoSigueASuTokenEnCaliente(t *testing.T) {
	t.Cleanup(func() { cambiarTema(t, "") })
	cambiarTema(t, "claro")
	vivo := NuevoParrafo("vivo", tema.TextoSM, Normal, "text-muted", tema.Claro)
	amano := NuevoParrafo("a mano", tema.TextoSM, Normal, "text-muted", tema.Claro)
	propia := color.NRGBA{R: 0x80, A: 0xff}
	amano.Tinta = propia
	fijo := NuevoParrafo("fijo", tema.TextoSM, Normal, "text-muted", tema.Oscuro)
	w := test.NewTempWindow(t, container.NewVBox(vivo, amano, fijo))
	w.Resize(fyne.NewSize(200, 100))
	tinta := func(p *Parrafo) color.Color {
		return test.TempWidgetRenderer(t, p).Objects()[0].(*canvas.Text).Color //nolint:forcetypeassert // la primera línea
	}
	cambiarTema(t, "oscuro")
	if got, quiero := tinta(vivo), tema.Color("text-muted", tema.Oscuro); got != quiero {
		t.Errorf("tras pasar a oscuro el párrafo pinta %v y su `text-muted` es %v", got, quiero)
	}
	if got := tinta(amano); got != propia {
		t.Errorf("el párrafo teñido a mano pinta %v tras el cambio, y se le puso %v", got, propia)
	}
	if got, quiero := tinta(fijo), tema.Color("text-muted", tema.Oscuro); got != quiero {
		t.Errorf("el párrafo pedido en oscuro pinta %v; tiene que seguir en %v", got, quiero)
	}
	cambiarTema(t, "claro")
	if got, quiero := tinta(vivo), tema.Color("text-muted", tema.Claro); got != quiero {
		t.Errorf("de vuelta en claro el párrafo pinta %v y su `text-muted` es %v", got, quiero)
	}
	if got, quiero := tinta(fijo), tema.Color("text-muted", tema.Oscuro); got != quiero {
		t.Errorf("el párrafo pedido en oscuro pinta %v en claro; tiene que seguir en %v", got, quiero)
	}
}
