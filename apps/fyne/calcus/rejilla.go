// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Celda son las coordenadas de una celda, de base cero.
type Celda struct{ C, F int }

// Rejilla es la cuadrícula de Calcus: el `role="grid"` de SheetCanvas.tsx con sus
// cabeceras pegajosas, su celda ancla, su rango y su celda en edición.
//
// # Por qué NO va en un `widget.List`
//
// La tabla de la suite sí: su cuerpo se desplaza en un solo eje y la virtualización de
// Fyne es la buena (medido: plana, 0,6 ms con 50 filas y con 5.000). Una hoja de
// cálculo se desplaza en DOS, y en un `widget.List` la segunda dimensión no existe:
// las cabeceras de columna no pueden quedarse pegadas mientras el cuerpo corre por
// debajo. Aquí la virtualización es propia y recicla primitivas: de las 2.600 celdas
// de la rejilla solo existen las que caben en la ventana.
//
// De regalo se esquiva la trampa medida de `widget.List`, que separa cada dos filas
// por el `padding` del TEMA —7 px, 35.000 en cinco mil filas— y obliga a envolverlo en
// un `container.ThemeOverride`. Aquí no hay lista, así que no hay hueco que quitar.
//
// # Y se recorta sola
//
// Fyne no recorta los objetos de un renderer a la caja de su widget… salvo si el
// widget implementa `fyne.Scrollable`, que es lo que mira `isClipWithRenderer` para
// abrir un límite de recorte. Esta rejilla lo implementa porque tiene que responder a
// la rueda, y por eso las medias celdas de los bordes se cortan donde toca en vez de
// pintarse por encima del cromo.
type Rejilla struct {
	widget.BaseWidget

	// Contexto es el del editor, para Ctrl/Cmd+P y el zoom con el foco en la rejilla; nil, nada.
	Contexto *editor.Contexto

	L *Libro
	// Cols y Fils son la EXTENSIÓN de la rejilla (`extension()` de la web): lo usado, la
	// selección y lo que se ve, con la rejilla mínima de 26×100 de holgura por delante,
	// hasta los límites de Excel. Los recalcula `crecer` al desplazarse, al seleccionar
	// y al pintar; lo que se pinta son solo las visibles, sea cual sea su tamaño (ver
	// `BenchmarkRejilla`).
	Cols, Fils int
	// resaltes son las celdas con alguna coincidencia de «Buscar», que se pintan de
	// amarillo como en la web; las pone la búsqueda.
	resaltes map[Celda]bool
	// comentarios son los hilos del libro, que marcan sus celdas con un triángulo, y
	// resalteComentario, el ancla del hilo con el puntero encima en el panel (nil: ninguno).
	// Ver comentarios.go.
	comentarios       []api.DocComment
	resalteComentario *api.AnclaCelda
	// SoloLectura es la hoja protegida de la web (`sheet.protect`): se ve, se recorre
	// y se copia, pero no se escribe. Es el estado de «permiso denegado» del lienzo.
	SoloLectura bool
	// AlMoverse avisa de que la celda ancla o el rango han cambiado; AlEditar, de que
	// el contenido lo ha hecho. Las dos las usa la barra de fórmulas.
	AlMoverse func()
	AlEditar  func()
	// AlDeshacer es Ctrl+Z (false) y Ctrl+Y o Ctrl+Mayús+Z (true). Lo atiende el lienzo,
	// porque deshacer puede cambiar de hoja o quitar una, y eso toca las tres piezas.
	AlDeshacer func(rehacer bool)
	// AlAtajos es Ctrl+/: abre la hoja de atajos, que es del lienzo (necesita el traductor).
	AlAtajos func()
	// AlIrA es Ctrl+G o F5, «Ir a»: lleva el foco al cuadro de nombres de la barra.
	AlIrA func()
	// AlAbrirEnlace sustituye al sistema al abrir el enlace de una celda (pruebas). Sin
	// él, el enlace va al navegador o al correo por las puertas de `api` (`enlaces.go`).
	AlAbrirEnlace func(href string)
	// Zoom es el `zoom` de CSS con el que la web escala la rejilla entera, cabeceras
	// incluidas: 1 es el 100 %. Cero cuenta como 1.
	Zoom float32
	// Señalar es el point-and-click de una fórmula en edición (lo pone el lienzo):
	// devuelve true si se queda el clic, que entonces no selecciona. AlSeñalar se llama
	// al soltar, para devolver el foco al editor de la fórmula.
	Señalar   func(zona int, c Celda, gesto int) bool
	AlSeñalar func()
	// AlCerrarEdicion avisa de que la celda abierta se ha confirmado o cancelado.
	AlCerrarEdicion func()
	// AlRechazar avisa de que una validación estricta no ha admitido lo escrito, con su
	// mensaje.
	AlRechazar func(mensaje string)
	// AlBotonValores es el botón «▾» de la celda con una validación de lista: abre o
	// cierra su lista de valores. AlPulsarFuera avisa de cualquier otra pulsación.
	AlBotonValores func()
	// AlBotonFiltro es el «▾» del autofiltro en la cabecera de la columna `col`.
	AlBotonFiltro func(col int)
	// AlEsquema es un botón del canalón del esquema (+/− de un grupo o un nivel): el
	// valor del comando `outline` que manda.
	AlEsquema func(valor string)
	// AlProtegida avisa de que la hoja protegida no deja escribir en esa celda.
	AlProtegida func(Celda)
	// Flotantes da los gráficos y las segmentaciones de la hoja ya colocados para esa
	// vista. Los pone el lienzo, que es quien sabe hacerlos; la rejilla solo los pinta
	// en su sitio de apilado y los recorta a su caja.
	Flotantes func(vw vista) []fyne.CanvasObject
	// AlPulsarFuera cierra lo que cuelgue de la rejilla (la lista de valores) al pulsar
	// en otra parte o con Escape; devuelve si había algo que cerrar.
	AlPulsarFuera func() bool
	// AlMenu abre el menú contextual de una variante (`menuCeldas`, `menuColumnas` o
	// `menuFilas`) colgando de `ancla`; lo pone el lienzo, que es quien sabe los comandos.
	// Sin él, el clic derecho y Mayús+F10 no hacen nada.
	AlMenu func(variante int, ancla fyne.CanvasObject)

	sel          Celda
	ext          *Celda // extremo del rango; nil si solo hay celda ancla
	desX, desY   float32
	enfocado     bool
	mays         bool
	arrastre     int // la zona donde empezó el arrastre, o zonaFuera
	señalando    bool
	mayRaton     bool // Mayús en la última pulsación del ratón
	ctrlRaton    bool // Ctrl (⌘ en el Mac) en la última pulsación: abre el enlace de la celda
	autoArrastre ui.AutoScrollArrastre
	cancelado    bool // ignora movimientos tardíos hasta DragEnd
	arrastrando  bool
	// tirador es lo que lleva el dedo en un arrastre táctil, y agarre, lo que el dedo queda
	// de la esquina del tirador (`tactil.go`).
	tirador int
	agarre  fyne.Position
	// medida es el arrastre del asa del borde de una cabecera (`redimensionar.go`), y
	// puntero, dónde está el ratón (nil fuera): de él sale el cursor de redimensionar.
	medida  *gestoMedida
	puntero *fyne.Position
	// El autorrelleno: arrastrando el asa, el rango de origen y el que se va a rellenar.
	rellenando bool
	origenRell [4]int
	destRell   *[4]int
	// refsFx son las referencias de la fórmula en edición que caen en esta hoja (los
	// marcos de color); celdaFx dice que la fórmula se escribe EN la celda (su tinte).
	refsFx   []formula.Referencia
	celdaFx  bool
	editando bool
	// boton es el «▾» que tiene el puntero encima (`hover:bg-selected`): 0 ninguno, −1 el
	// de la validación de la celda ancla y c+1 el del autofiltro de la columna c.
	boton   int
	ed      *ui.EditorPlano
	r       *rejillaRenderer
	copiado *copia
	// anclaMenu es el punto transparente del que cuelga el menú contextual.
	anclaMenu *canvas.Rectangle
	// T traduce (la del lienzo); nil, las claves tal cual.
	T func(clave string, vars i18n.Vars) string
	// pistaComentario es el `title` de una celda con comentarios: el primero, al pasar el
	// puntero (comentarios.go). celdaPista es la celda a la que está puesta, si hayPista.
	pistaComentario *ui.Pista
	celdaPista      Celda
	hayPista        bool
}

