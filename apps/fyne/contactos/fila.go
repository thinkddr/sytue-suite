// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La fila de un contacto NO es `ui.FilaLista`. Lo parecía, y hasta el 2026-09-22 lo era:
// la web pintaba esta lista con `ListItem`, pero al virtualizarla (`VirtualList`) el
// `renderItem` de ContactsApp.tsx pasó a escribir su propio marcado, y hoy difieren en
// tres cosas que se ven:
//
//   - La casilla marca la SELECCIÓN MÚLTIPLE (`selection.has(c.href)`), no la fila
//     abierta. `FilaLista` pinta la casilla con `Seleccionada`, así que abrir un
//     contacto lo marcaba para el lote y marcarlo no se veía.
//   - Entre el avatar y el nombre hay `gap-2.5` (8,75), no el `gap-3` (10,5) de
//     `ListItem`: el avatar y el texto van dentro del `<button>`, que lleva el suyo.
//   - Al final va la estrella de 14 en `warning-text` si el contacto está destacado.
//     Con red es un BOTÓN (`FavoriteToggle row`, hermano del de abrir, `estrellaFila`):
//     destaca sin abrir la ficha, se llega a él con el tabulador y se anuncia con su
//     nombre, y sin destacar se ve en trazo al pasar el puntero por la fila, con el
//     foco, o siempre en un teléfono, donde no hay «pasar por encima».
//
// La paridad manda sobre la reutilización (`docs/CONVENTIONS.md` §13.6): la misma
// decisión que tomó Mail con su fila.
//
//	div     `flex items-center gap-3 border-l-2 px-3 py-2`             hoja
//	casilla `h-4 w-4` en un `<label>` de `-m-2.5 p-2.5` (toque de 31,5) hoja
//	botón   `flex flex-1 items-center gap-2.5`: avatar sm, textos, ★   hoja
const (
	// HuecoBotonFila es el `gap-2.5` de dentro del botón.
	HuecoBotonFila = 2.5 * 0.25 * tema.Rem
	// LadoBotonEstrellaFila es el `IconButton size="sm"` de la estrella: `h-6 w-6`, que
	// con el dedo sube al suelo táctil (`ui.SueloTactil`).
	LadoBotonEstrellaFila = 6 * 0.25 * tema.Rem
	// ToqueCasilla es el `p-2.5` del `<label>`: la zona de toque se estira eso por
	// cada lado sin mover nada, porque el `-m-2.5` lo devuelve.
	ToqueCasilla = 2.5 * 0.25 * tema.Rem
)

// FilaContacto es una fila de la lista de contactos. Se RECICLA dentro de un
// `widget.List`: `Poner` cambia sus datos y la repinta.
type FilaContacto struct {
	widget.BaseWidget

	Nombre    string
	Subtitulo string
	// Destacado pinta la estrella del final.
	Destacado bool
	// Foto es la del avatar; nil, las iniciales.
	Foto image.Image
	// Abierta es la fila cuya ficha se está viendo (`aria-current`).
	Abierta bool
	// Marcada es la casilla de la selección múltiple.
	Marcada bool
	// RotuloCasilla es el `aria-label` de la casilla («Seleccionar a …»).
	RotuloCasilla string
	// RotuloEstrella es el de la estrella («Destacar a …» o «Quitar a … de destacados»).
	RotuloEstrella string

	AlAbrir  func()
	AlMarcar func()
	// AlDestacar destaca o quita el destacado desde la fila; nil (sin red) deja solo la
	// estrella de quien ya lo está, sin botón.
	AlDestacar func()

	encima bool
}

var (
	_ fyne.Widget            = (*FilaContacto)(nil)
	_ fyne.Tappable          = (*FilaContacto)(nil)
	_ fyne.SecondaryTappable = (*FilaContacto)(nil)
	_ desktop.Hoverable      = (*FilaContacto)(nil)
)

// NuevaFilaContacto crea una fila vacía, lista para reciclar.
func NuevaFilaContacto() *FilaContacto {
	f := &FilaContacto{}
	f.ExtendBaseWidget(f)
	return f
}

// MedidaFilaContacto es el alto de la fila: `py-2` más lo más alto de sus hijos. Con
// correo, las dos cajas de línea (19,25 + 15,75) mandan sobre el avatar de 32: 49,
// medido en producción. Sin él manda el avatar: 46.
func MedidaFilaContacto(conSubtitulo bool) float32 {
	texto := tema.TextoBase.Linea
	if conSubtitulo {
		texto += tema.TextoSM.Linea
	}
	return 2*ui.RellenoYFila() + max(texto, ui.MedidaAvatar(ui.AvatarSM).Height, ui.LadoEleccion)
}

