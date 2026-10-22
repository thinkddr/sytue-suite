// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"image/color"
	"strconv"
	"strings"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las medidas de la rejilla (Rejilla.tsx).
var (
	// margenCelda es el `px-3` de una celda y margenFila0 el `px-2` de la primera columna.
	margenCelda = tema.Espacio(3)
	margenFila0 = tema.Espacio(2)
	// minCelda es el `min-w-32`.
	minCelda = 8 * float32(tema.Rem)
	// maxCelda es el tope de una columna.
	//
	// ponytail: la tabla de la web es `min-w-max` y crece lo que pida su celda más
	// larga; aquí una columna se queda en 20rem y recorta con puntos suspensivos, que
	// es lo que deja medir y pintar 300 filas sin repartir párrafos. Lo entero se ve al
	// editar la celda. Si hace falta, repartir en líneas con ui.Parrafo.
	maxCelda = 20 * float32(tema.Rem)
	// altoCabeza es el `py-2` más la línea de `text-sm`.
	altoCabeza = 2*tema.Espacio(2) + tema.TextoSM.Linea
	// ladoCasilla es el `h-4 w-4` de la casilla de una celda.
	ladoCasilla = tema.Espacio(4)
)

// rejilla es la rejilla editable de una tabla, con el patrón «grid» de WAI-ARIA de la
// web: una sola parada de tabulador (ella), flechas, Inicio, Fin, RePág y AvPág para
// moverse, Intro, F2 o escribir para editar, Escape para cancelar, Espacio para marcar
// una casilla y Supr para vaciar. Lo que se edita se envía al servicio, que lo vuelve a
// validar: aquí solo se evita mandar lo que ya se sabe que no encaja.
//
// Es UN widget y no una tabla de widgets: con tres ventanas de cien registros son miles
// de celdas, y el foco, el editor y lo desplazado tienen que sobrevivir a cada repintado
// de la pantalla.
type rejilla struct {
	widget.BaseWidget
	t func(clave string, vars ...i18n.Vars) string

	Tabla       string
	Campos      []api.BasesField
	Registros   []api.BasesRecord
	PuedeEditar bool
	// Etiquetas son las de los registros de cada tabla enlazada, por id de tabla.
	Etiquetas map[string]map[string]string
	// Desde es cuántos registros hay ANTES de los que se enseñan (carga por ventanas).
	Desde int

	// AlCambiar guarda el valor de una celda (nil la vacía).
	AlCambiar func(r api.BasesRecord, campo api.BasesField, valor any)
	// AlElegirVarios abre el selector de una selección múltiple o de un enlace.
	AlElegirVarios func(r api.BasesRecord, campo api.BasesField)
	// AlAdjuntos abre los ficheros de una celda de adjuntos (también quien solo lee).
	AlAdjuntos func(r api.BasesRecord, campo api.BasesField)
	AlBorrar   func(r api.BasesRecord)
	// AlMover avisa de que el foco ha cambiado de celda, con su caja dentro de la
	// rejilla, para que quien desplaza la deje a la vista.
	AlMover func(pos fyne.Position, tam fyne.Size)

	foco     celda
	enfocada bool
	// version sube cuando cambian los datos: el renderer vuelve a armar las celdas.
	version int
	// Edición de texto, número o fecha: el campo va encima de la celda.
	editando bool
	editor   *ui.Entrada
	resuelto bool
	// focoEditor dice si el campo de edición (o su editor de dentro) tiene el foco.
	focoEditor bool
	// Aviso es lo último que la rejilla ha dicho (el `role="status"` de la web): un
	// valor que no encaja, un campo que se calcula solo.
	Aviso string
	r     *rejillaRenderer
}

var (
	_ fyne.Tappable       = (*rejilla)(nil)
	_ fyne.DoubleTappable = (*rejilla)(nil)
	_ fyne.Focusable      = (*rejilla)(nil)
	_ fyne.Shortcutable   = (*rejilla)(nil)
)