var (
	_ fyne.Widget            = (*Rejilla)(nil)
	_ fyne.Tappable          = (*Rejilla)(nil)
	_ fyne.DoubleTappable    = (*Rejilla)(nil)
	_ fyne.SecondaryTappable = (*Rejilla)(nil)
	_ fyne.Focusable         = (*Rejilla)(nil)
	_ fyne.Scrollable        = (*Rejilla)(nil)
	_ fyne.Draggable         = (*Rejilla)(nil)
	_ fyne.Accessible        = (*Rejilla)(nil)
	_ fyne.Shortcutable      = (*Rejilla)(nil)
	_ fyne.Tabbable          = (*Rejilla)(nil)
	_ desktop.Keyable        = (*Rejilla)(nil)
	_ desktop.Cursorable     = (*Rejilla)(nil)
	_ desktop.Mouseable      = (*Rejilla)(nil)
	_ desktop.Hoverable      = (*Rejilla)(nil)
)

// NuevaRejilla crea la cuadrícula de ese libro.
func NuevaRejilla(l *Libro) *Rejilla {
	r := &Rejilla{L: l, Cols: MinColumnas, Fils: MinFilas, anclaMenu: nuevaAnclaMenu()}
	r.pistaComentario = nuevaPistaComentario()
	r.ExtendBaseWidget(r)
	r.ed = ui.NuevoEditorPlano(EscalonCelda, nil)
	r.ed.Tecla = r.teclaEnEdicion
	r.ed.Tabulador = true // el tabulador confirma y pasa a la derecha, no saca el foco
	r.ed.Objeto().Hide()
	return r
}

// Seleccion es la celda ancla.
func (r *Rejilla) Seleccion() Celda { return r.sel }

// Rango es el rectángulo seleccionado, ordenado. Sin rango extendido es la propia
// celda ancla, igual que en la web.
func (r *Rejilla) Rango() (c1, f1, c2, f2 int) {
	if r.ext == nil {
		return r.sel.C, r.sel.F, r.sel.C, r.sel.F
	}
	return min(r.sel.C, r.ext.C), min(r.sel.F, r.ext.F), max(r.sel.C, r.ext.C), max(r.sel.F, r.ext.F)
}

// Editando dice si hay una celda abierta para escribir.
func (r *Rejilla) Editando() bool { return r.editando }

// Seleccionar mueve la celda ancla (o extiende el rango hasta ahí) y la trae a la
// vista.
func (r *Rejilla) Seleccionar(c, f int, extender bool) {
	c, f = clamp(c, MaxColumnas-1), clamp(f, MaxFilas-1)
	if extender {
		r.ext = &Celda{C: c, F: f}
	} else {
		// Una celda cubierta por una combinada es su ancla: no se selecciona ni se escribe.
		if u, ok := r.L.FusionEn(c, f); ok {
			c, f = u.C1, u.F1
		}
		r.sel, r.ext = Celda{C: c, F: f}, nil
	}
	r.crecer(r.desX, r.desY) // la selección empuja la extensión: la celda tiene que existir
	r.aLaVista(c, f)
	if r.AlMoverse != nil {
		r.AlMoverse()
	}
	r.Refresh()
}

// Escribir pone el texto crudo de la celda ancla. Es por donde entra la barra de
// fórmulas.
//
// Devuelve false si no escribió: una validación ESTRICTA de la celda no admite ese
// valor, y entonces se avisa con su mensaje (`sheet.msg.badValue`) y la celda se queda
// como estaba.
func (r *Rejilla) Escribir(texto string) bool {
	if r.SoloLectura || r.protegida(r.sel) {
		return false
	}
	if r.rechazado(texto) {
		return false
	}
	r.L.Escribir(r.sel.C, r.sel.F, texto)
	if r.AlEditar != nil {
		r.AlEditar()
	}
	r.Refresh()
	return true
}

// rechazado mira la validación estricta de la celda ancla y avisa si no admite el texto.
// Lo que no cambia la celda no se valida, como en la web (`commitCell` sale antes).
func (r *Rejilla) rechazado(texto string) bool {
	if conTexto(r.L.HTML(r.sel.C, r.sel.F), texto) == r.L.HTML(r.sel.C, r.sel.F) {
		return false
	}
	msg, no := r.L.rechaza(r.sel.C, r.sel.F, texto)
	if no && r.AlRechazar != nil {
		r.AlRechazar(msg)
	}
	return no
}

// Editar abre la celda ancla para escribir, con su texto crudo dentro. `inicial`
// sustituye al contenido cuando se ha empezado tecleando, que es lo que hace la web:
// teclear sobre una celda la reemplaza, F2 la abre para corregir.
func (r *Rejilla) Editar(inicial string) {
	if r.SoloLectura || r.protegida(r.sel) {
		return
	}
	texto := r.L.Crudo(r.sel.C, r.sel.F)
	if inicial != "" {
		texto = inicial
	}
	r.editando = true
	// La celda abierta es la MISMA caja con su letra: un 14 pt se escribe a 14 pt (la
	// familia, la negrita y la cursiva las pone `colocarEditor`).
	r.ed.Escalon(escalonCelda(r.L.Estilo(r.sel.C, r.sel.F), r.z()))
	r.ed.PonerTexto(texto)
	// PonerTexto deja el cursor al final DESPUÉS de avisar del cambio, y colocar el
	// cursor a mano no avisa: quien sigue el cursor (la edición de fórmulas) se
	// quedaría con el de antes.
	if f := r.ed.OnCursorChanged; f != nil {
		f()
	}
	r.ed.Objeto().Show()
	if c := fyne.CurrentApp().Driver().CanvasForObject(r); c != nil {
		c.Focus(r.ed)
	}
	r.Refresh()
}

// Confirmar guarda lo escrito y mueve la selección (dc, df).
func (r *Rejilla) Confirmar(dc, df int) {
	if !r.guardarEdicion() {
		return
	}
	r.recuperarFoco()
	// Intro y Tab desde una combinada salen de ella entera, como las flechas.
	d := moverPorFusiones(r.L.Fusiones(), r.sel, dc, df, MaxColumnas-1, MaxFilas-1)
	r.Seleccionar(d.C, d.F, false)
}