// xCasilla y xBoton son donde empiezan la casilla y el botón: tras el `border-l-2`
// y el `px-3`, y tras la casilla y el `gap-3`.
func xCasilla() float32 { return ui.InicioFila() }
func xBoton() float32   { return xCasilla() + ui.LadoEleccion + ui.HuecoFila }

// zonaEstrella es el ancho del botón de la estrella al final de la fila; 0 si no hay.
func (f *FilaContacto) zonaEstrella() float32 {
	if f.AlDestacar == nil {
		return 0
	}
	return ui.SueloTactil(LadoBotonEstrellaFila)
}

// Tapped abre la ficha, salvo que el toque caiga en la zona de la casilla o en la de
// la estrella (el botón recibe su propio toque; esto cubre lo que llegue a la fila).
//
// La casilla dibuja 14 px, pero su `<label>` recoge el toque 8,75 px más allá por cada
// lado: a 14 px, en un teléfono el dedo falla la casilla y lo que pasa entonces es que
// se abre la ficha, que no era lo que se pedía (el comentario de CHECKBOX_HIT en la
// web). Aquí la `Eleccion` recibe su propio clic dentro de sus 14 px y lo que cae en el
// margen llega a la fila, que lo reenvía.
func (f *FilaContacto) Tapped(e *fyne.PointEvent) {
	if x := e.Position.X; x >= xCasilla()-ToqueCasilla && x <= xCasilla()+ui.LadoEleccion+ToqueCasilla {
		if f.AlMarcar != nil {
			f.AlMarcar()
		}
		return
	}
	if z := f.zonaEstrella(); z > 0 && e.Position.X >= f.Size().Width-ui.MargenFila-z {
		f.AlDestacar()
		return
	}
	if f.AlAbrir != nil {
		f.AlAbrir()
	}
}

// Mantener pulsado selecciona sin abrir la ficha: la misma acción que su casilla.
func (f *FilaContacto) TappedSecondary(*fyne.PointEvent) {
	if f.AlMarcar != nil {
		f.AlMarcar()
	}
}

// MouseIn es el `hover:bg-bg-subtle`, que solo existe si la fila no está abierta.
func (f *FilaContacto) MouseIn(*desktop.MouseEvent) { f.encima = true; f.Refresh() }

// MouseMoved no hace nada: el estado lo fijan la entrada y la salida.
func (f *FilaContacto) MouseMoved(*desktop.MouseEvent) {}

// MouseOut devuelve la fila a su color de reposo.
func (f *FilaContacto) MouseOut() { f.encima = false; f.Refresh() }

// AccessibilityLabel es el nombre y el correo, que es lo que lee el `<button>`.
func (f *FilaContacto) AccessibilityLabel() string {
	if f.Subtitulo == "" {
		return f.Nombre
	}
	return f.Nombre + " — " + f.Subtitulo
}

// AccessibilityRole: la fila es un botón que abre la ficha.
func (f *FilaContacto) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// CreateRenderer compone la fila con primitivas y la casilla del catálogo.
func (f *FilaContacto) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaContactoRenderer{
		f:         f,
		base:      canvas.NewRectangle(tema.Color("surface", v)),
		velo:      canvas.NewRectangle(color.NRGBA{}),
		galon:     canvas.NewRectangle(color.NRGBA{}),
		avatar:    ui.NuevoAvatar(f.Nombre).Con(ui.AvatarSM),
		nombre:    ui.Texto(f.Nombre, tema.TextoBase, ui.Medio, tema.Color("text", v)),
		subtitulo: ui.Texto(f.Subtitulo, tema.TextoSM, ui.Normal, tema.Color("text-muted", v)),
		estrella:  GlifoRelleno(EstrellaD, LadoEstrellaFila, tema.Color("warning-text", v)),
		boton:     nuevaEstrellaFila(),
	}
	r.boton.AlPulsar = func() {
		if f.AlDestacar != nil {
			f.AlDestacar()
		}
	}
	// Fyne da el hover solo al Hoverable más hondo: al llegar a la estrella la fila
	// recibe MouseOut. En la web el `hover:` es del div de la fila y sigue encima de
	// la estrella, así que la fila se repinta cuando el puntero entra o sale de ella.
	r.boton.AlCambiarEstado(f.Refresh)
	r.casilla = ui.NuevaCasilla(f.RotuloCasilla, func(bool) {
		if f.AlMarcar != nil {
			f.AlMarcar()
		}
	})
	r.casilla.SinEtiqueta = true
	r.objetos = []fyne.CanvasObject{
		r.base, r.velo, r.galon, r.casilla, r.avatar, r.nombre, r.subtitulo, r.estrella.Objeto(), r.boton,
	}
	r.Refresh()
	return r
}