func nuevaRejilla(t func(string, ...i18n.Vars) string) *rejilla {
	g := &rejilla{t: t}
	g.ExtendBaseWidget(g)
	return g
}

// Poner cambia lo que se enseña y recoloca el foco si se ha quedado fuera (una fila
// borrada, otra página).
func (g *rejilla) Poner(campos []api.BasesField, registros []api.BasesRecord) {
	g.Campos, g.Registros = campos, registros
	g.foco.fila = min(g.foco.fila, max(len(registros)-1, 0))
	g.foco.col = min(g.foco.col, max(len(campos)-1, 0))
	g.version++
	g.Refresh()
}

// AccessibilityLabel es el `aria-label` de la tabla.
func (g *rejilla) AccessibilityLabel() string {
	return g.t("grid.label", i18n.Vars{"table": g.Tabla})
}

// actual es la celda con el foco, si existe.
func (g *rejilla) actual() (api.BasesRecord, api.BasesField, bool) {
	if g.foco.fila < 0 || g.foco.fila >= len(g.Registros) || g.foco.col < 0 || g.foco.col >= len(g.Campos) {
		return api.BasesRecord{}, api.BasesField{}, false
	}
	return g.Registros[g.foco.fila], g.Campos[g.foco.col], true
}

// textoCelda es lo que se lee en una celda que no es una casilla.
func (g *rejilla) textoCelda(r api.BasesRecord, campo api.BasesField) string {
	switch campo.Kind {
	case api.BasesKindAttachment:
		nombres := make([]string, len(r.Attachments[campo.ID]))
		for i, a := range r.Attachments[campo.ID] {
			nombres[i] = a.Name
		}
		return strings.Join(nombres, ", ")
	case api.BasesKindLink:
		etiquetas := make([]string, len(r.Links[campo.ID]))
		for i, id := range r.Links[campo.ID] {
			if e, ok := g.Etiquetas[campo.LinkTableID][id]; ok {
				etiquetas[i] = e
			} else {
				etiquetas[i] = id[:min(len(id), 8)]
			}
		}
		return strings.Join(etiquetas, ", ")
	}
	return textoDeValor(r.Data[campo.ID])
}

// enfocar mueve el foco a una celda y lo dice.
func (g *rejilla) enfocar(c celda) {
	g.foco = c
	g.Refresh()
	if g.AlMover != nil && g.r != nil {
		pos, tam := g.r.cajaDe(c)
		if c.col == 0 { // con la primera columna se ve también la de los números de fila
			pos.X, tam.Width = 0, tam.Width+pos.X
		}
		if c.fila == 0 { // y con la primera fila, la cabecera
			pos.Y, tam.Height = 0, tam.Height+pos.Y
		}
		g.AlMover(pos, tam)
	}
}

// pedirFoco le da el foco de la ventana a la rejilla.
func (g *rejilla) pedirFoco() {
	if c := fyne.CurrentApp().Driver().CanvasForObject(g); c != nil {
		c.Focus(g)
	}
}

// empezarEdicion es Intro, F2, el doble clic o empezar a escribir sobre una celda.
func (g *rejilla) empezarEdicion(inicial string) {
	r, campo, ok := g.actual()
	if !ok {
		return
	}
	if campo.Kind == api.BasesKindAttachment {
		if g.AlAdjuntos != nil {
			g.AlAdjuntos(r, campo)
		}
		return
	}
	if !g.PuedeEditar {
		return
	}
	switch campo.Kind {
	case api.BasesKindFormula:
		// Lo calcula el servicio: no se edita, y se dice.
		g.Aviso = g.t("grid.formulaReadOnly")
	case api.BasesKindCheckbox:
		var valor any = true
		if r.Data[campo.ID] == true {
			valor = nil
		}
		g.guardar(r, campo, valor)
	case api.BasesKindMultiselect, api.BasesKindLink:
		if g.AlElegirVarios != nil {
			g.AlElegirVarios(r, campo)
		}
	case api.BasesKindSelect:
		g.elegirOpcion(r, campo)
	default:
		g.abrirEditor(r, campo, inicial)
	}
}