// guardarEdicion cierra la celda abierta guardando lo escrito, sin mover la selección ni
// el foco: es el `commitCell` del `blur`. Devuelve si había una abierta.
func (r *Rejilla) guardarEdicion() bool {
	if !r.editando {
		return false
	}
	r.editando = false
	r.ed.Objeto().Hide()
	r.cerrarEdicion()
	// Lo rechazado no se escribe y la celda vuelve a enseñar lo que tenía, pero la
	// selección se mueve igual: es `finishEdit` tras un `commitCell` que no escribió.
	if r.L.Editable(r.sel.C, r.sel.F) && !r.rechazado(r.ed.Texto()) {
		r.L.Escribir(r.sel.C, r.sel.F, r.ed.Texto())
		if r.AlEditar != nil {
			r.AlEditar()
		}
	}
	return true
}

// Cancelar cierra la celda sin guardar (Escape).
func (r *Rejilla) Cancelar() {
	if !r.editando {
		return
	}
	r.editando = false
	r.ed.Objeto().Hide()
	r.cerrarEdicion()
	r.recuperarFoco()
	r.Refresh()
}

func (r *Rejilla) cerrarEdicion() {
	if r.AlCerrarEdicion != nil {
		r.AlCerrarEdicion()
	}
}

func (r *Rejilla) recuperarFoco() {
	if c := fyne.CurrentApp().Driver().CanvasForObject(r); c != nil {
		c.Focus(r)
	}
}

// BorrarRango vacía las celdas del rango (Supr), en un solo paso de deshacer.
func (r *Rejilla) BorrarRango() {
	c1, f1, c2, f2 := r.Rango()
	if r.SoloLectura || r.bloqueado(c1, f1, c2, f2) {
		return
	}
	r.L.Vaciar(r.Rango())
	if r.AlEditar != nil {
		r.AlEditar()
	}
	r.Refresh()
}

// protegida dice si la hoja protegida no deja escribir en la celda, y lo avisa.
func (r *Rejilla) protegida(c Celda) bool {
	if r.L.Editable(c.C, c.F) {
		return false
	}
	if r.AlProtegida != nil {
		r.AlProtegida(c)
	}
	return true
}

// bloqueado es lo mismo para un rango: basta una celda bloqueada.
func (r *Rejilla) bloqueado(c1, f1, c2, f2 int) bool {
	if !r.L.rangoBloqueado(c1, f1, c2, f2) {
		return false
	}
	if r.AlProtegida != nil {
		r.AlProtegida(Celda{C: c1, F: f1})
	}
	return true
}

// ---- desplazamiento ----

// Scrolled es la rueda. Implementarlo es además lo que hace que Fyne recorte el
// contenido a la caja de la rejilla (ver el comentario del tipo).
func (r *Rejilla) Scrolled(e *fyne.ScrollEvent) {
	r.cerrarPistaComentario() // la celda de debajo se ha movido
	r.Desplazar(-e.Scrolled.DX, -e.Scrolled.DY)
}

// Desplazar corre la vista y la deja dentro de los topes. dx y dy van en píxeles de la
// pantalla; el desplazamiento se guarda en los de la HOJA —sin zoom—, que es lo que no
// cambia al hacer zoom.
func (r *Rejilla) Desplazar(dx, dy float32) {
	z := r.z()
	// Primero la extensión hasta donde se quiere llegar, y luego los topes: así la rueda
	// sigue bajando pasada la fila 100, como en la web, en vez de chocar con la rejilla.
	r.crecer(max(r.desX+dx/z, 0), max(r.desY+dy/z, 0))
	r.desX = clampF(r.desX+dx/z, 0, r.maxDesX())
	r.desY = clampF(r.desY+dy/z, 0, r.maxDesY())
	r.Refresh()
}

// crecer recalcula la extensión (Cols, Fils) con la vista desplazada a (desX, desY).
// La vista va en filas y columnas DE LA HOJA, no en puestos de pantalla: con filas
// ocultas por el filtro o el esquema, lo que se ve llega más abajo de lo que mide.
func (r *Rejilla) crecer(desX, desY float32) {
	uc, uf := r.L.Usado()
	pc, pf := r.sel.C+1, r.sel.F+1
	if r.ext != nil {
		pc, pf = max(pc, r.ext.C+1), max(pf, r.ext.F+1)
	}
	t, z := r.Size(), r.z()
	mc, m := r.mapaCols(), r.mapa()
	der := mc.fila(mc.puestosHasta(desX + t.Width/z))
	abajo := m.fila(m.puestosHasta(desY + t.Height/z))
	vista := Vista{W: float32(der) * AnchoCelda, H: float32(abajo) * AltoCelda}
	r.Cols, r.Fils = extension(uc, uf, pc, pf, vista, AnchoCelda, AltoCelda)
}

// maxDesX y maxDesY son los topes: la hoja entera cabe justo en la ventana. Las
// columnas y filas inmovilizadas no cambian nada —en la web son `sticky`, que no
// alteran lo que mide el contenido—.
func (r *Rejilla) maxDesX() float32 {
	cx, _ := r.canal()
	return max(r.mapaCols().borde(r.Cols)+AnchoCabecera+cx-r.Size().Width/r.z(), 0)
}

func (r *Rejilla) maxDesY() float32 {
	_, cy := r.canal()
	return max(r.mapa().borde(r.Fils)+AltoCelda+cy-r.Size().Height/r.z(), 0)
}

// mapa son las filas que se ven en la hoja activa —sin las que esconden el autofiltro y
// el esquema— con sus altos propios.
func (r *Rejilla) mapa() eje {
	_, altos := r.L.Medidas()
	return nuevoEje(nuevoMapaFilas(r.L.FilasOcultas()), r.conPrevia(false, altos), AltoCelda, AltoMax)
}

// mapaCols son las columnas que se ven —sin las de un grupo contraído— con sus anchos
// propios.
func (r *Rejilla) mapaCols() eje {
	anchos, _ := r.L.Medidas()
	return nuevoEje(nuevoMapaFilas(r.L.ColumnasOcultas()), r.conPrevia(true, anchos), AnchoCelda, AnchoMax)
}

// z es el zoom, con el cero tapado.
func (r *Rejilla) z() float32 {
	if r.Zoom <= 0 {
		return 1
	}
	return r.Zoom
}

// fijas son las columnas y filas inmovilizadas de la hoja activa, acotadas a la rejilla.
func (r *Rejilla) fijas() (cols, fils int) {
	fj := r.L.Fijas()
	return min(fj.Cols, r.Cols), min(fj.Rows, r.Fils)
}