type filaContactoRenderer struct {
	f                 *FilaContacto
	base, velo, galon *canvas.Rectangle
	casilla           *ui.Eleccion
	avatar            *ui.Avatar
	nombre, subtitulo *canvas.Text
	// estrella es la que se ve sin red, sin botón; con red la pinta `boton`.
	estrella *iconos.Dibujo
	boton    *estrellaFila
	objetos  []fyne.CanvasObject
}

func (r *filaContactoRenderer) MinSize() fyne.Size {
	return fyne.NewSize(xBoton(), MedidaFilaContacto(r.f.Subtitulo != ""))
}

func (r *filaContactoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaContactoRenderer) Destroy()                     {}

func (r *filaContactoRenderer) Layout(t fyne.Size) {
	f := r.f
	r.base.Resize(t)
	ui.ColocarVeloFila(r.velo, r.galon, t.Width, t.Height)
	centro := func(alto float32) float32 { return (t.Height - alto) / 2 }

	m := ui.MedidaEleccion("")
	r.casilla.Move(fyne.NewPos(xCasilla(), centro(m.Height)))
	r.casilla.Resize(fyne.NewSize(ui.LadoEleccion, m.Height))

	x := xBoton()
	av := ui.MedidaAvatar(ui.AvatarSM)
	r.avatar.Move(fyne.NewPos(x, centro(av.Height)))
	r.avatar.Resize(av)
	x += av.Width + HuecoBotonFila

	derecha := t.Width - ui.MargenFila
	if z := f.zonaEstrella(); z > 0 {
		// El botón ocupa su sitio esté o no a la vista (`opacity-0`): el nombre no
		// salta de ancho al pasar el puntero. Va tras el `gap-3` de la fila.
		r.boton.Move(fyne.NewPos(derecha-z, centro(z)))
		r.boton.Resize(fyne.NewSize(z, z))
		derecha -= z + ui.HuecoFila
	} else if f.Destacado {
		r.estrella.Colocar(derecha-LadoEstrellaFila, centro(LadoEstrellaFila), LadoEstrellaFila)
		derecha -= LadoEstrellaFila + HuecoBotonFila
	}
	cabe := max(derecha-x, 0)
	alto := tema.TextoBase.Linea
	if f.Subtitulo != "" {
		alto += tema.TextoSM.Linea
	}
	y := centro(alto)
	ui.Truncar(r.nombre, f.Nombre, tema.TextoBase, ui.Medio, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoBase, x, y)
	if f.Subtitulo != "" {
		ui.Truncar(r.subtitulo, f.Subtitulo, tema.TextoSM, ui.Normal, cabe)
		ui.ColocarTexto(r.subtitulo, tema.TextoSM, x, y+tema.TextoBase.Linea)
	}
}

func (r *filaContactoRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.base.FillColor = tema.Color("surface", v)
	// El orden de la web: abierta gana, y el hover solo existe en la otra rama.
	switch {
	case f.Abierta:
		r.velo.FillColor = tema.Color("selected", v)
		r.galon.FillColor = tema.Color("primary", v)
	case f.encima || (f.AlDestacar != nil && r.boton.Encima()):
		r.velo.FillColor = tema.Color("bg-subtle", v)
		r.galon.FillColor = color.NRGBA{}
	default:
		r.velo.FillColor = color.NRGBA{}
		r.galon.FillColor = color.NRGBA{}
	}
	r.casilla.Etiqueta, r.casilla.Marcada = f.RotuloCasilla, f.Marcada
	r.avatar.Nombre, r.avatar.Foto = f.Nombre, f.Foto
	r.nombre.Color = tema.Color("text", v)
	r.subtitulo.Color = tema.Color("text-muted", v)
	r.subtitulo.Hidden = f.Subtitulo == ""
	r.estrella.TenirCon(tema.Color("warning-text", v))
	r.estrella.Ver(f.AlDestacar == nil && f.Destacado)
	// Oculto sin red: así tampoco es parada del tabulador.
	r.boton.Hidden = f.AlDestacar == nil
	r.boton.Nombre, r.boton.Activo, r.boton.FilaEncima = f.RotuloEstrella, f.Destacado, f.encima
	for _, o := range r.objetos {
		o.Refresh()
	}
	// Reciclada en un `widget.List` el tamaño no cambia y `Layout` no vuelve a correr
	// solo: el truncado y la estrella se quedarían los de la fila anterior.
	r.Layout(f.Size())
}
