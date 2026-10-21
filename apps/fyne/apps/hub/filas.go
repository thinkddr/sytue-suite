// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Las filas del Hub. NO son `ui.FilaLista`, que es el `ListItem` de `List.tsx`
// (`border-l-2 px-3 py-2`, con galón y casilla): estas son las tres filas propias del
// portal, cada una con su marcado, y por eso miden distinto.

// ---- Acceso rápido ----

// Medidas del acceso rápido, medidas en producción (`inicio.ancho.json`, 8 filas):
//
//	fila     368,67 × 42 · `rounded-md px-3 py-2 gap-3`, hueco 3,5 entre filas
//	caja     28 × 28 · `h-8 w-8 rounded-md border bg-surface`
//	glifo    16 · trazo 2
//	rótulo   12,25/19,25 · 500 · `truncate`
//
// 42 = 14 de `py-2` + max(28, 19,25).
const (
	// MargenAcceso es el `px-3` y AltoAcceso el `py-2`.
	MargenAcceso float32 = 3 * 0.25 * tema.Rem
	AltoAcceso   float32 = 2 * 0.25 * tema.Rem
	// HuecoAcceso es el `gap-3` entre la caja del glifo y el rótulo.
	HuecoAcceso float32 = 3 * 0.25 * tema.Rem
	// CajaGlifo es el `h-8 w-8` y LadoGlifo el `size={16}` del dibujo de dentro.
	CajaGlifo  float32 = 8 * 0.25 * tema.Rem
	LadoGlifo  float32 = 16
	SepAccesos float32 = 1 * 0.25 * tema.Rem // `gap-1` de la columna
)

// MedidaAcceso es lo que ocupa una fila de acceso rápido.
func MedidaAcceso() float32 { return 2*AltoAcceso + max(CajaGlifo, tema.TextoBase.Linea) }

// GlifoAcceso es de dónde sale el dibujo de un acceso rápido. Son las dos fuentes
// que usa la web en la MISMA lista: la lupa es `<Icon name="search">`, del catálogo
// compartido, y los otros siete son trazados que `Launcher.tsx` declara él mismo
// porque no están en `packages/ui`. Copiar la lupa aquí sería la segunda copia del
// mismo dibujo, y las copias divergen — lo dice el propio comentario de la web.
type GlifoAcceso struct {
	// Icono es del catálogo compartido; `Ninguno` manda usar Ruta.
	Icono iconos.Icono
	// Ruta es el trazado propio del Hub, verbatim de `Launcher.tsx`.
	Ruta string
}

// dibujo construye el glifo, venga de donde venga.
func (g GlifoAcceso) dibujo(c color.NRGBA) *iconos.Dibujo {
	if g.Icono != iconos.Ninguno {
		return iconos.TrazoConTinta(g.Icono, LadoGlifo, c)
	}
	return iconos.TrazoDeRuta(g.Ruta, LadoGlifo, c)
}

// FilaAcceso es un acceso rápido cross-app del lanzador.
type FilaAcceso struct {
	ui.Pulsable

	Rotulo string
	Glifo  GlifoAcceso
	// Tono es el color de grupo del icono: `CardIcon` sobre relleno sólido, uno por
	// acceso y por turno. No dice nada del acceso; lo separa del de al lado.
	Tono ui.TonoTarjeta
}

// tonosAcceso son los `TONOS_ACCESO` de Launcher.tsx: los cuatro colores de grupo de la
// tarjeta de recurso, por turno.
var tonosAcceso = [...]ui.TonoTarjeta{ui.TarjetaAzul, ui.TarjetaVioleta, ui.TarjetaMenta, ui.TarjetaAmbar}

var _ fyne.Widget = (*FilaAcceso)(nil)

