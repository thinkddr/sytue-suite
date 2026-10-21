// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Table.tsx

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	temafyne "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Tabla es `packages/ui/src/components/Table.tsx`: la tabla densa de la suite, con
// cabecera, orden, selección por fila y fila de totales.
//
// # El cuerpo va en un widget.List, y no es una preferencia
//
// Medido en este repositorio con piezas compuestas de primitivas: dentro de un
// `widget.List` el pintado es PLANO —9,6 / 8,7 / 9,3 ms con 50, 1.000 y 5.000 filas—
// y en un contenedor llano sube a 1.115 ms con 5.000. La virtualización de Fyne es la
// que hay que usar; escribir otra cuesta dos órdenes de magnitud.
//
// # El hueco que `widget.List` mete entre filas
//
// `listLayout` separa las filas por `theme.Size(SizeNamePadding)` —el del tema, no el
// del separador, y también con `HideSeparators`—. En el tema de la suite eso son 7 px:
// siete por fila, o sea 35.000 en una tabla de cinco mil. Por eso la lista va envuelta
// en un `container.ThemeOverride` con un tema que devuelve 0 en ese tamaño y deja el
// resto igual. Lo vigila `TestLaListaNoMeteHuecoEntreFilas`.

// Alineacion es el `align` de una columna de Table.tsx.
type Alineacion uint8

// Las tres alineaciones. El valor cero es `start`, como en la web.
const (
	Inicio Alineacion = iota
	Centro
	Fin
)

// Medidas de la tabla, en px de CSS con el rem de la suite.
const (
	// MargenCelda es `px-3` y AltoCelda es `py-2.5`, los dos en cabecera, cuerpo y
	// pie. Ojo: la lista de Drive usa `py-2`, que NO es esto.
	MargenCelda float32 = 3 * 0.25 * tema.Rem
	AltoCelda   float32 = 2.5 * 0.25 * tema.Rem
	// ColCasillaTabla es el `w-10` de la columna de casillas.
	ColCasillaTabla float32 = 10 * 0.25 * tema.Rem
	// HuecoOrden es el `gap-1` entre el rótulo y la flecha de orden.
	HuecoOrden float32 = 1 * 0.25 * tema.Rem
	// TamFlechaOrden es el `size={14}` del `Icon` de la cabecera.
	TamFlechaOrden float32 = 14
	// AltoVacio es `py-10` más la caja de una línea de `text-base`.
)

// AltoVacio es `py-10` más la caja de una línea de `text-base`. Es var y no const
// porque la caja de línea la declara `tema` como variable.
var AltoVacio = 2*(10*0.25*tema.Rem) + tema.TextoBase.Linea

// AltoCabeceraTabla es `py-2.5` más la caja de `.sytue-overline` (`text-xs`, 1rem)
// y el `border-b` de abajo. Sigue la línea tipográfica cuando cambia la raíz.
var AltoCabeceraTabla = 2*AltoCelda + tema.TextoXS.Linea + tema.Borde

// AltoFilaTabla() es `py-2.5` más la caja de `text-base` y el `border-b`.
//
// El número sale de Table.tsx (`px-3 py-2.5 text-base` en el `td`) y no de esta
// implementación: 2,5 × 0.25rem por lado y 1,375rem de caja.
//
// Es una función porque el relleno es de la FORMA: en Marco la fila lleva un píxel más
// por arriba y por abajo (`py-[calc(0.625rem + var(--sytue-row-grow))]`).
func AltoFilaTabla() float32 {
	return 2*(AltoCelda+tema.Forma("row-grow")) + tema.TextoBase.Linea + tema.Borde
}

// ColumnaTabla es una columna de Table.tsx.
type ColumnaTabla struct {
	// Clave es la que se le pasa a AlOrdenar.
	Clave string
	// Rotulo es la cabecera. Se pinta en versalitas (`sytue-overline`) y en negrita.
	Rotulo string
	// Ordenable enseña la flecha y hace pulsable la cabecera.
	Ordenable bool
	// Alineacion del contenido de la celda.
	Alineacion Alineacion
	// Ancho fija el ancho de la columna (el `width` de la web). Con 0, la columna
	// entra en el reparto del sitio que sobra.
	Ancho float32
}