// aLaVista corre lo justo para que una celda se vea entera, como hace el `scrollIntoView`
// implícito del navegador al mover el foco. Una celda de las inmovilizadas ya se ve
// siempre en ese eje; las demás tienen que caer FUERA de las inmovilizadas, que las
// taparían.
func (r *Rejilla) aLaVista(c, f int) {
	t := r.Size()
	if t.IsZero() {
		return
	}
	z := r.z()
	fc, ff := r.fijas()
	cx, cy := r.canal()
	if c >= fc {
		mc := r.mapaCols()
		x := mc.borde(c)
		r.desX = clampF(r.desX, x+mc.medida(c)+AnchoCabecera+cx-t.Width/z, x-mc.borde(fc))
	}
	if f >= ff {
		m := r.mapa()
		y := m.borde(f)
		r.desY = clampF(r.desY, y+m.medida(f)+AltoCelda+cy-t.Height/z, y-m.borde(ff))
	}
	r.desX, r.desY = clampF(r.desX, 0, r.maxDesX()), clampF(r.desY, 0, r.maxDesY())
}

// filasPorPagina es el `pageRows()` de la web: las filas que caben, menos una.
func (r *Rejilla) filasPorPagina() int {
	return max(1, int(r.Size().Height/r.z()/AltoCelda)-1)
}

// ---- puntero ----

// colEn y filaEn dicen qué columna y qué fila caen bajo una x o una y de la rejilla,
// sin contar las cabeceras. Sobre lo inmovilizado manda lo inmovilizado: está encima.
func (r *Rejilla) colEn(x float32) int {
	x /= r.z()
	fc, _ := r.fijas()
	cx, _ := r.canal()
	m := r.mapaCols()
	i := m.puestoEn(x - AnchoCabecera - cx)
	if i >= m.puesto(fc) {
		i = m.puestoEn(x - AnchoCabecera - cx + r.desX)
	}
	return m.fila(i)
}

func (r *Rejilla) filaEn(y float32) int {
	y /= r.z()
	_, ff := r.fijas()
	_, cy := r.canal()
	m := r.mapa()
	i := m.puestoEn(y - AltoCelda - cy)
	if i >= m.puesto(ff) {
		i = m.puestoEn(y - AltoCelda - cy + r.desY)
	}
	return m.fila(i)
}

// Lo que hay bajo un punto de la rejilla.
const (
	zonaFuera = iota
	zonaCelda
	zonaCabCol  // la cabecera de una columna
	zonaCabFila // la de una fila
	zonaEsquina
	zonaValores // el botón «▾» de la validación de lista
	zonaFiltro  // el «▾» del autofiltro; la celda dice de qué columna
	zonaEsquema // un botón del canalón del esquema: ver esquemaEn
)

// zonaEn dice qué hay bajo un punto y en qué celda cae (en una cabecera, la de su
// columna o su fila; la otra coordenada es 0).
func (r *Rejilla) zonaEn(p fyne.Position) (int, Celda) {
	if zona, c, ok := r.zonaBoton(p); ok {
		return zona, c
	}
	z := r.z()
	cx, cy := r.canal()
	x, y := p.X/z, p.Y/z
	cab, izq := y < cy+AltoCelda, x < cx+AnchoCabecera
	// El canalón no es de nadie fuera de sus botones; la esquina, en cambio, lo cruza.
	if y < cy || (!cab && x < cx) {
		return zonaFuera, Celda{}
	}
	c, f := r.colEn(p.X), r.filaEn(p.Y)
	switch {
	case cab && izq:
		return zonaEsquina, Celda{}
	case cab && c >= 0 && c < r.Cols:
		return zonaCabCol, Celda{C: c}
	case izq && f >= 0 && f < r.Fils:
		return zonaCabFila, Celda{F: f}
	case !cab && !izq && c >= 0 && c < r.Cols && f >= 0 && f < r.Fils:
		return zonaCelda, Celda{C: c, F: f}
	}
	return zonaFuera, Celda{}
}

// zonaBoton es lo que hay encima de las celdas: el «▾» de una lista, el del
// autofiltro y los botones del canalón del esquema.
func (r *Rejilla) zonaBoton(p fyne.Position) (int, Celda, bool) {
	if pos, tam, ok := r.botonValores(); ok && dentro(p, pos, tam) {
		return zonaValores, r.sel, true
	}
	if c, ok := r.filtroEn(p); ok {
		return zonaFiltro, Celda{C: c}, true
	}
	if _, ok := r.esquemaEn(p); ok {
		return zonaEsquema, Celda{}, true
	}
	return 0, Celda{}, false
}

// dentro dice si un punto cae en la caja de `pos` y `tam`.
func dentro(p, pos fyne.Position, tam fyne.Size) bool {
	return p.X >= pos.X && p.X < pos.X+tam.Width && p.Y >= pos.Y && p.Y < pos.Y+tam.Height
}

// celdaEn dice qué celda cae bajo un punto, o false si es cabecera o está fuera.
func (r *Rejilla) celdaEn(p fyne.Position) (Celda, bool) {
	zona, c := r.zonaEn(p)
	return c, zona == zonaCelda
}

// MouseDown es el `mousedown` de la web, que es cuando se decide si el clic SEÑALA una
// referencia para la fórmula en edición: hay que saberlo antes de que Fyne le quite el
// foco al editor, que es justo después.
func (r *Rejilla) MouseDown(e *desktop.MouseEvent) {
	r.cerrarPistaComentario() // como el `title` del navegador, que se va al pulsar
	r.mayRaton = e.Modifier&fyne.KeyModifierShift != 0
	// Solo Ctrl/⌘ a secas, como la web: Ctrl+Mayús o Ctrl+Alt+clic no abren el enlace.
	r.ctrlRaton = e.Modifier&(fyne.KeyModifierControl|fyne.KeyModifierSuper) != 0 &&
		e.Modifier&(fyne.KeyModifierShift|fyne.KeyModifierAlt) == 0
	r.señalando, r.rellenando, r.destRell = false, false, nil
	// El botón secundario abre el menú (TappedSecondary): ni señala una referencia para
	// la fórmula ni empieza a arrastrar el asa.
	if e.Button == desktop.MouseButtonSecondary {
		return
	}
	if zona, _ := r.zonaEn(e.Position); zona == zonaValores || zona == zonaFiltro || zona == zonaEsquema {
		return
	}
	// El borde de una cabecera es su asa de medida: arrastrar redimensiona, no selecciona.
	if r.empezarMedida(e.Position) {
		return
	}
	if !r.editando && !r.SoloLectura && r.enAsa(e.Position) {
		c1, f1, c2, f2 := r.Rango()
		r.rellenando, r.origenRell = true, [4]int{c1, f1, c2, f2}
		return
	}
	if r.Señalar == nil {
		return
	}
	zona, c := r.zonaEn(e.Position)
	gesto := señalarPulsar
	if r.mayRaton {
		gesto = señalarExtender
	}
	r.señalando = r.Señalar(zona, c, gesto)
}

// MouseUp no hace nada: lo que toca lo hacen Tapped y DragEnd.
func (r *Rejilla) MouseUp(*desktop.MouseEvent) {}

// señalado cierra un clic o un arrastre que señaló: el foco vuelve al editor.
func (r *Rejilla) señalado() bool {
	if !r.señalando {
		return false
	}
	r.señalando = false
	if r.AlSeñalar != nil {
		r.AlSeñalar()
	}
	return true
}

