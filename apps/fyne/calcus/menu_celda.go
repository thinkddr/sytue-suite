// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El menú contextual de la rejilla: clic derecho (o mantener pulsado en el teléfono, que
// Fyne entrega igual, como `TappedSecondary`), Mayús+F10 y la tecla Menú sobre la celda
// activa. Hasta ahora insertar o borrar una fila, ordenar o pegar solo valores exigía ir
// a la cinta, y ninguna hoja de cálculo se usa así.
//
// Todas las opciones son comandos que ya existen (`Mandar`), con el mismo nombre y valor
// que la cinta y que la web: el menú no sabe hacer nada que la cinta no sepa. Hay tres
// variantes, como en Excel y en Sheets: la de las celdas, la de la cabecera de columna
// y la de la cabecera de fila. Lo que la protección de la hoja (o el solo lectura) veta
// sale inerte y con su motivo en el nombre accesible, no escondido: esconderlo haría que
// el menú cambiara de forma según la hoja y el que lo busca no sabría por qué no está.

// Las tres variantes del menú.
const (
	menuCeldas = iota
	menuColumnas
	menuFilas
)

// opcionCtx es una entrada del menú. Sin clave, es un separador.
type opcionCtx struct {
	clave string
	cmd   editor.Comando
	valor string
	// borrar es «Borrar contenido», que no es un comando de la cinta sino el Supr de la
	// rejilla (`BorrarRango`).
	borrar bool
	vivo   bool
	// motivo es la clave del catálogo que dice por qué no vive; vacío si no vive porque
	// no haría nada (inmovilizar en A1), que no es una prohibición que explicar.
	motivo string
}

// separadorCtx separa los grupos del menú.
var separadorCtx = opcionCtx{}

// opcionesContextuales son las entradas del menú de una variante, para la selección y la
// protección de ahora. Es lo que se prueba sin pintar nada.
func (c *Lienzo) opcionesContextuales(zona int) []opcionCtx {
	r, l := c.Rejilla, c.Libro
	c1, f1, c2, f2 := r.Rango()
	// motivoDe dice por qué no se puede: el solo lectura antes que la protección, que es
	// el permiso más fuerte y el que no se arregla quitando una contraseña.
	motivoDe := func(vetado bool) (bool, string) {
		switch {
		case r.SoloLectura:
			return false, "public.readOnly"
		case vetado:
			return false, "sheet.msg.protectedSheet"
		}
		return true, ""
	}
	cmd := func(clave string, cmd editor.Comando, valor string) opcionCtx {
		o := opcionCtx{clave: clave, cmd: cmd, valor: valor}
		o.vivo, o.motivo = motivoDe(l.vetado(cmd))
		return o
	}
	bloqueado := l.rangoBloqueado(c1, f1, c2, f2)
	cortar := opcionCtx{clave: "cmd.cut", cmd: CmdCopiar, valor: "cut"}
	cortar.vivo, cortar.motivo = motivoDe(bloqueado) // cortar acaba vaciando el origen
	borrar := opcionCtx{clave: "sheet.ctx.clear", borrar: true}
	borrar.vivo, borrar.motivo = motivoDe(bloqueado)

	ops := []opcionCtx{
		cortar,
		{clave: "cmd.copy", cmd: CmdCopiar, vivo: true}, // copiar se puede siempre, como en la web
		cmd("cmd.paste", CmdPegar, ""),
		cmd("cmd.pasteValues", CmdPegar, "values"),
		separadorCtx,
	}
	filas := []opcionCtx{cmd("table.rowAbove", CmdInsFila, "above"), cmd("table.rowBelow", CmdInsFila, "below")}
	cols := []opcionCtx{cmd("table.colLeft", CmdInsCol, "left"), cmd("table.colRight", CmdInsCol, "right")}
	borrarFilas, borrarCols := cmd("table.deleteRows", CmdBorrarFilas, ""), cmd("table.deleteCols", CmdBorrarCols, "")
	ordenar := []opcionCtx{separadorCtx, cmd("data.sortAsc", CmdOrdenar, "asc"), cmd("data.sortDesc", CmdOrdenar, "desc")}
	switch zona {
	case menuColumnas:
		ops = append(ops, cols...)
		ops = append(ops, borrarCols, borrar)
		ops = append(ops, ordenar...)
	case menuFilas:
		// Sin ordenar: ordenar filas por una fila no es lo que nadie espera de ese menú,
		// y tampoco lo ofrece Excel en la cabecera de fila.
		ops = append(ops, filas...)
		ops = append(ops, borrarFilas, borrar)
	default:
		ops = append(ops, filas...)
		ops = append(ops, cols...)
		ops = append(ops, borrarFilas, borrarCols, borrar)
		ops = append(ops, ordenar...)
	}
	return append(ops, separadorCtx, c.opcionFijar(zona, c1, f1))
}

