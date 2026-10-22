// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Las filas de la suite son DOS componentes distintos de la web y aquí son dos tipos
// distintos. Confundirlos cuesta 2,5 px por fila y una tesela:
//
//   - FilaLista es `ListItem` de packages/ui/src/components/List.tsx —mail, chat,
//     contactos—: `border-l-2 px-3 py-2`, título y subtítulo apilados, meta a la
//     derecha, y el galón izquierdo teñido cuando está seleccionada.
//   - FilaDrive es la fila del explorador de Sytue Drive (apps/drive FileBrowser.tsx),
//     que NO usa ListItem: `border-b px-3 py-2` con una tesela de tipo de 36 y
//     columnas de tamaño y fecha que aparecen por corte de VENTANA.
//
// Las dos pintan un fondo OPACO debajo de todo. No es decoración: dentro de un
// `widget.List` cada fila va sobre un `listItem` de Fyne que pinta su propio fondo de
// selección y de hover con el tema de serie, y sin un opaco encima se transparenta.

// Alturas de la fila de lista, en px de CSS con el rem de la suite.
const (
	// BordeFila es `border-l-2`: 2 px que salen del hueco de contenido, no del
	// relleno. `px-3` empieza DESPUÉS de él.
	BordeFila float32 = 2
	// MargenFila es `px-3` y HuecoFila es `gap-3`.
	MargenFila float32 = 3 * 0.25 * tema.Rem
	HuecoFila  float32 = 3 * 0.25 * tema.Rem
	// AltoFila es `py-2` a cada lado.
	AltoFila float32 = 2 * 0.25 * tema.Rem
)

// La fila de List.tsx toma tres medidas y un radio de la FORMA (`--sytue-row-*` y
// `--radius-row`, en theme.css). En Carril son las de siempre: una barra de 2 a la
// izquierda, sin sangría y `py-2`. En Marco la fila es un píxel más alta por arriba y por
// abajo y, elegida o bajo el cursor, un BLOQUE redondeado que se separa 6 de los bordes,
// sin barra. El texto apenas se mueve: lo que entra de sangría sale de relleno
// (`px-[calc(0.75rem − sangría)]`).
//
// Las comparten las tres filas que en la web son `ListItem`: esta, la de Sytue Contacts
// y la de la bandeja de Sytue Mail.

// BarraFila es `border-l-[length:var(--sytue-row-bar)]`.
func BarraFila() float32 { return tema.Forma("row-bar") }

// RellenoYFila es `py-[calc(0.5rem + var(--sytue-row-grow))]`.
func RellenoYFila() float32 { return AltoFila + tema.Forma("row-grow") }

// InicioFila es dónde empieza el contenido de la fila: la sangría, la barra y el relleno
// que queda (`mx-[sangría]`, `border-l`, `px-[calc(0.75rem − sangría)]`).
func InicioFila() float32 { return BarraFila() + MargenFila }

// ColocarVeloFila coloca en una fila de ese tamaño el VELO —el fondo de la elegida o de
// la que tiene el cursor— y la barra de la izquierda, con la sangría y el radio de la
// forma. `alto` es el de la fila sin su filete de abajo, si lo lleva.
func ColocarVeloFila(velo, barra *canvas.Rectangle, ancho, alto float32) {
	sangria := tema.Forma("row-inset")
	velo.Move(fyne.NewPos(sangria, 0))
	velo.Resize(fyne.NewSize(max(ancho-2*sangria, 0), alto))
	velo.CornerRadius = tema.Forma("radius-row")
	barra.Move(fyne.NewPos(sangria, 0))
	barra.Resize(fyne.NewSize(BarraFila(), alto))
	barra.Hidden = BarraFila() <= 0
}

// MedidaFilaLista es el alto que ocupa una fila de lista: `py-2` arriba y abajo más
// el más alto de sus hijos, que es lo que hace un `flex items-center`.
//
// La columna de texto mide la caja de línea de `text-base` y, si hay subtítulo,
// la de `text-sm` debajo. Si una pieza delantera es más alta, manda ella.
func MedidaFilaLista(conSubtitulo bool, altoDelantero float32) float32 {
	texto := tema.TextoBase.Linea
	if conSubtitulo {
		texto += tema.TextoSM.Linea
	}
	return 2*RellenoYFila() + max(texto, altoDelantero, MedidaEleccion("").Height)
}

// FilaLista es el `ListItem` de List.tsx.
//
// Va envuelta en un `fyne.Widget` y no en un contenedor porque es SELECCIONABLE: sin
// la cáscara no llega un solo clic (`walkObjectTree` solo desciende en contenedores y
// widgets, y a una primitiva de lienzo no se le puede añadir `Tappable`), no hay
// hover y el lector de pantalla anuncia «Container» en vez del asunto del correo.
type FilaLista struct {
	widget.BaseWidget

	Titulo    string
	Subtitulo string
	Meta      string

	// Seleccionada es `aria-selected`: galón izquierdo teñido y fondo `bg-selected`.
	Seleccionada bool
	// SinLeer es el `unread` de la web: título y subtítulo en negrita y en
	// `primary-text`.
	SinLeer bool
	// ConCasilla enseña la casilla de selección delante de todo.
	ConCasilla bool
	// ConGalon enseña el galón de «entrar aquí» al final de la fila.
	ConGalon bool
	// Delantero es el `leading` de la web: un avatar, una tesela de tipo, un icono.
	// Se coloca a su MinSize y no encoge (`shrink-0`).
	Delantero fyne.CanvasObject
	// Fondo es el token de la superficie sobre la que va la fila. Hace falta para
	// componer el `bg-selected` translúcido y para tapar el fondo del `listItem` de
	// Fyne. Vacío es "surface".
	Fondo string
	// Envuelve es el ancho de la fila cuando título y subtítulo PARTEN en líneas en vez
	// de truncarse: el `min-w-0` que pone ListItem cuando le pasan un nodo y no una
	// cadena (la campana de avisos). Con 0, `truncate`.
	Envuelve float32

	AlPulsar func()
	AlMarcar func()

	encima bool
}