// NuevaFilaAcceso crea el acceso.
func NuevaFilaAcceso(rotulo string, g GlifoAcceso, alPulsar func()) *FilaAcceso {
	f := &FilaAcceso{Rotulo: rotulo, Glifo: g}
	f.Nombre = rotulo
	f.AlPulsar = alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityLabel es el rótulo, que es lo único que dice la web.
func (f *FilaAcceso) AccessibilityLabel() string { return f.Rotulo }

// CreateRenderer compone fondo, caja del glifo, glifo y rótulo.
func (f *FilaAcceso) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &accesoRenderer{
		f:      f,
		fondo:  canvas.NewRectangle(f.fondo(v)),
		aro:    ui.NuevoAro(tema.RadioMD, ui.SinHueco, v).Abierto(),
		caja:   ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioMD),
		glifo:  f.Glifo.dibujo(color.NRGBA{}).AcentoTinta(), // sobre relleno de color: `currentColor`
		rotulo: ui.Texto(f.Rotulo, tema.TextoBase, ui.Medio, tema.Color("text", v)),
	}
	r.fondo.CornerRadius = tema.RadioMD
	r.objetos = append(r.aro.Objetos(), r.fondo)
	r.objetos = append(r.objetos, r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.glifo.Objeto(), r.rotulo)
	r.Refresh()
	return r
}

// fondo es `hover:bg-bg-subtle focus-visible:bg-bg-subtle`.
func (f *FilaAcceso) fondo(v tema.Variante) color.NRGBA {
	if f.Encima() || f.Enfocado() {
		return tema.Color("bg-subtle", v)
	}
	return color.NRGBA{}
}

type accesoRenderer struct {
	f       *FilaAcceso
	fondo   *canvas.Rectangle
	aro     *ui.Aro
	caja    *ui.Caja
	glifo   *iconos.Dibujo
	rotulo  *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *accesoRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, MedidaAcceso()) }
func (r *accesoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *accesoRenderer) Destroy()                     {}

// Layout coloca la caja del glifo y el rótulo, centrados (`items-center`).
func (r *accesoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Resize(t)
	x := MargenAcceso
	ui.MoverCaja(r.caja, fyne.NewPos(x, (t.Height-CajaGlifo)/2), fyne.NewSize(CajaGlifo, CajaGlifo))
	r.glifo.Colocar(x+(CajaGlifo-LadoGlifo)/2, (t.Height-LadoGlifo)/2, LadoGlifo)
	x += CajaGlifo + HuecoAcceso
	ui.Truncar(r.rotulo, r.f.Rotulo, tema.TextoBase, ui.Medio, t.Width-x-MargenAcceso)
	ui.ColocarTexto(r.rotulo, tema.TextoBase, x, (t.Height-tema.TextoBase.Linea)/2)
}

// Refresh repinta el estado del puntero y del foco.
func (r *accesoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado())
	// `CardIcon`: el glifo sobre el relleno sólido de su color de grupo, sin borde.
	relleno, tinta := ui.TonoSolido(r.f.Tono, v)
	r.fondo.FillColor = r.f.fondo(v)
	r.fondo.Refresh()
	r.caja.Pintar(relleno, color.NRGBA{})
	r.glifo.TenirCon(tinta)
	r.rotulo.Color = tema.Color("text", v)
	r.rotulo.Refresh()
}

// ---- Fila de actividad ----

// Las dos filas de actividad del Hub son la MISMA información con dos marcados
// distintos, y miden distinto. No se unifican en una con parámetros porque lo que
// cambia no es un número: cambian el relleno, la tipografía de las dos líneas y si la
// meta va a la derecha o debajo.
//
//	lanzador   `flex items-start gap-3 px-4 py-3`, marca 20 con `mt-0.5`,
//	           título 14/19,25 500, resumen 11,375/15,75, meta en columna a la
//	           derecha. Medido: 56 de alto (57 con su filete).
//	actividad  `flex items-start gap-3 rounded-md px-3 py-2.5`, marca 18 con
//	           `mt-0.5`, título y resumen en `text-sm` con `mt-0.5` entre ellos,
//	           hora a la derecha en `text-xs`. Medido: 50,75.
const (
	MargenReciente float32 = 4 * 0.25 * tema.Rem   // `px-4`
	AltoReciente   float32 = 3 * 0.25 * tema.Rem   // `py-3`
	HuecoReciente  float32 = 3 * 0.25 * tema.Rem   // `gap-3`
	SangriaMarca   float32 = 0.5 * 0.25 * tema.Rem // `mt-0.5`
	HuecoMeta      float32 = 0.5 * 0.25 * tema.Rem // `gap-0.5` de la columna de la derecha
	// LadoPunto es el `h-2.5 w-2.5` del punto que sustituye a la marca cuando el
	// origen no está en el catálogo.
	LadoPunto float32 = 2.5 * 0.25 * tema.Rem
	// SangriaPunto es el `mt-1.5` del punto, que baja algo más que la tesela porque
	// es más pequeño.
	SangriaPunto float32 = 1.5 * 0.25 * tema.Rem
)

