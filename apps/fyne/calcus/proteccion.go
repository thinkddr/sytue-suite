// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La protección de hoja y de libro de la web (`SheetProtection`, `WorkbookProtection` y
// los predicados puros de `sheets/format.ts`), con el mismo criterio de Excel: con la
// hoja protegida, una celda está BLOQUEADA salvo `locked: false`; y con el libro
// protegido no se añaden, borran, renombran, duplican ni reordenan hojas.
//
// ponytail: la contraseña se coteja EN CLARO, como en la web y como en Excel: frena un
// descuido, no a quien quiera saltársela. Para datos sensibles, al servidor.

// Proteccion es `SheetProtection`. Su PRESENCIA —aunque sea `{}`— es la hoja protegida,
// y solo se guarda lo que difiere del valor por defecto: `selectCells` solo si es
// false, y los permisos solo si están concedidos.
//
// La contraseña va en el documento, en claro, como en la web: es un cotejo para frenar
// descuidos, no un secreto que guardar.
type Proteccion struct {
	// Los nombres los fija `SheetProtection` de la web, en camelCase.
	Password    string `json:"password,omitempty"`
	SelectCells *bool  `json:"selectCells,omitempty"` //nolint:tagliatelle // la web
	FormatCells bool   `json:"formatCells,omitempty"` //nolint:tagliatelle // la web
	InsertRows  bool   `json:"insertRows,omitempty"`  //nolint:tagliatelle // la web
	DeleteRows  bool   `json:"deleteRows,omitempty"`  //nolint:tagliatelle // la web
	InsertCols  bool   `json:"insertCols,omitempty"`  //nolint:tagliatelle // la web
	DeleteCols  bool   `json:"deleteCols,omitempty"`  //nolint:tagliatelle // la web
	Sort        bool   `json:"sort,omitempty"`
	AutoFilter  bool   `json:"autoFilter,omitempty"` //nolint:tagliatelle // la web
	Objects     bool   `json:"objects,omitempty"`
	Scenarios   bool   `json:"scenarios,omitempty"`
}

// ProteccionLibro es `WorkbookProtection`: la estructura del libro, con contraseña
// opcional.
type ProteccionLibro struct {
	Password string `json:"password,omitempty"`
}

// Permisos son los del cuadro «Proteger hoja», en su orden (`PROTECT_PERMS`): la clave
// del modelo, el rótulo y el campo que la guarda.
var Permisos = []struct {
	Clave, Rotulo string
	Campo         func(*Proteccion) *bool
}{
	{"insertRows", "table.rowAbove", func(p *Proteccion) *bool { return &p.InsertRows }},
	{"deleteRows", "table.deleteRows", func(p *Proteccion) *bool { return &p.DeleteRows }},
	{"insertCols", "data.perm.insertCols", func(p *Proteccion) *bool { return &p.InsertCols }},
	{"deleteCols", "data.perm.deleteCols", func(p *Proteccion) *bool { return &p.DeleteCols }},
	{"sort", "data.perm.sort", func(p *Proteccion) *bool { return &p.Sort }},
	{"autoFilter", "data.perm.autoFilter", func(p *Proteccion) *bool { return &p.AutoFilter }},
	{"objects", "data.perm.objects", func(p *Proteccion) *bool { return &p.Objects }},
	{"scenarios", "data.perm.scenarios", func(p *Proteccion) *bool { return &p.Scenarios }},
}

// protegerDe es `sanitizeProtection`: nil si no es un objeto.
func protegerDe(crudo any) *Proteccion {
	m, ok := crudo.(map[string]any)
	if !ok {
		return nil
	}
	p := &Proteccion{FormatCells: m["formatCells"] == true}
	if s, ok := m["password"].(string); ok {
		p.Password = s
	}
	if m["selectCells"] == false {
		no := false
		p.SelectCells = &no
	}
	for _, pm := range Permisos {
		*pm.Campo(p) = m[pm.Clave] == true
	}
	return p
}

// protegerLibroDe es `sanitizeWorkbookProtection`.
func protegerLibroDe(crudo any) *ProteccionLibro {
	m, ok := crudo.(map[string]any)
	if !ok {
		return nil
	}
	p := &ProteccionLibro{}
	if s, ok := m["password"].(string); ok {
		p.Password = s
	}
	return p
}

// Proteccion es la de la hoja activa, o nil.
func (l *Libro) Proteccion() *Proteccion { return l.h().proteccion }

// ProteccionLibro es la de la estructura del libro, o nil.
func (l *Libro) ProteccionLibro() *ProteccionLibro { return l.f.proteccion }