func (g *rejilla) guardar(r api.BasesRecord, campo api.BasesField, valor any) {
	g.Aviso = ""
	if g.AlCambiar != nil {
		g.AlCambiar(r, campo, valor)
	}
}

// elegirOpcion es el `<select>` de una celda de elección única: sus opciones cuelgan de
// la celda y elegir una la guarda. La primera, en blanco, la vacía.
func (g *rejilla) elegirOpcion(r api.BasesRecord, campo api.BasesField) {
	if g.r == nil {
		return
	}
	elementos := []fyne.CanvasObject{ui.NuevoElementoMenu("—", iconos.Ninguno, func() { g.guardar(r, campo, nil) })}
	for _, o := range campo.Choices {
		elementos = append(elementos, ui.NuevoElementoMenu(o, iconos.Ninguno, func() { g.guardar(r, campo, o) }))
	}
	m := ui.NuevoMenu(elementos...)
	m.Rotulo, m.AlInicio, m.Ancla = campo.Name, true, g.r.ancla
	m.Mostrar(g)
}

// abrirEditor pone el campo de texto encima de la celda, con lo que había o con la
// letra que lo ha abierto.
func (g *rejilla) abrirEditor(r api.BasesRecord, campo api.BasesField, inicial string) {
	texto := textoDeValor(r.Data[campo.ID])
	e := ui.NuevaEntrada("").Con(ui.CampoSM)
	if campo.Kind == api.BasesKindDate {
		e = ui.CampoFecha(ui.CampoSM)
		if f, err := time.Parse("2006-01-02", texto); err == nil {
			texto = ui.EscribirFecha(f)
		}
	}
	if inicial != "" {
		texto = inicial
	}
	e.Rotulo = campo.Name
	e.PonerTextoAlFinal(texto)
	e.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyEscape {
			g.cerrarEditor()
			g.pedirFoco()
			return true
		}
		if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
			return false
		}
		if g.confirmar(true) {
			if sig, ok := moverFoco(g.foco, "Down", false, len(g.Registros), len(g.Campos)); ok {
				g.enfocar(sig)
			}
		}
		return true
	}
	// Perder el foco confirma, como el `onBlur` de la web; salvo que ya esté resuelto.
	// Se mira DESPUÉS de que el suceso termine: pulsar dentro del campo pasa el foco de
	// la Entrada a su editor de dentro —pierde uno y gana el otro en el mismo clic—, y
	// eso no es salir del campo.
	e.AlEnfocar = func(con bool) {
		g.focoEditor = con
		if con {
			return
		}
		go fyne.Do(func() {
			if g.editando && g.editor == e && !g.resuelto && !g.focoEditor {
				g.confirmar(false)
			}
		})
	}
	g.editor, g.editando, g.resuelto, g.Aviso = e, true, false, ""
	g.Refresh()
	if c := fyne.CurrentApp().Driver().CanvasForObject(g); c != nil {
		c.Focus(e)
	}
}

// cerrarEditor quita el campo de encima de la celda.
func (g *rejilla) cerrarEditor() {
	g.resuelto, g.editando, g.editor = true, false, nil
	g.Refresh()
}

// confirmar guarda lo escrito si encaja en el tipo; si no, el campo sigue abierto y
// marcado. Devuelve si se ha cerrado. `devolver` deja el foco en la rejilla: no cuando
// se confirma porque el foco se ha ido a otro sitio.
func (g *rejilla) confirmar(devolver bool) bool {
	r, campo, ok := g.actual()
	if !ok || g.editor == nil {
		g.cerrarEditor()
		return true
	}
	texto := g.editor.Texto()
	if campo.Kind == api.BasesKindDate {
		// Se teclea como se escribe una fecha (05/10/2026) y viaja en ISO.
		if f, vacia, vale := ui.LeerFecha(texto, time.UTC); vale && !vacia {
			texto = f.Format("2006-01-02")
		} else if vacia {
			texto = ""
		} else {
			texto = "?"
		}
	}
	valor, vale := valorDeTexto(campo, texto)
	if !vale {
		g.Aviso = g.t("grid.invalidValue")
		g.editor.Invalida = true
		g.editor.Refresh()
		return false
	}
	g.cerrarEditor()
	if devolver {
		g.pedirFoco()
	}
	if textoDeValor(r.Data[campo.ID]) != textoDeValor(valor) {
		g.guardar(r, campo, valor)
	}
	return true
}