var (
	_ fyne.Widget       = (*FilaLista)(nil)
	_ fyne.Tappable     = (*FilaLista)(nil)
	_ desktop.Hoverable = (*FilaLista)(nil)
)

// NuevaFilaLista crea la fila con título y subtítulo (que admite "").
func NuevaFilaLista(titulo, subtitulo string) *FilaLista {
	f := &FilaLista{Titulo: titulo, Subtitulo: subtitulo}
	f.ExtendBaseWidget(f)
	return f
}

func (f *FilaLista) fondo() string {
	if f.Fondo == "" {
		return "surface"
	}
	return f.Fondo
}

// altoDelantero es lo que ocupa el `leading`, o 0 si no hay.
func (f *FilaLista) altoDelantero() float32 {
	if f.Delantero == nil {
		return 0
	}
	return f.Delantero.MinSize().Height
}

// CreateRenderer compone la fila con primitivas.
func (f *FilaLista) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaListaRenderer{
		f:      f,
		base:   canvas.NewRectangle(tema.Color(f.fondo(), v)),
		velo:   canvas.NewRectangle(color.NRGBA{}),
		galon:  canvas.NewRectangle(color.NRGBA{}),
		titulo: Texto(f.Titulo, tema.TextoBase, Medio, tema.Color("text", v)),
	}
	r.objetos = []fyne.CanvasObject{r.base, r.velo, r.galon}
	if f.ConCasilla {
		r.casilla = casillaDeFila(f.Seleccionada, f.AlMarcar)
		r.objetos = append(r.objetos, r.casilla)
	}
	if f.Delantero != nil {
		r.objetos = append(r.objetos, f.Delantero)
	}
	switch {
	case f.Envuelve > 0:
		r.pTitulo = NuevoParrafo(f.Titulo, tema.TextoBase, Medio, "text", v)
		r.objetos = append(r.objetos, r.pTitulo)
		if f.Subtitulo != "" {
			r.pSubtitulo = NuevoParrafo(f.Subtitulo, tema.TextoSM, Normal, "text-muted", v)
			r.objetos = append(r.objetos, r.pSubtitulo)
		}
	default:
		r.objetos = append(r.objetos, r.titulo)
		if f.Subtitulo != "" {
			r.subtitulo = Texto(f.Subtitulo, tema.TextoSM, Normal, tema.Color("text-muted", v))
			r.objetos = append(r.objetos, r.subtitulo)
		}
	}
	if f.Meta != "" {
		// `.sytue-meta`: `text-xs` y el color fijado a `text-subtle`.
		r.meta = Texto(f.Meta, tema.TextoXS, Normal, tema.Color("text-subtle", v))
		r.objetos = append(r.objetos, r.meta)
	}
	if f.ConGalon {
		r.chevron = iconos.Trazo(iconos.ChevronDerecha, LadoGalon, "text-subtle", v)
		r.objetos = append(r.objetos, r.chevron.Objeto())
	}
	r.Refresh()
	return r
}

// Tapped abre la fila.
//
// La casilla NO se reparte aquí: es una `Eleccion` de verdad dentro del árbol, y Fyne
// entrega el clic al objeto MÁS PROFUNDO que sepa recibirlo. Así sale gratis lo que
// en la web hace el `onChange` del `<input>`, que no burbujea al `onClick` del `li`:
// marcar no abre.
func (f *FilaLista) Tapped(*fyne.PointEvent) {
	if f.AlPulsar != nil {
		f.AlPulsar()
	}
}

// MouseIn es `:hover`: `hover:bg-bg-subtle`, que solo se pinta si NO está
// seleccionada (la web pone el hover en la rama del `else`).
func (f *FilaLista) MouseIn(*desktop.MouseEvent) { f.encima = true; f.Refresh() }

// MouseMoved no hace nada: el estado lo fijan la entrada y la salida.
func (f *FilaLista) MouseMoved(*desktop.MouseEvent) {}

// MouseOut devuelve la fila a su color de reposo.
func (f *FilaLista) MouseOut() { f.encima = false; f.Refresh() }

// AccessibilityLabel es el `title` de la web: título y subtítulo unidos por una raya.
func (f *FilaLista) AccessibilityLabel() string {
	if f.Subtitulo == "" {
		return f.Titulo
	}
	return f.Titulo + " — " + f.Subtitulo
}

// AccessibilityRole es `option` dentro de un listbox, que es lo que la web emite
// cuando la lista es seleccionable.
func (f *FilaLista) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleButton
}

type filaListaRenderer struct {
	f                       *FilaLista
	base, velo, galon       *canvas.Rectangle
	casilla                 *Eleccion
	titulo, subtitulo, meta *canvas.Text
	chevron                 *iconos.Dibujo
	objetos                 []fyne.CanvasObject
	// pTitulo y pSubtitulo sustituyen a titulo y subtitulo cuando la fila Envuelve.
	pTitulo, pSubtitulo *Parrafo
}