// Proteger protege la hoja activa (`protect`); Desproteger la desprotege si la
// contraseña coincide (`unprotect`) y dice si lo ha hecho.
func (l *Libro) Proteger(p *Proteccion) {
	l.anotar()
	l.h().proteccion = p
}

func (l *Libro) Desproteger(clave string) bool {
	p := l.h().proteccion
	if p == nil {
		return true
	}
	if p.Password != "" && p.Password != clave {
		return false
	}
	l.anotar()
	l.h().proteccion = nil
	return true
}

// ProtegerLibro y DesprotegerLibro son `protectbook` y `unprotectbook`.
func (l *Libro) ProtegerLibro(p *ProteccionLibro) {
	l.anotar()
	l.f.proteccion = p
}

func (l *Libro) DesprotegerLibro(clave string) bool {
	p := l.f.proteccion
	if p == nil {
		return true
	}
	if p.Password != "" && p.Password != clave {
		return false
	}
	l.anotar()
	l.f.proteccion = nil
	return true
}

// desbloqueada es `canEditCell` con la hoja protegida: solo `locked: false`.
func (l *Libro) desbloqueada(c, f int) bool {
	b := l.Estilo(c, f).Bloqueada
	return b != nil && !*b
}

// FormulaOculta es `isFormulaHidden` con una fórmula dentro: hoja protegida, celda
// `hidden` y un «=…». Su fórmula no se enseña en la barra ni sale al copiar.
func (l *Libro) FormulaOculta(c, f int) bool {
	return l.Proteccion() != nil && l.Estilo(c, f).Oculta && esFormula(l.HTML(c, f))
}

// Editable es `canEditCellContent`: sin protección, siempre; con ella, solo lo
// desbloqueado cuya fórmula no esté oculta —editarla la revelaría—.
func (l *Libro) Editable(c, f int) bool {
	return l.Proteccion() == nil || (l.desbloqueada(c, f) && !l.FormulaOculta(c, f))
}

// Seleccionable es `canSelectAt`: sin «Seleccionar celdas», solo las desbloqueadas.
func (l *Libro) Seleccionable(c, f int) bool {
	p := l.Proteccion()
	return p == nil || p.SelectCells == nil || l.desbloqueada(c, f)
}

// rangoBloqueado es `rangeLocked`: con la hoja protegida, alguna celda bloqueada.
func (l *Libro) rangoBloqueado(c1, f1, c2, f2 int) bool {
	if l.Proteccion() == nil {
		return false
	}
	for f := f1; f <= f2; f++ {
		for c := c1; c <= c2; c++ {
			if !l.desbloqueada(c, f) {
				return true
			}
		}
	}
	return false
}

// Comandos vetados con la hoja protegida (`PROTECT_CONTENT_CMDS`, `PROTECT_FORMAT_CMDS`
// y `PROTECT_PERMISSION_CMDS`). Los de la cinta genérica van con su equivalente de la
// hoja: el subrayado del cromo es aquí un `style`.
var (
	cmdsContenido = map[editor.Comando]bool{
		CmdPegar: true, CmdDuplicados: true, CmdTextoCols: true, CmdCombinar: true, CmdSubtotal: true,
		CmdDinamica: true, CmdDinamicaAct: true, CmdDinamicaQuitarla: true,
		CmdObjetivo: true, CmdSolver: true, CmdTablaDatos: true, CmdEscAplicar: true, CmdEscInforme: true,
		CmdRellenar: true, CmdTablaAnadir: true, CmdBloque: true,
	}
	cmdsFormato = map[editor.Comando]bool{
		CmdFormatoNum: true, CmdDecimales: true, CmdEstilo: true, CmdBordes: true, CmdQuitarEstil: true, CmdAlinear: true,
		editor.Negrita: true, editor.Cursiva: true, editor.Subrayado: true, editor.Tachado: true, editor.SinFormato: true,
		editor.AlinearIzq: true, editor.AlinearCen: true, editor.AlinearDer: true,
	}
	cmdsPermiso = map[editor.Comando]func(*Proteccion) bool{
		CmdInsFila:       func(p *Proteccion) bool { return p.InsertRows },
		CmdBorrarFilas:   func(p *Proteccion) bool { return p.DeleteRows },
		CmdInsCol:        func(p *Proteccion) bool { return p.InsertCols },
		CmdBorrarCols:    func(p *Proteccion) bool { return p.DeleteCols },
		CmdOrdenar:       func(p *Proteccion) bool { return p.Sort },
		CmdAutofiltro:    func(p *Proteccion) bool { return p.AutoFilter },
		CmdFiltrar:       func(p *Proteccion) bool { return p.AutoFilter },
		CmdCondFiltro:    func(p *Proteccion) bool { return p.AutoFilter },
		CmdOrdenarFiltro: func(p *Proteccion) bool { return p.Sort },
		CmdCondAnadir:    func(p *Proteccion) bool { return p.Objects },
		CmdCondQuitar:    func(p *Proteccion) bool { return p.Objects },
		CmdCondMover:     func(p *Proteccion) bool { return p.Objects },
		CmdValAnadir:     func(p *Proteccion) bool { return p.Objects },
		CmdValQuitar:     func(p *Proteccion) bool { return p.Objects },
		CmdGrafico:       func(p *Proteccion) bool { return p.Objects },
		CmdMiniAnadir:    func(p *Proteccion) bool { return p.Objects },
		CmdMiniQuitar:    func(p *Proteccion) bool { return p.Objects },
		CmdSegAnadir:     func(p *Proteccion) bool { return p.Objects },
	}
)