// Tapped enfoca la celda pulsada (y confirma lo que se estuviera editando en otra).
func (g *rejilla) Tapped(ev *fyne.PointEvent) {
	c, ok := g.celdaEn(ev.Position)
	if g.editando && !g.confirmar(true) {
		return
	}
	g.pedirFoco()
	if ok {
		g.enfocar(c)
	}
}

// DoubleTapped edita la celda, como el `onDblClick` de la web.
func (g *rejilla) DoubleTapped(ev *fyne.PointEvent) {
	if c, ok := g.celdaEn(ev.Position); ok && !g.editando {
		g.pedirFoco()
		g.enfocar(c)
		g.empezarEdicion("")
	}
}

func (g *rejilla) celdaEn(p fyne.Position) (celda, bool) {
	if g.r == nil {
		return celda{}, false
	}
	return g.r.celdaEn(p)
}

func (g *rejilla) FocusGained() { g.enfocada = true; g.Refresh() }
func (g *rejilla) FocusLost()   { g.enfocada = false; g.Refresh() }

// TypedShortcut es Ctrl+Inicio y Ctrl+Fin (⌘ en un Mac): la primera y la última celda.
// Con un modificador pulsado el driver no manda la tecla por `TypedKey`, sino como atajo
// al objeto con el foco (visto en una ventana real: por `TypedKey` no llegaba nunca).
func (g *rejilla) TypedShortcut(s fyne.Shortcut) {
	a, ok := s.(*desktop.CustomShortcut)
	if !ok || g.editando || a.Modifier&(fyne.KeyModifierControl|fyne.KeyModifierSuper) == 0 {
		return
	}
	if a.KeyName != fyne.KeyHome && a.KeyName != fyne.KeyEnd {
		return
	}
	if destino, mueve := moverFoco(g.foco, string(a.KeyName), true, len(g.Registros), len(g.Campos)); mueve {
		g.enfocar(destino)
	}
}

// TypedRune empieza a editar con esa letra en un campo de texto, número o fecha; y el
// espacio marca una casilla.
func (g *rejilla) TypedRune(ru rune) {
	_, campo, ok := g.actual()
	if !ok || g.editando {
		return
	}
	switch {
	case ru == ' ' && campo.Kind == api.BasesKindCheckbox:
		g.empezarEdicion("")
	case !unicode.IsSpace(ru) && (campo.Kind == api.BasesKindText || campo.Kind == api.BasesKindNumber || campo.Kind == api.BasesKindDate):
		g.empezarEdicion(string(ru))
	}
}

// TypedKey mueve el foco, abre la edición o vacía la celda.
func (g *rejilla) TypedKey(k *fyne.KeyEvent) {
	if g.editando {
		return
	}
	if destino, mueve := moverFoco(g.foco, string(k.Name), false, len(g.Registros), len(g.Campos)); mueve {
		g.enfocar(destino)
		return
	}
	switch k.Name {
	case fyne.KeyReturn, fyne.KeyEnter, fyne.KeyF2:
		g.empezarEdicion("")
	case fyne.KeyDelete, fyne.KeyBackspace:
		r, campo, ok := g.actual()
		if ok && g.PuedeEditar && campo.Kind != api.BasesKindLink && campo.Kind != api.BasesKindFormula && campo.Kind != api.BasesKindAttachment {
			g.guardar(r, campo, nil)
		}
	default:
		ui.SubirEscape(g, k)
	}
}

func (g *rejilla) CreateRenderer() fyne.WidgetRenderer {
	g.r = &rejillaRenderer{g: g, version: -1}
	g.r.Refresh()
	return g.r
}

