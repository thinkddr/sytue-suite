// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
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
	// esquinaDentro es la esquina del marco por dentro del borde: la de la cabecera
	// arriba y la de la última fila abajo (`overflow-auto` recorta en la web).
	esquinaDentro = tema.RadioLG - tema.Borde
	// ladoCasilla es el `h-4 w-4` de la casilla de una celda.
	ladoCasilla = tema.Espacio(4)
	// ladoIconoCabeza es el `size={14}` del glifo de tipo de cada cabecera, y
	// huecoCabeza el `gap-1.5` que lo separa del nombre.
	ladoIconoCabeza = float32(14)
	huecoCabeza     = tema.Espacio(1.5)
	// huecoChips es el `gap-1` entre los chips de una celda de elección, enlace o adjuntos.
	huecoChips = tema.Espacio(1)
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
	// AlPegar escribe un bloque copiado de una hoja de cálculo desde una celda.
	AlPegar func(desde celda, texto string)
	// AlAbrir abre la ficha del registro: el botón de cada fila, también para quien lee.
	AlAbrir func(r api.BasesRecord)
	// AlMover avisa de que el foco ha cambiado de celda, con su caja dentro de la
	// rejilla, para que quien desplaza la deje a la vista.
	AlMover func(pos fyne.Position, tam fyne.Size)

	foco     celda
	enfocada bool
	// filaEncima es la fila bajo el puntero (`hover:` de la fila), si hayEncima.
	filaEncima int
	hayEncima  bool
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
	// resaltada recuadra la rejilla mientras hay ficheros encima: se pueden soltar ahí.
	resaltada bool
	r         *rejillaRenderer
}

// Resaltar enciende o apaga el recuadro de «suelta aquí».
func (g *rejilla) Resaltar(si bool) {
	if g.resaltada == si {
		return
	}
	g.resaltada = si
	g.Refresh()
}

var (
	_ fyne.Tappable       = (*rejilla)(nil)
	_ fyne.DoubleTappable = (*rejilla)(nil)
	_ fyne.Focusable      = (*rejilla)(nil)
	_ fyne.Shortcutable   = (*rejilla)(nil)
	_ desktop.Hoverable   = (*rejilla)(nil)
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

// pieza es un chip de una celda: lo que dice y su color.
type pieza struct {
	texto string
	tono  ui.Tono
}

// conChips dice si la celda se pinta con chips y no con texto: las elecciones (con el
// color de su opción), los enlaces y los adjuntos.
func conChips(campo api.BasesField) bool {
	switch campo.Kind {
	case api.BasesKindSelect, api.BasesKindMultiselect, api.BasesKindLink, api.BasesKindAttachment:
		return true
	}
	return false
}

// piezas son los chips de una celda: una opción, varias, los registros enlazados o los
// ficheros adjuntos.
func (g *rejilla) piezas(r api.BasesRecord, campo api.BasesField) []pieza {
	var out []pieza
	switch campo.Kind {
	case api.BasesKindSelect:
		if v, _ := r.Data[campo.ID].(string); v != "" {
			out = append(out, pieza{v, tonoDeOpcion(campo, v)})
		}
	case api.BasesKindMultiselect:
		for _, v := range cadenasDe(r.Data[campo.ID]) {
			out = append(out, pieza{v, tonoDeOpcion(campo, v)})
		}
	case api.BasesKindLink:
		for _, id := range r.Links[campo.ID] {
			e, ok := g.Etiquetas[campo.LinkTableID][id]
			if !ok {
				e = id[:min(len(id), 8)]
			}
			out = append(out, pieza{e, ui.InsigniaPrimaria})
		}
	case api.BasesKindAttachment:
		for _, a := range r.Attachments[campo.ID] {
			out = append(out, pieza{a.Name, ui.InsigniaNeutra})
		}
	}
	return out
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
	texto := textoParaEditar(campo, r.Data[campo.ID])
	e := ui.NuevaEntrada("").Con(ui.CampoSM)
	if campo.Kind == api.BasesKindDate {
		e = ui.CampoFecha(ui.CampoSM)
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
	valor, vale := valorEscrito(campo, g.editor.Texto())
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

// Tapped enfoca la celda pulsada (y confirma lo que se estuviera editando en otra). Un
// toque sobre la celda que ya tiene el foco la edita, como en una hoja de cálculo: en un
// teléfono el doble toque es torpe y además amplía.
func (g *rejilla) Tapped(ev *fyne.PointEvent) {
	c, ok := g.celdaEn(ev.Position)
	editaba := g.editando
	if editaba && !g.confirmar(true) {
		return
	}
	otraVez := ok && g.enfocada && c == g.foco && !editaba
	g.pedirFoco()
	if !ok {
		return
	}
	g.enfocar(c)
	if otraVez {
		g.empezarEdicion("")
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

// MouseIn, MouseMoved y MouseOut son el `hover:` de la fila: solo se repintan los fondos
// de la fila que se deja y de la que se entra, no la rejilla entera (con trescientas
// filas, recolocar cada celda a cada movimiento del ratón se notaría).
func (g *rejilla) MouseIn(ev *desktop.MouseEvent)    { g.MouseMoved(ev) }
func (g *rejilla) MouseMoved(ev *desktop.MouseEvent) { g.ponerEncima(g.filaEn(ev.Position)) }
func (g *rejilla) MouseOut()                         { g.ponerEncima(-1) }

// filaEn es la fila de registros que hay a esa altura, o -1 (la cabecera, el vacío).
func (g *rejilla) filaEn(p fyne.Position) int {
	if g.r == nil || g.r.alto <= 0 || p.Y < altoCabeza {
		return -1
	}
	if fila := int((p.Y - altoCabeza) / g.r.alto); fila < len(g.Registros) {
		return fila
	}
	return -1
}

func (g *rejilla) ponerEncima(fila int) {
	antes := -1
	if g.hayEncima {
		antes = g.filaEncima
	}
	if fila == antes {
		return
	}
	g.filaEncima, g.hayEncima = fila, fila >= 0
	if g.r != nil {
		g.r.pintarFila(antes)
		g.r.pintarFila(fila)
	}
}

// TypedShortcut es copiar la celda, pegar un bloque desde ella, y Ctrl+Inicio y Ctrl+Fin
// (⌘ en un Mac): la primera y la última celda. Con un modificador pulsado el driver no
// manda la tecla por `TypedKey`, sino como atajo al objeto con el foco (visto en una
// ventana real: por `TypedKey` no llegaba nunca).
func (g *rejilla) TypedShortcut(s fyne.Shortcut) {
	if g.editando {
		return
	}
	switch x := s.(type) {
	case *fyne.ShortcutCopy:
		if r, campo, ok := g.actual(); ok && x.Clipboard != nil {
			x.Clipboard.SetContent(g.textoCelda(r, campo))
		}
		return
	case *fyne.ShortcutPaste:
		if g.PuedeEditar && g.AlPegar != nil && x.Clipboard != nil {
			g.AlPegar(g.foco, x.Clipboard.Content())
		}
		return
	}
	a, ok := s.(*desktop.CustomShortcut)
	if !ok || a.Modifier&(fyne.KeyModifierControl|fyne.KeyModifierSuper) == 0 {
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