// vetado dice si la hoja protegida no deja mandar ese comando.
func (l *Libro) vetado(cmd editor.Comando) bool {
	p := l.Proteccion()
	if p == nil {
		return false
	}
	if permiso, ok := cmdsPermiso[cmd]; ok {
		return !permiso(p)
	}
	return cmdsContenido[cmd] || (cmdsFormato[cmd] && !p.FormatCells)
}

// Los comandos de la protección.
const (
	CmdProteger         editor.Comando = "protect"       // JSON de una SheetProtection
	CmdDesproteger      editor.Comando = "unprotect"     // la contraseña tecleada
	CmdProtegerLibro    editor.Comando = "protectbook"   // JSON {password?}
	CmdDesprotegerLibro editor.Comando = "unprotectbook" // la contraseña tecleada
)

// mandarProteccion ejecuta los cuatro comandos de la protección. false si no es uno.
func (c *Lienzo) mandarProteccion(cmd editor.Comando, valor string) bool {
	var crudo any
	switch cmd {
	case CmdProteger, CmdProtegerLibro:
		if json.Unmarshal([]byte(valor), &crudo) != nil {
			return true
		}
	case CmdDesproteger, CmdDesprotegerLibro:
	default:
		return false
	}
	l := c.Libro
	switch cmd {
	case CmdProteger:
		l.Proteger(protegerDe(crudo))
		c.Rejilla.Cancelar() // cierra cualquier edición en curso
	case CmdProtegerLibro:
		l.ProtegerLibro(protegerLibroDe(crudo))
	case CmdDesproteger:
		if !l.Desproteger(valor) {
			c.avisar2(ui.AvisoAdvertencia, "sheet.msg.wrongPassword", "sheet.msg.unprotectSheetFailed")
			return true
		}
	default:
		if !l.DesprotegerLibro(valor) {
			c.avisar2(ui.AvisoAdvertencia, "sheet.msg.wrongPassword", "sheet.msg.unprotectBookFailed")
			return true
		}
	}
	c.trasCambio()
	return true
}

// avisar2 enseña un aviso de dos claves del catálogo.
func (c *Lienzo) avisar2(tono ui.TonoAviso, titulo, detalle string) {
	if c.AlAviso != nil {
		c.AlAviso(tono, c.t(titulo, nil), c.t(detalle, nil))
	}
}

// EsperaAvisoProtegida es el antirrebote del aviso «hoja protegida»: teclear encima de
// una celda bloqueada no llena la pila de avisos.
const EsperaAvisoProtegida = 1500 * time.Millisecond

// avisarProtegida es `notifyProtected`.
func (c *Lienzo) avisarProtegida(celda Celda) {
	if time.Since(c.avisoProtegida) < EsperaAvisoProtegida || c.AlAviso == nil {
		return
	}
	c.avisoProtegida = time.Now()
	c.AlAviso(ui.AvisoInfo, c.t("sheet.msg.protectedSheet", nil),
		c.t("sheet.msg.protectedCell", i18n.Vars{"ref": Ref(celda.C, celda.F)}))
}

// estructura es `guardStructure`: con el libro protegido, no, y se dice.
func (c *Lienzo) estructura() bool {
	if c.Libro.ProteccionLibro() == nil {
		return true
	}
	c.avisar2(ui.AvisoAdvertencia, "sheet.msg.protectedBook", "sheet.msg.unprotectToEdit")
	return false
}