// casillaDeFila es la `Eleccion` sin etiqueta que marca una fila: el texto ya está en
// la fila, así que la casilla solo lo ANUNCIA. Es el `aria-label="Seleccionar fila"`
// de Table.tsx.
func casillaDeFila(marcada bool, alMarcar func()) *Eleccion {
	c := NuevaCasilla(textoComun("ui.selectRow"), func(bool) {
		if alMarcar != nil {
			alMarcar()
		}
	})
	c.SinEtiqueta, c.Marcada = true, marcada
	return c
}

func (r *filaListaRenderer) MinSize() fyne.Size {
	f := r.f
	if f.Envuelve > 0 && r.pTitulo != nil {
		_, w := r.columna(f.Envuelve)
		texto := AltoParrafo(r.pTitulo, w)
		if r.pSubtitulo != nil {
			texto += AltoParrafo(r.pSubtitulo, w)
		}
		return fyne.NewSize(f.Envuelve, 2*RellenoYFila()+max(texto, f.altoDelantero(), MedidaEleccion("").Height))
	}
	alto := MedidaFilaLista(f.Subtitulo != "", f.altoDelantero())
	return fyne.NewSize(BordeFila+2*MargenFila, alto)
}

// columna es dónde empieza la columna de texto y cuánto mide en una fila de ese ancho:
// lo que dejan la casilla y el `leading` a la izquierda y el galón y la meta a la
// derecha. La usan la colocación y, cuando la fila Envuelve, la medida.
func (r *filaListaRenderer) columna(ancho float32) (x, w float32) {
	f := r.f
	x = InicioFila()
	if r.casilla != nil {
		x += MedidaEleccion("").Width + HuecoFila
	}
	if f.Delantero != nil {
		x += f.Delantero.MinSize().Width + HuecoFila
	}
	derecha := ancho - MargenFila
	if r.chevron != nil {
		derecha -= LadoGalon + HuecoFila
	}
	if r.meta != nil {
		derecha -= Medir(f.Meta, tema.TextoXS, Normal).Width + HuecoFila
	}
	return x, derecha - x
}

func (r *filaListaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaListaRenderer) Destroy()                     {}

func (r *filaListaRenderer) Layout(t fyne.Size) {
	f := r.f
	r.base.Resize(t)
	r.base.Move(fyne.NewPos(0, 0))
	// El `border-l` es un BORDE: ocupa su ancho y el relleno empieza detrás.
	ColocarVeloFila(r.velo, r.galon, t.Width, t.Height)

	x := InicioFila()
	centro := func(alto float32) float32 { return (t.Height - alto) / 2 }

	if r.casilla != nil {
		m := MedidaEleccion("")
		r.casilla.Resize(m)
		r.casilla.Move(fyne.NewPos(x, centro(m.Height)))
		x += m.Width + HuecoFila
	}
	if f.Delantero != nil {
		m := f.Delantero.MinSize()
		f.Delantero.Resize(m)
		f.Delantero.Move(fyne.NewPos(x, centro(m.Height)))
		x += m.Width + HuecoFila
	}

	derecha := t.Width - MargenFila
	if r.chevron != nil {
		r.chevron.Colocar(derecha-LadoGalon, centro(LadoGalon), LadoGalon)
		derecha -= LadoGalon + HuecoFila
	}
	if r.meta != nil {
		// `shrink-0`: la meta nunca encoge, encoge la columna de texto.
		ancho := Medir(f.Meta, tema.TextoXS, Normal).Width
		ColocarTexto(r.meta, tema.TextoXS, derecha-ancho, centro(tema.TextoXS.Linea))
		derecha -= ancho + HuecoFila
	}

	// La columna de texto es `flex-1 min-w-0`: lo que queda, y con `truncate`.
	disponible := derecha - x
	if r.pTitulo != nil {
		// …o partida en líneas, apiladas y centradas como las dos de siempre.
		mt := MedidaParrafo(r.pTitulo, disponible)
		alto := mt.Height
		var ms fyne.Size
		if r.pSubtitulo != nil {
			ms = MedidaParrafo(r.pSubtitulo, disponible)
			alto += ms.Height
		}
		y := centro(alto)
		r.pTitulo.Resize(mt)
		r.pTitulo.Move(fyne.NewPos(x, y))
		if r.pSubtitulo != nil {
			r.pSubtitulo.Resize(ms)
			r.pSubtitulo.Move(fyne.NewPos(x, y+mt.Height))
		}
		return
	}
	altoTexto := tema.TextoBase.Linea
	if r.subtitulo != nil {
		altoTexto += tema.TextoSM.Linea
	}
	y := centro(altoTexto)
	Truncar(r.titulo, f.Titulo, tema.TextoBase, r.pesoTitulo(), disponible)
	ColocarTexto(r.titulo, tema.TextoBase, x, y)
	if r.subtitulo != nil {
		Truncar(r.subtitulo, f.Subtitulo, tema.TextoSM, r.pesoSubtitulo(), disponible)
		ColocarTexto(r.subtitulo, tema.TextoSM, x, y+tema.TextoBase.Linea)
	}
}

func (r *filaListaRenderer) pesoTitulo() Peso {
	if r.f.SinLeer {
		return Negrita
	}
	return Medio
}

