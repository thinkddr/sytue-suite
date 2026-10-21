// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// BotonPaleta es el «Ir a…  ⌘K» de arriba del menú.
//
// No es `ui.Boton`: el de la suite centra su rótulo y este reparte —el texto pegado a
// la izquierda y la tecla a la derecha (`justify-between`)—, así que con el botón del
// catálogo el atajo saldría pegado al rótulo en mitad del carril. Es el mismo criterio
// por el que Mail escribe su propia fila de conversación.
//
// Medido en `facturas.ancho.json` el 2026-09-22: x 10,5, w 188, h **31,75**, que es
// `py-2` a los dos lados sobre la caja de `text-sm` más el borde (7+15,75+7+2).
type BotonPaleta struct {
	ui.Pulsable

	// Rotulo es el texto de la izquierda («Ir a…»).
	Rotulo string
	// Atajo es la tecla de la derecha, y es DECORACIÓN: en la web va en un `<kbd>` con
	// `aria-hidden`, porque el nombre accesible ya la nombra sin depender de que se
	// entienda el símbolo.
	Atajo string
}

// Medidas del botón, de hoja salvo el alto, que está medido.
const (
	// BotonPaletaPX es `px-3` y BotonPaletaPY el `py-2`.
	BotonPaletaPX float32 = 3 * 0.25 * tema.Rem
	BotonPaletaPY float32 = 2 * 0.25 * tema.Rem
)

// AltoBotonPaleta es `py-2` sobre la caja de `text-sm`, con el borde por dentro, y el
// mínimo de los botones del cajón (`#erp-cajon > button` en app.css), que es el que
// manda: 44.
var AltoBotonPaleta = max(tema.TextoSM.Linea+2*BotonPaletaPY+2*tema.Borde, AltoMinimoCajon)

// AltoBotonIA es el `h-8` del botón de Sytue AI con ese mismo mínimo: 44.
var AltoBotonIA = max(2*tema.Rem, AltoMinimoCajon)

// radioBotonCajon es el `rounded-control` de los dos botones de arriba del cajón.
func radioBotonCajon() float32 { return tema.Forma("radius-control") }

// fondoBotonCajon es su `bg-chrome-hover hover:bg-chrome-active`, que dentro del carril
// son los velos del menú. Opaco y no translúcido: `ui.Caja` pinta el borde como un
// rectángulo lleno por debajo del relleno, así que el velo se compone aquí sobre el
// color del carril.
func fondoBotonCajon(encima bool, v tema.Variante) color.NRGBA {
	velo := "sidebar-hover-bg"
	if encima {
		velo = "sidebar-active-bg"
	}
	return sobre(tema.Color(velo, v), tema.Color("sidebar-bg", v))
}

var _ fyne.Widget = (*BotonPaleta)(nil)

