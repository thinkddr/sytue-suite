// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Los comandos de la hoja: los `SheetCommand` de SheetCanvas.tsx que este lienzo sabe
// hacer, con el MISMO nombre y el mismo valor que la web —el mismo documento se edita
// en las dos, y dos vocabularios son dos comportamientos—. Llegan por Mandar, que es
// por donde la cinta del cromo los manda. `bold` e `italic` son los de la cinta genérica
// (`editor.Negrita` y `editor.Cursiva`): se llaman igual en la web.
const (
	CmdAlinear       editor.Comando = "align"      // "left", "center" o "right"
	CmdFormatoNum    editor.Comando = "numfmt"     // un preset, o "custom:<máscara>"
	CmdDecimales     editor.Comando = "decimals"   // "+1" o "-1"
	CmdEstilo        editor.Comando = "style"      // JSON de un Partial<CellStyle>; null quita
	CmdBordes        editor.Comando = "border"     // "all", "outer", "bottom" o "none"
	CmdQuitarEstil   editor.Comando = "clearstyle" // el formato visual fuera
	CmdCombinar      editor.Comando = "merge"
	CmdFijar         editor.Comando = "freeze"    // "cols,filas"; "0,0" descongela
	CmdCopiar        editor.Comando = "copy"      // "cut" corta
	CmdPegar         editor.Comando = "paste"     // "values" pega solo valores
	CmdOrdenar       editor.Comando = "sort"      // "asc" o "desc"
	CmdDuplicados    editor.Comando = "dedupe"    // quita las filas repetidas del rango
	CmdTextoCols     editor.Comando = "text2cols" // el valor es el separador
	CmdInsFila       editor.Comando = "insrow"    // "above" o "below"
	CmdInsCol        editor.Comando = "inscol"    // "left" o "right"
	CmdBorrarFilas   editor.Comando = "delrow"
	CmdBorrarCols    editor.Comando = "delcol"
	CmdCondAnadir    editor.Comando = "condadd"    // JSON de una CondRule
	CmdCondQuitar    editor.Comando = "conddel"    // índice de la regla
	CmdCondMover     editor.Comando = "condmove"   // "i,dir": dir −1 sube, +1 baja
	CmdValAnadir     editor.Comando = "validadd"   // JSON de una DataValidation
	CmdValQuitar     editor.Comando = "validdel"   // índice de la validación
	CmdBloque        editor.Comando = "bloquehoja" // JSON de un bloque reutilizable {cells, formats, styles, charts}
	CmdNombrePoner   editor.Comando = "nameset"    // JSON de una PeticionNombre: crea o edita
	CmdNombreQuitar  editor.Comando = "namedel"    // JSON {name, sheet}
	CmdAutofiltro    editor.Comando = "autofilter" // lo pone sobre el rango, o lo quita
	CmdFiltrar       editor.Comando = "filterset"  // JSON {col, value, hidden}
	CmdCondFiltro    editor.Comando = "filtercond" // JSON {col, cond}: cond null la quita
	CmdOrdenarFiltro editor.Comando = "filtersort" // JSON {col, dir}: dir "asc" o "desc"
)

// Mandar ejecuta un comando de la cinta sobre la selección. El cortafuegos de solo
// lectura está en el cromo (`Contexto.Mandar`) y otra vez aquí: la rejilla también
// manda comandos con el teclado.
func (c *Lienzo) Mandar(cmd editor.Comando, valor string) {
	r := c.Rejilla
	switch cmd {
	case editor.Deshacer, editor.Rehacer:
		c.Deshacer(cmd == editor.Rehacer)
		return
	case CmdCopiar:
		r.Copiar(fyne.CurrentApp().Clipboard(), valor == "cut")
		return
	default: // el resto escribe: se mira abajo
	}
	if r.SoloLectura || c.mandarProteccion(cmd, valor) {
		return
	}
	// La hoja protegida veta lo que edita celdas, el formato sin su permiso y lo que
	// tiene permiso propio sin él.
	if c.Libro.vetado(cmd) {
		c.avisarProtegida(r.sel)
		return
	}
	if cmd == CmdPegar {
		r.Pegar(fyne.CurrentApp().Clipboard().Content(), valor == "values")
		return // Pegar ya avisa
	}
	c1, f1, c2, f2 := r.Rango()
	if c.mandarFormato(cmd, valor, c1, f1, c2, f2) || c.mandarDatos(cmd, valor, c1, f1, c2, f2) ||
		c.mandarEsquema(cmd, valor, c1, f1, c2, f2) || c.mandarDinamica(cmd, valor) ||
		c.mandarHipotesis(cmd, valor) || c.mandarRelleno(cmd, valor) {
		c.trasCambio()
	}
}

