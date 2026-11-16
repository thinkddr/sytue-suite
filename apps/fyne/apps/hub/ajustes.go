// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El marco de las pantallas de AJUSTES del portal —«Tus datos» y «Conectar tus
// dispositivos»—: `SettingsShell` y `SettingsSection` de `@sytue/ui`.
//
// Medido en producción en «Tus datos» (`medidas/portal/datos.*.json`): a 1280 la caja
// empieza en x = 213 y mide 854 (`max-w-5xl` de 896 menos el `sm:px-6`); a 390, en
// x = 14 y mide 362 (`px-4`). El título (24,5/31,5, 700) cae 21 por debajo de la
// topbar, la entrada 5,25 por debajo del título y la primera sección 21 más abajo.
// Cada sección es una `Card` con `shadow-sm`: 1 de borde, 14 de relleno y el `h2` de
// 15,75/22,75 arriba, con `gap-4` (14) hasta lo de dentro.
//
// El marco y las secciones son `ui.MarcoAjustes` y `ui.SeccionAjustes` (§2.8: la
// configuración guiada no se reimplementa por app). Los nombres de aquí se quedan para
// las cabeceras de «Dispositivos», que colocan a mano el progreso del asistente.
const (
	// AnchoAjustes es el `max-w-5xl`.
	AnchoAjustes = ui.AnchoMarcoAjustes
	// MargenAjustesY es el `py-6`, y también el `mt-6` y el `gap-6` entre secciones.
	MargenAjustesY = ui.MargenMarcoAjustesY
	// MargenAjustesX es el `px-4` de la caja; desde `sm:` pasa a `px-6` (MargenX).
	MargenAjustesX = ui.MargenMarcoAjustesX
	// RellenoSeccion es el `p-4` del `CardBody`, y HuecoSeccion su `gap-4`.
	RellenoSeccion = ui.RellenoSeccionAjustes
	HuecoSeccion   = ui.HuecoSeccionAjustes
)

// Los interlineados de estas pantallas que no son los de la escala: `leading-relaxed`
// (1,625) sobre `text-base`, `text-sm` y `text-xs`.
var (
	baseRelajada = tema.Escalon{Tam: tema.TextoBase.Tam, Linea: 1.625 * tema.TextoBase.Tam}
	smRelajada   = tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}
	xsRelajada   = tema.Escalon{Tam: tema.TextoXS.Tam, Linea: 1.625 * tema.TextoXS.Tam}
)

// CajaAjustes es dónde empieza y cuánto mide la caja de un `SettingsShell` para un
// ancho de ventana. El `sm:` mide la VENTANA con el rem de 16, como todos los cortes.
func CajaAjustes(ventana float32) (x, ancho float32) { return ui.CajaAjustes(ventana) }

// InteriorSeccion es lo que le queda a lo de dentro de una sección de ese ancho.
func InteriorSeccion(ancho float32) float32 { return ui.InteriorSeccionAjustes(ancho) }

// ponerParrafo pone un párrafo a ese ancho y devuelve dónde acaba; con `mover` a
// false solo mide.
func ponerParrafo(p *ui.Parrafo, x, y, ancho float32, mover bool) float32 {
	return ponerObjeto(p, x, y, fyne.NewSize(ancho, ui.AltoParrafo(p, ancho)), mover)
}

// ponerObjeto coloca un objeto con ese tamaño y devuelve dónde acaba.
func ponerObjeto(o fyne.CanvasObject, x, y float32, t fyne.Size, mover bool) float32 {
	if mover {
		o.Move(fyne.NewPos(x, y))
		o.Resize(t)
	}
	return y + t.Height
}

// ---- La acción principal de estas pantallas ----

// Medidas del enlace con cara de botón principal (`bg-primary min-h-11 px-5 py-2.5
// rounded-md font-semibold`): 38,5 de alto y 17,5 a cada lado, medido en producción
// («Descargar copia», 119 × 38,5).
const (
	AltoAccion   float32 = 11 * 0.25 * tema.Rem
	MargenAccion float32 = 5 * 0.25 * tema.Rem
)

// Accion es ese enlace. No es `ui.Boton` porque la web tampoco usa `Button`: son clases
// sueltas sobre un `<a>`, con el alto de `lg` (`min-h-11 px-5`) y la letra de `sm` en
// «Tus datos» —o `md` en el perfil de Apple de «Dispositivos»—, que ninguna talla de
// `Button` combina.
type Accion struct {
	ui.Pulsable
	Texto   string
	Escalon tema.Escalon
}

var _ fyne.Widget = (*Accion)(nil)

// NuevaAccion crea la acción con la letra de ese escalón.
func NuevaAccion(texto string, e tema.Escalon, alPulsar func()) *Accion {
	a := &Accion{Texto: texto, Escalon: e}
	a.Nombre, a.AlPulsar = texto, alPulsar
	a.ExtendBaseWidget(a)
	return a
}

// MedidaAccion es el rótulo con su relleno, por los 38,5 de alto.
func MedidaAccion(a *Accion) fyne.Size {
	return fyne.NewSize(ui.Medir(a.Texto, a.Escalon, ui.SemiNegrita).Width+2*MargenAccion, AltoAccion)
}

// CreateRenderer compone el anillo, la caja y el rótulo.
func (a *Accion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &accionRenderer{
		a:     a,
		aro:   ui.NuevoAro(tema.RadioMD, ui.ConHueco, v), // `sytue-focus-ring-inset`
		caja:  ui.NuevaCaja(tema.Color("primary", v), color.NRGBA{}, tema.RadioMD),
		texto: ui.Texto(a.Texto, a.Escalon, ui.SemiNegrita, tema.Color("on-primary", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.texto)
	r.Refresh()
	return r
}

type accionRenderer struct {
	a       *Accion
	aro     *ui.Aro
	caja    *ui.Caja
	texto   *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *accionRenderer) MinSize() fyne.Size           { return MedidaAccion(r.a) }
func (r *accionRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *accionRenderer) Destroy()                     {}

func (r *accionRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	w := ui.Medir(r.a.Texto, r.a.Escalon, ui.SemiNegrita).Width
	ui.ColocarTexto(r.texto, r.a.Escalon, (t.Width-w)/2, (t.Height-r.a.Escalon.Linea)/2)
}

// Refresh: `hover:bg-[var(--sytue-primary-hover)]` y el anillo al enfocar.
func (r *accionRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := "primary"
	if r.a.Encima() {
		fondo = "primary-hover"
	}
	r.caja.Pintar(tema.Color(fondo, v), color.NRGBA{})
	r.texto.Text, r.texto.Color = r.a.Texto, tema.Color("on-primary", v)
	r.texto.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.a.Enfocado())
	r.Layout(r.a.Size())
}