// Tabla es el widget.
type Tabla struct {
	widget.BaseWidget

	Columnas []ColumnaTabla
	// Filas devuelve cuántas hay; Celda, el texto de una. Son callbacks y no una
	// rebanada porque el cuerpo está virtualizado: de cinco mil filas se piden las
	// veinte que se ven.
	Filas func() int
	Celda func(fila, columna int) string
	// CeldaObjeto es la celda que NO es texto: una insignia de estado, un avatar, un
	// icono. Devolver nil —o dejar el gancho a nil— deja la celda en `Celda`, que es
	// el caso normal y el barato.
	//
	// Existe porque la web sí pinta objetos en el `td` (`<Badge tone=…>` para el
	// estado de una factura) y sin gancho la única salida era escribir la tabla otra
	// vez en la app, o pintar el estado como texto plano —que es lo que hacía el ERP,
	// con su `ponytail:` pidiendo justo esto—.
	//
	// El objeto se coloca a su `MinSize`, recortado al ancho de la columna y centrado
	// en vertical, y sigue la `Alineacion` de su columna igual que el texto. Se pide
	// en cada `Refresh` de la fila —o sea, en cada reciclaje del `widget.List`—, así
	// que tiene que ser barato: construir un widget, no una consulta.
	CeldaObjeto func(fila, columna int) fyne.CanvasObject

	// Ordenada es la Clave de la columna por la que está ordenado, y Descendente su
	// sentido.
	Ordenada    string
	Descendente bool
	AlOrdenar   func(clave string)

	// Marcada dice si una fila está seleccionada; con nil no hay columna de
	// casillas, igual que en la web (`selectable = Boolean(onToggleRow)`).
	Marcada  func(fila int) bool
	AlMarcar func(fila int)

	// Resumen es el `tfoot`: un texto por columna. Con nil no se pinta pie.
	Resumen []string
	// Vacia es el texto de «sin resultados».
	Vacia string
}

var _ fyne.Widget = (*Tabla)(nil)

// NuevaTabla crea la tabla.
func NuevaTabla(columnas []ColumnaTabla, filas func() int, celda func(fila, columna int) string) *Tabla {
	t := &Tabla{Columnas: columnas, Filas: filas, Celda: celda, Vacia: textoComun("ui.noResults")}
	t.ExtendBaseWidget(t)
	return t
}

func (t *Tabla) seleccionable() bool { return t.Marcada != nil }

// AnchosColumna reparte el ancho de la tabla entre sus columnas.
//
// El reparto sigue la regla del navegador: se redondea cada BORDE, no cada ancho. Con
// división entera la última columna se queda corta del margen, y eso se ve como una
// raya clara al final de la cabecera.
//
// ponytail: las columnas sin `Ancho` se reparten a partes IGUALES. La web usa
// `table-layout: auto`, que las dimensiona por contenido; clavarlo pide medir el
// máximo de cada columna sobre todas las filas, que en una tabla virtualizada de
// cinco mil es justo lo que la virtualización evita. Quien necesite el reparto por
// contenido pone `Ancho` en la columna, que es lo que la web ya ofrece (`width`).
func (t *Tabla) AnchosColumna(total float32) []float32 {
	anchos := make([]float32, len(t.Columnas))
	sobra, libres := total, 0
	if t.seleccionable() {
		sobra -= ColCasillaTabla
	}
	for i, c := range t.Columnas {
		if c.Ancho > 0 {
			anchos[i] = c.Ancho
			sobra -= c.Ancho
			continue
		}
		libres++
	}
	if libres == 0 {
		return anchos
	}
	paso := float64(max(sobra, 0)) / float64(libres)
	n := 0
	for i, c := range t.Columnas {
		if c.Ancho > 0 {
			continue
		}
		izq := math.Round(float64(n) * paso)
		der := math.Round(float64(n+1) * paso)
		anchos[i] = float32(der - izq)
		n++
	}
	return anchos
}

// CreateRenderer compone contenedor, cabecera, cuerpo y pie.
func (t *Tabla) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &tablaRenderer{
		t: t,
		// `rounded-lg border border-border bg-surface`.
		caja:     NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioLG),
		cabecera: nuevaCabeceraTabla(t),
		vacia:    Texto(t.Vacia, tema.TextoBase, Normal, tema.Color("text-muted", v)),
	}
	lista := widget.NewList(
		func() int { return t.Filas() },
		func() fyne.CanvasObject { return nuevaFilaTabla(t) },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			f, ok := o.(*FilaTabla)
			if !ok {
				return
			}
			f.Poner(i)
		},
	)
	lista.HideSeparators = true
	r.lista = lista
	// El tema sin relleno: ver el comentario del tipo.
	r.cuerpo = container.NewThemeOverride(lista, temaSinRelleno{})
	r.objetos = append(r.caja.Objetos(), r.cabecera, r.cuerpo, r.vacia)
	if t.Resumen != nil {
		r.pie = nuevaPieTabla(t)
		r.objetos = append(r.objetos, r.pie)
	}
	return r
}

