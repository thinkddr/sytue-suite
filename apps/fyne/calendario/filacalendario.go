// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/nucleo/tokens"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La fila de «Mis calendarios». Es la ÚNICA del menú de la suite que lleva casilla, y
// por eso no es `ui.FilaLateral`: allí el hueco de la izquierda es un icono o un punto
// de color, y aquí es una casilla de 14 (`h-3.5 w-3.5`) TEÑIDA con el color del
// calendario (`accentColor`), que es un dato y no un token.
//
// El alto sí es el del catálogo y sale de la misma cuenta:
//
//	py-2 × 2 + max(casilla 14, caja de línea de `text-base` 19,25) = 33,25   medido
//
// Medido en `calendar/mes.ancho.json`: las filas del menú miden 33,25 × 231.
const (
	// LadoCasillaCalendario es el `h-3.5 w-3.5`.
	LadoCasillaCalendario float32 = 3.5 * 0.25 * tema.Rem
	// HuecoFilaCalendario es el `gap-2.5` entre casilla y nombre.
	HuecoFilaCalendario float32 = 2.5 * 0.25 * tema.Rem
	// LadoKebab es el `h-6 w-6` del botón de opciones.
	LadoKebab float32 = 6 * 0.25 * tema.Rem
	// LadoTicCasilla es el tic de dentro, un píxel por dentro de la casilla.
	LadoTicCasilla = LadoCasillaCalendario - 2
)

// AltoFilaCalendario es la medida declarada, y la única cuenta.
func AltoFilaCalendario() float32 {
	return 2*espacio2 + max(LadoCasillaCalendario, tema.TextoBase.Linea)
}

// FilaCalendario es una entrada de «Mis calendarios» o de «Compartidos conmigo».
//
// Se enfoca con el tabulador como la casilla de la web, y Espacio o Intro la conmutan:
// es `ui.Pulsable`. El aro va en la CASILLA y no en la fila, con su esquina de 2 y el
// blanco del cromo, que es el `<span class="sytue-focus-ring-choice
// sytue-focus-ring-chrome">` de CalendarApp.tsx.
type FilaCalendario struct {
	ui.Pulsable

	// Nombre es el `displayName` del calendario.
	Nombre string
	// Color es el suyo, y tiñe la casilla.
	Color color.NRGBA
	// Marcada es si sus eventos se pintan en la rejilla. NO se llama `Visible`:
	// `fyne.Widget` ya tiene un método con ese nombre y un campo lo taparía.
	Marcada bool
	// Suscrito pone el globo de «solo lectura» al final del nombre.
	Suscrito bool
	// Permiso es la insignia de «Compartidos conmigo» («Puede editar» / «Puede ver»),
	// ya traducida. Vacía: no hay insignia y sí puede haber kebab.
	Permiso string
	// PermisoEditar tiñe la insignia como la de un writer.
	PermisoEditar bool
	// RotuloVer es el `aria-label` de la casilla, ya traducido.
	RotuloVer string
	// RotuloOpciones es el del kebab. Vacío: sin kebab (sin conexión, o compartido).
	RotuloOpciones string

	AlConmutar func()
	// AlAbrirMenu despliega el menú «⋮» colgando del kebab, que es lo que recibe.
	AlAbrirMenu func(disparador fyne.CanvasObject)
}

var (
	_ fyne.Widget       = (*FilaCalendario)(nil)
	_ fyne.Tappable     = (*FilaCalendario)(nil)
	_ fyne.Focusable    = (*FilaCalendario)(nil)
	_ desktop.Hoverable = (*FilaCalendario)(nil)
)