func (r *filaListaRenderer) pesoSubtitulo() Peso {
	if r.f.SinLeer {
		return Negrita
	}
	return Normal
}

func (r *filaListaRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.base.FillColor = tema.Color(f.fondo(), v)

	// El orden de la web: seleccionada gana, y el hover solo existe en la otra rama.
	switch {
	case f.Seleccionada:
		r.velo.FillColor = tema.Color("selected", v)
		r.galon.FillColor = tema.Color("primary", v)
	case f.encima:
		r.velo.FillColor = tema.Color("bg-subtle", v)
		r.galon.FillColor = color.NRGBA{}
	default:
		r.velo.FillColor = color.NRGBA{}
		r.galon.FillColor = color.NRGBA{}
	}

	tintaTitulo, pesoTitulo := tema.Color("text", v), Medio
	tintaSub := tema.Color("text-muted", v)
	if f.SinLeer {
		tintaTitulo, pesoTitulo = tema.Color("primary-text", v), Negrita
		tintaSub = tema.Color("primary-text", v)
	}
	r.titulo.Color, r.titulo.TextStyle = tintaTitulo, fyne.TextStyle{}
	r.titulo.FontSource = pesoTitulo.cara()
	if r.pTitulo != nil {
		r.pTitulo.Texto, r.pTitulo.Tinta, r.pTitulo.Peso = f.Titulo, tintaTitulo, pesoTitulo
	}
	if r.pSubtitulo != nil {
		r.pSubtitulo.Texto, r.pSubtitulo.Tinta, r.pSubtitulo.Peso = f.Subtitulo, tintaSub, r.pesoSubtitulo()
	}
	if r.subtitulo != nil {
		r.subtitulo.Color = tintaSub
		r.subtitulo.FontSource = r.pesoSubtitulo().cara()
	}
	if r.casilla != nil {
		r.casilla.Marcada = f.Seleccionada
	}
	if r.chevron != nil {
		r.chevron.Tenir("text-subtle", v)
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
	// Por lo mismo que en FilaDrive: reciclada en un `widget.List`, el tamaño no
	// cambia y `Layout` no vuelve a correr. Aquí lo que se descoloca es la meta, que
	// también va pegada a la derecha, y el truncado del título.
	r.Layout(r.f.Size())
}

// ---- La fila del explorador de Sytue Drive ----

// TeselaDrive es el `size={36}` con el que FileBrowser.tsx pinta el icono de tipo.
// Es lo que decide el alto de la fila, no el texto.
const TeselaDrive float32 = 36

// Anchos de las columnas de la derecha: `w-24`, `w-28` y `w-8` en rem de la suite.
const (
	ColTamano   float32 = 24 * 0.25 * tema.Rem
	ColFecha    float32 = 28 * 0.25 * tema.Rem
	ColAcciones float32 = 8 * 0.25 * tema.Rem
)

// AltoFilaDrive es el alto de la fila SIN el hairline de abajo: `py-2` a cada lado
// más la tesela de 36.
//
// La medida real la lee VirtualList; aquí se declara desde las mismas clases.
const AltoFilaDrive float32 = 2*AltoFila + TeselaDrive

// MedidaFilaDrive es el alto de la fila con su `border-b`.
func MedidaFilaDrive() float32 { return AltoFilaDrive + tema.Borde }

// separaMetaFila es el `mt-0.5` de las líneas de debajo del nombre.
const separaMetaFila = 0.5 * 0.25 * tema.Rem

// AltoFilaDriveCon es el paso de una fila con `lineas` líneas de `sytue-meta` bajo el
// nombre: manda la tesela de 36 hasta que el texto pasa de ella. VirtualList
// (TanStack) coloca cada fila con su alto medido y REDONDEADO, así que el paso es entero.
func AltoFilaDriveCon(lineas int) float32 {
	texto := tema.TextoBase.Linea + float32(lineas)*(separaMetaFila+tema.TextoXS.Linea)
	return float32(math.Round(float64(2*AltoFila + max(TeselaDrive, texto) + tema.Borde)))
}

// FilaDrive es la fila del explorador de ficheros de Sytue Drive.
type FilaDrive struct {
	widget.BaseWidget

	Nombre string
	// Tamano y Fecha son las dos columnas de la derecha, YA formateadas por quien
	// llama: el separador de miles y el formato de fecha son del idioma.
	Tamano string
	Fecha  string
	// Extension decide la tesela; vacía con Carpeta = false pinta la tesela neutra.
	Extension string
	Carpeta   bool

	Seleccionada bool
	ConCasilla   bool
	// Ubicacion es la carpeta donde vive, en una búsqueda (`showLocation`): una línea de
	// `sytue-meta` bajo el nombre con la carpeta de 12 delante. Vacía, no hay línea.
	Ubicacion string
	// Meta es la línea que la web pinta bajo el nombre por debajo de `sm` (`sm:hidden`),
	// cuando se esconden las columnas: «Carpeta · 06 sept 2026». Solo se ve estrecha.
	Meta string
	// Destino es que se está arrastrando algo que puede soltarse en esta carpeta: `bg-selected
	// ring-2 ring-primary ring-inset`. Solo se pinta donde el movimiento se aceptaría: un
	// resalte que sale también donde se va a rechazar enseña a desconfiar del resalte.
	Destino bool
	// SeleccionActiva es que hay algo marcado en la lista: entonces TODAS las casillas
	// se ven. Sin ella, la de una fila solo aparece al pasar por encima (`opacity-0
	// group-hover:opacity-100`), siempre en un puntero grueso y siempre si está marcada.
	// Ocupa su sitio igual: es opacidad, no `hidden`.
	SeleccionActiva bool
	// Ventana es el ancho de la VENTANA en px de CSS, no el de la fila: `sm:` y
	// `md:` son media queries. Con 0 se enseñan las dos columnas.
	Ventana float32

	AlAbrir  func()
	AlMarcar func()
	// AlMenu abre el menú de acciones de la fila —el «⋯» de `ActionsMenu`— colgado de
	// `disparador`, que es la caja del botón. nil: la fila no lleva menú.
	AlMenu func(disparador fyne.CanvasObject)
	// GlifoMas es el dibujo del «⋯». Lo pone la app: es el `more` de Drive (tres
	// círculos en vertical), que no es ni el `ellipsis` del catálogo ni el de Rooms.
	GlifoMas *iconos.Dibujo
	// SiempreMenu enseña el «⋯» sin pasar por encima: `pointer-coarse:opacity-100`.
	// En un teléfono no hay «por encima», y un menú invisible pero pulsable deja sin
	// forma de descargar, renombrar ni borrar nada.
	SiempreMenu bool

	encima, encimaMas bool
	// zonaMas es la caja del «⋯», de la que se cuelga el menú. La crea el renderer.
	zonaMas *canvas.Rectangle
}

var (
	_ fyne.Widget       = (*FilaDrive)(nil)
	_ fyne.Tappable     = (*FilaDrive)(nil)
	_ desktop.Hoverable = (*FilaDrive)(nil)
)

// LadoMasFila es el `h-8 w-8` del botón «⋯», que llena la columna `w-8` de acciones.
const LadoMasFila = ColAcciones

// cajaMas es dónde cae el «⋯» en una fila de ese tamaño: pegado a la derecha, dentro
// del `px-3`, y centrado en el alto sin el filete.
func cajaMas(t fyne.Size) (fyne.Position, fyne.Size) {
	contenido := t.Height - tema.Borde
	return fyne.NewPos(t.Width-MargenFila-LadoMasFila, (contenido-LadoMasFila)/2),
		fyne.NewSize(LadoMasFila, LadoMasFila)
}

// enMas dice si un punto de la fila cae sobre el «⋯».
func (f *FilaDrive) enMas(p fyne.Position) bool {
	if f.AlMenu == nil {
		return false
	}
	o, t := cajaMas(f.Size())
	return p.X >= o.X && p.X < o.X+t.Width && p.Y >= o.Y && p.Y < o.Y+t.Height
}

// NuevaFilaDrive crea la fila de un fichero.
func NuevaFilaDrive(nombre, extension string) *FilaDrive {
	f := &FilaDrive{Nombre: nombre, Extension: extension}
	f.ExtendBaseWidget(f)
	return f
}

// haySM y hayMD dicen si las columnas de tamaño y fecha están visibles. Con Ventana
// a 0 se asume ancha, que es el caso del escritorio.
func (f *FilaDrive) haySM() bool { return f.Ventana == 0 || f.Ventana >= CorteSM }
func (f *FilaDrive) hayMD() bool { return f.Ventana == 0 || f.Ventana >= CorteMD }

// Lineas es cuántas líneas de `sytue-meta` van bajo el nombre ahora.
func (f *FilaDrive) Lineas() int {
	n := 0
	if f.Ubicacion != "" {
		n++
	}
	if !f.haySM() {
		n++
	}
	return n
}

// CreateRenderer compone la fila.
func (f *FilaDrive) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaDriveRenderer{
		f:         f,
		base:      canvas.NewRectangle(tema.Color("bg", v)),
		velo:      canvas.NewRectangle(color.NRGBA{}),
		linea:     canvas.NewRectangle(tokens.ConAlfa(tema.Color("border", v), 0.6)),
		tesela:    NuevaTeselaTipo(f.Extension, f.Carpeta, TeselaDrive),
		nombre:    Texto(f.Nombre, tema.TextoBase, Medio, tema.Color("text", v)),
		tamano:    Texto(f.Tamano, tema.TextoBase, Normal, tema.Color("text-muted", v)),
		fecha:     Texto(f.Fecha, tema.TextoBase, Normal, tema.Color("text-muted", v)),
		ubicacion: Texto("", tema.TextoXS, Normal, tema.Color("text-subtle", v)),
		meta:      Texto("", tema.TextoXS, Normal, tema.Color("text-subtle", v)),
		carpeta:   iconos.Trazo(iconos.DriveCarpeta, LadoCarpetaUbicacion, "text-subtle", v),
	}
	r.objetos = []fyne.CanvasObject{r.base, r.velo}
	if f.ConCasilla {
		r.casilla = casillaDeFila(f.Seleccionada, f.AlMarcar)
		r.objetos = append(r.objetos, r.casilla)
	}
	r.objetos = append(r.objetos, r.tesela.Objetos()...)
	r.objetos = append(r.objetos, r.nombre, r.tamano, r.fecha, r.linea, r.ubicacion, r.meta, r.carpeta.Objeto())
	for i := range r.anillo {
		r.anillo[i] = canvas.NewRectangle(tema.Color("primary", v))
		r.objetos = append(r.objetos, r.anillo[i])
	}
	// El «⋯»: la caja donde se cuelga el menú (transparente: solo da la posición), el
	// velo de `hover:bg-bg-subtle` y el glifo de 16.
	r.zonaMas = canvas.NewRectangle(color.NRGBA{})
	f.zonaMas = r.zonaMas
	r.veloMas = canvas.NewRectangle(color.NRGBA{})
	r.veloMas.CornerRadius = tema.RadioMD
	r.objetos = append(r.objetos, r.zonaMas, r.veloMas)
	if f.GlifoMas != nil {
		r.objetos = append(r.objetos, f.GlifoMas.Objeto())
	}
	r.Refresh()
	return r
}

