// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// CasillaNativa es el `<input type="checkbox">` de la tabla y de las tarjetas del ERP,
// que la web deja con el aspecto del NAVEGADOR (sus clases `border-border rounded` no
// tocan un control con `appearance: auto`): 13×13, esquina de 2, y los colores de
// Chromium con `color-scheme` claro u oscuro. No es `ui.Eleccion`, que es el `Checkbox`
// de la suite (14, `border-strong` y el primario).
//
// Los colores, medidos en el arnés píxel a píxel, son los de `ui.CasillaNativa`.
type CasillaNativa struct {
	ui.Pulsable
	Marcada bool
}

var _ fyne.Widget = (*CasillaNativa)(nil)

// LadoCasillaNativa es el lado del control del navegador: 13. Medido.
const LadoCasillaNativa float32 = 13

// NuevaCasillaNativa crea la casilla. `nombre` es su `aria-label`.
func NuevaCasillaNativa(nombre string, marcada bool, alPulsar func()) *CasillaNativa {
	c := &CasillaNativa{Marcada: marcada}
	c.Nombre, c.AlPulsar = nombre, alPulsar
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel es el `aria-label`.
func (c *CasillaNativa) AccessibilityLabel() string { return c.Nombre }

// MinSize es el lado del control.
func (c *CasillaNativa) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	return fyne.NewSize(LadoCasillaNativa, LadoCasillaNativa)
}

// CreateRenderer arma el anillo de foco y la pieza de Chromium de `ui`, que es la que
// dibuja la caja, el tic y sus colores.
func (c *CasillaNativa) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &casillaNativaRenderer{
		c:     c,
		aro:   ui.NuevoAro(ui.RadioCasillaNativa, ui.SinHueco, v),
		pieza: ui.NuevaPiezaCasilla(LadoCasillaNativa, false, true, v),
	}
	r.Refresh()
	return r
}

type casillaNativaRenderer struct {
	c     *CasillaNativa
	aro   *ui.Aro
	pieza *ui.PiezaCasilla
}

func (r *casillaNativaRenderer) MinSize() fyne.Size { return r.c.MinSize() }
func (r *casillaNativaRenderer) Objects() []fyne.CanvasObject {
	return append(r.aro.Objetos(), r.pieza.Objetos()...)
}
func (r *casillaNativaRenderer) Destroy() {}

func (r *casillaNativaRenderer) Layout(t fyne.Size) {
	lado := fyne.NewSize(LadoCasillaNativa, LadoCasillaNativa)
	pos := fyne.NewPos((t.Width-lado.Width)/2, (t.Height-lado.Height)/2)
	r.pieza.Colocar(pos)
	r.aro.ColocarEn(pos, lado)
}

// Refresh pinta el estado: marcada, con el ratón encima, y en reposo.
func (r *casillaNativaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.pieza.Pintar(ui.EstadoCasilla{Marcada: r.c.Marcada, Encima: r.c.Encima()}, v)
	r.aro.Ver(r.c.Enfocado())
	r.aro.Pintar(v)
}