// NuevaFilaCalendario crea la fila.
func NuevaFilaCalendario(nombre string, c color.NRGBA, marcada bool) *FilaCalendario {
	f := &FilaCalendario{Nombre: nombre, Color: c, Marcada: marcada}
	// Pulsar conmuta la visibilidad: en la web la casilla va dentro de un `<label>` que
	// ocupa toda la fila, así que pulsar el nombre también la marca.
	f.AlPulsar = func() {
		if f.AlConmutar != nil {
			f.AlConmutar()
		}
	}
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityLabel es el `aria-label` de la casilla, que es lo que la web anuncia.
func (f *FilaCalendario) AccessibilityLabel() string {
	if f.RotuloVer != "" {
		return f.RotuloVer
	}
	return f.Nombre
}

// El rol es el botón de `ui.Pulsable`: es una casilla, pero Fyne 2.8.1 solo conoce
// cuatro roles y ninguno dice «marcado» —techo del toolkit, `docs/roadmap.md`.

// CreateRenderer compone fondo, casilla, nombre y lo que lleve a la derecha.
func (f *FilaCalendario) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaCalRenderer{
		f:     f,
		fondo: canvas.NewRectangle(color.NRGBA{}),
		// La esquina es la de la casilla de Chromium (2) y no `RadioSM` (4): la web la
		// pinta nativa, teñida con `accentColor`.
		casilla: ui.NuevaCaja(f.relleno(), f.Color, ui.RadioCasillaNativa),
		aro:     ui.NuevoAroCon("sidebar-focus", ui.RadioCasillaNativa, ui.SinHueco, v).Abierto(),
		nombre:  ui.Texto(f.Nombre, tema.TextoBase, ui.Medio, tema.Color("sidebar-fg", v)),
	}
	r.fondo.CornerRadius = tema.RadioMD
	r.objetos = []fyne.CanvasObject{r.fondo}
	r.objetos = append(r.objetos, r.casilla.Objetos()...)
	r.objetos = append(r.objetos, r.aro.Objetos()...)
	if f.Marcada {
		r.tic = iconos.TrazoConTinta(iconos.Tic, LadoTicCasilla, blanco)
		r.objetos = append(r.objetos, r.tic.Objeto())
	}
	r.objetos = append(r.objetos, r.nombre)
	if f.Suscrito {
		r.globo = iconos.Trazo(iconos.Globo, 14, "sidebar-fg-muted", v)
		r.objetos = append(r.objetos, r.globo.Objeto())
	}
	if f.Permiso != "" {
		r.permiso = nuevaInsigniaPermiso(f.Permiso, f.PermisoEditar)
		r.objetos = append(r.objetos, r.permiso.objetos()...)
	}
	if f.RotuloOpciones != "" {
		punto := iconos.Trazo(iconos.Puntos, 16, "sidebar-fg-muted", v)
		r.kebab = ui.NuevaAccionLateralCon(f.RotuloOpciones, punto, func() {
			if f.AlAbrirMenu != nil {
				f.AlAbrirMenu(r.kebab)
			}
		})
		r.kebab.AlCambiarEstado(f.Refresh)
		r.objetos = append(r.objetos, r.kebab)
	}
	r.Refresh() // regla 7 de `ui`: el kebab nace escondido, no a la vista
	return r
}

// blanco es el tic de una casilla marcada: la web usa `accent-color` con
// `color-scheme: light`, y ahí el navegador pinta la marca en blanco.
var blanco = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

// relleno es el interior de la casilla: el color del calendario cuando está marcada y
// nada cuando no (el `accent-color` nativo con esquema claro).
func (f *FilaCalendario) relleno() color.NRGBA {
	if f.Marcada {
		return f.Color
	}
	return blanco
}

type filaCalRenderer struct {
	f       *FilaCalendario
	fondo   *canvas.Rectangle
	casilla *ui.Caja
	aro     *ui.Aro
	tic     *iconos.Dibujo
	nombre  *canvas.Text
	globo   *iconos.Dibujo
	permiso *insigniaPermiso
	kebab   *ui.AccionLateral
	objetos []fyne.CanvasObject
}

func (r *filaCalRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, AltoFilaCalendario())
}
func (r *filaCalRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaCalRenderer) Destroy()                     {}

// Layout coloca casilla, nombre y lo de la derecha. La fila propia es `pr-1` con el kebab
// al final y la compartida `pr-2` con la insignia; entre ellos y el `<label>`, `gap-1`, y
// dentro del `<label>`, `gap-2.5` hasta el globo de las suscripciones.
//
// El kebab OCUPA su sitio aunque no se vea (`opacity-0` no lo saca del flujo): el nombre
// se recorta igual con el ratón fuera que encima, y no baila al pasar.
func (r *filaCalRenderer) Layout(t fyne.Size) {
	f := r.f
	r.fondo.Resize(t)
	centro := t.Height / 2
	x := espacio2p5
	casilla := fyne.NewSize(LadoCasillaCalendario, LadoCasillaCalendario)
	ui.MoverCaja(r.casilla, fyne.NewPos(x, centro-LadoCasillaCalendario/2), casilla)
	r.aro.ColocarEn(fyne.NewPos(x, centro-LadoCasillaCalendario/2), casilla)
	if r.tic != nil {
		r.tic.Colocar(x+1, centro-LadoTicCasilla/2, LadoTicCasilla)
	}
	x += LadoCasillaCalendario + HuecoFilaCalendario

	der := t.Width - tema.Espacio(1)
	if r.permiso != nil {
		der = t.Width - espacio2
	}
	if r.kebab != nil {
		der -= LadoKebab
		r.kebab.Move(fyne.NewPos(der, centro-LadoKebab/2))
		r.kebab.Resize(fyne.NewSize(LadoKebab, LadoKebab))
		der -= tema.Espacio(1)
	}
	if r.permiso != nil {
		m := r.permiso.medida()
		der -= m.Width
		r.permiso.colocar(fyne.NewPos(der, centro-m.Height/2))
		der -= tema.Espacio(1)
	}
	if r.globo != nil {
		der -= 14
		r.globo.Colocar(der, centro-7, 14)
		der -= HuecoFilaCalendario
	}
	ui.Truncar(r.nombre, f.Nombre, tema.TextoBase, ui.Medio, max(der-x, 0))
	ui.ColocarTexto(r.nombre, tema.TextoBase, x, centro-tema.TextoBase.Linea/2)
}