// Tapped abre, salvo que el toque caiga en el «⋯», que abre el menú: en la web el botón
// para la propagación para que abrir el menú no abra también la fila. La casilla se la
// queda la `Eleccion`, que está más adentro.
//
// El «⋯» NO es un widget hijo, y es a propósito: Fyne da el puntero al Hoverable más
// profundo, así que al pasar de la fila a un botón hijo la fila recibía `MouseOut`,
// escondía el botón, y el botón desaparecía justo al llegar a él.
func (f *FilaDrive) Tapped(e *fyne.PointEvent) {
	if e != nil && f.enMas(e.Position) {
		if f.zonaMas != nil {
			f.AlMenu(f.zonaMas)
		}
		return
	}
	if f.AlAbrir != nil {
		f.AlAbrir()
	}
}

// MouseIn es `hover:bg-bg-subtle`, y además destapa el «⋯».
func (f *FilaDrive) MouseIn(e *desktop.MouseEvent) {
	f.encima = true
	f.encimaMas = f.enMas(e.Position)
	f.Refresh()
}

// MouseMoved enciende el velo del «⋯» al pasar por encima de él.
func (f *FilaDrive) MouseMoved(e *desktop.MouseEvent) {
	if en := f.enMas(e.Position); en != f.encimaMas {
		f.encimaMas = en
		f.Refresh()
	}
}