// EnvolverLista es cómo se monta CUALQUIER `widget.List` de la suite.
//
// No es una comodidad: `listLayout` separa las filas por `theme.Size(SizeNamePadding)`
// —el del TEMA, no el del separador, y también con `HideSeparators`—, que en el tema
// de la suite son 7 px. Siete por fila son 35.000 en una bandeja de cinco mil. La
// función existe para que ninguna pantalla tenga que acordarse: se le da la lista y
// devuelve lo que hay que colocar. Lo vigila `TestLaListaNoMeteHuecoEntreFilas`.
func EnvolverLista(l *widget.List) fyne.CanvasObject {
	l.HideSeparators = true
	return container.NewThemeOverride(l, temaSinRelleno{})
}

// temaSinRelleno es el tema de la suite con `SizeNamePadding` a cero.
//
// Solo lo usa el cuerpo de la tabla y la lista de Drive: `widget.List` usa ESE tamaño
// como separación entre filas, y con los 7 px del tema de la suite cada fila saldría
// 7 px más abajo que en la web, acumulando.
type temaSinRelleno struct{ tema.Sytue }

// Size devuelve 0 para el relleno y lo del tema de la suite para todo lo demás.
func (s temaSinRelleno) Size(n fyne.ThemeSizeName) float32 {
	if n == temafyne.SizeNamePadding {
		return 0
	}
	return s.Sytue.Size(n)
}

type tablaRenderer struct {
	t        *Tabla
	caja     *Caja
	cabecera *CabeceraTabla
	lista    *widget.List
	cuerpo   fyne.CanvasObject
	pie      *PieTabla
	vacia    *canvas.Text
	objetos  []fyne.CanvasObject
}

// MinSize es el contenedor con su cabecera y una fila: lo que ocupa una tabla con un
// solo resultado. El alto real lo pone quien la coloca, como el `overflow` de la web.
func (r *tablaRenderer) MinSize() fyne.Size {
	alto := 2*tema.Borde + AltoCabeceraTabla + AltoFilaTabla()
	if r.pie != nil {
		alto += AltoFilaTabla()
	}
	return fyne.NewSize(2*tema.Borde+ColCasillaTabla, alto)
}

func (r *tablaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *tablaRenderer) Destroy()                     {}

func (r *tablaRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	dentro := t.Width - 2*tema.Borde
	y := tema.Borde

	r.cabecera.Resize(fyne.NewSize(dentro, AltoCabeceraTabla))
	r.cabecera.Move(fyne.NewPos(tema.Borde, y))
	y += AltoCabeceraTabla

	altoCuerpo := t.Height - tema.Borde - y
	if r.pie != nil {
		altoCuerpo -= AltoFilaTabla()
	}
	altoCuerpo = max(altoCuerpo, 0)

	if r.t.Filas() == 0 {
		// `colSpan` con `px-3 py-10 text-center`.
		r.cuerpo.Hide()
		r.vacia.Show()
		ancho := Medir(r.t.Vacia, tema.TextoBase, Normal).Width
		ColocarTexto(r.vacia, tema.TextoBase, tema.Borde+(dentro-ancho)/2, y+(AltoVacio-tema.TextoBase.Linea)/2)
	} else {
		r.vacia.Hide()
		r.cuerpo.Show()
		r.cuerpo.Resize(fyne.NewSize(dentro, altoCuerpo))
		r.cuerpo.Move(fyne.NewPos(tema.Borde, y))
	}
	y += altoCuerpo

	if r.pie != nil {
		r.pie.Resize(fyne.NewSize(dentro, AltoFilaTabla()))
		r.pie.Move(fyne.NewPos(tema.Borde, y))
	}
}

func (r *tablaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color("surface", v), tema.Color("border", v))
	r.vacia.Text = r.t.Vacia
	r.vacia.Color = tema.Color("text-muted", v)
	r.lista.Refresh()
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// ---- Cabecera ----