// Refresh pinta el `hover:bg-sidebar-hover` y el estado de la casilla.
func (r *filaCalRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if f.Encima() {
		r.fondo.FillColor = tema.Color("sidebar-hover-bg", v)
	} else {
		r.fondo.FillColor = color.NRGBA{}
	}
	r.fondo.Refresh()
	r.casilla.Pintar(f.relleno(), f.Color)
	r.aro.Pintar(v)
	r.aro.Ver(f.Enfocado())
	r.nombre.Color = tema.Color("sidebar-fg", v)
	r.nombre.Refresh()
	if r.kebab != nil {
		// `opacity-0 group-hover:opacity-100 group-focus-within:opacity-100`, y siempre
		// visible con puntero grueso (`pointer-coarse:opacity-100`): sin ratón que pase
		// por encima era inalcanzable.
		r.kebab.Hidden = !f.Encima() && !f.Enfocado() && !r.kebab.Encima() && !r.kebab.Enfocado() && !ui.EsTactil()
		r.kebab.Refresh()
	}
	if r.permiso != nil {
		r.permiso.pintar(v, f.Encima())
	}
}

// insigniaPermiso es la píldora de «Compartidos conmigo»: `text-2xs rounded-full px-1.5
// py-0.5 font-semibold`. NO es `ui.Insignia`: va sobre el cromo oscuro y sus dos tonos
// son del menú —el de editar RELLENO con `sidebar-hover`, el de ver solo con un BORDE
// de `sidebar-fg-muted` al 40 %—.
//
// El borde de la de ver SUMA: es un `<span>` en línea, y ahí el borde va por fuera del
// relleno, así que mide dos píxeles más de ancho y de alto que la de editar.
type insigniaPermiso struct {
	texto  string
	editar bool
	caja   *ui.Caja
	rotulo *canvas.Text
}

func nuevaInsigniaPermiso(texto string, editar bool) *insigniaPermiso {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	i := &insigniaPermiso{texto: texto, editar: editar}
	// La de editar no lleva borde; la de ver, sí. Se decide al construir la caja, que es
	// cuando `ui.Caja` decide si pinta su rectángulo de fuera.
	borde := color.NRGBA{}
	if !editar {
		borde.A = 0xff
	}
	i.caja = ui.NuevaCaja(color.NRGBA{}, borde, tema.RadioPill)
	i.rotulo = ui.Texto(texto, tema.Texto2XS, ui.SemiNegrita, tema.Color("sidebar-fg", v))
	i.pintar(v, false)
	return i
}

func (i *insigniaPermiso) objetos() []fyne.CanvasObject {
	return append(i.caja.Objetos(), i.rotulo)
}

// medida es la ÚNICA cuenta de lo que ocupa.
func (i *insigniaPermiso) medida() fyne.Size {
	m := fyne.NewSize(ui.Medir(i.texto, tema.Texto2XS, ui.SemiNegrita).Width+2*espacio1p5,
		tema.Texto2XS.Linea+2*espacio0p5)
	if !i.editar {
		m = m.AddWidthHeight(2*tema.Borde, 2*tema.Borde)
	}
	return m
}

func (i *insigniaPermiso) colocar(p fyne.Position) {
	m := i.medida()
	ui.MoverCaja(i.caja, p, m)
	ancho := ui.Medir(i.texto, tema.Texto2XS, ui.SemiNegrita).Width
	ui.ColocarTexto(i.rotulo, tema.Texto2XS, p.X+(m.Width-ancho)/2, p.Y+(m.Height-tema.Texto2XS.Linea)/2)
}

// pintar pone los dos tonos. `encima` es si la FILA está bajo el puntero: la de ver no
// tiene relleno, y como `ui.Caja` pinta el borde como un rectángulo lleno debajo del
// relleno, su interior se pinta del fondo que tiene la fila en ese momento —el del
// menú o, al pasar, el `sidebar-hover` compuesto sobre él—, y su borde, compuesto igual.
func (i *insigniaPermiso) pintar(v tema.Variante, encima bool) {
	fila := ui.FondoCarril(v)
	if encima {
		fila = tokens.SobreSRGB(tema.Color("sidebar-hover-bg", v), fila)
	}
	if i.editar {
		i.caja.Pintar(tokens.SobreSRGB(tema.Color("sidebar-hover-bg", v), fila), color.NRGBA{})
		i.rotulo.Color = tema.Color("sidebar-fg", v)
	} else {
		// `border-sidebar-fg-muted/40`: el token al 40 % de su propio alfa.
		borde := tema.Color("sidebar-fg-muted", v)
		borde.A = uint8(float32(borde.A)*0.4 + 0.5)
		i.caja.Pintar(fila, tokens.SobreSRGB(borde, fila))
		i.rotulo.Color = tema.Color("sidebar-fg-muted", v)
	}
	i.rotulo.Refresh()
}