// MouseOut quita el hover.
func (f *FilaDrive) MouseOut() { f.encima, f.encimaMas = false, false; f.Refresh() }

// verMas dice si el «⋯» se pinta: al pasar por encima de la fila, o siempre en un
// puntero grueso (`opacity-0 group-hover:opacity-100 pointer-coarse:opacity-100`).
func (f *FilaDrive) verMas() bool {
	return f.AlMenu != nil && f.GlifoMas != nil && (f.encima || f.SiempreMenu)
}

// AccessibilityLabel es el nombre del fichero.
func (f *FilaDrive) AccessibilityLabel() string { return f.Nombre }

// AccessibilityRole implementa fyne.Accessible.
func (f *FilaDrive) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

type filaDriveRenderer struct {
	f                     *FilaDrive
	base, velo, linea     *canvas.Rectangle
	anillo                [4]*canvas.Rectangle // el `ring-2` de destino, por dentro
	zonaMas, veloMas      *canvas.Rectangle
	casilla               *Eleccion
	tesela                *TeselaTipo
	nombre, tamano, fecha *canvas.Text
	ubicacion, meta       *canvas.Text
	carpeta               *iconos.Dibujo
	objetos               []fyne.CanvasObject
}

func (r *filaDriveRenderer) MinSize() fyne.Size {
	return fyne.NewSize(2*MargenFila+TeselaDrive, AltoFilaDriveCon(r.f.Lineas()))
}

// LadoCarpetaUbicacion es el `size={12}` de la carpeta de la línea de ubicación, y
// HuecoUbicacion su `gap-1`.
const (
	LadoCarpetaUbicacion float32 = 12
	HuecoUbicacion       float32 = 1 * 0.25 * tema.Rem
)

func (r *filaDriveRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaDriveRenderer) Destroy()                     {}