// CabeceraTabla es el `thead` de Table.tsx: banda blanca con los rótulos en tinta,
// versalitas y negrita, y su `border-b`.
type CabeceraTabla struct {
	widget.BaseWidget
	t      *Tabla
	encima int // índice de la columna bajo el puntero, -1 si ninguna
}

var (
	_ fyne.Widget       = (*CabeceraTabla)(nil)
	_ fyne.Tappable     = (*CabeceraTabla)(nil)
	_ desktop.Hoverable = (*CabeceraTabla)(nil)
)

func nuevaCabeceraTabla(t *Tabla) *CabeceraTabla {
	c := &CabeceraTabla{t: t, encima: -1}
	c.ExtendBaseWidget(c)
	return c
}

// CreateRenderer compone los rótulos y las flechas.
func (c *CabeceraTabla) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &cabeceraTablaRenderer{
		c:     c,
		base:  canvas.NewRectangle(tema.Color("surface", v)),
		linea: canvas.NewRectangle(tema.Color("border", v)),
	}
	r.objetos = []fyne.CanvasObject{r.base}
	for _, col := range c.t.Columnas {
		// `sytue-overline ... font-bold`: la clase pone 600 y `font-bold` lo sube a
		// 700. Gana la utilidad, que es la que va después en la cascada.
		e := NuevoVersal(col.Rotulo, tema.TextoXS, Negrita, tema.Color("text", v))
		r.rotulos = append(r.rotulos, e)
		r.objetos = append(r.objetos, e.Objetos()...)
		var g *iconos.Dibujo
		if col.Ordenable {
			// `chevronUp` salvo en descendente, que es lo que hace Table.tsx.
			g = iconos.Trazo(iconos.ChevronArriba, TamFlechaOrden, "indicator", v)
			r.objetos = append(r.objetos, g.Objeto())
		}
		r.flechas = append(r.flechas, g)
	}
	r.objetos = append(r.objetos, r.linea)
	r.Refresh()
	return r
}

// Tapped ordena por la columna pulsada.
func (c *CabeceraTabla) Tapped(e *fyne.PointEvent) {
	if c.t.AlOrdenar == nil {
		return
	}
	if i := c.columnaEn(e.Position.X); i >= 0 && c.t.Columnas[i].Ordenable {
		c.t.AlOrdenar(c.t.Columnas[i].Clave)
	}
}

// columnaEn dice qué columna cae bajo una x.
func (c *CabeceraTabla) columnaEn(x float32) int {
	anchos := c.t.AnchosColumna(c.Size().Width)
	borde := float32(0)
	if c.t.seleccionable() {
		borde = ColCasillaTabla
	}
	for i, a := range anchos {
		if x >= borde && x < borde+a {
			return i
		}
		borde += a
	}
	return -1
}

// MouseIn empieza a seguir el puntero.
func (c *CabeceraTabla) MouseIn(e *desktop.MouseEvent) { c.MouseMoved(e) }

// MouseMoved enseña la flecha de la columna que se puede ordenar, que en la web
// aparece al hover al 60 % de opacidad.
func (c *CabeceraTabla) MouseMoved(e *desktop.MouseEvent) {
	if i := c.columnaEn(e.Position.X); i != c.encima {
		c.encima = i
		c.Refresh()
	}
}

// MouseOut esconde las flechas de las columnas sin ordenar.
func (c *CabeceraTabla) MouseOut() { c.encima = -1; c.Refresh() }

type cabeceraTablaRenderer struct {
	c           *CabeceraTabla
	base, linea *canvas.Rectangle
	rotulos     []*Versal
	flechas     []*iconos.Dibujo
	objetos     []fyne.CanvasObject
}

func (r *cabeceraTablaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(ColCasillaTabla, AltoCabeceraTabla)
}

func (r *cabeceraTablaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *cabeceraTablaRenderer) Destroy()                     {}

