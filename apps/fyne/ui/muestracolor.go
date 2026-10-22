// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// MuestraColor es una muestra REDONDA que elige un color. Calcus conserva el lado de
// `h-5`; CertiPDF fija Lado a los 24 px de `pdf/Barra.tsx`, grupo «Trazo»:
//
//	<button class="sytue-focus-ring h-[24px] w-[24px] rounded-full border transition
//	               {elegida ? 'border-text scale-110' : 'border-border'}"
//	        style="background-color: {color}" aria-pressed={elegida}>
//
// # Por qué es UNA pieza y no la muestra de color de la suite
//
// La web pinta muestras de color en otros tres sitios y NINGUNO es esta: la paleta del
// editor (`editor/ribbon/primitives.tsx`) es CUADRADA, `h-6 w-6 rounded-[3px]`; la del
// avatar (`AvatarPicker.tsx`) mide `h-7 w-7` y su borde es un `ring-inset`, por DENTRO;
// y el punto del calendario es un `h-2.5` sin borde que no se pulsa. Una pieza con
// parámetros para las cuatro sería cuatro piezas con un `switch` dentro, y la paridad
// manda sobre la reutilización. Quien porte otra de esas escribe la suya aquí al lado.
//
// # La escala de la elegida no ocupa sitio
//
// `scale-110` es un `transform`, y un `transform` no participa en el layout de CSS: la
// elegida CRECE un 10 % alrededor de su centro sin empujar a sus vecinas. Aquí
// igual: MinSize conserva el lado declarado y la caja pintada se sale por los lados,
// como hace el [Aro]. El borde sí se queda en 1 px (el navegador lo escala a 1,1): esa
// décima no la distingue nadie y escalarlo pediría una tercera caja.
type MuestraColor struct {
	pulsable

	// Color es el relleno, tal cual: NO es un token, es el color que la app va a
	// escribir (en CertiPDF, dentro del PDF).
	Color color.NRGBA
	// Lado es el tamaño declarado en píxeles. Cero conserva el `h-5` compartido.
	Lado float32
	// Elegida es `aria-pressed`: borde de `text` y un 10 % más grande. El color solo no
	// comunica estado (WCAG 1.4.1); el borde oscuro es lo que lo dice.
	Elegida bool
}

// LadoMuestraColor es el lado por defecto: `h-5 w-5`, 20 con el rem de 16.
var LadoMuestraColor = tema.Espacio(5)

// EscalaMuestraElegida es `scale-110`.
const EscalaMuestraElegida float32 = 1.1

var _ fyne.Widget = (*MuestraColor)(nil)

// NuevaMuestraColor crea la muestra. `nombre` es el `aria-label`, YA traducido y
// obligatorio: una paleta sin nombres no la puede usar quien no distingue el ámbar del
// verde.
func NuevaMuestraColor(c color.NRGBA, nombre string, alPulsar func()) *MuestraColor {
	m := &MuestraColor{Color: c}
	m.Nombre, m.AlPulsar = nombre, alPulsar
	m.ExtendBaseWidget(m)
	return m
}

// MedidaMuestraColor es lo que ocupa, elegida o no: la escala no cuenta.
func MedidaMuestraColor() fyne.Size { return fyne.NewSize(LadoMuestraColor, LadoMuestraColor) }

func (m *MuestraColor) lado() float32 {
	if m.Lado > 0 {
		return m.Lado
	}
	return LadoMuestraColor
}

// MinSize conserva el lado declarado, aunque la muestra esté elegida.
func (m *MuestraColor) MinSize() fyne.Size {
	m.ExtendBaseWidget(m)
	lado := m.lado()
	return fyne.NewSize(lado, lado)
}

func (m *MuestraColor) borde(v tema.Variante) color.NRGBA {
	if m.Elegida {
		return tema.Color("text", v)
	}
	return tema.Color("border", v)
}

// CreateRenderer: el aro detrás y la caja redonda encima. El radio es el lado ENTERO y
// no la mitad a propósito: Fyne lo recorta a la mitad del lado que tenga la caja en cada
// momento, así que el mismo número vale para la muestra en reposo y para la escalada.
func (m *MuestraColor) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	lado := m.lado() * EscalaMuestraElegida
	r := &muestraColorRenderer{
		m:    m,
		aro:  NuevoAro(lado, SinHueco, v),
		caja: NuevaCaja(m.Color, m.borde(v), lado),
	}
	r.objetos = r.aro.Antes(r.caja.Objetos()...)
	r.Refresh()
	return r
}

type muestraColorRenderer struct {
	m       *MuestraColor
	aro     *Aro
	caja    *Caja
	objetos []fyne.CanvasObject
}

func (r *muestraColorRenderer) MinSize() fyne.Size           { return r.m.MinSize() }
func (r *muestraColorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *muestraColorRenderer) Destroy()                     {}

// Layout escala alrededor del CENTRO, que es el `transform-origin` por defecto.
func (r *muestraColorRenderer) Layout(t fyne.Size) {
	lado := min(t.Width, t.Height)
	if r.m.Elegida {
		lado *= EscalaMuestraElegida
	}
	pos := fyne.NewPos((t.Width-lado)/2, (t.Height-lado)/2)
	tam := fyne.NewSize(lado, lado)
	MoverCaja(r.caja, pos, tam)
	r.aro.ColocarEn(pos, tam)
}

// Refresh repinta y RECOLOCA: elegir cambia el tamaño pintado sin cambiar la medida,
// así que Fyne no llamaría a Layout por su cuenta.
func (r *muestraColorRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(r.m.Color, r.m.borde(v))
	r.aro.Pintar(v)
	r.aro.Ver(r.m.Enfocado())
	r.Layout(r.m.Size())
}