// opcionFijar es «Congelar hasta aquí», o «Descongelar paneles» si ya hay algo
// inmovilizado. Inmoviliza lo que queda por encima y a la izquierda de la esquina del
// rango, que es lo que hace el botón de la cinta (`toggleFreeze`) y Excel: en una
// cabecera de columna, las columnas de su izquierda; en una de fila, las de encima; en
// la celda A1, la primera fila, como el botón.
func (c *Lienzo) opcionFijar(zona, c1, f1 int) opcionCtx {
	o := opcionCtx{clave: "sheet.ctx.freeze", cmd: CmdFijar}
	if c.Libro.Fijas() != (Fijas{}) {
		o.clave, o.valor = "sheet.ctx.unfreeze", "0,0"
	} else {
		switch zona {
		case menuColumnas:
			f1 = 0
		case menuFilas:
			c1 = 0
		default:
			if c1 == 0 && f1 == 0 {
				f1 = 1 // en A1, la primera fila, como `toggleFreeze`
			}
		}
		o.valor = strconv.Itoa(c1) + "," + strconv.Itoa(f1)
	}
	if c.Rejilla.SoloLectura {
		o.motivo = "public.readOnly"
		return o
	}
	o.vivo = o.valor != "0,0" || o.clave == "sheet.ctx.unfreeze"
	return o
}

// rotuloMenu es el nombre accesible de cada variante.
var rotuloMenu = map[int]string{
	menuCeldas: "sheet.ctx.cells", menuColumnas: "sheet.ctx.cols", menuFilas: "sheet.ctx.rows",
}

// abrirMenuContextual despliega el menú de esa variante colgando de `ancla`, que la
// rejilla ya ha puesto donde toca (el puntero o la celda activa).
func (c *Lienzo) abrirMenuContextual(zona int, ancla fyne.CanvasObject) {
	if c.menuCtx != nil && c.menuCtx.Abierto() {
		c.menuCtx.Cerrar()
	}
	var elems []fyne.CanvasObject
	for _, o := range c.opcionesContextuales(zona) {
		if o.clave == "" {
			elems = append(elems, ui.NuevoSeparadorMenu())
			continue
		}
		texto := c.t(o.clave, nil)
		e := ui.NuevoElementoMenu(texto, iconos.Ninguno, c.accionContextual(o))
		e.Denso, e.Inerte = true, !o.vivo // el cromo denso de los editores, `disabled`
		if !o.vivo && o.motivo != "" {
			e.Rotulo = c.t("sheet.ctx.unavailable", i18n.Vars{"action": texto, "reason": c.t(o.motivo, nil)})
		}
		elems = append(elems, e)
	}
	m := ui.NuevoMenu(elems...)
	m.AlInicio, m.Ancla, m.Rotulo = true, ancla, c.t(rotuloMenu[zona], nil)
	c.menuCtx = m
	// La rejilla ya tiene el foco: el menú lo devuelve ahí al cerrarse, con Escape o
	// tras la opción, para seguir moviéndose con las flechas.
	m.Mostrar(c.Rejilla)
}

// accionContextual es lo que hace una opción: su comando, por el mismo `Mandar` que la
// cinta (con su cortafuegos de solo lectura y de protección).
func (c *Lienzo) accionContextual(o opcionCtx) func() {
	return func() {
		if o.borrar {
			c.Rejilla.BorrarRango()
			return
		}
		c.Mandar(o.cmd, o.valor)
		if c.AlSeleccionar != nil {
			c.AlSeleccionar() // insertar o borrar mueve la selección: la cinta se entera
		}
	}
}

// MenuContextual es el menú de la rejilla si está abierto (para las pruebas y el cromo).
func (c *Lienzo) MenuContextual() *ui.Menu {
	if c.menuCtx == nil || !c.menuCtx.Abierto() {
		return nil
	}
	return c.menuCtx
}

// ---- la rejilla: el gesto, la selección y el ancla ----