// Tapped selecciona lo pulsado, como el `mousedown` de la web: una celda (con Mayús
// extiende el rango); la cabecera de una columna o de una fila, la columna o la fila
// ENTERA; y la esquina, la hoja completa.
func (r *Rejilla) Tapped(e *fyne.PointEvent) {
	ctrl := r.ctrlRaton
	defer func() { r.mayRaton, r.ctrlRaton = false, false }()
	if r.señalado() {
		return
	}
	if r.medida != nil { // pulsar el borde sin arrastrar no selecciona nada, como en Excel
		r.medida = nil
		return
	}
	if r.rellenando { // pulsar el asa sin arrastrar no hace nada
		r.rellenando, r.destRell = false, nil
		r.Refresh()
		return
	}
	// Tocar un tirador sin arrastrarlo no deshace el rango que lleva.
	if esMovil() && r.tiradorEn(e.Position) != tiradorNinguno {
		return
	}
	zona, c := r.zonaEn(e.Position)
	if zona == zonaValores {
		if r.AlBotonValores != nil {
			r.AlBotonValores()
		}
		return
	}
	if zona == zonaFiltro {
		if r.AlBotonFiltro != nil {
			r.AlBotonFiltro(c.C)
		}
		return
	}
	if zona == zonaEsquema {
		if valor, ok := r.esquemaEn(e.Position); ok && r.AlEsquema != nil {
			r.AlEsquema(valor)
		}
		return
	}
	if r.AlPulsarFuera != nil {
		r.AlPulsarFuera()
	}
	if r.editando {
		r.Confirmar(0, 0)
	}
	r.recuperarFoco()
	// Ctrl/⌘+clic sobre un enlace lo abre (Excel, Sheets y la web); el clic solo selecciona,
	// para poder seguir editando la celda sin salir disparado al navegador.
	if zona == zonaCelda && ctrl && r.L.Enlace(c.C, c.F) != "" {
		if r.L.Seleccionable(c.C, c.F) {
			r.Seleccionar(c.C, c.F, false)
		}
		r.AbrirEnlace(c.C, c.F)
		return
	}
	r.pulsar(zona, c)
}

// ListaValidacion es la validación de LISTA de la celda ancla, si la hay y se puede
// escribir en ella: es cuando la web pone el desplegable en la celda.
func (r *Rejilla) ListaValidacion() *Validacion {
	if r.editando || r.SoloLectura {
		return nil
	}
	v := r.L.validacionEn(r.sel.C, r.sel.F)
	if v == nil || (v.Tipo != "" && v.Tipo != "list") || len(v.Valores) == 0 {
		return nil
	}
	return v
}

// botonValores es dónde va el «▾»: pegado al borde derecho de la celda ancla, de su
// alto (`left: colX[sel.c + 1] - 18`).
func (r *Rejilla) botonValores() (fyne.Position, fyne.Size, bool) {
	if r.ListaValidacion() == nil {
		return fyne.Position{}, fyne.Size{}, false
	}
	vw := r.vista()
	return fyne.NewPos(vw.x(r.sel.C)+vw.ancho(r.sel.C)-AnchoBotonValores*vw.z, vw.y(r.sel.F)),
		fyne.NewSize(AnchoBotonValores*vw.z, vw.alto(r.sel.F)), true
}

// pulsar es lo que hace un clic en cada zona. El ancla de una columna entera baja a la
// fila 1, para que el rango cubra la columna y no medio rectángulo.
func (r *Rejilla) pulsar(zona int, c Celda) {
	mays := r.mayus()
	switch zona {
	case zonaCelda:
		// Hoja protegida sin «Seleccionar celdas»: la bloqueada no se selecciona.
		if r.L.Seleccionable(c.C, c.F) {
			r.Seleccionar(c.C, c.F, mays)
		}
		return
	case zonaCabCol:
		ancla := c.C
		if mays {
			ancla = r.sel.C
		}
		r.sel, r.ext = Celda{C: ancla}, &Celda{C: c.C, F: r.Fils - 1}
	case zonaCabFila:
		ancla := c.F
		if mays {
			ancla = r.sel.F
		}
		r.sel, r.ext = Celda{F: ancla}, &Celda{C: r.Cols - 1, F: c.F}
	case zonaEsquina:
		r.SeleccionarTodo()
		return
	default:
		return
	}
	if r.AlMoverse != nil {
		r.AlMoverse()
	}
	r.Refresh()
}

// DoubleTapped abre la celda para escribir, igual que el doble clic de la web.
//
// Sobre el borde de una cabecera, ajusta esa columna o esa fila a lo escrito.
func (r *Rejilla) DoubleTapped(e *fyne.PointEvent) {
	r.medida = nil
	if col, i, ok := r.asaMedidaEn(e.Position); ok {
		if !r.SoloLectura {
			r.redimensionar(col, i, medidaPedida{auto: true})
		}
		return
	}
	if c, ok := r.celdaEn(e.Position); ok {
		r.Seleccionar(c.C, c.F, false)
		r.Editar("")
	}
}

// Dragged marca el rango arrastrando, que es como se selecciona con el ratón. El
// arrastre empieza donde se pulsó: sobre una celda, marca celdas; sobre una cabecera,
// columnas o filas enteras. Volver a la celda ancla deshace el rango, como en la web.
func (r *Rejilla) Dragged(e *fyne.DragEvent) {
	if r.cancelado {
		return
	}
	if r.medida != nil {
		r.arrastrando = true
		r.arrastrarMedida(e.Position)
		return
	}
	// Con el dedo, arrastrar desplaza la hoja y el rango se marca con sus tiradores.
	if esMovil() && !r.rellenando && !r.señalando {
		if e = r.arrastrarTactil(e); e == nil {
			return
		}
	}
	r.arrastrarRango(e)
	if r.arrastre == zonaFuera && !r.rellenando && !r.señalando {
		return
	}
	r.arrastrando = true
	pos := e.AbsolutePosition
	if pos.IsZero() {
		pos = fyne.CurrentApp().Driver().AbsolutePositionForObject(r).Add(e.Position)
	}
	r.autoArrastre.Actualizar(r, pos, func(dx, dy float32) {
		r.Desplazar(dx, dy)
		inicio := fyne.CurrentApp().Driver().AbsolutePositionForObject(r)
		r.arrastrarRango(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: pos.Subtract(inicio), AbsolutePosition: pos}})
	})
}

func (r *Rejilla) arrastrarRango(e *fyne.DragEvent) {
	if r.rellenando {
		r.arrastrarAsa(Celda{C: clamp(r.colEn(e.Position.X), r.Cols-1), F: clamp(r.filaEn(e.Position.Y), r.Fils-1)})
		return
	}
	if r.señalando {
		c := Celda{C: clamp(r.colEn(e.Position.X), r.Cols-1), F: clamp(r.filaEn(e.Position.Y), r.Fils-1)}
		r.Señalar(zonaCelda, c, señalarArrastrar)
		return
	}
	if r.arrastre == zonaFuera {
		zona, c := r.zonaEn(e.Position.Subtract(e.Dragged))
		if zona == zonaFuera || zona == zonaEsquina {
			return
		}
		if r.editando {
			r.Confirmar(0, 0)
		}
		r.recuperarFoco()
		r.arrastre = zona
		r.pulsar(zona, c)
	}
	var hasta Celda
	switch r.arrastre {
	case zonaCelda:
		c, ok := r.celdaEn(e.Position)
		if !ok || !r.L.Seleccionable(c.C, c.F) {
			return
		}
		hasta = c
	case zonaCabCol:
		hasta = Celda{C: clamp(r.colEn(e.Position.X), r.Cols-1), F: r.Fils - 1}
	case zonaCabFila:
		hasta = Celda{C: r.Cols - 1, F: clamp(r.filaEn(e.Position.Y), r.Fils-1)}
	default:
		return
	}
	if r.arrastre == zonaCelda && hasta == r.sel {
		r.ext = nil
	} else {
		r.ext = &hasta
	}
	if r.tirador == tiradorInicio {
		r.recogerEnFusion()
	}
	if r.AlMoverse != nil {
		r.AlMoverse()
	}
	r.Refresh()
}