func (r *cabeceraTablaRenderer) Layout(t fyne.Size) {
	r.base.Resize(t)
	r.base.Move(fyne.NewPos(0, 0))
	r.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
	r.linea.Move(fyne.NewPos(0, t.Height-tema.Borde))

	y := (t.Height - tema.Borde - tema.TextoXS.Linea) / 2
	x := float32(0)
	if r.c.t.seleccionable() {
		// El `th` de la casilla va VACÍO en la web: solo ocupa `w-10`.
		x += ColCasillaTabla
	}
	anchos := r.c.t.AnchosColumna(t.Width)
	for i, col := range r.c.t.Columnas {
		ancho := r.rotulos[i].Ancho()
		if r.flechas[i] != nil {
			ancho += HuecoOrden + TamFlechaOrden
		}
		rx := x + MargenCelda
		switch col.Alineacion {
		case Centro:
			rx = x + (anchos[i]-ancho)/2
		case Fin:
			rx = x + anchos[i] - MargenCelda - ancho
		case Inicio:
		}
		r.rotulos[i].Colocar(rx, y)
		if g := r.flechas[i]; g != nil {
			g.Colocar(rx+r.rotulos[i].Ancho()+HuecoOrden,
				(t.Height-tema.Borde-TamFlechaOrden)/2, TamFlechaOrden)
		}
		x += anchos[i]
	}
}

