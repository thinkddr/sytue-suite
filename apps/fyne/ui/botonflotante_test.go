// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElBotonFlotanteEsUnMasRedondoEnTodosLosAnchos es el contrato que pidió producto el
// 2026-11-25 (BotonFlotante.test.tsx en la web): la acción de crear es un «+» REDONDO de
// 56 que flota a 16 de la esquina de abajo a la derecha EN TODOS LOS ANCHOS —dentro de la
// hoja: en «Marco» y desde 768, 10 más—. Era cuadrado con la esquina de los botones y
// `md:hidden`: desde 768 no se veía.
func TestElBotonFlotanteEsUnMasRedondoEnTodosLosAnchos(t *testing.T) {
	forma := "carril"
	if tema.EsMarco() {
		forma = "marco"
	}
	t.Cleanup(func() { tema.ElegirForma(forma) })
	fab := NuevoBotonFlotante("Crear evento", nil)
	capa := container.New(CapaFlotante{}, fab)
	for _, c := range []struct {
		forma  string
		ancho  fyne.Size
		margen float32
	}{
		{"marco", fyne.NewSize(390, 844), 16},
		{"marco", fyne.NewSize(1280, 800), 26},
		{"carril", fyne.NewSize(1280, 800), 16},
	} {
		tema.ElegirForma(c.forma)
		ancho := c.ancho
		capa.Resize(fyne.NewSize(0, 0))
		capa.Resize(ancho)
		quiero := fyne.NewPos(ancho.Width-c.margen-56, ancho.Height-c.margen-56)
		if !fab.Visible() || fab.Position() != quiero || fab.Size() != fyne.NewSize(56, 56) {
			t.Errorf("a %v: visible %v en %v de %v; quiero %v de 56×56",
				ancho, fab.Visible(), fab.Position(), fab.Size(), quiero)
		}
	}

	r := test.WidgetRenderer(fab).(*flotanteRenderer)
	if r.fondo.CornerRadius != 28 || r.velo.CornerRadius != 28 {
		t.Errorf("el radio es %v (velo %v) y `rounded-full` sobre 56 es 28",
			r.fondo.CornerRadius, r.velo.CornerRadius)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	suelto := NuevoBotonFlotante("Crear evento", nil)
	suelto.Resize(fyne.NewSize(56, 56))
	img := pinta(t, 56, 56, suelto)
	// (8, 8) cae dentro de un cuadrado con la esquina de 4 y fuera del círculo.
	if c := px(img, int(8*escala), int(8*escala)); c == tema.Color("primary", v) {
		t.Errorf("la esquina del botón está rellena (%v): no es redondo", c)
	}
	if c := px(img, int(28*escala), int(6*escala)); c != tema.Color("primary", v) {
		t.Errorf("arriba en el centro el botón no es `primary`: %v", c)
	}

	// El glifo es el «+» de 24 en `on-primary`, píxel a píxel, sobre el `primary` del botón:
	// sobre el blanco de `pinta` un glifo blanco no se ve y cualquiera daría igual.
	primario := tema.Color("primary", v)
	ref := iconos.Trazo(iconos.Mas, iconos.TallaXL, "on-primary", v)
	ref.Colocar(0, 0, iconos.TallaXL)
	esperado := pintaSobre(t, primario, iconos.TallaXL, iconos.TallaXL, ref.Objeto())
	quieto := r.glifo.Objeto().Position()
	r.glifo.Colocar(0, 0, iconos.TallaXL)
	defer r.glifo.Colocar(quieto.X, quieto.Y, iconos.TallaXL)
	actual := pintaSobre(t, primario, iconos.TallaXL, iconos.TallaXL, r.glifo.Objeto())
	distintos := 0
	for y := range esperado.Bounds().Dy() {
		for x := range esperado.Bounds().Dx() {
			if px(esperado, x, y) != primario {
				distintos++
			}
		}
	}
	if distintos == 0 {
		t.Fatal("el «+» de referencia no pinta nada: la comparación no probaría nada")
	}
	for y := range actual.Bounds().Dy() {
		for x := range actual.Bounds().Dx() {
			if px(actual, x, y) != px(esperado, x, y) {
				t.Fatalf("el glifo no es el «+» en (%d,%d)", x, y)
			}
		}
	}
}

// TestElBotonFlotanteInerteSeApagaYNoCrea: `disabled:opacity-50`, como Contacts sin libreta.
func TestElBotonFlotanteInerteSeApagaYNoCrea(t *testing.T) {
	pulsado := 0
	fab := NuevoBotonFlotante("Nuevo contacto", func() { pulsado++ })
	fab.Inerte = true
	w := test.NewTempWindow(t, container.New(CapaFlotante{}, fab))
	w.Resize(fyne.NewSize(1280, 800))
	test.Tap(fab)
	if pulsado != 0 {
		t.Error("un «+» inerte crea igual")
	}
	r := test.WidgetRenderer(fab).(*flotanteRenderer)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if r.fondo.FillColor == tema.Color("primary", v) {
		t.Error("un «+» inerte se pinta como uno vivo")
	}
}

func TestElBotonFlotanteSeAtenuaYCierraElCajonAbierto(t *testing.T) {
	sinAnimacionDelMenu(t)
	pulsaciones := 0
	a := NuevoArmazon(contenidoDePrueba())
	a.Menu = NuevoLateral("Menú", nil)
	fab := NuevoBotonFlotante("Redactar", func() { pulsaciones++ })
	raiz := container.NewStack(a, container.New(CapaFlotante{}, fab))
	w := test.NewTempWindow(t, raiz)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 844))
	rf := test.WidgetRenderer(fab).(*flotanteRenderer)

	if rf.velo.Visible() {
		t.Fatal("el botón flotante aparece velado con el cajón cerrado")
	}
	a.AbrirCajon()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if !rf.velo.Visible() || rf.velo.FillColor != ColorVelo(v, 1) {
		t.Fatalf("el botón flotante no se atenúa bajo el velo del cajón: visible=%v color=%v", rf.velo.Visible(), rf.velo.FillColor)
	}
	test.Tap(fab)
	if pulsaciones != 0 || a.CajonAbierto || rf.velo.Visible() {
		t.Fatalf("pulsar el botón flotante bajo el velo disparó la acción (%d) o no cerró el cajón (%v)", pulsaciones, a.CajonAbierto)
	}
	test.Tap(fab)
	if pulsaciones != 1 {
		t.Fatalf("con el cajón cerrado el botón flotante no ejecutó su acción: %d", pulsaciones)
	}

	fab.Inerte = true
	a.AbrirCajon()
	fab.Refresh()
	if rf.velo.Visible() {
		t.Fatal("un botón flotante inerte no debe pintar un segundo velo")
	}
}