// NuevoBotonPaleta crea el botón. `nombre` es el `aria-label`, que la web compone como
// «Ir a… — Pulsa Ctrl+K desde cualquier sitio»: la tecla se NOMBRA, no se dibuja.
func NuevoBotonPaleta(rotulo, atajo, nombre string, alPulsar func()) *BotonPaleta {
	b := &BotonPaleta{Rotulo: rotulo, Atajo: atajo}
	b.Nombre, b.AlPulsar = nombre, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// MedidaBotonPaleta es rótulo + atajo con su `px-3`, por los 31,75 de alto.
func MedidaBotonPaleta(b *BotonPaleta) fyne.Size {
	ancho := ui.Medir(b.Rotulo, tema.TextoSM, ui.Normal).Width +
		ui.Medir(b.Atajo, tema.TextoXS, ui.Normal).Width +
		2*BotonPaletaPX + tema.Espacio(2)
	return fyne.NewSize(ancho, AltoBotonPaleta)
}

// CreateRenderer construye caja, rótulo y tecla.
func (b *BotonPaleta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonPaletaRenderer{
		b:     b,
		aro:   ui.NuevoAroCon("sidebar-focus", radioBotonCajon(), ui.SinHueco, v).Abierto(),
		caja:  ui.NuevaCaja(tema.Color("sidebar-bg", v), ui.BordeSobre("sidebar-border", "sidebar-bg", v), radioBotonCajon()),
		texto: ui.Texto(b.Rotulo, tema.TextoSM, ui.Normal, tema.Color("sidebar-fg-muted", v)),
		tecla: ui.Texto(b.Atajo, tema.TextoXS, ui.Normal, tema.Color("sidebar-fg-muted", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.texto, r.tecla)
	r.Refresh()
	return r
}

type botonPaletaRenderer struct {
	b            *BotonPaleta
	aro          *ui.Aro
	caja         *ui.Caja
	texto, tecla *canvas.Text
	objetos      []fyne.CanvasObject
}

func (r *botonPaletaRenderer) MinSize() fyne.Size           { return MedidaBotonPaleta(r.b) }
func (r *botonPaletaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonPaletaRenderer) Destroy()                     {}

func (r *botonPaletaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	anchoTecla := ui.Medir(r.b.Atajo, tema.TextoXS, ui.Normal).Width
	ui.ColocarTexto(r.tecla, tema.TextoXS,
		t.Width-BotonPaletaPX-anchoTecla, (t.Height-tema.TextoXS.Linea)/2)
	cabe := max(t.Width-2*BotonPaletaPX-anchoTecla-tema.Espacio(2), 0)
	ui.Truncar(r.texto, r.b.Rotulo, tema.TextoSM, ui.Normal, cabe)
	ui.ColocarTexto(r.texto, tema.TextoSM, BotonPaletaPX, (t.Height-tema.TextoSM.Linea)/2)
}

// Refresh pinta el `bg-chrome-hover hover:bg-chrome-active hover:text-sidebar-fg`.
func (r *botonPaletaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta := tema.Color("sidebar-fg-muted", v)
	if r.b.Encima() {
		tinta = tema.Color("sidebar-fg", v)
	}
	r.aro.CambiarRadio(radioBotonCajon()) // la forma se cambia en caliente
	r.aro.Ver(r.b.Enfocado())
	r.aro.Pintar(v)
	r.caja.CambiarRadio(radioBotonCajon())
	r.caja.Pintar(fondoBotonCajon(r.b.Encima(), v), ui.BordeSobre("sidebar-border", "sidebar-bg", v))
	r.texto.Color = tinta
	r.texto.Refresh()
	// La tecla NO sigue al rótulo: en la hoja se queda en `text-sidebar-fg-muted`
	// también al pasar el ratón.
	r.tecla.Color = tema.Color("sidebar-fg-muted", v)
	r.tecla.Refresh()
}

// contar es el número de una insignia. La web mete el número tal cual (`{tareas}` en
// JSX, que es `Number.prototype.toString`), sin separador de millar: un contador de
// tareas no llega a cuatro cifras y agruparlo aquí y no allí sería una diferencia
// visible sin ninguna ganancia.
func contar(n int) string { return strconv.Itoa(n) }

// sobre compone un color translúcido sobre uno opaco, en sRGB como el navegador.
func sobre(encima, debajo color.NRGBA) color.NRGBA { return tokens.SobreSRGB(encima, debajo) }

// BotonIA es el «Sytue AI» de arriba del menú. NO es `ui.Boton`: en la web es un
// `<button>` escrito a mano, perfilado sobre la tinta del carril (`border-chrome-border
// bg-chrome-hover text-sidebar-fg hover:bg-chrome-active`, `rounded-control`), `h-8`, `text-sm
// font-semibold`, con el icono de 16 y `gap-1.5`, centrado. Un botón secundario de la
// suite sale BLANCO macizo, que es justo lo que la web dice que no quiere («un bloque
// blanco en mitad del menú pesaría más que la acción principal de la página»).
type BotonIA struct {
	ui.Pulsable
	Rotulo string
}

// HuecoIconoIA es el `gap-1.5` entre el icono y el rótulo.
const HuecoIconoIA float32 = 1.5 * 0.25 * tema.Rem

// NuevoBotonIA crea el botón.
func NuevoBotonIA(rotulo string, alPulsar func()) *BotonIA {
	b := &BotonIA{Rotulo: rotulo}
	b.Nombre, b.AlPulsar = rotulo, alPulsar
	b.ExtendBaseWidget(b)
	return b
}

// MinSize es el `h-8` con el icono y el rótulo; el ancho lo pone el carril (`w-full`).
func (b *BotonIA) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	return fyne.NewSize(16+HuecoIconoIA+ui.Medir(b.Rotulo, tema.TextoSM, ui.SemiNegrita).Width+2*BotonPaletaPX, AltoBotonIA)
}

// CreateRenderer construye el perfil, el icono y el rótulo.
func (b *BotonIA) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonIARenderer{
		b:     b,
		aro:   ui.NuevoAroCon("sidebar-focus", radioBotonCajon(), ui.SinHueco, v).Abierto(),
		caja:  ui.NuevaCaja(tema.Color("sidebar-bg", v), ui.BordeSobre("sidebar-border", "sidebar-bg", v), radioBotonCajon()),
		icono: iconos.Trazo(iconos.IA, 16, "sidebar-fg", v),
		texto: ui.Texto(b.Rotulo, tema.TextoSM, ui.SemiNegrita, tema.Color("sidebar-fg", v)),
	}
	r.Refresh()
	return r
}

type botonIARenderer struct {
	b     *BotonIA
	aro   *ui.Aro
	caja  *ui.Caja
	icono *iconos.Dibujo
	texto *canvas.Text
}

func (r *botonIARenderer) MinSize() fyne.Size { return r.b.MinSize() }
func (r *botonIARenderer) Objects() []fyne.CanvasObject {
	return append(append(r.aro.Objetos(), r.caja.Objetos()...), r.icono.Objeto(), r.texto)
}
func (r *botonIARenderer) Destroy() {}

func (r *botonIARenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	ancho := ui.Medir(r.b.Rotulo, tema.TextoSM, ui.SemiNegrita).Width
	x := (t.Width - 16 - HuecoIconoIA - ancho) / 2
	r.icono.Colocar(x, (t.Height-16)/2, 16)
	ui.ColocarTexto(r.texto, tema.TextoSM, x+16+HuecoIconoIA, (t.Height-tema.TextoSM.Linea)/2)
}

func (r *botonIARenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.CambiarRadio(radioBotonCajon())
	r.caja.Pintar(fondoBotonCajon(r.b.Encima(), v), ui.BordeSobre("sidebar-border", "sidebar-bg", v))
	r.aro.CambiarRadio(radioBotonCajon())
	r.aro.Ver(r.b.Enfocado())
	r.aro.Pintar(v)
}