func (r *filaDriveRenderer) Layout(t fyne.Size) {
	f := r.f
	r.base.Resize(t)
	r.base.Move(fyne.NewPos(0, 0))
	r.velo.Resize(t)
	r.velo.Move(fyne.NewPos(0, 0))
	// El `border-b` va DENTRO de la caja: `box-sizing: border-box`.
	r.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
	r.linea.Move(fyne.NewPos(0, t.Height-tema.Borde))

	contenido := t.Height - tema.Borde
	centro := func(alto float32) float32 { return (contenido - alto) / 2 }
	// `ring-2 ring-inset`: dos píxeles por dentro de la caja, sin el filete.
	const g = 2
	for i, c := range [4][4]float32{{0, 0, t.Width, g}, {0, t.Height - g, t.Width, g}, {0, 0, g, t.Height}, {t.Width - g, 0, g, t.Height}} {
		r.anillo[i].Move(fyne.NewPos(c[0], c[1]))
		r.anillo[i].Resize(fyne.NewSize(c[2], c[3]))
	}

	x := MargenFila
	if r.casilla != nil {
		m := MedidaEleccion("")
		r.casilla.Resize(m)
		r.casilla.Move(fyne.NewPos(x, centro(m.Height)))
		x += m.Width + HuecoFila
	}
	r.tesela.Colocar(x, centro(TeselaDrive))
	x += TeselaDrive + HuecoFila

	derecha := t.Width - MargenFila
	// El menú de acciones (`w-8`) ocupa sitio SIEMPRE, aunque el icono solo aparezca
	// al pasar por encima: es `opacity-0`, no `hidden`.
	o, m := cajaMas(t)
	r.zonaMas.Move(o)
	r.zonaMas.Resize(m)
	r.veloMas.Move(o)
	r.veloMas.Resize(m)
	if f.GlifoMas != nil {
		f.GlifoMas.Colocar(o.X+(m.Width-iconos.TallaSM)/2, o.Y+(m.Height-iconos.TallaSM)/2, iconos.TallaSM)
	}
	derecha -= ColAcciones + HuecoFila
	if f.hayMD() {
		ColocarTexto(r.fecha, tema.TextoBase, derecha-ColFecha, centro(tema.TextoBase.Linea))
		r.fecha.Show()
		derecha -= ColFecha + HuecoFila
	} else {
		r.fecha.Hide()
	}
	if f.haySM() {
		// `text-right`: la columna mide `w-24` y el texto se pega a su derecha.
		ancho := Medir(f.Tamano, tema.TextoBase, Normal).Width
		ColocarTexto(r.tamano, tema.TextoBase, derecha-ancho, centro(tema.TextoBase.Linea))
		r.tamano.Show()
		derecha -= ColTamano + HuecoFila
	} else {
		r.tamano.Hide()
	}

	// El nombre y sus líneas de debajo se centran JUNTOS, como el `span` de la web.
	cabe := derecha - x
	alto := tema.TextoBase.Linea + float32(f.Lineas())*(separaMetaFila+tema.TextoXS.Linea)
	y := centro(alto)
	Truncar(r.nombre, f.Nombre, tema.TextoBase, Medio, cabe)
	ColocarTexto(r.nombre, tema.TextoBase, x, y)
	y += tema.TextoBase.Linea
	r.ubicacion.Hidden, r.meta.Hidden = f.Ubicacion == "", f.haySM()
	r.carpeta.Ver(f.Ubicacion != "")
	if f.Ubicacion != "" {
		y += separaMetaFila
		r.carpeta.Colocar(x, y+(tema.TextoXS.Linea-LadoCarpetaUbicacion)/2, LadoCarpetaUbicacion)
		dx := LadoCarpetaUbicacion + HuecoUbicacion
		Truncar(r.ubicacion, f.Ubicacion, tema.TextoXS, Normal, max(cabe-dx, 0))
		ColocarTexto(r.ubicacion, tema.TextoXS, x+dx, y)
		y += tema.TextoXS.Linea
	}
	if !f.haySM() {
		y += separaMetaFila
		Truncar(r.meta, f.Meta, tema.TextoXS, Normal, cabe)
		ColocarTexto(r.meta, tema.TextoXS, x, y)
	}
}

func (r *filaDriveRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.base.FillColor = tema.Color("bg", v)
	r.linea.FillColor = tokens.ConAlfa(tema.Color("border", v), 0.6)
	for _, a := range r.anillo {
		a.FillColor = tema.Color("primary", v)
		a.Hidden = !f.Destino
	}
	switch {
	case f.Seleccionada || f.Destino:
		r.velo.FillColor = tema.Color("selected", v)
	case f.encima:
		r.velo.FillColor = tema.Color("bg-subtle", v)
	default:
		r.velo.FillColor = color.NRGBA{}
	}
	// `text-text-muted hover:bg-bg-subtle hover:text-text` del disparador del menú.
	r.veloMas.FillColor = color.NRGBA{}
	if f.encimaMas {
		r.veloMas.FillColor = tema.Color("bg-subtle", v)
	}
	if f.GlifoMas != nil {
		tinta := "text-muted"
		if f.encimaMas {
			tinta = "text"
		}
		f.GlifoMas.Tenir(tinta, v)
		f.GlifoMas.Ver(f.verMas())
	}
	r.nombre.Text, r.tamano.Text, r.fecha.Text = f.Nombre, f.Tamano, f.Fecha
	r.ubicacion.Color, r.meta.Color = tema.Color("text-subtle", v), tema.Color("text-subtle", v)
	r.carpeta.Tenir("text-subtle", v)
	r.tesela.Poner(f.Extension, f.Carpeta, v)
	if r.casilla != nil {
		r.casilla.Marcada = f.Seleccionada
		r.casilla.Hidden = !f.Seleccionada && !f.SeleccionActiva && !f.encima && !f.SiempreMenu
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
	// Y SE VUELVE A COLOCAR. Medido el 2026-09-22 mirando el PNG del explorador:
	//
	// dentro de un `widget.List` la fila se RECICLA —`UpdateItem` la rellena con otro
	// fichero— y el tamaño no cambia, así que Fyne no vuelve a llamar a `Layout`. Con
	// los datos nuevos y la geometría vieja se rompen las dos cosas que dependen del
	// contenido: la columna de tamaño va pegada a la DERECHA, así que su x se calcula
	// con el ancho del texto ANTERIOR (con la fila recién creada el texto está vacío,
	// la x sale 10,5 px a la derecha y «4,0 KB» se pinta ENCIMA de la fecha), y la
	// tesela decide en `Colocar` si enseña la carpeta o la extensión, así que una
	// carpeta salía como un cuadrado vacío.
	//
	// Los dos fallos pasaban los guardianes de la fila, que construyen una fila nueva
	// y la miden una vez. Lo vigila `TestLaFilaRecicladaSeVuelveAColocar`.
	r.Layout(r.f.Size())
}

// ---- La cabecera de columnas del explorador ----

// AltoCabeceraDrive es `py-2.5` a cada lado más la caja de `sytue-overline`
// (`text-xs`, caja de 1rem). El `shadow-[inset_0_-1px_0_…]` es INTERIOR, así que no
// añade alto. La caja sigue la línea tipográfica cuando cambia la raíz.
var AltoCabeceraDrive = 2*(2.5*0.25*tema.Rem) + tema.TextoXS.Linea

// CabeceraDrive es la fila de rótulos de columna del explorador.
//
// No es la cabecera de Table.tsx: va sobre `bg-bg` (se funde con el lienzo), lleva
// `mx-3` y el hairline de abajo es una SOMBRA INTERIOR, no un borde. Es lo que hace
// que la lista de Drive y una tabla de la suite no se puedan portar con el mismo
// código aunque de lejos se parezcan.
type CabeceraDrive struct {
	widget.BaseWidget

	Nombre, Tamano, Fecha string
	ConCasilla            bool
	Todas, Algunas        bool
	Ventana               float32
	AlMarcarTodas         func()
}

var _ fyne.Widget = (*CabeceraDrive)(nil)

// NuevaCabeceraDrive crea la cabecera con los tres rótulos ya traducidos.
func NuevaCabeceraDrive(nombre, tamano, fecha string) *CabeceraDrive {
	c := &CabeceraDrive{Nombre: nombre, Tamano: tamano, Fecha: fecha}
	c.ExtendBaseWidget(c)
	return c
}

// CreateRenderer compone la cabecera.
func (c *CabeceraDrive) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta := tema.Color("text-subtle", v)
	r := &cabeceraDriveRenderer{
		c:      c,
		base:   canvas.NewRectangle(tema.Color("bg", v)),
		sombra: canvas.NewRectangle(BordeSobre("border", "bg", v)),
		nombre: NuevoVersal(c.Nombre, tema.TextoXS, SemiNegrita, tinta),
		tamano: NuevoVersal(c.Tamano, tema.TextoXS, SemiNegrita, tinta),
		fecha:  NuevoVersal(c.Fecha, tema.TextoXS, SemiNegrita, tinta),
	}
	r.objetos = []fyne.CanvasObject{r.base}
	if c.ConCasilla {
		// ponytail: `Eleccion` no tiene estado indeterminado y la cabecera de Drive
		// sí (`someSelected`). Aquí se pinta marcada o no; el tercer estado se cierra
		// añadiendo un `Parcial bool` a `Eleccion`, que es de otro agente.
		r.casilla = casillaDeFila(c.Todas, c.AlMarcarTodas)
		r.objetos = append(r.objetos, r.casilla)
	}
	r.objetos = append(r.objetos, r.nombre.Objetos()...)
	r.objetos = append(r.objetos, r.tamano.Objetos()...)
	r.objetos = append(r.objetos, r.fecha.Objetos()...)
	r.objetos = append(r.objetos, r.sombra)
	return r
}