// mandarFormato son los comandos de formato de la pestaña Inicio. Devuelve false si
// no es uno de ellos.
func (c *Lienzo) mandarFormato(cmd editor.Comando, valor string, c1, f1, c2, f2 int) bool {
	r, l := c.Rejilla, c.Libro
	switch cmd {
	case editor.Negrita:
		l.NegritaOCursiva(true, r.sel.C, r.sel.F, c1, f1, c2, f2)
	case editor.Cursiva:
		l.NegritaOCursiva(false, r.sel.C, r.sel.F, c1, f1, c2, f2)
	case CmdAlinear:
		l.Alinear(valor, c1, f1, c2, f2)
	case editor.AlinearIzq, editor.AlinearCen, editor.AlinearDer:
		l.Alinear(map[editor.Comando]string{editor.AlinearCen: "center", editor.AlinearDer: "right"}[cmd], c1, f1, c2, f2)
	case CmdFormatoNum:
		if mascara, ok := strings.CutPrefix(valor, "custom:"); ok {
			l.PonerFormatoNumero("custom", mascara, c1, f1, c2, f2)
		} else {
			l.PonerFormatoNumero(valor, "", c1, f1, c2, f2)
		}
	case CmdDecimales:
		l.Decimales(map[bool]int{true: -1, false: 1}[valor == "-1"], c1, f1, c2, f2)
	case CmdEstilo:
		p, ok := parcheJSON(valor)
		if !ok {
			return false
		}
		l.CambiarEstilo(p, c1, f1, c2, f2)
	case editor.Subrayado, editor.Tachado:
		// Los de la cinta genérica: conmutan según la celda ancla, como los de InicioSheets.
		e := l.Estilo(r.sel.C, r.sel.F)
		if cmd == editor.Subrayado {
			l.CambiarEstilo(conSubraya(!e.Subraya), c1, f1, c2, f2)
		} else {
			l.CambiarEstilo(conTacha(!e.Tacha), c1, f1, c2, f2)
		}
	case CmdBordes:
		l.PonerBordes(valor, c1, f1, c2, f2)
	case CmdQuitarEstil, editor.SinFormato:
		l.QuitarEstilo(c1, f1, c2, f2)
	case CmdCombinar:
		if l.Combinar(c1, f1, c2, f2) {
			r.sel, r.ext = Celda{C: c1, F: f1}, nil
		}
	case CmdFijar:
		cols, filas, _ := strings.Cut(valor, ",")
		l.Fijar(enteroJS(cols), enteroJS(filas))
	default:
		return false // un comando que este lienzo no conoce: el contrato es ignorarlo
	}
	return true
}

// mandarDatos son los comandos de la pestaña Datos. Devuelve false si no es uno de
// ellos.
func (c *Lienzo) mandarDatos(cmd editor.Comando, valor string, c1, f1, c2, f2 int) bool {
	r, l := c.Rejilla, c.Libro
	switch cmd {
	case CmdOrdenar:
		l.Ordenar(valor == "desc", r.sel, c1, f1, c2, f2)
	case CmdDuplicados:
		n := l.QuitarDuplicados(c1, f1, c2, f2)
		if n == 0 {
			return true
		}
		r.sel, r.ext = Celda{C: c1, F: f1}, &Celda{C: c2, F: f2 - n}
		if c.AlAviso != nil {
			c.AlAviso(ui.AvisoInfo, c.t("sheet.msg.dupesRemoved", nil), c.t("sheet.msg.dupesRemoved.detail", i18n.Vars{"count": n}))
		}
	case CmdTextoCols:
		l.TextoEnColumnas(valor, c1, f1, f2)
	case CmdInsFila, CmdInsCol, CmdBorrarFilas, CmdBorrarCols:
		l.Estructura(string(cmd), valor, c1, f1, c2, f2)
		r.ext = nil
	case CmdAutofiltro:
		return l.AlternarAutofiltro(c1, f1, c2, f2)
	case CmdFiltrar:
		var req struct {
			Col    any  `json:"col"`
			Value  any  `json:"value"`
			Hidden bool `json:"hidden"`
		}
		if json.Unmarshal([]byte(valor), &req) != nil {
			return false
		}
		texto, _ := req.Value.(string) // `typeof req.value === 'string' ? req.value : ''`
		return l.Filtrar(enteroJS(req.Col), texto, req.Hidden)
	case CmdCondFiltro:
		var req struct {
			Col  any             `json:"col"`
			Cond json.RawMessage `json:"cond"`
		}
		if json.Unmarshal([]byte(valor), &req) != nil {
			return false
		}
		// Solo un `null` explícito quita la condición; una que no vale no toca nada.
		if strings.TrimSpace(string(req.Cond)) == "null" {
			return l.PonerCondicionFiltro(enteroJS(req.Col), nil)
		}
		var crudo any
		if json.Unmarshal(req.Cond, &crudo) != nil {
			return false
		}
		cond, ok := sanearCondFiltro(crudo)
		return ok && l.PonerCondicionFiltro(enteroJS(req.Col), &cond)
	case CmdOrdenarFiltro:
		var req struct {
			Col any    `json:"col"`
			Dir string `json:"dir"`
		}
		if json.Unmarshal([]byte(valor), &req) != nil {
			return false
		}
		return l.OrdenarFiltro(enteroJS(req.Col), req.Dir == "desc")
	default:
		return c.mandarReglas(cmd, valor) || c.mandarObjetos(cmd, valor)
	}
	return true
}