type rejillaRenderer struct {
	g       *rejilla
	version int

	marco, cabeza, anillo, ancla *canvas.Rectangle
	lineas                       []*canvas.Rectangle
	titulos, tipos, numeros      []*canvas.Text
	borrar                       []*ui.Boton
	// textos[fila][col] es el texto de la celda; nil donde va una casilla.
	textos   [][]*canvas.Text
	casillas map[celda]fyne.CanvasObject
	objetos  []fyne.CanvasObject

	// La geometría: el ancho de cada columna de campo (a su contenido) y de la primera,
	// el alto de una fila, y lo que cada columna se estira para llenar el ancho dado.
	anchos       []float32
	ancho0, alto float32
	ladoBoton    float32
	estirado     float32
	// conEditor es el campo de edición que hay entre los objetos, o nil.
	conEditor *ui.Entrada
}

func (r *rejillaRenderer) Destroy()                     {}
func (r *rejillaRenderer) Objects() []fyne.CanvasObject { return r.objetos }

// poner vuelve a armar las celdas si los datos han cambiado.
func (r *rejillaRenderer) poner() {
	if r.version != r.g.version {
		r.armar()
	}
}

// armar crea los objetos de la cabecera y de cada fila, y mide las columnas.
func (r *rejillaRenderer) armar() {
	g := r.g
	r.version = g.version
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta, apagada := tema.Color("text", v), tema.Color("text-muted", v)

	r.marco = canvas.NewRectangle(tema.Color("surface", v))
	r.marco.CornerRadius, r.marco.StrokeWidth, r.marco.StrokeColor = tema.RadioLG, tema.Borde, tema.Color("border", v)
	r.cabeza = canvas.NewRectangle(tema.Color("bg-subtle", v))
	r.anillo = canvas.NewRectangle(color.Transparent)
	r.anillo.StrokeWidth, r.anillo.StrokeColor = tema.AnilloFoco, tema.Color("focus-ring", v)
	r.ancla = canvas.NewRectangle(color.Transparent)
	r.objetos = []fyne.CanvasObject{r.marco, r.cabeza}

	muestra := ui.NuevoBoton("", nil).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Papelera, 14)
	muestra.SoloIcono = true
	r.ladoBoton = ui.MedidaBotonDe(muestra).Height
	r.alto = 2*tema.Espacio(1) + r.ladoBoton

	r.anchos = make([]float32, len(g.Campos))
	r.titulos, r.tipos = r.titulos[:0], r.tipos[:0]
	for i, c := range g.Campos {
		titulo := ui.Texto(c.Name, tema.TextoSM, ui.SemiNegrita, tinta)
		tipo := ui.Texto(g.t("field.kind."+c.Kind), tema.TextoXS, ui.Normal, apagada)
		r.titulos, r.tipos = append(r.titulos, titulo), append(r.tipos, tipo)
		r.objetos = append(r.objetos, titulo, tipo)
		r.anchos[i] = ui.Medir(c.Name, tema.TextoSM, ui.SemiNegrita).Width + tema.Espacio(1) + ui.Medir(tipo.Text, tema.TextoXS, ui.Normal).Width
	}

	r.lineas, r.numeros, r.borrar, r.textos = r.lineas[:0], r.numeros[:0], r.borrar[:0], r.textos[:0]
	r.casillas = map[celda]fyne.CanvasObject{}
	var anchoNumero float32
	for fila, reg := range g.Registros {
		linea := canvas.NewRectangle(tema.Color("border", v))
		n := strconv.Itoa(g.Desde + fila + 1)
		numero := ui.Texto(n, tema.TextoSM, ui.Normal, apagada)
		anchoNumero = max(anchoNumero, ui.Medir(n, tema.TextoSM, ui.Normal).Width)
		r.lineas, r.numeros = append(r.lineas, linea), append(r.numeros, numero)
		r.objetos = append(r.objetos, linea, numero)
		if g.PuedeEditar {
			b := ui.NuevoBoton(g.t("grid.deleteRecord", i18n.Vars{"n": g.Desde + fila + 1}), func() {
				if g.AlBorrar != nil {
					g.AlBorrar(reg)
				}
			}).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Papelera, 14)
			b.SoloIcono = true
			r.borrar = append(r.borrar, b)
			r.objetos = append(r.objetos, b)
		}
		textos := make([]*canvas.Text, len(g.Campos))
		for col, c := range g.Campos {
			if c.Kind == api.BasesKindCheckbox {
				o := casilla(reg.Data[c.ID] == true, v)
				r.casillas[celda{fila, col}] = o
				r.objetos = append(r.objetos, o)
				continue
			}
			completo := g.textoCelda(reg, c)
			textos[col] = ui.Texto(completo, tema.TextoSM, ui.Normal, tinta)
			r.objetos = append(r.objetos, textos[col])
			r.anchos[col] = max(r.anchos[col], ui.Medir(completo, tema.TextoSM, ui.Normal).Width)
		}
		r.textos = append(r.textos, textos)
	}
	for i := range r.anchos {
		r.anchos[i] = min(max(r.anchos[i]+2*margenCelda, minCelda), maxCelda)
	}
	r.ancho0 = 2*margenFila0 + anchoNumero
	if g.PuedeEditar {
		r.ancho0 += tema.Espacio(1) + r.ladoBoton
	}
	r.ancho0 = max(r.ancho0, tema.Espacio(12)) // el `w-12`
	r.objetos = append(r.objetos, r.ancla, r.anillo)
	r.conEditor = nil
	if g.editando && g.editor != nil {
		r.conEditor = g.editor
		r.objetos = append(r.objetos, g.editor)
	}
}