// DragEnd suelta el arrastre: el rango ya está puesto.
func (r *Rejilla) DragEnd() {
	r.autoArrastre.Parar()
	r.arrastrando, r.tirador = false, tiradorSinDecidir
	if r.cancelado {
		r.cancelado = false
		return
	}
	if r.medida != nil {
		r.soltarMedida()
		return
	}
	r.arrastre, r.mayRaton = zonaFuera, false
	r.señalado()
	if r.rellenando {
		r.soltarAsa()
	}
}

// CancelarArrastre quita la previa del autorrelleno sin escribir en el libro ni
// cerrar una celda que se estuviera editando.
func (r *Rejilla) CancelarArrastre() {
	r.autoArrastre.Parar()
	r.tirador = tiradorSinDecidir
	if r.medida != nil { // la medida vuelve a la de antes: no se guardó nada
		r.medida = nil
		r.cancelado = r.cancelado || r.arrastrando
		r.arrastrando = false
		r.Refresh()
		return
	}
	if r.arrastre == zonaFuera && !r.rellenando && !r.señalando {
		return
	}
	r.cancelado = r.cancelado || r.arrastrando
	r.arrastrando = false
	r.arrastre, r.mayRaton = zonaFuera, false
	r.rellenando, r.señalando, r.destRell = false, false, nil
	r.Refresh()
}

// vista es la geometría del pintado de ahora.
func (r *Rejilla) vista() vista {
	fc, ff := r.fijas()
	cx, cy := r.canal()
	return vista{
		z: r.z(), fc: fc, ff: ff, desX: r.desX * r.z(), desY: r.desY * r.z(), filas: r.mapa(),
		cols: r.mapaCols(), cx: cx, cy: cy, fus: r.L.Fusiones(),
	}
}

// LadoAsa es el `h-[7px] w-[7px]` del asa de autorrelleno, con su borde de un píxel
// dentro (`box-sizing: border-box`); va 4 px hacia dentro de la esquina del rango.
const (
	LadoAsa   float32 = 7
	RetiroAsa float32 = 4
)

// asa es dónde está el asa de autorrelleno: la esquina de abajo a la derecha del rango.
func (r *Rejilla) asa() (fyne.Position, float32) {
	vw := r.vista()
	_, _, c2, f2 := r.Rango()
	if u, ok := r.L.FusionEn(r.sel.C, r.sel.F); ok && r.ext == nil {
		c2, f2 = u.C2, u.F2 // el asa va en la esquina de la combinada, que es lo que se ve
	}
	return fyne.NewPos(vw.x(c2+1)-RetiroAsa*vw.z, vw.y(f2)+vw.alto(f2)-RetiroAsa*vw.z), LadoAsa * vw.z
}

func (r *Rejilla) enAsa(p fyne.Position) bool {
	pos, lado := r.asa()
	return p.X >= pos.X && p.X < pos.X+lado && p.Y >= pos.Y && p.Y < pos.Y+lado
}

// arrastrarAsa es `onFillMove`: una sola dirección, la vertical primero (criterio de
// Excel), desde el origen hasta la celda bajo el puntero.
func (r *Rejilla) arrastrarAsa(p Celda) {
	o := r.origenRell
	var d *[4]int
	switch {
	case p.F > o[3]:
		d = &[4]int{o[0], o[3] + 1, o[2], p.F}
	case p.F < o[1]:
		d = &[4]int{o[0], p.F, o[2], o[1] - 1}
	case p.C > o[2]:
		d = &[4]int{o[2] + 1, o[1], p.C, o[3]}
	case p.C < o[0]:
		d = &[4]int{p.C, o[1], o[0] - 1, o[3]}
	}
	if (d == nil) != (r.destRell == nil) || (d != nil && *d != *r.destRell) {
		r.destRell = d
		r.Refresh()
	}
}

// soltarAsa es `onFillUp`: rellena y deja seleccionado el origen más lo rellenado.
func (r *Rejilla) soltarAsa() {
	d, o := r.destRell, r.origenRell
	r.rellenando, r.destRell = false, nil
	if d == nil {
		r.Refresh()
		return
	}
	if r.bloqueado(d[0], d[1], d[2], d[3]) {
		r.Refresh()
		return
	}
	r.L.Rellenar(o, *d)
	r.sel = Celda{C: min(o[0], d[0]), F: min(o[1], d[1])}
	r.ext = &Celda{C: max(o[2], d[2]), F: max(o[3], d[3])}
	if r.AlEditar != nil {
		r.AlEditar()
	}
	r.Refresh()
}

// Cursor es la cruz de una hoja de cálculo (`cursor-cell`), salvo sobre el borde de una
// cabecera, que es el de redimensionar (`cursor-col-resize`, `cursor-row-resize`).
func (r *Rejilla) Cursor() desktop.Cursor {
	if c, ok := r.cursorMedida(); ok {
		return c
	}
	return desktop.CrosshairCursor
}

// ---- teclado ----

// FocusGained enciende el aro de foco de la rejilla.
func (r *Rejilla) FocusGained() { r.enfocado = true; r.Refresh() }

// FocusLost lo apaga.
func (r *Rejilla) FocusLost() {
	r.autoArrastre.Parar()
	if r.arrastre != zonaFuera || r.rellenando {
		r.CancelarArrastre()
	}
	r.enfocado = false
	r.Refresh()
}

// AcceptsTab se queda con el tabulador: en una hoja de cálculo pasa a la celda de al
// lado, como en la web. Para salir de la rejilla con el teclado está Escape.
func (r *Rejilla) AcceptsTab() bool { return true }

// KeyDown solo sigue la tecla Mayús: `fyne.KeyEvent` no trae modificadores, así que
// hay que recordarla, que es lo que hace también `widget.Entry`.
func (r *Rejilla) KeyDown(k *fyne.KeyEvent) {
	if k.Name == desktop.KeyShiftLeft || k.Name == desktop.KeyShiftRight {
		r.mays = true
	}
}

// KeyUp la suelta.
func (r *Rejilla) KeyUp(k *fyne.KeyEvent) {
	if k.Name == desktop.KeyShiftLeft || k.Name == desktop.KeyShiftRight {
		r.mays = false
	}
}

// TypedRune empieza a editar con lo tecleado, como en Excel y en la web.
func (r *Rejilla) TypedRune(ru rune) {
	// Mayús+Espacio marca las filas enteras (Excel): sin esto empezaba a escribir un espacio.
	if ru == ' ' && r.mayus() {
		r.SeleccionarEnteras(true)
		return
	}
	if ru >= ' ' {
		r.Editar(string(ru))
	}
}