type cabeceraDriveRenderer struct {
	c                     *CabeceraDrive
	base, sombra          *canvas.Rectangle
	casilla               *Eleccion
	nombre, tamano, fecha *Versal
	objetos               []fyne.CanvasObject
}

func (r *cabeceraDriveRenderer) MinSize() fyne.Size {
	return fyne.NewSize(2*MargenFila, AltoCabeceraDrive)
}

func (r *cabeceraDriveRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *cabeceraDriveRenderer) Destroy()                     {}

func (r *cabeceraDriveRenderer) Layout(t fyne.Size) {
	c := r.c
	r.base.Resize(t)
	r.base.Move(fyne.NewPos(0, 0))
	// `shadow-[inset_0_-1px_0_var(--sytue-border)]`: un píxel POR DENTRO del borde
	// de abajo, no por fuera. La sombra no suma un píxel al alto de la cabecera.
	r.sombra.Resize(fyne.NewSize(t.Width, tema.Borde))
	r.sombra.Move(fyne.NewPos(0, t.Height-tema.Borde))

	y := (t.Height - tema.TextoXS.Linea) / 2
	x := MargenFila
	if r.casilla != nil {
		m := MedidaEleccion("")
		r.casilla.Resize(m)
		r.casilla.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		x += m.Width + HuecoFila
	}
	r.nombre.Colocar(x, y)

	// Las columnas que no caben (`hidden md:block`, `hidden sm:block`) se esconden: sin
	// colocar, sus letras se amontonaban en la esquina de arriba a la izquierda, un
	// borrón encima de la casilla a 360 (visto en una ventana real el 2026-09-27).
	derecha := t.Width - MargenFila - ColAcciones - HuecoFila
	conFecha, conTamano := c.Ventana == 0 || c.Ventana >= CorteMD, c.Ventana == 0 || c.Ventana >= CorteSM
	r.fecha.Ver(conFecha)
	r.tamano.Ver(conTamano)
	if conFecha {
		r.fecha.Colocar(derecha-ColFecha, y)
		derecha -= ColFecha + HuecoFila
	}
	if conTamano {
		r.tamano.Colocar(derecha-r.tamano.Ancho(), y)
	}
}

func (r *cabeceraDriveRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.base.FillColor = tema.Color("bg", v)
	r.sombra.FillColor = BordeSobre("border", "bg", v)
	tinta := tema.Color("text-subtle", v)
	for _, e := range []*Versal{r.nombre, r.tamano, r.fecha} {
		e.Pintar(tinta)
	}
	if r.casilla != nil {
		r.casilla.Marcada = r.c.Todas
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}