// casilla es la casilla de una celda: se ve y no se pulsa (`pointer-events-none`); la
// marca Espacio, Intro o el doble clic sobre la celda.
func casilla(marcada bool, v tema.Variante) fyne.CanvasObject {
	caja := canvas.NewRectangle(tema.Color("surface", v))
	caja.CornerRadius, caja.StrokeWidth, caja.StrokeColor = tema.RadioSM, tema.Borde, tema.Color("border-strong", v)
	caja.Resize(fyne.NewSquareSize(ladoCasilla))
	if !marcada {
		return container.NewWithoutLayout(caja)
	}
	caja.FillColor, caja.StrokeColor = tema.Color("primary", v), tema.Color("primary", v)
	tic := iconos.Trazo(iconos.Tic, ladoCasilla-4, "on-primary", v).Objeto()
	tic.Move(fyne.NewPos(2, 2))
	tic.Resize(fyne.NewSquareSize(ladoCasilla - 4))
	return container.NewWithoutLayout(caja, tic)
}

func (r *rejillaRenderer) MinSize() fyne.Size {
	r.poner()
	ancho := r.ancho0
	for _, a := range r.anchos {
		ancho += a
	}
	return fyne.NewSize(ancho, altoCabeza+float32(len(r.g.Registros))*r.alto)
}

// columna es dónde empieza la columna `col` y cuánto mide, ya estirada.
func (r *rejillaRenderer) columna(col int) (x, ancho float32) {
	x = r.ancho0
	for i := 0; i < col; i++ {
		x += r.anchos[i] + r.estirado
	}
	return x, r.anchos[col] + r.estirado
}

// cajaDe es la caja de una celda dentro de la rejilla.
func (r *rejillaRenderer) cajaDe(c celda) (fyne.Position, fyne.Size) {
	if c.col < 0 || c.col >= len(r.anchos) {
		return fyne.Position{}, fyne.Size{}
	}
	x, ancho := r.columna(c.col)
	return fyne.NewPos(x, altoCabeza+float32(c.fila)*r.alto), fyne.NewSize(ancho, r.alto)
}

func (r *rejillaRenderer) celdaEn(p fyne.Position) (celda, bool) {
	if p.Y < altoCabeza || p.X < r.ancho0 || r.alto <= 0 {
		return celda{}, false
	}
	fila := int((p.Y - altoCabeza) / r.alto)
	if fila >= len(r.g.Registros) {
		return celda{}, false
	}
	for col := range r.anchos {
		if x, ancho := r.columna(col); p.X >= x && p.X < x+ancho {
			return celda{fila, col}, true
		}
	}
	return celda{}, false
}