// altoReciente es el `py-3` de la fila y, en la forma Marco, su `marco:py-2`.
func altoReciente() float32 {
	if tema.EsMarco() {
		return 2 * 0.25 * tema.Rem
	}
	return AltoReciente
}

// MedidaReciente es el alto de una fila de «Reciente en toda la suite».
func MedidaReciente(conResumen bool) float32 {
	texto := tema.TextoRaiz.Linea
	if conResumen {
		texto += tema.TextoSM.Linea
	}
	return 2*altoReciente() + max(texto, SangriaMarca+iconos.MarcaXS)
}

// FilaReciente es una fila de la tarjeta «Reciente en toda la suite».
type FilaReciente struct {
	ui.Pulsable

	Titulo  string
	Resumen string
	// Origen es el nombre corto del producto, ya resuelto; Marca es su tesela, y
	// Conocido dice si el catálogo sabe de él (si no, va un punto).
	Origen   string
	Marca    iconos.Producto
	Conocido bool
	// Cuando es la antigüedad ya formateada; vacía, no se pinta.
	Cuando string
}

var _ fyne.Widget = (*FilaReciente)(nil)

// NuevaFilaReciente crea la fila.
func NuevaFilaReciente(titulo, resumen string, alPulsar func()) *FilaReciente {
	f := &FilaReciente{Titulo: titulo, Resumen: resumen}
	f.Nombre = titulo
	f.AlPulsar = alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityLabel junta título y origen, que es lo que la web deja para el lector
// de pantalla con el `<span class="sr-only">`.
func (f *FilaReciente) AccessibilityLabel() string {
	if f.Origen == "" {
		return f.Titulo
	}
	return f.Titulo + ". " + f.Origen
}

// CreateRenderer compone la fila.
func (f *FilaReciente) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &recienteRenderer{
		f:      f,
		fondo:  canvas.NewRectangle(f.fondo(v)),
		aro:    ui.NuevoAro(0, ui.SinHueco, v).Abierto(),
		titulo: ui.Texto(f.Titulo, tema.TextoRaiz, ui.Medio, tema.Color("text", v)),
		origen: ui.Texto(f.Origen, tema.TextoXS, ui.Medio, tema.Color("text-muted", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.fondo)
	if f.Conocido {
		r.marca = iconos.Marca(f.Marca, iconos.MarcaXS, v)
		r.objetos = append(r.objetos, r.marca.Objeto())
	} else {
		// `bg-primary … rounded-full`: el punto de los orígenes sin tesela.
		r.punto = canvas.NewRectangle(tema.Color("primary", v))
		r.punto.CornerRadius = LadoPunto / 2
		r.objetos = append(r.objetos, r.punto)
	}
	r.objetos = append(r.objetos, r.titulo, r.origen)
	if f.Resumen != "" {
		r.resumen = ui.Texto(f.Resumen, tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
		r.objetos = append(r.objetos, r.resumen)
	}
	if f.Cuando != "" {
		r.cuando = ui.Texto(f.Cuando, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
		r.objetos = append(r.objetos, r.cuando)
	}
	return r
}

func (f *FilaReciente) fondo(v tema.Variante) color.NRGBA {
	if f.Encima() || f.Enfocado() {
		return tema.Color("bg-subtle", v)
	}
	return color.NRGBA{}
}

type recienteRenderer struct {
	f                       *FilaReciente
	fondo, punto            *canvas.Rectangle
	aro                     *ui.Aro
	marca                   *iconos.Dibujo
	titulo, resumen         *canvas.Text
	origen, cuando          *canvas.Text
	objetos                 []fyne.CanvasObject
	anchoOrigen, anchoDesde float32
}

func (r *recienteRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, MedidaReciente(r.f.Resumen != ""))
}

func (r *recienteRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *recienteRenderer) Destroy()                     {}

// Layout reparte la fila: marca, columna de texto elástica y columna de meta a la
// derecha, alineada a su borde (`items-end`).
func (r *recienteRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Resize(t)
	x := MargenReciente
	y := altoReciente()
	if r.marca != nil {
		r.marca.Colocar(x, y+SangriaMarca, iconos.MarcaXS)
	} else {
		// `mt-1.5`: el punto baja algo más que la tesela, porque es más pequeño.
		r.punto.Move(fyne.NewPos(x, y+SangriaPunto))
		r.punto.Resize(fyne.NewSize(LadoPunto, LadoPunto))
	}
	x += iconos.MarcaXS + HuecoReciente

	// La columna de la derecha se mide primero: es la que decide cuánto le queda al
	// título, igual que un `shrink-0` de flexbox.
	r.anchoOrigen = ui.Medir(r.f.Origen, tema.TextoXS, ui.Medio).Width
	r.anchoDesde = 0
	if r.cuando != nil {
		r.anchoDesde = ui.Medir(r.f.Cuando, tema.TextoXS, ui.Normal).Width
	}
	meta := max(r.anchoOrigen, r.anchoDesde)
	der := t.Width - MargenReciente
	ui.ColocarTexto(r.origen, tema.TextoXS, der-r.anchoOrigen, y)
	if r.cuando != nil {
		ui.ColocarTexto(r.cuando, tema.TextoXS, der-r.anchoDesde, y+tema.TextoXS.Linea+HuecoMeta)
	}

	cabe := max(der-meta-HuecoReciente-x, 0)
	ui.Truncar(r.titulo, r.f.Titulo, tema.TextoRaiz, ui.Medio, cabe)
	ui.ColocarTexto(r.titulo, tema.TextoRaiz, x, y)
	if r.resumen != nil {
		ui.Truncar(r.resumen, r.f.Resumen, tema.TextoSM, ui.Normal, cabe)
		ui.ColocarTexto(r.resumen, tema.TextoSM, x, y+tema.TextoRaiz.Linea)
	}
}

// Refresh repinta con el estado del puntero.
func (r *recienteRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = r.f.fondo(v)
	r.fondo.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado())
	r.titulo.Color = tema.Color("text", v)
	r.titulo.Refresh()
	r.origen.Color = tema.Color("text-muted", v)
	r.origen.Refresh()
	if r.resumen != nil {
		r.resumen.Color = tema.Color("text-muted", v)
		r.resumen.Refresh()
	}
	if r.cuando != nil {
		r.cuando.Color = tema.Color("text-subtle", v)
		r.cuando.Refresh()
	}
}

// ---- Fila de la pantalla de actividad ----

// Medidas de la fila de `/actividad`, medidas en producción (50,75 de alto, nueve
// grupos): `rounded-md px-3 py-2.5 gap-3`, marca de 18 con `mt-0.5`, título y resumen
// en `text-sm` separados por `mt-0.5`, y la antigüedad a la derecha en `text-xs`.
//
// 50,75 = 17,5 de `py-2.5` + max(18 + 1,75 · 15,75 + 1,75 + 15,75).
const (
	MargenActividad float32 = 3 * 0.25 * tema.Rem   // `px-3`
	AltoActividad   float32 = 2.5 * 0.25 * tema.Rem // `py-2.5`
	HuecoActividad  float32 = 3 * 0.25 * tema.Rem   // `gap-3`
	SangriaResumen  float32 = 0.5 * 0.25 * tema.Rem // `mt-0.5` entre título y resumen
	// LadoMarcaActividad es el `<ProductIcon size={18} />`. No es una talla del
	// catálogo de marcas —que tiene 20, 24, 28 y 40— porque en la web tampoco:
	// esta pantalla pide 18 a mano.
	LadoMarcaActividad float32 = 18
)

// MedidaActividad es el alto de una fila de la pantalla de actividad.
func MedidaActividad(conResumen bool) float32 {
	texto := tema.TextoSM.Linea
	if conResumen {
		texto += SangriaResumen + tema.TextoSM.Linea
	}
	return 2*AltoActividad + max(texto, SangriaMarca+LadoMarcaActividad)
}

// FilaActividad es una fila de `/actividad`.
type FilaActividad struct {
	ui.Pulsable

	Titulo   string
	Resumen  string
	Origen   string
	Marca    iconos.Producto
	Conocido bool
	Cuando   string
}

var _ fyne.Widget = (*FilaActividad)(nil)

// NuevaFilaActividad crea la fila.
func NuevaFilaActividad(titulo, resumen string, alPulsar func()) *FilaActividad {
	f := &FilaActividad{Titulo: titulo, Resumen: resumen}
	f.Nombre = titulo
	f.AlPulsar = alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// AccessibilityLabel junta título y origen: el origen va en un `sr-only` en la web,
// así que en pantalla no se ve pero se anuncia.
func (f *FilaActividad) AccessibilityLabel() string {
	if f.Origen == "" {
		return f.Titulo
	}
	return f.Titulo + ". " + f.Origen
}

// CreateRenderer compone la fila.
func (f *FilaActividad) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &actividadFilaRenderer{
		f:      f,
		fondo:  canvas.NewRectangle(f.fondo(v)),
		aro:    ui.NuevoAro(tema.RadioMD, ui.SinHueco, v).Abierto(),
		titulo: ui.Texto(f.Titulo, tema.TextoSM, ui.Medio, tema.Color("text", v)),
	}
	r.fondo.CornerRadius = tema.RadioMD
	r.objetos = append(r.aro.Objetos(), r.fondo)
	if f.Conocido {
		r.marca = iconos.Marca(f.Marca, LadoMarcaActividad, v)
		r.objetos = append(r.objetos, r.marca.Objeto())
	}
	r.objetos = append(r.objetos, r.titulo)
	if f.Resumen != "" {
		r.resumen = ui.Texto(f.Resumen, tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
		r.objetos = append(r.objetos, r.resumen)
	}
	if f.Cuando != "" {
		r.cuando = ui.Texto(f.Cuando, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
		r.objetos = append(r.objetos, r.cuando)
	}
	return r
}

// fondo es `hover:bg-chrome-hover`, que NO es el `bg-bg-subtle` de las filas del
// lanzador: son dos tokens distintos y aquí manda el del cromo.
func (f *FilaActividad) fondo(v tema.Variante) color.NRGBA {
	if f.Encima() || f.Enfocado() {
		return tema.Color("chrome-hover", v)
	}
	return color.NRGBA{}
}

type actividadFilaRenderer struct {
	f               *FilaActividad
	fondo           *canvas.Rectangle
	aro             *ui.Aro
	marca           *iconos.Dibujo
	titulo, resumen *canvas.Text
	cuando          *canvas.Text
	objetos         []fyne.CanvasObject
}

func (r *actividadFilaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, MedidaActividad(r.f.Resumen != ""))
}

func (r *actividadFilaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *actividadFilaRenderer) Destroy()                     {}

// Layout coloca marca, columna de texto y la antigüedad a la derecha.
func (r *actividadFilaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Resize(t)
	x, y := MargenActividad, AltoActividad
	if r.marca != nil {
		r.marca.Colocar(x, y+SangriaMarca, LadoMarcaActividad)
	}
	x += LadoMarcaActividad + HuecoActividad

	der := t.Width - MargenActividad
	if r.cuando != nil {
		ancho := ui.Medir(r.f.Cuando, tema.TextoXS, ui.Normal).Width
		ui.ColocarTexto(r.cuando, tema.TextoXS, der-ancho, y)
		der -= ancho + HuecoActividad
	}
	cabe := max(der-x, 0)
	ui.Truncar(r.titulo, r.f.Titulo, tema.TextoSM, ui.Medio, cabe)
	ui.ColocarTexto(r.titulo, tema.TextoSM, x, y)
	if r.resumen != nil {
		ui.Truncar(r.resumen, r.f.Resumen, tema.TextoSM, ui.Normal, cabe)
		ui.ColocarTexto(r.resumen, tema.TextoSM, x, y+tema.TextoSM.Linea+SangriaResumen)
	}
}

// Refresh repinta con el estado del puntero.
func (r *actividadFilaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = r.f.fondo(v)
	r.fondo.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado())
	r.titulo.Color = tema.Color("text", v)
	r.titulo.Refresh()
	if r.resumen != nil {
		r.resumen.Color = tema.Color("text-muted", v)
		r.resumen.Refresh()
	}
	if r.cuando != nil {
		r.cuando.Color = tema.Color("text-subtle", v)
		r.cuando.Refresh()
	}
}

// ---- Fila de resultado de búsqueda ----

// Medidas del resultado (`flex flex-col gap-0.5 px-4 py-3`): título de 14/19,25 en
// `font-medium` y resumen de `text-sm`, apilados. Van dentro de una lista con borde
// y `divide-y`, así que la fila no lleva borde propio.
const (
	MargenBusquedaFila float32 = 4 * 0.25 * tema.Rem
	AltoBusquedaFila   float32 = 3 * 0.25 * tema.Rem
	HuecoBusquedaFila  float32 = 0.5 * 0.25 * tema.Rem
)

// MedidaResultado es el alto de un resultado.
func MedidaResultado(conResumen bool) float32 {
	alto := 2*AltoBusquedaFila + tema.TextoRaiz.Linea
	if conResumen {
		alto += HuecoBusquedaFila + tema.TextoSM.Linea
	}
	return alto
}

// FilaResultado es un resultado de la búsqueda federada.
type FilaResultado struct {
	ui.Pulsable

	Titulo  string
	Resumen string
}

var _ fyne.Widget = (*FilaResultado)(nil)

// NuevaFilaResultado crea el resultado.
func NuevaFilaResultado(titulo, resumen string, alPulsar func()) *FilaResultado {
	f := &FilaResultado{Titulo: titulo, Resumen: resumen}
	f.Nombre = titulo
	f.AlPulsar = alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// CreateRenderer compone la fila.
func (f *FilaResultado) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &resultadoRenderer{
		f:      f,
		fondo:  canvas.NewRectangle(f.fondo(v)),
		aro:    ui.NuevoAro(0, ui.SinHueco, v).Abierto(),
		titulo: ui.Texto(f.Titulo, tema.TextoRaiz, ui.Medio, tema.Color("text", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.fondo, r.titulo)
	if f.Resumen != "" {
		r.resumen = ui.Texto(f.Resumen, tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
		r.objetos = append(r.objetos, r.resumen)
	}
	return r
}

func (f *FilaResultado) fondo(v tema.Variante) color.NRGBA {
	if f.Encima() || f.Enfocado() {
		return tema.Color("bg-subtle", v)
	}
	return color.NRGBA{}
}

type resultadoRenderer struct {
	f               *FilaResultado
	fondo           *canvas.Rectangle
	aro             *ui.Aro
	titulo, resumen *canvas.Text
	objetos         []fyne.CanvasObject
}

func (r *resultadoRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, MedidaResultado(r.f.Resumen != ""))
}

func (r *resultadoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *resultadoRenderer) Destroy()                     {}

// Layout apila título y resumen.
func (r *resultadoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Resize(t)
	cabe := t.Width - 2*MargenBusquedaFila
	ui.Truncar(r.titulo, r.f.Titulo, tema.TextoRaiz, ui.Medio, cabe)
	ui.ColocarTexto(r.titulo, tema.TextoRaiz, MargenBusquedaFila, AltoBusquedaFila)
	if r.resumen != nil {
		ui.Truncar(r.resumen, r.f.Resumen, tema.TextoSM, ui.Normal, cabe)
		ui.ColocarTexto(r.resumen, tema.TextoSM, MargenBusquedaFila,
			AltoBusquedaFila+tema.TextoRaiz.Linea+HuecoBusquedaFila)
	}
}

// Refresh repinta con el estado del puntero.
func (r *resultadoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = r.f.fondo(v)
	r.fondo.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado())
	r.titulo.Color = tema.Color("text", v)
	r.titulo.Refresh()
	if r.resumen != nil {
		r.resumen.Color = tema.Color("text-muted", v)
		r.resumen.Refresh()
	}
}