// mandarReglas son los comandos de las reglas de la hoja: formato condicional y
// validaciones de datos.
func (c *Lienzo) mandarReglas(cmd editor.Comando, valor string) bool {
	l := c.Libro
	switch cmd {
	case CmdCondAnadir:
		var crudo reglaCruda
		if json.Unmarshal([]byte(valor), &crudo) == nil {
			l.AnadirCondicion(crudo)
		}
	case CmdValAnadir:
		var crudo validacionCruda
		if json.Unmarshal([]byte(valor), &crudo) == nil {
			l.AnadirValidacion(crudo)
		}
	case CmdValQuitar:
		l.QuitarValidacion(enteroJS(valor))
	case CmdBloque:
		c1, f1, _, _ := c.Rejilla.Rango()
		l.ColocarBloque(valor, c1, f1)
	case CmdNombrePoner, CmdNombreQuitar:
		if codigo := l.MandarNombre(string(cmd), valor); codigo != "" && c.AlAviso != nil {
			c.AlAviso(ui.AvisoAdvertencia, c.t("names.title", nil), c.t(ClaveErrorNombre[codigo], nil))
		}
	case CmdCondQuitar:
		l.QuitarCondicion(enteroJS(valor))
	case CmdCondMover:
		i, dir, _ := strings.Cut(valor, ",")
		l.MoverCondicion(enteroJS(i), enteroJS(dir))
	default:
		return false
	}
	return true
}

// trasCambio es lo que sigue a cualquier cambio del libro: la rejilla se repinta, la
// barra de fórmulas y la de hojas se enteran, y el cromo también.
func (c *Lienzo) trasCambio() {
	c.sincronizar()
	c.Rejilla.Refresh()
	c.avisar()
}

// parcheJSON lee el valor del comando `style`: cada clave presente sustituye a la del
// estilo, y `null` la quita (`mergeCellStyle`).
func parcheJSON(valor string) (Parche, bool) {
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(valor), &m) != nil {
		return nil, false
	}
	var p estiloCrudo
	if json.Unmarshal([]byte(valor), &p) != nil {
		return nil, false
	}
	return func(c *estiloCrudo) {
		for k := range m {
			switch k {
			case "font":
				c.Fuente = p.Fuente
			case "size":
				c.Tam = p.Tam
			case "color":
				c.Tinta = p.Tinta
			case "fill":
				c.Relleno = p.Relleno
			case "u":
				c.Subraya = p.Subraya
			case "s":
				c.Tacha = p.Tacha
			case "valign":
				c.Vertical = p.Vertical
			case "wrap":
				c.Ajustar = p.Ajustar
			case "border":
				c.Bordes = p.Bordes
			case "locked":
				c.Bloq = p.Bloq
			case "hidden":
				c.Oculta = p.Oculta
			}
		}
	}, true
}

// EstadoCinta es el `SheetRibbonState` que la cinta pinta: el formato de número y el
// visual de la celda ancla, y los paneles inmovilizados de la hoja.
type EstadoCinta struct {
	Numero     FormatoNum
	Estilo     Estilo
	Fijas      Fijas
	Autofiltro bool
	// Protegida y ProtegidoLibro, con si piden contraseña para quitarlas.
	Protegida, ProtegidoLibro bool
	ConClave, ConClaveLibro   bool
	// Tabla es la tabla con formato que cubre la celda ancla (`table`), o nil.
	Tabla *TablaFmt
	// Seleccion es el rango en A1 (`selection`), con el que se prellenan los paneles.
	Seleccion string
}

// Estado es el estado de la celda ancla para la cinta.
func (c *Lienzo) Estado() EstadoCinta {
	s := c.Rejilla.sel
	e := EstadoCinta{
		Numero: c.Libro.FormatoNumero(s.C, s.F), Estilo: c.Libro.Estilo(s.C, s.F), Fijas: c.Libro.Fijas(),
		Autofiltro: c.Libro.Autofiltro() != nil,
	}
	if p := c.Libro.Proteccion(); p != nil {
		e.Protegida, e.ConClave = true, p.Password != ""
	}
	if p := c.Libro.ProteccionLibro(); p != nil {
		e.ProtegidoLibro, e.ConClaveLibro = true, p.Password != ""
	}
	if t, i := c.Libro.TablaEn(s.C, s.F); i >= 0 {
		e.Tabla = &t
	}
	e.Seleccion = rangoTexto(c.Rejilla)
	return e
}

// Formato es el `FormatState` de la celda ancla: negrita, cursiva y alineación del
// envoltorio, subrayado y tachado del estilo, y si se puede deshacer.
func (c *Lienzo) Formato() editor.Formato {
	s := c.Rejilla.sel
	b, i, a := c.Libro.Formato(s.C, s.F)
	e := c.Libro.Estilo(s.C, s.F)
	f := editor.Formato{
		Negrita: b, Cursiva: i, Subrayado: e.Subraya, Tachado: e.Tacha,
		PuedeDeshacer: c.Libro.PuedeDeshacer(), PuedeRehacer: c.Libro.PuedeRehacer(),
	}
	switch a {
	case "center":
		f.Alineacion = editor.Centro
	case "right":
		f.Alineacion = editor.Derecha
	}
	return f
}
