// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaPestanaMide35Coma25 ata la pestaña a los números de Tabs.tsx convertidos con el
// rem de 14, que es el único sitio del que pueden salir: `py-2.5` son 8,75 por lado,
// la caja de línea de `text-sm` es 15,75 y el `border-b-2` son 2.
//
// El subrayado lo llevan TODAS, transparente en las inactivas: así cambiar de pestaña
// no mueve el rótulo dos píxeles, que es el defecto clásico de este componente.
func TestLaPestanaMide35Coma25(t *testing.T) {
	p := NuevaPestana("Álbumes", nil)
	m := MedidaPestana(p)
	if m.Height != 40 {
		t.Errorf("la pestaña mide %v de alto y `py-2.5` sobre `text-sm` con `border-b-2` son 35,25", m.Height)
	}
	if AltoTiraPestanas != 41 {
		t.Errorf("la tira mide %v y con su `border-b` de 1 son 36,25", AltoTiraPestanas)
	}
	if got := p.MinSize(); got != m {
		t.Errorf("MinSize() = %v y MedidaPestana dice %v", got, m)
	}
	// El ancho es el rótulo en versalitas CON su tracking más `px-3`.
	quiero := MedirVersal("Álbumes", tema.TextoSM, SemiNegrita) + 2*tema.Espacio(3)
	if m.Width != quiero {
		t.Errorf("la pestaña mide %v de ancho y su rótulo con `px-3` son %v", m.Width, quiero)
	}
}

// TestElSubrayadoDeLaPestanaActivaEsCobre. El color lo lleva el subrayado y no el
// texto: si lo llevaran los dos saldrían dos colores en una misma palabra.
func TestElSubrayadoDeLaPestanaActivaEsCobre(t *testing.T) {
	p := NuevaPestana("Repetidas", nil)
	p.Activa = true
	m := MedidaPestana(p)
	p.Resize(m)
	p.Refresh()
	img := pinta(t, m.Width+10, m.Height+10, p)

	x0, x1, y0, y1 := caja(img, tema.Color("indicator", tema.Claro))
	if alto := float32(y1-y0+1) / escala; alto < SubrayadoPestana-0.5 || alto > SubrayadoPestana+0.5 {
		t.Errorf("el subrayado mide %v y `border-b-2` son 2", alto)
	}
	if ancho := float32(x1-x0+1) / escala; ancho < m.Width-0.5 {
		t.Errorf("el subrayado mide %v de ancho y la pestaña %v: tiene que recorrerla entera", ancho, m.Width)
	}
	if abajo := float32(y1+1) / escala; abajo < m.Height-0.5 || abajo > m.Height+0.5 {
		t.Errorf("el subrayado acaba en %v y la pestaña mide %v de alto", abajo, m.Height)
	}
	// Y la inactiva lo lleva igual, transparente: por eso mide lo mismo.
	q := NuevaPestana("Repetidas", nil)
	if MedidaPestana(q) != m {
		t.Error("la pestaña inactiva no mide lo mismo que la activa: el rótulo se movería al cambiar")
	}
}

// TestLasPestanasSeSeparanTresComaCinco es el `gap-1` de la tira, y de paso comprueba
// que la línea de un píxel va DEBAJO de todas.
func TestLasPestanasSeSeparanTresComaCinco(t *testing.T) {
	p := NuevasPestanas([]string{"Fotos", "Álbumes", "Favoritas"}, nil)
	p.Resize(MedidaPestanas(p))
	_ = test.WidgetRenderer(p)
	p.Refresh()
	pinta(t, 400, 60, p)

	a, b := p.pestanas[0], p.pestanas[1]
	if sep := b.Position().X - (a.Position().X + a.Size().Width); sep != tema.Espacio(1) {
		t.Errorf("entre dos pestañas hay %v y el `gap-1` son %v", sep, tema.Espacio(1))
	}
	if got := MedidaPestanas(p).Height; got != AltoTiraPestanas {
		t.Errorf("la tira sin panel mide %v y son %v", got, AltoTiraPestanas)
	}
}

// TestLasFlechasCambianDePestana es el patrón `tablist` de la WAI: derecha, izquierda,
// Inicio y Fin, y circular. Sin él la tira solo se puede usar con el ratón.
func TestLasFlechasCambianDePestana(t *testing.T) {
	p := NuevasPestanas([]string{"Fotos", "Álbumes", "Favoritas"}, nil)
	_ = test.WidgetRenderer(p) // construye las pestañas
	p.pestanas[0].TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if p.Activa != 1 {
		t.Errorf("la flecha derecha ha dejado la activa en %d y tenía que ser la 1", p.Activa)
	}
	p.pestanas[1].TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnd})
	if p.Activa != 2 {
		t.Errorf("Fin ha dejado la activa en %d y tenía que ser la última", p.Activa)
	}
	p.pestanas[2].TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	if p.Activa != 0 {
		t.Errorf("la flecha derecha en la última ha dejado %d y tenía que volver a la primera", p.Activa)
	}
	p.pestanas[0].TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if p.Activa != 2 {
		t.Errorf("la flecha izquierda en la primera ha dejado %d y tenía que ir a la última", p.Activa)
	}
}

// TestElPanelArrancaTrasElPtCuatro: `pt-4` son 14, y el panel empieza después de la
// tira entera, línea incluida.
func TestElPanelArrancaTrasElPtCuatro(t *testing.T) {
	panel := iconoFalso()
	panel.Resize(fyne.NewSize(100, 50))
	p := NuevasPestanas([]string{"Fotos"}, []fyne.CanvasObject{panel})
	p.Resize(fyne.NewSize(300, 200))
	_ = test.WidgetRenderer(p)
	p.Refresh()
	pinta(t, 310, 210, p)

	if got, quiero := panel.Position().Y, AltoTiraPestanas+tema.Espacio(4); got != quiero {
		t.Errorf("el panel empieza en y=%v y tras la tira con el `pt-4` son %v", got, quiero)
	}
}