// mayus dice si Mayús está pulsada: la recordada o, en una ventana de verdad, la que
// dice el sistema (la celda abierta se queda con las teclas y no se entera).
func (r *Rejilla) mayus() bool {
	if r.mays || r.mayRaton {
		return true
	}
	d, ok := fyne.CurrentApp().Driver().(desktop.Driver)
	return ok && d.CurrentKeyModifiers()&fyne.KeyModifierShift != 0
}

// TypedKey es la navegación del §7.1: flechas mueven, Mayús extiende, Inicio va a la
// columna A y Fin a la última celda con algo de la fila, AvPág/RePág saltan una
// pantalla, Intro y F2 abren la celda, Supr vacía el rango.
//
// El cuerpo es una tabla de saltos y no tiene ramas anidadas a propósito: la
// complejidad ciclomática del linter se mide por función y esto es un `switch` largo,
// no lógica enredada.
func (r *Rejilla) TypedKey(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyEscape {
		r.CancelarArrastre()
	}
	if r.teclaMenu(k) {
		return
	}
	ext := r.mayus()
	base := r.sel
	if r.ext != nil && ext {
		base = *r.ext
	}
	switch k.Name {
	case fyne.KeyUp:
		r.mover(base, 0, -1, ext)
	case fyne.KeyDown:
		r.mover(base, 0, 1, ext)
	case fyne.KeyLeft:
		r.mover(base, -1, 0, ext)
	case fyne.KeyRight:
		r.mover(base, 1, 0, ext)
	case fyne.KeyHome:
		r.mover(base, -base.C, 0, ext)
	case fyne.KeyEnd:
		fin, ok := r.L.UltimaUsada(base.F, true)
		if !ok {
			fin = base
		}
		r.Seleccionar(fin.C, fin.F, ext)
	case fyne.KeyPageUp:
		r.mover(base, 0, -r.filasPorPagina(), ext)
	case fyne.KeyPageDown:
		r.mover(base, 0, r.filasPorPagina(), ext)
	case fyne.KeyTab:
		// Mayús+Tab retrocede, como en la web.
		if ext {
			r.mover(r.sel, -1, 0, false)
		} else {
			r.mover(r.sel, 1, 0, false)
		}
	case fyne.KeyReturn, fyne.KeyEnter, fyne.KeyF2:
		r.Editar("")
	case fyne.KeyF5:
		r.enfocarCuadro()
	case fyne.KeyDelete, fyne.KeyBackspace:
		r.BorrarRango()
	case fyne.KeyEscape:
		r.escape()
	default: // el resto de teclas no navegan; las letras entran por TypedRune
	}
}

// mover es `move`: (dc, df) desde `base`. Si el destino no se puede seleccionar —hoja
// protegida sin «Seleccionar celdas»—, sigue en esa dirección hasta la primera que sí,
// como Excel; si no hay ninguna antes del borde, no se mueve.
func (r *Rejilla) mover(base Celda, dc, df int, ext bool) {
	// Salir de una combinada la salta entera, y caer en una lleva a su ancla (Excel).
	d := moverPorFusiones(r.L.Fusiones(), base, dc, df, MaxColumnas-1, MaxFilas-1)
	c, f := d.C, d.F
	pc, pf := signo(dc), signo(df)
	for !r.L.Seleccionable(c, f) {
		c, f = c+pc, f+pf
		if (pc == 0 && pf == 0) || c < 0 || c >= MaxColumnas || f < 0 || f >= MaxFilas {
			return
		}
	}
	r.Seleccionar(c, f, ext)
}

func signo(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}

