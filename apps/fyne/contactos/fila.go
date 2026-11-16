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

	// EsCursor es la fila del cursor del teclado: la ÚNICA de la lista que está en el
	// recorrido del tabulador (el `tabIndex={parada}` itinerante de la web). Las demás
	// dicen `Disabled()` sin estar inertes, como `Boton.EnElRecorrido`.
	EsCursor bool
	// AlMover recibe ↑ ↓ Inicio Fin con la fila enfocada; la lista decide adónde.
	AlMover func(fyne.KeyName)

	// Cabecera, si no está vacía, convierte la fila en la cabecera A–Z de esa inicial
	// (CONT-05): la versal de la web (`sytue-overline text-text-subtle`), sin casilla,
	// avatar ni estrella, fuera del tabulador y sorda al toque. Es la MISMA fila y no
	// otro widget porque `widget.List` recicla un solo tipo de plantilla.
	Cabecera string

	encima, enfocada bool
}

var (
	_ fyne.Widget            = (*FilaContacto)(nil)
	_ fyne.Tappable          = (*FilaContacto)(nil)
	_ fyne.SecondaryTappable = (*FilaContacto)(nil)
	_ desktop.Hoverable      = (*FilaContacto)(nil)
	_ fyne.Focusable         = (*FilaContacto)(nil)
	_ fyne.Disableable       = (*FilaContacto)(nil)
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

// AltoCabeceraLista es la cabecera A–Z de la web: `pt-3 pb-1` alrededor de la caja de
// línea de `sytue-overline` (`text-xs`, 14). Son 28.
var AltoCabeceraLista = RellenoArribaCabecera + tema.TextoXS.Linea + RellenoAbajoCabecera

// RellenoArribaCabecera y RellenoAbajoCabecera son el `pt-3` y el `pb-1`: más aire
// encima que debajo, para que la letra se lea como el título de lo que sigue y no
// como el pie de lo anterior.
const (
	RellenoArribaCabecera = 3 * 0.25 * tema.Rem
	RellenoAbajoCabecera  = 1 * 0.25 * tema.Rem
)

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
	if f.Cabecera != "" {
		return
	}
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
	if f.Cabecera == "" && f.AlMarcar != nil {
		f.AlMarcar()
	}
}

// MouseIn es el `hover:bg-bg-subtle`, que solo existe si la fila no está abierta.
func (f *FilaContacto) MouseIn(*desktop.MouseEvent) { f.encima = true; f.Refresh() }

// MouseMoved no hace nada: el estado lo fijan la entrada y la salida.
func (f *FilaContacto) MouseMoved(*desktop.MouseEvent) {}

// MouseOut devuelve la fila a su color de reposo.
func (f *FilaContacto) MouseOut() { f.encima = false; f.Refresh() }

// ---- Teclado ----
//
// Hasta CONT-04 la fila no era `Focusable`: con el teclado no se abría ningún contacto.
// Ahora es el `<button data-abrir>` de la web: Intro y Espacio abren la ficha, las
// flechas mueven el cursor por la lista y las demás letras SUBEN a la ventana, donde
// `App.Atajo` las atiende (j, k, e, s, x, /, ?), igual que si no hubiera nada enfocado.

// Disabled saca del tabulador a las filas que no son la del cursor. Siguen pulsándose y
// la lista las enfoca con las flechas tras moverles el cursor.
// Una cabecera nunca está en él.
func (f *FilaContacto) Disabled() bool { return !f.EsCursor || f.Cabecera != "" }

// Enable y Disable completan `fyne.Disableable`: el recorrido lo decide el cursor.
func (f *FilaContacto) Enable()  {}
func (f *FilaContacto) Disable() {}

// FocusGained y FocusLost encienden el aro de dentro del botón (`sytue-focus-ring-inner`).
func (f *FilaContacto) FocusGained() { f.enfocada = true; f.Refresh() }
func (f *FilaContacto) FocusLost()   { f.enfocada = false; f.Refresh() }

// Enfocada dice si la fila tiene el foco del teclado.
func (f *FilaContacto) Enfocada() bool { return f.enfocada }

// TypedRune abre con Espacio, como un `<button>`, y deja subir lo demás a la ventana:
// con una fila enfocada siguen valiendo los atajos de la app.
func (f *FilaContacto) TypedRune(r rune) {
	if r == ' ' {
		if f.AlAbrir != nil {
			f.AlAbrir()
		}
		return
	}
	ui.SubirRuna(fyne.CurrentApp().Driver().CanvasForObject(f), r)
}

// TypedKey abre con Intro y lleva ↑ ↓ Inicio Fin a la lista. Escape y «atrás» son de
// la capa de encima, si la hay.
func (f *FilaContacto) TypedKey(k *fyne.KeyEvent) {
	switch k.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		if f.AlAbrir != nil {
			f.AlAbrir()
		}
	case fyne.KeyUp, fyne.KeyDown, fyne.KeyHome, fyne.KeyEnd:
		if f.AlMover != nil {
			f.AlMover(k.Name)
		}
	default:
		ui.SubirEscape(f, k)
	}
}

// AccessibilityLabel es el nombre y el correo, que es lo que lee el `<button>`.
func (f *FilaContacto) AccessibilityLabel() string {
	if f.Cabecera != "" {
		return f.Cabecera
	}
	if f.Subtitulo == "" {
		return f.Nombre
	}
	return f.Nombre + " — " + f.Subtitulo
}