func (r *rejillaRenderer) Layout(t fyne.Size) {
	r.poner()
	g := r.g
	// `w-full`: lo que sobre del ancho se reparte entre las columnas de campo.
	r.estirado = 0
	if sobra := t.Width - r.MinSize().Width; sobra > 0 && len(r.anchos) > 0 {
		r.estirado = sobra / float32(len(r.anchos))
	}
	r.marco.Resize(t)
	r.cabeza.Move(fyne.NewPos(tema.Borde, tema.Borde))
	r.cabeza.Resize(fyne.NewSize(max(t.Width-2*tema.Borde, 0), altoCabeza-tema.Borde))
	for i := range g.Campos {
		x, _ := r.columna(i)
		ui.ColocarTexto(r.titulos[i], tema.TextoSM, x+margenCelda, tema.Espacio(2))
		ancho := ui.Medir(r.titulos[i].Text, tema.TextoSM, ui.SemiNegrita).Width
		ui.ColocarTexto(r.tipos[i], tema.TextoXS, x+margenCelda+ancho+tema.Espacio(1), tema.Espacio(2)+(tema.TextoSM.Linea-tema.TextoXS.Linea)/2)
	}
	yTexto := (r.alto - tema.TextoSM.Linea) / 2
	for fila := range g.Registros {
		y := altoCabeza + float32(fila)*r.alto
		r.lineas[fila].Move(fyne.NewPos(tema.Borde, y))
		r.lineas[fila].Resize(fyne.NewSize(max(t.Width-2*tema.Borde, 0), tema.Borde))
		ui.ColocarTexto(r.numeros[fila], tema.TextoSM, margenFila0, y+yTexto)
		if fila < len(r.borrar) {
			b := r.borrar[fila]
			b.Move(fyne.NewPos(r.ancho0-margenFila0-r.ladoBoton, y+tema.Espacio(1)))
			b.Resize(fyne.NewSquareSize(r.ladoBoton))
		}
		for col, c := range g.Campos {
			x, ancho := r.columna(col)
			if o, ok := r.casillas[celda{fila, col}]; ok {
				o.Move(fyne.NewPos(x+margenCelda, y+(r.alto-ladoCasilla)/2))
				o.Resize(fyne.NewSquareSize(ladoCasilla))
				continue
			}
			ui.Truncar(r.textos[fila][col], g.textoCelda(g.Registros[fila], c), tema.TextoSM, ui.Normal, ancho-2*margenCelda)
			ui.ColocarTexto(r.textos[fila][col], tema.TextoSM, x+margenCelda, y+yTexto)
		}
	}
	// El aro de la celda con el foco (`sytue-focus-ring-inset`), el ancla de un menú de
	// opciones y, si se está editando, el campo encima de su celda.
	pos, tam := r.cajaDe(g.foco)
	r.ancla.Move(pos)
	r.ancla.Resize(tam)
	r.anillo.Move(pos)
	r.anillo.Resize(tam)
	if g.enfocada && !g.editando && len(g.Registros) > 0 && len(g.Campos) > 0 {
		r.anillo.Show()
	} else {
		r.anillo.Hide()
	}
	if g.editando && g.editor != nil {
		alto := g.editor.MinSize().Height
		g.editor.Move(fyne.NewPos(pos.X+tema.Espacio(1), pos.Y+(tam.Height-alto)/2))
		g.editor.Resize(fyne.NewSize(max(tam.Width-2*tema.Espacio(1), 0), alto))
	}
}

func (r *rejillaRenderer) Refresh() {
	// El campo de edición entra y sale de los objetos con la edición.
	var editor *ui.Entrada
	if r.g.editando {
		editor = r.g.editor
	}
	if r.version != r.g.version || r.conEditor != editor {
		r.armar()
	}
	r.Layout(r.g.Size())
	canvas.Refresh(r.g)
}