func (r *cabeceraTablaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.base.FillColor = tema.Color("surface", v)
	r.linea.FillColor = tema.Color("border", v)
	for i, col := range r.c.t.Columnas {
		r.rotulos[i].Pintar(tema.Color("text", v))
		g := r.flechas[i]
		if g == nil {
			continue
		}
		switch {
		case r.c.t.Ordenada == col.Clave:
			// La columna ordenada lleva el color de «estás aquí» y la flecha en el
			// sentido del orden: `chevronDown` SOLO en descendente.
			flecha := iconos.ChevronArriba
			if r.c.t.Descendente {
				flecha = iconos.ChevronAbajo
			}
			g.Cambiar(flecha, tema.Color("indicator", v))
			g.Ver(true)
		case r.c.encima == i:
			// `group-hover/sort:opacity-60` sobre el color del texto.
			g.Cambiar(iconos.ChevronArriba, tokens.ConAlfa(tema.Color("text", v), 0.6))
			g.Ver(true)
		default:
			// `opacity-0`: ocupa sitio pero no se ve.
			g.Ver(false)
		}
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// ---- Fila del cuerpo ----

// FilaTabla es un `tr` del `tbody`: celdas con `px-3 py-2.5` y el hairline de abajo.
type FilaTabla struct {
	widget.BaseWidget
	t      *Tabla
	fila   int
	encima bool
}

var (
	_ fyne.Widget       = (*FilaTabla)(nil)
	_ fyne.Tappable     = (*FilaTabla)(nil)
	_ desktop.Hoverable = (*FilaTabla)(nil)
)

func nuevaFilaTabla(t *Tabla) *FilaTabla {
	f := &FilaTabla{t: t, fila: -1}
	f.ExtendBaseWidget(f)
	return f
}

// Poner recicla la fila con otro índice: es lo que llama el `UpdateItem` de la lista.
func (f *FilaTabla) Poner(fila int) {
	f.fila, f.encima = fila, false
	f.Refresh()
}

func (f *FilaTabla) marcada() bool {
	return f.t.Marcada != nil && f.fila >= 0 && f.t.Marcada(f.fila)
}

// CreateRenderer compone las celdas.
func (f *FilaTabla) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaTablaRenderer{
		f:     f,
		base:  canvas.NewRectangle(tema.Color("surface", v)),
		velo:  canvas.NewRectangle(color.NRGBA{}),
		linea: canvas.NewRectangle(tema.Color("border-subtle", v)),
	}
	r.fijos = []fyne.CanvasObject{r.base, r.velo}
	if f.t.seleccionable() {
		r.casilla = casillaDeFila(false, func() {
			if f.t.AlMarcar != nil && f.fila >= 0 {
				f.t.AlMarcar(f.fila)
			}
		})
		r.fijos = append(r.fijos, r.casilla)
	}
	for range f.t.Columnas {
		c := Texto("", tema.TextoBase, Normal, tema.Color("text", v))
		r.celdas = append(r.celdas, c)
		r.textos = append(r.textos, "")
		r.piezas = append(r.piezas, nil)
	}
	r.rearmar()
	return r
}

// Tapped no hace nada en el cuerpo: en Table.tsx la fila NO es pulsable, solo lo es
// su casilla, y esa es una `Eleccion` de verdad que se queda el clic por estar más
// adentro del árbol. El widget sigue siendo `Tappable` para que Fyne le entregue el
// evento y no se lo pase a lo que haya debajo.
func (f *FilaTabla) Tapped(*fyne.PointEvent) {}

// MouseIn es `hover:bg-bg-subtle`, que en la web solo existe si NO está seleccionada.
func (f *FilaTabla) MouseIn(*desktop.MouseEvent) { f.encima = true; f.Refresh() }

// MouseMoved no hace nada.
func (f *FilaTabla) MouseMoved(*desktop.MouseEvent) {}

// MouseOut quita el hover.
func (f *FilaTabla) MouseOut() { f.encima = false; f.Refresh() }

type filaTablaRenderer struct {
	f          *FilaTabla
	base, velo *canvas.Rectangle
	linea      *canvas.Rectangle
	casilla    *Eleccion
	celdas     []*canvas.Text
	// textos es el contenido SIN truncar de cada celda. Truncar sobre lo ya truncado
	// se come una letra por reciclaje de fila, y en un `widget.List` eso es una letra
	// por cada vez que la fila pasa por pantalla.
	textos []string
	// piezas es el objeto de cada celda cuando la tabla trae `CeldaObjeto` y devuelve
	// algo; nil deja esa celda en su `canvas.Text`. Se rearma la lista de objetos solo
	// cuando CAMBIA alguno: la fila se recicla en cada desplazamiento.
	piezas      []fyne.CanvasObject
	fijos       []fyne.CanvasObject
	objetos     []fyne.CanvasObject
	ultimoAncho float32
}

// rearmar rehace la lista de objetos del renderer: lo fijo, una celda por columna —el
// texto o su pieza— y el hairline al final, que va el último para quedar encima.
func (r *filaTablaRenderer) rearmar() {
	r.objetos = append(r.objetos[:0], r.fijos...)
	for i, p := range r.piezas {
		if p != nil {
			r.objetos = append(r.objetos, p)
			continue
		}
		r.objetos = append(r.objetos, r.celdas[i])
	}
	r.objetos = append(r.objetos, r.linea)
}

// MinSize es la que decide el alto de fila de todo el `widget.List`: la lista toma el
// MinSize del objeto que devuelve CreateItem y lo aplica a todas.
func (r *filaTablaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(ColCasillaTabla, AltoFilaTabla())
}

func (r *filaTablaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaTablaRenderer) Destroy()                     {}

func (r *filaTablaRenderer) Layout(t fyne.Size) {
	r.ultimoAncho = t.Width
	r.base.Resize(t)
	r.base.Move(fyne.NewPos(0, 0))
	r.velo.Resize(t)
	r.velo.Move(fyne.NewPos(0, 0))
	r.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
	r.linea.Move(fyne.NewPos(0, t.Height-tema.Borde))

	contenido := t.Height - tema.Borde
	x := float32(0)
	if r.casilla != nil {
		// La casilla va en su `w-10`, tras el `px-3` de su `td`.
		m := MedidaEleccion("")
		r.casilla.Resize(m)
		r.casilla.Move(fyne.NewPos(MargenCelda, (contenido-m.Height)/2))
		x += ColCasillaTabla
	}
	anchos := r.f.t.AnchosColumna(t.Width)
	y := (contenido - tema.TextoBase.Linea) / 2
	for i, col := range r.f.t.Columnas {
		disponible := anchos[i] - 2*MargenCelda
		var ancho float32
		if p := r.piezas[i]; p != nil {
			ancho = min(p.MinSize().Width, max(disponible, 0))
		} else {
			ancho = Truncar(r.celdas[i], r.textos[i], tema.TextoBase, Normal, disponible)
		}
		cx := x + MargenCelda
		switch col.Alineacion {
		case Centro:
			cx = x + (anchos[i]-ancho)/2
		case Fin:
			cx = x + anchos[i] - MargenCelda - ancho
		case Inicio:
		}
		if p := r.piezas[i]; p != nil {
			// La pieza va a su alto y centrada en la caja de la fila, como el `td` con
			// `items-center` de la web; el ancho es el suyo, recortado a la columna.
			alto := p.MinSize().Height
			p.Resize(fyne.NewSize(ancho, alto))
			p.Move(fyne.NewPos(cx, (contenido-alto)/2))
			x += anchos[i]
			continue
		}
		ColocarTexto(r.celdas[i], tema.TextoBase, cx, y)
		x += anchos[i]
	}
}

func (r *filaTablaRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.base.FillColor = tema.Color("surface", v)
	r.linea.FillColor = tema.Color("border-subtle", v)
	switch {
	case f.marcada():
		r.velo.FillColor = tema.Color("selected", v)
	case f.encima:
		r.velo.FillColor = tema.Color("bg-subtle", v)
	default:
		r.velo.FillColor = color.NRGBA{}
	}
	cambio := false
	for i := range f.t.Columnas {
		var pieza fyne.CanvasObject
		if f.fila >= 0 && f.t.CeldaObjeto != nil {
			pieza = f.t.CeldaObjeto(f.fila, i)
		}
		if pieza != r.piezas[i] {
			r.piezas[i], cambio = pieza, true
		}
		texto := ""
		if pieza == nil && f.fila >= 0 {
			texto = f.t.Celda(f.fila, i)
		}
		r.textos[i] = texto
		r.celdas[i].Text = texto
		r.celdas[i].Color = tema.Color("text", v)
	}
	if cambio {
		r.rearmar()
	}
	if r.casilla != nil {
		r.casilla.Marcada = f.marcada()
	}
	// El texto ya colocado hay que volver a repartirlo: la fila se recicla con
	// contenidos de anchos distintos y el truncado depende de ellos.
	if r.ultimoAncho > 0 {
		r.Layout(fyne.NewSize(r.ultimoAncho, AltoFilaTabla()))
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// ---- Pie de totales ----

// PieTabla es el `tfoot` de Table.tsx: `border-t` y las celdas en `font-semibold`.
type PieTabla struct {
	widget.BaseWidget
	t *Tabla
}

var _ fyne.Widget = (*PieTabla)(nil)

func nuevaPieTabla(t *Tabla) *PieTabla {
	p := &PieTabla{t: t}
	p.ExtendBaseWidget(p)
	return p
}

// CreateRenderer compone las celdas del pie.
func (p *PieTabla) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pieTablaRenderer{
		p:     p,
		base:  canvas.NewRectangle(tema.Color("surface", v)),
		linea: canvas.NewRectangle(tema.Color("border", v)),
	}
	r.objetos = []fyne.CanvasObject{r.base, r.linea}
	for i := range p.t.Columnas {
		texto := ""
		if i < len(p.t.Resumen) {
			texto = p.t.Resumen[i]
		}
		c := Texto(texto, tema.TextoBase, SemiNegrita, tema.Color("text", v))
		r.celdas = append(r.celdas, c)
		r.objetos = append(r.objetos, c)
	}
	return r
}

type pieTablaRenderer struct {
	p           *PieTabla
	base, linea *canvas.Rectangle
	celdas      []*canvas.Text
	objetos     []fyne.CanvasObject
}

func (r *pieTablaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(ColCasillaTabla, AltoFilaTabla())
}

func (r *pieTablaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pieTablaRenderer) Destroy()                     {}

func (r *pieTablaRenderer) Layout(t fyne.Size) {
	r.base.Resize(t)
	r.base.Move(fyne.NewPos(0, 0))
	// El `border-t` va ARRIBA, al revés que el de las filas del cuerpo.
	r.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
	r.linea.Move(fyne.NewPos(0, 0))

	x := float32(0)
	if r.p.t.seleccionable() {
		x += ColCasillaTabla
	}
	anchos := r.p.t.AnchosColumna(t.Width)
	y := tema.Borde + (t.Height-tema.Borde-tema.TextoBase.Linea)/2
	for i, col := range r.p.t.Columnas {
		ancho := Medir(r.celdas[i].Text, tema.TextoBase, SemiNegrita).Width
		cx := x + MargenCelda
		switch col.Alineacion {
		case Centro:
			cx = x + (anchos[i]-ancho)/2
		case Fin:
			cx = x + anchos[i] - MargenCelda - ancho
		case Inicio:
		}
		ColocarTexto(r.celdas[i], tema.TextoBase, cx, y)
		x += anchos[i]
	}
}

func (r *pieTablaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.base.FillColor = tema.Color("surface", v)
	r.linea.FillColor = tema.Color("border", v)
	for i := range r.celdas {
		if i < len(r.p.t.Resumen) {
			r.celdas[i].Text = r.p.t.Resumen[i]
		}
		r.celdas[i].Color = tema.Color("text", v)
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}