// AccessibilityRole: la fila es un botón que abre la ficha.
// La cabecera es texto: el `role="presentation"` de la web, que no cuenta como elemento.
func (f *FilaContacto) AccessibilityRole() fyne.AccessibleRole {
	if f.Cabecera != "" {
		return fyne.AccessibleRoleText
	}
	return fyne.AccessibleRoleButton
}

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
		cabecera:  ui.Texto(f.Cabecera, tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v)),
		estrella:  GlifoRelleno(EstrellaD, LadoEstrellaFila, tema.Color("warning-text", v)),
		boton:     nuevaEstrellaFila(),
		// `sytue-focus-ring-inner`: el aro va DENTRO del botón, porque el `overflow` del
		// panel recorta el que se pinta fuera (el comentario de la web). Abierto: el
		// botón no tiene caja opaca que tape el centro.
		aro: ui.NuevoAro(max(tema.RadioMD-tema.AnilloFoco, 0), ui.SinHueco, v).Abierto(),
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
	r.objetos = append([]fyne.CanvasObject{
		r.base, r.velo, r.galon, r.casilla, r.avatar, r.nombre, r.subtitulo, r.estrella.Objeto(), r.boton,
		r.cabecera,
	}, r.aro.Objetos()...)
	r.Refresh()
	return r
}

type filaContactoRenderer struct {
	f                 *FilaContacto
	base, velo, galon *canvas.Rectangle
	casilla           *ui.Eleccion
	avatar            *ui.Avatar
	nombre, subtitulo *canvas.Text
	// cabecera es la letra del modo cabecera; oculta en una fila de contacto.
	cabecera *canvas.Text
	// estrella es la que se ve sin red, sin botón; con red la pinta `boton`.
	estrella *iconos.Dibujo
	boton    *estrellaFila
	aro      *ui.Aro
	objetos  []fyne.CanvasObject
}

func (r *filaContactoRenderer) MinSize() fyne.Size {
	if r.f.Cabecera != "" {
		return fyne.NewSize(xCasilla(), AltoCabeceraLista)
	}
	return fyne.NewSize(xBoton(), MedidaFilaContacto(r.f.Subtitulo != ""))
}

func (r *filaContactoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaContactoRenderer) Destroy()                     {}

func (r *filaContactoRenderer) Layout(t fyne.Size) {
	f := r.f
	r.base.Resize(t)
	ui.ColocarVeloFila(r.velo, r.galon, t.Width, t.Height)
	if f.Cabecera != "" {
		// Alineada con la casilla: el `border-l-2 px-3` de las filas de contacto.
		ui.ColocarTexto(r.cabecera, tema.TextoXS, xCasilla(), RellenoArribaCabecera)
		r.aro.Ver(false)
		return
	}
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
	// La caja del `<button>`: del avatar al final del texto, con el alto de lo más alto
	// de dentro (`items-center`). El aro se pinta por fuera de la caja que recibe, así
	// que se le da la caja encogida en su grosor: queda dentro, como el `inset`.
	altoBoton := max(alto, av.Height)
	ma := r.aro.Margen()
	r.aro.ColocarEn(fyne.NewPos(xBoton()+ma, centro(altoBoton)+ma),
		fyne.NewSize(max(derecha-xBoton()-2*ma, 0), max(altoBoton-2*ma, 0)))
	ui.Truncar(r.nombre, f.Nombre, tema.TextoBase, ui.Medio, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoBase, x, y)
	if f.Subtitulo != "" {
		ui.Truncar(r.subtitulo, f.Subtitulo, tema.TextoSM, ui.Normal, cabe)
		ui.ColocarTexto(r.subtitulo, tema.TextoSM, x, y+tema.TextoBase.Linea)
	}
	// Tras colocarlo: el aro abierto no enciende franjas que aún miden cero.
	r.aro.Ver(f.enfocada)
}

func (r *filaContactoRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.base.FillColor = tema.Color("surface", v)
	// El orden de la web: abierta gana, y el hover solo existe en la otra rama. La
	// cabecera no se abre ni se ilumina.
	switch {
	case f.Cabecera != "":
		r.velo.FillColor = color.NRGBA{}
		r.galon.FillColor = color.NRGBA{}
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
	// La casilla y la estrella solo son paradas del tabulador en la fila del cursor,
	// como el `tabIndex={parada}` de la web: si no, cada fila pintada sumaba dos.
	r.casilla.EnElRecorrido(f.EsCursor)
	r.boton.fueraDelRecorrido = !f.EsCursor
	r.avatar.Nombre, r.avatar.Foto = f.Nombre, f.Foto
	r.nombre.Color = tema.Color("text", v)
	r.subtitulo.Color = tema.Color("text-muted", v)
	r.subtitulo.Hidden = f.Subtitulo == ""
	r.estrella.TenirCon(tema.Color("warning-text", v))
	r.estrella.Ver(f.AlDestacar == nil && f.Destacado)
	// Oculto sin red: así tampoco es parada del tabulador.
	r.boton.Hidden = f.AlDestacar == nil
	r.boton.Nombre, r.boton.Activo, r.boton.FilaEncima = f.RotuloEstrella, f.Destacado, f.encima
	r.aro.Pintar(v)
	// `sytue-overline` pone la versalita por CSS; aquí, a mano.
	r.cabecera.Text = ui.Versalitas(f.Cabecera)
	r.cabecera.Color = tema.Color("text-subtle", v)
	cabecera := f.Cabecera != ""
	r.cabecera.Hidden = !cabecera
	if cabecera {
		for _, o := range []fyne.CanvasObject{r.casilla, r.avatar, r.nombre, r.subtitulo, r.estrella.Objeto(), r.boton} {
			o.Hide()
		}
	} else {
		r.casilla.Show()
		r.avatar.Show()
		r.nombre.Show()
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
	// Reciclada en un `widget.List` el tamaño no cambia y `Layout` no vuelve a correr
	// solo: el truncado y la estrella se quedarían los de la fila anterior.
	r.Layout(f.Size())
}