// TappedSecondary es el clic derecho del escritorio y mantener pulsado en el teléfono.
// Como en Excel, pulsar FUERA del rango seleccionado selecciona lo pulsado (la celda, la
// columna o la fila) y pulsar DENTRO lo conserva, para poder borrar o copiar un rango
// entero desde su menú.
func (r *Rejilla) TappedSecondary(e *fyne.PointEvent) {
	if r.AlMenu == nil {
		return
	}
	zona, c := r.zonaEn(e.Position)
	var menu int
	switch zona {
	case zonaCelda:
		menu = menuCeldas
	case zonaCabCol:
		menu = menuColumnas
	case zonaCabFila:
		menu = menuFilas
	case zonaEsquina:
		menu = menuCeldas
	default:
		return // los «▾», el canalón del esquema y lo de fuera no tienen menú
	}
	// El clic derecho no extiende nada: la Mayús de su MouseDown no se queda puesta
	// para la flecha siguiente (la de Tapped la limpia su `defer`).
	defer func() { r.mayRaton, r.ctrlRaton = false, false }()
	if r.AlPulsarFuera != nil {
		r.AlPulsarFuera()
	}
	if r.editando {
		r.Confirmar(0, 0)
	}
	r.recuperarFoco()
	r.seleccionarParaMenu(zona, c)
	r.anclaMenu.Move(e.Position)
	r.anclaMenu.Resize(fyne.Size{})
	r.abrirMenu(menu)
}

// seleccionarParaMenu es la selección que hace el clic derecho antes de abrir el menú.
func (r *Rejilla) seleccionarParaMenu(zona int, c Celda) {
	c1, f1, c2, f2 := r.Rango()
	enteras := func(cols bool) bool { // el rango ya son columnas (o filas) enteras
		if cols {
			return f1 == 0 && f2 >= r.Fils-1
		}
		return c1 == 0 && c2 >= r.Cols-1
	}
	switch zona {
	case zonaCelda:
		if (c.C < c1 || c.C > c2 || c.F < f1 || c.F > f2) && r.L.Seleccionable(c.C, c.F) {
			r.Seleccionar(c.C, c.F, false)
		}
	case zonaCabCol:
		if c.C < c1 || c.C > c2 || !enteras(true) {
			r.mayRaton, r.mays = false, false
			r.pulsar(zonaCabCol, c)
		}
	case zonaCabFila:
		if c.F < f1 || c.F > f2 || !enteras(false) {
			r.mayRaton, r.mays = false, false
			r.pulsar(zonaCabFila, c)
		}
	case zonaEsquina:
		r.SeleccionarTodo()
	}
}

// abrirMenuTeclado es Mayús+F10 y la tecla Menú: el menú de la selección, colgando de la
// celda activa (que antes se trae a la vista). Si el rango son columnas o filas enteras,
// es el menú de su cabecera, como si se hubiera pulsado allí.
func (r *Rejilla) abrirMenuTeclado() {
	if r.AlMenu == nil || r.editando {
		return
	}
	c1, f1, c2, f2 := r.Rango()
	menu := menuCeldas
	switch {
	case c1 == 0 && c2 >= r.Cols-1 && f1 == 0 && f2 >= r.Fils-1: // la hoja entera
	case f1 == 0 && f2 >= r.Fils-1:
		menu = menuColumnas
	case c1 == 0 && c2 >= r.Cols-1:
		menu = menuFilas
	}
	r.aLaVista(r.sel.C, r.sel.F)
	r.Refresh()
	vw := r.vista()
	r.anclaMenu.Move(fyne.NewPos(vw.x(r.sel.C), vw.y(r.sel.F)))
	r.anclaMenu.Resize(fyne.NewSize(vw.ancho(r.sel.C), vw.alto(r.sel.F)))
	r.abrirMenu(menu)
}

// abrirMenu cuelga el menú del ancla. El menú va en una capa con su propio foco, así
// que la rejilla no recibe el KeyUp de la Mayús de Mayús+F10 (ni FocusLost): si no la
// olvida aquí, al volver del menú las flechas y los clics seguirían extendiendo el
// rango. Si de verdad sigue pulsada, el driver de escritorio la ve (`mayus`).
func (r *Rejilla) abrirMenu(menu int) {
	r.mays = false
	r.AlMenu(menu, r.anclaMenu)
}

// teclaMenu atiende Mayús+F10 y la tecla Menú (WCAG 2.1.1); dice si era una de ellas.
func (r *Rejilla) teclaMenu(k *fyne.KeyEvent) bool {
	if k.Name == desktop.KeyMenu || (k.Name == fyne.KeyF10 && r.mayus()) {
		r.abrirMenuTeclado()
		return true
	}
	return false
}

// nuevaAnclaMenu es el punto transparente del que cuelga el menú: tiene que estar entre
// los objetos de la rejilla para que `ui.Menu` sepa dónde está en la ventana.
func nuevaAnclaMenu() *canvas.Rectangle {
	return canvas.NewRectangle(color.Transparent)
}