// escape cierra primero el rango y, si no hay, SUELTA el foco: Tab navega entre
// celdas, así que sin esto no habría forma de salir de la rejilla con el teclado
// (trampa de teclado, WCAG 2.1.2). Es lo que hace la web.
func (r *Rejilla) escape() {
	if r.AlPulsarFuera != nil && r.AlPulsarFuera() {
		return
	}
	if r.ext != nil {
		r.ext = nil
		if r.AlMoverse != nil {
			r.AlMoverse()
		}
		r.Refresh()
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(r); c != nil {
		c.Unfocus()
	}
}

// SeleccionarTodo es Ctrl+A: la rejilla entera, no el texto de la ventana.
func (r *Rejilla) SeleccionarTodo() {
	r.sel, r.ext = Celda{}, &Celda{C: r.Cols - 1, F: r.Fils - 1}
	if r.AlMoverse != nil {
		r.AlMoverse()
	}
	r.Refresh()
}

// TypedShortcut son los atajos de `onGridKeyDown`: copiar, cortar, pegar (y pegar solo
// valores con Mayús), seleccionar todo, deshacer y rehacer.
func (r *Rejilla) TypedShortcut(s fyne.Shortcut) {
	switch a := s.(type) {
	case *fyne.ShortcutCopy:
		r.Copiar(a.Clipboard, false)
	case *fyne.ShortcutCut:
		r.Copiar(a.Clipboard, true)
	case *fyne.ShortcutPaste:
		r.Pegar(a.Clipboard.Content(), false)
	case *fyne.ShortcutSelectAll:
		r.SeleccionarTodo()
	case *fyne.ShortcutUndo:
		r.deshacer(false)
	case *fyne.ShortcutRedo:
		r.deshacer(true)
	case *desktop.CustomShortcut:
		if !r.atajoEnlace(a) {
			r.atajo(a)
		}
	}
}

// atajoEnlace es Alt+Intro: abre el enlace de la celda activa (el atajo de Sheets, como en
// la web). Sin enlace, la tecla sigue su camino.
func (r *Rejilla) atajoEnlace(a *desktop.CustomShortcut) bool {
	if a.Modifier != fyne.KeyModifierAlt || (a.KeyName != fyne.KeyReturn && a.KeyName != fyne.KeyEnter) {
		return false
	}
	return r.AbrirEnlace(r.sel.C, r.sel.F)
}

// atajo son los atajos con Ctrl (⌘ en el Mac) que Fyne no trae con nombre: Ctrl+Mayús+Z
// rehace, Ctrl+Mayús+V pega solo valores, Ctrl+B y Ctrl+I son negrita y cursiva, y
// Ctrl+Inicio y Ctrl+Fin van a A1 y a la última celda con algo de la hoja, y
// Ctrl+flecha salta al borde del bloque de datos (con Mayús, extiende).
func (r *Rejilla) atajo(a *desktop.CustomShortcut) {
	if r.Contexto.AtajoDelEditor(a) || r.atajoDeHoja(a.KeyName, a.Modifier) {
		return
	}
	ctrl := fyne.KeyModifierShortcutDefault
	switch {
	case a.Modifier == ctrl|fyne.KeyModifierShift && a.KeyName == fyne.KeyZ:
		r.deshacer(true)
	case a.Modifier == ctrl|fyne.KeyModifierShift && a.KeyName == fyne.KeyV:
		r.Pegar(fyne.CurrentApp().Clipboard().Content(), true)
	case a.Modifier == ctrl && (a.KeyName == fyne.KeyB || a.KeyName == fyne.KeyI):
		r.NegritaOCursiva(a.KeyName == fyne.KeyB)
	case a.Modifier&^fyne.KeyModifierShift == ctrl && esFlecha(a.KeyName):
		r.saltarADatos(a.KeyName, a.Modifier&fyne.KeyModifierShift != 0)
	case a.Modifier&ctrl != 0 && (a.KeyName == fyne.KeyHome || a.KeyName == fyne.KeyEnd):
		ext := a.Modifier&fyne.KeyModifierShift != 0
		if a.KeyName == fyne.KeyHome {
			r.Seleccionar(0, 0, ext)
			return
		}
		fin, ok := r.L.UltimaUsada(0, false)
		if !ok {
			fin = r.sel
			if ext && r.ext != nil {
				fin = *r.ext
			}
		}
		r.Seleccionar(fin.C, fin.F, ext)
	}
}

func esFlecha(k fyne.KeyName) bool {
	return k == fyne.KeyUp || k == fyne.KeyDown || k == fyne.KeyLeft || k == fyne.KeyRight
}

// saltarADatos es `moveToData`: Ctrl+flecha con el criterio de Excel (`saltoADatos`)
// sobre las celdas con algo de la fila o la columna de la celda activa. Pasa por
// `mover`, así que en una hoja protegida respeta lo que no se puede seleccionar.
func (r *Rejilla) saltarADatos(k fyne.KeyName, ext bool) {
	base := r.sel
	if ext && r.ext != nil {
		base = *r.ext
	}
	switch k {
	case fyne.KeyUp, fyne.KeyDown:
		paso := 1
		if k == fyne.KeyUp {
			paso = -1
		}
		r.mover(base, 0, saltoADatos(r.L.llenasEn(base.C, 0, true), base.F, paso, MaxFilas)-base.F, ext)
	default:
		paso := 1
		if k == fyne.KeyLeft {
			paso = -1
		}
		r.mover(base, saltoADatos(r.L.llenasEn(0, base.F, false), base.C, paso, MaxColumnas)-base.C, 0, ext)
	}
}

// NegritaOCursiva conmuta la negrita (o la cursiva) del rango: el comando `bold` o
// `italic` de la web, que también es Ctrl+B y Ctrl+I.
func (r *Rejilla) NegritaOCursiva(negrita bool) {
	if r.SoloLectura {
		return
	}
	c1, f1, c2, f2 := r.Rango()
	r.L.NegritaOCursiva(negrita, r.sel.C, r.sel.F, c1, f1, c2, f2)
	if r.AlEditar != nil {
		r.AlEditar()
	}
	r.Refresh()
}

func (r *Rejilla) deshacer(rehacer bool) {
	if r.AlDeshacer != nil {
		r.AlDeshacer(rehacer)
	}
}

// teclaEnEdicion son las cuatro teclas que la celda abierta NO le deja al editor.
// Devuelve true si se la queda.
//
// Las flechas verticales confirman y mueven —el modo «Modificar» de Excel, que es lo
// que hace la web— salvo si lo que se está escribiendo es una fórmula: ahí las flechas
// sirven para señalar referencias en la rejilla, y comérselas rompería la otra mitad
// del comportamiento de una hoja de cálculo.
//
// ponytail: señalar referencias con las flechas todavía no está; lo que se respeta hoy
// es no confirmar a mitad de una fórmula, que es lo que destruiría lo escrito. Se
// completa cuando entre el editor de fórmulas con sus marcos de colores.
func (r *Rejilla) teclaEnEdicion(k *fyne.KeyEvent) bool {
	enFormula := strings.HasPrefix(r.ed.Texto(), "=")
	switch k.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		r.Confirmar(0, 1)
		return true
	case fyne.KeyTab:
		// Mayús+Tab confirma y retrocede.
		if r.mayus() {
			r.Confirmar(-1, 0)
		} else {
			r.Confirmar(1, 0)
		}
		return true
	case fyne.KeyEscape:
		r.Cancelar()
		return true
	case fyne.KeyUp, fyne.KeyDown:
		if enFormula {
			return false
		}
		df := 1
		if k.Name == fyne.KeyUp {
			df = -1
		}
		r.Confirmar(0, df)
		return true
	default:
		return false
	}
}

// ---- accesibilidad ----

// AccessibilityLabel anuncia la celda ancla y lo que hay en ella, que es lo que el
// `aria-activedescendant` de la web le da al lector de pantalla. Sin esto, moverse con
// las flechas no dice nada.
//
// Con comentarios, detrás va lo que dice su marca: es el `aria-description` de la celda.
func (r *Rejilla) AccessibilityLabel() string {
	ref := Ref(r.sel.C, r.sel.F)
	if texto := r.L.Texto(r.sel.C, r.sel.F); texto != "" {
		ref += ": " + texto
	}
	if len(r.comentarios) > 0 {
		k := r.sel
		if fu, es := r.L.FusionEn(k.C, k.F); es {
			k = Celda{C: fu.C1, F: fu.F1}
		}
		if primero, n := r.L.marcaDeComentario(r.comentarios, k); n > 0 {
			ref += ". " + TextoMarcaComentario(r.T, primero, n)
		}
	}
	return ref
}

// AccessibilityRole es contenedor: Fyne v2.8 solo tiene cuatro roles y no hay «grid».
func (r *Rejilla) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// MinSize es una cabecera y una fila: lo mínimo en lo que una rejilla se reconoce. El
// tamaño real lo pone quien la coloca, como el `overflow-auto` de la web.
func (r *Rejilla) MinSize() fyne.Size {
	r.ExtendBaseWidget(r)
	return fyne.NewSize(AnchoCabecera+AnchoCelda, 2*AltoCelda)
}

// clamp deja un índice dentro de la rejilla. El suelo es siempre 0 —no hay fila −1— y
// por eso no es un parámetro: un tercer argumento que siempre vale lo mismo es una
// ocasión de equivocarse, no una función más general.
func clamp(v, hi int) int { return min(max(v, 0), hi) }

func clampF(v, lo, hi float32) float32 { return min(max(v, lo), hi) }

// MouseIn, MouseMoved y MouseOut siguen el puntero por los «▾» de la rejilla, que en la
// web cambian con `hover:bg-selected hover:text-text`. Solo se repinta al cambiar de uno
// a otro: moverse por las celdas no cuesta nada.
func (r *Rejilla) MouseIn(e *desktop.MouseEvent) { r.seguirPuntero(e.Position) }

// MouseMoved sigue el puntero.
func (r *Rejilla) MouseMoved(e *desktop.MouseEvent) { r.seguirPuntero(e.Position) }

// MouseOut apaga el «▾» que estuviera encendido.
func (r *Rejilla) MouseOut() { r.seguirPuntero(fyne.NewPos(-1, -1)) }

func (r *Rejilla) seguirPuntero(p fyne.Position) {
	if p.X < 0 && p.Y < 0 {
		r.puntero = nil
	} else {
		r.puntero = &p
	}
	boton := 0
	if pos, tam, ok := r.botonValores(); ok && dentro(p, pos, tam) {
		boton = -1
	} else if c, ok := r.filtroEn(p); ok {
		boton = c + 1
	}
	if boton != r.boton {
		r.boton = boton
		r.Refresh()
	}
	r.seguirPistaComentario(p)
}
